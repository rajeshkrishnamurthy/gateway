package submissionmanager

import (
	"context"
	"database/sql"
	"time"
)

func (s *sqlStore) recordWebhookAttempt(ctx context.Context, fence LeaseFence, intentID string, status string, attemptedAt time.Time, errMsg string) (bool, error) {
	attemptedAt = attemptedAt.UTC()
	var deliveredAt sql.NullTime
	if status == webhookDelivered {
		deliveredAt = sql.NullTime{Time: attemptedAt, Valid: true}
	}
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE dbo.submission_intents
     SET webhook_status = @p1,
         webhook_attempted_at = @p2,
         webhook_delivered_at = @p3,
         webhook_error = @p4,
         updated_at = @p5,
         last_modified_at = SYSUTCDATETIME()
     WHERE intent_id = @p6 AND webhook_status = @p7
       AND EXISTS (
         SELECT 1
         FROM dbo.submission_manager_leases
         WHERE lease_name = @p8
           AND holder_id = @p9
           AND lease_epoch = @p10
           AND expires_at > SYSUTCDATETIME()
       )`,
		status,
		attemptedAt,
		deliveredAt,
		nullString(errMsg),
		attemptedAt,
		intentID,
		webhookPending,
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
	return affected > 0, nil
}
