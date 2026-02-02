package main

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gateway/submissionmanager"
)

type managerTemplates struct {
	historyResults *template.Template
}

type managerUIServer struct {
	templates managerTemplates
	manager   *submissionmanager.Manager
}

type managerHistoryView struct {
	IntentID         string
	SubmissionTarget string
	Status           string
	CreatedAt        string
	CompletedAt      string
	RejectedReason   string
	ExhaustedReason  string
	Attempts         []managerAttemptView
}

type managerAttemptView struct {
	Number        int
	StartedAt     string
	FinishedAt    string
	OutcomeStatus string
	OutcomeReason string
	Error         string
}

func loadManagerTemplates(uiDir string) (managerTemplates, error) {
	historyResults, err := template.ParseFiles(filepath.Join(uiDir, "manager_history_results.tmpl"))
	if err != nil {
		return managerTemplates{}, err
	}
	return managerTemplates{
		historyResults: historyResults,
	}, nil
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
		if _, err := os.Stat(filepath.Join(candidate, "manager_history_results.tmpl")); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("ui templates not found")
}

func (u *managerUIServer) handleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	intentID := strings.TrimSpace(r.FormValue("intentId"))
	if intentID == "" {
		http.Error(w, "intentId is required", http.StatusBadRequest)
		return
	}
	if u.manager == nil {
		http.Error(w, "manager not configured", http.StatusInternalServerError)
		return
	}
	intent, ok := u.manager.GetIntent(intentID)
	if !ok {
		http.Error(w, "intent not found", http.StatusNotFound)
		return
	}
	view := buildHistoryView(intent)
	u.renderFragment(w, u.templates.historyResults, "manager_history_results.tmpl", view)
}

func (u *managerUIServer) renderFragment(w http.ResponseWriter, tmpl *template.Template, name string, data any) {
	fragment, err := executeTemplate(tmpl, name, data)
	if err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(fragment); err != nil {
		return
	}
}

func executeTemplate(tmpl *template.Template, name string, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildHistoryView(intent submissionmanager.Intent) managerHistoryView {
	view := managerHistoryView{
		IntentID:         intent.IntentID,
		SubmissionTarget: intent.SubmissionTarget,
		Status:           string(intent.Status),
		CreatedAt:        formatTime(intent.CreatedAt),
	}
	if !intent.CompletedAt.IsZero() {
		view.CompletedAt = formatTime(intent.CompletedAt)
	}
	if intent.Status == submissionmanager.IntentRejected {
		view.RejectedReason = intent.FinalOutcome.Reason
	}
	if intent.Status == submissionmanager.IntentExhausted {
		view.ExhaustedReason = intent.ExhaustedReason
	}
	for _, attempt := range intent.Attempts {
		view.Attempts = append(view.Attempts, managerAttemptView{
			Number:        attempt.Number,
			StartedAt:     formatTime(attempt.StartedAt),
			FinishedAt:    formatTime(attempt.FinishedAt),
			OutcomeStatus: attempt.GatewayOutcome.Status,
			OutcomeReason: attempt.GatewayOutcome.Reason,
			Error:         attempt.Error,
		})
	}
	return view
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02 15:04:05 MST")
}
