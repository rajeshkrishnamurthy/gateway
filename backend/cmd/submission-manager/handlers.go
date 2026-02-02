package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"gateway/submissionmanager"
)

type apiServer struct {
	manager *submissionmanager.Manager
}

type submitRequest struct {
	IntentID         string          `json:"intentId"`
	SubmissionTarget string          `json:"submissionTarget"`
	Payload          json.RawMessage `json:"payload"`
}

type intentResponse struct {
	IntentID         string `json:"intentId"`
	SubmissionTarget string `json:"submissionTarget"`
	CreatedAt        string `json:"createdAt"`
	Status           string `json:"status"`
	CompletedAt      string `json:"completedAt,omitempty"`
	RejectedReason   string `json:"rejectedReason,omitempty"`
	ExhaustedReason  string `json:"exhaustedReason,omitempty"`
}

type intentHistoryResponse struct {
	Intent   intentResponse    `json:"intent"`
	Attempts []attemptResponse `json:"attempts"`
}

type attemptResponse struct {
	AttemptNumber int    `json:"attemptNumber"`
	StartedAt     string `json:"startedAt,omitempty"`
	FinishedAt    string `json:"finishedAt,omitempty"`
	OutcomeStatus string `json:"outcomeStatus,omitempty"`
	OutcomeReason string `json:"outcomeReason,omitempty"`
	Error         string `json:"error,omitempty"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

func handleMetrics(metrics *submissionmanager.Metrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if metrics == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		metrics.WritePrometheus(w)
	}
}

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

func (s *apiServer) handleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}

	dec := json.NewDecoder(r.Body)
	var req submitRequest
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body", nil)
		return
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body", nil)
		return
	}

	intent := submissionmanager.Intent{
		IntentID:         strings.TrimSpace(req.IntentID),
		SubmissionTarget: strings.TrimSpace(req.SubmissionTarget),
		Payload:          req.Payload,
	}
	if intent.IntentID == "" || intent.SubmissionTarget == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "intentId and submissionTarget are required", nil)
		return
	}

	stored, err := s.manager.SubmitIntent(r.Context(), intent)
	if err != nil {
		var conflict submissionmanager.IdempotencyConflictError
		if errors.As(err, &conflict) {
			writeError(w, http.StatusConflict, "idempotency_conflict", "intentId already exists with different payload", map[string]string{
				"intentId":       conflict.IntentID,
				"existingTarget": conflict.ExistingTarget,
				"incomingTarget": conflict.IncomingTarget,
			})
			return
		}
		var unknown submissionmanager.UnknownSubmissionTargetError
		if errors.As(err, &unknown) {
			writeError(w, http.StatusBadRequest, "invalid_request", "unknown submissionTarget", map[string]string{
				"submissionTarget": unknown.SubmissionTarget,
			})
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error", nil)
		return
	}

	writeJSON(w, http.StatusOK, toIntentResponse(stored))
}

func (s *apiServer) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/intents/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "intentId is required", nil)
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) == 2 && parts[1] == "history" {
		intentID := strings.TrimSpace(parts[0])
		if intentID == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "intentId is required", nil)
			return
		}
		s.handleHistory(w, r, intentID)
		return
	}
	if len(parts) != 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "intentId is required", nil)
		return
	}

	intentID := strings.TrimSpace(parts[0])
	if intentID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "intentId is required", nil)
		return
	}

	intent, ok := s.manager.GetIntent(intentID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "intent not found", map[string]string{"intentId": intentID})
		return
	}

	writeJSON(w, http.StatusOK, toIntentResponse(intent))
}

func (s *apiServer) handleHistory(w http.ResponseWriter, r *http.Request, intentID string) {
	intent, ok := s.manager.GetIntent(intentID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "intent not found", map[string]string{"intentId": intentID})
		return
	}
	response := intentHistoryResponse{
		Intent:   toIntentResponse(intent),
		Attempts: toAttemptResponses(intent.Attempts),
	}
	writeJSON(w, http.StatusOK, response)
}

func toIntentResponse(intent submissionmanager.Intent) intentResponse {
	completedAt := ""
	if intent.Status == submissionmanager.IntentAccepted ||
		intent.Status == submissionmanager.IntentRejected ||
		intent.Status == submissionmanager.IntentExhausted {
		if !intent.CompletedAt.IsZero() {
			completedAt = intent.CompletedAt.UTC().Format(timeFormat)
		}
	}

	rejectedReason := ""
	if intent.Status == submissionmanager.IntentRejected {
		rejectedReason = intent.FinalOutcome.Reason
	}

	exhaustedReason := ""
	if intent.Status == submissionmanager.IntentExhausted {
		exhaustedReason = intent.ExhaustedReason
	}

	return intentResponse{
		IntentID:         intent.IntentID,
		SubmissionTarget: intent.SubmissionTarget,
		CreatedAt:        intent.CreatedAt.UTC().Format(timeFormat),
		Status:           string(intent.Status),
		CompletedAt:      completedAt,
		RejectedReason:   rejectedReason,
		ExhaustedReason:  exhaustedReason,
	}
}

func toAttemptResponses(attempts []submissionmanager.Attempt) []attemptResponse {
	out := make([]attemptResponse, 0, len(attempts))
	for _, attempt := range attempts {
		out = append(out, attemptResponse{
			AttemptNumber: attempt.Number,
			StartedAt:     formatAttemptTime(attempt.StartedAt),
			FinishedAt:    formatAttemptTime(attempt.FinishedAt),
			OutcomeStatus: attempt.GatewayOutcome.Status,
			OutcomeReason: attempt.GatewayOutcome.Reason,
			Error:         attempt.Error,
		})
	}
	return out
}

func formatAttemptTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(timeFormat)
}

const timeFormat = time.RFC3339Nano

func writeError(w http.ResponseWriter, status int, code, message string, details map[string]string) {
	writeJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message, Details: details}})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
