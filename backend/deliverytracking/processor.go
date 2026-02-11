package deliverytracking

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	mssql "github.com/microsoft/go-mssqldb"

	"gateway/submission"
)

// SubmissionStatus is the persisted submission status used for late-observation checks.
type SubmissionStatus string

const (
	SubmissionStatusAccepted  SubmissionStatus = "accepted"
	SubmissionStatusRejected  SubmissionStatus = "rejected"
	SubmissionStatusExhausted SubmissionStatus = "exhausted"
)

// Processor applies correlated delivery records to durable state/history.
type Processor struct {
	store *sqlStore
}

type sqlStore struct {
	db                              *sql.DB
	deliveryApplyAfterHistoryInsert func() error
	metrics                         *Metrics
}

// NewProcessor creates a delivery processor backed by the provided SQL DB.
func NewProcessor(db *sql.DB) (*Processor, error) {
	store, err := newSQLStore(db)
	if err != nil {
		return nil, err
	}
	return &Processor{store: store}, nil
}

// NewProcessorWithMetrics creates a delivery processor with observability metrics.
func NewProcessorWithMetrics(db *sql.DB, metrics *Metrics) (*Processor, error) {
	store, err := newSQLStoreWithMetrics(db, metrics)
	if err != nil {
		return nil, err
	}
	return &Processor{store: store}, nil
}

func newSQLStore(db *sql.DB) (*sqlStore, error) {
	return newSQLStoreWithMetrics(db, nil)
}

func newSQLStoreWithMetrics(db *sql.DB, metrics *Metrics) (*sqlStore, error) {
	if db == nil {
		return nil, errors.New("db is required")
	}
	return &sqlStore{db: db, metrics: metrics}, nil
}

// DeliveryCorrelationResult is the deterministic output from intent correlation.
type DeliveryCorrelationResult string

const (
	DeliveryCorrelationMatched   DeliveryCorrelationResult = "matched"
	DeliveryCorrelationUnmatched DeliveryCorrelationResult = "unmatched"
	DeliveryCorrelationInvalid   DeliveryCorrelationResult = "invalid"
)

// DeliverySignalClass is the normalized provider signal class consumed by core processing.
type DeliverySignalClass string

const (
	DeliverySignalInProgress      DeliverySignalClass = "in_progress"
	DeliverySignalTerminalSuccess DeliverySignalClass = "terminal_success"
	DeliverySignalTerminalFailure DeliverySignalClass = "terminal_failure"
)

// DeliveryStatus is the canonical current delivery status projection.
type DeliveryStatus string

const (
	DeliveryStatusUnknown    DeliveryStatus = "unknown"
	DeliveryStatusInProgress DeliveryStatus = "in_progress"
	DeliveryStatusDelivered  DeliveryStatus = "delivered"
	DeliveryStatusFailed     DeliveryStatus = "failed"
)

// DeliveryFreshness is the canonical current freshness projection.
type DeliveryFreshness string

const (
	DeliveryFreshnessFresh         DeliveryFreshness = "fresh"
	DeliveryFreshnessStale         DeliveryFreshness = "stale"
	DeliveryFreshnessNotApplicable DeliveryFreshness = "not_applicable"
)

const (
	deliveryNoOpReasonCorrelationUnmatched = "correlation_unmatched"
	deliveryNoOpReasonCorrelationInvalid   = "correlation_invalid"
	deliveryNoOpReasonModeOff              = "mode_off"
	deliveryNoOpReasonDuplicateSource      = "duplicate_source_record"
)

// CorrelatedDeliveryRecord is the normalized input for delivery core processing.
type CorrelatedDeliveryRecord struct {
	SourceRecordID     string
	ProviderEventID    string
	IntentID           string
	SignalClass        DeliverySignalClass
	ProviderObservedAt *time.Time
	ReceivedAt         time.Time
	CorrelationResult  DeliveryCorrelationResult
}

