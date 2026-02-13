package submissionmanager

import (
	"context"
	"fmt"
	setulog "gateway/logging"
	"log/slog"
	"strings"
	"time"

	"gateway/submission"
)

const retryDelay = 5 * time.Second

const (
	gatewayAccepted = "accepted"
	gatewayRejected = "rejected"
)

func (m *Manager) executeAttempt(ctx context.Context, intentID string, due time.Time) {
	// Flow intent: load intent, call gateway, apply policy, save result.
	logger := slog.Default().With(
		"component", "submission-manager-attempts",
		"commProfile", setulog.CommProfileWithinSetu,
		"operation", "execute_attempt",
		"intentId", intentID,
	)
	fence, ok := m.currentFence()
	if !ok {
		return
	}
	start := m.clock.Now()

	intent, attemptCount, ok, err := m.store.loadIntentForExecution(ctx, intentID)
	if err != nil {
		if ctx == nil || ctx.Err() == nil {
			m.notifyLeaseLoss()
		}
		return
	}
	if !ok {
		return
	}
	if m.metrics != nil {
		m.metrics.ObserveQueueDelay(start.Sub(due))
	}
	logger.Info(
		"submission attempt start",
		"event", "submission.attempt.start",
		"stage", "attempt_start",
		"outcome", "started",
		"attempt", attemptCount+1,
		"gatewayType", intent.Contract.GatewayType,
	)
	if intent.Contract.Policy == submission.PolicyDeadline {
		deadline := intent.CreatedAt.Add(time.Duration(intent.Contract.MaxAcceptanceSeconds) * time.Second)
		// Policy vs outcome: do not execute attempts after the acceptance deadline.
		if !start.Before(deadline) {
			applied, err := m.store.markExhausted(ctx, fence, intentID, "deadline_exceeded", start)
			if err != nil || !applied {
				if ctx == nil || ctx.Err() == nil {
					m.notifyLeaseLoss()
				}
				return
			}
			if m.metrics != nil {
				m.metrics.ObserveIntentTerminal(IntentExhausted, start.Sub(intent.CreatedAt))
				m.metrics.ObserveExhausted("deadline_exceeded")
			}
			logger.Info(
				"submission attempt exhausted before execution",
				"event", "submission.attempt.policy_exhausted",
				"stage", "policy_gate",
				"outcome", string(IntentExhausted),
				"status", IntentExhausted,
				"exhaustedReason", "deadline_exceeded",
			)
			return
		}
	}

	attemptNumber := attemptCount + 1
	contract := intent.Contract
	payload := clonePayload(intent.Payload)

	if m.metrics != nil {
		m.metrics.IncInflight()
		defer m.metrics.DecInflight()
	}

	if !m.isLeader() {
		return
	}

	outcome, err := m.exec(ctx, AttemptInput{
		IntentID:    intentID,
		GatewayType: contract.GatewayType,
		GatewayURL:  contract.GatewayURL,
		Payload:     payload,
	})
	finish := m.clock.Now()

	attempt := Attempt{
		Number:     attemptNumber,
		StartedAt:  start,
		FinishedAt: finish,
	}
	if err != nil {
		attempt.Error = err.Error()
	} else {
		attempt.GatewayOutcome = GatewayOutcome{
			Status: strings.TrimSpace(outcome.Status),
			Reason: strings.TrimSpace(outcome.Reason),
		}
	}

	retry, due := m.evaluateAttempt(&intent, &attempt)
	nextDue := ""
	if retry {
		nextDue = due.UTC().Format(time.RFC3339Nano)
	}
	logger.Info(
		"submission attempt result",
		"event", "submission.attempt.result",
		"stage", "attempt_result",
		"outcome", string(intent.Status),
		"attempt", attempt.Number,
		"outcomeStatus", attempt.GatewayOutcome.Status,
		"outcomeReason", attempt.GatewayOutcome.Reason,
		"error", attempt.Error,
		"status", intent.Status,
		"retry", retry,
		"nextDue", nextDue,
	)
	var nextAttemptAt *time.Time
	if retry {
		nextAttemptAt = &due
	}
	applied, err := m.store.recordAttempt(ctx, fence, intentID, attempt, intent.Status, intent.FinalOutcome, intent.ExhaustedReason, nextAttemptAt, finish)
	if err != nil || !applied {
		if ctx == nil || ctx.Err() == nil {
			m.notifyLeaseLoss()
		}
		return
	}
	if m.metrics != nil {
		m.metrics.ObserveAttemptDuration(finish.Sub(start))
		if attempt.Error != "" {
			m.metrics.ObserveAttemptOutcome("error")
		} else if attempt.GatewayOutcome.Status != "" {
			m.metrics.ObserveAttemptOutcome(attempt.GatewayOutcome.Status)
		} else {
			m.metrics.ObserveAttemptOutcome("error")
		}
		if retry {
			m.metrics.ObserveRetryScheduled()
		}
		if intent.Status == IntentAccepted || intent.Status == IntentRejected || intent.Status == IntentExhausted {
			m.metrics.ObserveIntentTerminal(intent.Status, finish.Sub(intent.CreatedAt))
			if intent.Status == IntentExhausted {
				m.metrics.ObserveExhausted(intent.ExhaustedReason)
			}
		}
	}
	if intent.Status == IntentAccepted || intent.Status == IntentRejected || intent.Status == IntentExhausted {
		intent.CompletedAt = finish
		m.dispatchWebhook(ctx, intent, finish)
	}
	if retry {
		m.enqueueAttempt(intentID, due)
	}
}

