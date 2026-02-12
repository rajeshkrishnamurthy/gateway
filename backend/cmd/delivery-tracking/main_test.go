package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/microsoft/go-mssqldb"

	"gateway/deliverytracking"
	"gateway/submission"
)

func TestDeliveryRuntimeWebhookInvalidPayloadMappedTo400(t *testing.T) {
	server, db := newTestServer(t)
	mux := newMux(server)

	req := httptest.NewRequest(http.MethodPost, "/v1/delivery/provider-signal-webhook", strings.NewReader(`{"intentId":"intent-1"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%q", rr.Code, rr.Body.String())
	}
	var resp errorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if resp.Error.Code != "invalid_request" {
		t.Fatalf("expected invalid_request code, got %q", resp.Error.Code)
	}
	if countWebhookIngestionRows(t, db, "intent-1") != 0 {
		t.Fatalf("expected no ingestion rows for invalid payload")
	}
}

func TestDeliveryRuntimeWebhookMalformedObservedAtFallbackAccepted(t *testing.T) {
	server, _ := newTestServer(t)
	mux := newMux(server)

	req := httptest.NewRequest(http.MethodPost, "/v1/delivery/provider-signal-webhook", strings.NewReader(`{
  "intentId":"intent-fallback-http",
  "providerDeliverySignal":"in_progress",
  "providerObservedAt":"bad-timestamp"
}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%q", rr.Code, rr.Body.String())
	}
	var resp deliveryWebhookIngestionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ProviderObservedAt != "" {
		t.Fatalf("expected providerObservedAt omitted for malformed timestamp, got %q", resp.ProviderObservedAt)
	}
	if resp.EffectiveAt == "" || resp.ReceivedAt == "" || resp.EffectiveAt != resp.ReceivedAt {
		t.Fatalf("expected effectiveAt fallback to receivedAt, got effectiveAt=%q receivedAt=%q", resp.EffectiveAt, resp.ReceivedAt)
	}
}

func TestDeliveryRuntimeWebhookAcceptedAppliesCoreAndReadVisibility(t *testing.T) {
	server, db := newTestServer(t)
	mux := newMux(server)
	now := time.Now().UTC()
	insertIntent(t, db, "intent-webhook-e2e", submission.DeliveryTrackingModeOn, 300, now.Add(-time.Hour))

	req := httptest.NewRequest(http.MethodPost, "/v1/delivery/provider-signal-webhook", strings.NewReader(`{
  "intentId":"intent-webhook-e2e",
  "providerDeliverySignal":"in_progress",
  "providerEventId":"evt-webhook-e2e"
}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%q", rr.Code, rr.Body.String())
	}

	if countWebhookIngestionRows(t, db, "intent-webhook-e2e") != 1 {
		t.Fatalf("expected one persisted ingress-audit row")
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/intents/intent-webhook-e2e/delivery", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for current delivery, got %d body=%q", rr.Code, rr.Body.String())
	}
	var current deliveryCurrentResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &current); err != nil {
		t.Fatalf("decode current response: %v", err)
	}
	if current.DeliveryTrackingMode != string(submission.DeliveryTrackingModeOn) {
		t.Fatalf("expected mode on, got %q", current.DeliveryTrackingMode)
	}
	if current.DeliveryStatus != string(deliverytracking.DeliveryStatusInProgress) {
		t.Fatalf("expected deliveryStatus in_progress, got %q", current.DeliveryStatus)
	}
	if current.DeliveryFreshness != string(deliverytracking.DeliveryFreshnessFresh) {
		t.Fatalf("expected deliveryFreshness fresh, got %q", current.DeliveryFreshness)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/intents/intent-webhook-e2e/delivery/history", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for delivery history, got %d body=%q", rr.Code, rr.Body.String())
	}
	var history deliveryHistoryResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &history); err != nil {
		t.Fatalf("decode history response: %v", err)
	}
	if len(history.Entries) != 1 {
		t.Fatalf("expected one history entry, got %d", len(history.Entries))
	}
	if history.Entries[0].ProviderDeliverySignal != string(deliverytracking.DeliverySignalInProgress) {
		t.Fatalf("expected history signal in_progress, got %q", history.Entries[0].ProviderDeliverySignal)
	}
}

func TestDeliveryRuntimeWebhookUnmatchedIntentAcceptedAsNoOp(t *testing.T) {
	server, db := newTestServer(t)
	mux := newMux(server)

	req := httptest.NewRequest(http.MethodPost, "/v1/delivery/provider-signal-webhook", strings.NewReader(`{
  "intentId":"intent-unmatched-webhook",
  "providerDeliverySignal":"terminal_success"
}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for unmatched signal, got %d body=%q", rr.Code, rr.Body.String())
	}

	if countWebhookIngestionRows(t, db, "intent-unmatched-webhook") != 1 {
		t.Fatalf("expected ingress-audit row for unmatched signal")
	}
	snapshot := loadDeliveryReadSnapshot(t, db, "intent-unmatched-webhook")
	if snapshot.stateCount != 0 {
		t.Fatalf("expected unmatched signal to keep state rows at 0, got %d", snapshot.stateCount)
	}
	if snapshot.historyCount != 0 {
		t.Fatalf("expected unmatched signal to keep history rows at 0, got %d", snapshot.historyCount)
	}
}

func TestDeliveryRuntimeWebhookTransientFailureMappedTo503AndIngressAuditPersists(t *testing.T) {
	server, db := newTestServer(t)
	mux := newMux(server)
	insertIntent(t, db, "intent-503", submission.DeliveryTrackingModeOn, 300, time.Now().UTC().Add(-time.Hour))

	if _, err := db.ExecContext(context.Background(), `DROP TABLE dbo.intent_delivery_history`); err != nil {
		t.Fatalf("drop history table: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/delivery/provider-signal-webhook", strings.NewReader(`{
  "intentId":"intent-503",
  "providerDeliverySignal":"terminal_success"
}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%q", rr.Code, rr.Body.String())
	}
	var resp errorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if resp.Error.Code != "service_unavailable" {
		t.Fatalf("expected service_unavailable code, got %q", resp.Error.Code)
	}
	if countWebhookIngestionRows(t, db, "intent-503") != 1 {
		t.Fatalf("expected ingress-audit record to remain persisted after downstream failure")
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRR := httptest.NewRecorder()
	mux.ServeHTTP(metricsRR, metricsReq)
	assertContainsRuntimeMetric(t, metricsRR.Body.String(), `delivery_processing_failures_total{stage="core_processing",reason="storage_unavailable"} 1`)
}

func TestDeliveryRuntimeMetricsEndpointExposesDeliveryMetrics(t *testing.T) {
	server, _ := newTestServer(t)
	mux := newMux(server)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "text/plain; version=0.0.4" {
		t.Fatalf("expected metrics content type, got %q", got)
	}
	body := rr.Body.String()
	requiredMetrics := []string{
		"delivery_provider_signals_total",
		"delivery_status_current_count",
		"delivery_status_transitions_total",
		"delivery_freshness_transitions_total",
		"delivery_ignored_signals_total",
		"delivery_processing_failures_total",
		"delivery_read_api_requests_total",
	}
	for _, metricName := range requiredMetrics {
		if !strings.Contains(body, metricName) {
			t.Fatalf("expected metrics output to include %q, body=%q", metricName, body)
		}
	}
}

func TestDeliveryRuntimeMetricsEndpointReturns404WhenRegistryUnavailable(t *testing.T) {
	server, _ := newTestServer(t)
	server.metrics = nil
	mux := newMux(server)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%q", rr.Code, rr.Body.String())
	}
}

func TestDeliveryRuntimeWebhookInvalidPayloadObservability(t *testing.T) {
	server, _ := newTestServer(t)
	mux := newMux(server)

	var logBuf bytes.Buffer
	restoreLogs := captureRuntimeLogs(&logBuf)
	defer restoreLogs()

	req := httptest.NewRequest(http.MethodPost, "/v1/delivery/provider-signal-webhook", strings.NewReader(`{"intentId":"intent-obsv-http"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%q", rr.Code, rr.Body.String())
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRR := httptest.NewRecorder()
	mux.ServeHTTP(metricsRR, metricsReq)
	metricsBody := metricsRR.Body.String()

	if !strings.Contains(metricsBody, `delivery_processing_failures_total{stage="webhook_ingestion",reason="invalid_payload"} 1`) {
		t.Fatalf("expected webhook invalid payload failure metric, body=%q", metricsBody)
	}

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, `event=delivery_webhook_ingestion_rejected`) {
		t.Fatalf("expected webhook rejection log, logs=%q", logOutput)
	}
	if strings.Contains(logOutput, "providerDeliverySignal") {
		t.Fatalf("expected log to avoid payload/body fields, logs=%q", logOutput)
	}
}

func TestDeliveryRuntimeReadAPIObservabilityMetricsAndFailureMapping(t *testing.T) {
	server, db := newTestServer(t)
	mux := newMux(server)
	insertIntent(t, db, "intent-read-obsv", submission.DeliveryTrackingModeOn, 120, time.Now().UTC().Add(-time.Minute))

	req := httptest.NewRequest(http.MethodGet, "/v1/intents/missing/delivery", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%q", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/intents/missing/delivery/history", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 history, got %d body=%q", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/intents/intent-read-obsv/delivery", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%q", rr.Code, rr.Body.String())
	}

	if _, err := db.ExecContext(
		context.Background(),
		`UPDATE dbo.submission_intents
     SET delivery_tracking_mode = @p1
     WHERE intent_id = @p2`,
		"x",
		"intent-read-obsv",
	); err != nil {
		t.Fatalf("set invalid delivery tracking mode: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/intents/intent-read-obsv/delivery", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%q", rr.Code, rr.Body.String())
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRR := httptest.NewRecorder()
	mux.ServeHTTP(metricsRR, metricsReq)
	metricsBody := metricsRR.Body.String()

	assertContainsRuntimeMetric(t, metricsBody, `delivery_read_api_requests_total{endpoint="/v1/intents/{intentId}/delivery",code="404"} 1`)
	assertContainsRuntimeMetric(t, metricsBody, `delivery_read_api_requests_total{endpoint="/v1/intents/{intentId}/delivery",code="405"} 1`)
	assertContainsRuntimeMetric(t, metricsBody, `delivery_read_api_requests_total{endpoint="/v1/intents/{intentId}/delivery",code="500"} 1`)
	assertContainsRuntimeMetric(t, metricsBody, `delivery_read_api_requests_total{endpoint="/v1/intents/{intentId}/delivery/history",code="404"} 1`)
	assertContainsRuntimeMetric(t, metricsBody, `delivery_processing_failures_total{stage="delivery_read_api",reason="internal_error"} 1`)
}

func TestDeliveryRuntimeGetCurrentModeOff(t *testing.T) {
	server, db := newTestServer(t)
	mux := newMux(server)
	insertIntent(t, db, "intent-off", submission.DeliveryTrackingModeOff, 0, time.Now().UTC().Add(-time.Minute))

	req := httptest.NewRequest(http.MethodGet, "/v1/intents/intent-off/delivery", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rr.Code, rr.Body.String())
	}
	var resp deliveryCurrentResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.DeliveryTrackingMode != "off" {
		t.Fatalf("expected mode off, got %q", resp.DeliveryTrackingMode)
	}
	if resp.DeliveryStatus != "" || resp.DeliveryFreshness != "" {
		t.Fatalf("expected no delivery fields for mode off, got status=%q freshness=%q", resp.DeliveryStatus, resp.DeliveryFreshness)
	}
}

func TestDeliveryRuntimeGetHistoryModeOff(t *testing.T) {
	server, db := newTestServer(t)
	mux := newMux(server)
	insertIntent(t, db, "intent-off-history", submission.DeliveryTrackingModeOff, 0, time.Now().UTC().Add(-time.Minute))

	req := httptest.NewRequest(http.MethodGet, "/v1/intents/intent-off-history/delivery/history", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rr.Code, rr.Body.String())
	}
	var resp deliveryHistoryResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.DeliveryTrackingMode != "off" {
		t.Fatalf("expected mode off, got %q", resp.DeliveryTrackingMode)
	}
	if len(resp.Entries) != 0 {
		t.Fatalf("expected no history entries for mode off, got %d", len(resp.Entries))
	}
}

func TestDeliveryRuntimeGetCurrentSparseTrackedFallback(t *testing.T) {
	server, db := newTestServer(t)
	mux := newMux(server)
	now := time.Now().UTC()
	insertIntent(t, db, "intent-fallback-fresh", submission.DeliveryTrackingModeOn, 3600, now.Add(-time.Minute))
	insertIntent(t, db, "intent-fallback-stale", submission.DeliveryTrackingModeOn, 60, now.Add(-2*time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/v1/intents/intent-fallback-fresh/delivery", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for fresh fallback, got %d body=%q", rr.Code, rr.Body.String())
	}
	var freshResp deliveryCurrentResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &freshResp); err != nil {
		t.Fatalf("decode fresh fallback response: %v", err)
	}
	if freshResp.DeliveryTrackingMode != "on" {
		t.Fatalf("expected mode on, got %q", freshResp.DeliveryTrackingMode)
	}
	if freshResp.DeliveryStatus != string(deliverytracking.DeliveryStatusUnknown) {
		t.Fatalf("expected fallback status unknown, got %q", freshResp.DeliveryStatus)
	}
	if freshResp.DeliveryFreshness != string(deliverytracking.DeliveryFreshnessFresh) {
		t.Fatalf("expected fallback freshness fresh, got %q", freshResp.DeliveryFreshness)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/intents/intent-fallback-stale/delivery", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for stale fallback, got %d body=%q", rr.Code, rr.Body.String())
	}
	var staleResp deliveryCurrentResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &staleResp); err != nil {
		t.Fatalf("decode stale fallback response: %v", err)
	}
	if staleResp.DeliveryTrackingMode != "on" {
		t.Fatalf("expected mode on, got %q", staleResp.DeliveryTrackingMode)
	}
	if staleResp.DeliveryStatus != string(deliverytracking.DeliveryStatusUnknown) {
		t.Fatalf("expected fallback status unknown, got %q", staleResp.DeliveryStatus)
	}
	if staleResp.DeliveryFreshness != string(deliverytracking.DeliveryFreshnessStale) {
		t.Fatalf("expected fallback freshness stale, got %q", staleResp.DeliveryFreshness)
	}
}

func TestDeliveryRuntimeGetHistoryOrdered(t *testing.T) {
	server, db := newTestServer(t)
	mux := newMux(server)
	insertIntent(t, db, "intent-history", submission.DeliveryTrackingModeOn, 120, time.Now().UTC().Add(-time.Hour))

	base := time.Now().UTC().Add(-10 * time.Minute)
	insertHistory(t, db, "intent-history", "src-1", deliverytracking.DeliverySignalTerminalFailure, base.Add(2*time.Minute), false)
	insertHistory(t, db, "intent-history", "src-2", deliverytracking.DeliverySignalInProgress, base, false)
	insertHistory(t, db, "intent-history", "src-3", deliverytracking.DeliverySignalTerminalSuccess, base.Add(time.Minute), true)

	req := httptest.NewRequest(http.MethodGet, "/v1/intents/intent-history/delivery/history", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rr.Code, rr.Body.String())
	}
	var resp deliveryHistoryResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.DeliveryTrackingMode != "on" {
		t.Fatalf("expected mode on, got %q", resp.DeliveryTrackingMode)
	}
	if len(resp.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(resp.Entries))
	}
	if resp.Entries[0].ProviderDeliverySignal != string(deliverytracking.DeliverySignalTerminalFailure) {
		t.Fatalf("expected first signal terminal_failure, got %q", resp.Entries[0].ProviderDeliverySignal)
	}
	if resp.Entries[1].ProviderDeliverySignal != string(deliverytracking.DeliverySignalInProgress) {
		t.Fatalf("expected second signal in_progress, got %q", resp.Entries[1].ProviderDeliverySignal)
	}
	if resp.Entries[2].ProviderDeliverySignal != string(deliverytracking.DeliverySignalTerminalSuccess) {
		t.Fatalf("expected third signal terminal_success, got %q", resp.Entries[2].ProviderDeliverySignal)
	}
	if !resp.Entries[2].LateObservation {
		t.Fatalf("expected third entry lateObservation=true")
	}
}

