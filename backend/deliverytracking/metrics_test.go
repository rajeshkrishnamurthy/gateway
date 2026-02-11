package deliverytracking

import (
	"bytes"
	"context"
	"database/sql"
	"log"
	"strings"
	"testing"
	"time"

	"gateway/submission"
)

func TestDeliveryMetricsWritePrometheusIncludesRequiredMetricsAndStatusMaterialization(t *testing.T) {
	store, db := newDeliveryStore(t)
	metrics := NewMetrics(db)

	now := time.Date(2026, 2, 11, 7, 0, 0, 0, time.UTC)
	insertDeliveryIntent(t, store, "intent-obsv-unknown", submission.DeliveryTrackingModeOn, 300, now.Add(-time.Hour))
	insertDeliveryIntent(t, store, "intent-obsv-in-progress", submission.DeliveryTrackingModeOn, 300, now.Add(-time.Hour))
	insertDeliveryIntent(t, store, "intent-obsv-delivered", submission.DeliveryTrackingModeOn, 300, now.Add(-time.Hour))
	insertDeliveryIntent(t, store, "intent-obsv-failed", submission.DeliveryTrackingModeOn, 300, now.Add(-time.Hour))
	insertDeliveryIntent(t, store, "intent-obsv-off", submission.DeliveryTrackingModeOff, 0, now.Add(-time.Hour))

	insertDeliveryStateRow(t, db, "intent-obsv-in-progress", DeliveryStatusInProgress, DeliveryFreshnessFresh, 300)
	insertDeliveryStateRow(t, db, "intent-obsv-delivered", DeliveryStatusDelivered, DeliveryFreshnessNotApplicable, 300)
	insertDeliveryStateRow(t, db, "intent-obsv-failed", DeliveryStatusFailed, DeliveryFreshnessNotApplicable, 300)
	insertDeliveryStateRow(t, db, "intent-obsv-off", DeliveryStatusInProgress, DeliveryFreshnessFresh, 300)

	metrics.ObserveProviderSignal(webhookIngressSourceProviderSignalWebhook, DeliveryCorrelationMatched)
	metrics.ObserveIgnoredSignal(deliveryNoOpReasonModeOff)
	metrics.ObserveStatusTransition(DeliveryStatusUnknown, DeliveryStatusInProgress)
	metrics.ObserveFreshnessTransition(DeliveryFreshnessStale, DeliveryFreshnessFresh)
	metrics.ObserveProcessingFailure(ProcessingStageWebhookIngestion, "not_allowlisted")
	metrics.ObserveReadAPIRequest(DeliveryReadEndpointCurrent, httpStatusTeapot)

	var buf bytes.Buffer
	metrics.WritePrometheus(&buf)
	output := buf.String()

	requiredMetricNames := []string{
		"delivery_provider_signals_total",
		"delivery_status_current_count",
		"delivery_status_transitions_total",
		"delivery_freshness_transitions_total",
		"delivery_ignored_signals_total",
		"delivery_processing_failures_total",
		"delivery_read_api_requests_total",
	}
	for _, metricName := range requiredMetricNames {
		if !strings.Contains(output, metricName) {
			t.Fatalf("expected metrics output to include %q, output=%q", metricName, output)
		}
	}

	assertMetricsLine(t, output, `delivery_status_current_count{delivery_status="unknown"} 1`)
	assertMetricsLine(t, output, `delivery_status_current_count{delivery_status="in_progress"} 1`)
	assertMetricsLine(t, output, `delivery_status_current_count{delivery_status="delivered"} 1`)
	assertMetricsLine(t, output, `delivery_status_current_count{delivery_status="failed"} 1`)
	assertMetricsLine(t, output, `delivery_provider_signals_total{source="provider signal webhook",correlation_result="matched"} 1`)
	assertMetricsLine(t, output, `delivery_ignored_signals_total{reason="mode_off"} 1`)
	assertMetricsLine(t, output, `delivery_status_transitions_total{from_delivery_status="unknown",to_delivery_status="in_progress"} 1`)
	assertMetricsLine(t, output, `delivery_freshness_transitions_total{from_delivery_freshness="stale",to_delivery_freshness="fresh"} 1`)
	assertMetricsLine(t, output, `delivery_processing_failures_total{stage="webhook_ingestion",reason="unknown_reason"} 1`)
	assertMetricsLine(t, output, `delivery_read_api_requests_total{endpoint="/v1/intents/{intentId}/delivery",code="other"} 1`)
}

