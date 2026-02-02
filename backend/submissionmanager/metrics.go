package submissionmanager

import (
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"
)

// Metrics tracks SubmissionManager metrics for Prometheus.
type Metrics struct {
	mu sync.Mutex

	intentsCreated       uint64
	idempotentHits       uint64
	idempotencyConflicts uint64

	terminalAccepted  uint64
	terminalRejected  uint64
	terminalExhausted uint64

	exhaustedDeadline uint64
	exhaustedMax      uint64
	exhaustedOneShot  uint64
	exhaustedUnknown  uint64
	exhaustedOther    uint64

	attemptsAccepted uint64
	attemptsRejected uint64
	attemptsError    uint64

	retriesScheduled uint64

	queueDepth int
	inflight   int

	intentAcceptedDuration  histogram
	intentRejectedDuration  histogram
	intentExhaustedDuration histogram
	attemptDuration         histogram
	queueDelay              histogram
}

type histogram struct {
	buckets []float64
	counts  []uint64
	count   uint64
	sum     float64
}

// NewMetrics constructs a Metrics registry with default histogram buckets.
func NewMetrics() *Metrics {
	return &Metrics{
		intentAcceptedDuration:  newHistogram(durationBucketsIntentTerminal),
		intentRejectedDuration:  newHistogram(durationBucketsIntentTerminal),
		intentExhaustedDuration: newHistogram(durationBucketsIntentTerminal),
		attemptDuration:         newHistogram(durationBucketsAttempt),
		queueDelay:              newHistogram(durationBucketsQueueDelay),
	}
}

var durationBucketsAttempt = []float64{
	0.1,
	0.25,
	0.5,
	1,
	2.5,
	5,
}

var durationBucketsIntentTerminal = []float64{
	0.5,
	1,
	2,
	5,
	10,
	30,
	60,
	120,
}

var durationBucketsQueueDelay = []float64{
	0.01,
	0.05,
	0.1,
	0.25,
	0.5,
	1,
	2,
	5,
}

// ObserveIntentCreated records a new intent creation.
func (m *Metrics) ObserveIntentCreated() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.intentsCreated++
	m.mu.Unlock()
}

// ObserveIdempotentHit records an idempotent re-submission.
func (m *Metrics) ObserveIdempotentHit() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.idempotentHits++
	m.mu.Unlock()
}

// ObserveIdempotencyConflict records an idempotency conflict.
func (m *Metrics) ObserveIdempotencyConflict() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.idempotencyConflicts++
	m.mu.Unlock()
}

// ObserveAttemptOutcome records an attempt outcome status.
func (m *Metrics) ObserveAttemptOutcome(status string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	switch status {
	case gatewayAccepted:
		m.attemptsAccepted++
	case gatewayRejected:
		m.attemptsRejected++
	default:
		m.attemptsError++
	}
	m.mu.Unlock()
}

// ObserveRetryScheduled records a scheduled retry.
func (m *Metrics) ObserveRetryScheduled() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.retriesScheduled++
	m.mu.Unlock()
}

// ObserveIntentTerminal records a terminal intent and its duration.
func (m *Metrics) ObserveIntentTerminal(status IntentStatus, duration time.Duration) {
	if m == nil {
		return
	}
	seconds := duration.Seconds()
	if seconds < 0 {
		seconds = 0
	}
	m.mu.Lock()
	switch status {
	case IntentAccepted:
		m.terminalAccepted++
		m.intentAcceptedDuration.observe(seconds)
	case IntentRejected:
		m.terminalRejected++
		m.intentRejectedDuration.observe(seconds)
	case IntentExhausted:
		m.terminalExhausted++
		m.intentExhaustedDuration.observe(seconds)
	}
	m.mu.Unlock()
}

// ObserveExhausted records an exhaustion reason.
func (m *Metrics) ObserveExhausted(reason string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	switch reason {
	case "deadline_exceeded":
		m.exhaustedDeadline++
	case "max_attempts":
		m.exhaustedMax++
	case "one_shot":
		m.exhaustedOneShot++
	case "unknown_policy":
		m.exhaustedUnknown++
	default:
		m.exhaustedOther++
	}
	m.mu.Unlock()
}

