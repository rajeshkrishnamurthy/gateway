package main

import (
	"time"

	"gateway/submissionmanager"
)

type intentResponse struct {
	IntentID         string `json:"intentId"`
	SubmissionTarget string `json:"submissionTarget"`
	CreatedAt        string `json:"createdAt"`
	Status           string `json:"status"`
	CompletedAt      string `json:"completedAt,omitempty"`
	RejectedReason   string `json:"rejectedReason,omitempty"`
	ExhaustedReason  string `json:"exhaustedReason,omitempty"`
}

type intentHistoryResponse struct {
	Intent   intentResponse    `json:"intent"`
	Attempts []attemptResponse `json:"attempts"`
}

type attemptResponse struct {
	AttemptNumber int    `json:"attemptNumber"`
	StartedAt     string `json:"startedAt,omitempty"`
	FinishedAt    string `json:"finishedAt,omitempty"`
	OutcomeStatus string `json:"outcomeStatus,omitempty"`
	OutcomeReason string `json:"outcomeReason,omitempty"`
	Error         string `json:"error,omitempty"`
}

func toIntentResponse(intent submissionmanager.Intent) intentResponse {
	completedAt := ""
	if intent.Status == submissionmanager.IntentAccepted ||
		intent.Status == submissionmanager.IntentRejected ||
		intent.Status == submissionmanager.IntentExhausted {
		if !intent.CompletedAt.IsZero() {
			completedAt = intent.CompletedAt.UTC().Format(timeFormat)
		}
	}

	rejectedReason := ""
	if intent.Status == submissionmanager.IntentRejected {
		rejectedReason = intent.FinalOutcome.Reason
	}

	exhaustedReason := ""
	if intent.Status == submissionmanager.IntentExhausted {
		exhaustedReason = intent.ExhaustedReason
	}

	return intentResponse{
		IntentID:         intent.IntentID,
		SubmissionTarget: intent.SubmissionTarget,
		CreatedAt:        intent.CreatedAt.UTC().Format(timeFormat),
		Status:           string(intent.Status),
		CompletedAt:      completedAt,
		RejectedReason:   rejectedReason,
		ExhaustedReason:  exhaustedReason,
	}
}

func toAttemptResponses(attempts []submissionmanager.Attempt) []attemptResponse {
	out := make([]attemptResponse, 0, len(attempts))
	for _, attempt := range attempts {
		out = append(out, attemptResponse{
			AttemptNumber: attempt.Number,
			StartedAt:     formatAttemptTime(attempt.StartedAt),
			FinishedAt:    formatAttemptTime(attempt.FinishedAt),
			OutcomeStatus: attempt.GatewayOutcome.Status,
			OutcomeReason: attempt.GatewayOutcome.Reason,
			Error:         attempt.Error,
		})
	}
	return out
}

func formatAttemptTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(timeFormat)
}

const timeFormat = time.RFC3339Nano