func TestProcessorWithMetricsEmitsNoOpAndTransitionCounters(t *testing.T) {
	store, metrics, _ := newDeliveryStoreWithMetrics(t)
	base := time.Date(2026, 2, 11, 7, 30, 0, 0, time.UTC)

	insertDeliveryIntent(t, store, "intent-obsv-mode-off", submission.DeliveryTrackingModeOff, 0, base.Add(-time.Minute))
	insertDeliveryIntent(t, store, "intent-obsv-on", submission.DeliveryTrackingModeOn, 120, base.Add(-time.Minute))

	_, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-obsv-unmatched",
		IntentID:          "intent-obsv-on",
		ReceivedAt:        base,
		CorrelationResult: DeliveryCorrelationUnmatched,
	})
	if err != nil {
		t.Fatalf("apply unmatched: %v", err)
	}

	_, err = store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-obsv-invalid",
		ReceivedAt:        base.Add(time.Second),
		CorrelationResult: DeliveryCorrelationInvalid,
	})
	if err != nil {
		t.Fatalf("apply invalid: %v", err)
	}

	_, err = store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-obsv-mode-off",
		IntentID:          "intent-obsv-mode-off",
		SignalClass:       DeliverySignalInProgress,
		ReceivedAt:        base.Add(2 * time.Second),
		CorrelationResult: DeliveryCorrelationMatched,
	})
	if err != nil {
		t.Fatalf("apply mode off: %v", err)
	}

	_, err = store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-obsv-on-1",
		IntentID:          "intent-obsv-on",
		SignalClass:       DeliverySignalInProgress,
		ReceivedAt:        base.Add(3 * time.Second),
		CorrelationResult: DeliveryCorrelationMatched,
	})
	if err != nil {
		t.Fatalf("apply in_progress: %v", err)
	}

	_, err = store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-obsv-on-1",
		IntentID:          "intent-obsv-on",
		SignalClass:       DeliverySignalTerminalSuccess,
		ReceivedAt:        base.Add(4 * time.Second),
		CorrelationResult: DeliveryCorrelationMatched,
	})
	if err != nil {
		t.Fatalf("apply duplicate source record: %v", err)
	}

	_, err = store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-obsv-on-2",
		IntentID:          "intent-obsv-on",
		SignalClass:       DeliverySignalTerminalSuccess,
		ReceivedAt:        base.Add(5 * time.Second),
		CorrelationResult: DeliveryCorrelationMatched,
	})
	if err != nil {
		t.Fatalf("apply terminal success: %v", err)
	}

	var buf bytes.Buffer
	metrics.WritePrometheus(&buf)
	output := buf.String()

	assertMetricsLine(t, output, `delivery_provider_signals_total{source="provider signal webhook",correlation_result="unmatched"} 1`)
	assertMetricsLine(t, output, `delivery_provider_signals_total{source="provider signal webhook",correlation_result="invalid"} 1`)
	assertMetricsLine(t, output, `delivery_ignored_signals_total{reason="correlation_unmatched"} 1`)
	assertMetricsLine(t, output, `delivery_ignored_signals_total{reason="correlation_invalid"} 1`)
	assertMetricsLine(t, output, `delivery_ignored_signals_total{reason="mode_off"} 1`)
	assertMetricsLine(t, output, `delivery_ignored_signals_total{reason="duplicate_source_record"} 1`)
	assertMetricsLine(t, output, `delivery_status_transitions_total{from_delivery_status="unknown",to_delivery_status="in_progress"} 1`)
	assertMetricsLine(t, output, `delivery_status_transitions_total{from_delivery_status="in_progress",to_delivery_status="delivered"} 1`)
	assertMetricsLine(t, output, `delivery_freshness_transitions_total{from_delivery_freshness="fresh",to_delivery_freshness="not_applicable"} 1`)
}

