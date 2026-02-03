package submissionmanager

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type leaseRow struct {
	holderID   string
	leaseEpoch int64
	expiresAt  time.Time
}

func (s *sqlStore) acquireLease(ctx context.Context, cfg LeaseConfig) (leaseRow, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	leaseName := strings.TrimSpace(cfg.LeaseName)
	holderID := strings.TrimSpace(cfg.HolderID)
	if leaseName == "" || holderID == "" {
		return leaseRow{}, false, errors.New("lease name and holder id are required")
	}
	durationMs := cfg.LeaseDuration.Milliseconds()

	row := s.db.QueryRowContext(
		ctx,
		`UPDATE dbo.submission_manager_leases
     SET holder_id = @p1,
         lease_epoch = lease_epoch + 1,
         acquired_at = SYSUTCDATETIME(),
         renewed_at = SYSUTCDATETIME(),
         expires_at = DATEADD(MILLISECOND, @p2, SYSUTCDATETIME())
     OUTPUT inserted.lease_epoch, inserted.expires_at
     WHERE lease_name = @p3 AND expires_at <= SYSUTCDATETIME()`,
		holderID,
		durationMs,
		leaseName,
	)
	var epoch int64
	var expiresAt time.Time
	if err := row.Scan(&epoch, &expiresAt); err == nil {
		return leaseRow{holderID: holderID, leaseEpoch: epoch, expiresAt: normalizeDBTime(expiresAt)}, true, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return leaseRow{}, false, err
	}

	row = s.db.QueryRowContext(
		ctx,
		`INSERT INTO dbo.submission_manager_leases (
      lease_name, holder_id, lease_epoch, acquired_at, renewed_at, expires_at
    ) OUTPUT inserted.lease_epoch, inserted.expires_at
    VALUES (
      @p1, @p2, 1, SYSUTCDATETIME(), SYSUTCDATETIME(), DATEADD(MILLISECOND, @p3, SYSUTCDATETIME())
    )`,
		leaseName,
		holderID,
		durationMs,
	)
	if err := row.Scan(&epoch, &expiresAt); err != nil {
		if isUniqueViolation(err) {
			return leaseRow{}, false, nil
		}
		return leaseRow{}, false, err
	}
	return leaseRow{holderID: holderID, leaseEpoch: epoch, expiresAt: normalizeDBTime(expiresAt)}, true, nil
}

func (s *sqlStore) renewLease(ctx context.Context, cfg LeaseConfig, epoch int64) (leaseRow, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	leaseName := strings.TrimSpace(cfg.LeaseName)
	holderID := strings.TrimSpace(cfg.HolderID)
	if leaseName == "" || holderID == "" {
		return leaseRow{}, false, errors.New("lease name and holder id are required")
	}
	durationMs := cfg.LeaseDuration.Milliseconds()

	row := s.db.QueryRowContext(
		ctx,
		`UPDATE dbo.submission_manager_leases
     SET renewed_at = SYSUTCDATETIME(),
         expires_at = DATEADD(MILLISECOND, @p1, SYSUTCDATETIME())
     OUTPUT inserted.lease_epoch, inserted.expires_at
     WHERE lease_name = @p2
       AND holder_id = @p3
       AND lease_epoch = @p4
       AND expires_at > SYSUTCDATETIME()`,
		durationMs,
		leaseName,
		holderID,
		epoch,
	)
	var newEpoch int64
	var expiresAt time.Time
	if err := row.Scan(&newEpoch, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return leaseRow{}, false, nil
		}
		return leaseRow{}, false, err
	}
	return leaseRow{holderID: holderID, leaseEpoch: newEpoch, expiresAt: normalizeDBTime(expiresAt)}, true, nil
}

func (s *sqlStore) readLease(ctx context.Context, leaseName string) (leaseRow, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	name := strings.TrimSpace(leaseName)
	if name == "" {
		return leaseRow{}, false, errors.New("lease name is required")
	}

	row := s.db.QueryRowContext(
		ctx,
		`SELECT holder_id, lease_epoch, expires_at
     FROM dbo.submission_manager_leases
     WHERE lease_name = @p1`,
		name,
	)
	var holderID string
	var epoch int64
	var expiresAt time.Time
	if err := row.Scan(&holderID, &epoch, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return leaseRow{}, false, nil
		}
		return leaseRow{}, false, err
	}
	return leaseRow{holderID: holderID, leaseEpoch: epoch, expiresAt: normalizeDBTime(expiresAt)}, true, nil
}
