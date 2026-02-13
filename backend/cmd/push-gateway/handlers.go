package main

import (
	"encoding/json"
	"errors"
	"gateway"
	setulog "gateway/logging"
	"gateway/metrics"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const maxBodyBytes = 16 << 10

func newMux(gw *gateway.PushGateway, metricsRegistry *metrics.Registry, ui *uiServer) *http.ServeMux {
	mux := http.NewServeMux()
	var sendResult *template.Template
	if ui != nil {
		sendResult = ui.templates.sendResult
	}
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/readyz", handleReadyz)
	mux.HandleFunc("/push/send", handlePushSend(gw, metricsRegistry, sendResult))
	mux.HandleFunc("/metrics", handleMetrics(metricsRegistry))
	if ui != nil {
		mux.Handle("/ui/static/", http.StripPrefix("/ui/static/", http.FileServer(http.Dir(ui.staticDir))))
		mux.HandleFunc("/ui", ui.handleOverview)
		mux.HandleFunc("/ui/send", ui.handleSend)
		mux.HandleFunc("/ui/metrics", ui.handleUIMetrics)
	}
	return mux
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

func handlePushSend(gw *gateway.PushGateway, metricsRegistry *metrics.Registry, sendResult *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := slog.Default().With(
			"component", "push-gateway-http",
			"commProfile", setulog.CommProfileOutsideToSetu,
			"boundaryDirection", setulog.BoundaryDirectionIngress,
			"peerSystem", "client",
			"operation", "push_send",
		)
		start := time.Now()
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

		dec := json.NewDecoder(r.Body)
		var req gateway.PushRequest
		if err := dec.Decode(&req); err != nil {
			logger.Warn(
				"push decision",
				"event", "gateway.decision",
				"outcome", "rejected",
				"referenceId", "",
				"status", "rejected",
				"reason", "invalid_request",
				"source", "validation",
				"detail", "decode_error",
				"error", err,
			)
			writePushSendResponse(w, r, http.StatusOK, gateway.PushResponse{
				Status: "rejected",
				Reason: "invalid_request",
			}, sendResult)
			if metricsRegistry != nil {
				metricsRegistry.ObserveRequest("rejected", "invalid_request", time.Since(start))
			}
			return
		}
		if err := dec.Decode(&struct{}{}); err != io.EOF {
			logger.Warn(
				"push decision",
				"event", "gateway.decision",
				"outcome", "rejected",
				"referenceId", req.ReferenceID,
				"status", "rejected",
				"reason", "invalid_request",
				"source", "validation",
				"detail", "trailing_json",
			)
			writePushSendResponse(w, r, http.StatusOK, gateway.PushResponse{
				Status: "rejected",
				Reason: "invalid_request",
			}, sendResult)
			if metricsRegistry != nil {
				metricsRegistry.ObserveRequest("rejected", "invalid_request", time.Since(start))
			}
			return
		}

		resp, err := gw.SendPush(r.Context(), req)
		source := "provider_result"
		if err != nil && errors.Is(err, gateway.ErrInvalidRequest) {
			source = "validation"
		} else if resp.Reason == "provider_failure" {
			source = "provider_failure"
		}
		logger.Info(
			"push decision",
			"event", "gateway.decision",
			"outcome", resp.Status,
			"referenceId", resp.ReferenceID,
			"status", resp.Status,
			"reason", resp.Reason,
			"source", source,
			"gatewayMessageId", resp.GatewayMessageID,
		)
		writePushSendResponse(w, r, http.StatusOK, resp, sendResult)
		if metricsRegistry != nil {
			metricsRegistry.ObserveRequest(resp.Status, resp.Reason, time.Since(start))
		}
	}
}

func handleMetrics(metricsRegistry *metrics.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if metricsRegistry == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		metricsRegistry.WritePrometheus(w)
	}
}

func writePushResponse(w http.ResponseWriter, status int, resp gateway.PushResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Default().With(
			"component", "push-gateway-http",
			"commProfile", setulog.CommProfileWithinSetu,
			"operation", "http_response_write",
			"stage", "encode_json",
			"outcome", "failed",
		).Error("encode response failed", "event", "gateway.response.encode_failed", "error", err)
	}
}

func writePushSendResponse(w http.ResponseWriter, r *http.Request, status int, resp gateway.PushResponse, sendResult *template.Template) {
	if sendResult != nil && isHTMX(r) {
		fragmentStatus := status
		if fragmentStatus >= http.StatusBadRequest {
			fragmentStatus = http.StatusOK
		}
		writePushResponseFragment(w, fragmentStatus, resp, sendResult)
		return
	}
	writePushResponse(w, status, resp)
}

func writePushResponseFragment(w http.ResponseWriter, status int, resp gateway.PushResponse, tmpl *template.Template) {
	fragment, err := executeTemplate(tmpl, "send_result.tmpl", resp)
	if err != nil {
		slog.Default().With(
			"component", "push-gateway-http",
			"commProfile", setulog.CommProfileWithinSetu,
			"operation", "html_fragment_render",
			"stage", "template_execute",
			"outcome", "failed",
		).Error("render send result failed", "event", "gateway.fragment.render_failed", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(fragment); err != nil {
		slog.Default().With(
			"component", "push-gateway-http",
			"commProfile", setulog.CommProfileWithinSetu,
			"operation", "html_fragment_write",
			"stage", "response_write",
			"outcome", "failed",
		).Error("write send result failed", "event", "gateway.fragment.write_failed", "error", err)
	}
}

func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func isEmbed(r *http.Request) bool {
	embed := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("embed")))
	return embed == "1" || embed == "true"
}
