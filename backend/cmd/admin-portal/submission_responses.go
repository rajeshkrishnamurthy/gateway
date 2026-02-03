package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

func submissionViewFromResponse(status int, body []byte, statusPath string) (submissionResultView, error) {
	view := submissionResultView{}
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		var resp submissionIntentResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return submissionResultView{}, err
		}
		view.IntentID = resp.IntentID
		view.StatusEndpoint = statusEndpoint(statusPath, resp.IntentID)
		view.Status = resp.Status
		view.RejectedReason = resp.RejectedReason
		view.ExhaustedReason = resp.ExhaustedReason
		view.CompletedAt = resp.CompletedAt
		return view, nil
	}

	view.Error = submissionErrorMessage(body)
	if view.Error == "" {
		view.Error = "submission failed"
	}
	return view, nil
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
