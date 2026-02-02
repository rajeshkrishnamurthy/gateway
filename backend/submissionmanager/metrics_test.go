package submissionmanager

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestMetricsWritePrometheus(t *testing.T) {
	metrics := NewMetrics()
	metrics.ObserveIntentCreated()
	metrics.ObserveIdempotentHit()
	metrics.ObserveIdempotencyConflict()
	metrics.ObserveAttemptOutcome(gatewayAccepted)
	metrics.ObserveAttemptOutcome(gatewayRejected)
	metrics.ObserveAttemptOutcome("error")
	metrics.ObserveRetryScheduled()
	metrics.ObserveIntentTerminal(IntentAccepted, 2*time.Second)
	metrics.ObserveIntentTerminal(IntentRejected, 3*time.Second)
	metrics.ObserveIntentTerminal(IntentExhausted, 4*time.Second)
	metrics.ObserveExhausted("deadline_exceeded")
	metrics.ObserveExhausted("max_attempts")
	metrics.ObserveExhausted("one_shot")
	metrics.ObserveExhausted("unknown_policy")
	metrics.ObserveExhausted("other")
	metrics.ObserveAttemptDuration(500 * time.Millisecond)
	metrics.ObserveQueueDelay(10 * time.Millisecond)
	metrics.SetQueueDepth(3)
	metrics.IncInflight()
	metrics.DecInflight()

	var buf bytes.Buffer
	metrics.WritePrometheus(&buf)
	output := buf.String()

	expectContains := []string{
		"submission_intents_created_total 1",
		"submission_idempotent_hits_total 1",
		"submission_idempotency_conflicts_total 1",
		`submission_intents_terminal_total{status="accepted"} 1`,
		`submission_intents_terminal_total{status="rejected"} 1`,
		`submission_intents_terminal_total{status="exhausted"} 1`,
		`submission_exhausted_total{reason="unknown_reason"} 1`,
		`submission_attempts_total{outcome_status="accepted"} 1`,
		`submission_attempts_total{outcome_status="rejected"} 1`,
		`submission_attempts_total{outcome_status="error"} 1`,
		"submission_retries_scheduled_total 1",
		"submission_queue_depth 3",
		"submission_inflight_attempts 0",
		"submission_attempt_duration_seconds_bucket",
		"submission_intent_time_to_terminal_seconds_bucket",
		"submission_queue_delay_seconds_bucket",
	}
	for _, needle := range expectContains {
		if !strings.Contains(output, needle) {
			t.Fatalf("expected output to contain %q", needle)
		}
	}
}
