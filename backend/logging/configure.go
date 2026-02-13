package logging

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

const (
	// PreviousSchemaVersion is supported during schema migration windows.
	PreviousSchemaVersion = "setu.log.v0"
	// CurrentSchemaVersion is the active log schema version.
	CurrentSchemaVersion = "setu.log.v1"
)

const (
	CommProfileOutsideToSetu  = "outside_to_setu"
	CommProfileSetuToSetu     = "setu_to_setu"
	CommProfileWithinSetu     = "within_setu_service"
	BoundaryDirectionIngress  = "ingress"
	BoundaryDirectionEgress   = "egress"
	defaultServiceName        = "unknown-service"
	defaultInstanceIDHostname = "unknown-host"
)

// Configure installs a process-wide slog logger with Setu base attributes.
func Configure(service string) *slog.Logger {
	service = strings.TrimSpace(service)
	if service == "" {
		service = defaultServiceName
	}

	opts := &slog.HandlerOptions{
		Level: parseLevel(strings.TrimSpace(os.Getenv("SETU_LOG_LEVEL"))),
	}

	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SETU_LOG_FORMAT"))) {
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	logger := slog.New(handler).With(
		"logSchemaVersion", CurrentSchemaVersion,
		"service", service,
		"instance", instanceID(),
	)
	slog.SetDefault(logger)
	return logger
}

func instanceID() string {
	if value := strings.TrimSpace(os.Getenv("SETU_INSTANCE_ID")); value != "" {
		return value
	}
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = defaultInstanceIDHostname
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

func parseLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
