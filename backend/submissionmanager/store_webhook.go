package submissionmanager

import (
	"context"
	"database/sql"
	"time"
)

func (s *sqlStore) recordWebhookAttempt(ctx context.Context, intentID string, status string, attemptedAt time.Time, errMsg string) error {
	attemptedAt = attemptedAt.UTC()
	var deliveredAt sql.NullTime
	if status == webhookDelivered {
		deliveredAt = sql.NullTime{Time: attemptedAt, Valid: true}
	}
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE dbo.submission_intents
     SET webhook_status = @p1,
         webhook_attempted_at = @p2,
         webhook_delivered_at = @p3,
         webhook_error = @p4,
         updated_at = @p5
     WHERE intent_id = @p6 AND webhook_status = @p7`,
		status,
		attemptedAt,
		deliveredAt,
		nullString(errMsg),
		attemptedAt,
		intentID,
		webhookPending,
	)
	return err
}
