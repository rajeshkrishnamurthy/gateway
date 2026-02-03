package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/microsoft/go-mssqldb"

	"gateway/submission"
	"gateway/submissionmanager"
)

var (
	addrFlag                 = flag.String("addr", ":8082", "HTTP listen address")
	registryPathFlag         = flag.String("registry", "conf/submission/submission_targets.json", "SubmissionTarget registry path")
	mssqlHostFlag            = flag.String("sql-host", envOrDefault("MSSQL_HOST", "localhost"), "SQL Server host")
	mssqlPortFlag            = flag.String("sql-port", envOrDefault("MSSQL_PORT", "1433"), "SQL Server port")
	mssqlUserFlag            = flag.String("sql-user", envOrDefault("MSSQL_USER", "sa"), "SQL Server user")
	mssqlPasswordFlag        = flag.String("sql-password", envOrDefault("MSSQL_SA_PASSWORD", ""), "SQL Server password")
	mssqlDBFlag              = flag.String("sql-db", envOrDefault("MSSQL_DATABASE", "setu"), "SQL Server database")
	mssqlEncryptFlag         = flag.String("sql-encrypt", envOrDefault("MSSQL_ENCRYPT", "disable"), "SQL Server encrypt setting")
	leaseDurationFlag        = flag.String("lease-duration", envOrDefault("SM_LEASE_DURATION", "60s"), "Leader lease duration (example: 60s)")
	leaseRenewIntervalFlag   = flag.String("lease-renew-interval", envOrDefault("SM_RENEW_INTERVAL", "20s"), "Leader lease renew interval (example: 20s)")
	leaseAcquireIntervalFlag = flag.String("lease-acquire-interval", envOrDefault("SM_ACQUIRE_INTERVAL", "30s"), "Leader lease acquire interval (example: 30s)")
	scheduleRefreshFlag      = flag.String("schedule-refresh-interval", envOrDefault("SM_SCHEDULE_REFRESH_INTERVAL", "1s"), "Schedule refresh interval (example: 1s)")
	leaseNameFlag            = flag.String("lease-name", envOrDefault("SM_LEASE_NAME", "submission-manager-executor"), "Leader lease name")
	holderIDFlag             = flag.String("holder-id", envOrDefault("SM_HOLDER_ID", ""), "Leader holder id (defaults to hostname-pid-rand)")
)

func main() {
	flag.Parse()

	leaseDuration, err := parseDurationFlag("lease-duration", *leaseDurationFlag)
	if err != nil {
		log.Fatalf("parse lease-duration: %v", err)
	}
	renewInterval, err := parseDurationFlag("lease-renew-interval", *leaseRenewIntervalFlag)
	if err != nil {
		log.Fatalf("parse lease-renew-interval: %v", err)
	}
	acquireInterval, err := parseDurationFlag("lease-acquire-interval", *leaseAcquireIntervalFlag)
	if err != nil {
		log.Fatalf("parse lease-acquire-interval: %v", err)
	}
	scheduleRefreshInterval, err := parseDurationFlag("schedule-refresh-interval", *scheduleRefreshFlag)
	if err != nil {
		log.Fatalf("parse schedule-refresh-interval: %v", err)
	}
	if renewInterval >= leaseDuration {
		log.Fatalf("lease-renew-interval must be less than lease-duration")
	}

	registry, err := submission.LoadRegistry(*registryPathFlag)
	if err != nil {
		log.Fatalf("load registry: %v", err)
	}

	dsn, err := buildSQLServerDSN(*mssqlHostFlag, *mssqlPortFlag, *mssqlUserFlag, *mssqlPasswordFlag, *mssqlDBFlag, *mssqlEncryptFlag)
	if err != nil {
		log.Fatalf("build SQL Server DSN: %v", err)
	}

	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		log.Fatalf("open SQL Server: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := db.PingContext(ctx); err != nil {
		cancel()
		log.Fatalf("ping SQL Server: %v", err)
	}
	cancel()

	client := &http.Client{}
	exec := newGatewayExecutor(client)
	manager, err := submissionmanager.NewManager(registry, exec, submissionmanager.Clock{}, db)
	if err != nil {
		log.Fatalf("construct manager: %v", err)
	}
	metrics := submissionmanager.NewMetrics()
	manager.SetMetrics(metrics)
	manager.SetWebhookSender(newWebhookSender(client))

	holderID := strings.TrimSpace(*holderIDFlag)
	if holderID == "" {
		holderID = defaultHolderID()
	}
	leaseName := strings.TrimSpace(*leaseNameFlag)
	if leaseName == "" {
		log.Fatalf("lease-name is required")
	}

	leaseCfg := submissionmanager.LeaseConfig{
		LeaseName:               leaseName,
		HolderID:                holderID,
		LeaseDuration:           leaseDuration,
		RenewInterval:           renewInterval,
		AcquireInterval:         acquireInterval,
		ScheduleRefreshInterval: scheduleRefreshInterval,
	}
	runner := submissionmanager.NewLeaderRunnerFromManager(manager, leaseCfg)

	var uiServer *managerUIServer
	uiDir, err := findUIDir()
	if err != nil {
		log.Printf("ui disabled: %v", err)
	} else if templates, err := loadManagerTemplates(uiDir); err != nil {
		log.Printf("ui disabled: %v", err)
	} else {
		uiServer = &managerUIServer{templates: templates, manager: manager}
	}

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	go runner.Run(ctx)

	server := &apiServer{manager: manager}
	mux := newMux(server, uiServer, metrics, runner.Status)

	httpServer := &http.Server{
		Addr:    *addrFlag,
		Handler: mux,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-stop
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		_ = httpServer.Shutdown(shutdownCtx)
		cancel()
	}()

	log.Printf("submission-manager listening on %s", *addrFlag)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("http server: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func buildSQLServerDSN(host, port, user, password, database, encrypt string) (string, error) {
	if password == "" {
		return "", fmt.Errorf("sql password is required")
	}
	uri := &url.URL{
		Scheme: "sqlserver",
		User:   url.UserPassword(user, password),
		Host:   fmt.Sprintf("%s:%s", host, port),
	}
	query := url.Values{}
	query.Set("database", database)
	query.Set("encrypt", encrypt)
	uri.RawQuery = query.Encode()
	return uri.String(), nil
}

func parseDurationFlag(name, value string) (time.Duration, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, fmt.Errorf("%s is required", name)
	}
	duration, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, err
	}
	if duration <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", name)
	}
	return duration, nil
}

func defaultHolderID() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = "unknown"
	}
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return fmt.Sprintf("%s-%d-%d", host, os.Getpid(), time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%d-%x", host, os.Getpid(), suffix)
}
