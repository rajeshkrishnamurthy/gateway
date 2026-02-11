package deliverytracking

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"crypto/sha256"

	"gateway/submission"
)

func TestDeliveryApplyCorrelationGating(t *testing.T) {
	store, db := newDeliveryStore(t)
	now := time.Date(2026, 2, 11, 1, 0, 0, 0, time.UTC)
	insertDeliveryIntent(t, store, "intent-1", submission.DeliveryTrackingModeOn, 120, now.Add(-5*time.Minute))

	unmatched, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-unmatched",
		IntentID:          "intent-1",
		ReceivedAt:        now,
		CorrelationResult: DeliveryCorrelationUnmatched,
	})
	if err != nil {
		t.Fatalf("apply unmatched: %v", err)
	}
	if unmatched.NoOpReason != deliveryNoOpReasonCorrelationUnmatched {
		t.Fatalf("expected unmatched no-op, got %+v", unmatched)
	}
	assertNoDeliveryMutation(t, db, "intent-1")

	invalid, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-invalid",
		ReceivedAt:        now.Add(time.Second),
		CorrelationResult: DeliveryCorrelationInvalid,
	})
	if err != nil {
		t.Fatalf("apply invalid: %v", err)
	}
	if invalid.NoOpReason != deliveryNoOpReasonCorrelationInvalid {
		t.Fatalf("expected invalid no-op, got %+v", invalid)
	}
	assertNoDeliveryMutation(t, db, "intent-1")

	matched, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-matched",
		IntentID:          "intent-1",
		SignalClass:       DeliverySignalInProgress,
		ReceivedAt:        now.Add(2 * time.Second),
		CorrelationResult: DeliveryCorrelationMatched,
	})
	if err != nil {
		t.Fatalf("apply matched: %v", err)
	}
	if !matched.HistoryInserted || !matched.StateMutated {
		t.Fatalf("expected matched mutation, got %+v", matched)
	}
	state, ok := loadDeliveryState(t, db, "intent-1")
	if !ok {
		t.Fatal("expected delivery state row")
	}
	if state.deliveryStatus != DeliveryStatusInProgress {
		t.Fatalf("expected in_progress status, got %q", state.deliveryStatus)
	}
	if state.deliveryFreshness != DeliveryFreshnessFresh {
		t.Fatalf("expected fresh freshness, got %q", state.deliveryFreshness)
	}
	if historyCount(t, db, "intent-1") != 1 {
		t.Fatalf("expected 1 history row")
	}
}

func TestCorrelateByIntentID(t *testing.T) {
	store, _ := newDeliveryStore(t)
	now := time.Date(2026, 2, 11, 1, 1, 0, 0, time.UTC)
	insertDeliveryIntent(t, store, "intent-correlate", submission.DeliveryTrackingModeOn, 120, now.Add(-time.Minute))

	matched, err := store.correlateByIntentID(context.Background(), "intent-correlate")
	if err != nil {
		t.Fatalf("correlate matched: %v", err)
	}
	if matched != DeliveryCorrelationMatched {
		t.Fatalf("expected matched, got %q", matched)
	}

	unmatched, err := store.correlateByIntentID(context.Background(), "intent-missing")
	if err != nil {
		t.Fatalf("correlate unmatched: %v", err)
	}
	if unmatched != DeliveryCorrelationUnmatched {
		t.Fatalf("expected unmatched, got %q", unmatched)
	}

	invalid, err := store.correlateByIntentID(context.Background(), "   ")
	if err != nil {
		t.Fatalf("correlate invalid: %v", err)
	}
	if invalid != DeliveryCorrelationInvalid {
		t.Fatalf("expected invalid, got %q", invalid)
	}
}

func TestDeliveryApplyModeOffNoMutation(t *testing.T) {
	store, db := newDeliveryStore(t)
	now := time.Date(2026, 2, 11, 1, 5, 0, 0, time.UTC)
	insertDeliveryIntent(t, store, "intent-mode-off", submission.DeliveryTrackingModeOff, 0, now.Add(-time.Minute))

	result, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-1",
		IntentID:          "intent-mode-off",
		SignalClass:       DeliverySignalInProgress,
		ReceivedAt:        now,
		CorrelationResult: DeliveryCorrelationMatched,
	})
	if err != nil {
		t.Fatalf("apply mode off: %v", err)
	}
	if result.NoOpReason != deliveryNoOpReasonModeOff {
		t.Fatalf("expected mode_off no-op, got %+v", result)
	}
	assertNoDeliveryMutation(t, db, "intent-mode-off")
}

func TestDeliveryApplyOrderingTieBreak(t *testing.T) {
	store, db := newDeliveryStore(t)
	base := time.Date(2026, 2, 11, 2, 0, 0, 0, time.UTC)
	insertDeliveryIntent(t, store, "intent-ordering", submission.DeliveryTrackingModeOn, 60, base.Add(-10*time.Minute))

	observed := base.Add(-2 * time.Minute)
	receivedFive := base
	receivedFour := base.Add(-time.Second)

	_, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:     "b-record",
		IntentID:           "intent-ordering",
		SignalClass:        DeliverySignalInProgress,
		ProviderObservedAt: &observed,
		ReceivedAt:         receivedFive,
		CorrelationResult:  DeliveryCorrelationMatched,
	})
	if err != nil {
		t.Fatalf("apply b-record: %v", err)
	}

	older, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:     "a-record",
		IntentID:           "intent-ordering",
		SignalClass:        DeliverySignalInProgress,
		ProviderObservedAt: &observed,
		ReceivedAt:         receivedFour,
		CorrelationResult:  DeliveryCorrelationMatched,
	})
	if err != nil {
		t.Fatalf("apply older record: %v", err)
	}
	if older.StateMutated {
		t.Fatalf("expected older record to be history-only, got %+v", older)
	}

	_, err = store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:     "c-record",
		IntentID:           "intent-ordering",
		SignalClass:        DeliverySignalInProgress,
		ProviderObservedAt: &observed,
		ReceivedAt:         receivedFive,
		CorrelationResult:  DeliveryCorrelationMatched,
	})
	if err != nil {
		t.Fatalf("apply c-record: %v", err)
	}

	state, ok := loadDeliveryState(t, db, "intent-ordering")
	if !ok {
		t.Fatal("expected delivery state row")
	}
	if !state.lastNonTerminalSourceRecordID.Valid || state.lastNonTerminalSourceRecordID.String != "c-record" {
		t.Fatalf("expected tie-break winner c-record, got %+v", state.lastNonTerminalSourceRecordID)
	}
	if !state.lastNonTerminalReceivedAt.Valid || !normalizeDBTime(state.lastNonTerminalReceivedAt.Time).Equal(receivedFive) {
		t.Fatalf("expected receivedAt %s, got %+v", receivedFive, state.lastNonTerminalReceivedAt)
	}
}