func TestDeliveryRuntimeUnknownIntentNotFound(t *testing.T) {
	server, _ := newTestServer(t)
	mux := newMux(server)

	req := httptest.NewRequest(http.MethodGet, "/v1/intents/missing/delivery", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%q", rr.Code, rr.Body.String())
	}
}

func TestDeliveryRuntimeUnknownIntentHistoryNotFound(t *testing.T) {
	server, _ := newTestServer(t)
	mux := newMux(server)

	req := httptest.NewRequest(http.MethodGet, "/v1/intents/missing/delivery/history", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%q", rr.Code, rr.Body.String())
	}
}

func TestDeliveryRuntimeCrossInstanceEquivalentCurrentResponses(t *testing.T) {
	db := newTestDB(t)
	insertIntent(t, db, "intent-same", submission.DeliveryTrackingModeOn, 120, time.Now().UTC().Add(-time.Hour))
	insertState(t, db, "intent-same", deliverytracking.DeliveryStatusInProgress, deliverytracking.DeliveryFreshnessFresh, 120)

	serverA := newServerFromDB(t, db)
	serverB := newServerFromDB(t, db)
	muxA := newMux(serverA)
	muxB := newMux(serverB)

	reqA := httptest.NewRequest(http.MethodGet, "/v1/intents/intent-same/delivery", nil)
	rrA := httptest.NewRecorder()
	muxA.ServeHTTP(rrA, reqA)

	reqB := httptest.NewRequest(http.MethodGet, "/v1/intents/intent-same/delivery", nil)
	rrB := httptest.NewRecorder()
	muxB.ServeHTTP(rrB, reqB)

	if rrA.Code != http.StatusOK || rrB.Code != http.StatusOK {
		t.Fatalf("expected both instances to return 200, got A=%d B=%d", rrA.Code, rrB.Code)
	}
	if rrA.Body.String() != rrB.Body.String() {
		t.Fatalf("expected equivalent responses across instances, got A=%q B=%q", rrA.Body.String(), rrB.Body.String())
	}
}

