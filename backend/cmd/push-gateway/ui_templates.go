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
		if _, err := os.Stat(filepath.Join(candidate, "overview.tmpl")); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("ui templates not found")
}

func loadUITemplates(uiDir string) (uiTemplates, error) {
	overview, err := template.ParseFiles(filepath.Join(uiDir, "nav.tmpl"), filepath.Join(uiDir, "overview.tmpl"))
	if err != nil {
		return uiTemplates{}, err
	}
	send, err := template.ParseFiles(filepath.Join(uiDir, "nav.tmpl"), filepath.Join(uiDir, "send.tmpl"))
	if err != nil {
		return uiTemplates{}, err
	}
	sendResult, err := template.ParseFiles(filepath.Join(uiDir, "send_result.tmpl"))
	if err != nil {
		return uiTemplates{}, err
	}
	metrics, err := template.ParseFiles(filepath.Join(uiDir, "nav.tmpl"), filepath.Join(uiDir, "metrics.tmpl"))
	if err != nil {
		return uiTemplates{}, err
	}
	return uiTemplates{
		overview:   overview,
		send:       send,
		sendResult: sendResult,
		metrics:    metrics,
	}, nil
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
	if _, err := io.WriteString(w, "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><link rel=\"stylesheet\" href=\"/ui/static/ui.css\"><title>"+title+"</title></head><body><div class=\"topbar\"><div class=\"topbar-brand\"><svg class=\"topbar-logo\" viewBox=\"0 0 48 24\" aria-hidden=\"true\" focusable=\"false\"><path d=\"M2 18c6-10 12-14 22-14s16 4 22 14\" fill=\"none\" stroke=\"currentColor\" stroke-width=\"2\" stroke-linecap=\"round\"/><path d=\"M8 18v-6M40 18v-6M16 18v-4M32 18v-4\" stroke=\"currentColor\" stroke-width=\"2\" stroke-linecap=\"round\"/><path d=\"M2 18h44\" stroke=\"currentColor\" stroke-width=\"2\" stroke-linecap=\"round\"/></svg><span class=\"topbar-title\">Setu</span></div></div><div id=\"ui-root\">"); err != nil {
		log.Printf("write shell start: %v", err)
		return
	}
	if _, err := w.Write(fragment); err != nil {
		log.Printf("write shell fragment: %v", err)
		return
	}
	if _, err := io.WriteString(w, "</div><script src=\"/ui/static/htmx.min.js\"></script><script src=\"/ui/static/json-enc.js\"></script></body></html>"); err != nil {
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
