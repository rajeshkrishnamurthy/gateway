package submissionmanager

import (
	"container/heap"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"gateway/submission"
)

const retryDelay = 5 * time.Second
const waitPollInterval = 250 * time.Millisecond

const (
	gatewayAccepted  = "accepted"
	gatewayRejected  = "rejected"
	webhookPending   = "pending"
	webhookDelivered = "delivered"
	webhookFailed    = "failed"
)

// IntentStatus represents the lifecycle state of a SubmissionIntent.
type IntentStatus string

const (
	// IntentPending means the intent is awaiting completion.
	IntentPending IntentStatus = "pending"
	// IntentAccepted means the intent was accepted by the gateway.
	IntentAccepted IntentStatus = "accepted"
	// IntentRejected means the intent was rejected with a terminal outcome.
	IntentRejected IntentStatus = "rejected"
	// IntentExhausted means the intent exhausted its policy without acceptance.
	IntentExhausted IntentStatus = "exhausted"
)

// Intent is the SubmissionManager record for a client submission.
type Intent struct {
	IntentID           string
	SubmissionTarget   string
	Payload            json.RawMessage
	CreatedAt          time.Time
	CompletedAt        time.Time
	Status             IntentStatus
	Contract           submission.TargetContract
	Attempts           []Attempt
	FinalOutcome       GatewayOutcome // meaningful for accepted/rejected intents only
	ExhaustedReason    string         // explains policy exhaustion, not gateway failure
	WebhookStatus      string
	WebhookAttemptedAt time.Time
	WebhookDeliveredAt time.Time
	WebhookError       string
}

// Attempt captures a single gateway submission attempt.
type Attempt struct {
	Number         int
	StartedAt      time.Time
	FinishedAt     time.Time
	GatewayOutcome GatewayOutcome
	Error          string
}

// GatewayOutcome is the normalized gateway response outcome.
type GatewayOutcome struct {
	Status string
	Reason string
}

// AttemptInput provides resolved routing and payload for an attempt executor.
type AttemptInput struct {
	GatewayType submission.GatewayType
	GatewayURL  string
	Payload     json.RawMessage
}

// AttemptExecutor performs a single gateway submission attempt.
type AttemptExecutor func(context.Context, AttemptInput) (GatewayOutcome, error)

// WebhookDelivery contains the resolved webhook request details.
type WebhookDelivery struct {
	URL        string
	Headers    map[string]string
	HeadersEnv map[string]string
	SecretEnv  string
	Body       []byte
}

// WebhookSender posts a terminal webhook callback.
type WebhookSender func(context.Context, WebhookDelivery) error

// Clock provides time functions for deterministic scheduling.
type Clock struct {
	Now   func() time.Time
	After func(time.Duration) <-chan time.Time
}

// Manager orchestrates SubmissionIntents, attempts, and policy evaluation.
type Manager struct {
	reg           submission.Registry
	exec          AttemptExecutor
	store         *sqlStore
	clock         Clock
	mu            sync.Mutex
	queue         attemptQueue
	wake          chan struct{}
	nextSeq       int
	metrics       *Metrics
	webhookSender WebhookSender
}

// IdempotencyConflictError reports a conflicting submission for the same intentId.
type IdempotencyConflictError struct {
	IntentID        string
	ExistingTarget  string
	ExistingPayload string
	IncomingTarget  string
	IncomingPayload string
	ExistingStatus  IntentStatus
}

func (e IdempotencyConflictError) Error() string {
	return fmt.Sprintf("intent %q already exists with target %q and payload %q (status %q); incoming target %q payload %q", e.IntentID, e.ExistingTarget, e.ExistingPayload, e.ExistingStatus, e.IncomingTarget, e.IncomingPayload)
}

// UnknownSubmissionTargetError reports a submissionTarget that is not in the registry.
type UnknownSubmissionTargetError struct {
	SubmissionTarget string
}

func (e UnknownSubmissionTargetError) Error() string {
	return fmt.Sprintf("unknown submissionTarget %q", e.SubmissionTarget)
}

// NewManager constructs a SubmissionManager with the provided registry, executor, and SQL store.
func NewManager(reg submission.Registry, exec AttemptExecutor, clock Clock, db *sql.DB) (*Manager, error) {
	if exec == nil {
		return nil, errors.New("executor is required")
	}
	if db == nil {
		return nil, errors.New("db is required")
	}
	if clock.Now == nil {
		clock.Now = time.Now
	}
	if clock.After == nil {
		clock.After = time.After
	}

	store, err := newSQLStore(db)
	if err != nil {
		return nil, err
	}

	manager := &Manager{
		reg:   reg,
		exec:  exec,
		store: store,
		clock: clock,
		wake:  make(chan struct{}, 1),
	}
	heap.Init(&manager.queue)
	scheduled, err := store.loadPendingSchedule(context.Background())
	if err != nil {
		return nil, err
	}
	for _, item := range scheduled {
		manager.nextSeq++
		heap.Push(&manager.queue, scheduledAttempt{
			intentID: item.intentID,
			due:      item.due,
			seq:      manager.nextSeq,
		})
	}
	return manager, nil
}