func TestDeliveryRuntimeCrossInstanceEquivalentHistoryResponses(t *testing.T) {
	db := newTestDB(t)
	insertIntent(t, db, "intent-same-history", submission.DeliveryTrackingModeOn, 120, time.Now().UTC().Add(-time.Hour))
	base := time.Now().UTC().Add(-10 * time.Minute)
	insertHistory(t, db, "intent-same-history", "src-1", deliverytracking.DeliverySignalInProgress, base, false)
	insertHistory(t, db, "intent-same-history", "src-2", deliverytracking.DeliverySignalTerminalSuccess, base.Add(time.Minute), true)

	serverA := newServerFromDB(t, db)
	serverB := newServerFromDB(t, db)
	muxA := newMux(serverA)
	muxB := newMux(serverB)

	reqA := httptest.NewRequest(http.MethodGet, "/v1/intents/intent-same-history/delivery/history", nil)
	rrA := httptest.NewRecorder()
	muxA.ServeHTTP(rrA, reqA)

	reqB := httptest.NewRequest(http.MethodGet, "/v1/intents/intent-same-history/delivery/history", nil)
	rrB := httptest.NewRecorder()
	muxB.ServeHTTP(rrB, reqB)

	if rrA.Code != http.StatusOK || rrB.Code != http.StatusOK {
		t.Fatalf("expected both instances to return 200, got A=%d B=%d", rrA.Code, rrB.Code)
	}
	if rrA.Body.String() != rrB.Body.String() {
		t.Fatalf("expected equivalent history responses across instances, got A=%q B=%q", rrA.Body.String(), rrB.Body.String())
	}
}

