package submissionmanager

import "time"

func (m *Manager) setLeader(fence LeaseFence, onLoss func()) {
	m.mu.Lock()
	m.leader = true
	m.leaseFence = fence
	m.leaseLossFn = onLoss
	m.mu.Unlock()
}

func (m *Manager) setFollower() {
	m.mu.Lock()
	m.leader = false
	m.leaseFence = LeaseFence{}
	m.leaseLossFn = nil
	m.clearScheduleLocked()
	m.mu.Unlock()
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Manager) isLeader() bool {
	m.mu.Lock()
	leader := m.leader
	m.mu.Unlock()
	return leader
}

func (m *Manager) currentFence() (LeaseFence, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.leader {
		return LeaseFence{}, false
	}
	return m.leaseFence, true
}

func (m *Manager) notifyLeaseLoss() {
	m.mu.Lock()
	callback := m.leaseLossFn
	m.mu.Unlock()
	if callback != nil {
		callback()
	}
}

func normalizeLeaseExpiry(value time.Time) time.Time {
	if value.IsZero() {
		return value
	}
	return normalizeDBTime(value)
}
