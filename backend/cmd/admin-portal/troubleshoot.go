package main

import (
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (s *portalServer) handleSMSTroubleshoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.renderError(w, r, http.StatusMethodNotAllowed, "Method not allowed", "method not allowed", navSMS)
		return
	}
	view := troubleshootView{
		HistoryAction:  "/sms/ui/troubleshoot/history",
		HistoryEnabled: s.config.SubmissionManagerURL != "",
	}
	s.renderPage(w, r, s.templates.troubleshoot, "portal_troubleshoot.tmpl", view, navSMS)
}

func (s *portalServer) handleTroubleshoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.renderError(w, r, http.StatusMethodNotAllowed, "Method not allowed", "method not allowed", navTroubleshoot)
		return
	}
	view := troubleshootView{
		HistoryAction:  "/troubleshoot/history",
		HistoryEnabled: s.config.SubmissionManagerURL != "",
	}
	s.renderPage(w, r, s.templates.troubleshoot, "portal_troubleshoot.tmpl", view, navTroubleshoot)
}

func (s *portalServer) handlePushTroubleshoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.renderError(w, r, http.StatusMethodNotAllowed, "Method not allowed", "method not allowed", navPush)
		return
	}
	view := troubleshootView{
		HistoryAction:  "/push/ui/troubleshoot/history",
		HistoryEnabled: s.config.SubmissionManagerURL != "",
	}
	s.renderPage(w, r, s.templates.troubleshoot, "portal_troubleshoot.tmpl", view, navPush)
}

func (s *portalServer) handleSMSTroubleshootHistory(w http.ResponseWriter, r *http.Request) {
	s.handleManagerHistory(w, r, navSMS)
}

func (s *portalServer) handleTroubleshootHistory(w http.ResponseWriter, r *http.Request) {
	s.handleManagerHistory(w, r, navTroubleshoot)
}

func (s *portalServer) handlePushTroubleshootHistory(w http.ResponseWriter, r *http.Request) {
	s.handleManagerHistory(w, r, navPush)
}

func (s *portalServer) handleManagerHistory(w http.ResponseWriter, r *http.Request, active string) {
	if s.config.SubmissionManagerURL == "" {
		s.renderTroubleshootError(w, r, http.StatusNotFound, "SubmissionManager not configured", "submissionManagerUrl is empty in the portal config.", active)
		return
	}
	if r.Method != http.MethodPost {
		s.renderTroubleshootError(w, r, http.StatusMethodNotAllowed, "Method not allowed", "method not allowed", active)
		return
	}
	intentID := strings.TrimSpace(r.FormValue("intentId"))
	if intentID == "" {
		s.renderTroubleshootError(w, r, http.StatusBadRequest, "IntentId required", "intentId is required", active)
		return
	}
	form := url.Values{}
	form.Set("intentId", intentID)
	s.proxyTroubleshoot(w, r, active, s.config.SubmissionManagerURL, "/ui/history", form)
}

func (s *portalServer) proxyTroubleshoot(w http.ResponseWriter, r *http.Request, active string, baseURL string, path string, form url.Values) {
	remoteURL, err := buildTargetURL(baseURL, path, "", false)
	if err != nil {
		s.renderTroubleshootError(w, r, http.StatusBadGateway, "Invalid upstream URL", err.Error(), active)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, remoteURL, strings.NewReader(form.Encode()))
	if err != nil {
		s.renderTroubleshootError(w, r, http.StatusBadGateway, "Upstream request failed", err.Error(), active)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	resp, err := s.client.Do(req)
	if err != nil {
		s.renderTroubleshootError(w, r, http.StatusBadGateway, "Upstream request failed", err.Error(), active)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.renderTroubleshootError(w, r, http.StatusBadGateway, "Upstream response failed", err.Error(), active)
		return
	}
	if resp.StatusCode >= http.StatusBadRequest {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = "upstream error"
		}
		s.renderTroubleshootError(w, r, resp.StatusCode, "Upstream error", message, active)
		return
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "text/html; charset=utf-8"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(body); err != nil {
		return
	}
}

func (s *portalServer) renderTroubleshootError(w http.ResponseWriter, r *http.Request, status int, title, message, active string) {
	if !isHTMX(r) {
		s.renderError(w, r, status, title, message, active)
		return
	}
	fragment, err := executeTemplate(s.templates.errView, "portal_error.tmpl", errorView{Title: title, Message: message})
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Render failed", err.Error(), active)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(fragment)
}