func TestDeliveryApplyIdempotencyDuplicateSourceRecordID(t *testing.T) {
	store, db := newDeliveryStore(t)
	now := time.Date(2026, 2, 11, 2, 15, 0, 0, time.UTC)
	insertDeliveryIntent(t, store, "intent-idempotent", submission.DeliveryTrackingModeOn, 90, now.Add(-time.Minute))

	_, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-dup",
		IntentID:          "intent-idempotent",
		SignalClass:       DeliverySignalInProgress,
		ReceivedAt:        now,
		CorrelationResult: DeliveryCorrelationMatched,
	})
	if err != nil {
		t.Fatalf("apply first: %v", err)
	}
	second, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-dup",
		IntentID:          "intent-idempotent",
		SignalClass:       DeliverySignalTerminalSuccess,
		ReceivedAt:        now.Add(time.Second),
		CorrelationResult: DeliveryCorrelationMatched,
	})
	if err != nil {
		t.Fatalf("apply duplicate: %v", err)
	}
	if second.NoOpReason != deliveryNoOpReasonDuplicateSource {
		t.Fatalf("expected duplicate no-op, got %+v", second)
	}
	if historyCount(t, db, "intent-idempotent") != 1 {
		t.Fatalf("expected 1 history row after duplicate")
	}
	state, ok := loadDeliveryState(t, db, "intent-idempotent")
	if !ok {
		t.Fatal("expected delivery state row")
	}
	if state.deliveryStatus != DeliveryStatusInProgress {
		t.Fatalf("expected in_progress status, got %q", state.deliveryStatus)
	}
}

func TestDeliveryApplyStaleToFreshOnlyOnNewerValidInProgress(t *testing.T) {
	store, db := newDeliveryStore(t)
	base := time.Date(2026, 2, 11, 3, 0, 0, 0, time.UTC)
	insertDeliveryIntent(t, store, "intent-freshness", submission.DeliveryTrackingModeOn, 60, base.Add(-10*time.Minute))

	firstObserved := base.Add(-time.Minute)
	_, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:     "src-1",
		IntentID:           "intent-freshness",
		SignalClass:        DeliverySignalInProgress,
		ProviderObservedAt: &firstObserved,
		ReceivedAt:         base,
		CorrelationResult:  DeliveryCorrelationMatched,
	})
	if err != nil {
		t.Fatalf("apply first in_progress: %v", err)
	}
	forceFreshness(t, db, "intent-freshness", DeliveryFreshnessStale)

	olderObserved := firstObserved.Add(-time.Second)
	older, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:     "src-older",
		IntentID:           "intent-freshness",
		SignalClass:        DeliverySignalInProgress,
		ProviderObservedAt: &olderObserved,
		ReceivedAt:         base.Add(-time.Second),
		CorrelationResult:  DeliveryCorrelationMatched,
	})
	if err != nil {
		t.Fatalf("apply older in_progress: %v", err)
	}
	if older.StateMutated {
		t.Fatalf("expected older signal to keep stale freshness, got %+v", older)
	}
	state, ok := loadDeliveryState(t, db, "intent-freshness")
	if !ok {
		t.Fatal("expected delivery state row")
	}
	if state.deliveryFreshness != DeliveryFreshnessStale {
		t.Fatalf("expected stale to remain stale on non-newer signal, got %q", state.deliveryFreshness)
	}

	newerObserved := firstObserved.Add(5 * time.Minute)
	newer, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:     "src-newer",
		IntentID:           "intent-freshness",
		SignalClass:        DeliverySignalInProgress,
		ProviderObservedAt: &newerObserved,
		ReceivedAt:         base.Add(5 * time.Minute),
		CorrelationResult:  DeliveryCorrelationMatched,
	})
	if err != nil {
		t.Fatalf("apply newer in_progress: %v", err)
	}
	if !newer.StateMutated {
		t.Fatalf("expected newer signal to refresh stale state, got %+v", newer)
	}
	state, ok = loadDeliveryState(t, db, "intent-freshness")
	if !ok {
		t.Fatal("expected delivery state row")
	}
	if state.deliveryFreshness != DeliveryFreshnessFresh {
		t.Fatalf("expected freshness fresh after newer signal, got %q", state.deliveryFreshness)
	}
	if !state.lastNonTerminalSourceRecordID.Valid || state.lastNonTerminalSourceRecordID.String != "src-newer" {
		t.Fatalf("expected last non-terminal source src-newer, got %+v", state.lastNonTerminalSourceRecordID)
	}
}

