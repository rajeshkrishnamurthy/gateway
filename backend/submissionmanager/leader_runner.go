package submissionmanager

import (
	"context"
	"errors"
	setulog "gateway/logging"
	"log/slog"
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
	logger := r.logger()
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
			logger.Warn(
				"leader acquire failed",
				"event", "leader_acquire_failed",
				"stage", "acquire",
				"outcome", "failed",
				"holder_id", r.cfg.HolderID,
				"sql_error", err,
			)
		} else if !acquired {
			logger.Debug(
				"leader acquire not granted",
				"event", "leader_acquire_failed",
				"stage", "acquire",
				"outcome", "not_acquired",
				"holder_id", r.cfg.HolderID,
			)
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
	logger := r.logger()
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
	logger.Info(
		"leader acquired",
		"event", "leader_acquired",
		"stage", "acquire",
		"outcome", "acquired",
		"holder_id", r.cfg.HolderID,
		"lease_epoch", lease.leaseEpoch,
		"leaseEpoch", lease.leaseEpoch,
		"expires_at", lease.expiresAt.UTC().Format(time.RFC3339Nano),
	)

	cursor, err := r.manager.rebuildSchedule(ctx)
	if err != nil {
		signalLoss(err)
	}

	if err == nil {
		renewed, ok, err := r.store.renewLease(ctx, r.cfg, lease.leaseEpoch)
		if err != nil || !ok {
			if err != nil {
				logger.Warn(
					"leader renew failed",
					"event", "leader_renew_failed",
					"stage", "renew",
					"outcome", "failed",
					"holder_id", r.cfg.HolderID,
					"lease_epoch", lease.leaseEpoch,
					"leaseEpoch", lease.leaseEpoch,
					"sql_error", err,
				)
			} else {
				logger.Warn(
					"leader renew failed",
					"event", "leader_renew_failed",
					"stage", "renew",
					"outcome", "failed",
					"holder_id", r.cfg.HolderID,
					"lease_epoch", lease.leaseEpoch,
					"leaseEpoch", lease.leaseEpoch,
				)
			}
			signalLoss(err)
		} else {
			r.setStatus(LeaseStatus{
				Mode:       leaseModeLeader,
				HolderID:   r.cfg.HolderID,
				LeaseEpoch: renewed.leaseEpoch,
				ExpiresAt:  renewed.expiresAt,
			})
			logger.Debug(
				"leader renewed",
				"event", "leader_renewed",
				"stage", "renew",
				"outcome", "renewed",
				"holder_id", r.cfg.HolderID,
				"lease_epoch", renewed.leaseEpoch,
				"leaseEpoch", renewed.leaseEpoch,
				"expires_at", renewed.expiresAt.UTC().Format(time.RFC3339Nano),
			)
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
	logger := r.logger()
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
					logger.Warn(
						"leader renew failed",
						"event", "leader_renew_failed",
						"stage", "renew",
						"outcome", "failed",
						"holder_id", r.cfg.HolderID,
						"lease_epoch", epoch,
						"leaseEpoch", epoch,
						"sql_error", err,
					)
				} else {
					logger.Warn(
						"leader renew failed",
						"event", "leader_renew_failed",
						"stage", "renew",
						"outcome", "failed",
						"holder_id", r.cfg.HolderID,
						"lease_epoch", epoch,
						"leaseEpoch", epoch,
					)
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
			logger.Debug(
				"leader renewed",
				"event", "leader_renewed",
				"stage", "renew",
				"outcome", "renewed",
				"holder_id", r.cfg.HolderID,
				"lease_epoch", renewed.leaseEpoch,
				"leaseEpoch", renewed.leaseEpoch,
				"expires_at", renewed.expiresAt.UTC().Format(time.RFC3339Nano),
			)
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
	logger := r.logger()
	r.manager.setFollower()
	r.setStatus(LeaseStatus{Mode: leaseModeFollower, HolderID: r.cfg.HolderID})
	if err != nil {
		logger.Warn(
			"leader lost",
			"event", "leader_lost",
			"stage", "loss",
			"outcome", "lost",
			"holder_id", r.cfg.HolderID,
			"sql_error", err,
		)
	} else {
		logger.Warn(
			"leader lost",
			"event", "leader_lost",
			"stage", "loss",
			"outcome", "lost",
			"holder_id", r.cfg.HolderID,
		)
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

func (r *LeaderRunner) logger() *slog.Logger {
	return slog.Default().With(
		"component", "submission-manager-leader",
		"commProfile", setulog.CommProfileWithinSetu,
		"operation", "leader_lease",
	)
}
