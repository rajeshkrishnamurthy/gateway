package submissionmanager

import (
	"context"
	"database/sql"
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"gateway/submission"
)

type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

type fakeTimer struct {
	at    time.Time
	ch    chan time.Time
	fired bool
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	if d <= 0 {
		ch <- c.now
		return ch
	}
	timer := &fakeTimer{at: c.now.Add(d), ch: ch}
	c.timers = append(c.timers, timer)
	return ch
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	now := c.now
	timers := append([]*fakeTimer(nil), c.timers...)
	c.mu.Unlock()

	for _, timer := range timers {
		c.mu.Lock()
		if timer.fired || now.Before(timer.at) {
			c.mu.Unlock()
			continue
		}
		timer.fired = true
		ch := timer.ch
		c.mu.Unlock()
		ch <- now
	}
}

type execResult struct {
	outcome GatewayOutcome
	err     error
	advance time.Duration
}

type stubExecutor struct {
	mu      sync.Mutex
	results []execResult
	calls   chan AttemptInput
	clock   *fakeClock
}

func newStubExecutor(clock *fakeClock, results []execResult) *stubExecutor {
	return &stubExecutor{
		results: results,
		calls:   make(chan AttemptInput, 10),
		clock:   clock,
	}
}

func (s *stubExecutor) Exec(ctx context.Context, input AttemptInput) (GatewayOutcome, error) {
	_ = ctx
	s.calls <- input
	s.mu.Lock()
	if len(s.results) == 0 {
		s.mu.Unlock()
		return GatewayOutcome{}, errors.New("no stub result")
	}
	res := s.results[0]
	s.results = s.results[1:]
	s.mu.Unlock()
	if res.advance != 0 && s.clock != nil {
		s.clock.Advance(res.advance)
	}
	return res.outcome, res.err
}

func waitForCall(t *testing.T, calls <-chan AttemptInput) AttemptInput {
	t.Helper()
	select {
	case input := <-calls:
		return input
	case <-time.After(2 * time.Second):
		t.Fatalf("executor was not called")
	}
	return AttemptInput{}
}

func assertNoCall(t *testing.T, calls <-chan AttemptInput) {
	t.Helper()
	select {
	case <-calls:
		t.Fatalf("unexpected executor call")
	case <-time.After(100 * time.Millisecond):
	}
}

func waitForStatus(t *testing.T, manager *Manager, intentID string, status IntentStatus) Intent {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		intent, ok := manager.GetIntent(intentID)
		if ok && intent.Status == status {
			return intent
		}
		runtime.Gosched()
	}
	t.Fatalf("intent %q did not reach status %q", intentID, status)
	return Intent{}
}

func waitForAttempts(t *testing.T, manager *Manager, intentID string, count int) Intent {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		intent, ok := manager.GetIntent(intentID)
		if ok && len(intent.Attempts) == count {
			return intent
		}
		runtime.Gosched()
	}
	t.Fatalf("intent %q did not reach %d attempts", intentID, count)
	return Intent{}
}

func baseContract(policy submission.ContractPolicy) submission.TargetContract {
	contract := submission.TargetContract{
		SubmissionTarget: "sms.realtime",
		GatewayType:      submission.GatewaySMS,
		GatewayURL:       "http://sms",
		Policy:           policy,
		TerminalOutcomes: []string{"invalid_request"},
	}
	switch policy {
	case submission.PolicyDeadline:
		contract.MaxAcceptanceSeconds = 10
	case submission.PolicyMaxAttempts:
		contract.MaxAttempts = 2
	}
	return contract
}

func newManager(t *testing.T, reg submission.Registry, exec AttemptExecutor, clock *fakeClock, db *sql.DB) *Manager {
	t.Helper()
	manager, err := NewManager(reg, exec, Clock{Now: clock.Now, After: clock.After}, db)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	return manager
}

func startManager(t *testing.T, manager *Manager) (context.Context, context.CancelFunc, chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		manager.Run(ctx)
		close(done)
	}()
	return ctx, cancel, done
}

