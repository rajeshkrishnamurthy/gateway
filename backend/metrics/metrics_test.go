package metrics

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestRegistryPrometheusOutput(t *testing.T) {
	registry := New("model-provider", []time.Duration{100 * time.Millisecond})
	registry.ObserveRequest("accepted", "", 120*time.Millisecond)
	registry.ObserveRequest("rejected", "invalid_request", 10*time.Millisecond)
	registry.ObserveRequest("rejected", "unregistered_token", 5*time.Millisecond)
	registry.ObserveProviderCall(50*time.Millisecond, context.DeadlineExceeded, true)

	var buf bytes.Buffer
	registry.WritePrometheus(&buf)
	out := buf.String()

	if !strings.Contains(out, `gateway_requests_total{provider="model-provider"} 3`) {
		t.Fatalf("expected request count in output")
	}
	if !strings.Contains(out, `gateway_outcomes_total{provider="model-provider",outcome="accepted"} 1`) {
		t.Fatalf("expected accepted count in output")
	}
	if !strings.Contains(out, `gateway_rejections_total{provider="model-provider",reason="invalid_request"} 1`) {
		t.Fatalf("expected rejection reason count in output")
	}
	if !strings.Contains(out, `gateway_rejections_total{provider="model-provider",reason="unregistered_token"} 1`) {
		t.Fatalf("expected unregistered_token count in output")
	}
	if !strings.Contains(out, `gateway_provider_timeouts_total{provider="model-provider"} 1`) {
		t.Fatalf("expected provider timeout count in output")
	}
	if !strings.Contains(out, `gateway_provider_panics_total{provider="model-provider"} 1`) {
		t.Fatalf("expected provider panic count in output")
	}
	if !strings.Contains(out, "gateway_request_duration_seconds_bucket") {
		t.Fatalf("expected request duration histogram in output")
	}
	if !strings.Contains(out, "gateway_provider_duration_seconds_bucket") {
		t.Fatalf("expected provider duration histogram in output")
	}
}
