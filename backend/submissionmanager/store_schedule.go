package submissionmanager

import (
	"context"
	"database/sql"
	"time"
)

type scheduleSnapshotRow struct {
	intentID     string
	due          time.Time
	lastModified time.Time
}

type scheduleChangeRow struct {
	intentID     string
	status       IntentStatus
	due          *time.Time
	lastModified time.Time
}

func (s *sqlStore) loadScheduleSnapshot(ctx context.Context) ([]scheduleSnapshotRow, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT intent_id, next_attempt_at, last_modified_at
     FROM dbo.submission_intents
     WHERE status = @p1 AND next_attempt_at IS NOT NULL
     ORDER BY last_modified_at, intent_id`,
		string(IntentPending),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scheduled []scheduleSnapshotRow
	for rows.Next() {
		var intentID string
		var due time.Time
		var lastModified time.Time
		if err := rows.Scan(&intentID, &due, &lastModified); err != nil {
			return nil, err
		}
		scheduled = append(scheduled, scheduleSnapshotRow{
			intentID:     intentID,
			due:          normalizeDBTime(due),
			lastModified: normalizeDBTime(lastModified),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return scheduled, nil
}

func (s *sqlStore) loadScheduleChanges(ctx context.Context, cursor scheduleCursor) ([]scheduleChangeRow, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT intent_id, status, next_attempt_at, last_modified_at
     FROM dbo.submission_intents
     WHERE (last_modified_at > @p1) OR (last_modified_at = @p1 AND intent_id > @p2)
     ORDER BY last_modified_at, intent_id`,
		cursor.lastModified,
		cursor.intentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var changes []scheduleChangeRow
	for rows.Next() {
		var intentID string
		var status string
		var due sql.NullTime
		var lastModified time.Time
		if err := rows.Scan(&intentID, &status, &due, &lastModified); err != nil {
			return nil, err
		}
		var nextAttempt *time.Time
		if due.Valid {
			value := normalizeDBTime(due.Time)
			nextAttempt = &value
		}
		changes = append(changes, scheduleChangeRow{
			intentID:     intentID,
			status:       IntentStatus(status),
			due:          nextAttempt,
			lastModified: normalizeDBTime(lastModified),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return changes, nil
}
