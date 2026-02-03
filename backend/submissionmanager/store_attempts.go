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

func (s *sqlStore) recordAttempt(ctx context.Context, intentID string, attempt Attempt, status IntentStatus, finalOutcome GatewayOutcome, exhaustedReason string, nextAttemptAt *time.Time, now time.Time) (bool, error) {
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
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO dbo.submission_attempts (
      intent_id, attempt_number, started_at, finished_at, outcome_status, outcome_reason, error
    ) VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7)`,
		intentID,
		attemptNumber,
		startedAt,
		finishedAt,
		nullString(attempt.GatewayOutcome.Status),
		nullString(attempt.GatewayOutcome.Reason),
		nullString(attempt.Error),
	)
	if err != nil {
		return false, err
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
	_, err = tx.ExecContext(
		ctx,
		`UPDATE dbo.submission_intents
     SET attempt_count = @p1,
         status = @p2,
         final_outcome_status = @p3,
         final_outcome_reason = @p4,
         exhausted_reason = @p5,
         next_attempt_at = @p6,
         updated_at = @p7
     WHERE intent_id = @p8`,
		attemptNumber,
		string(status),
		finalStatus,
		finalReason,
		exhausted,
		nextAttemptValue,
		now,
		intentID,
	)
	if err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
