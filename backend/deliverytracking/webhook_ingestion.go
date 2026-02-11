package deliverytracking

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	webhookIngressSourceProviderSignalWebhook = "provider signal webhook"
)

type webhookIngestionPayload struct {
	IntentID               string          `json:"intentId"`
	ProviderDeliverySignal string          `json:"providerDeliverySignal"`
	ProviderObservedAtRaw  json.RawMessage `json:"providerObservedAt"`
	ProviderEventIDRaw     json.RawMessage `json:"providerEventId"`
}

type webhookIngestionRecord struct {
	sourceRecordID         string
	intentID               string
	providerDeliverySignal DeliverySignalClass
	providerObservedAt     sql.NullTime
	providerEventID        sql.NullString
	receivedAt             time.Time
	effectiveAt            time.Time
	ingressSource          string
}

type webhookCorrelationHandoffRecord struct {
	sourceRecordID         string
	intentID               string
	providerDeliverySignal DeliverySignalClass
	providerObservedAt     sql.NullTime
	providerEventID        sql.NullString
	receivedAt             time.Time
	effectiveAt            time.Time
	ingressSource          string
}

type webhookCorrelationHandoffWriter func(context.Context, *sql.Tx, webhookCorrelationHandoffRecord) error

// InvalidWebhookPayloadError reports structurally invalid webhook payloads.
type InvalidWebhookPayloadError struct {
	Message string
}

func (e InvalidWebhookPayloadError) Error() string {
	return e.Message
}

// WebhookIngestionResult is the normalized output for accepted webhook payloads.
type WebhookIngestionResult struct {
	SourceRecordID         string
	IntentID               string
	ProviderDeliverySignal DeliverySignalClass
	ProviderObservedAt     *time.Time
	ProviderEventID        string
	ReceivedAt             time.Time
	EffectiveAt            time.Time
	IngressSource          string
}

// WebhookIngestor validates, normalizes, and persists delivery webhook ingress records.
type WebhookIngestor struct {
	store                   *sqlStore
	writeCorrelationHandoff webhookCorrelationHandoffWriter
}

// NewWebhookIngestor constructs a webhook ingestor backed by SQL persistence.
func NewWebhookIngestor(db *sql.DB) (*WebhookIngestor, error) {
	store, err := newSQLStore(db)
	if err != nil {
		return nil, err
	}
	return &WebhookIngestor{
		store:                   store,
		writeCorrelationHandoff: store.insertWebhookCorrelationHandoff,
	}, nil
}