// DeliveryApplyResult reports whether history and/or current state changed.
type DeliveryApplyResult struct {
	HistoryInserted  bool
	StateMutated     bool
	NoOpReason       string
	LateObservation  bool
	TerminalConflict bool
}

type normalizedCorrelatedDeliveryRecord struct {
	sourceRecordID     string
	providerEventID    string
	intentID           string
	signalClass        DeliverySignalClass
	providerObservedAt sql.NullTime
	receivedAt         time.Time
	effectiveAt        time.Time
	correlationResult  DeliveryCorrelationResult
}

type deliveryIntentSnapshot struct {
	submissionTarget  string
	status            SubmissionStatus
	completedAt       time.Time
	createdAt         time.Time
	deliveryMode      submission.DeliveryTrackingMode
	staleAfterSeconds int
}

type deliveryStateProjection struct {
	deliveryStatus                DeliveryStatus
	deliveryFreshness             DeliveryFreshness
	lastNonTerminalEffectiveAt    sql.NullTime
	lastNonTerminalReceivedAt     sql.NullTime
	lastNonTerminalSourceRecordID sql.NullString
	terminalLocked                bool
	terminalStatus                sql.NullString
	terminalEffectiveAt           sql.NullTime
	terminalReceivedAt            sql.NullTime
	terminalSourceRecordID        sql.NullString
	staleAfterSeconds             int
	staleAt                       sql.NullTime
}

type deliveryOrderingKey struct {
	effectiveAt    time.Time
	receivedAt     time.Time
	sourceRecordID string
}

// ApplyCorrelatedDeliveryRecord applies a correlated delivery record to delivery state/history.
func (p *Processor) ApplyCorrelatedDeliveryRecord(ctx context.Context, record CorrelatedDeliveryRecord) (DeliveryApplyResult, error) {
	if p == nil || p.store == nil {
		return DeliveryApplyResult{}, errors.New("processor store is required")
	}
	return p.store.applyCorrelatedDeliveryRecord(ctx, record)
}

// EvaluateDeliveryFreshnessStaleness marks due unresolved intents as stale.
func (p *Processor) EvaluateDeliveryFreshnessStaleness(ctx context.Context) (int64, error) {
	if p == nil || p.store == nil {
		return 0, errors.New("processor store is required")
	}
	return p.store.evaluateDeliveryFreshnessStaleness(ctx)
}

