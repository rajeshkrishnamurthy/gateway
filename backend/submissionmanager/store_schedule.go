package submissionmanager

import (
	"context"
	"time"
)

func (s *sqlStore) loadPendingSchedule(ctx context.Context) ([]scheduledAttempt, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT intent_id, next_attempt_at
     FROM dbo.submission_intents
     WHERE status = @p1 AND next_attempt_at IS NOT NULL
     ORDER BY next_attempt_at, intent_id`,
		string(IntentPending),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scheduled []scheduledAttempt
	for rows.Next() {
		var intentID string
		var due time.Time
		if err := rows.Scan(&intentID, &due); err != nil {
			return nil, err
		}
		scheduled = append(scheduled, scheduledAttempt{
			intentID: intentID,
			due:      normalizeDBTime(due),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return scheduled, nil
}
