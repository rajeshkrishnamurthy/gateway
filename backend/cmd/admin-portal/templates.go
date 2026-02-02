package main

import (
	"bytes"
	"html/template"
	"path/filepath"
)

func loadPortalTemplates(uiDir string) (portalTemplates, error) {
	topbar, err := template.ParseFiles(filepath.Join(uiDir, "portal_topbar.tmpl"))
	if err != nil {
		return portalTemplates{}, err
	}
	overview, err := template.ParseFiles(filepath.Join(uiDir, "portal_overview.tmpl"))
	if err != nil {
		return portalTemplates{}, err
	}
	haproxy, err := template.ParseFiles(filepath.Join(uiDir, "portal_haproxy.tmpl"))
	if err != nil {
		return portalTemplates{}, err
	}
	errView, err := template.ParseFiles(filepath.Join(uiDir, "portal_error.tmpl"))
	if err != nil {
		return portalTemplates{}, err
	}
	troubleshoot, err := template.ParseFiles(filepath.Join(uiDir, "portal_troubleshoot.tmpl"))
	if err != nil {
		return portalTemplates{}, err
	}
	dashboards, err := template.ParseFiles(filepath.Join(uiDir, "portal_dashboards.tmpl"))
	if err != nil {
		return portalTemplates{}, err
	}
	dashboardEmbed, err := template.ParseFiles(filepath.Join(uiDir, "portal_dashboard_embed.tmpl"))
	if err != nil {
		return portalTemplates{}, err
	}
	submissionResult, err := template.ParseFiles(filepath.Join(uiDir, "submission_result.tmpl"))
	if err != nil {
		return portalTemplates{}, err
	}
	return portalTemplates{
		topbar:           topbar,
		overview:         overview,
		haproxy:          haproxy,
		errView:          errView,
		troubleshoot:     troubleshoot,
		dashboards:       dashboards,
		dashboardEmbed:   dashboardEmbed,
		submissionResult: submissionResult,
	}, nil
}

func executeTemplate(tmpl *template.Template, name string, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