// IngestProviderSignalWebhook applies webhook-ingestion validation, normalization, and persistence/handoff atomically.
func (i *WebhookIngestor) IngestProviderSignalWebhook(ctx context.Context, rawPayload []byte) (WebhookIngestionResult, error) {
	if i == nil || i.store == nil {
		return WebhookIngestionResult{}, WrapProcessingStageError(ProcessingStageWebhookIngestion, errors.New("webhook ingestor store is required"))
	}
	normalized, err := normalizeWebhookIngestionPayload(rawPayload)
	if err != nil {
		return WebhookIngestionResult{}, err
	}
	if i.writeCorrelationHandoff == nil {
		return WebhookIngestionResult{}, WrapProcessingStageError(ProcessingStageCorrelationHandoff, errors.New("webhook correlation handoff writer is required"))
	}

	if ctx == nil {
		ctx = context.Background()
	}
	tx, err := i.store.db.BeginTx(ctx, nil)
	if err != nil {
		return WebhookIngestionResult{}, WrapProcessingStageError(ProcessingStageWebhookIngestion, err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	receivedAt, err := loadSQLTimeTx(ctx, tx)
	if err != nil {
		return WebhookIngestionResult{}, WrapProcessingStageError(ProcessingStageWebhookIngestion, err)
	}
	sourceRecordID, err := newWebhookSourceRecordID()
	if err != nil {
		return WebhookIngestionResult{}, WrapProcessingStageError(ProcessingStageWebhookIngestion, err)
	}

	effectiveAt := receivedAt
	providerObservedAt := sql.NullTime{}
	if normalized.providerObservedAt != nil {
		providerObservedAt = sql.NullTime{Time: normalized.providerObservedAt.UTC(), Valid: true}
		effectiveAt = normalizeDBTime(normalized.providerObservedAt.UTC())
	}
	providerEventID := nullString(normalized.providerEventID)

	ingestionRecord := webhookIngestionRecord{
		sourceRecordID:         sourceRecordID,
		intentID:               normalized.intentID,
		providerDeliverySignal: normalized.providerDeliverySignal,
		providerObservedAt:     providerObservedAt,
		providerEventID:        providerEventID,
		receivedAt:             receivedAt,
		effectiveAt:            effectiveAt,
		ingressSource:          webhookIngressSourceProviderSignalWebhook,
	}
	if err := i.store.insertWebhookIngestionRecord(ctx, tx, ingestionRecord); err != nil {
		return WebhookIngestionResult{}, WrapProcessingStageError(ProcessingStageWebhookIngestion, err)
	}

	handoffRecord := webhookCorrelationHandoffRecord{
		sourceRecordID:         sourceRecordID,
		intentID:               normalized.intentID,
		providerDeliverySignal: normalized.providerDeliverySignal,
		providerObservedAt:     providerObservedAt,
		providerEventID:        providerEventID,
		receivedAt:             receivedAt,
		effectiveAt:            effectiveAt,
		ingressSource:          webhookIngressSourceProviderSignalWebhook,
	}
	if err := i.writeCorrelationHandoff(ctx, tx, handoffRecord); err != nil {
		return WebhookIngestionResult{}, WrapProcessingStageError(ProcessingStageCorrelationHandoff, err)
	}

	if err := tx.Commit(); err != nil {
		return WebhookIngestionResult{}, WrapProcessingStageError(ProcessingStageCorrelationHandoff, err)
	}

	result := WebhookIngestionResult{
		SourceRecordID:         sourceRecordID,
		IntentID:               normalized.intentID,
		ProviderDeliverySignal: normalized.providerDeliverySignal,
		ProviderEventID:        normalized.providerEventID,
		ReceivedAt:             receivedAt,
		EffectiveAt:            effectiveAt,
		IngressSource:          webhookIngressSourceProviderSignalWebhook,
	}
	if providerObservedAt.Valid {
		value := normalizeDBTime(providerObservedAt.Time)
		result.ProviderObservedAt = &value
	}
	return result, nil
}

type normalizedWebhookIngestionPayload struct {
	intentID               string
	providerDeliverySignal DeliverySignalClass
	providerObservedAt     *time.Time
	providerEventID        string
}

func normalizeWebhookIngestionPayload(rawPayload []byte) (normalizedWebhookIngestionPayload, error) {
	if strings.TrimSpace(string(rawPayload)) == "" {
		return normalizedWebhookIngestionPayload{}, InvalidWebhookPayloadError{Message: "invalid request body"}
	}

	var payload webhookIngestionPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return normalizedWebhookIngestionPayload{}, InvalidWebhookPayloadError{Message: "invalid request body"}
	}

	intentID := strings.TrimSpace(payload.IntentID)
	if intentID == "" {
		return normalizedWebhookIngestionPayload{}, InvalidWebhookPayloadError{Message: "intentId is required"}
	}

	providerDeliverySignal := strings.TrimSpace(payload.ProviderDeliverySignal)
	var signalClass DeliverySignalClass
	switch providerDeliverySignal {
	case string(DeliverySignalInProgress):
		signalClass = DeliverySignalInProgress
	case string(DeliverySignalTerminalSuccess):
		signalClass = DeliverySignalTerminalSuccess
	case string(DeliverySignalTerminalFailure):
		signalClass = DeliverySignalTerminalFailure
	default:
		return normalizedWebhookIngestionPayload{}, InvalidWebhookPayloadError{Message: "providerDeliverySignal is required and must be one of: in_progress, terminal_success, terminal_failure"}
	}

	var providerObservedAt *time.Time
	if value, ok := parseOptionalObservedAt(payload.ProviderObservedAtRaw); ok {
		providerObservedAt = &value
	}

	providerEventID := parseOptionalString(payload.ProviderEventIDRaw)

	return normalizedWebhookIngestionPayload{
		intentID:               intentID,
		providerDeliverySignal: signalClass,
		providerObservedAt:     providerObservedAt,
		providerEventID:        providerEventID,
	}, nil
}

func parseOptionalObservedAt(raw json.RawMessage) (time.Time, bool) {
	if len(raw) == 0 {
		return time.Time{}, false
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return time.Time{}, false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return time.Time{}, false
	}
	return normalizeDBTime(parsed.UTC()), true
}

func parseOptionalString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return ""
	}
	return strings.TrimSpace(text)
}

func loadSQLTimeTx(ctx context.Context, tx *sql.Tx) (time.Time, error) {
	row := tx.QueryRowContext(ctx, `SELECT SYSUTCDATETIME()`)
	var now time.Time
	if err := row.Scan(&now); err != nil {
		return time.Time{}, err
	}
	return normalizeDBTime(now), nil
}

func newWebhookSourceRecordID() (string, error) {
	value := make([]byte, 10)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return fmt.Sprintf("dws-%s", hex.EncodeToString(value)), nil
}

func (s *sqlStore) insertWebhookIngestionRecord(ctx context.Context, tx *sql.Tx, record webhookIngestionRecord) error {
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO dbo.intent_delivery_webhook_ingestion (
      source_record_id,
      intent_id,
      provider_delivery_signal,
      provider_observed_at,
      provider_event_id,
      received_at,
      effective_at,
      ingress_source,
      created_at
    ) VALUES (
      @p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9
    )`,
		record.sourceRecordID,
		record.intentID,
		string(record.providerDeliverySignal),
		record.providerObservedAt,
		record.providerEventID,
		record.receivedAt,
		record.effectiveAt,
		record.ingressSource,
		record.receivedAt,
	)
	return err
}

func (s *sqlStore) insertWebhookCorrelationHandoff(ctx context.Context, tx *sql.Tx, record webhookCorrelationHandoffRecord) error {
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO dbo.intent_delivery_correlation_handoff (
      source_record_id,
      intent_id,
      provider_delivery_signal,
      provider_observed_at,
      provider_event_id,
      received_at,
      effective_at,
      ingress_source,
      created_at
    ) VALUES (
      @p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9
    )`,
		record.sourceRecordID,
		record.intentID,
		string(record.providerDeliverySignal),
		record.providerObservedAt,
		record.providerEventID,
		record.receivedAt,
		record.effectiveAt,
		record.ingressSource,
		record.receivedAt,
	)
	return err
}