func TestDeliveryApplyTerminalLockConflict(t *testing.T) {
	store, db := newDeliveryStore(t)
	base := time.Date(2026, 2, 11, 3, 30, 0, 0, time.UTC)
	insertDeliveryIntent(t, store, "intent-terminal-lock", submission.DeliveryTrackingModeOn, 90, base.Add(-time.Minute))

	_, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-terminal-success",
		IntentID:          "intent-terminal-lock",
		SignalClass:       DeliverySignalTerminalSuccess,
		ReceivedAt:        base,
		CorrelationResult: DeliveryCorrelationMatched,
	})
	if err != nil {
		t.Fatalf("apply terminal success: %v", err)
	}
	conflict, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-terminal-failure",
		IntentID:          "intent-terminal-lock",
		SignalClass:       DeliverySignalTerminalFailure,
		ReceivedAt:        base.Add(time.Second),
		CorrelationResult: DeliveryCorrelationMatched,
	})
	if err != nil {
		t.Fatalf("apply terminal failure: %v", err)
	}
	if !conflict.TerminalConflict || conflict.StateMutated {
		t.Fatalf("expected history-only terminal conflict, got %+v", conflict)
	}

	state, ok := loadDeliveryState(t, db, "intent-terminal-lock")
	if !ok {
		t.Fatal("expected delivery state row")
	}
	if state.deliveryStatus != DeliveryStatusDelivered {
		t.Fatalf("expected delivered lock status, got %q", state.deliveryStatus)
	}
	if state.deliveryFreshness != DeliveryFreshnessNotApplicable {
		t.Fatalf("expected not_applicable freshness, got %q", state.deliveryFreshness)
	}
	if !state.terminalLocked {
		t.Fatal("expected terminal lock")
	}
	if !state.terminalStatus.Valid || state.terminalStatus.String != string(DeliveryStatusDelivered) {
		t.Fatalf("expected terminal status delivered, got %+v", state.terminalStatus)
	}

	history := loadDeliveryHistory(t, db, "intent-terminal-lock")
	if len(history) != 2 {
		t.Fatalf("expected 2 history rows, got %d", len(history))
	}
	if history[1].terminalConflict != true {
		t.Fatalf("expected second row terminal conflict marker, got %+v", history[1])
	}
}

func TestDeliveryApplyLateObservation(t *testing.T) {
	store, db := newDeliveryStore(t)
	base := time.Date(2026, 2, 11, 4, 0, 0, 0, time.UTC)
	insertDeliveryIntent(t, store, "intent-late", submission.DeliveryTrackingModeOn, 120, base.Add(-10*time.Minute))

	completedAt := base
	_, err := db.ExecContext(
		context.Background(),
		`UPDATE dbo.submission_intents
     SET status = @p1,
         exhausted_reason = @p2,
         next_attempt_at = NULL,
         updated_at = @p3,
         last_modified_at = @p3
     WHERE intent_id = @p4`,
		string(SubmissionStatusExhausted),
		"deadline_exceeded",
		completedAt,
		"intent-late",
	)
	if err != nil {
		t.Fatalf("seed exhausted intent: %v", err)
	}

	receivedAt := completedAt.Add(time.Second)
	result, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-late",
		IntentID:          "intent-late",
		SignalClass:       DeliverySignalTerminalSuccess,
		ReceivedAt:        receivedAt,
		CorrelationResult: DeliveryCorrelationMatched,
	})
	if err != nil {
		t.Fatalf("apply late terminal: %v", err)
	}
	if !result.LateObservation {
		t.Fatalf("expected late observation, got %+v", result)
	}

	history := loadDeliveryHistory(t, db, "intent-late")
	if len(history) != 1 || !history[0].lateObservation {
		t.Fatalf("expected history late observation marker, got %+v", history)
	}

	status := loadIntentStatus(t, db, "intent-late")
	if status != SubmissionStatusExhausted {
		t.Fatalf("expected submission status to remain exhausted, got %q", status)
	}
}

func TestDeliveryApplyAtomicRollbackOnFailure(t *testing.T) {
	store, db := newDeliveryStore(t)
	base := time.Date(2026, 2, 11, 4, 30, 0, 0, time.UTC)
	insertDeliveryIntent(t, store, "intent-rollback", submission.DeliveryTrackingModeOn, 90, base.Add(-time.Minute))

	store.deliveryApplyAfterHistoryInsert = func() error {
		return errors.New("forced delivery apply failure")
	}
	_, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-rollback",
		IntentID:          "intent-rollback",
		SignalClass:       DeliverySignalInProgress,
		ReceivedAt:        base,
		CorrelationResult: DeliveryCorrelationMatched,
	})
	if err == nil {
		t.Fatal("expected forced apply failure")
	}
	if !strings.Contains(err.Error(), "forced delivery apply failure") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNoDeliveryMutation(t, db, "intent-rollback")
}

func TestProcessorApplyCorrelatedDeliveryRecord(t *testing.T) {
	db := newTestDB(t)
	store, err := newSQLStore(db)
	if err != nil {
		t.Fatalf("new sql store: %v", err)
	}
	base := time.Date(2026, 2, 11, 4, 45, 0, 0, time.UTC)
	insertDeliveryIntent(t, store, "intent-processor-apply", submission.DeliveryTrackingModeOn, 60, base.Add(-time.Minute))

	processor, err := NewProcessor(db)
	if err != nil {
		t.Fatalf("new processor: %v", err)
	}

	result, err := processor.ApplyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-processor-apply",
		IntentID:          "intent-processor-apply",
		SignalClass:       DeliverySignalInProgress,
		ReceivedAt:        base,
		CorrelationResult: DeliveryCorrelationMatched,
	})
	if err != nil {
		t.Fatalf("processor apply: %v", err)
	}
	if !result.HistoryInserted || !result.StateMutated {
		t.Fatalf("expected history insert + state mutation, got %+v", result)
	}

	state, ok := loadDeliveryState(t, db, "intent-processor-apply")
	if !ok {
		t.Fatal("expected delivery state row")
	}
	if state.deliveryStatus != DeliveryStatusInProgress {
		t.Fatalf("expected in_progress status, got %q", state.deliveryStatus)
	}
}

