package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

type uiTemplates struct {
	overview *template.Template
	services *template.Template
	config   *template.Template
}

func findUIDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	candidates := []string{
		filepath.Join(wd, "..", "ui"),
		filepath.Join(wd, "..", "..", "ui"),
		filepath.Join(wd, "..", "..", "..", "ui"),
		filepath.Join(wd, "ui"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(candidate, "health_overview.tmpl")); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("ui templates not found")
}

func loadUITemplates(uiDir string) (uiTemplates, error) {
	overview, err := template.ParseFiles(filepath.Join(uiDir, "health_overview.tmpl"), filepath.Join(uiDir, "health_services.tmpl"))
	if err != nil {
		return uiTemplates{}, err
	}
	services, err := template.ParseFiles(filepath.Join(uiDir, "health_services.tmpl"))
	if err != nil {
		return uiTemplates{}, err
	}
	config, err := template.ParseFiles(filepath.Join(uiDir, "health_config.tmpl"))
	if err != nil {
		return uiTemplates{}, err
	}
	return uiTemplates{overview: overview, services: services, config: config}, nil
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
	renderShell(w, fragment, u.title)
}

func renderFragment(w http.ResponseWriter, tmpl *template.Template, name string, data any) {
	fragment, err := executeTemplate(tmpl, name, data)
	if err != nil {
		log.Printf("render fragment: %v", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(fragment); err != nil {
		log.Printf("write fragment: %v", err)
	}
}

func renderShell(w http.ResponseWriter, fragment []byte, title string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><link rel="stylesheet" href="/static/ui.css"><title>%s</title></head><body><div class="topbar"><div class="topbar-brand"><svg class="topbar-logo" viewBox="0 0 48 24" aria-hidden="true" focusable="false"><path d="M2 18c6-10 12-14 22-14s16 4 22 14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/><path d="M8 18v-6M40 18v-6M16 18v-4M32 18v-4" stroke="currentColor" stroke-width="2" stroke-linecap="round"/><path d="M2 18h44" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg><span class="topbar-title">Setu</span></div></div><div id="ui-root">`, title); err != nil {
		log.Printf("write shell start: %v", err)
		return
	}
	if _, err := w.Write(fragment); err != nil {
		log.Printf("write shell fragment: %v", err)
		return
	}
	if _, err := io.WriteString(w, `</div><script src="/static/htmx.min.js"></script><script src="/static/theme.js"></script></body></html>`); err != nil {
		log.Printf("write shell end: %v", err)
	}
}

func executeTemplate(tmpl *template.Template, name string, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
