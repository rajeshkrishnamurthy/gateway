package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"gateway/deliverytracking"
)

type apiServer struct {
	webhookIngestor *deliverytracking.WebhookIngestor
	reader          *deliverytracking.Reader
	metrics         *deliverytracking.Metrics
}

type deliveryReadRoute struct {
	endpoint string
	intentID string
	history  bool
	matched  bool
}

func handleMetrics(metrics *deliverytracking.Metrics) http.HandlerFunc {
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

func (s *apiServer) handleDeliveryWebhookIngestion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if s.webhookIngestor == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "service unavailable", nil)
		s.observeProcessingFailure(deliverytracking.ProcessingStageWebhookIngestion, deliverytracking.ProcessingFailureReasonDependencyUnavailable)
		s.logDeliveryProcessingFailure(
			deliverytracking.ProcessingStageWebhookIngestion,
			deliverytracking.ProcessingFailureReasonDependencyUnavailable,
			"unknown",
			"/v1/delivery/provider-signal-webhook",
			"unknown",
		)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body", nil)
		s.observeProcessingFailure(deliverytracking.ProcessingStageWebhookIngestion, deliverytracking.ProcessingFailureReasonInvalidPayload)
		log.Printf(
			"event=delivery_webhook_ingestion_rejected source=%q intentId=%q submissionTarget=%q deliveryTrackingMode=%q correlationResult=%q reason=%q",
			"provider signal webhook",
			"unknown",
			"unknown",
			"unknown",
			"unknown",
			deliverytracking.ProcessingFailureReasonInvalidPayload,
		)
		return
	}

	result, err := s.webhookIngestor.IngestProviderSignalWebhook(r.Context(), body)
	if err != nil {
		var invalidErr deliverytracking.InvalidWebhookPayloadError
		if errors.As(err, &invalidErr) {
			writeError(w, http.StatusBadRequest, "invalid_request", invalidErr.Error(), nil)
			intentID := extractIntentIDForLog(body)
			s.observeProcessingFailure(deliverytracking.ProcessingStageWebhookIngestion, deliverytracking.ProcessingFailureReasonInvalidPayload)
			log.Printf(
				"event=delivery_webhook_ingestion_rejected source=%q intentId=%q submissionTarget=%q deliveryTrackingMode=%q correlationResult=%q reason=%q",
				"provider signal webhook",
				intentID,
				"unknown",
				"unknown",
				"unknown",
				deliverytracking.ProcessingFailureReasonInvalidPayload,
			)
			return
		}
		intentID := extractIntentIDForLog(body)
		stage := deliverytracking.ProcessingFailureStageFromError(err, deliverytracking.ProcessingStageWebhookIngestion)
		reason := deliverytracking.MapProcessingFailureReason(err)
		s.observeProcessingFailure(stage, reason)
		s.logDeliveryProcessingFailure(stage, reason, intentID, "/v1/delivery/provider-signal-webhook", "unknown")
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "service unavailable", nil)
		return
	}

	writeJSON(w, http.StatusAccepted, toDeliveryWebhookIngestionResponse(result))
}

func (s *apiServer) handleDeliveryReadRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		route := parseDeliveryReadRoute(r.URL.Path)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		if route.matched {
			s.observeReadAPIRequest(route.endpoint, http.StatusMethodNotAllowed)
		}
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/intents/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "intentId is required", nil)
		return
	}

	route := parseDeliveryReadRoute(r.URL.Path)
	if !route.matched {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	if s.reader == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "service unavailable", nil)
		s.observeReadAPIRequest(route.endpoint, http.StatusServiceUnavailable)
		s.observeProcessingFailure(deliverytracking.ProcessingStageDeliveryReadAPI, deliverytracking.ProcessingFailureReasonDependencyUnavailable)
		s.logDeliveryProcessingFailure(
			deliverytracking.ProcessingStageDeliveryReadAPI,
			deliverytracking.ProcessingFailureReasonDependencyUnavailable,
			route.intentID,
			route.endpoint,
			"unknown",
		)
		return
	}

	var statusCode int
	if route.history {
		statusCode = s.handleGetDeliveryHistory(w, r, route.intentID)
	} else {
		statusCode = s.handleGetCurrentDelivery(w, r, route.intentID)
	}
	s.observeReadAPIRequest(route.endpoint, statusCode)
}

