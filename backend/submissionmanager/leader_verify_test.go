package submissionmanager

import (
	"context"
	"testing"
	"time"

	"gateway/submission"
)

func TestFenceRejectsStaleLeaderWrites(t *testing.T) {
	db := newTestDB(t)
	exec := newBlockingExecutor()
	manager := newLiveManager(t, db, exec.Exec)

	cfg := newLeaseConfig("holder-a", 2*time.Second)
	runner := NewLeaderRunnerFromManager(manager, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runner.Run(ctx)

	waitForLeaderOnly(t, runner)

	intentID := "intent-fence-epoch"
	if _, err := manager.SubmitIntent(context.Background(), Intent{IntentID: intentID, SubmissionTarget: "sms.realtime"}); err != nil {
		t.Fatalf("submit intent: %v", err)
	}

	waitForCall(t, exec.calls)

	if _, err := db.ExecContext(
		context.Background(),
		`UPDATE dbo.submission_manager_leases
     SET holder_id = @p1,
         lease_epoch = lease_epoch + 1,
         renewed_at = SYSUTCDATETIME(),
         expires_at = DATEADD(SECOND, 10, SYSUTCDATETIME())
     WHERE lease_name = @p2`,
		"other-holder",
		cfg.LeaseName,
	); err != nil {
		t.Fatalf("rotate lease: %v", err)
	}

	exec.Unblock()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !runner.IsLeader() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if runner.IsLeader() {
		t.Fatalf("expected leader to drop after fenced write failure")
	}

	deadline = time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		intent, ok := manager.GetIntent(intentID)
		if !ok {
			t.Fatalf("expected intent %q to exist", intentID)
		}
		if len(intent.Attempts) != 0 {
			t.Fatalf("expected no attempts recorded after lease epoch change")
		}
		if intent.Status != IntentPending {
			t.Fatalf("expected status pending, got %q", intent.Status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestScheduleRefreshHandlesEqualLastModifiedAt(t *testing.T) {
	db := newTestDB(t)
	clock := newFakeClock(time.Unix(0, 0))
	contract := baseContract(submission.PolicyOneShot)
	reg := submission.Registry{Targets: map[string]submission.TargetContract{contract.SubmissionTarget: contract}}
	stub := newStubExecutor(clock, nil)
	leader := newManager(t, reg, stub.Exec, clock, db)
	follower := newManager(t, reg, stub.Exec, clock, db)

	intentA := Intent{IntentID: "intent-a", SubmissionTarget: contract.SubmissionTarget}
	if _, err := follower.SubmitIntent(context.Background(), intentA); err != nil {
		t.Fatalf("submit intent A: %v", err)
	}

	fixed := time.Date(2026, 2, 3, 12, 0, 0, 0, time.UTC)
	if _, err := db.ExecContext(
		context.Background(),
		`UPDATE dbo.submission_intents
     SET last_modified_at = @p1
     WHERE intent_id = @p2`,
		fixed,
		intentA.IntentID,
	); err != nil {
		t.Fatalf("force last_modified_at intent A: %v", err)
	}

	activateLeader(t, leader)
	cursor, err := leader.rebuildSchedule(context.Background())
	if err != nil {
		t.Fatalf("rebuild schedule: %v", err)
	}

	intentB := Intent{IntentID: "intent-b", SubmissionTarget: contract.SubmissionTarget}
	if _, err := follower.SubmitIntent(context.Background(), intentB); err != nil {
		t.Fatalf("submit intent B: %v", err)
	}
	if _, err := db.ExecContext(
		context.Background(),
		`UPDATE dbo.submission_intents
     SET last_modified_at = @p1
     WHERE intent_id = @p2`,
		fixed,
		intentB.IntentID,
	); err != nil {
		t.Fatalf("force last_modified_at intent B: %v", err)
	}

	cursor, err = leader.refreshSchedule(context.Background(), cursor)
	if err != nil {
		t.Fatalf("refresh schedule: %v", err)
	}

	leader.mu.Lock()
	_, okA := leader.scheduled[intentA.IntentID]
	_, okB := leader.scheduled[intentB.IntentID]
	leader.mu.Unlock()
	if !okA || !okB {
		t.Fatalf("expected both intents scheduled; intentA=%t intentB=%t", okA, okB)
	}
	_ = cursor
}

func TestScheduleUsesSQLTimeWhenLocalAhead(t *testing.T) {
	db := newTestDB(t)
	exec := newCallRecorder()
	skew := 500 * time.Millisecond
	clock := Clock{
		Now:   func() time.Time { return time.Now().Add(skew) },
		After: time.After,
	}
	contract := baseContract(submission.PolicyOneShot)
	reg := submission.Registry{Targets: map[string]submission.TargetContract{contract.SubmissionTarget: contract}}
	manager, err := NewManager(reg, exec.Exec, clock, db)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	activateLeader(t, manager)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.Run(ctx)

	if _, err := manager.SubmitIntent(context.Background(), Intent{IntentID: "intent-sql-time-ahead", SubmissionTarget: contract.SubmissionTarget}); err != nil {
		t.Fatalf("submit intent: %v", err)
	}

	waitForCall(t, exec.calls)
}

func TestScheduleUsesSQLTimeWhenLocalBehind(t *testing.T) {
	db := newTestDB(t)
	exec := newCallRecorder()
	skew := -2 * time.Second
	clock := Clock{
		Now:   func() time.Time { return time.Now().Add(skew) },
		After: time.After,
	}
	contract := baseContract(submission.PolicyOneShot)
	reg := submission.Registry{Targets: map[string]submission.TargetContract{contract.SubmissionTarget: contract}}
	manager, err := NewManager(reg, exec.Exec, clock, db)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	intentID := "intent-sql-time-behind"
	if _, err := manager.SubmitIntent(context.Background(), Intent{IntentID: intentID, SubmissionTarget: contract.SubmissionTarget}); err != nil {
		t.Fatalf("submit intent: %v", err)
	}
	if _, err := db.ExecContext(
		context.Background(),
		`UPDATE dbo.submission_intents
     SET next_attempt_at = SYSUTCDATETIME(),
         last_modified_at = SYSUTCDATETIME()
     WHERE intent_id = @p1`,
		intentID,
	); err != nil {
		t.Fatalf("force next_attempt_at: %v", err)
	}

	activateLeader(t, manager)
	if _, err := manager.rebuildSchedule(context.Background()); err != nil {
		t.Fatalf("rebuild schedule: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.Run(ctx)

	select {
	case <-exec.calls:
	case <-time.After(750 * time.Millisecond):
		t.Fatalf("executor was not called within expected SQL-time window")
	}
}
