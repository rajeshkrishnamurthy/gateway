package logging

import (
	"encoding/json"
	"testing"
)

func TestParseRecordLine_ValidOutsideToSetu(t *testing.T) {
	line := mustJSON(t, map[string]any{
		"logSchemaVersion":  CurrentSchemaVersion,
		"time":              "2026-02-13T00:00:00Z",
		"level":             "INFO",
		"msg":               "gateway decision",
		"event":             "gateway.decision",
		"service":           "sms-gateway",
		"component":         "sms-gateway-http",
		"instance":          "sms-gateway-1",
		"commProfile":       CommProfileOutsideToSetu,
		"boundaryDirection": BoundaryDirectionIngress,
		"peerSystem":        "client",
		"operation":         "sms_send",
		"outcome":           "rejected",
		"referenceId":       "ref-1",
	})

	record, err := ParseRecordLine(line)
	if err != nil {
		t.Fatalf("ParseRecordLine returned error: %v", err)
	}
	if record.CommProfile != CommProfileOutsideToSetu {
		t.Fatalf("expected commProfile %q, got %q", CommProfileOutsideToSetu, record.CommProfile)
	}
}

func TestParseRecordLine_ValidSetuToSetu(t *testing.T) {
	duration := 12.5
	line := mustJSON(t, map[string]any{
		"logSchemaVersion": CurrentSchemaVersion,
		"time":             "2026-02-13T00:00:00Z",
		"level":            "INFO",
		"msg":              "gateway call completed",
		"event":            "submission.gateway_call.completed",
		"service":          "submission-manager",
		"component":        "submission-manager-executor",
		"instance":         "submission-manager-1",
		"commProfile":      CommProfileSetuToSetu,
		"callerService":    "submission-manager",
		"calleeService":    "sms-gateway",
		"operation":        "gateway_submit",
		"outcome":          "accepted",
		"durationMs":       duration,
	})

	record, err := ParseRecordLine(line)
	if err != nil {
		t.Fatalf("ParseRecordLine returned error: %v", err)
	}
	if record.DurationMS == nil || *record.DurationMS != duration {
		t.Fatalf("expected durationMs %v, got %#v", duration, record.DurationMS)
	}
}

func TestParseRecordLine_ValidWithinSetuService(t *testing.T) {
	line := mustJSON(t, map[string]any{
		"logSchemaVersion": CurrentSchemaVersion,
		"time":             "2026-02-13T00:00:00Z",
		"level":            "INFO",
		"msg":              "leader renewed",
		"event":            "leader_renewed",
		"service":          "submission-manager",
		"component":        "submission-manager-leader",
		"instance":         "submission-manager-1",
		"commProfile":      CommProfileWithinSetu,
		"operation":        "leader_lease",
		"stage":            "renew",
		"outcome":          "renewed",
	})

	record, err := ParseRecordLine(line)
	if err != nil {
		t.Fatalf("ParseRecordLine returned error: %v", err)
	}
	if record.CommProfile != CommProfileWithinSetu {
		t.Fatalf("expected commProfile %q, got %q", CommProfileWithinSetu, record.CommProfile)
	}
}

func TestParseRecordLine_AllowsPreviousSchemaVersion(t *testing.T) {
	line := mustJSON(t, map[string]any{
		"logSchemaVersion": PreviousSchemaVersion,
		"time":             "2026-02-13T00:00:00Z",
		"level":            "INFO",
		"msg":              "leader renewed",
		"event":            "leader_renewed",
		"service":          "submission-manager",
		"component":        "submission-manager-leader",
		"instance":         "submission-manager-1",
		"commProfile":      CommProfileWithinSetu,
		"operation":        "leader_lease",
		"stage":            "renew",
		"outcome":          "renewed",
	})

	if _, err := ParseRecordLine(line); err != nil {
		t.Fatalf("ParseRecordLine returned error for previous schema version: %v", err)
	}
}

func TestParseRecordLine_MissingRequiredBaseField(t *testing.T) {
	line := mustJSON(t, map[string]any{
		"logSchemaVersion": CurrentSchemaVersion,
		"time":             "2026-02-13T00:00:00Z",
		"level":            "INFO",
		"msg":              "x",
		"event":            "x",
		"service":          "x",
		"instance":         "x",
		"commProfile":      CommProfileWithinSetu,
		"operation":        "x",
		"stage":            "x",
		"outcome":          "x",
	})

	if _, err := ParseRecordLine(line); err == nil {
		t.Fatalf("expected missing required field error")
	}
}

func TestParseRecordLine_InvalidProfile(t *testing.T) {
	line := mustJSON(t, map[string]any{
		"logSchemaVersion": CurrentSchemaVersion,
		"time":             "2026-02-13T00:00:00Z",
		"level":            "INFO",
		"msg":              "x",
		"event":            "x",
		"service":          "x",
		"component":        "x",
		"instance":         "x",
		"commProfile":      "unknown_profile",
	})

	if _, err := ParseRecordLine(line); err == nil {
		t.Fatalf("expected invalid profile error")
	}
}

func TestParseRecordLine_MissingProfileSpecificField(t *testing.T) {
	line := mustJSON(t, map[string]any{
		"logSchemaVersion": CurrentSchemaVersion,
		"time":             "2026-02-13T00:00:00Z",
		"level":            "INFO",
		"msg":              "x",
		"event":            "x",
		"service":          "x",
		"component":        "x",
		"instance":         "x",
		"commProfile":      CommProfileSetuToSetu,
		"callerService":    "submission-manager",
		"calleeService":    "sms-gateway",
		"operation":        "gateway_submit",
		"outcome":          "accepted",
	})

	if _, err := ParseRecordLine(line); err == nil {
		t.Fatalf("expected missing durationMs error")
	}
}

func TestParseRecordLine_MalformedJSON(t *testing.T) {
	if _, err := ParseRecordLine([]byte(`{"event":`)); err == nil {
		t.Fatalf("expected malformed json error")
	}
}

func TestParseRecordLine_RedactionSensitiveForbiddenKey(t *testing.T) {
	line := mustJSON(t, map[string]any{
		"logSchemaVersion": CurrentSchemaVersion,
		"time":             "2026-02-13T00:00:00Z",
		"level":            "INFO",
		"msg":              "x",
		"event":            "x",
		"service":          "x",
		"component":        "x",
		"instance":         "x",
		"commProfile":      CommProfileWithinSetu,
		"operation":        "x",
		"stage":            "x",
		"outcome":          "x",
		"apiKey":           "secret-value",
	})

	if _, err := ParseRecordLine(line); err == nil {
		t.Fatalf("expected forbidden key error")
	}
}

func mustJSON(t *testing.T, v map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json marshal failed: %v", err)
	}
	return data
}