// ObserveAttemptDuration records an attempt execution duration.
func (m *Metrics) ObserveAttemptDuration(duration time.Duration) {
	if m == nil {
		return
	}
	seconds := duration.Seconds()
	if seconds < 0 {
		seconds = 0
	}
	m.mu.Lock()
	m.attemptDuration.observe(seconds)
	m.mu.Unlock()
}

// ObserveQueueDelay records scheduling lag for an attempt.
func (m *Metrics) ObserveQueueDelay(duration time.Duration) {
	if m == nil {
		return
	}
	seconds := duration.Seconds()
	if seconds < 0 {
		seconds = 0
	}
	m.mu.Lock()
	m.queueDelay.observe(seconds)
	m.mu.Unlock()
}

// SetQueueDepth updates the queue depth gauge.
func (m *Metrics) SetQueueDepth(depth int) {
	if m == nil {
		return
	}
	if depth < 0 {
		depth = 0
	}
	m.mu.Lock()
	m.queueDepth = depth
	m.mu.Unlock()
}

// IncInflight increments the inflight attempt gauge.
func (m *Metrics) IncInflight() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.inflight++
	m.mu.Unlock()
}

// DecInflight decrements the inflight attempt gauge.
func (m *Metrics) DecInflight() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.inflight > 0 {
		m.inflight--
	}
	m.mu.Unlock()
}

// WritePrometheus writes metrics in Prometheus exposition format.
func (m *Metrics) WritePrometheus(w io.Writer) {
	if m == nil {
		return
	}

	m.mu.Lock()
	intentsCreated := m.intentsCreated
	idempotentHits := m.idempotentHits
	idempotencyConflicts := m.idempotencyConflicts
	terminalAccepted := m.terminalAccepted
	terminalRejected := m.terminalRejected
	terminalExhausted := m.terminalExhausted
	exhaustedDeadline := m.exhaustedDeadline
	exhaustedMax := m.exhaustedMax
	exhaustedOneShot := m.exhaustedOneShot
	exhaustedUnknown := m.exhaustedUnknown
	exhaustedOther := m.exhaustedOther
	attemptsAccepted := m.attemptsAccepted
	attemptsRejected := m.attemptsRejected
	attemptsError := m.attemptsError
	retriesScheduled := m.retriesScheduled
	queueDepth := m.queueDepth
	inflight := m.inflight
	intentAcceptedDuration := copyHistogram(m.intentAcceptedDuration)
	intentRejectedDuration := copyHistogram(m.intentRejectedDuration)
	intentExhaustedDuration := copyHistogram(m.intentExhaustedDuration)
	attemptDuration := copyHistogram(m.attemptDuration)
	queueDelay := copyHistogram(m.queueDelay)
	m.mu.Unlock()

	fmt.Fprintf(w, "# HELP submission_intents_created_total Total intents created.\n")
	fmt.Fprintf(w, "# TYPE submission_intents_created_total counter\n")
	fmt.Fprintf(w, "submission_intents_created_total %d\n", intentsCreated)

	fmt.Fprintf(w, "# HELP submission_idempotent_hits_total Idempotent intent submissions.\n")
	fmt.Fprintf(w, "# TYPE submission_idempotent_hits_total counter\n")
	fmt.Fprintf(w, "submission_idempotent_hits_total %d\n", idempotentHits)

	fmt.Fprintf(w, "# HELP submission_idempotency_conflicts_total Idempotency conflicts.\n")
	fmt.Fprintf(w, "# TYPE submission_idempotency_conflicts_total counter\n")
	fmt.Fprintf(w, "submission_idempotency_conflicts_total %d\n", idempotencyConflicts)

	fmt.Fprintf(w, "# HELP submission_intents_terminal_total Terminal intent outcomes.\n")
	fmt.Fprintf(w, "# TYPE submission_intents_terminal_total counter\n")
	fmt.Fprintf(w, "submission_intents_terminal_total{status=%q} %d\n", "accepted", terminalAccepted)
	fmt.Fprintf(w, "submission_intents_terminal_total{status=%q} %d\n", "rejected", terminalRejected)
	fmt.Fprintf(w, "submission_intents_terminal_total{status=%q} %d\n", "exhausted", terminalExhausted)

	fmt.Fprintf(w, "# HELP submission_exhausted_total Exhausted intents by reason.\n")
	fmt.Fprintf(w, "# TYPE submission_exhausted_total counter\n")
	fmt.Fprintf(w, "submission_exhausted_total{reason=%q} %d\n", "deadline_exceeded", exhaustedDeadline)
	fmt.Fprintf(w, "submission_exhausted_total{reason=%q} %d\n", "max_attempts", exhaustedMax)
	fmt.Fprintf(w, "submission_exhausted_total{reason=%q} %d\n", "one_shot", exhaustedOneShot)
	fmt.Fprintf(w, "submission_exhausted_total{reason=%q} %d\n", "unknown_policy", exhaustedUnknown)
	fmt.Fprintf(w, "submission_exhausted_total{reason=%q} %d\n", "unknown_reason", exhaustedOther)

	fmt.Fprintf(w, "# HELP submission_attempts_total Attempt outcomes by status.\n")
	fmt.Fprintf(w, "# TYPE submission_attempts_total counter\n")
	fmt.Fprintf(w, "submission_attempts_total{outcome_status=%q} %d\n", "accepted", attemptsAccepted)
	fmt.Fprintf(w, "submission_attempts_total{outcome_status=%q} %d\n", "rejected", attemptsRejected)
	fmt.Fprintf(w, "submission_attempts_total{outcome_status=%q} %d\n", "error", attemptsError)

	fmt.Fprintf(w, "# HELP submission_retries_scheduled_total Retries scheduled.\n")
	fmt.Fprintf(w, "# TYPE submission_retries_scheduled_total counter\n")
	fmt.Fprintf(w, "submission_retries_scheduled_total %d\n", retriesScheduled)

	fmt.Fprintf(w, "# HELP submission_queue_depth Pending scheduled attempts.\n")
	fmt.Fprintf(w, "# TYPE submission_queue_depth gauge\n")
	fmt.Fprintf(w, "submission_queue_depth %d\n", queueDepth)

	fmt.Fprintf(w, "# HELP submission_inflight_attempts Attempts currently executing.\n")
	fmt.Fprintf(w, "# TYPE submission_inflight_attempts gauge\n")
	fmt.Fprintf(w, "submission_inflight_attempts %d\n", inflight)

	writeHistogram(w, "submission_intent_time_to_terminal_seconds", "Intent time to terminal in seconds.", `status="accepted"`, intentAcceptedDuration)
	writeHistogram(w, "submission_intent_time_to_terminal_seconds", "Intent time to terminal in seconds.", `status="rejected"`, intentRejectedDuration)
	writeHistogram(w, "submission_intent_time_to_terminal_seconds", "Intent time to terminal in seconds.", `status="exhausted"`, intentExhaustedDuration)
	writeHistogram(w, "submission_attempt_duration_seconds", "Attempt execution duration in seconds.", "", attemptDuration)
	writeHistogram(w, "submission_queue_delay_seconds", "Queue delay before attempt execution in seconds.", "", queueDelay)
}

