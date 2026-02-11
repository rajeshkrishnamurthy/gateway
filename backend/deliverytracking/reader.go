package deliverytracking

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"gateway/submission"
)

// Reader materializes delivery tracking views from persisted state.
type Reader struct {
	store *sqlStore
}

// CurrentDeliveryView is the canonical delivery read projection for one intent.
type CurrentDeliveryView struct {
	IntentID             string
	DeliveryTrackingMode submission.DeliveryTrackingMode
	DeliveryStatus       *DeliveryStatus
	DeliveryFreshness    *DeliveryFreshness
}

// DeliveryHistoryEntry is one ordered delivery history row.
type DeliveryHistoryEntry struct {
	RecordedAt             time.Time
	ProviderDeliverySignal DeliverySignalClass
	DeliveryStatus         DeliveryStatus
	LateObservation        bool
}

// DeliveryHistoryView is the canonical delivery history payload for one intent.
type DeliveryHistoryView struct {
	IntentID             string
	DeliveryTrackingMode submission.DeliveryTrackingMode
	Entries              []DeliveryHistoryEntry
}

type intentDeliveryReadSnapshot struct {
	intentID          string
	deliveryMode      submission.DeliveryTrackingMode
	staleAfterSeconds int
	createdAt         time.Time
}

// NewReader constructs a delivery read materializer backed by SQL state.
func NewReader(db *sql.DB) (*Reader, error) {
	store, err := newSQLStore(db)
	if err != nil {
		return nil, err
	}
	return &Reader{store: store}, nil
}

