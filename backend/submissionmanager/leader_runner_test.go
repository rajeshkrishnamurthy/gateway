package submissionmanager

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"gateway/submission"
)

type callRecorder struct {
	calls chan AttemptInput
}

func newCallRecorder() *callRecorder {
	return &callRecorder{calls: make(chan AttemptInput, 10)}
}

func (c *callRecorder) Exec(ctx context.Context, input AttemptInput) (GatewayOutcome, error) {
	_ = ctx
	c.calls <- input
	return GatewayOutcome{Status: gatewayAccepted}, nil
}

type blockingExecutor struct {
	calls  chan AttemptInput
	block  chan struct{}
	closed atomic.Bool
}

func newBlockingExecutor() *blockingExecutor {
	return &blockingExecutor{
		calls: make(chan AttemptInput, 10),
		block: make(chan struct{}),
	}
}

func (b *blockingExecutor) Exec(ctx context.Context, input AttemptInput) (GatewayOutcome, error) {
	_ = ctx
	b.calls <- input
	<-b.block
	return GatewayOutcome{Status: gatewayAccepted}, nil
}

func (b *blockingExecutor) Unblock() {
	if b.closed.CompareAndSwap(false, true) {
		close(b.block)
	}
}

func newLiveManager(t *testing.T, db *sql.DB, exec AttemptExecutor) *Manager {
	t.Helper()
	contract := baseContract(submission.PolicyOneShot)
	reg := submission.Registry{Targets: map[string]submission.TargetContract{contract.SubmissionTarget: contract}}
	manager, err := NewManager(reg, exec, Clock{}, db)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	return manager
}

func newLeaseConfig(holderID string, duration time.Duration) LeaseConfig {
	return LeaseConfig{
		LeaseName:               "submission-manager-executor",
		HolderID:                holderID,
		LeaseDuration:           duration,
		RenewInterval:           duration / 3,
		AcquireInterval:         duration / 3,
		ScheduleRefreshInterval: 50 * time.Millisecond,
	}
}

func waitForLeader(t *testing.T, runnerA, runnerB *LeaderRunner) (*LeaderRunner, *LeaderRunner) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		aLeader := runnerA.IsLeader()
		bLeader := runnerB.IsLeader()
		if aLeader && !bLeader {
			return runnerA, runnerB
		}
		if bLeader && !aLeader {
			return runnerB, runnerA
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected a single leader to be elected")
	return nil, nil
}

func waitForLeaderOnly(t *testing.T, runner *LeaderRunner) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runner.IsLeader() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected runner to become leader")
}

func assertNoCallFor(t *testing.T, calls <-chan AttemptInput, wait time.Duration) {
	t.Helper()
	select {
	case <-calls:
		t.Fatalf("unexpected executor call")
	case <-time.After(wait):
	}
}

func TestLeaderSingleOnConcurrentStart(t *testing.T) {
	db := newTestDB(t)
	execA := newCallRecorder()
	execB := newCallRecorder()

	managerA := newLiveManager(t, db, execA.Exec)
	managerB := newLiveManager(t, db, execB.Exec)

	cfgA := newLeaseConfig("holder-a", 800*time.Millisecond)
	cfgB := newLeaseConfig("holder-b", 800*time.Millisecond)

	runnerA := NewLeaderRunnerFromManager(managerA, cfgA)
	runnerB := NewLeaderRunnerFromManager(managerB, cfgB)

	ctxA, cancelA := context.WithCancel(context.Background())
	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelA()
	defer cancelB()

	go runnerA.Run(ctxA)
	go runnerB.Run(ctxB)

	leaderRunner, followerRunner := waitForLeader(t, runnerA, runnerB)

	intent := Intent{IntentID: "intent-leader-1", SubmissionTarget: "sms.realtime"}
	if _, err := leaderRunner.manager.SubmitIntent(context.Background(), intent); err != nil {
		t.Fatalf("submit intent: %v", err)
	}

	if leaderRunner == runnerA {
		waitForCall(t, execA.calls)
		assertNoCall(t, execB.calls)
	} else {
		waitForCall(t, execB.calls)
		assertNoCall(t, execA.calls)
	}

	if followerRunner.IsLeader() {
		t.Fatalf("expected follower to remain non-leader")
	}
}

