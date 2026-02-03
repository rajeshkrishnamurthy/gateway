package submissionmanager

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *sqlStore) loadAttempts(ctx context.Context, intentID string) ([]Attempt, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT attempt_number, started_at, finished_at, outcome_status, outcome_reason, error
     FROM dbo.submission_attempts
     WHERE intent_id = @p1
     ORDER BY attempt_number`,
		intentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	attempts := []Attempt{}
	for rows.Next() {
		var (
			number     int
			startedAt  time.Time
			finishedAt time.Time
			status     sql.NullString
			reason     sql.NullString
			attemptErr sql.NullString
		)
		if err := rows.Scan(&number, &startedAt, &finishedAt, &status, &reason, &attemptErr); err != nil {
			return nil, err
		}
		attempts = append(attempts, Attempt{
			Number:     number,
			StartedAt:  normalizeDBTime(startedAt),
			FinishedAt: normalizeDBTime(finishedAt),
			GatewayOutcome: GatewayOutcome{
				Status: status.String,
				Reason: reason.String,
			},
			Error: attemptErr.String,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return attempts, nil
}

func (s *sqlStore) recordAttempt(ctx context.Context, fence LeaseFence, intentID string, attempt Attempt, status IntentStatus, finalOutcome GatewayOutcome, exhaustedReason string, nextAttemptAt *time.Time, now time.Time) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var currentStatus string
	var attemptCount int
	row := tx.QueryRowContext(ctx, `SELECT status, attempt_count FROM dbo.submission_intents WHERE intent_id = @p1`, intentID)
	if err = row.Scan(&currentStatus, &attemptCount); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, errors.New("intent not found")
		}
		return false, err
	}

	// Non-obvious constraint: attempt_count is the authoritative attempt number source.
	if currentStatus != string(IntentPending) {
		return false, nil
	}

	attemptNumber := attemptCount + 1
	startedAt := attempt.StartedAt.UTC()
	finishedAt := attempt.FinishedAt.UTC()
	result, err := tx.ExecContext(
		ctx,
		`INSERT INTO dbo.submission_attempts (
      intent_id, attempt_number, started_at, finished_at, outcome_status, outcome_reason, error
    )
    SELECT @p1, @p2, @p3, @p4, @p5, @p6, @p7
    WHERE EXISTS (
      SELECT 1
      FROM dbo.submission_manager_leases
      WHERE lease_name = @p8
        AND holder_id = @p9
        AND lease_epoch = @p10
        AND expires_at > SYSUTCDATETIME()
    )`,
		intentID,
		attemptNumber,
		startedAt,
		finishedAt,
		nullString(attempt.GatewayOutcome.Status),
		nullString(attempt.GatewayOutcome.Reason),
		nullString(attempt.Error),
		fence.LeaseName,
		fence.HolderID,
		fence.LeaseEpoch,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, nil
	}

	var nextAttemptValue sql.NullTime
	if nextAttemptAt != nil {
		nextAttemptValue = sql.NullTime{Time: nextAttemptAt.UTC(), Valid: true}
	}

	finalStatus := sql.NullString{}
	finalReason := sql.NullString{}
	exhausted := sql.NullString{}
	if status == IntentAccepted || status == IntentRejected {
		finalStatus = nullString(finalOutcome.Status)
		finalReason = nullString(finalOutcome.Reason)
	} else if status == IntentExhausted {
		exhausted = nullString(exhaustedReason)
	}

	now = now.UTC()
	result, err = tx.ExecContext(
		ctx,
		`UPDATE dbo.submission_intents
     SET attempt_count = @p1,
         status = @p2,
         final_outcome_status = @p3,
         final_outcome_reason = @p4,
         exhausted_reason = @p5,
         next_attempt_at = @p6,
         updated_at = @p7,
         last_modified_at = SYSUTCDATETIME()
     WHERE intent_id = @p8
       AND EXISTS (
         SELECT 1
         FROM dbo.submission_manager_leases
         WHERE lease_name = @p9
           AND holder_id = @p10
           AND lease_epoch = @p11
           AND expires_at > SYSUTCDATETIME()
       )`,
		attemptNumber,
		string(status),
		finalStatus,
		finalReason,
		exhausted,
		nextAttemptValue,
		now,
		intentID,
		fence.LeaseName,
		fence.HolderID,
		fence.LeaseEpoch,
	)
	if err != nil {
		return false, err
	}
	affected, err = result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, nil
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
