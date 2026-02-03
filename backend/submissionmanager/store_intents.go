package submissionmanager

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gateway/submission"
)

func (s *sqlStore) insertIntent(ctx context.Context, intent Intent, payloadHash []byte, now time.Time) (Intent, bool, error) {
	terminalOutcomes, err := json.Marshal(intent.Contract.TerminalOutcomes)
	if err != nil {
		return Intent{}, false, err
	}
	webhookURL := ""
	webhookSecretEnv := ""
	var webhookHeadersJSON []byte
	var webhookHeadersEnvJSON []byte
	webhookStatus := ""
	if intent.Contract.Webhook != nil {
		webhookURL = intent.Contract.Webhook.URL
		webhookSecretEnv = intent.Contract.Webhook.SecretEnv
		if len(intent.Contract.Webhook.Headers) > 0 {
			webhookHeadersJSON, err = json.Marshal(intent.Contract.Webhook.Headers)
			if err != nil {
				return Intent{}, false, err
			}
		}
		if len(intent.Contract.Webhook.HeadersEnv) > 0 {
			webhookHeadersEnvJSON, err = json.Marshal(intent.Contract.Webhook.HeadersEnv)
			if err != nil {
				return Intent{}, false, err
			}
		}
		webhookStatus = webhookPending
	}
	now = now.UTC()

	_, err = s.db.ExecContext(
		ctx,
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
      next_attempt_at
    ) VALUES (
      @p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10, @p11, @p12, @p13, @p14, @p15, @p16, @p17, @p18, @p19, @p20, @p21, @p22, @p23, @p24, @p25, @p26
    )`,
		intent.IntentID,
		intent.SubmissionTarget,
		intent.Payload,
		payloadHash,
		string(intent.Contract.GatewayType),
		intent.Contract.GatewayURL,
		string(intent.Contract.Policy),
		nullInt(intent.Contract.MaxAcceptanceSeconds),
		nullInt(intent.Contract.MaxAttempts),
		string(terminalOutcomes),
		nullString(webhookURL),
		nullString(string(webhookHeadersJSON)),
		nullString(string(webhookHeadersEnvJSON)),
		nullString(webhookSecretEnv),
		nullString(webhookStatus),
		sql.NullTime{},
		sql.NullTime{},
		nullString(""),
		string(IntentPending),
		nullString(""),
		nullString(""),
		nullString(""),
		0,
		now,
		now,
		now,
	)
	if err == nil {
		intent.Status = IntentPending
		intent.CreatedAt = now
		return intent, true, nil
	}
	if !isUniqueViolation(err) {
		return Intent{}, false, err
	}

	existing, ok, err := s.loadIntent(ctx, intent.IntentID)
	if err != nil {
		return Intent{}, false, err
	}
	if !ok {
		return Intent{}, false, errors.New("intent already exists but could not be loaded")
	}
	if existing.SubmissionTarget == intent.SubmissionTarget && bytes.Equal(existing.Payload, intent.Payload) {
		return existing, false, nil
	}

	return Intent{}, false, IdempotencyConflictError{
		IntentID:        intent.IntentID,
		ExistingTarget:  existing.SubmissionTarget,
		ExistingPayload: string(existing.Payload),
		IncomingTarget:  intent.SubmissionTarget,
		IncomingPayload: string(intent.Payload),
		ExistingStatus:  existing.Status,
	}
}

func (s *sqlStore) loadIntent(ctx context.Context, intentID string) (Intent, bool, error) {
	intent, _, ok, err := s.loadIntentRow(ctx, intentID)
	if err != nil || !ok {
		return Intent{}, false, err
	}

	attempts, err := s.loadAttempts(ctx, intentID)
	if err != nil {
		return Intent{}, false, err
	}
	intent.Attempts = attempts
	return intent, true, nil
}

func (s *sqlStore) loadIntentForExecution(ctx context.Context, intentID string, now time.Time) (Intent, int, bool, error) {
	now = now.UTC()
	row := s.db.QueryRowContext(
		ctx,
		`SELECT intent_id,
      submission_target,
      payload,
      gateway_type,
      gateway_url,
      policy,
      max_acceptance_seconds,
      max_attempts,
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
      next_attempt_at
    FROM dbo.submission_intents
    WHERE intent_id = @p1
      AND status = @p2
      AND next_attempt_at IS NOT NULL
      AND next_attempt_at <= @p3`,
		intentID,
		string(IntentPending),
		now,
	)

	intent, attemptCount, ok, err := s.scanIntentRow(row)
	if err != nil || !ok {
		return Intent{}, 0, ok, err
	}
	return intent, attemptCount, true, nil
}

func (s *sqlStore) loadIntentRow(ctx context.Context, intentID string) (Intent, int, bool, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT intent_id,
      submission_target,
      payload,
      gateway_type,
      gateway_url,
      policy,
      max_acceptance_seconds,
      max_attempts,
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
      next_attempt_at
    FROM dbo.submission_intents
    WHERE intent_id = @p1`,
		intentID,
	)
	return s.scanIntentRow(row)
}

