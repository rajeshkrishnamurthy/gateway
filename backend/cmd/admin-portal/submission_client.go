package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
)

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