func TestSubmitIntentIdempotency(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	db := newTestDB(t)
	contract := baseContract(submission.PolicyOneShot)
	reg := submission.Registry{Targets: map[string]submission.TargetContract{contract.SubmissionTarget: contract}}
	stub := newStubExecutor(clock, nil)
	manager := newManager(t, reg, stub.Exec, clock, db)

	intent := Intent{
		IntentID:         "intent-1",
		SubmissionTarget: contract.SubmissionTarget,
		Payload:          []byte(`{"a":1}`),
	}

	first, err := manager.SubmitIntent(context.Background(), intent)
	if err != nil {
		t.Fatalf("submit intent: %v", err)
	}
	second, err := manager.SubmitIntent(context.Background(), intent)
	if err != nil {
		t.Fatalf("submit intent again: %v", err)
	}
	if first.IntentID != second.IntentID || first.SubmissionTarget != second.SubmissionTarget {
		t.Fatalf("expected idempotent submit, got %+v and %+v", first, second)
	}

	_, err = manager.SubmitIntent(context.Background(), Intent{
		IntentID:         "intent-1",
		SubmissionTarget: contract.SubmissionTarget,
		Payload:          []byte(`{"a":2}`),
	})
	var conflict IdempotencyConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
	if conflict.ExistingStatus != IntentPending {
		t.Fatalf("expected existing status pending, got %q", conflict.ExistingStatus)
	}
	if conflict.ExistingPayload == conflict.IncomingPayload {
		t.Fatalf("expected conflicting payloads to differ")
	}
}

func TestIdempotencyAcrossRestart(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	db := newTestDB(t)
	contract := baseContract(submission.PolicyOneShot)
	reg := submission.Registry{Targets: map[string]submission.TargetContract{contract.SubmissionTarget: contract}}
	stub := newStubExecutor(clock, nil)
	manager := newManager(t, reg, stub.Exec, clock, db)

	intent := Intent{
		IntentID:         "intent-1",
		SubmissionTarget: contract.SubmissionTarget,
		Payload:          []byte(`{"a":1}`),
	}

	first, err := manager.SubmitIntent(context.Background(), intent)
	if err != nil {
		t.Fatalf("submit intent: %v", err)
	}

	manager = newManager(t, reg, stub.Exec, clock, db)
	second, err := manager.SubmitIntent(context.Background(), intent)
	if err != nil {
		t.Fatalf("submit intent again: %v", err)
	}
	if first.IntentID != second.IntentID || first.SubmissionTarget != second.SubmissionTarget {
		t.Fatalf("expected idempotent submit after restart, got %+v and %+v", first, second)
	}
}

func TestTerminalOutcomeRejects(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	db := newTestDB(t)
	contract := baseContract(submission.PolicyMaxAttempts)
	reg := submission.Registry{Targets: map[string]submission.TargetContract{contract.SubmissionTarget: contract}}
	stub := newStubExecutor(clock, []execResult{{
		outcome: GatewayOutcome{Status: "rejected", Reason: "invalid_request"},
	}})
	manager := newManager(t, reg, stub.Exec, clock, db)
	_, cancel, done := startManager(t, manager)
	defer func() {
		cancel()
		<-done
	}()

	_, err := manager.SubmitIntent(context.Background(), Intent{IntentID: "intent-1", SubmissionTarget: contract.SubmissionTarget})
	if err != nil {
		t.Fatalf("submit intent: %v", err)
	}
	waitForCall(t, stub.calls)
	intent := waitForStatus(t, manager, "intent-1", IntentRejected)
	if intent.FinalOutcome.Reason != "invalid_request" {
		t.Fatalf("expected terminal rejection reason, got %q", intent.FinalOutcome.Reason)
	}
	if len(intent.Attempts) != 1 {
		t.Fatalf("expected 1 attempt, got %d", len(intent.Attempts))
	}
	select {
	case <-stub.calls:
		t.Fatalf("unexpected retry for terminal rejection")
	default:
	}
}

func TestRetryDelaySchedulingAndAcceptance(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	db := newTestDB(t)
	contract := baseContract(submission.PolicyMaxAttempts)
	reg := submission.Registry{Targets: map[string]submission.TargetContract{contract.SubmissionTarget: contract}}
	stub := newStubExecutor(clock, []execResult{
		{outcome: GatewayOutcome{Status: "rejected", Reason: "provider_failure"}},
		{outcome: GatewayOutcome{Status: "accepted"}},
	})
	manager := newManager(t, reg, stub.Exec, clock, db)
	_, cancel, done := startManager(t, manager)
	defer func() {
		cancel()
		<-done
	}()

	_, err := manager.SubmitIntent(context.Background(), Intent{IntentID: "intent-1", SubmissionTarget: contract.SubmissionTarget})
	if err != nil {
		t.Fatalf("submit intent: %v", err)
	}
	waitForCall(t, stub.calls)
	waitForAttempts(t, manager, "intent-1", 1)

	select {
	case <-stub.calls:
		t.Fatalf("unexpected retry before advancing clock")
	default:
	}

	clock.Advance(4 * time.Second)
	select {
	case <-stub.calls:
		t.Fatalf("unexpected retry before fixed delay elapsed")
	default:
	}

	clock.Advance(1 * time.Second)
	waitForCall(t, stub.calls)
	intent := waitForStatus(t, manager, "intent-1", IntentAccepted)
	if len(intent.Attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(intent.Attempts))
	}
}

