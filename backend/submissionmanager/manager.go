package submissionmanager

import (
	"container/heap"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gateway/submission"
)

const waitPollInterval = 250 * time.Millisecond

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
	scheduled     map[string]time.Time
	leader        bool
	leaseFence    LeaseFence
	leaseLossFn   func()
	scheduleNow   func(context.Context) (time.Time, error)
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
		reg:         reg,
		exec:        exec,
		store:       store,
		clock:       clock,
		wake:        make(chan struct{}, 1),
		scheduled:   make(map[string]time.Time),
		scheduleNow: store.loadSQLTime,
	}
	heap.Init(&manager.queue)
	return manager, nil
}

// SetMetrics assigns a metrics registry to the manager.
func (m *Manager) SetMetrics(metrics *Metrics) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.metrics = metrics
	depth := len(m.scheduled)
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

func (m *Manager) scheduleTimeNow(ctx context.Context) (time.Time, error) {
	if m.scheduleNow == nil {
		return time.Time{}, errors.New("schedule time source is required")
	}
	return m.scheduleNow(ctx)
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
		if m.isLeader() {
			m.enqueueAttempt(intentID, createdAt)
		}
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
