package submissionmanager

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"
)

type LeaderRunner struct {
	store   *sqlStore
	manager *Manager
	cfg     LeaseConfig

	mu     sync.Mutex
	status LeaseStatus
}

func NewLeaderRunner(store *sqlStore, manager *Manager, cfg LeaseConfig) *LeaderRunner {
	status := LeaseStatus{
		Mode:     leaseModeFollower,
		HolderID: cfg.HolderID,
	}
	return &LeaderRunner{
		store:   store,
		manager: manager,
		cfg:     cfg,
		status:  status,
	}
}

func NewLeaderRunnerFromManager(manager *Manager, cfg LeaseConfig) *LeaderRunner {
	if manager == nil {
		return NewLeaderRunner(nil, nil, cfg)
	}
	return NewLeaderRunner(manager.store, manager, cfg)
}

func (r *LeaderRunner) Run(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	r.setStatus(LeaseStatus{Mode: leaseModeFollower, HolderID: r.cfg.HolderID})

	for {
		select {
		case <-ctx.Done():
			r.manager.setFollower()
			r.setStatus(LeaseStatus{Mode: leaseModeFollower, HolderID: r.cfg.HolderID})
			return
		default:
		}

		lease, acquired, err := r.store.acquireLease(ctx, r.cfg)
		if err != nil {
			log.Printf("leader_acquire_failed holder_id=%s sql_error=%v", r.cfg.HolderID, err)
		} else if !acquired {
			log.Printf("leader_acquire_failed holder_id=%s", r.cfg.HolderID)
		} else {
			r.runLeader(ctx, lease)
		}

		if !sleepWithContext(ctx, r.cfg.AcquireInterval) {
			r.manager.setFollower()
			r.setStatus(LeaseStatus{Mode: leaseModeFollower, HolderID: r.cfg.HolderID})
			return
		}
	}
}

func (r *LeaderRunner) Status() LeaseStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

func (r *LeaderRunner) IsLeader() bool {
	status := r.Status()
	return status.Mode == leaseModeLeader
}

func (r *LeaderRunner) CurrentLease() (holderID string, epoch int64, ok bool) {
	status := r.Status()
	if status.Mode != leaseModeLeader {
		return "", 0, false
	}
	return status.HolderID, status.LeaseEpoch, true
}

func (r *LeaderRunner) runLeader(ctx context.Context, lease leaseRow) {
	lostCh := make(chan error, 1)
	var lostOnce sync.Once
	signalLoss := func(err error) {
		lostOnce.Do(func() {
			lostCh <- err
		})
	}

	fence := LeaseFence{
		LeaseName:  r.cfg.LeaseName,
		HolderID:   r.cfg.HolderID,
		LeaseEpoch: lease.leaseEpoch,
	}
	r.manager.setLeader(fence, func() {
		signalLoss(errors.New("lease lost"))
	})
	r.setStatus(LeaseStatus{
		Mode:       leaseModeLeader,
		HolderID:   r.cfg.HolderID,
		LeaseEpoch: lease.leaseEpoch,
		ExpiresAt:  lease.expiresAt,
	})
	log.Printf("leader_acquired holder_id=%s lease_epoch=%d expires_at=%s", r.cfg.HolderID, lease.leaseEpoch, lease.expiresAt.UTC().Format(time.RFC3339Nano))

	cursor, err := r.manager.rebuildSchedule(ctx)
	if err != nil {
		signalLoss(err)
	}

	if err == nil {
		renewed, ok, err := r.store.renewLease(ctx, r.cfg, lease.leaseEpoch)
		if err != nil || !ok {
			if err != nil {
				log.Printf("leader_renew_failed holder_id=%s lease_epoch=%d sql_error=%v", r.cfg.HolderID, lease.leaseEpoch, err)
			} else {
				log.Printf("leader_renew_failed holder_id=%s lease_epoch=%d", r.cfg.HolderID, lease.leaseEpoch)
			}
			signalLoss(err)
		} else {
			r.setStatus(LeaseStatus{
				Mode:       leaseModeLeader,
				HolderID:   r.cfg.HolderID,
				LeaseEpoch: renewed.leaseEpoch,
				ExpiresAt:  renewed.expiresAt,
			})
			log.Printf("leader_renewed holder_id=%s lease_epoch=%d expires_at=%s", r.cfg.HolderID, renewed.leaseEpoch, renewed.expiresAt.UTC().Format(time.RFC3339Nano))
		}
	}

	select {
	case err := <-lostCh:
		r.dropLeadership(err)
		return
	default:
	}

	leaderCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go r.manager.Run(leaderCtx)
	go r.runRenewLoop(leaderCtx, lease.leaseEpoch, signalLoss)
	go r.runRefreshLoop(leaderCtx, cursor, signalLoss)

	select {
	case <-ctx.Done():
		cancel()
		r.dropLeadership(nil)
	case err := <-lostCh:
		cancel()
		r.dropLeadership(err)
	}
}

func (r *LeaderRunner) runRenewLoop(ctx context.Context, epoch int64, signalLoss func(error)) {
	ticker := time.NewTicker(r.cfg.RenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			renewed, ok, err := r.store.renewLease(ctx, r.cfg, epoch)
			if err != nil || !ok {
				if err != nil {
					log.Printf("leader_renew_failed holder_id=%s lease_epoch=%d sql_error=%v", r.cfg.HolderID, epoch, err)
				} else {
					log.Printf("leader_renew_failed holder_id=%s lease_epoch=%d", r.cfg.HolderID, epoch)
				}
				signalLoss(err)
				return
			}
			r.setStatus(LeaseStatus{
				Mode:       leaseModeLeader,
				HolderID:   r.cfg.HolderID,
				LeaseEpoch: renewed.leaseEpoch,
				ExpiresAt:  renewed.expiresAt,
			})
			log.Printf("leader_renewed holder_id=%s lease_epoch=%d expires_at=%s", r.cfg.HolderID, renewed.leaseEpoch, renewed.expiresAt.UTC().Format(time.RFC3339Nano))
		}
	}
}

func (r *LeaderRunner) runRefreshLoop(ctx context.Context, cursor scheduleCursor, signalLoss func(error)) {
	ticker := time.NewTicker(r.cfg.ScheduleRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			next, err := r.manager.refreshSchedule(ctx, cursor)
			if err != nil {
				signalLoss(err)
				return
			}
			cursor = next
		}
	}
}

func (r *LeaderRunner) dropLeadership(err error) {
	r.manager.setFollower()
	r.setStatus(LeaseStatus{Mode: leaseModeFollower, HolderID: r.cfg.HolderID})
	if err != nil {
		log.Printf("leader_lost holder_id=%s sql_error=%v", r.cfg.HolderID, err)
	} else {
		log.Printf("leader_lost holder_id=%s", r.cfg.HolderID)
	}
}

func (r *LeaderRunner) setStatus(status LeaseStatus) {
	r.mu.Lock()
	r.status = status
	r.mu.Unlock()
}

func sleepWithContext(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