// GetCurrentDelivery returns the current delivery view for an intent.
func (r *Reader) GetCurrentDelivery(ctx context.Context, intentID string) (CurrentDeliveryView, bool, error) {
	if r == nil || r.store == nil {
		return CurrentDeliveryView{}, false, errors.New("reader store is required")
	}
	intentID = strings.TrimSpace(intentID)
	if intentID == "" {
		return CurrentDeliveryView{}, false, errors.New("intentId is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	tx, err := r.store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return CurrentDeliveryView{}, false, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	intent, found, err := loadIntentDeliveryReadSnapshot(ctx, tx, intentID)
	if err != nil {
		return CurrentDeliveryView{}, false, err
	}
	if !found {
		return CurrentDeliveryView{}, false, nil
	}

	view := CurrentDeliveryView{
		IntentID:             intent.intentID,
		DeliveryTrackingMode: intent.deliveryMode,
	}
	if intent.deliveryMode == submission.DeliveryTrackingModeOn {
		state, stateFound, err := loadCurrentDeliveryState(ctx, tx, intent.intentID)
		if err != nil {
			return CurrentDeliveryView{}, false, err
		}
		if stateFound {
			status := state.deliveryStatus
			freshness := state.deliveryFreshness
			view.DeliveryStatus = &status
			view.DeliveryFreshness = &freshness
		} else {
			fallbackStatus := DeliveryStatusUnknown
			fallbackFreshness, err := computeFallbackFreshness(ctx, tx, intent.createdAt, intent.staleAfterSeconds)
			if err != nil {
				return CurrentDeliveryView{}, false, err
			}
			view.DeliveryStatus = &fallbackStatus
			view.DeliveryFreshness = &fallbackFreshness
		}
	}

	if err := tx.Commit(); err != nil {
		return CurrentDeliveryView{}, false, err
	}
	return view, true, nil
}

// GetDeliveryHistory returns the ordered delivery history view for an intent.
func (r *Reader) GetDeliveryHistory(ctx context.Context, intentID string) (DeliveryHistoryView, bool, error) {
	if r == nil || r.store == nil {
		return DeliveryHistoryView{}, false, errors.New("reader store is required")
	}
	intentID = strings.TrimSpace(intentID)
	if intentID == "" {
		return DeliveryHistoryView{}, false, errors.New("intentId is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	tx, err := r.store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return DeliveryHistoryView{}, false, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	intent, found, err := loadIntentDeliveryReadSnapshot(ctx, tx, intentID)
	if err != nil {
		return DeliveryHistoryView{}, false, err
	}
	if !found {
		return DeliveryHistoryView{}, false, nil
	}

	view := DeliveryHistoryView{
		IntentID:             intent.intentID,
		DeliveryTrackingMode: intent.deliveryMode,
		Entries:              []DeliveryHistoryEntry{},
	}
	if intent.deliveryMode == submission.DeliveryTrackingModeOn {
		entries, err := loadDeliveryHistoryEntries(ctx, tx, intent.intentID)
		if err != nil {
			return DeliveryHistoryView{}, false, err
		}
		view.Entries = entries
	}

	if err := tx.Commit(); err != nil {
		return DeliveryHistoryView{}, false, err
	}
	return view, true, nil
}

func loadIntentDeliveryReadSnapshot(ctx context.Context, tx *sql.Tx, intentID string) (intentDeliveryReadSnapshot, bool, error) {
	row := tx.QueryRowContext(
		ctx,
		`SELECT intent_id, delivery_tracking_mode, delivery_tracking_stale_after_seconds, created_at
     FROM dbo.submission_intents
     WHERE intent_id = @p1`,
		intentID,
	)

	var (
		intentIDValue     string
		deliveryModeValue string
		staleAfterSeconds sql.NullInt32
		createdAt         time.Time
	)
	if err := row.Scan(&intentIDValue, &deliveryModeValue, &staleAfterSeconds, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return intentDeliveryReadSnapshot{}, false, nil
		}
		return intentDeliveryReadSnapshot{}, false, err
	}

	mode := submission.DeliveryTrackingMode(strings.TrimSpace(deliveryModeValue))
	switch mode {
	case submission.DeliveryTrackingModeOn, submission.DeliveryTrackingModeOff:
	default:
		return intentDeliveryReadSnapshot{}, false, fmt.Errorf("unsupported delivery tracking mode %q", deliveryModeValue)
	}

	staleAfter := 0
	if staleAfterSeconds.Valid {
		staleAfter = int(staleAfterSeconds.Int32)
	}
	if mode == submission.DeliveryTrackingModeOn && staleAfter <= 0 {
		return intentDeliveryReadSnapshot{}, false, errors.New("deliveryTracking.staleAfterSeconds must be greater than zero when mode=on")
	}

	return intentDeliveryReadSnapshot{
		intentID:          strings.TrimSpace(intentIDValue),
		deliveryMode:      mode,
		staleAfterSeconds: staleAfter,
		createdAt:         normalizeDBTime(createdAt),
	}, true, nil
}

func loadCurrentDeliveryState(ctx context.Context, tx *sql.Tx, intentID string) (deliveryStateProjection, bool, error) {
	row := tx.QueryRowContext(
		ctx,
		`SELECT delivery_status, delivery_freshness
     FROM dbo.intent_delivery_state
     WHERE intent_id = @p1`,
		intentID,
	)
	var (
		deliveryStatus    string
		deliveryFreshness string
	)
	if err := row.Scan(&deliveryStatus, &deliveryFreshness); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return deliveryStateProjection{}, false, nil
		}
		return deliveryStateProjection{}, false, err
	}
	return deliveryStateProjection{
		deliveryStatus:    DeliveryStatus(strings.TrimSpace(deliveryStatus)),
		deliveryFreshness: DeliveryFreshness(strings.TrimSpace(deliveryFreshness)),
	}, true, nil
}

func computeFallbackFreshness(ctx context.Context, tx *sql.Tx, createdAt time.Time, staleAfterSeconds int) (DeliveryFreshness, error) {
	now, err := loadSQLTimeTx(ctx, tx)
	if err != nil {
		return "", err
	}
	staleAt := normalizeDBTime(createdAt).Add(time.Duration(staleAfterSeconds) * time.Second)
	if now.Before(staleAt) {
		return DeliveryFreshnessFresh, nil
	}
	return DeliveryFreshnessStale, nil
}

func loadDeliveryHistoryEntries(ctx context.Context, tx *sql.Tx, intentID string) ([]DeliveryHistoryEntry, error) {
	rows, err := tx.QueryContext(
		ctx,
		`SELECT received_at, signal_class, late_observation
     FROM dbo.intent_delivery_history
     WHERE intent_id = @p1
     ORDER BY history_seq`,
		intentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]DeliveryHistoryEntry, 0)
	for rows.Next() {
		var (
			recordedAt   time.Time
			signalClass  string
			lateObserved bool
		)
		if err := rows.Scan(&recordedAt, &signalClass, &lateObserved); err != nil {
			return nil, err
		}
		signal := DeliverySignalClass(strings.TrimSpace(signalClass))
		entries = append(entries, DeliveryHistoryEntry{
			RecordedAt:             normalizeDBTime(recordedAt),
			ProviderDeliverySignal: signal,
			DeliveryStatus:         historyDeliveryStatus(signal),
			LateObservation:        lateObserved,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func historyDeliveryStatus(signal DeliverySignalClass) DeliveryStatus {
	switch signal {
	case DeliverySignalInProgress:
		return DeliveryStatusInProgress
	case DeliverySignalTerminalSuccess:
		return DeliveryStatusDelivered
	case DeliverySignalTerminalFailure:
		return DeliveryStatusFailed
	default:
		return DeliveryStatusUnknown
	}
}