func TestDeliveryRuntimeReadEndpointsAreReadOnly(t *testing.T) {
	server, db := newTestServer(t)
	mux := newMux(server)
	now := time.Now().UTC()
	insertIntent(t, db, "intent-readonly", submission.DeliveryTrackingModeOn, 300, now.Add(-time.Hour))
	insertState(t, db, "intent-readonly", deliverytracking.DeliveryStatusInProgress, deliverytracking.DeliveryFreshnessFresh, 300)
	insertHistory(t, db, "intent-readonly", "src-readonly-1", deliverytracking.DeliverySignalInProgress, now.Add(-time.Minute), false)

	before := loadDeliveryReadSnapshot(t, db, "intent-readonly")

	req := httptest.NewRequest(http.MethodGet, "/v1/intents/intent-readonly/delivery", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for current delivery read, got %d body=%q", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/intents/intent-readonly/delivery/history", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for delivery history read, got %d body=%q", rr.Code, rr.Body.String())
	}

	after := loadDeliveryReadSnapshot(t, db, "intent-readonly")
	if before.stateCount != after.stateCount {
		t.Fatalf("expected state row count to remain unchanged, before=%d after=%d", before.stateCount, after.stateCount)
	}
	if before.historyCount != after.historyCount {
		t.Fatalf("expected history row count to remain unchanged, before=%d after=%d", before.historyCount, after.historyCount)
	}
	if before.stateUpdatedAt.Valid != after.stateUpdatedAt.Valid {
		t.Fatalf("expected state updated_at validity unchanged, before=%v after=%v", before.stateUpdatedAt.Valid, after.stateUpdatedAt.Valid)
	}
	if before.stateUpdatedAt.Valid && !normalizeTime(before.stateUpdatedAt.Time).Equal(normalizeTime(after.stateUpdatedAt.Time)) {
		t.Fatalf("expected state updated_at unchanged, before=%s after=%s", before.stateUpdatedAt.Time, after.stateUpdatedAt.Time)
	}
}

