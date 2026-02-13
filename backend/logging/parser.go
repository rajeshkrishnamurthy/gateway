package logging

import (
	"encoding/json"
	"fmt"
	"strings"
)

var forbiddenTopLevelKeys = map[string]struct{}{
	"authorization": {},
	"authHeader":    {},
	"apiKey":        {},
	"api_key":       {},
	"password":      {},
	"secret":        {},
	"credentials":   {},
	"rawPayload":    {},
	"rawBody":       {},
	"token":         {},
	"recipient":     {},
	"fullMessage":   {},
}

// Record is the normalized log representation used by Setu parsing workflows.
type Record struct {
	LogSchemaVersion  string   `json:"logSchemaVersion"`
	Time              string   `json:"time"`
	Level             string   `json:"level"`
	Msg               string   `json:"msg"`
	Event             string   `json:"event"`
	Service           string   `json:"service"`
	Component         string   `json:"component"`
	Instance          string   `json:"instance"`
	CommProfile       string   `json:"commProfile"`
	BoundaryDirection string   `json:"boundaryDirection,omitempty"`
	PeerSystem        string   `json:"peerSystem,omitempty"`
	Operation         string   `json:"operation,omitempty"`
	Outcome           string   `json:"outcome,omitempty"`
	CallerService     string   `json:"callerService,omitempty"`
	CalleeService     string   `json:"calleeService,omitempty"`
	DurationMS        *float64 `json:"durationMs,omitempty"`
	Stage             string   `json:"stage,omitempty"`
}

// ParseRecordLine parses one JSON-line record and validates schema/profile constraints.
func ParseRecordLine(line []byte) (Record, error) {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return Record{}, fmt.Errorf("parse log line: %w", err)
	}

	if err := validateNoForbiddenKeys(raw); err != nil {
		return Record{}, err
	}

	var rec Record
	if err := json.Unmarshal(line, &rec); err != nil {
		return Record{}, fmt.Errorf("decode normalized record: %w", err)
	}

	if err := validateBaseFields(rec); err != nil {
		return Record{}, err
	}
	if err := validateSchemaVersion(rec.LogSchemaVersion); err != nil {
		return Record{}, err
	}
	if err := validateProfileFields(rec); err != nil {
		return Record{}, err
	}

	return rec, nil
}

func validateNoForbiddenKeys(raw map[string]any) error {
	for key := range raw {
		if _, banned := forbiddenTopLevelKeys[key]; banned {
			return fmt.Errorf("forbidden key present: %s", key)
		}
	}
	return nil
}

func validateBaseFields(rec Record) error {
	required := map[string]string{
		"logSchemaVersion": rec.LogSchemaVersion,
		"time":             rec.Time,
		"level":            rec.Level,
		"msg":              rec.Msg,
		"event":            rec.Event,
		"service":          rec.Service,
		"component":        rec.Component,
		"instance":         rec.Instance,
		"commProfile":      rec.CommProfile,
	}
	for key, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("missing required field: %s", key)
		}
	}
	return nil
}

func validateSchemaVersion(version string) error {
	switch version {
	case CurrentSchemaVersion, PreviousSchemaVersion:
		return nil
	default:
		return fmt.Errorf("unsupported logSchemaVersion: %s", version)
	}
}

func validateProfileFields(rec Record) error {
	switch rec.CommProfile {
	case CommProfileOutsideToSetu:
		if strings.TrimSpace(rec.BoundaryDirection) != BoundaryDirectionIngress &&
			strings.TrimSpace(rec.BoundaryDirection) != BoundaryDirectionEgress {
			return fmt.Errorf("outside_to_setu requires boundaryDirection ingress or egress")
		}
		if strings.TrimSpace(rec.PeerSystem) == "" {
			return fmt.Errorf("outside_to_setu requires peerSystem")
		}
		if strings.TrimSpace(rec.Operation) == "" {
			return fmt.Errorf("outside_to_setu requires operation")
		}
		if strings.TrimSpace(rec.Outcome) == "" {
			return fmt.Errorf("outside_to_setu requires outcome")
		}
		return nil
	case CommProfileSetuToSetu:
		if strings.TrimSpace(rec.CallerService) == "" {
			return fmt.Errorf("setu_to_setu requires callerService")
		}
		if strings.TrimSpace(rec.CalleeService) == "" {
			return fmt.Errorf("setu_to_setu requires calleeService")
		}
		if strings.TrimSpace(rec.Operation) == "" {
			return fmt.Errorf("setu_to_setu requires operation")
		}
		if strings.TrimSpace(rec.Outcome) == "" {
			return fmt.Errorf("setu_to_setu requires outcome")
		}
		if rec.DurationMS == nil || *rec.DurationMS < 0 {
			return fmt.Errorf("setu_to_setu requires non-negative durationMs")
		}
		return nil
	case CommProfileWithinSetu:
		if strings.TrimSpace(rec.Operation) == "" {
			return fmt.Errorf("within_setu_service requires operation")
		}
		if strings.TrimSpace(rec.Stage) == "" {
			return fmt.Errorf("within_setu_service requires stage")
		}
		if strings.TrimSpace(rec.Outcome) == "" {
			return fmt.Errorf("within_setu_service requires outcome")
		}
		return nil
	default:
		return fmt.Errorf("unsupported commProfile: %s", rec.CommProfile)
	}
}
