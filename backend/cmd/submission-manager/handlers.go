package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"gateway/submissionmanager"
)

type apiServer struct {
	manager *submissionmanager.Manager
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
	// Flow intent: check input, submit, maybe wait, return intent.
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}

	// Non-obvious constraint: waitSeconds only controls how long we wait; it must not change idempotency or storage.
	wait, err := parseWaitSeconds(r.URL.Query().Get("waitSeconds"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
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

	if wait > 0 {
		waited, ok, err := s.manager.WaitForIntent(r.Context(), stored.IntentID, wait)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error", nil)
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "not_found", "intent not found", map[string]string{"intentId": stored.IntentID})
			return
		}
		stored = waited
	}

	writeJSON(w, http.StatusOK, toIntentResponse(stored))
}

func (s *apiServer) handleGet(w http.ResponseWriter, r *http.Request) {
	// Flow intent: find intent or history and return it.
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