func TestProcessorWithMetricsEmitsFreshnessEvaluatorAndFailureMetrics(t *testing.T) {
	store, metrics, db := newDeliveryStoreWithMetrics(t)
	base := time.Now().UTC().Add(-10 * time.Minute)

	insertDeliveryIntent(t, store, "intent-obsv-eval", submission.DeliveryTrackingModeOn, 60, base.Add(-time.Hour))
	observedAt := base.Add(-5 * time.Minute)
	if _, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:     "src-obsv-eval-1",
		IntentID:           "intent-obsv-eval",
		SignalClass:        DeliverySignalInProgress,
		ProviderObservedAt: &observedAt,
		ReceivedAt:         base,
		CorrelationResult:  DeliveryCorrelationMatched,
	}); err != nil {
		t.Fatalf("apply in_progress for evaluator: %v", err)
	}

	affected, err := store.evaluateDeliveryFreshnessStaleness(context.Background())
	if err != nil {
		t.Fatalf("evaluate freshness staleness: %v", err)
	}
	if affected != 1 {
		t.Fatalf("expected one freshness transition from evaluator, got %d", affected)
	}

	if _, err := db.ExecContext(context.Background(), `DROP TABLE dbo.intent_delivery_state`); err != nil {
		t.Fatalf("drop delivery state table: %v", err)
	}
	if _, err := store.evaluateDeliveryFreshnessStaleness(context.Background()); err == nil {
		t.Fatal("expected evaluator failure after dropping delivery state table")
	}

	var buf bytes.Buffer
	metrics.WritePrometheus(&buf)
	output := buf.String()

	assertMetricsLine(t, output, `delivery_freshness_transitions_total{from_delivery_freshness="fresh",to_delivery_freshness="stale"} 1`)
	assertMetricsLine(t, output, `delivery_processing_failures_total{stage="freshness_evaluator",reason="storage_unavailable"} 1`)
}

func TestProcessorObservabilityLogsDecisionEvents(t *testing.T) {
	store, _, _ := newDeliveryStoreWithMetrics(t)
	base := time.Date(2026, 2, 11, 8, 30, 0, 0, time.UTC)
	insertDeliveryIntent(t, store, "intent-obsv-log", submission.DeliveryTrackingModeOn, 120, base.Add(-time.Minute))

	var buf bytes.Buffer
	restore := captureLogs(&buf)
	defer restore()

	_, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-obsv-log-unmatched",
		IntentID:          "intent-obsv-log",
		ReceivedAt:        base,
		CorrelationResult: DeliveryCorrelationUnmatched,
	})
	if err != nil {
		t.Fatalf("apply unmatched for log: %v", err)
	}
	_, err = store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-obsv-log-matched",
		IntentID:          "intent-obsv-log",
		SignalClass:       DeliverySignalInProgress,
		ReceivedAt:        base.Add(time.Second),
		CorrelationResult: DeliveryCorrelationMatched,
	})
	if err != nil {
		t.Fatalf("apply matched for log: %v", err)
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, `event=delivery_correlation_result`) {
		t.Fatalf("expected correlation log event, got %q", logOutput)
	}
	if !strings.Contains(logOutput, `event=delivery_status_transition`) {
		t.Fatalf("expected status transition log event, got %q", logOutput)
	}
	if strings.Contains(logOutput, "providerDeliverySignal") {
		t.Fatalf("expected logs to avoid payload/body fields, got %q", logOutput)
	}
}

func assertMetricsLine(t *testing.T, output, expected string) {
	t.Helper()
	if !strings.Contains(output, expected) {
		t.Fatalf("expected metrics output to contain %q, output=%q", expected, output)
	}
}

func newDeliveryStoreWithMetrics(t *testing.T) (*sqlStore, *Metrics, *sql.DB) {
	t.Helper()
	db := newTestDB(t)
	metrics := NewMetrics(db)
	store, err := newSQLStoreWithMetrics(db, metrics)
	if err != nil {
		t.Fatalf("new sql store with metrics: %v", err)
	}
	return store, metrics, db
}

func captureLogs(buf *bytes.Buffer) func() {
	currentWriter := log.Writer()
	currentFlags := log.Flags()
	currentPrefix := log.Prefix()
	log.SetOutput(buf)
	log.SetFlags(0)
	log.SetPrefix("")
	return func() {
		log.SetOutput(currentWriter)
		log.SetFlags(currentFlags)
		log.SetPrefix(currentPrefix)
	}
}

const httpStatusTeapot = 418