func newHistogram(buckets []float64) histogram {
	return histogram{
		buckets: buckets,
		counts:  make([]uint64, len(buckets)),
	}
}

func (h *histogram) observe(value float64) {
	h.count++
	h.sum += value
	for i, bound := range h.buckets {
		if value <= bound {
			h.counts[i]++
		}
	}
}

func copyHistogram(h histogram) histogram {
	return histogram{
		buckets: append([]float64(nil), h.buckets...),
		counts:  append([]uint64(nil), h.counts...),
		count:   h.count,
		sum:     h.sum,
	}
}

func writeHistogram(w io.Writer, name, help, labels string, h histogram) {
	fmt.Fprintf(w, "# HELP %s %s\n", name, help)
	fmt.Fprintf(w, "# TYPE %s histogram\n", name)
	labelPrefix := labels
	if labelPrefix != "" {
		labelPrefix += ","
	}
	for i, bound := range h.buckets {
		fmt.Fprintf(
			w,
			"%s_bucket{%sle=%q} %d\n",
			name,
			labelPrefix,
			formatFloat(bound),
			h.counts[i],
		)
	}
	fmt.Fprintf(w, "%s_bucket{%sle=%q} %d\n", name, labelPrefix, "+Inf", h.count)
	fmt.Fprintf(w, "%s_sum{%s} %s\n", name, labels, formatFloat(h.sum))
	fmt.Fprintf(w, "%s_count{%s} %d\n", name, labels, h.count)
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
