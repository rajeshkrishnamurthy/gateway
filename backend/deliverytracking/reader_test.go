package deliverytracking

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"gateway/submission"
)

func TestReaderCurrentDeliveryUnknownIntent(t *testing.T) {
	db := newTestDB(t)
	reader, err := NewReader(db)
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}

	_, found, err := reader.GetCurrentDelivery(context.Background(), "missing")
	if err != nil {
		t.Fatalf("get current delivery: %v", err)
	}
	if found {
		t.Fatalf("expected missing intent to be not found")
	}
}

func TestReaderDeliveryHistoryUnknownIntent(t *testing.T) {
	db := newTestDB(t)
	reader, err := NewReader(db)
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}

	_, found, err := reader.GetDeliveryHistory(context.Background(), "missing")
	if err != nil {
		t.Fatalf("get delivery history: %v", err)
	}
	if found {
		t.Fatalf("expected missing intent history to be not found")
	}
}

func TestReaderCurrentAndHistoryModeOff(t *testing.T) {
	store, _ := newDeliveryStore(t)
	insertDeliveryIntent(t, store, "intent-off", submission.DeliveryTrackingModeOff, 0, time.Now().UTC().Add(-time.Minute))

	reader, err := NewReader(store.db)
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}

	current, found, err := reader.GetCurrentDelivery(context.Background(), "intent-off")
	if err != nil {
		t.Fatalf("get current delivery: %v", err)
	}
	if !found {
		t.Fatalf("expected intent-off to exist")
	}
	if current.DeliveryTrackingMode != submission.DeliveryTrackingModeOff {
		t.Fatalf("expected mode off, got %q", current.DeliveryTrackingMode)
	}
	if current.DeliveryStatus != nil {
		t.Fatalf("expected no delivery status for mode off")
	}
	if current.DeliveryFreshness != nil {
		t.Fatalf("expected no delivery freshness for mode off")
	}

	history, found, err := reader.GetDeliveryHistory(context.Background(), "intent-off")
	if err != nil {
		t.Fatalf("get delivery history: %v", err)
	}
	if !found {
		t.Fatalf("expected intent-off to exist")
	}
	if history.DeliveryTrackingMode != submission.DeliveryTrackingModeOff {
		t.Fatalf("expected mode off, got %q", history.DeliveryTrackingMode)
	}
	if len(history.Entries) != 0 {
		t.Fatalf("expected no entries for mode off, got %d", len(history.Entries))
	}
}

func TestReaderCurrentFallbackFreshness(t *testing.T) {
	store, _ := newDeliveryStore(t)
	insertDeliveryIntent(t, store, "intent-fresh", submission.DeliveryTrackingModeOn, 3600, time.Now().UTC().Add(-time.Minute))
	insertDeliveryIntent(t, store, "intent-stale", submission.DeliveryTrackingModeOn, 60, time.Now().UTC().Add(-2*time.Hour))

	reader, err := NewReader(store.db)
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}

	fresh, found, err := reader.GetCurrentDelivery(context.Background(), "intent-fresh")
	if err != nil {
		t.Fatalf("get current delivery fresh: %v", err)
	}
	if !found {
		t.Fatalf("expected intent-fresh to exist")
	}
	if fresh.DeliveryStatus == nil || *fresh.DeliveryStatus != DeliveryStatusUnknown {
		t.Fatalf("expected fallback status unknown, got %v", fresh.DeliveryStatus)
	}
	if fresh.DeliveryFreshness == nil || *fresh.DeliveryFreshness != DeliveryFreshnessFresh {
		t.Fatalf("expected fallback freshness fresh, got %v", fresh.DeliveryFreshness)
	}

	stale, found, err := reader.GetCurrentDelivery(context.Background(), "intent-stale")
	if err != nil {
		t.Fatalf("get current delivery stale: %v", err)
	}
	if !found {
		t.Fatalf("expected intent-stale to exist")
	}
	if stale.DeliveryStatus == nil || *stale.DeliveryStatus != DeliveryStatusUnknown {
		t.Fatalf("expected fallback status unknown, got %v", stale.DeliveryStatus)
	}
	if stale.DeliveryFreshness == nil || *stale.DeliveryFreshness != DeliveryFreshnessStale {
		t.Fatalf("expected fallback freshness stale, got %v", stale.DeliveryFreshness)
	}
}