func TestProcessorEvaluateDeliveryFreshnessStaleness(t *testing.T) {
	db := newTestDB(t)
	store, err := newSQLStore(db)
	if err != nil {
		t.Fatalf("new sql store: %v", err)
	}
	now := time.Now().UTC()
	base := now.Add(-10 * time.Minute)

	insertDeliveryIntent(t, store, "intent-stale-due", submission.DeliveryTrackingModeOn, 60, base.Add(-10*time.Minute))
	insertDeliveryIntent(t, store, "intent-fresh-not-due", submission.DeliveryTrackingModeOn, 3600, base.Add(-time.Minute))
	insertDeliveryIntent(t, store, "intent-terminal", submission.DeliveryTrackingModeOn, 60, base.Add(-10*time.Minute))

	oldObserved := now.Add(-5 * time.Minute)
	if _, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:     "src-due",
		IntentID:           "intent-stale-due",
		SignalClass:        DeliverySignalInProgress,
		ProviderObservedAt: &oldObserved,
		ReceivedAt:         now,
		CorrelationResult:  DeliveryCorrelationMatched,
	}); err != nil {
		t.Fatalf("apply due in_progress: %v", err)
	}

	currentObserved := now
	if _, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:     "src-future",
		IntentID:           "intent-fresh-not-due",
		SignalClass:        DeliverySignalInProgress,
		ProviderObservedAt: &currentObserved,
		ReceivedAt:         now,
		CorrelationResult:  DeliveryCorrelationMatched,
	}); err != nil {
		t.Fatalf("apply future in_progress: %v", err)
	}

	if _, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-terminal",
		IntentID:          "intent-terminal",
		SignalClass:       DeliverySignalTerminalSuccess,
		ReceivedAt:        now,
		CorrelationResult: DeliveryCorrelationMatched,
	}); err != nil {
		t.Fatalf("apply terminal: %v", err)
	}

	processor, err := NewProcessor(db)
	if err != nil {
		t.Fatalf("new processor: %v", err)
	}

	affected, err := processor.EvaluateDeliveryFreshnessStaleness(context.Background())
	if err != nil {
		t.Fatalf("evaluate freshness staleness: %v", err)
	}
	if affected != 1 {
		t.Fatalf("expected exactly one stale transition, got %d", affected)
	}

	dueState, ok := loadDeliveryState(t, db, "intent-stale-due")
	if !ok {
		t.Fatal("expected due state row")
	}
	if dueState.deliveryFreshness != DeliveryFreshnessStale {
		t.Fatalf("expected due intent to become stale, got %q", dueState.deliveryFreshness)
	}

	notDueState, ok := loadDeliveryState(t, db, "intent-fresh-not-due")
	if !ok {
		t.Fatal("expected not-due state row")
	}
	if notDueState.deliveryFreshness != DeliveryFreshnessFresh {
		t.Fatalf("expected not-due intent to remain fresh, got %q", notDueState.deliveryFreshness)
	}

	terminalState, ok := loadDeliveryState(t, db, "intent-terminal")
	if !ok {
		t.Fatal("expected terminal state row")
	}
	if terminalState.deliveryFreshness != DeliveryFreshnessNotApplicable {
		t.Fatalf("expected terminal intent to remain not_applicable, got %q", terminalState.deliveryFreshness)
	}
}

func TestProcessorEvaluateDeliveryFreshnessStalenessNoOpWithNilContext(t *testing.T) {
	db := newTestDB(t)
	processor, err := NewProcessor(db)
	if err != nil {
		t.Fatalf("new processor: %v", err)
	}

	affected, err := processor.EvaluateDeliveryFreshnessStaleness(nil)
	if err != nil {
		t.Fatalf("evaluate freshness staleness with nil context: %v", err)
	}
	if affected != 0 {
		t.Fatalf("expected no-op evaluator result 0, got %d", affected)
	}
}

func TestProcessorEvaluateDeliveryFreshnessStalenessFailsWhenStateTableUnavailable(t *testing.T) {
	db := newTestDB(t)
	processor, err := NewProcessor(db)
	if err != nil {
		t.Fatalf("new processor: %v", err)
	}

	if _, err := db.ExecContext(context.Background(), `DROP TABLE dbo.intent_delivery_state`); err != nil {
		t.Fatalf("drop delivery state table: %v", err)
	}

	if _, err := processor.EvaluateDeliveryFreshnessStaleness(context.Background()); err == nil {
		t.Fatal("expected evaluator failure when delivery state table is unavailable")
	}
}

func TestDeliveryApplyConcurrentFirstMatchedSignalsCreateSingleStateRow(t *testing.T) {
	storeA, db := newDeliveryStore(t)
	storeB, err := newSQLStore(db)
	if err != nil {
		t.Fatalf("new sql store: %v", err)
	}

	base := time.Date(2026, 2, 11, 5, 45, 0, 0, time.UTC)
	insertDeliveryIntent(t, storeA, "intent-concurrent-first-state", submission.DeliveryTrackingModeOn, 120, base.Add(-time.Minute))

	if got := deliveryStateRowCount(t, db, "intent-concurrent-first-state"); got != 0 {
		t.Fatalf("expected no state row before first matched signals, got %d", got)
	}

	olderObserved := base.Add(-time.Minute)
	newerObserved := base.Add(time.Minute)
	older := CorrelatedDeliveryRecord{
		SourceRecordID:     "src-concurrent-first-older",
		IntentID:           "intent-concurrent-first-state",
		SignalClass:        DeliverySignalInProgress,
		ProviderObservedAt: &olderObserved,
		ReceivedAt:         base,
		CorrelationResult:  DeliveryCorrelationMatched,
	}
	newer := CorrelatedDeliveryRecord{
		SourceRecordID:     "src-concurrent-first-newer",
		IntentID:           "intent-concurrent-first-state",
		SignalClass:        DeliverySignalInProgress,
		ProviderObservedAt: &newerObserved,
		ReceivedAt:         base.Add(time.Second),
		CorrelationResult:  DeliveryCorrelationMatched,
	}

	errs := make([]error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, errs[0] = storeA.applyCorrelatedDeliveryRecord(context.Background(), older)
	}()
	go func() {
		defer wg.Done()
		<-start
		_, errs[1] = storeB.applyCorrelatedDeliveryRecord(context.Background(), newer)
	}()
	close(start)
	wg.Wait()

	for i, applyErr := range errs {
		if applyErr != nil {
			t.Fatalf("apply concurrent first matched signal %d: %v", i, applyErr)
		}
	}

	if got := deliveryStateRowCount(t, db, "intent-concurrent-first-state"); got != 1 {
		t.Fatalf("expected exactly one delivery state row after concurrent first signals, got %d", got)
	}
	if got := historyCount(t, db, "intent-concurrent-first-state"); got != 2 {
		t.Fatalf("expected two history rows after concurrent first signals, got %d", got)
	}

	state, ok := loadDeliveryState(t, db, "intent-concurrent-first-state")
	if !ok {
		t.Fatal("expected delivery state row")
	}
	if !state.lastNonTerminalSourceRecordID.Valid || state.lastNonTerminalSourceRecordID.String != "src-concurrent-first-newer" {
		t.Fatalf("expected deterministic convergence to newest non-terminal key, got %+v", state.lastNonTerminalSourceRecordID)
	}
}

