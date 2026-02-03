package main

import (
	"html/template"
	"log"
	"net/http"
	"time"
)

func (u *uiServer) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	showNav := !isEmbed(r)
	view := overviewView{
		ConsoleTitle:    u.consoleTitle,
		SendNavLabel:    u.sendNavLabel,
		MetricsURL:      u.metricsURL,
		ShowNav:         showNav,
		GatewayName:     u.gatewayName,
		Version:         u.version,
		ProviderName:    u.providerName,
		ProviderTimeout: u.providerTimeout.String(),
		Uptime:          formatUptime(time.Since(u.startTime)),
	}
	u.renderPage(w, r, u.templates.overview, "overview.tmpl", view)
}

func (u *uiServer) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	showNav := !isEmbed(r)
	view := sendView{
		SendTitle:    u.sendTitle,
		SendEndpoint: u.sendEndpoint,
		SendNavLabel: u.sendNavLabel,
		MetricsURL:   u.metricsURL,
		ShowNav:      showNav,
		IsPush:       u.isPush,
	}
	u.renderPage(w, r, u.templates.send, "send.tmpl", view)
}

func (u *uiServer) handleUIMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	view := buildMetricsView(u.metricsRegistry)
	view.SendNavLabel = u.sendNavLabel
	view.MetricsURL = u.metricsURL
	view.ShowNav = !isEmbed(r)
	if u.isPush {
		view.Title = "Push Gateway Dashboard"
	} else {
		view.Title = "SMS Gateway Dashboard"
	}
	u.renderPage(w, r, u.templates.metrics, "metrics.tmpl", view)
}

func (u *uiServer) renderPage(w http.ResponseWriter, r *http.Request, tmpl *template.Template, name string, data any) {
	if isHTMX(r) {
		renderFragment(w, tmpl, name, data)
		return
	}
	fragment, err := executeTemplate(tmpl, name, data)
	if err != nil {
		log.Printf("render page: %v", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	renderShell(w, fragment, u.consoleTitle)
}