func (s *sqlStore) applyCorrelatedDeliveryRecord(ctx context.Context, record CorrelatedDeliveryRecord) (result DeliveryApplyResult, err error) {
	intentID := strings.TrimSpace(record.IntentID)
	submissionTarget := "unknown"
	deliveryTrackingMode := "unknown"
	correlationResult := canonicalCorrelationResult(record.CorrelationResult)

	defer func() {
		if err == nil {
			return
		}
		reason := MapProcessingFailureReason(err)
		s.observeProcessingFailure(ProcessingStageCoreProcessing, reason)
		log.Printf(
			"event=delivery_processing_failure stage=%q reason=%q source=%q intentId=%q submissionTarget=%q deliveryTrackingMode=%q correlationResult=%q",
			ProcessingStageCoreProcessing,
			reason,
			webhookIngressSourceProviderSignalWebhook,
			intentID,
			submissionTarget,
			deliveryTrackingMode,
			correlationResult,
		)
	}()

	normalized, err := normalizeCorrelatedDeliveryRecord(record)
	if err != nil {
		return DeliveryApplyResult{}, err
	}

	intentID = normalized.intentID
	correlationResult = canonicalCorrelationResult(normalized.correlationResult)
	s.observeProviderSignal(webhookIngressSourceProviderSignalWebhook, normalized.correlationResult)

	switch normalized.correlationResult {
	case DeliveryCorrelationUnmatched:
		result = DeliveryApplyResult{NoOpReason: deliveryNoOpReasonCorrelationUnmatched}
		s.observeIgnoredSignal(result.NoOpReason)
		log.Printf(
			"event=delivery_correlation_result source=%q intentId=%q submissionTarget=%q deliveryTrackingMode=%q correlationResult=%q reason=%q",
			webhookIngressSourceProviderSignalWebhook,
			intentID,
			submissionTarget,
			deliveryTrackingMode,
			correlationResult,
			result.NoOpReason,
		)
		return result, nil
	case DeliveryCorrelationInvalid:
		result = DeliveryApplyResult{NoOpReason: deliveryNoOpReasonCorrelationInvalid}
		s.observeIgnoredSignal(result.NoOpReason)
		log.Printf(
			"event=delivery_correlation_result source=%q intentId=%q submissionTarget=%q deliveryTrackingMode=%q correlationResult=%q reason=%q",
			webhookIngressSourceProviderSignalWebhook,
			intentID,
			submissionTarget,
			deliveryTrackingMode,
			correlationResult,
			result.NoOpReason,
		)
		return result, nil
	case DeliveryCorrelationMatched:
	default:
		return DeliveryApplyResult{}, fmt.Errorf("unknown correlationResult %q", normalized.correlationResult)
	}

	if ctx == nil {
		ctx = context.Background()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DeliveryApplyResult{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	intent, found, err := loadDeliveryIntentSnapshotForUpdate(ctx, tx, normalized.intentID)
	if err != nil {
		return DeliveryApplyResult{}, err
	}
	if !found {
		return DeliveryApplyResult{}, errors.New("intent not found")
	}
	submissionTarget = intent.submissionTarget
	deliveryTrackingMode = string(intent.deliveryMode)

	if intent.deliveryMode != submission.DeliveryTrackingModeOn {
		if err := tx.Commit(); err != nil {
			return DeliveryApplyResult{}, err
		}
		result = DeliveryApplyResult{NoOpReason: deliveryNoOpReasonModeOff}
		s.observeIgnoredSignal(result.NoOpReason)
		log.Printf(
			"event=delivery_signal_ignored source=%q intentId=%q submissionTarget=%q deliveryTrackingMode=%q correlationResult=%q reason=%q",
			webhookIngressSourceProviderSignalWebhook,
			intentID,
			submissionTarget,
			deliveryTrackingMode,
			correlationResult,
			result.NoOpReason,
		)
		return result, nil
	}
	if intent.staleAfterSeconds <= 0 {
		return DeliveryApplyResult{}, errors.New("deliveryTracking.staleAfterSeconds must be greater than zero when mode=on")
	}

	state, stateFound, err := loadDeliveryStateForUpdate(ctx, tx, normalized.intentID)
	if err != nil {
		return DeliveryApplyResult{}, err
	}

	dup, err := insertDeliveryHistoryRow(ctx, tx, normalized)
	if err != nil {
		return DeliveryApplyResult{}, err
	}
	if dup {
		if err := tx.Commit(); err != nil {
			return DeliveryApplyResult{}, err
		}
		result = DeliveryApplyResult{NoOpReason: deliveryNoOpReasonDuplicateSource}
		s.observeIgnoredSignal(result.NoOpReason)
		log.Printf(
			"event=delivery_signal_ignored source=%q intentId=%q submissionTarget=%q deliveryTrackingMode=%q correlationResult=%q reason=%q",
			webhookIngressSourceProviderSignalWebhook,
			intentID,
			submissionTarget,
			deliveryTrackingMode,
			correlationResult,
			result.NoOpReason,
		)
		return result, nil
	}

	if s.deliveryApplyAfterHistoryInsert != nil {
		if err := s.deliveryApplyAfterHistoryInsert(); err != nil {
			return DeliveryApplyResult{}, err
		}
	}

	if !stateFound {
		if err := insertInitialDeliveryState(ctx, tx, normalized.intentID, intent.createdAt, intent.staleAfterSeconds, normalized.receivedAt); err != nil {
			return DeliveryApplyResult{}, err
		}
		state, stateFound, err = loadDeliveryStateForUpdate(ctx, tx, normalized.intentID)
		if err != nil {
			return DeliveryApplyResult{}, err
		}
		if !stateFound {
			return DeliveryApplyResult{}, errors.New("delivery state row missing after initialization")
		}
	}

	previousStatus := state.deliveryStatus
	previousFreshness := state.deliveryFreshness
	result = DeliveryApplyResult{HistoryInserted: true}
	key := deliveryOrderingKey{
		effectiveAt:    normalized.effectiveAt,
		receivedAt:     normalized.receivedAt,
		sourceRecordID: normalized.sourceRecordID,
	}

	switch normalized.signalClass {
	case DeliverySignalInProgress:
		if !state.terminalLocked && isNewerNonTerminalSignal(state, key) {
			if state.deliveryStatus == DeliveryStatusUnknown || state.deliveryStatus == DeliveryStatusInProgress {
				state.deliveryStatus = DeliveryStatusInProgress
			}
			state.deliveryFreshness = DeliveryFreshnessFresh
			state.lastNonTerminalEffectiveAt = sql.NullTime{Time: key.effectiveAt, Valid: true}
			state.lastNonTerminalReceivedAt = sql.NullTime{Time: key.receivedAt, Valid: true}
			state.lastNonTerminalSourceRecordID = sql.NullString{String: key.sourceRecordID, Valid: true}
			staleAt := key.effectiveAt.Add(time.Duration(state.staleAfterSeconds) * time.Second)
			state.staleAt = sql.NullTime{Time: staleAt, Valid: true}
			result.StateMutated = true
		}
	case DeliverySignalTerminalSuccess, DeliverySignalTerminalFailure:
		result.LateObservation = isLateTerminalObservation(intent.status, intent.completedAt, normalized.receivedAt)
		incomingStatus := terminalStatusForSignal(normalized.signalClass)
		if !state.terminalLocked {
			state.deliveryStatus = incomingStatus
			state.deliveryFreshness = DeliveryFreshnessNotApplicable
			state.terminalLocked = true
			state.terminalStatus = sql.NullString{String: string(incomingStatus), Valid: true}
			state.terminalEffectiveAt = sql.NullTime{Time: key.effectiveAt, Valid: true}
			state.terminalReceivedAt = sql.NullTime{Time: key.receivedAt, Valid: true}
			state.terminalSourceRecordID = sql.NullString{String: key.sourceRecordID, Valid: true}
			state.staleAt = sql.NullTime{}
			result.StateMutated = true
		} else if state.terminalStatus.Valid && DeliveryStatus(state.terminalStatus.String) != incomingStatus {
			result.TerminalConflict = true
		}
	default:
		return DeliveryApplyResult{}, fmt.Errorf("unknown signalClass %q", normalized.signalClass)
	}

	if result.StateMutated {
		if err := updateDeliveryState(ctx, tx, normalized.intentID, state, normalized.receivedAt); err != nil {
			return DeliveryApplyResult{}, err
		}
	}

	if result.LateObservation || result.TerminalConflict {
		if err := updateDeliveryHistoryAnnotations(ctx, tx, normalized.intentID, normalized.sourceRecordID, result.LateObservation, result.TerminalConflict); err != nil {
			return DeliveryApplyResult{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return DeliveryApplyResult{}, err
	}

	if result.StateMutated {
		if previousStatus != state.deliveryStatus {
			s.observeStatusTransition(previousStatus, state.deliveryStatus)
			log.Printf(
				"event=delivery_status_transition source=%q intentId=%q submissionTarget=%q deliveryTrackingMode=%q correlationResult=%q from_delivery_status=%q to_delivery_status=%q",
				webhookIngressSourceProviderSignalWebhook,
				intentID,
				submissionTarget,
				deliveryTrackingMode,
				correlationResult,
				canonicalDeliveryStatus(previousStatus),
				canonicalDeliveryStatus(state.deliveryStatus),
			)
		}
		if previousFreshness != state.deliveryFreshness {
			s.observeFreshnessTransition(previousFreshness, state.deliveryFreshness)
			log.Printf(
				"event=delivery_freshness_transition source=%q intentId=%q submissionTarget=%q deliveryTrackingMode=%q correlationResult=%q from_delivery_freshness=%q to_delivery_freshness=%q",
				webhookIngressSourceProviderSignalWebhook,
				intentID,
				submissionTarget,
				deliveryTrackingMode,
				correlationResult,
				canonicalDeliveryFreshness(previousFreshness),
				canonicalDeliveryFreshness(state.deliveryFreshness),
			)
		}
	}

	return result, nil
}

func (s *sqlStore) evaluateDeliveryFreshnessStaleness(ctx context.Context) (affected int64, err error) {
	defer func() {
		if err == nil {
			return
		}
		reason := MapProcessingFailureReason(err)
		s.observeProcessingFailure(ProcessingStageFreshnessEvaluator, reason)
		log.Printf(
			"event=delivery_processing_failure stage=%q reason=%q source=%q intentId=%q submissionTarget=%q deliveryTrackingMode=%q correlationResult=%q",
			ProcessingStageFreshnessEvaluator,
			reason,
			webhookIngressSourceProviderSignalWebhook,
			"unknown",
			"unknown",
			"unknown",
			"unknown",
		)
	}()

	if ctx == nil {
		ctx = context.Background()
	}
	updateResult, err := s.db.ExecContext(
		ctx,
		`UPDATE dbo.intent_delivery_state
     SET delivery_freshness = @p1,
         updated_at = SYSUTCDATETIME()
     WHERE delivery_status IN (@p2, @p3)
       AND delivery_freshness = @p4
       AND stale_at IS NOT NULL
       AND stale_at <= SYSUTCDATETIME()`,
		string(DeliveryFreshnessStale),
		string(DeliveryStatusUnknown),
		string(DeliveryStatusInProgress),
		string(DeliveryFreshnessFresh),
	)
	if err != nil {
		return 0, err
	}
	affected, err = updateResult.RowsAffected()
	if err != nil {
		return 0, err
	}
	if affected > 0 {
		s.observeFreshnessTransitions(DeliveryFreshnessFresh, DeliveryFreshnessStale, uint64(affected))
		log.Printf(
			"event=delivery_freshness_transition source=%q intentId=%q submissionTarget=%q deliveryTrackingMode=%q correlationResult=%q from_delivery_freshness=%q to_delivery_freshness=%q transition_count=%d",
			webhookIngressSourceProviderSignalWebhook,
			"unknown",
			"unknown",
			"unknown",
			"unknown",
			string(DeliveryFreshnessFresh),
			string(DeliveryFreshnessStale),
			affected,
		)
	}
	return affected, nil
}

func (s *sqlStore) observeProviderSignal(source string, result DeliveryCorrelationResult) {
	if s == nil || s.metrics == nil {
		return
	}
	s.metrics.ObserveProviderSignal(source, result)
}

func (s *sqlStore) observeStatusTransition(from, to DeliveryStatus) {
	if s == nil || s.metrics == nil {
		return
	}
	s.metrics.ObserveStatusTransition(from, to)
}

func (s *sqlStore) observeFreshnessTransition(from, to DeliveryFreshness) {
	if s == nil || s.metrics == nil {
		return
	}
	s.metrics.ObserveFreshnessTransition(from, to)
}

func (s *sqlStore) observeFreshnessTransitions(from, to DeliveryFreshness, count uint64) {
	if s == nil || s.metrics == nil {
		return
	}
	s.metrics.ObserveFreshnessTransitions(from, to, count)
}

func (s *sqlStore) observeIgnoredSignal(reason string) {
	if s == nil || s.metrics == nil {
		return
	}
	s.metrics.ObserveIgnoredSignal(reason)
}

func (s *sqlStore) observeProcessingFailure(stage, reason string) {
	if s == nil || s.metrics == nil {
		return
	}
	s.metrics.ObserveProcessingFailure(stage, reason)
}

func normalizeCorrelatedDeliveryRecord(record CorrelatedDeliveryRecord) (normalizedCorrelatedDeliveryRecord, error) {
	sourceRecordID := strings.TrimSpace(record.SourceRecordID)
	if sourceRecordID == "" {
		return normalizedCorrelatedDeliveryRecord{}, errors.New("sourceRecordId is required")
	}

	receivedAt := record.ReceivedAt.UTC()
	if receivedAt.IsZero() {
		return normalizedCorrelatedDeliveryRecord{}, errors.New("receivedAt is required")
	}

	result := normalizedCorrelatedDeliveryRecord{
		sourceRecordID:    sourceRecordID,
		providerEventID:   strings.TrimSpace(record.ProviderEventID),
		intentID:          strings.TrimSpace(record.IntentID),
		correlationResult: record.CorrelationResult,
		receivedAt:        receivedAt,
		effectiveAt:       receivedAt,
	}

	if record.ProviderObservedAt != nil && !record.ProviderObservedAt.IsZero() {
		observed := record.ProviderObservedAt.UTC()
		result.providerObservedAt = sql.NullTime{Time: observed, Valid: true}
		result.effectiveAt = observed
	}

	switch record.CorrelationResult {
	case DeliveryCorrelationMatched:
		if result.intentID == "" {
			return normalizedCorrelatedDeliveryRecord{}, errors.New("intentId is required for matched signals")
		}
		switch record.SignalClass {
		case DeliverySignalInProgress, DeliverySignalTerminalSuccess, DeliverySignalTerminalFailure:
			result.signalClass = record.SignalClass
		default:
			return normalizedCorrelatedDeliveryRecord{}, fmt.Errorf("unknown signalClass %q", record.SignalClass)
		}
	case DeliveryCorrelationUnmatched:
		if result.intentID == "" {
			return normalizedCorrelatedDeliveryRecord{}, errors.New("intentId is required for unmatched signals")
		}
	case DeliveryCorrelationInvalid:
	default:
		return normalizedCorrelatedDeliveryRecord{}, fmt.Errorf("unknown correlationResult %q", record.CorrelationResult)
	}

	return result, nil
}

func loadDeliveryIntentSnapshotForUpdate(ctx context.Context, tx *sql.Tx, intentID string) (deliveryIntentSnapshot, bool, error) {
	row := tx.QueryRowContext(
		ctx,
		`SELECT submission_target,
      status,
      updated_at,
      created_at,
      delivery_tracking_mode,
      delivery_tracking_stale_after_seconds
    FROM dbo.submission_intents WITH (UPDLOCK, ROWLOCK)
    WHERE intent_id = @p1`,
		intentID,
	)

	var (
		submissionTarget  string
		statusValue       string
		updatedAt         time.Time
		createdAt         time.Time
		deliveryMode      string
		staleAfterSeconds sql.NullInt32
	)
	if err := row.Scan(&submissionTarget, &statusValue, &updatedAt, &createdAt, &deliveryMode, &staleAfterSeconds); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return deliveryIntentSnapshot{}, false, nil
		}
		return deliveryIntentSnapshot{}, false, err
	}

	mode := submission.DeliveryTrackingMode(strings.TrimSpace(deliveryMode))
	switch mode {
	case submission.DeliveryTrackingModeOn:
	default:
		mode = submission.DeliveryTrackingModeOff
	}

	snapshot := deliveryIntentSnapshot{
		submissionTarget:  strings.TrimSpace(submissionTarget),
		status:            SubmissionStatus(statusValue),
		createdAt:         normalizeDBTime(createdAt),
		deliveryMode:      mode,
		staleAfterSeconds: int(staleAfterSeconds.Int32),
	}
	if isTerminalSubmissionStatus(snapshot.status) {
		snapshot.completedAt = normalizeDBTime(updatedAt)
	}
	return snapshot, true, nil
}

func loadDeliveryStateForUpdate(ctx context.Context, tx *sql.Tx, intentID string) (deliveryStateProjection, bool, error) {
	row := tx.QueryRowContext(
		ctx,
		`SELECT delivery_status,
      delivery_freshness,
      last_non_terminal_effective_at,
      last_non_terminal_received_at,
      last_non_terminal_source_record_id,
      terminal_locked,
      terminal_status,
      terminal_effective_at,
      terminal_received_at,
      terminal_source_record_id,
      stale_after_seconds,
      stale_at
    FROM dbo.intent_delivery_state WITH (UPDLOCK, HOLDLOCK, ROWLOCK)
    WHERE intent_id = @p1`,
		intentID,
	)

	var (
		state             deliveryStateProjection
		deliveryStatus    string
		deliveryFreshness string
	)
	if err := row.Scan(
		&deliveryStatus,
		&deliveryFreshness,
		&state.lastNonTerminalEffectiveAt,
		&state.lastNonTerminalReceivedAt,
		&state.lastNonTerminalSourceRecordID,
		&state.terminalLocked,
		&state.terminalStatus,
		&state.terminalEffectiveAt,
		&state.terminalReceivedAt,
		&state.terminalSourceRecordID,
		&state.staleAfterSeconds,
		&state.staleAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return deliveryStateProjection{}, false, nil
		}
		return deliveryStateProjection{}, false, err
	}

	state.deliveryStatus = DeliveryStatus(strings.TrimSpace(deliveryStatus))
	state.deliveryFreshness = DeliveryFreshness(strings.TrimSpace(deliveryFreshness))
	return state, true, nil
}

func insertDeliveryHistoryRow(ctx context.Context, tx *sql.Tx, record normalizedCorrelatedDeliveryRecord) (bool, error) {
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO dbo.intent_delivery_history (
      intent_id,
      source_record_id,
      provider_event_id,
      signal_class,
      provider_observed_at,
      effective_at,
      received_at,
      late_observation,
      terminal_conflict,
      created_at
    ) VALUES (
      @p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10
    )`,
		record.intentID,
		record.sourceRecordID,
		nullString(record.providerEventID),
		string(record.signalClass),
		record.providerObservedAt,
		record.effectiveAt,
		record.receivedAt,
		false,
		false,
		record.receivedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

func insertInitialDeliveryState(ctx context.Context, tx *sql.Tx, intentID string, intentCreatedAt time.Time, staleAfterSeconds int, updatedAt time.Time) error {
	staleAt := normalizeDBTime(intentCreatedAt).Add(time.Duration(staleAfterSeconds) * time.Second)
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO dbo.intent_delivery_state (
      intent_id,
      delivery_status,
      delivery_freshness,
      last_non_terminal_effective_at,
      last_non_terminal_received_at,
      last_non_terminal_source_record_id,
      terminal_locked,
      terminal_status,
      terminal_effective_at,
      terminal_received_at,
      terminal_source_record_id,
      stale_after_seconds,
      stale_at,
      created_at,
      updated_at
    ) VALUES (
      @p1, @p2, @p3, NULL, NULL, NULL, @p4, NULL, NULL, NULL, NULL, @p5, @p6, @p7, @p7
    )`,
		intentID,
		string(DeliveryStatusUnknown),
		string(DeliveryFreshnessFresh),
		false,
		staleAfterSeconds,
		staleAt,
		updatedAt,
	)
	if err == nil {
		return nil
	}
	if isUniqueViolation(err) {
		return nil
	}
	return err
}

