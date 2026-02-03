package submissionmanager

import (
	"context"
	"errors"
	"time"
)

type scheduleCursor struct {
	lastModified time.Time
	intentID     string
}

func (m *Manager) rebuildSchedule(ctx context.Context) (scheduleCursor, error) {
	if !m.isLeader() {
		return scheduleCursor{}, errors.New("schedule rebuild requires leader")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	rows, err := m.store.loadScheduleSnapshot(ctx)
	if err != nil {
		return scheduleCursor{}, err
	}

	var cursor scheduleCursor
	m.mu.Lock()
	m.clearScheduleLocked()
	for _, row := range rows {
		m.enqueueAttemptLocked(row.intentID, row.due)
		cursor.lastModified = row.lastModified
		cursor.intentID = row.intentID
	}
	m.mu.Unlock()
	return cursor, nil
}

func (m *Manager) refreshSchedule(ctx context.Context, cursor scheduleCursor) (scheduleCursor, error) {
	if !m.isLeader() {
		return cursor, errors.New("schedule refresh requires leader")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	changes, err := m.store.loadScheduleChanges(ctx, cursor)
	if err != nil {
		return cursor, err
	}
	if len(changes) == 0 {
		return cursor, nil
	}

	m.mu.Lock()
	for _, change := range changes {
		cursor.lastModified = change.lastModified
		cursor.intentID = change.intentID
		if change.status != IntentPending || change.due == nil {
			delete(m.scheduled, change.intentID)
			continue
		}
		m.enqueueAttemptLocked(change.intentID, *change.due)
	}
	if m.metrics != nil {
		m.metrics.SetQueueDepth(len(m.scheduled))
	}
	m.mu.Unlock()
	return cursor, nil
}

func (m *Manager) clearScheduleLocked() {
	m.queue.items = nil
	if m.scheduled == nil {
		m.scheduled = make(map[string]time.Time)
	} else {
		for key := range m.scheduled {
			delete(m.scheduled, key)
		}
	}
	m.nextSeq = 0
	if m.metrics != nil {
		m.metrics.SetQueueDepth(len(m.scheduled))
	}
}