func TestDeliveryRuntimeDoesNotServeSubmissionReadRoute(t *testing.T) {
	server, _ := newTestServer(t)
	mux := newMux(server)

	req := httptest.NewRequest(http.MethodGet, "/v1/intents/intent-1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%q", rr.Code, rr.Body.String())
	}
}

func newTestServer(t *testing.T) (*apiServer, *sql.DB) {
	t.Helper()
	db := newTestDB(t)
	return newServerFromDB(t, db), db
}

func newServerFromDB(t *testing.T, db *sql.DB) *apiServer {
	t.Helper()
	ingestor, err := deliverytracking.NewWebhookIngestor(db)
	if err != nil {
		t.Fatalf("new webhook ingestor: %v", err)
	}
	reader, err := deliverytracking.NewReader(db)
	if err != nil {
		t.Fatalf("new delivery reader: %v", err)
	}
	metrics := deliverytracking.NewMetrics(db)
	processor, err := deliverytracking.NewProcessorWithMetrics(db, metrics)
	if err != nil {
		t.Fatalf("new delivery processor: %v", err)
	}
	return &apiServer{
		webhookIngestor: ingestor,
		processor:       processor,
		reader:          reader,
		metrics:         metrics,
	}
}

func countWebhookIngestionRows(t *testing.T, db *sql.DB, intentID string) int {
	t.Helper()
	row := db.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM dbo.intent_delivery_webhook_ingestion WHERE intent_id = @p1`, intentID)
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count webhook ingestion rows: %v", err)
	}
	return count
}

type deliveryReadSnapshot struct {
	stateCount     int
	historyCount   int
	stateUpdatedAt sql.NullTime
}

func loadDeliveryReadSnapshot(t *testing.T, db *sql.DB, intentID string) deliveryReadSnapshot {
	t.Helper()
	var snapshot deliveryReadSnapshot

	stateRow := db.QueryRowContext(context.Background(), `SELECT COUNT(1), MAX(updated_at) FROM dbo.intent_delivery_state WHERE intent_id = @p1`, intentID)
	if err := stateRow.Scan(&snapshot.stateCount, &snapshot.stateUpdatedAt); err != nil {
		t.Fatalf("load delivery state snapshot: %v", err)
	}

	historyRow := db.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM dbo.intent_delivery_history WHERE intent_id = @p1`, intentID)
	if err := historyRow.Scan(&snapshot.historyCount); err != nil {
		t.Fatalf("load delivery history snapshot: %v", err)
	}

	return snapshot
}

