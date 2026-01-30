package main

import (
	"html/template"
	"io"
	"log"
	"net/http"
)

func (s *portalServer) renderPage(w http.ResponseWriter, r *http.Request, tmpl *template.Template, name string, data any, active string) {
	fragment, err := executeTemplate(tmpl, name, data)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Render failed", err.Error(), active)
		return
	}
	if isHTMX(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := w.Write(fragment); err != nil {
			log.Printf("write fragment: %v", err)
		}
		return
	}
	s.renderShell(w, fragment, active, http.StatusOK)
}

func (s *portalServer) renderShell(w http.ResponseWriter, fragment []byte, active string, status int) {
	topbar, err := executeTemplate(s.templates.topbar, "portal_topbar.tmpl", topbarView{
		Active:            active,
		ShowSMS:           s.config.SMSGatewayURL != "",
		ShowPush:          s.config.PushGatewayURL != "",
		ShowHAProxy:       s.config.HAProxyStatsURL != "",
		ShowCommandCenter: s.config.CommandCenterURL != "",
	})
	if err != nil {
		log.Printf("render topbar: %v", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}

	if status <= 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := io.WriteString(w, "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><link rel=\"stylesheet\" href=\"/ui/static/ui.css\"><title>"+template.HTMLEscapeString(resolveTitle(s.config.Title))+"</title></head><body>"); err != nil {
		log.Printf("write shell start: %v", err)
		return
	}
	if _, err := w.Write(topbar); err != nil {
		log.Printf("write topbar: %v", err)
		return
	}
	if _, err := io.WriteString(w, "<div id=\"ui-root\" class=\"portal-root\">"); err != nil {
		log.Printf("write shell root: %v", err)
		return
	}
	if _, err := w.Write(fragment); err != nil {
		log.Printf("write shell fragment: %v", err)
		return
	}
	if _, err := io.WriteString(w, "</div><script src=\"/ui/static/htmx.min.js\"></script><script src=\"/ui/static/json-enc.js\"></script><script src=\"/ui/static/theme.js\"></script></body></html>"); err != nil {
		log.Printf("write shell end: %v", err)
	}
}

func (s *portalServer) renderError(w http.ResponseWriter, r *http.Request, status int, title, message, active string) {
	fragment, err := executeTemplate(s.templates.errView, "portal_error.tmpl", errorView{Title: title, Message: message})
	if err != nil {
		http.Error(w, message, status)
		return
	}
	if isHTMX(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		if _, err := w.Write(fragment); err != nil {
			log.Printf("write error fragment: %v", err)
		}
		return
	}
	s.renderShell(w, fragment, active, status)
}