func TestRestartRebuildsSchedule(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	db := newTestDB(t)
	contract := baseContract(submission.PolicyMaxAttempts)
	reg := submission.Registry{Targets: map[string]submission.TargetContract{contract.SubmissionTarget: contract}}
	stub := newStubExecutor(clock, []execResult{
		{outcome: GatewayOutcome{Status: "rejected", Reason: "provider_failure"}},
		{outcome: GatewayOutcome{Status: "accepted"}},
	})
	manager := newManager(t, reg, stub.Exec, clock, db)
	_, cancel, done := startManager(t, manager)

	_, err := manager.SubmitIntent(context.Background(), Intent{IntentID: "intent-1", SubmissionTarget: contract.SubmissionTarget})
	if err != nil {
		cancel()
		<-done
		t.Fatalf("submit intent: %v", err)
	}
	waitForCall(t, stub.calls)
	waitForAttempts(t, manager, "intent-1", 1)
	cancel()
	<-done

	manager = newManager(t, reg, stub.Exec, clock, db)
	_, cancel, done = startManager(t, manager)
	defer func() {
		cancel()
		<-done
	}()

	clock.Advance(5 * time.Second)
	waitForCall(t, stub.calls)
	intent := waitForStatus(t, manager, "intent-1", IntentAccepted)
	if len(intent.Attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(intent.Attempts))
	}
}

func TestUnknownRejectionReasonIsRetryable(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	db := newTestDB(t)
	contract := baseContract(submission.PolicyMaxAttempts)
	reg := submission.Registry{Targets: map[string]submission.TargetContract{contract.SubmissionTarget: contract}}
	stub := newStubExecutor(clock, []execResult{
		{outcome: GatewayOutcome{Status: "rejected", Reason: "new_reason"}},
		{outcome: GatewayOutcome{Status: "accepted"}},
	})
	manager := newManager(t, reg, stub.Exec, clock, db)
	_, cancel, done := startManager(t, manager)
	defer func() {
		cancel()
		<-done
	}()

	_, err := manager.SubmitIntent(context.Background(), Intent{IntentID: "intent-1", SubmissionTarget: contract.SubmissionTarget})
	if err != nil {
		t.Fatalf("submit intent: %v", err)
	}
	waitForCall(t, stub.calls)
	waitForAttempts(t, manager, "intent-1", 1)

	clock.Advance(5 * time.Second)
	waitForCall(t, stub.calls)
	intent := waitForStatus(t, manager, "intent-1", IntentAccepted)
	if len(intent.Attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(intent.Attempts))
	}
	if intent.Attempts[0].Error != "" {
		t.Fatalf("expected no attempt error for non-terminal reason, got %q", intent.Attempts[0].Error)
	}
}

func TestDeadlineAcceptanceAfterDeadlineExhausted(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	db := newTestDB(t)
	contract := baseContract(submission.PolicyDeadline)
	reg := submission.Registry{Targets: map[string]submission.TargetContract{contract.SubmissionTarget: contract}}
	stub := newStubExecutor(clock, []execResult{{
		outcome: GatewayOutcome{Status: "accepted"},
		advance: 15 * time.Second,
	}})
	manager := newManager(t, reg, stub.Exec, clock, db)
	_, cancel, done := startManager(t, manager)
	defer func() {
		cancel()
		<-done
	}()

	_, err := manager.SubmitIntent(context.Background(), Intent{IntentID: "intent-1", SubmissionTarget: contract.SubmissionTarget})
	if err != nil {
		t.Fatalf("submit intent: %v", err)
	}
	waitForCall(t, stub.calls)
	intent := waitForStatus(t, manager, "intent-1", IntentExhausted)
	if intent.ExhaustedReason != "deadline_exceeded" {
		t.Fatalf("expected deadline_exceeded, got %q", intent.ExhaustedReason)
	}
	if intent.FinalOutcome.Status != "" {
		t.Fatalf("expected no final outcome on exhaustion")
	}
}

