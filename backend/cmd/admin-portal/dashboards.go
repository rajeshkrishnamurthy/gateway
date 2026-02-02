package main

import "net/http"

func (s *portalServer) handleDashboards(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.renderError(w, r, http.StatusMethodNotAllowed, "Method not allowed", "method not allowed", navDashboards)
		return
	}
	view := dashboardsView{
		SubmissionURL:  s.submissionManagerDashboardPath(),
		SMSGatewayURL:  s.gatewayDashboardURL(s.config.SMSGatewayURL, "/sms/ui/metrics"),
		PushGatewayURL: s.gatewayDashboardURL(s.config.PushGatewayURL, "/push/ui/metrics"),
	}
	s.renderPage(w, r, s.templates.dashboards, "portal_dashboards.tmpl", view, navDashboards)
}

func (s *portalServer) gatewayDashboardURL(baseURL, fallback string) string {
	if baseURL == "" {
		return ""
	}
	return fallback
}

func (s *portalServer) submissionManagerDashboardPath() string {
	if s.config.SubmissionManagerDashboardURL == "" {
		return ""
	}
	return "/dashboards/submission-manager"
}

func (s *portalServer) handleSubmissionManagerDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.renderError(w, r, http.StatusMethodNotAllowed, "Method not allowed", "method not allowed", navDashboards)
		return
	}
	if s.config.SubmissionManagerDashboardURL == "" {
		s.renderError(w, r, http.StatusNotFound, "Dashboard not configured", "submissionManagerDashboardUrl is empty in the portal config.", navDashboards)
		return
	}
	view := dashboardEmbedView{
		Title:        "Submission Manager Dashboard",
		Description:  "",
		DashboardURL: s.config.SubmissionManagerDashboardURL,
	}
	s.renderPage(w, r, s.templates.dashboardEmbed, "portal_dashboard_embed.tmpl", view, navDashboards)
}
