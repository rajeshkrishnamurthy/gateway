package main

import (
	"io"
	"net/http"
)

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func handleReadyz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *portalServer) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.renderError(w, r, http.StatusMethodNotAllowed, "Method not allowed", "method not allowed", navOverview)
		return
	}
	view := overviewView{
		Title:    resolveTitle(s.config.Title),
		Consoles: buildConsoleViews(s.config),
	}
	s.renderPage(w, r, s.templates.overview, "portal_overview.tmpl", view, navOverview)
}

func (s *portalServer) handleHAProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.renderError(w, r, http.StatusMethodNotAllowed, "Method not allowed", "method not allowed", navHAProxy)
		return
	}
	if s.config.HAProxyStatsURL == "" {
		s.renderError(w, r, http.StatusNotFound, "HAProxy not configured", "haproxyStatsUrl is empty in the portal config.", navHAProxy)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, s.config.HAProxyStatsURL, nil)
	if err != nil {
		s.renderError(w, r, http.StatusBadGateway, "HAProxy request failed", err.Error(), navHAProxy)
		return
	}
	resp, err := s.client.Do(req)
	if err != nil {
		s.renderError(w, r, http.StatusBadGateway, "HAProxy request failed", err.Error(), navHAProxy)
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.renderError(w, r, http.StatusBadGateway, "HAProxy response failed", err.Error(), navHAProxy)
		return
	}
	frontends, backends, err := parseHAProxyCSV(body)
	view := haproxyView{Frontends: frontends, Backends: backends}
	if err != nil {
		view.Error = err.Error()
	}
	s.renderPage(w, r, s.templates.haproxy, "portal_haproxy.tmpl", view, navHAProxy)
}

func (s *portalServer) handleSMSUI(w http.ResponseWriter, r *http.Request) {
	s.proxyUI(w, r, s.config.SMSGatewayURL, "/sms", navSMS, false)
}

func (s *portalServer) handlePushUI(w http.ResponseWriter, r *http.Request) {
	s.proxyUI(w, r, s.config.PushGatewayURL, "/push", navPush, false)
}

func (s *portalServer) handleCommandCenterUI(w http.ResponseWriter, r *http.Request) {
	s.proxyUI(w, r, s.config.CommandCenterURL, "/command-center", navCommandCenter, true)
}

func (s *portalServer) handleSMSAPI(w http.ResponseWriter, r *http.Request) {
	if s.useSubmissionManagerSMS() {
		s.handleSMSSubmission(w, r)
		return
	}
	s.proxyAPI(w, r, s.config.SMSGatewayURL)
}

func (s *portalServer) handlePushAPI(w http.ResponseWriter, r *http.Request) {
	if s.useSubmissionManagerPush() {
		s.handlePushSubmission(w, r)
		return
	}
	s.proxyAPI(w, r, s.config.PushGatewayURL)
}

func buildConsoleViews(cfg fileConfig) []consoleView {
	var consoles []consoleView
	if cfg.SMSGatewayURL != "" {
		consoles = append(consoles, consoleView{Label: "SMS Gateway", Meta: cfg.SMSGatewayURL, Href: "/sms/ui"})
	}
	if cfg.PushGatewayURL != "" {
		consoles = append(consoles, consoleView{Label: "Push Gateway", Meta: cfg.PushGatewayURL, Href: "/push/ui"})
	}
	if cfg.CommandCenterURL != "" {
		consoles = append(consoles, consoleView{Label: "Command Center", Meta: cfg.CommandCenterURL, Href: "/command-center/ui"})
	}
	if cfg.HAProxyStatsURL != "" {
		consoles = append(consoles, consoleView{Label: "HAProxy", Meta: cfg.HAProxyStatsURL, Href: "/haproxy"})
	}
	return consoles
}
