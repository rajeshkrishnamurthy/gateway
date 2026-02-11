package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (s *portalServer) deliveryRoutesEnabled() bool {
	return strings.TrimSpace(s.config.DeliveryTrackingURL) != ""
}

func (s *portalServer) deliveryTestProducerNavEnabled() bool {
	return s.deliveryRoutesEnabled() &&
		s.config.DeliveryTestProducerEnabled &&
		s.config.PortalEnvironment != "prod"
}

func (s *portalServer) handleDeliveryCurrentUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.renderError(w, r, http.StatusMethodNotAllowed, "Method not allowed", "method not allowed", navDeliveryCurrent)
		return
	}
	if !s.deliveryRoutesEnabled() {
		s.renderError(w, r, http.StatusNotFound, "Delivery tracking not configured", "deliveryTrackingUrl is empty in the portal config.", navDeliveryCurrent)
		return
	}
	view := deliveryReadView{
		Title:       "Delivery Current",
		Description: "Read current delivery state for one intent.",
		FormAction:  "/delivery/current",
		ResultHint:  "Submit an intentId to proxy GET /v1/intents/{intentId}/delivery.",
	}
	s.renderPage(w, r, s.templates.deliveryRead, "portal_delivery_read.tmpl", view, navDeliveryCurrent)
}

func (s *portalServer) handleDeliveryHistoryUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.renderError(w, r, http.StatusMethodNotAllowed, "Method not allowed", "method not allowed", navDeliveryHistory)
		return
	}
	if !s.deliveryRoutesEnabled() {
		s.renderError(w, r, http.StatusNotFound, "Delivery tracking not configured", "deliveryTrackingUrl is empty in the portal config.", navDeliveryHistory)
		return
	}
	view := deliveryReadView{
		Title:       "Delivery History",
		Description: "Read delivery history for one intent.",
		FormAction:  "/delivery/history",
		ResultHint:  "Submit an intentId to proxy GET /v1/intents/{intentId}/delivery/history.",
	}
	s.renderPage(w, r, s.templates.deliveryRead, "portal_delivery_read.tmpl", view, navDeliveryHistory)
}

func (s *portalServer) handleDeliveryCurrent(w http.ResponseWriter, r *http.Request) {
	s.handleDeliveryRead(w, r, false)
}

func (s *portalServer) handleDeliveryHistory(w http.ResponseWriter, r *http.Request) {
	s.handleDeliveryRead(w, r, true)
}

func (s *portalServer) handleDeliveryRead(w http.ResponseWriter, r *http.Request, history bool) {
	if !s.deliveryRoutesEnabled() {
		http.Error(w, "delivery tracking not configured", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	intentID := strings.TrimSpace(r.URL.Query().Get("intentId"))
	if intentID == "" {
		http.Error(w, "intentId is required", http.StatusBadRequest)
		return
	}

	path := "/v1/intents/" + url.PathEscape(intentID) + "/delivery"
	if history {
		path += "/history"
	}
	s.proxyDeliveryRequest(w, r, http.MethodGet, path, nil)
}

func (s *portalServer) handleDeliveryTestProducerUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.renderError(w, r, http.StatusMethodNotAllowed, "Method not allowed", "method not allowed", navDeliveryTestProducer)
		return
	}
	if status, message, ok := s.deliveryTestProducerAccess(); !ok {
		s.renderError(w, r, status, "Delivery test producer unavailable", message, navDeliveryTestProducer)
		return
	}
	s.renderPage(
		w,
		r,
		s.templates.deliveryProducer,
		"portal_delivery_test_producer.tmpl",
		deliveryProducerView{FormAction: "/delivery/test-producer"},
		navDeliveryTestProducer,
	)
}

func (s *portalServer) handleDeliveryTestProducer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if status, message, ok := s.deliveryTestProducerAccess(); !ok {
		http.Error(w, message, status)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form body", http.StatusBadRequest)
		return
	}

	intentID := strings.TrimSpace(r.FormValue("intentId"))
	providerDeliverySignal := strings.TrimSpace(r.FormValue("providerDeliverySignal"))
	if intentID == "" || providerDeliverySignal == "" {
		http.Error(w, "intentId and providerDeliverySignal are required", http.StatusBadRequest)
		return
	}

	payload := map[string]any{
		"intentId":               intentID,
		"providerDeliverySignal": providerDeliverySignal,
	}
	if providerObservedAt := strings.TrimSpace(r.FormValue("providerObservedAt")); providerObservedAt != "" {
		payload["providerObservedAt"] = providerObservedAt
	}
	if providerEventID := strings.TrimSpace(r.FormValue("providerEventId")); providerEventID != "" {
		payload["providerEventId"] = providerEventID
	}
	if providerMetadata := strings.TrimSpace(r.FormValue("providerMetadata")); providerMetadata != "" {
		var metadata map[string]any
		if err := json.Unmarshal([]byte(providerMetadata), &metadata); err != nil {
			http.Error(w, "providerMetadata must be a JSON object", http.StatusBadRequest)
			return
		}
		payload["providerMetadata"] = metadata
	}

	body, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "failed to encode request body", http.StatusInternalServerError)
		return
	}
	s.proxyDeliveryRequest(w, r, http.MethodPost, "/v1/delivery/provider-signal-webhook", body)
}

func (s *portalServer) deliveryTestProducerAccess() (int, string, bool) {
	if s.config.PortalEnvironment == "prod" {
		return http.StatusForbidden, "delivery test producer is disabled in production", false
	}
	if !s.config.DeliveryTestProducerEnabled {
		return http.StatusNotFound, "delivery test producer is disabled", false
	}
	if !s.deliveryRoutesEnabled() {
		return http.StatusNotFound, "delivery tracking not configured", false
	}
	return 0, "", true
}

func (s *portalServer) proxyDeliveryRequest(w http.ResponseWriter, r *http.Request, method, path string, body []byte) {
	targetURL, err := buildTargetURL(s.config.DeliveryTrackingURL, path, "", false)
	if err != nil {
		http.Error(w, "invalid upstream URL", http.StatusBadGateway)
		return
	}

	var requestBody io.Reader
	if body != nil {
		requestBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(r.Context(), method, targetURL, requestBody)
	if err != nil {
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	copyHeader(req.Header, r.Header, []string{"Accept"})

	resp, err := s.client.Do(req)
	if err != nil {
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
