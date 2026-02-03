package main

import (
	"bufio"
	"bytes"
	"gateway/metrics"
	"strings"
	"time"
)

var latencyBuckets = []time.Duration{
	100 * time.Millisecond,
	250 * time.Millisecond,
	500 * time.Millisecond,
	1 * time.Second,
	2500 * time.Millisecond,
	5 * time.Second,
}

type metricsView struct {
	SendNavLabel         string
	MetricsURL           string
	ShowNav              bool
	Title                string
	TotalRequests        string
	AcceptedTotal        string
	RejectedTotal        string
	ProviderFailureCount string
	Rejections           []rejectionCount
	RequestLatency       []latencyBucket
	ProviderLatency      []latencyBucket
}

type rejectionCount struct {
	Reason string
	Count  string
}

type latencyBucket struct {
	Label string
	Count string
}

func buildMetricsView(metricsRegistry *metrics.Registry) metricsView {
	if metricsRegistry == nil {
		return metricsView{}
	}
	var buf bytes.Buffer
	metricsRegistry.WritePrometheus(&buf)
	return parseMetrics(buf.String())
}

func parseMetrics(text string) metricsView {
	view := metricsView{}
	rejections := make(map[string]string)
	var requestLatency []latencyBucket
	var providerLatency []latencyBucket
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		name, labels, value, ok := parseMetricLine(line)
		if !ok {
			continue
		}
		switch name {
		case "gateway_requests_total":
			view.TotalRequests = value
		case "gateway_outcomes_total":
			switch labels["outcome"] {
			case "accepted":
				view.AcceptedTotal = value
			case "rejected":
				view.RejectedTotal = value
			}
		case "gateway_rejections_total":
			if reason := labels["reason"]; reason != "" {
				rejections[reason] = value
			}
		case "gateway_provider_failures_total":
			view.ProviderFailureCount = value
		case "gateway_request_duration_seconds_bucket":
			if le := labels["le"]; le != "" && le != "+Inf" {
				requestLatency = append(requestLatency, latencyBucket{
					Label: "<= " + le + "s",
					Count: value,
				})
			}
		case "gateway_provider_duration_seconds_bucket":
			if le := labels["le"]; le != "" && le != "+Inf" {
				providerLatency = append(providerLatency, latencyBucket{
					Label: "<= " + le + "s",
					Count: value,
				})
			}
		}
	}
	if len(rejections) > 0 {
		reasonOrder := []string{
			"invalid_request",
			"duplicate_reference",
			"invalid_recipient",
			"invalid_message",
			"provider_failure",
		}
		for _, reason := range reasonOrder {
			count := rejections[reason]
			if count == "" {
				count = "0"
			}
			view.Rejections = append(view.Rejections, rejectionCount{
				Reason: reason,
				Count:  count,
			})
		}
	}
	view.RequestLatency = requestLatency
	view.ProviderLatency = providerLatency
	return view
}

func parseMetricLine(line string) (string, map[string]string, string, bool) {
	if line == "" || strings.HasPrefix(line, "#") {
		return "", nil, "", false
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", nil, "", false
	}
	metric := fields[0]
	value := fields[1]
	name := metric
	labels := map[string]string{}
	if idx := strings.Index(metric, "{"); idx != -1 && strings.HasSuffix(metric, "}") {
		name = metric[:idx]
		labelPart := strings.TrimSuffix(metric[idx+1:], "}")
		labels = parseLabels(labelPart)
	}
	return name, labels, value, true
}

func parseLabels(labelPart string) map[string]string {
	labels := make(map[string]string)
	if labelPart == "" {
		return labels
	}
	parts := strings.Split(labelPart, ",")
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		value := strings.Trim(kv[1], "\"")
		labels[key] = value
	}
	return labels
}