func updateDeliveryState(ctx context.Context, tx *sql.Tx, intentID string, state deliveryStateProjection, updatedAt time.Time) error {
	result, err := tx.ExecContext(
		ctx,
		`UPDATE dbo.intent_delivery_state
     SET delivery_status = @p1,
         delivery_freshness = @p2,
         last_non_terminal_effective_at = @p3,
         last_non_terminal_received_at = @p4,
         last_non_terminal_source_record_id = @p5,
         terminal_locked = @p6,
         terminal_status = @p7,
         terminal_effective_at = @p8,
         terminal_received_at = @p9,
         terminal_source_record_id = @p10,
         stale_after_seconds = @p11,
         stale_at = @p12,
         updated_at = @p13
     WHERE intent_id = @p14`,
		string(state.deliveryStatus),
		string(state.deliveryFreshness),
		state.lastNonTerminalEffectiveAt,
		state.lastNonTerminalReceivedAt,
		state.lastNonTerminalSourceRecordID,
		state.terminalLocked,
		state.terminalStatus,
		state.terminalEffectiveAt,
		state.terminalReceivedAt,
		state.terminalSourceRecordID,
		state.staleAfterSeconds,
		state.staleAt,
		updatedAt,
		intentID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return errors.New("delivery state update affected unexpected row count")
	}
	return nil
}

