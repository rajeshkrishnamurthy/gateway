package main

import "net/http"

func (s *portalServer) handleDashboards(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.renderError(w, r, http.StatusMethodNotAllowed, "Method not allowed", "method not allowed", navDashboards)
		return
	}
	view := dashboardsView{
		SubmissionURL:               s.submissionManagerDashboardPath(),
		SMSGatewayURL:               s.gatewayDashboardURL(s.config.SMSGatewayURL, "/sms/ui/metrics"),
		PushGatewayURL:              s.gatewayDashboardURL(s.config.PushGatewayURL, "/push/ui/metrics"),
		DeliveryTrackingURL:         s.deliveryTrackingDashboardPath(),
		DeliveryTrackingActivityURL: s.deliveryTrackingActivityDashboardPath(),
	}
	s.renderPage(w, r, s.templates.dashboards, "portal_dashboards.tmpl", view, navDashboards)
}

func (s *portalServer) hasAnyDashboard() bool {
	return s.submissionManagerDashboardPath() != "" ||
		s.gatewayDashboardURL(s.config.SMSGatewayURL, "/sms/ui/metrics") != "" ||
		s.gatewayDashboardURL(s.config.PushGatewayURL, "/push/ui/metrics") != "" ||
		s.deliveryTrackingDashboardPath() != "" ||
		s.deliveryTrackingActivityDashboardPath() != ""
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

func (s *portalServer) deliveryTrackingDashboardPath() string {
	if s.config.DeliveryTrackingDashboardURL == "" {
		return ""
	}
	return "/dashboards/delivery-tracking"
}

func (s *portalServer) deliveryTrackingActivityDashboardPath() string {
	if s.config.DeliveryTrackingActivityDashboardURL == "" {
		return ""
	}
	return "/dashboards/delivery-tracking-activity"
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

func (s *portalServer) handleDeliveryTrackingDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.renderError(w, r, http.StatusMethodNotAllowed, "Method not allowed", "method not allowed", navDashboards)
		return
	}
	if s.config.DeliveryTrackingDashboardURL == "" {
		s.renderError(w, r, http.StatusNotFound, "Dashboard not configured", "deliveryTrackingDashboardUrl is empty in the portal config.", navDashboards)
		return
	}
	view := dashboardEmbedView{
		Title:        "Delivery Tracking Health Dashboard",
		Description:  "",
		DashboardURL: s.config.DeliveryTrackingDashboardURL,
	}
	s.renderPage(w, r, s.templates.dashboardEmbed, "portal_dashboard_embed.tmpl", view, navDashboards)
}

func (s *portalServer) handleDeliveryTrackingActivityDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.renderError(w, r, http.StatusMethodNotAllowed, "Method not allowed", "method not allowed", navDashboards)
		return
	}
	if s.config.DeliveryTrackingActivityDashboardURL == "" {
		s.renderError(w, r, http.StatusNotFound, "Dashboard not configured", "deliveryTrackingActivityDashboardUrl is empty in the portal config.", navDashboards)
		return
	}
	view := dashboardEmbedView{
		Title:        "Delivery Tracking Activity Dashboard",
		Description:  "",
		DashboardURL: s.config.DeliveryTrackingActivityDashboardURL,
	}
	s.renderPage(w, r, s.templates.dashboardEmbed, "portal_dashboard_embed.tmpl", view, navDashboards)
}
