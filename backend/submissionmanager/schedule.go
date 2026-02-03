package submissionmanager

import (
	"container/heap"
	"context"
	"time"
)

// Run executes scheduled attempts until the context is canceled.
func (m *Manager) Run(ctx context.Context) {
	// Flow intent: run due attempts in time order until stop.
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if !m.isLeader() {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}

		m.mu.Lock()
		if !m.leader {
			m.mu.Unlock()
			return
		}
		if len(m.queue.items) == 0 {
			m.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-m.wake:
				continue
			}
		}
		m.mu.Unlock()

		now, err := m.scheduleTimeNow(ctx)
		if err != nil {
			if ctx.Err() == nil {
				m.notifyLeaseLoss()
			}
			return
		}

		m.mu.Lock()
		if !m.leader {
			m.mu.Unlock()
			return
		}
		if len(m.queue.items) == 0 {
			m.mu.Unlock()
			continue
		}

		next := m.queue.items[0]
		wait := next.due.Sub(now)
		if wait <= 0 {
			// Concurrency/locking intent: pop under lock so the queue stays correct,
			// then run outside the lock so we do not hold it during the gateway call.
			heap.Pop(&m.queue)
			due, ok := m.scheduled[next.intentID]
			if !ok || !due.Equal(next.due) {
				m.mu.Unlock()
				continue
			}
			delete(m.scheduled, next.intentID)
			if m.metrics != nil {
				m.metrics.SetQueueDepth(len(m.scheduled))
			}
			m.mu.Unlock()
			if !m.isLeader() {
				return
			}
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

func (m *Manager) enqueueAttempt(intentID string, due time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enqueueAttemptLocked(intentID, due)
}

func (m *Manager) enqueueAttemptLocked(intentID string, due time.Time) {
	if m.scheduled == nil {
		m.scheduled = make(map[string]time.Time)
	}
	if existing, ok := m.scheduled[intentID]; ok && existing.Equal(due) {
		return
	}
	m.nextSeq++
	m.scheduled[intentID] = due
	heap.Push(&m.queue, scheduledAttempt{
		intentID: intentID,
		due:      due,
		seq:      m.nextSeq,
	})
	if m.metrics != nil {
		m.metrics.SetQueueDepth(len(m.scheduled))
	}
	select {
	case m.wake <- struct{}{}:
	default:
	}
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