// SetMetrics assigns a metrics registry to the manager.
func (m *Manager) SetMetrics(metrics *Metrics) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.metrics = metrics
	depth := len(m.queue.items)
	m.mu.Unlock()
	if metrics != nil {
		metrics.SetQueueDepth(depth)
	}
}

// SetWebhookSender assigns the webhook sender used for terminal callbacks.
func (m *Manager) SetWebhookSender(sender WebhookSender) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.webhookSender = sender
	m.mu.Unlock()
}

// Run executes scheduled attempts until the context is canceled.
func (m *Manager) Run(ctx context.Context) {
	// Flow intent: run due attempts in time order until stop.
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		m.mu.Lock()
		if len(m.queue.items) == 0 {
			m.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-m.wake:
				continue
			}
		}

		next := m.queue.items[0]
		now := m.clock.Now()
		wait := next.due.Sub(now)
		if wait <= 0 {
			// Concurrency/locking intent: pop under lock so the queue stays correct,
			// then run outside the lock so we do not hold it during the gateway call.
			heap.Pop(&m.queue)
			if m.metrics != nil {
				m.metrics.SetQueueDepth(len(m.queue.items))
			}
			m.mu.Unlock()
			m.executeAttempt(ctx, next.intentID, next.due)
			continue
		}
		m.mu.Unlock()

		select {
		case <-ctx.Done():
			return
		case <-m.wake:
			continue
		case <-m.clock.After(wait):
			continue
		}
	}
}

// SubmitIntent registers an intent and schedules its first attempt.
func (m *Manager) SubmitIntent(ctx context.Context, intent Intent) (Intent, error) {
	// Flow intent: check idempotency, store intent, schedule first attempt.
	intentID := strings.TrimSpace(intent.IntentID)
	if intentID == "" {
		return Intent{}, errors.New("intentId is required")
	}
	submissionTarget := strings.TrimSpace(intent.SubmissionTarget)
	if submissionTarget == "" {
		return Intent{}, errors.New("submissionTarget is required")
	}

	payload := normalizePayload(intent.Payload)

	contract, ok := m.reg.ContractFor(submissionTarget)
	if !ok {
		return Intent{}, UnknownSubmissionTargetError{SubmissionTarget: submissionTarget}
	}
	// Freeze a contract snapshot so registry changes never affect existing intents.
	contract = cloneContract(contract)

	createdAt := m.clock.Now()
	newIntent := Intent{
		IntentID:         intentID,
		SubmissionTarget: submissionTarget,
		Payload:          payload,
		CreatedAt:        createdAt,
		Status:           IntentPending,
		Contract:         contract,
	}

	stored, inserted, err := m.store.insertIntent(ctx, newIntent, payloadHash(payload), createdAt)
	if err != nil {
		var conflict IdempotencyConflictError
		if errors.As(err, &conflict) {
			if m.metrics != nil {
				m.metrics.ObserveIdempotencyConflict()
			}
		}
		return Intent{}, err
	}
	if inserted {
		if m.metrics != nil {
			m.metrics.ObserveIntentCreated()
		}
		m.enqueueAttempt(intentID, createdAt)
	} else if m.metrics != nil {
		m.metrics.ObserveIdempotentHit()
	}
	return stored, nil
}

// GetIntent returns the current intent state by intentId.
func (m *Manager) GetIntent(intentID string) (Intent, bool) {
	trimmed := strings.TrimSpace(intentID)
	if trimmed == "" {
		return Intent{}, false
	}
	intent, ok, err := m.store.loadIntent(context.Background(), trimmed)
	if err != nil || !ok {
		return Intent{}, false
	}
	return intent, true
}

