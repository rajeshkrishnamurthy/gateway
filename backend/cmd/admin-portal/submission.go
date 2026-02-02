package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (s *portalServer) useSubmissionManagerSMS() bool {
	return s.config.SubmissionManagerURL != "" && s.config.SMSSubmissionTarget != ""
}

func (s *portalServer) useSubmissionManagerPush() bool {
	return s.config.SubmissionManagerURL != "" && s.config.PushSubmissionTarget != ""
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

	view := submissionResultView{}
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		var resp submissionIntentResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			s.renderSubmissionFailure(w, r, http.StatusBadGateway, "decode response failed")
			return
		}
		view.IntentID = resp.IntentID
		view.StatusEndpoint = statusEndpoint("/sms/status", resp.IntentID)
		view.Status = resp.Status
		view.RejectedReason = resp.RejectedReason
		view.ExhaustedReason = resp.ExhaustedReason
		view.CompletedAt = resp.CompletedAt
	} else {
		view.Error = submissionErrorMessage(body)
		if view.Error == "" {
			view.Error = "submission failed"
		}
	}

	s.renderSubmissionResult(w, status, view)
}

func (s *portalServer) handlePushStatus(w http.ResponseWriter, r *http.Request) {
	if !s.useSubmissionManagerPush() {
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

	view := submissionResultView{}
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		var resp submissionIntentResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			s.renderSubmissionFailure(w, r, http.StatusBadGateway, "decode response failed")
			return
		}
		view.IntentID = resp.IntentID
		view.StatusEndpoint = statusEndpoint("/push/status", resp.IntentID)
		view.Status = resp.Status
		view.RejectedReason = resp.RejectedReason
		view.ExhaustedReason = resp.ExhaustedReason
		view.CompletedAt = resp.CompletedAt
	} else {
		view.Error = submissionErrorMessage(body)
		if view.Error == "" {
			view.Error = "submission failed"
		}
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

	view := submissionResultView{}
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		var resp submissionIntentResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			s.renderSubmissionFailure(w, r, http.StatusBadGateway, "decode response failed")
			return
		}
		view.IntentID = resp.IntentID
		view.StatusEndpoint = statusEndpoint("/sms/status", resp.IntentID)
		view.Status = resp.Status
		view.RejectedReason = resp.RejectedReason
		view.ExhaustedReason = resp.ExhaustedReason
		view.CompletedAt = resp.CompletedAt
	} else {
		view.Error = submissionErrorMessage(body)
		if view.Error == "" {
			view.Error = "submission failed"
		}
	}

	s.renderSubmissionResult(w, status, view)
}

func (s *portalServer) handlePushSubmission(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.renderSubmissionFailure(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req pushTestRequest
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
	req.Token = strings.TrimSpace(req.Token)
	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)
	req.TenantID = strings.TrimSpace(req.TenantID)
	req.WaitSeconds = strings.TrimSpace(req.WaitSeconds)
	if req.ReferenceID == "" || req.Token == "" {
		s.renderSubmissionFailure(w, r, http.StatusBadRequest, "referenceId and token are required")
		return
	}
	waitSeconds, err := parseWaitSeconds(req.WaitSeconds)
	if err != nil {
		s.renderSubmissionFailure(w, r, http.StatusBadRequest, err.Error())
		return
	}

	payload := map[string]string{
		"referenceId": req.ReferenceID,
		"token":       req.Token,
	}
	if req.Title != "" {
		payload["title"] = req.Title
	}
	if req.Body != "" {
		payload["body"] = req.Body
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
		SubmissionTarget: s.config.PushSubmissionTarget,
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

	view := submissionResultView{}
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		var resp submissionIntentResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			s.renderSubmissionFailure(w, r, http.StatusBadGateway, "decode response failed")
			return
		}
		view.IntentID = resp.IntentID
		view.StatusEndpoint = statusEndpoint("/push/status", resp.IntentID)
		view.Status = resp.Status
		view.RejectedReason = resp.RejectedReason
		view.ExhaustedReason = resp.ExhaustedReason
		view.CompletedAt = resp.CompletedAt
	} else {
		view.Error = submissionErrorMessage(body)
		if view.Error == "" {
			view.Error = "submission failed"
		}
	}

	s.renderSubmissionResult(w, status, view)
}

func (s *portalServer) submitIntent(ctx context.Context, intent submissionIntentRequest, waitSeconds string) (int, []byte, string, error) {
	query := url.Values{}
	if waitSeconds != "" {
		query.Set("waitSeconds", waitSeconds)
	}
	targetURL, err := buildTargetURL(s.config.SubmissionManagerURL, "/v1/intents", query.Encode(), false)
	if err != nil {
		return 0, nil, "", err
	}
	body, err := json.Marshal(intent)
	if err != nil {
		return 0, nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return 0, nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, "", err
	}
	return resp.StatusCode, respBody, resp.Header.Get("Content-Type"), nil
}

func parseWaitSeconds(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 0 {
		return "", errors.New("waitSeconds must be a non-negative integer")
	}
	return strconv.Itoa(seconds), nil
}

func (s *portalServer) fetchIntent(ctx context.Context, intentID string) (int, []byte, string, error) {
	escaped := url.PathEscape(intentID)
	targetURL, err := buildTargetURL(s.config.SubmissionManagerURL, "/v1/intents/"+escaped, "", false)
	if err != nil {
		return 0, nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return 0, nil, "", err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, "", err
	}
	return resp.StatusCode, respBody, resp.Header.Get("Content-Type"), nil
}

func (s *portalServer) renderSubmissionResult(w http.ResponseWriter, status int, view submissionResultView) {
	if s.templates.submissionResult == nil {
		http.Error(w, "template not configured", http.StatusInternalServerError)
		return
	}
	fragment, err := executeTemplate(s.templates.submissionResult, "submission_result.tmpl", view)
	if err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	if status >= http.StatusBadRequest {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(fragment); err != nil {
		log.Printf("write submission fragment: %v", err)
	}
}

func (s *portalServer) renderSubmissionFailure(w http.ResponseWriter, r *http.Request, status int, message string) {
	if !isHTMX(r) {
		http.Error(w, message, status)
		return
	}
	s.renderSubmissionResult(w, status, submissionResultView{Error: message})
}

func submissionErrorMessage(body []byte) string {
	var errResp submissionErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return ""
	}
	if strings.TrimSpace(errResp.Error.Message) != "" {
		return errResp.Error.Message
	}
	return strings.TrimSpace(errResp.Error.Code)
}

func statusEndpoint(basePath, intentID string) string {
	if strings.TrimSpace(intentID) == "" {
		return ""
	}
	return basePath + "?intentId=" + url.QueryEscape(intentID)
}