func TestDeliveryApplyRollbackWhenStateUpdateAffectsUnexpectedRows(t *testing.T) {
	store, db := newDeliveryStore(t)
	base := time.Date(2026, 2, 11, 6, 0, 0, 0, time.UTC)
	insertDeliveryIntent(t, store, "intent-update-rollback", submission.DeliveryTrackingModeOn, 120, base.Add(-time.Minute))

	firstObserved := base.Add(-time.Minute)
	if _, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:     "src-update-initial",
		IntentID:           "intent-update-rollback",
		SignalClass:        DeliverySignalInProgress,
		ProviderObservedAt: &firstObserved,
		ReceivedAt:         base,
		CorrelationResult:  DeliveryCorrelationMatched,
	}); err != nil {
		t.Fatalf("apply initial in_progress: %v", err)
	}
	beforeHistory := historyCount(t, db, "intent-update-rollback")
	beforeState, ok := loadDeliveryState(t, db, "intent-update-rollback")
	if !ok {
		t.Fatal("expected initial delivery state row")
	}

	createAfterInsertHistoryTrigger(
		t,
		db,
		"trg_delete_state_before_update",
		`DELETE s
FROM dbo.intent_delivery_state s
INNER JOIN inserted i ON i.intent_id = s.intent_id
WHERE i.source_record_id = 'src-update-fail';`,
	)

	nextObserved := base.Add(2 * time.Minute)
	_, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:     "src-update-fail",
		IntentID:           "intent-update-rollback",
		SignalClass:        DeliverySignalInProgress,
		ProviderObservedAt: &nextObserved,
		ReceivedAt:         base.Add(time.Second),
		CorrelationResult:  DeliveryCorrelationMatched,
	})
	if err == nil {
		t.Fatal("expected state update row-count failure")
	}
	if !strings.Contains(err.Error(), "delivery state update affected unexpected row count") {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := historyCount(t, db, "intent-update-rollback"); got != beforeHistory {
		t.Fatalf("expected history rollback to preserve count=%d, got %d", beforeHistory, got)
	}
	afterState, ok := loadDeliveryState(t, db, "intent-update-rollback")
	if !ok {
		t.Fatal("expected delivery state row after rollback")
	}
	if !afterState.lastNonTerminalSourceRecordID.Valid || !beforeState.lastNonTerminalSourceRecordID.Valid || afterState.lastNonTerminalSourceRecordID.String != beforeState.lastNonTerminalSourceRecordID.String {
		t.Fatalf("expected state rollback to preserve last non-terminal source, before=%+v after=%+v", beforeState.lastNonTerminalSourceRecordID, afterState.lastNonTerminalSourceRecordID)
	}
}

func TestDeliveryApplyRollbackWhenHistoryAnnotationAffectsUnexpectedRows(t *testing.T) {
	store, db := newDeliveryStore(t)
	base := time.Date(2026, 2, 11, 6, 15, 0, 0, time.UTC)
	insertDeliveryIntent(t, store, "intent-annotation-rollback", submission.DeliveryTrackingModeOn, 120, base.Add(-time.Minute))

	if _, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-terminal-lock",
		IntentID:          "intent-annotation-rollback",
		SignalClass:       DeliverySignalTerminalSuccess,
		ReceivedAt:        base,
		CorrelationResult: DeliveryCorrelationMatched,
	}); err != nil {
		t.Fatalf("apply terminal lock signal: %v", err)
	}
	beforeHistory := historyCount(t, db, "intent-annotation-rollback")
	beforeState, ok := loadDeliveryState(t, db, "intent-annotation-rollback")
	if !ok {
		t.Fatal("expected delivery state row before annotation rollback test")
	}

	createAfterInsertHistoryTrigger(
		t,
		db,
		"trg_delete_history_before_annotation",
		`DELETE h
FROM dbo.intent_delivery_history h
INNER JOIN inserted i ON i.intent_id = h.intent_id AND i.source_record_id = h.source_record_id
WHERE i.source_record_id = 'src-terminal-conflict';`,
	)

	_, err := store.applyCorrelatedDeliveryRecord(context.Background(), CorrelatedDeliveryRecord{
		SourceRecordID:    "src-terminal-conflict",
		IntentID:          "intent-annotation-rollback",
		SignalClass:       DeliverySignalTerminalFailure,
		ReceivedAt:        base.Add(time.Second),
		CorrelationResult: DeliveryCorrelationMatched,
	})
	if err == nil {
		t.Fatal("expected history annotation row-count failure")
	}
	if !strings.Contains(err.Error(), "delivery history annotation update affected unexpected row count") {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := historyCount(t, db, "intent-annotation-rollback"); got != beforeHistory {
		t.Fatalf("expected history rollback to preserve count=%d, got %d", beforeHistory, got)
	}
	afterState, ok := loadDeliveryState(t, db, "intent-annotation-rollback")
	if !ok {
		t.Fatal("expected delivery state row after annotation rollback")
	}
	if afterState.deliveryStatus != beforeState.deliveryStatus || afterState.deliveryFreshness != beforeState.deliveryFreshness || afterState.terminalLocked != beforeState.terminalLocked {
		t.Fatalf("expected state rollback to preserve locked terminal state, before=%+v after=%+v", beforeState, afterState)
	}
}

