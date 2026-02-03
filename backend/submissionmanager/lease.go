package submissionmanager

import "time"

const (
	leaseModeLeader   = "leader"
	leaseModeFollower = "follower"
)

// LeaseConfig defines the SQL lease timing and identity parameters.
type LeaseConfig struct {
	LeaseName               string
	HolderID                string
	LeaseDuration           time.Duration
	RenewInterval           time.Duration
	AcquireInterval         time.Duration
	ScheduleRefreshInterval time.Duration
}

// LeaseStatus captures the local view of leadership for readiness.
type LeaseStatus struct {
	Mode       string
	HolderID   string
	LeaseEpoch int64
	ExpiresAt  time.Time
}

// LeaseFence guards executor-side writes with the current lease token.
type LeaseFence struct {
	LeaseName  string
	HolderID   string
	LeaseEpoch int64
}