func insertIntent(t *testing.T, db *sql.DB, intentID string, mode submission.DeliveryTrackingMode, staleAfter int, createdAt time.Time) {
	t.Helper()
	payload := []byte(`{"sms":"hello"}`)
	sum := sha256.Sum256(payload)
	var staleValue sql.NullInt32
	if mode == submission.DeliveryTrackingModeOn {
		staleValue = sql.NullInt32{Int32: int32(staleAfter), Valid: true}
	}
	_, err := db.ExecContext(
		context.Background(),
		`INSERT INTO dbo.submission_intents (
      intent_id,
      submission_target,
      payload,
      payload_hash,
      gateway_type,
      gateway_url,
      policy,
      max_acceptance_seconds,
      max_attempts,
      delivery_tracking_mode,
      delivery_tracking_stale_after_seconds,
      terminal_outcomes,
      webhook_url,
      webhook_headers,
      webhook_headers_env,
      webhook_secret_env,
      webhook_status,
      webhook_attempted_at,
      webhook_delivered_at,
      webhook_error,
      status,
      final_outcome_status,
      final_outcome_reason,
      exhausted_reason,
      attempt_count,
      created_at,
      updated_at,
      last_modified_at,
      next_attempt_at
    ) VALUES (
      @p1, @p2, @p3, @p4, @p5, @p6, @p7, NULL, NULL, @p8, @p9, @p10, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, @p11, NULL, NULL, NULL, 0, @p12, @p12, @p12, NULL
    )`,
		intentID,
		"sms.realtime",
		payload,
		sum[:],
		string(submission.GatewaySMS),
		"http://gateway.local",
		string(submission.PolicyOneShot),
		string(mode),
		staleValue,
		`["invalid_request"]`,
		"pending",
		createdAt.UTC(),
	)
	if err != nil {
		t.Fatalf("insert intent: %v", err)
	}
}

