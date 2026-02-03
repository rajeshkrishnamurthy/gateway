package main

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

type submitRequest struct {
	IntentID         string          `json:"intentId"`
	SubmissionTarget string          `json:"submissionTarget"`
	Payload          json.RawMessage `json:"payload"`
}

const maxWaitSeconds = 30

func parseWaitSeconds(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("waitSeconds must be a non-negative integer")
	}
	if value < 0 {
		return 0, errors.New("waitSeconds must be a non-negative integer")
	}
	if value > maxWaitSeconds {
		value = maxWaitSeconds
	}
	return time.Duration(value) * time.Second, nil
}
