package deliverytracking

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestWebhookIngestRequiredFieldValidation(t *testing.T) {
	ingestor, db := newWebhookIngestor(t)
	cases := []struct {
		name string
		body string
	}{
		{
			name: "missing intentId",
			body: `{"providerDeliverySignal":"in_progress"}`,
		},
		{
			name: "missing providerDeliverySignal",
			body: `{"intentId":"intent-1"}`,
		},
		{
			name: "invalid providerDeliverySignal",
			body: `{"intentId":"intent-1","providerDeliverySignal":"done"}`,
		},
		{
			name: "invalid body",
			body: `{"intentId":"intent-1",`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ingestor.IngestProviderSignalWebhook(context.Background(), []byte(tc.body))
			if err == nil {
				t.Fatal("expected validation error")
			}
			var invalid InvalidWebhookPayloadError
			if !errors.As(err, &invalid) {
				t.Fatalf("expected InvalidWebhookPayloadError, got %T: %v", err, err)
			}
			if countWebhookIngestionRows(t, db, "intent-1") != 0 {
				t.Fatalf("expected no ingestion rows for invalid payload")
			}
			if countWebhookCorrelationHandoffRows(t, db, "intent-1") != 0 {
				t.Fatalf("expected no handoff rows for invalid payload")
			}
		})
	}
}

func TestWebhookIngestMalformedProviderObservedAtFallbackAccepted(t *testing.T) {
	ingestor, db := newWebhookIngestor(t)
	result, err := ingestor.IngestProviderSignalWebhook(context.Background(), []byte(`{
  "intentId":"intent-fallback",
  "providerDeliverySignal":"in_progress",
  "providerObservedAt":"not-a-time"
}`))
	if err != nil {
		t.Fatalf("ingest webhook: %v", err)
	}
	if result.ProviderObservedAt != nil {
		t.Fatalf("expected providerObservedAt omitted on malformed input, got %v", result.ProviderObservedAt)
	}
	if !result.EffectiveAt.Equal(result.ReceivedAt) {
		t.Fatalf("expected effectiveAt to fall back to receivedAt, got effectiveAt=%s receivedAt=%s", result.EffectiveAt, result.ReceivedAt)
	}
	row := loadWebhookIngestionRowBySource(t, db, result.SourceRecordID)
	if row.providerObservedAt.Valid {
		t.Fatalf("expected provider_observed_at NULL for malformed input, got %+v", row.providerObservedAt)
	}
	if !normalizeDBTime(row.effectiveAt).Equal(normalizeDBTime(row.receivedAt)) {
		t.Fatalf("expected stored effective_at to match received_at on fallback")
	}
}

func TestWebhookIngestNormalizationUTCAndTimestampPrecedence(t *testing.T) {
	ingestor, db := newWebhookIngestor(t)

	withObserved, err := ingestor.IngestProviderSignalWebhook(context.Background(), []byte(`{
  "intentId":"intent-observed",
  "providerDeliverySignal":"terminal_success",
  "providerObservedAt":"2026-02-11T09:15:45+05:30",
  "providerEventId":"evt-1"
}`))
	if err != nil {
		t.Fatalf("ingest webhook with providerObservedAt: %v", err)
	}
	if withObserved.ProviderObservedAt == nil {
		t.Fatal("expected providerObservedAt to be present")
	}
	expectedObservedUTC := time.Date(2026, 2, 11, 3, 45, 45, 0, time.UTC)
	if !withObserved.ProviderObservedAt.Equal(expectedObservedUTC) {
		t.Fatalf("expected providerObservedAt UTC %s, got %s", expectedObservedUTC, withObserved.ProviderObservedAt.UTC())
	}
	if !withObserved.EffectiveAt.Equal(expectedObservedUTC) {
		t.Fatalf("expected effectiveAt to use providerObservedAt, got %s", withObserved.EffectiveAt.UTC())
	}
	if withObserved.ReceivedAt.Location() != time.UTC {
		t.Fatalf("expected receivedAt UTC, got %s", withObserved.ReceivedAt.Location())
	}
	handoffObserved := loadWebhookCorrelationHandoffRowBySource(t, db, withObserved.SourceRecordID)
	if !handoffObserved.providerObservedAt.Valid {
		t.Fatalf("expected handoff provider_observed_at set")
	}
	if !normalizeDBTime(handoffObserved.effectiveAt).Equal(expectedObservedUTC) {
		t.Fatalf("expected handoff effective_at to use providerObservedAt, got %s", normalizeDBTime(handoffObserved.effectiveAt))
	}
	if handoffObserved.ingressSource != webhookIngressSourceProviderSignalWebhook {
		t.Fatalf("expected ingress source %q, got %q", webhookIngressSourceProviderSignalWebhook, handoffObserved.ingressSource)
	}

	withoutObserved, err := ingestor.IngestProviderSignalWebhook(context.Background(), []byte(`{
  "intentId":"intent-no-observed",
  "providerDeliverySignal":"terminal_failure"
}`))
	if err != nil {
		t.Fatalf("ingest webhook without providerObservedAt: %v", err)
	}
	if withoutObserved.ProviderObservedAt != nil {
		t.Fatalf("expected nil providerObservedAt, got %v", withoutObserved.ProviderObservedAt)
	}
	if !withoutObserved.EffectiveAt.Equal(withoutObserved.ReceivedAt) {
		t.Fatalf("expected effectiveAt to fall back to receivedAt, got effectiveAt=%s receivedAt=%s", withoutObserved.EffectiveAt, withoutObserved.ReceivedAt)
	}
	handoffNoObserved := loadWebhookCorrelationHandoffRowBySource(t, db, withoutObserved.SourceRecordID)
	if handoffNoObserved.providerObservedAt.Valid {
		t.Fatalf("expected handoff provider_observed_at NULL")
	}
	if !normalizeDBTime(handoffNoObserved.effectiveAt).Equal(normalizeDBTime(handoffNoObserved.receivedAt)) {
		t.Fatalf("expected handoff effective_at to equal received_at when providerObservedAt absent")
	}
}