func updateDeliveryHistoryAnnotations(ctx context.Context, tx *sql.Tx, intentID, sourceRecordID string, lateObservation, terminalConflict bool) error {
	result, err := tx.ExecContext(
		ctx,
		`UPDATE dbo.intent_delivery_history
     SET late_observation = @p1,
         terminal_conflict = @p2
     WHERE intent_id = @p3
       AND source_record_id = @p4`,
		lateObservation,
		terminalConflict,
		intentID,
		sourceRecordID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return errors.New("delivery history annotation update affected unexpected row count")
	}
	return nil
}

func isNewerNonTerminalSignal(state deliveryStateProjection, incoming deliveryOrderingKey) bool {
	if !state.lastNonTerminalEffectiveAt.Valid || !state.lastNonTerminalReceivedAt.Valid || !state.lastNonTerminalSourceRecordID.Valid {
		return true
	}
	existing := deliveryOrderingKey{
		effectiveAt:    normalizeDBTime(state.lastNonTerminalEffectiveAt.Time),
		receivedAt:     normalizeDBTime(state.lastNonTerminalReceivedAt.Time),
		sourceRecordID: state.lastNonTerminalSourceRecordID.String,
	}
	return compareDeliveryOrderingKey(incoming, existing) > 0
}

func compareDeliveryOrderingKey(left, right deliveryOrderingKey) int {
	switch {
	case left.effectiveAt.Before(right.effectiveAt):
		return -1
	case left.effectiveAt.After(right.effectiveAt):
		return 1
	}
	switch {
	case left.receivedAt.Before(right.receivedAt):
		return -1
	case left.receivedAt.After(right.receivedAt):
		return 1
	}
	switch {
	case left.sourceRecordID < right.sourceRecordID:
		return -1
	case left.sourceRecordID > right.sourceRecordID:
		return 1
	default:
		return 0
	}
}