func TestDeadlineRetryNotScheduledAfterCutoff(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	db := newTestDB(t)
	contract := baseContract(submission.PolicyDeadline)
	reg := submission.Registry{Targets: map[string]submission.TargetContract{contract.SubmissionTarget: contract}}
	stub := newStubExecutor(clock, []execResult{{
		outcome: GatewayOutcome{Status: "rejected", Reason: "provider_failure"},
		advance: 9 * time.Second,
	}})
	manager := newManager(t, reg, stub.Exec, clock, db)
	_, cancel, done := startManager(t, manager)
	defer func() {
		cancel()
		<-done
	}()

	_, err := manager.SubmitIntent(context.Background(), Intent{IntentID: "intent-1", SubmissionTarget: contract.SubmissionTarget})
	if err != nil {
		t.Fatalf("submit intent: %v", err)
	}
	waitForCall(t, stub.calls)
	intent := waitForStatus(t, manager, "intent-1", IntentExhausted)
	if intent.ExhaustedReason != "deadline_exceeded" {
		t.Fatalf("expected deadline_exceeded, got %q", intent.ExhaustedReason)
	}

	clock.Advance(20 * time.Second)
	assertNoCall(t, stub.calls)
}

func TestMaxAttemptsExhausted(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	db := newTestDB(t)
	contract := baseContract(submission.PolicyMaxAttempts)
	reg := submission.Registry{Targets: map[string]submission.TargetContract{contract.SubmissionTarget: contract}}
	stub := newStubExecutor(clock, []execResult{
		{outcome: GatewayOutcome{Status: "rejected", Reason: "provider_failure"}},
		{outcome: GatewayOutcome{Status: "rejected", Reason: "provider_failure"}},
	})
	manager := newManager(t, reg, stub.Exec, clock, db)
	_, cancel, done := startManager(t, manager)
	defer func() {
		cancel()
		<-done
	}()

	_, err := manager.SubmitIntent(context.Background(), Intent{IntentID: "intent-1", SubmissionTarget: contract.SubmissionTarget})
	if err != nil {
		t.Fatalf("submit intent: %v", err)
	}
	waitForCall(t, stub.calls)
	clock.Advance(5 * time.Second)
	waitForCall(t, stub.calls)
	intent := waitForStatus(t, manager, "intent-1", IntentExhausted)
	if intent.ExhaustedReason != "max_attempts" {
		t.Fatalf("expected max_attempts, got %q", intent.ExhaustedReason)
	}
	if len(intent.Attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(intent.Attempts))
	}
}

func TestOneShotExhausted(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	db := newTestDB(t)
	contract := baseContract(submission.PolicyOneShot)
	reg := submission.Registry{Targets: map[string]submission.TargetContract{contract.SubmissionTarget: contract}}
	stub := newStubExecutor(clock, []execResult{{
		outcome: GatewayOutcome{Status: "rejected", Reason: "provider_failure"},
	}})
	manager := newManager(t, reg, stub.Exec, clock, db)
	_, cancel, done := startManager(t, manager)
	defer func() {
		cancel()
		<-done
	}()

	_, err := manager.SubmitIntent(context.Background(), Intent{IntentID: "intent-1", SubmissionTarget: contract.SubmissionTarget})
	if err != nil {
		t.Fatalf("submit intent: %v", err)
	}
	waitForCall(t, stub.calls)
	intent := waitForStatus(t, manager, "intent-1", IntentExhausted)
	if intent.ExhaustedReason != "one_shot" {
		t.Fatalf("expected one_shot, got %q", intent.ExhaustedReason)
	}
	select {
	case <-stub.calls:
		t.Fatalf("unexpected retry for one_shot policy")
	default:
	}
}

func TestDuplicateReferenceNonTerminal(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	db := newTestDB(t)
	contract := baseContract(submission.PolicyMaxAttempts)
	reg := submission.Registry{Targets: map[string]submission.TargetContract{contract.SubmissionTarget: contract}}
	stub := newStubExecutor(clock, []execResult{
		{outcome: GatewayOutcome{Status: "rejected", Reason: "duplicate_reference"}},
		{outcome: GatewayOutcome{Status: "rejected", Reason: "duplicate_reference"}},
	})
	manager := newManager(t, reg, stub.Exec, clock, db)
	_, cancel, done := startManager(t, manager)
	defer func() {
		cancel()
		<-done
	}()

	_, err := manager.SubmitIntent(context.Background(), Intent{IntentID: "intent-1", SubmissionTarget: contract.SubmissionTarget})
	if err != nil {
		t.Fatalf("submit intent: %v", err)
	}
	waitForCall(t, stub.calls)
	clock.Advance(5 * time.Second)
	waitForCall(t, stub.calls)
	intent := waitForStatus(t, manager, "intent-1", IntentExhausted)
	if intent.Status == IntentRejected {
		t.Fatalf("expected duplicate_reference to be non-terminal")
	}
}