// WaitForIntent polls SQL until the intent reaches a terminal state, completes its first attempt,
// or the wait duration elapses.
func (m *Manager) WaitForIntent(ctx context.Context, intentID string, wait time.Duration) (Intent, bool, error) {
	// Flow intent: poll SQL until terminal, first attempt done, or timeout.
	trimmed := strings.TrimSpace(intentID)
	if trimmed == "" {
		return Intent{}, false, nil
	}
	if wait <= 0 {
		intent, ok := m.GetIntent(trimmed)
		return intent, ok, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	deadline := m.clock.Now().Add(wait)
	var last Intent
	var ok bool

	for {
		// Non-obvious constraint: read SQL because the executor may be another process.
		intent, attemptCount, found, err := m.store.loadIntentRow(context.Background(), trimmed)
		if err != nil || !found {
			return Intent{}, found, err
		}
		last = intent
		ok = true
		if intent.Status == IntentAccepted || intent.Status == IntentRejected || intent.Status == IntentExhausted {
			return intent, true, nil
		}
		// Non-obvious constraint: wait stops after the first attempt, even if still pending.
		if attemptCount >= 1 {
			return intent, true, nil
		}

		now := m.clock.Now()
		if !now.Before(deadline) {
			return last, ok, nil
		}
		remaining := deadline.Sub(now)
		waitFor := waitPollInterval
		if remaining < waitFor {
			waitFor = remaining
		}

		select {
		case <-ctx.Done():
			return last, ok, nil
		case <-m.clock.After(waitFor):
		}
	}
}

func (m *Manager) enqueueAttempt(intentID string, due time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enqueueAttemptLocked(intentID, due)
}

func (m *Manager) enqueueAttemptLocked(intentID string, due time.Time) {
	m.nextSeq++
	heap.Push(&m.queue, scheduledAttempt{
		intentID: intentID,
		due:      due,
		seq:      m.nextSeq,
	})
	if m.metrics != nil {
		m.metrics.SetQueueDepth(len(m.queue.items))
	}
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Manager) executeAttempt(ctx context.Context, intentID string, due time.Time) {
	// Flow intent: load intent, call gateway, apply policy, save result.
	start := m.clock.Now()

	intent, attemptCount, ok, err := m.store.loadIntentForExecution(ctx, intentID, start)
	if err != nil || !ok {
		return
	}
	if m.metrics != nil {
		m.metrics.ObserveQueueDelay(start.Sub(due))
	}
	log.Printf("intentId=%q attempt=%d gatewayType=%s action=start", intentID, attemptCount+1, intent.Contract.GatewayType)
	if intent.Contract.Policy == submission.PolicyDeadline {
		deadline := intent.CreatedAt.Add(time.Duration(intent.Contract.MaxAcceptanceSeconds) * time.Second)
		// Policy vs outcome: do not execute attempts after the acceptance deadline.
		if !start.Before(deadline) {
			applied, _ := m.store.markExhausted(ctx, intentID, "deadline_exceeded", start)
			if applied && m.metrics != nil {
				m.metrics.ObserveIntentTerminal(IntentExhausted, start.Sub(intent.CreatedAt))
				m.metrics.ObserveExhausted("deadline_exceeded")
			}
			log.Printf("intentId=%q status=%s exhaustedReason=%s", intentID, IntentExhausted, "deadline_exceeded")
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

	outcome, err := m.exec(ctx, AttemptInput{
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
	log.Printf("intentId=%q attempt=%d outcomeStatus=%q outcomeReason=%q error=%q status=%s retry=%t nextDue=%s", intentID, attempt.Number, attempt.GatewayOutcome.Status, attempt.GatewayOutcome.Reason, attempt.Error, intent.Status, retry, nextDue)
	var nextAttemptAt *time.Time
	if retry {
		nextAttemptAt = &due
	}
	applied, err := m.store.recordAttempt(ctx, intentID, attempt, intent.Status, intent.FinalOutcome, intent.ExhaustedReason, nextAttemptAt, finish)
	if err != nil || !applied {
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

func (m *Manager) dispatchWebhook(ctx context.Context, intent Intent, occurredAt time.Time) {
	if m.webhookSender == nil || intent.Contract.Webhook == nil {
		return
	}
	if intent.WebhookStatus != webhookPending {
		return
	}
	delivery, err := buildWebhookDelivery(intent, occurredAt)
	if err != nil {
		_ = m.store.recordWebhookAttempt(ctx, intent.IntentID, webhookFailed, occurredAt, err.Error())
		return
	}
	if err := m.webhookSender(ctx, delivery); err != nil {
		_ = m.store.recordWebhookAttempt(ctx, intent.IntentID, webhookFailed, occurredAt, err.Error())
		return
	}
	_ = m.store.recordWebhookAttempt(ctx, intent.IntentID, webhookDelivered, occurredAt, "")
}

func buildWebhookDelivery(intent Intent, occurredAt time.Time) (WebhookDelivery, error) {
	webhook := intent.Contract.Webhook
	if webhook == nil {
		return WebhookDelivery{}, errors.New("webhook is not configured")
	}
	intentPayload := struct {
		IntentID         string `json:"intentId"`
		SubmissionTarget string `json:"submissionTarget"`
		CreatedAt        string `json:"createdAt"`
		CompletedAt      string `json:"completedAt"`
		Status           string `json:"status"`
		RejectedReason   string `json:"rejectedReason,omitempty"`
		ExhaustedReason  string `json:"exhaustedReason,omitempty"`
	}{
		IntentID:         intent.IntentID,
		SubmissionTarget: intent.SubmissionTarget,
		CreatedAt:        intent.CreatedAt.UTC().Format(time.RFC3339Nano),
		CompletedAt:      occurredAt.UTC().Format(time.RFC3339Nano),
		Status:           string(intent.Status),
	}
	switch intent.Status {
	case IntentRejected:
		intentPayload.RejectedReason = intent.FinalOutcome.Reason
	case IntentExhausted:
		intentPayload.ExhaustedReason = intent.ExhaustedReason
	}
	payload := struct {
		EventID    string      `json:"eventId"`
		EventType  string      `json:"eventType"`
		OccurredAt string      `json:"occurredAt"`
		Intent     interface{} `json:"intent"`
	}{
		EventID:    intent.IntentID,
		EventType:  "intent.terminal",
		OccurredAt: occurredAt.UTC().Format(time.RFC3339Nano),
		Intent:     intentPayload,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return WebhookDelivery{}, err
	}
	headers := make(map[string]string, len(webhook.Headers)+3)
	for key, value := range webhook.Headers {
		headers[key] = value
	}
	headers["Content-Type"] = "application/json"
	headers["X-Setu-Event-Type"] = "intent.terminal"
	headers["X-Setu-Event-Id"] = intent.IntentID

	headersEnv := map[string]string{}
	for key, value := range webhook.HeadersEnv {
		headersEnv[key] = value
	}

	return WebhookDelivery{
		URL:        webhook.URL,
		Headers:    headers,
		HeadersEnv: headersEnv,
		SecretEnv:  webhook.SecretEnv,
		Body:       body,
	}, nil
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

func normalizePayload(payload json.RawMessage) json.RawMessage {
	if payload == nil {
		return []byte{}
	}
	return clonePayload(payload)
}

func clonePayload(payload json.RawMessage) json.RawMessage {
	copyPayload := make([]byte, len(payload))
	copy(copyPayload, payload)
	return copyPayload
}

func cloneContract(contract submission.TargetContract) submission.TargetContract {
	clone := contract
	if len(contract.TerminalOutcomes) > 0 {
		clone.TerminalOutcomes = append([]string(nil), contract.TerminalOutcomes...)
	}
	if contract.Webhook != nil {
		clone.Webhook = cloneWebhook(contract.Webhook)
	}
	return clone
}

func cloneWebhook(webhook *submission.WebhookConfig) *submission.WebhookConfig {
	if webhook == nil {
		return nil
	}
	clone := &submission.WebhookConfig{
		URL:       webhook.URL,
		SecretEnv: webhook.SecretEnv,
	}
	if len(webhook.Headers) > 0 {
		clone.Headers = make(map[string]string, len(webhook.Headers))
		for key, value := range webhook.Headers {
			clone.Headers[key] = value
		}
	}
	if len(webhook.HeadersEnv) > 0 {
		clone.HeadersEnv = make(map[string]string, len(webhook.HeadersEnv))
		for key, value := range webhook.HeadersEnv {
			clone.HeadersEnv[key] = value
		}
	}
	return clone
}

type scheduledAttempt struct {
	intentID string
	due      time.Time
	seq      int
}

type attemptQueue struct {
	items []scheduledAttempt
}

// attemptQueue is a min-heap ordered by due time. seq preserves FIFO ordering
// for attempts with the same due time.
func (q attemptQueue) Len() int { return len(q.items) }

func (q attemptQueue) Less(i, j int) bool {
	if q.items[i].due.Equal(q.items[j].due) {
		return q.items[i].seq < q.items[j].seq
	}
	return q.items[i].due.Before(q.items[j].due)
}

func (q attemptQueue) Swap(i, j int) {
	q.items[i], q.items[j] = q.items[j], q.items[i]
}

func (q *attemptQueue) Push(x any) {
	q.items = append(q.items, x.(scheduledAttempt))
}

func (q *attemptQueue) Pop() any {
	item := q.items[len(q.items)-1]
	q.items = q.items[:len(q.items)-1]
	return item
}