func TestInsertInitialDeliveryStateDuplicateIsNoOp(t *testing.T) {
	store, db := newDeliveryStore(t)
	base := time.Date(2026, 2, 11, 6, 30, 0, 0, time.UTC)
	insertDeliveryIntent(t, store, "intent-initial-state-dup", submission.DeliveryTrackingModeOn, 120, base.Add(-time.Minute))

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := insertInitialDeliveryState(context.Background(), tx, "intent-initial-state-dup", base.Add(-time.Minute), 120, base); err != nil {
		t.Fatalf("first insertInitialDeliveryState: %v", err)
	}
	if err := insertInitialDeliveryState(context.Background(), tx, "intent-initial-state-dup", base.Add(-time.Minute), 120, base.Add(time.Second)); err != nil {
		t.Fatalf("second insertInitialDeliveryState should be duplicate no-op, got: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit tx: %v", err)
	}
	if got := deliveryStateRowCount(t, db, "intent-initial-state-dup"); got != 1 {
		t.Fatalf("expected one delivery state row after duplicate insert no-op, got %d", got)
	}
}

func TestInsertInitialDeliveryStateReturnsErrorOnNonUniqueFailure(t *testing.T) {
	db := newTestDB(t)
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	err = insertInitialDeliveryState(context.Background(), tx, "missing-intent", time.Now().UTC(), 120, time.Now().UTC())
	if err == nil {
		t.Fatal("expected non-unique insertInitialDeliveryState failure for missing parent intent")
	}
}

func TestDeliveryApplyConcurrentDuplicateSourceRecordID(t *testing.T) {
	storeA, db := newDeliveryStore(t)
	storeB, err := newSQLStore(db)
	if err != nil {
		t.Fatalf("new sql store: %v", err)
	}

	base := time.Date(2026, 2, 11, 5, 0, 0, 0, time.UTC)
	insertDeliveryIntent(t, storeA, "intent-concurrent-dup", submission.DeliveryTrackingModeOn, 120, base.Add(-time.Minute))

	record := CorrelatedDeliveryRecord{
		SourceRecordID:    "src-concurrent-dup",
		IntentID:          "intent-concurrent-dup",
		SignalClass:       DeliverySignalInProgress,
		ReceivedAt:        base,
		CorrelationResult: DeliveryCorrelationMatched,
	}

	results := make([]DeliveryApplyResult, 2)
	errs := make([]error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		results[0], errs[0] = storeA.applyCorrelatedDeliveryRecord(context.Background(), record)
	}()
	go func() {
		defer wg.Done()
		<-start
		results[1], errs[1] = storeB.applyCorrelatedDeliveryRecord(context.Background(), record)
	}()
	close(start)
	wg.Wait()

	for i, applyErr := range errs {
		if applyErr != nil {
			t.Fatalf("apply concurrent duplicate %d: %v", i, applyErr)
		}
	}

	mutated := 0
	duplicateNoOp := 0
	for _, result := range results {
		if result.StateMutated && result.HistoryInserted {
			mutated++
		}
		if result.NoOpReason == deliveryNoOpReasonDuplicateSource {
			duplicateNoOp++
		}
	}
	if mutated != 1 || duplicateNoOp != 1 {
		t.Fatalf("expected one mutation and one duplicate no-op, got %+v", results)
	}

	if historyCount(t, db, "intent-concurrent-dup") != 1 {
		t.Fatalf("expected 1 history row after concurrent duplicate apply")
	}
	state, ok := loadDeliveryState(t, db, "intent-concurrent-dup")
	if !ok {
		t.Fatal("expected delivery state row")
	}
	if state.deliveryStatus != DeliveryStatusInProgress {
		t.Fatalf("expected in_progress status, got %q", state.deliveryStatus)
	}
}

func TestDeliveryApplyConcurrentOutOfOrderInProgressConverges(t *testing.T) {
	storeA, db := newDeliveryStore(t)
	storeB, err := newSQLStore(db)
	if err != nil {
		t.Fatalf("new sql store: %v", err)
	}

	base := time.Date(2026, 2, 11, 5, 15, 0, 0, time.UTC)
	insertDeliveryIntent(t, storeA, "intent-concurrent-order", submission.DeliveryTrackingModeOn, 120, base.Add(-time.Minute))

	olderObserved := base.Add(-time.Minute)
	newerObserved := base.Add(2 * time.Minute)
	older := CorrelatedDeliveryRecord{
		SourceRecordID:     "src-older-concurrent",
		IntentID:           "intent-concurrent-order",
		SignalClass:        DeliverySignalInProgress,
		ProviderObservedAt: &olderObserved,
		ReceivedAt:         base,
		CorrelationResult:  DeliveryCorrelationMatched,
	}
	newer := CorrelatedDeliveryRecord{
		SourceRecordID:     "src-newer-concurrent",
		IntentID:           "intent-concurrent-order",
		SignalClass:        DeliverySignalInProgress,
		ProviderObservedAt: &newerObserved,
		ReceivedAt:         base.Add(time.Second),
		CorrelationResult:  DeliveryCorrelationMatched,
	}

	errs := make([]error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, errs[0] = storeA.applyCorrelatedDeliveryRecord(context.Background(), older)
	}()
	go func() {
		defer wg.Done()
		<-start
		_, errs[1] = storeB.applyCorrelatedDeliveryRecord(context.Background(), newer)
	}()
	close(start)
	wg.Wait()

	for i, applyErr := range errs {
		if applyErr != nil {
			t.Fatalf("apply concurrent ordering %d: %v", i, applyErr)
		}
	}

	state, ok := loadDeliveryState(t, db, "intent-concurrent-order")
	if !ok {
		t.Fatal("expected delivery state row")
	}
	if state.deliveryStatus != DeliveryStatusInProgress {
		t.Fatalf("expected in_progress status, got %q", state.deliveryStatus)
	}
	if state.deliveryFreshness != DeliveryFreshnessFresh {
		t.Fatalf("expected fresh freshness, got %q", state.deliveryFreshness)
	}
	if !state.lastNonTerminalSourceRecordID.Valid || state.lastNonTerminalSourceRecordID.String != "src-newer-concurrent" {
		t.Fatalf("expected newest key source src-newer-concurrent, got %+v", state.lastNonTerminalSourceRecordID)
	}
	if historyCount(t, db, "intent-concurrent-order") != 2 {
		t.Fatalf("expected 2 history rows after concurrent apply")
	}
}