func insertState(t *testing.T, db *sql.DB, intentID string, status deliverytracking.DeliveryStatus, freshness deliverytracking.DeliveryFreshness, staleAfter int) {
	t.Helper()
	now := time.Now().UTC()
	staleAt := now.Add(time.Duration(staleAfter) * time.Second)
	_, err := db.ExecContext(
		context.Background(),
		`INSERT INTO dbo.intent_delivery_state (
      intent_id,
      delivery_status,
      delivery_freshness,
      last_non_terminal_effective_at,
      last_non_terminal_received_at,
      last_non_terminal_source_record_id,
      terminal_locked,
      terminal_status,
      terminal_effective_at,
      terminal_received_at,
      terminal_source_record_id,
      stale_after_seconds,
      stale_at,
      created_at,
      updated_at
    ) VALUES (
      @p1, @p2, @p3, NULL, NULL, NULL, 0, NULL, NULL, NULL, NULL, @p4, @p5, @p6, @p6
    )`,
		intentID,
		string(status),
		string(freshness),
		staleAfter,
		normalizeTime(staleAt),
		normalizeTime(now),
	)
	if err != nil {
		t.Fatalf("insert state: %v", err)
	}
}

func insertHistory(t *testing.T, db *sql.DB, intentID, sourceRecordID string, signal deliverytracking.DeliverySignalClass, recordedAt time.Time, lateObservation bool) {
	t.Helper()
	_, err := db.ExecContext(
		context.Background(),
		`INSERT INTO dbo.intent_delivery_history (
      intent_id,
      source_record_id,
      provider_event_id,
      signal_class,
      provider_observed_at,
      effective_at,
      received_at,
      late_observation,
      terminal_conflict,
      created_at
    ) VALUES (
      @p1, @p2, NULL, @p3, NULL, @p4, @p4, @p5, 0, @p4
    )`,
		intentID,
		sourceRecordID,
		string(signal),
		normalizeTime(recordedAt),
		lateObservation,
	)
	if err != nil {
		t.Fatalf("insert history: %v", err)
	}
}

