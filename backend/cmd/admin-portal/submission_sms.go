package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
)

func (s *portalServer) useSubmissionManagerSMS() bool {
	return s.config.SubmissionManagerURL != "" && s.config.SMSSubmissionTarget != ""
}

func (s *portalServer) handleSMSStatus(w http.ResponseWriter, r *http.Request) {
	if !s.useSubmissionManagerSMS() {
		s.renderSubmissionFailure(w, r, http.StatusNotFound, "submission manager not configured")
		return
	}
	if r.Method != http.MethodGet {
		s.renderSubmissionFailure(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	intentID := strings.TrimSpace(r.URL.Query().Get("intentId"))
	if intentID == "" {
		s.renderSubmissionFailure(w, r, http.StatusBadRequest, "intentId is required")
		return
	}

	status, body, contentType, err := s.fetchIntent(r.Context(), intentID)
	if err != nil {
		s.renderSubmissionFailure(w, r, http.StatusBadGateway, err.Error())
		return
	}

	if !isHTMX(r) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(status)
		if _, err := w.Write(body); err != nil {
			log.Printf("write submission response: %v", err)
		}
		return
	}

	view, err := submissionViewFromResponse(status, body, "/sms/status")
	if err != nil {
		s.renderSubmissionFailure(w, r, http.StatusBadGateway, "decode response failed")
		return
	}

	s.renderSubmissionResult(w, status, view)
}

func (s *portalServer) handleSMSSubmission(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.renderSubmissionFailure(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req smsTestRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		s.renderSubmissionFailure(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		s.renderSubmissionFailure(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	req.ReferenceID = strings.TrimSpace(req.ReferenceID)
	req.To = strings.TrimSpace(req.To)
	req.Message = strings.TrimSpace(req.Message)
	req.TenantID = strings.TrimSpace(req.TenantID)
	req.WaitSeconds = strings.TrimSpace(req.WaitSeconds)
	if req.ReferenceID == "" || req.To == "" || req.Message == "" {
		s.renderSubmissionFailure(w, r, http.StatusBadRequest, "referenceId, to, and message are required")
		return
	}
	waitSeconds, err := parseWaitSeconds(req.WaitSeconds)
	if err != nil {
		s.renderSubmissionFailure(w, r, http.StatusBadRequest, err.Error())
		return
	}

	payload := map[string]string{
		"referenceId": req.ReferenceID,
		"to":          req.To,
		"message":     req.Message,
	}
	if req.TenantID != "" {
		payload["tenantId"] = req.TenantID
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		s.renderSubmissionFailure(w, r, http.StatusInternalServerError, "encode payload failed")
		return
	}

	intentReq := submissionIntentRequest{
		IntentID:         req.ReferenceID,
		SubmissionTarget: s.config.SMSSubmissionTarget,
		Payload:          payloadBytes,
	}

	status, body, contentType, err := s.submitIntent(r.Context(), intentReq, waitSeconds)
	if err != nil {
		s.renderSubmissionFailure(w, r, http.StatusBadGateway, err.Error())
		return
	}

	if !isHTMX(r) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(status)
		if _, err := w.Write(body); err != nil {
			log.Printf("write submission response: %v", err)
		}
		return
	}

	view, err := submissionViewFromResponse(status, body, "/sms/status")
	if err != nil {
		s.renderSubmissionFailure(w, r, http.StatusBadGateway, "decode response failed")
		return
	}

	s.renderSubmissionResult(w, status, view)
}