func (s *apiServer) handleGetCurrentDelivery(w http.ResponseWriter, r *http.Request, intentID string) int {
	if intentID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "intentId is required", nil)
		return http.StatusBadRequest
	}
	view, found, err := s.reader.GetCurrentDelivery(r.Context(), intentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error", nil)
		reason := deliverytracking.MapProcessingFailureReason(err)
		s.observeProcessingFailure(deliverytracking.ProcessingStageDeliveryReadAPI, reason)
		s.logDeliveryProcessingFailure(deliverytracking.ProcessingStageDeliveryReadAPI, reason, intentID, deliverytracking.DeliveryReadEndpointCurrent, "unknown")
		return http.StatusInternalServerError
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "intent not found", map[string]string{"intentId": intentID})
		return http.StatusNotFound
	}
	writeJSON(w, http.StatusOK, toDeliveryCurrentResponse(view))
	return http.StatusOK
}

func (s *apiServer) handleGetDeliveryHistory(w http.ResponseWriter, r *http.Request, intentID string) int {
	if intentID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "intentId is required", nil)
		return http.StatusBadRequest
	}
	view, found, err := s.reader.GetDeliveryHistory(r.Context(), intentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error", nil)
		reason := deliverytracking.MapProcessingFailureReason(err)
		s.observeProcessingFailure(deliverytracking.ProcessingStageDeliveryReadAPI, reason)
		s.logDeliveryProcessingFailure(deliverytracking.ProcessingStageDeliveryReadAPI, reason, intentID, deliverytracking.DeliveryReadEndpointHistory, "unknown")
		return http.StatusInternalServerError
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "intent not found", map[string]string{"intentId": intentID})
		return http.StatusNotFound
	}
	writeJSON(w, http.StatusOK, toDeliveryHistoryResponse(view))
	return http.StatusOK
}

func (s *apiServer) observeReadAPIRequest(endpoint string, statusCode int) {
	if s == nil || s.metrics == nil {
		return
	}
	s.metrics.ObserveReadAPIRequest(endpoint, statusCode)
}

func (s *apiServer) observeProcessingFailure(stage, reason string) {
	if s == nil || s.metrics == nil {
		return
	}
	s.metrics.ObserveProcessingFailure(stage, reason)
}

func (s *apiServer) logDeliveryProcessingFailure(stage, reason, intentID, endpoint, correlationResult string) {
	log.Printf(
		"event=delivery_processing_failure stage=%q reason=%q source=%q endpoint=%q intentId=%q submissionTarget=%q deliveryTrackingMode=%q correlationResult=%q",
		stage,
		reason,
		"provider signal webhook",
		endpoint,
		intentIDOrUnknown(intentID),
		"unknown",
		"unknown",
		intentIDOrUnknown(correlationResult),
	)
}

func parseDeliveryReadRoute(path string) deliveryReadRoute {
	trimmed := strings.TrimPrefix(path, "/v1/intents/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return deliveryReadRoute{}
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 2 && parts[1] == "delivery" {
		return deliveryReadRoute{
			endpoint: deliverytracking.DeliveryReadEndpointCurrent,
			intentID: strings.TrimSpace(parts[0]),
			history:  false,
			matched:  true,
		}
	}
	if len(parts) == 3 && parts[1] == "delivery" && parts[2] == "history" {
		return deliveryReadRoute{
			endpoint: deliverytracking.DeliveryReadEndpointHistory,
			intentID: strings.TrimSpace(parts[0]),
			history:  true,
			matched:  true,
		}
	}
	return deliveryReadRoute{}
}

func extractIntentIDForLog(body []byte) string {
	var payload struct {
		IntentID string `json:"intentId"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "unknown"
	}
	return intentIDOrUnknown(payload.IntentID)
}

func intentIDOrUnknown(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}