func normalizeTime(value time.Time) time.Time {
	return time.Date(
		value.Year(),
		value.Month(),
		value.Day(),
		value.Hour(),
		value.Minute(),
		value.Second(),
		value.Nanosecond(),
		time.UTC,
	)
}

func assertContainsRuntimeMetric(t *testing.T, body, expected string) {
	t.Helper()
	if !strings.Contains(body, expected) {
		t.Fatalf("expected metrics output to include %q, body=%q", expected, body)
	}
}

func captureRuntimeLogs(buf *bytes.Buffer) func() {
	currentWriter := log.Writer()
	currentFlags := log.Flags()
	currentPrefix := log.Prefix()
	log.SetOutput(buf)
	log.SetFlags(0)
	log.SetPrefix("")
	return func() {
		log.SetOutput(currentWriter)
		log.SetFlags(currentFlags)
		log.SetPrefix(currentPrefix)
	}
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	password, ok, err := resolveSQLPassword(t)
	if err != nil {
		t.Fatalf("resolve sql password: %v", err)
	}
	if !ok {
		t.Skip("MSSQL_SA_PASSWORD not set; start docker compose and set env or backend/.env")
	}

	host := envOrDefault("MSSQL_HOST", "localhost")
	port := envOrDefault("MSSQL_PORT", "1433")

	masterDSN, err := buildSQLServerDSN(host, port, "sa", password, "master", "disable")
	if err != nil {
		t.Fatalf("build master dsn: %v", err)
	}
	masterDB, err := sql.Open("sqlserver", masterDSN)
	if err != nil {
		t.Fatalf("open master db: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	if err := masterDB.PingContext(ctx); err != nil {
		_ = masterDB.Close()
		t.Fatalf("ping master db: %v", err)
	}

	dbName := fmt.Sprintf("deliverytracking_http_test_%d", time.Now().UnixNano())
	if _, err := masterDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE [%s]", dbName)); err != nil {
		_ = masterDB.Close()
		t.Fatalf("create database: %v", err)
	}

	testDSN, err := buildSQLServerDSN(host, port, "sa", password, dbName, "disable")
	if err != nil {
		_ = dropTestDB(ctx, masterDB, dbName)
		t.Fatalf("build test dsn: %v", err)
	}
	db, err := sql.Open("sqlserver", testDSN)
	if err != nil {
		_ = dropTestDB(ctx, masterDB, dbName)
		t.Fatalf("open test db: %v", err)
	}

	schemaPath := filepath.Join(moduleRoot(t), "conf", "sql", "submissionmanager", "001_create_schema.sql")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		_ = db.Close()
		_ = dropTestDB(ctx, masterDB, dbName)
		t.Fatalf("read schema: %v", err)
	}
	if _, err := db.ExecContext(ctx, string(schema)); err != nil {
		_ = db.Close()
		_ = dropTestDB(ctx, masterDB, dbName)
		t.Fatalf("apply schema: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
		_ = dropTestDB(context.Background(), masterDB, dbName)
		_ = masterDB.Close()
	})

	return db
}

func dropTestDB(ctx context.Context, masterDB *sql.DB, dbName string) error {
	_, _ = masterDB.ExecContext(ctx, fmt.Sprintf("ALTER DATABASE [%s] SET SINGLE_USER WITH ROLLBACK IMMEDIATE", dbName))
	_, err := masterDB.ExecContext(ctx, fmt.Sprintf("DROP DATABASE [%s]", dbName))
	return err
}

func resolveSQLPassword(t *testing.T) (string, bool, error) {
	t.Helper()
	if value, ok := os.LookupEnv("MSSQL_SA_PASSWORD"); ok && strings.TrimSpace(value) != "" {
		return value, true, nil
	}

	data, err := os.ReadFile(filepath.Join(moduleRoot(t), ".env"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "MSSQL_SA_PASSWORD" && value != "" {
			return strings.Trim(value, "\"'"), true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", false, err
	}
	return "", false, nil
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve module root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