func TestDeliveryApplyConcurrentOppositeTerminalConverges(t *testing.T) {
	storeA, db := newDeliveryStore(t)
	storeB, err := newSQLStore(db)
	if err != nil {
		t.Fatalf("new sql store: %v", err)
	}

	base := time.Date(2026, 2, 11, 5, 30, 0, 0, time.UTC)
	insertDeliveryIntent(t, storeA, "intent-concurrent-terminal", submission.DeliveryTrackingModeOn, 120, base.Add(-time.Minute))

	success := CorrelatedDeliveryRecord{
		SourceRecordID:    "src-concurrent-terminal-success",
		IntentID:          "intent-concurrent-terminal",
		SignalClass:       DeliverySignalTerminalSuccess,
		ReceivedAt:        base,
		CorrelationResult: DeliveryCorrelationMatched,
	}
	failure := CorrelatedDeliveryRecord{
		SourceRecordID:    "src-concurrent-terminal-failure",
		IntentID:          "intent-concurrent-terminal",
		SignalClass:       DeliverySignalTerminalFailure,
		ReceivedAt:        base.Add(time.Second),
		CorrelationResult: DeliveryCorrelationMatched,
	}

	results := make([]DeliveryApplyResult, 2)
	errs := make([]error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		results[0], errs[0] = storeA.applyCorrelatedDeliveryRecord(context.Background(), success)
	}()
	go func() {
		defer wg.Done()
		<-start
		results[1], errs[1] = storeB.applyCorrelatedDeliveryRecord(context.Background(), failure)
	}()
	close(start)
	wg.Wait()

	for i, applyErr := range errs {
		if applyErr != nil {
			t.Fatalf("apply concurrent terminal %d: %v", i, applyErr)
		}
	}

	state, ok := loadDeliveryState(t, db, "intent-concurrent-terminal")
	if !ok {
		t.Fatal("expected delivery state row")
	}
	if !state.terminalLocked {
		t.Fatal("expected terminal lock")
	}
	if state.deliveryStatus != DeliveryStatusDelivered && state.deliveryStatus != DeliveryStatusFailed {
		t.Fatalf("expected terminal delivered/failed status, got %q", state.deliveryStatus)
	}
	if state.deliveryFreshness != DeliveryFreshnessNotApplicable {
		t.Fatalf("expected not_applicable freshness, got %q", state.deliveryFreshness)
	}

	history := loadDeliveryHistory(t, db, "intent-concurrent-terminal")
	if len(history) != 2 {
		t.Fatalf("expected 2 history rows, got %d", len(history))
	}
	conflicts := 0
	for _, row := range history {
		if row.terminalConflict {
			conflicts++
		}
	}
	if conflicts != 1 {
		t.Fatalf("expected exactly one terminal conflict row, got %d history=%+v results=%+v", conflicts, history, results)
	}
}

func newDeliveryStore(t *testing.T) (*sqlStore, *sql.DB) {
	t.Helper()
	db := newTestDB(t)
	store, err := newSQLStore(db)
	if err != nil {
		t.Fatalf("new sql store: %v", err)
	}
	return store, db
}

func insertDeliveryIntent(t *testing.T, store *sqlStore, intentID string, mode submission.DeliveryTrackingMode, staleAfter int, createdAt time.Time) {
	t.Helper()
	payload := []byte(`{"sms":"hello"}`)
	sum := sha256.Sum256(payload)
	var staleValue sql.NullInt32
	if mode == submission.DeliveryTrackingModeOn {
		staleValue = sql.NullInt32{Int32: int32(staleAfter), Valid: true}
	}
	_, err := store.db.ExecContext(
		context.Background(),
		`INSERT INTO dbo.submission_intents (
      intent_id,
      submission_target,
      payload,
      payload_hash,
      gateway_type,
      gateway_url,
      policy,
      max_acceptance_seconds,
      max_attempts,
      delivery_tracking_mode,
      delivery_tracking_stale_after_seconds,
      terminal_outcomes,
      webhook_url,
      webhook_headers,
      webhook_headers_env,
      webhook_secret_env,
      webhook_status,
      webhook_attempted_at,
      webhook_delivered_at,
      webhook_error,
      status,
      final_outcome_status,
      final_outcome_reason,
      exhausted_reason,
      attempt_count,
      created_at,
      updated_at,
      last_modified_at,
      next_attempt_at
    ) VALUES (
      @p1, @p2, @p3, @p4, @p5, @p6, @p7, NULL, NULL, @p8, @p9, @p10, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, @p11, NULL, NULL, NULL, 0, @p12, @p12, @p12, NULL
    )`,
		intentID,
		"sms.realtime",
		payload,
		sum[:],
		string(submission.GatewaySMS),
		"http://gateway.local",
		string(submission.PolicyOneShot),
		string(mode),
		staleValue,
		`["invalid_request"]`,
		"pending",
		createdAt.UTC(),
	)
	if err != nil {
		t.Fatalf("insert intent: %v", err)
	}
}

func assertNoDeliveryMutation(t *testing.T, db *sql.DB, intentID string) {
	t.Helper()
	if historyCount(t, db, intentID) != 0 {
		t.Fatalf("expected no delivery history rows for %q", intentID)
	}
	if _, ok := loadDeliveryState(t, db, intentID); ok {
		t.Fatalf("expected no delivery state row for %q", intentID)
	}
}

func historyCount(t *testing.T, db *sql.DB, intentID string) int {
	t.Helper()
	row := db.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM dbo.intent_delivery_history WHERE intent_id = @p1`, intentID)
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count history: %v", err)
	}
	return count
}

func deliveryStateRowCount(t *testing.T, db *sql.DB, intentID string) int {
	t.Helper()
	row := db.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM dbo.intent_delivery_state WHERE intent_id = @p1`, intentID)
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count delivery state rows: %v", err)
	}
	return count
}