func (s *sqlStore) scanIntentRow(row *sql.Row) (Intent, int, bool, error) {
	var (
		storedIntentID        string
		submissionTarget      string
		payload               []byte
		gatewayType           string
		gatewayURL            string
		policy                string
		maxAcceptanceSeconds  sql.NullInt32
		maxAttempts           sql.NullInt32
		terminalOutcomesJSON  string
		webhookURL            sql.NullString
		webhookHeadersJSON    sql.NullString
		webhookHeadersEnvJSON sql.NullString
		webhookSecretEnv      sql.NullString
		webhookStatus         sql.NullString
		webhookAttemptedAt    sql.NullTime
		webhookDeliveredAt    sql.NullTime
		webhookError          sql.NullString
		status                string
		finalOutcomeStatus    sql.NullString
		finalOutcomeReason    sql.NullString
		exhaustedReason       sql.NullString
		attemptCount          int
		createdAt             time.Time
		updatedAt             time.Time
		nextAttemptAt         sql.NullTime
	)

	if err := row.Scan(
		&storedIntentID,
		&submissionTarget,
		&payload,
		&gatewayType,
		&gatewayURL,
		&policy,
		&maxAcceptanceSeconds,
		&maxAttempts,
		&terminalOutcomesJSON,
		&webhookURL,
		&webhookHeadersJSON,
		&webhookHeadersEnvJSON,
		&webhookSecretEnv,
		&webhookStatus,
		&webhookAttemptedAt,
		&webhookDeliveredAt,
		&webhookError,
		&status,
		&finalOutcomeStatus,
		&finalOutcomeReason,
		&exhaustedReason,
		&attemptCount,
		&createdAt,
		&updatedAt,
		&nextAttemptAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Intent{}, 0, false, nil
		}
		return Intent{}, 0, false, err
	}

	var terminalOutcomes []string
	if err := json.Unmarshal([]byte(terminalOutcomesJSON), &terminalOutcomes); err != nil {
		return Intent{}, 0, false, err
	}
	var webhook *submission.WebhookConfig
	if webhookURL.Valid {
		webhook = &submission.WebhookConfig{
			URL:       webhookURL.String,
			SecretEnv: webhookSecretEnv.String,
		}
		if webhookHeadersJSON.Valid && strings.TrimSpace(webhookHeadersJSON.String) != "" {
			var headers map[string]string
			if err := json.Unmarshal([]byte(webhookHeadersJSON.String), &headers); err != nil {
				return Intent{}, 0, false, err
			}
			webhook.Headers = headers
		}
		if webhookHeadersEnvJSON.Valid && strings.TrimSpace(webhookHeadersEnvJSON.String) != "" {
			var headersEnv map[string]string
			if err := json.Unmarshal([]byte(webhookHeadersEnvJSON.String), &headersEnv); err != nil {
				return Intent{}, 0, false, err
			}
			webhook.HeadersEnv = headersEnv
		}
	}

	intent := Intent{
		IntentID:         storedIntentID,
		SubmissionTarget: submissionTarget,
		Payload:          payload,
		CreatedAt:        normalizeDBTime(createdAt),
		Status:           IntentStatus(status),
		Contract: submission.TargetContract{
			SubmissionTarget:     submissionTarget,
			GatewayType:          submission.GatewayType(strings.TrimSpace(gatewayType)),
			GatewayURL:           gatewayURL,
			Policy:               submission.ContractPolicy(strings.TrimSpace(policy)),
			MaxAcceptanceSeconds: int(maxAcceptanceSeconds.Int32),
			MaxAttempts:          int(maxAttempts.Int32),
			TerminalOutcomes:     terminalOutcomes,
			Webhook:              webhook,
		},
		FinalOutcome: GatewayOutcome{
			Status: finalOutcomeStatus.String,
			Reason: finalOutcomeReason.String,
		},
		ExhaustedReason: exhaustedReason.String,
		WebhookStatus:   webhookStatus.String,
		WebhookError:    webhookError.String,
	}
	if webhookAttemptedAt.Valid {
		intent.WebhookAttemptedAt = normalizeDBTime(webhookAttemptedAt.Time)
	}
	if webhookDeliveredAt.Valid {
		intent.WebhookDeliveredAt = normalizeDBTime(webhookDeliveredAt.Time)
	}

	if intent.Status != IntentPending {
		intent.CompletedAt = normalizeDBTime(updatedAt)
	}
	if nextAttemptAt.Valid {
		_ = normalizeDBTime(nextAttemptAt.Time)
	}
	return intent, attemptCount, true, nil
}

func (s *sqlStore) markExhausted(ctx context.Context, intentID string, exhaustedReason string, now time.Time) (bool, error) {
	now = now.UTC()
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE dbo.submission_intents
     SET status = @p1,
         final_outcome_status = NULL,
         final_outcome_reason = NULL,
         exhausted_reason = @p2,
         next_attempt_at = NULL,
         updated_at = @p3
     WHERE intent_id = @p4 AND status = @p5`,
		string(IntentExhausted),
		nullString(exhaustedReason),
		now,
		intentID,
		string(IntentPending),
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