func TestReaderCurrentUsesPersistedState(t *testing.T) {
	store, _ := newDeliveryStore(t)
	insertDeliveryIntent(t, store, "intent-state", submission.DeliveryTrackingModeOn, 60, time.Now().UTC().Add(-time.Hour))
	insertDeliveryStateRow(t, store.db, "intent-state", DeliveryStatusFailed, DeliveryFreshnessNotApplicable, 60)

	reader, err := NewReader(store.db)
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}

	current, found, err := reader.GetCurrentDelivery(context.Background(), "intent-state")
	if err != nil {
		t.Fatalf("get current delivery: %v", err)
	}
	if !found {
		t.Fatalf("expected intent-state to exist")
	}
	if current.DeliveryStatus == nil || *current.DeliveryStatus != DeliveryStatusFailed {
		t.Fatalf("expected failed status, got %v", current.DeliveryStatus)
	}
	if current.DeliveryFreshness == nil || *current.DeliveryFreshness != DeliveryFreshnessNotApplicable {
		t.Fatalf("expected not_applicable freshness, got %v", current.DeliveryFreshness)
	}
}

func TestReaderHistoryOrderedByAppendSequence(t *testing.T) {
	store, _ := newDeliveryStore(t)
	insertDeliveryIntent(t, store, "intent-history", submission.DeliveryTrackingModeOn, 120, time.Now().UTC().Add(-time.Hour))

	base := time.Now().UTC().Add(-5 * time.Minute)
	insertDeliveryHistoryEntry(t, store.db, "intent-history", "src-1", DeliverySignalTerminalFailure, base.Add(2*time.Minute), false)
	insertDeliveryHistoryEntry(t, store.db, "intent-history", "src-2", DeliverySignalInProgress, base, false)
	insertDeliveryHistoryEntry(t, store.db, "intent-history", "src-3", DeliverySignalTerminalSuccess, base.Add(time.Minute), true)

	reader, err := NewReader(store.db)
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}

	history, found, err := reader.GetDeliveryHistory(context.Background(), "intent-history")
	if err != nil {
		t.Fatalf("get delivery history: %v", err)
	}
	if !found {
		t.Fatalf("expected intent-history to exist")
	}
	if len(history.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(history.Entries))
	}
	if history.Entries[0].ProviderDeliverySignal != DeliverySignalTerminalFailure {
		t.Fatalf("expected first signal terminal_failure, got %q", history.Entries[0].ProviderDeliverySignal)
	}
	if history.Entries[1].ProviderDeliverySignal != DeliverySignalInProgress {
		t.Fatalf("expected second signal in_progress, got %q", history.Entries[1].ProviderDeliverySignal)
	}
	if history.Entries[2].ProviderDeliverySignal != DeliverySignalTerminalSuccess {
		t.Fatalf("expected third signal terminal_success, got %q", history.Entries[2].ProviderDeliverySignal)
	}
	if history.Entries[0].DeliveryStatus != DeliveryStatusFailed {
		t.Fatalf("expected first status failed, got %q", history.Entries[0].DeliveryStatus)
	}
	if history.Entries[1].DeliveryStatus != DeliveryStatusInProgress {
		t.Fatalf("expected second status in_progress, got %q", history.Entries[1].DeliveryStatus)
	}
	if history.Entries[2].DeliveryStatus != DeliveryStatusDelivered {
		t.Fatalf("expected third status delivered, got %q", history.Entries[2].DeliveryStatus)
	}
	if !history.Entries[2].LateObservation {
		t.Fatalf("expected third entry lateObservation true")
	}
}

func insertDeliveryStateRow(t *testing.T, db *sql.DB, intentID string, status DeliveryStatus, freshness DeliveryFreshness, staleAfter int) {
	t.Helper()
	now := time.Now().UTC()
	staleAt := now.Add(time.Duration(staleAfter) * time.Second)
	_, err := db.ExecContext(
		context.Background(),
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
      @p1, @p2, @p3, NULL, NULL, NULL, 0, NULL, NULL, NULL, NULL, @p4, @p5, @p6, @p6
    )`,
		intentID,
		string(status),
		string(freshness),
		staleAfter,
		normalizeDBTime(staleAt),
		normalizeDBTime(now),
	)
	if err != nil {
		t.Fatalf("insert delivery state: %v", err)
	}
}

func insertDeliveryHistoryEntry(t *testing.T, db *sql.DB, intentID, sourceRecordID string, signal DeliverySignalClass, recordedAt time.Time, lateObservation bool) {
	t.Helper()
	_, err := db.ExecContext(
		context.Background(),
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
      @p1, @p2, NULL, @p3, NULL, @p4, @p4, @p5, 0, @p4
    )`,
		intentID,
		sourceRecordID,
		string(signal),
		normalizeDBTime(recordedAt),
		lateObservation,
	)
	if err != nil {
		t.Fatalf("insert delivery history: %v", err)
	}
}