func createAfterInsertHistoryTrigger(t *testing.T, db *sql.DB, triggerName, body string) {
	t.Helper()
	if _, err := db.ExecContext(
		context.Background(),
		fmt.Sprintf(
			`CREATE TRIGGER dbo.%s
ON dbo.intent_delivery_history
AFTER INSERT
AS
BEGIN
  SET NOCOUNT ON;
  %s
END;`,
			triggerName,
			body,
		),
	); err != nil {
		t.Fatalf("create after-insert history trigger %s: %v", triggerName, err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(
			context.Background(),
			fmt.Sprintf(
				`IF OBJECT_ID('dbo.%s', 'TR') IS NOT NULL
BEGIN
  DROP TRIGGER dbo.%s;
END;`,
				triggerName,
				triggerName,
			),
		)
	})
}

func forceFreshness(t *testing.T, db *sql.DB, intentID string, freshness DeliveryFreshness) {
	t.Helper()
	_, err := db.ExecContext(
		context.Background(),
		`UPDATE dbo.intent_delivery_state
     SET delivery_freshness = @p1
     WHERE intent_id = @p2`,
		string(freshness),
		intentID,
	)
	if err != nil {
		t.Fatalf("force freshness: %v", err)
	}
}

func loadDeliveryState(t *testing.T, db *sql.DB, intentID string) (deliveryStateProjection, bool) {
	t.Helper()
	row := db.QueryRowContext(
		context.Background(),
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
    FROM dbo.intent_delivery_state
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
			return deliveryStateProjection{}, false
		}
		t.Fatalf("load delivery state: %v", err)
	}
	state.deliveryStatus = DeliveryStatus(deliveryStatus)
	state.deliveryFreshness = DeliveryFreshness(deliveryFreshness)
	return state, true
}

type deliveryHistoryRow struct {
	sourceRecordID   string
	signalClass      DeliverySignalClass
	lateObservation  bool
	terminalConflict bool
}

func loadDeliveryHistory(t *testing.T, db *sql.DB, intentID string) []deliveryHistoryRow {
	t.Helper()
	rows, err := db.QueryContext(
		context.Background(),
		`SELECT source_record_id, signal_class, late_observation, terminal_conflict
     FROM dbo.intent_delivery_history
     WHERE intent_id = @p1
     ORDER BY history_seq`,
		intentID,
	)
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	defer rows.Close()

	var history []deliveryHistoryRow
	for rows.Next() {
		var row deliveryHistoryRow
		var signalClass string
		if err := rows.Scan(&row.sourceRecordID, &signalClass, &row.lateObservation, &row.terminalConflict); err != nil {
			t.Fatalf("scan history: %v", err)
		}
		row.signalClass = DeliverySignalClass(signalClass)
		history = append(history, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate history: %v", err)
	}
	return history
}

func loadIntentStatus(t *testing.T, db *sql.DB, intentID string) SubmissionStatus {
	t.Helper()
	row := db.QueryRowContext(context.Background(), `SELECT status FROM dbo.submission_intents WHERE intent_id = @p1`, intentID)
	var status string
	if err := row.Scan(&status); err != nil {
		t.Fatalf("load intent status: %v", err)
	}
	return SubmissionStatus(status)
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	password, ok, err := resolveSQLPassword(t)
	if err != nil {
		t.Fatalf("resolve sql password: %v", err)
	}
	if !ok {
		t.Skip("MSSQL_SA_PASSWORD not set; start docker compose and set env or backend/.env")
	}

	host := envOrDefault("MSSQL_HOST", "localhost")
	port := envOrDefault("MSSQL_PORT", "1433")

	masterDB, err := sql.Open("sqlserver", buildSQLServerDSN(host, port, password, "master"))
	if err != nil {
		t.Fatalf("open master db: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	if err := masterDB.PingContext(ctx); err != nil {
		_ = masterDB.Close()
		t.Fatalf("ping master db: %v", err)
	}

	dbName := fmt.Sprintf("deliverytracking_test_%d", time.Now().UnixNano())
	if _, err := masterDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE [%s]", dbName)); err != nil {
		_ = masterDB.Close()
		t.Fatalf("create database: %v", err)
	}

	db, err := sql.Open("sqlserver", buildSQLServerDSN(host, port, password, dbName))
	if err != nil {
		_ = dropTestDB(ctx, masterDB, dbName)
		t.Fatalf("open test db: %v", err)
	}

	schemaPath := filepath.Join(moduleRoot(t), "conf", "sql", "submissionmanager", "001_create_schema.sql")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		_ = db.Close()
		_ = dropTestDB(ctx, masterDB, dbName)
		t.Fatalf("read schema: %v", err)
	}
	if _, err := db.ExecContext(ctx, string(schema)); err != nil {
		_ = db.Close()
		_ = dropTestDB(ctx, masterDB, dbName)
		t.Fatalf("apply schema: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
		_ = dropTestDB(context.Background(), masterDB, dbName)
		_ = masterDB.Close()
	})

	return db
}

func dropTestDB(ctx context.Context, masterDB *sql.DB, dbName string) error {
	_, _ = masterDB.ExecContext(ctx, fmt.Sprintf("ALTER DATABASE [%s] SET SINGLE_USER WITH ROLLBACK IMMEDIATE", dbName))
	_, err := masterDB.ExecContext(ctx, fmt.Sprintf("DROP DATABASE [%s]", dbName))
	return err
}

func buildSQLServerDSN(host, port, password, database string) string {
	u := &url.URL{
		Scheme: "sqlserver",
		User:   url.UserPassword("sa", password),
		Host:   fmt.Sprintf("%s:%s", host, port),
	}
	query := url.Values{}
	query.Set("database", database)
	query.Set("encrypt", "disable")
	u.RawQuery = query.Encode()
	return u.String()
}

func resolveSQLPassword(t *testing.T) (string, bool, error) {
	t.Helper()
	if value, ok := os.LookupEnv("MSSQL_SA_PASSWORD"); ok && strings.TrimSpace(value) != "" {
		return value, true, nil
	}

	data, err := os.ReadFile(filepath.Join(moduleRoot(t), ".env"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "MSSQL_SA_PASSWORD" && value != "" {
			return strings.Trim(value, "\"'"), true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", false, err
	}
	return "", false, nil
}

func envOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return fallback
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve module root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), ".."))
}