func TestUnknownGatewayOutcomeStatusExhausted(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	db := newTestDB(t)
	contract := baseContract(submission.PolicyOneShot)
	reg := submission.Registry{Targets: map[string]submission.TargetContract{contract.SubmissionTarget: contract}}
	stub := newStubExecutor(clock, []execResult{{
		outcome: GatewayOutcome{Status: "mystery"},
	}})
	manager := newManager(t, reg, stub.Exec, clock, db)
	_, cancel, done := startManager(t, manager)
	defer func() {
		cancel()
		<-done
	}()

	_, err := manager.SubmitIntent(context.Background(), Intent{IntentID: "intent-1", SubmissionTarget: contract.SubmissionTarget})
	if err != nil {
		t.Fatalf("submit intent: %v", err)
	}
	waitForCall(t, stub.calls)
	intent := waitForStatus(t, manager, "intent-1", IntentExhausted)
	if len(intent.Attempts) != 1 {
		t.Fatalf("expected 1 attempt, got %d", len(intent.Attempts))
	}
	if !strings.Contains(intent.Attempts[0].Error, "unknown gateway outcome status") {
		t.Fatalf("expected unknown gateway outcome status error, got %q", intent.Attempts[0].Error)
	}
}

func TestContractSnapshotStableAfterRegistryChange(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	db := newTestDB(t)
	contract := baseContract(submission.PolicyOneShot)
	reg := submission.Registry{Targets: map[string]submission.TargetContract{contract.SubmissionTarget: contract}}
	stub := newStubExecutor(clock, []execResult{{
		outcome: GatewayOutcome{Status: "accepted"},
	}})
	manager := newManager(t, reg, stub.Exec, clock, db)

	_, err := manager.SubmitIntent(context.Background(), Intent{IntentID: "intent-1", SubmissionTarget: contract.SubmissionTarget})
	if err != nil {
		t.Fatalf("submit intent: %v", err)
	}

	updated := contract
	updated.GatewayURL = "http://new"
	reg.Targets[contract.SubmissionTarget] = updated

	_, cancel, done := startManager(t, manager)
	defer func() {
		cancel()
		<-done
	}()

	input := waitForCall(t, stub.calls)
	if input.GatewayURL != contract.GatewayURL {
		t.Fatalf("expected gateway url %q, got %q", contract.GatewayURL, input.GatewayURL)
	}

	intent := waitForStatus(t, manager, "intent-1", IntentAccepted)
	if intent.Contract.GatewayURL != contract.GatewayURL {
		t.Fatalf("expected contract snapshot url %q, got %q", contract.GatewayURL, intent.Contract.GatewayURL)
	}
}

func TestWaitForIntentMissing(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	db := newTestDB(t)
	contract := baseContract(submission.PolicyOneShot)
	reg := submission.Registry{Targets: map[string]submission.TargetContract{contract.SubmissionTarget: contract}}
	stub := newStubExecutor(clock, nil)
	manager := newManager(t, reg, stub.Exec, clock, db)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)

	intent, ok, err := manager.WaitForIntent(ctx, "missing-intent", time.Second)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if ok {
		t.Fatalf("expected not found, got %+v", intent)
	}
}

func TestWaitForIntentAfterFirstAttempt(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	db := newTestDB(t)
	contract := baseContract(submission.PolicyMaxAttempts)
	contract.MaxAttempts = 3
	reg := submission.Registry{Targets: map[string]submission.TargetContract{contract.SubmissionTarget: contract}}
	stub := newStubExecutor(clock, []execResult{{outcome: GatewayOutcome{Status: "rejected", Reason: "provider_failure"}, advance: 1 * time.Second}})
	manager := newManager(t, reg, stub.Exec, clock, db)

	_, err := manager.SubmitIntent(context.Background(), Intent{
		IntentID:         "intent-1",
		SubmissionTarget: contract.SubmissionTarget,
	})
	if err != nil {
		t.Fatalf("submit intent: %v", err)
	}

	_, cancel, done := startManager(t, manager)
	defer func() {
		cancel()
		<-done
	}()
	waitForCall(t, stub.calls)

	ctx, cancelWait := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancelWait)

	intent, ok, err := manager.WaitForIntent(ctx, "intent-1", 3*time.Second)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if !ok {
		t.Fatal("expected intent found")
	}
	if intent.Status != IntentPending {
		t.Fatalf("expected pending after first attempt, got %q", intent.Status)
	}
	_ = waitForAttempts(t, manager, "intent-1", 1)
}