func (m *Manager) evaluateAttempt(intent *Intent, attempt *Attempt) (bool, time.Time) {
	// Flow intent: read outcome and decide terminal or retry.
	if attempt.Error != "" {
		return m.applyPolicy(intent, attempt)
	}

	switch attempt.GatewayOutcome.Status {
	case gatewayAccepted:
		if intent.Contract.Policy == submission.PolicyDeadline {
			deadline := intent.CreatedAt.Add(time.Duration(intent.Contract.MaxAcceptanceSeconds) * time.Second)
			if !attempt.FinishedAt.Before(deadline) {
				intent.Status = IntentExhausted
				intent.ExhaustedReason = "deadline_exceeded"
				return false, time.Time{}
			}
		}
		intent.Status = IntentAccepted
		intent.FinalOutcome = attempt.GatewayOutcome
		return false, time.Time{}
	case gatewayRejected:
		reason := attempt.GatewayOutcome.Reason
		if reason == "" {
			attempt.Error = "gateway outcome rejection reason is required"
			return m.applyPolicy(intent, attempt)
		}
		if isTerminalOutcome(intent.Contract.TerminalOutcomes, reason) {
			intent.Status = IntentRejected
			intent.FinalOutcome = attempt.GatewayOutcome
			return false, time.Time{}
		}
		return m.applyPolicy(intent, attempt)
	case "":
		attempt.Error = "gateway outcome status is required"
		return m.applyPolicy(intent, attempt)
	default:
		attempt.Error = fmt.Sprintf("unknown gateway outcome status %q", attempt.GatewayOutcome.Status)
		return m.applyPolicy(intent, attempt)
	}
}

func (m *Manager) applyPolicy(intent *Intent, attempt *Attempt) (bool, time.Time) {
	// Flow intent: apply policy to decide retry or exhausted.
	switch intent.Contract.Policy {
	case submission.PolicyOneShot:
		intent.Status = IntentExhausted
		intent.ExhaustedReason = "one_shot"
		return false, time.Time{}
	case submission.PolicyMaxAttempts:
		if attempt.Number >= intent.Contract.MaxAttempts {
			intent.Status = IntentExhausted
			intent.ExhaustedReason = "max_attempts"
			return false, time.Time{}
		}
		return true, attempt.FinishedAt.Add(retryDelay)
	case submission.PolicyDeadline:
		deadline := intent.CreatedAt.Add(time.Duration(intent.Contract.MaxAcceptanceSeconds) * time.Second)
		if !attempt.FinishedAt.Before(deadline) {
			intent.Status = IntentExhausted
			intent.ExhaustedReason = "deadline_exceeded"
			return false, time.Time{}
		}
		nextDue := attempt.FinishedAt.Add(retryDelay)
		if !nextDue.Before(deadline) {
			intent.Status = IntentExhausted
			intent.ExhaustedReason = "deadline_exceeded"
			return false, time.Time{}
		}
		return true, nextDue
	default:
		intent.Status = IntentExhausted
		intent.ExhaustedReason = "unknown_policy"
		return false, time.Time{}
	}
}

func isTerminalOutcome(outcomes []string, reason string) bool {
	for _, outcome := range outcomes {
		if outcome == reason {
			return true
		}
	}
	return false
}
