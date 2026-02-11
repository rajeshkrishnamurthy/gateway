package main

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"gateway/deliverytracking"
)

type apiServer struct {
	webhookIngestor *deliverytracking.WebhookIngestor
	reader          *deliverytracking.Reader
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

func (s *apiServer) handleDeliveryWebhookIngestion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if s.webhookIngestor == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "service unavailable", nil)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body", nil)
		return
	}

	result, err := s.webhookIngestor.IngestProviderSignalWebhook(r.Context(), body)
	if err != nil {
		var invalidErr deliverytracking.InvalidWebhookPayloadError
		if errors.As(err, &invalidErr) {
			writeError(w, http.StatusBadRequest, "invalid_request", invalidErr.Error(), nil)
			return
		}
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "service unavailable", nil)
		return
	}

	writeJSON(w, http.StatusAccepted, toDeliveryWebhookIngestionResponse(result))
}

func (s *apiServer) handleDeliveryReadRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if s.reader == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "service unavailable", nil)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/intents/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "intentId is required", nil)
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) == 2 && parts[1] == "delivery" {
		s.handleGetCurrentDelivery(w, r, strings.TrimSpace(parts[0]))
		return
	}
	if len(parts) == 3 && parts[1] == "delivery" && parts[2] == "history" {
		s.handleGetDeliveryHistory(w, r, strings.TrimSpace(parts[0]))
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "not found", nil)
}

func (s *apiServer) handleGetCurrentDelivery(w http.ResponseWriter, r *http.Request, intentID string) {
	if intentID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "intentId is required", nil)
		return
	}
	view, found, err := s.reader.GetCurrentDelivery(r.Context(), intentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error", nil)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "intent not found", map[string]string{"intentId": intentID})
		return
	}
	writeJSON(w, http.StatusOK, toDeliveryCurrentResponse(view))
}

func (s *apiServer) handleGetDeliveryHistory(w http.ResponseWriter, r *http.Request, intentID string) {
	if intentID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "intentId is required", nil)
		return
	}
	view, found, err := s.reader.GetDeliveryHistory(r.Context(), intentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error", nil)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "intent not found", map[string]string{"intentId": intentID})
		return
	}
	writeJSON(w, http.StatusOK, toDeliveryHistoryResponse(view))
}