func TestWebhookIngestPersistenceAndHandoffAreAtomic(t *testing.T) {
	ingestor, db := newWebhookIngestor(t)
	ingestor.writeCorrelationHandoff = func(ctx context.Context, tx *sql.Tx, record webhookCorrelationHandoffRecord) error {
		_ = ctx
		_ = tx
		_ = record
		return errors.New("forced handoff failure")
	}

	_, err := ingestor.IngestProviderSignalWebhook(context.Background(), []byte(`{
  "intentId":"intent-atomic",
  "providerDeliverySignal":"in_progress"
}`))
	if err == nil {
		t.Fatal("expected handoff failure")
	}
	if countWebhookIngestionRows(t, db, "intent-atomic") != 0 {
		t.Fatalf("expected no ingestion rows after rollback")
	}
	if countWebhookCorrelationHandoffRows(t, db, "intent-atomic") != 0 {
		t.Fatalf("expected no handoff rows after rollback")
	}
}

func TestWebhookIngestDuplicateReplayAccepted(t *testing.T) {
	ingestor, db := newWebhookIngestor(t)
	payload := []byte(`{
  "intentId":"intent-dup",
  "providerDeliverySignal":"in_progress",
  "providerEventId":"evt-dup"
}`)

	first, err := ingestor.IngestProviderSignalWebhook(context.Background(), payload)
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	second, err := ingestor.IngestProviderSignalWebhook(context.Background(), payload)
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if first.SourceRecordID == second.SourceRecordID {
		t.Fatalf("expected unique sourceRecordId per accepted payload instance")
	}
	if countWebhookIngestionRows(t, db, "intent-dup") != 2 {
		t.Fatalf("expected duplicate/replay payload to produce 2 ingestion rows")
	}
	if countWebhookCorrelationHandoffRows(t, db, "intent-dup") != 2 {
		t.Fatalf("expected duplicate/replay payload to produce 2 handoff rows")
	}
}

func newWebhookIngestor(t *testing.T) (*WebhookIngestor, *sql.DB) {
	t.Helper()
	db := newTestDB(t)
	ingestor, err := NewWebhookIngestor(db)
	if err != nil {
		t.Fatalf("new webhook ingestor: %v", err)
	}
	return ingestor, db
}

func countWebhookIngestionRows(t *testing.T, db *sql.DB, intentID string) int {
	t.Helper()
	row := db.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM dbo.intent_delivery_webhook_ingestion WHERE intent_id = @p1`, intentID)
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count ingestion rows: %v", err)
	}
	return count
}

func countWebhookCorrelationHandoffRows(t *testing.T, db *sql.DB, intentID string) int {
	t.Helper()
	row := db.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM dbo.intent_delivery_correlation_handoff WHERE intent_id = @p1`, intentID)
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count handoff rows: %v", err)
	}
	return count
}

type webhookIngestionRow struct {
	providerObservedAt sql.NullTime
	receivedAt         time.Time
	effectiveAt        time.Time
}

func loadWebhookIngestionRowBySource(t *testing.T, db *sql.DB, sourceRecordID string) webhookIngestionRow {
	t.Helper()
	row := db.QueryRowContext(
		context.Background(),
		`SELECT provider_observed_at, received_at, effective_at
     FROM dbo.intent_delivery_webhook_ingestion
     WHERE source_record_id = @p1`,
		sourceRecordID,
	)
	var result webhookIngestionRow
	if err := row.Scan(&result.providerObservedAt, &result.receivedAt, &result.effectiveAt); err != nil {
		t.Fatalf("load ingestion row: %v", err)
	}
	return result
}

type webhookCorrelationHandoffRow struct {
	providerObservedAt sql.NullTime
	receivedAt         time.Time
	effectiveAt        time.Time
	ingressSource      string
}

func loadWebhookCorrelationHandoffRowBySource(t *testing.T, db *sql.DB, sourceRecordID string) webhookCorrelationHandoffRow {
	t.Helper()
	row := db.QueryRowContext(
		context.Background(),
		`SELECT provider_observed_at, received_at, effective_at, ingress_source
     FROM dbo.intent_delivery_correlation_handoff
     WHERE source_record_id = @p1`,
		sourceRecordID,
	)
	var result webhookCorrelationHandoffRow
	if err := row.Scan(&result.providerObservedAt, &result.receivedAt, &result.effectiveAt, &result.ingressSource); err != nil {
		t.Fatalf("load handoff row: %v", err)
	}
	return result
}