func TestLeaderFailoverAfterExpiry(t *testing.T) {
	db := newTestDB(t)
	execA := newCallRecorder()
	execB := newCallRecorder()

	managerA := newLiveManager(t, db, execA.Exec)
	managerB := newLiveManager(t, db, execB.Exec)

	cfgA := newLeaseConfig("holder-a", 600*time.Millisecond)
	cfgB := newLeaseConfig("holder-b", 600*time.Millisecond)

	runnerA := NewLeaderRunnerFromManager(managerA, cfgA)
	runnerB := NewLeaderRunnerFromManager(managerB, cfgB)

	ctxA, cancelA := context.WithCancel(context.Background())
	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelA()
	defer cancelB()

	go runnerA.Run(ctxA)
	go runnerB.Run(ctxB)

	leaderRunner, followerRunner := waitForLeader(t, runnerA, runnerB)
	if leaderRunner == runnerA {
		cancelA()
	} else {
		cancelB()
	}

	waitForLeaderOnly(t, followerRunner)
}

func TestNoExecutionWithoutLeadership(t *testing.T) {
	db := newTestDB(t)
	exec := newCallRecorder()
	manager := newLiveManager(t, db, exec.Exec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.Run(ctx)

	_, err := manager.SubmitIntent(context.Background(), Intent{IntentID: "intent-follower-1", SubmissionTarget: "sms.realtime"})
	if err != nil {
		t.Fatalf("submit intent: %v", err)
	}
	assertNoCall(t, exec.calls)
}

func TestLeaderStopsOnLeaseLoss(t *testing.T) {
	db := newTestDB(t)
	exec := newBlockingExecutor()
	manager := newLiveManager(t, db, exec.Exec)
	cfg := newLeaseConfig("holder-a", 800*time.Millisecond)
	runner := NewLeaderRunnerFromManager(manager, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runner.Run(ctx)

	waitForLeaderOnly(t, runner)

	if _, err := manager.SubmitIntent(context.Background(), Intent{IntentID: "intent-1", SubmissionTarget: "sms.realtime"}); err != nil {
		t.Fatalf("submit intent: %v", err)
	}
	if _, err := manager.SubmitIntent(context.Background(), Intent{IntentID: "intent-2", SubmissionTarget: "sms.realtime"}); err != nil {
		t.Fatalf("submit intent: %v", err)
	}

	waitForCall(t, exec.calls)

	_, err := db.ExecContext(
		context.Background(),
		`UPDATE dbo.submission_manager_leases
     SET expires_at = DATEADD(SECOND, -1, SYSUTCDATETIME())
     WHERE lease_name = @p1`,
		cfg.LeaseName,
	)
	if err != nil {
		t.Fatalf("expire lease: %v", err)
	}

	exec.Unblock()
	assertNoCallFor(t, exec.calls, 500*time.Millisecond)
}

func TestLeaderRefreshesFollowerIntents(t *testing.T) {
	db := newTestDB(t)
	execA := newCallRecorder()
	execB := newCallRecorder()

	managerA := newLiveManager(t, db, execA.Exec)
	managerB := newLiveManager(t, db, execB.Exec)

	cfgA := newLeaseConfig("holder-a", 800*time.Millisecond)
	cfgB := newLeaseConfig("holder-b", 800*time.Millisecond)

	runnerA := NewLeaderRunnerFromManager(managerA, cfgA)
	runnerB := NewLeaderRunnerFromManager(managerB, cfgB)

	ctxA, cancelA := context.WithCancel(context.Background())
	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelA()
	defer cancelB()

	go runnerA.Run(ctxA)
	go runnerB.Run(ctxB)

	leaderRunner, followerRunner := waitForLeader(t, runnerA, runnerB)
	var leaderExec, followerExec *callRecorder
	if leaderRunner == runnerA {
		leaderExec = execA
		followerExec = execB
	} else {
		leaderExec = execB
		followerExec = execA
	}

	intentID := fmt.Sprintf("intent-refresh-%d", time.Now().UnixNano())
	if _, err := followerRunner.manager.SubmitIntent(context.Background(), Intent{IntentID: intentID, SubmissionTarget: "sms.realtime"}); err != nil {
		t.Fatalf("submit intent: %v", err)
	}

	waitForCall(t, leaderExec.calls)
	assertNoCall(t, followerExec.calls)
}