func isLateTerminalObservation(submissionStatus SubmissionStatus, completedAt, receivedAt time.Time) bool {
	if !isTerminalSubmissionStatus(submissionStatus) {
		return false
	}
	if completedAt.IsZero() {
		return false
	}
	return !normalizeDBTime(completedAt).After(receivedAt)
}

func isTerminalSubmissionStatus(status SubmissionStatus) bool {
	return status == SubmissionStatusAccepted || status == SubmissionStatusRejected || status == SubmissionStatusExhausted
}

func terminalStatusForSignal(signalClass DeliverySignalClass) DeliveryStatus {
	switch signalClass {
	case DeliverySignalTerminalSuccess:
		return DeliveryStatusDelivered
	case DeliverySignalTerminalFailure:
		return DeliveryStatusFailed
	default:
		return DeliveryStatusUnknown
	}
}

func normalizeDBTime(value time.Time) time.Time {
	return time.Date(
		value.Year(),
		value.Month(),
		value.Day(),
		value.Hour(),
		value.Minute(),
		value.Second(),
		value.Nanosecond(),
		time.UTC,
	)
}

func nullString(value string) sql.NullString {
	if strings.TrimSpace(value) == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func isUniqueViolation(err error) bool {
	var mssqlErr mssql.Error
	if !errors.As(err, &mssqlErr) {
		return false
	}
	return mssqlErr.Number == 2627 || mssqlErr.Number == 2601
}
