package main

import (
	"gateway/metrics"
	"html/template"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

const defaultGrafanaDashboardURL = "http://localhost:3000/d/gateway-overview-push"
const defaultGrafanaRefresh = "5s"

type uiTemplates struct {
	overview   *template.Template
	send       *template.Template
	sendResult *template.Template
	metrics    *template.Template
}

type uiServer struct {
	templates       uiTemplates
	staticDir       string
	consoleTitle    string
	sendTitle       string
	sendNavLabel    string
	sendEndpoint    string
	metricsURL      string
	isPush          bool
	gatewayName     string
	version         string
	providerName    string
	providerTimeout time.Duration
	startTime       time.Time
	metricsRegistry *metrics.Registry
}

type overviewView struct {
	ConsoleTitle    string
	SendNavLabel    string
	MetricsURL      string
	ShowNav         bool
	GatewayName     string
	Version         string
	ProviderName    string
	ProviderTimeout string
	Uptime          string
}

type sendView struct {
	SendTitle    string
	SendEndpoint string
	SendNavLabel string
	MetricsURL   string
	ShowNav      bool
	IsPush       bool
}

func newUIServer(providerName string, providerTimeout time.Duration, grafanaDashboardURL string, metricsRegistry *metrics.Registry, startTime time.Time) (*uiServer, error) {
	uiDir, err := findUIDir()
	if err != nil {
		return nil, err
	}
	templates, err := loadUITemplates(uiDir)
	if err != nil {
		return nil, err
	}
	metricsURL := normalizeGrafanaURL(grafanaDashboardURL, defaultGrafanaDashboardURL, defaultGrafanaRefresh)
	return &uiServer{
		templates:       templates,
		staticDir:       filepath.Join(uiDir, "static"),
		consoleTitle:    "Push Gateway Console",
		sendTitle:       "Send Test Push",
		sendNavLabel:    "Send Test Push",
		sendEndpoint:    "/push/send",
		metricsURL:      metricsURL,
		isPush:          true,
		gatewayName:     "push-gateway",
		version:         version,
		providerName:    providerName,
		providerTimeout: providerTimeout,
		startTime:       startTime,
		metricsRegistry: metricsRegistry,
	}, nil
}

func normalizeGrafanaURL(raw, fallback, refresh string) string {
	metricsURL := strings.TrimSpace(raw)
	if metricsURL == "" {
		metricsURL = fallback
	}
	return ensureGrafanaRefresh(metricsURL, refresh)
}

func ensureGrafanaRefresh(raw, refresh string) string {
	if strings.TrimSpace(refresh) == "" {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	query := parsed.Query()
	if query.Get("refresh") == "" {
		query.Set("refresh", refresh)
		parsed.RawQuery = query.Encode()
	}
	return parsed.String()
}

func formatUptime(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	return d.Truncate(time.Second).String()
}
