package metrics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"
)

// Registry tracks gateway metrics for a single provider.
type Registry struct {
	providerName string

	mu sync.Mutex

	requestsTotal uint64
	acceptedTotal uint64
	rejectedTotal uint64

	rejectedInvalidRequest     uint64
	rejectedDuplicateReference uint64
	rejectedInvalidRecipient   uint64
	rejectedInvalidMessage     uint64
	rejectedProviderFailure    uint64
	rejectedUnregisteredToken  uint64

	providerFailures uint64
	providerTimeouts uint64
	providerPanics   uint64

	requestDuration  histogram
	providerDuration histogram
}

type histogram struct {
	buckets []float64
	counts  []uint64
	count   uint64
	sum     float64
}

// New constructs a Registry for a provider and histogram bucket set.
func New(providerName string, buckets []time.Duration) *Registry {
	bucketSeconds := make([]float64, len(buckets))
	for i, b := range buckets {
		bucketSeconds[i] = b.Seconds()
	}
	return &Registry{
		providerName:     providerName,
		requestDuration:  newHistogram(bucketSeconds),
		providerDuration: newHistogram(bucketSeconds),
	}
}

// ObserveRequest records a gateway request outcome and its duration.
func (r *Registry) ObserveRequest(outcome, reason string, duration time.Duration) {
	if r == nil {
		return
	}

	// Snapshot under lock so we do not hold the mutex while writing to the output writer.
	r.mu.Lock()
	defer r.mu.Unlock()

	r.requestsTotal++
	r.requestDuration.observe(duration.Seconds())

	switch outcome {
	case "accepted":
		r.acceptedTotal++
	case "rejected":
		r.rejectedTotal++
		switch reason {
		case "invalid_request":
			r.rejectedInvalidRequest++
		case "duplicate_reference":
			r.rejectedDuplicateReference++
		case "invalid_recipient":
			r.rejectedInvalidRecipient++
		case "invalid_message":
			r.rejectedInvalidMessage++
		case "provider_failure":
			r.rejectedProviderFailure++
			r.providerFailures++
		case "unregistered_token":
			r.rejectedUnregisteredToken++
		}
	}
}

// ObserveProviderCall records provider call duration and error classification.
func (r *Registry) ObserveProviderCall(duration time.Duration, err error, panicRecovered bool) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.providerDuration.observe(duration.Seconds())
	if panicRecovered {
		r.providerPanics++
	}
	if err != nil && errors.Is(err, context.DeadlineExceeded) {
		r.providerTimeouts++
	}
}

// WritePrometheus writes current metrics in Prometheus exposition format.
func (r *Registry) WritePrometheus(w io.Writer) {
	if r == nil {
		return
	}

	r.mu.Lock()
	providerName := r.providerName
	requestsTotal := r.requestsTotal
	acceptedTotal := r.acceptedTotal
	rejectedTotal := r.rejectedTotal
	rejectedInvalidRequest := r.rejectedInvalidRequest
	rejectedDuplicateReference := r.rejectedDuplicateReference
	rejectedInvalidRecipient := r.rejectedInvalidRecipient
	rejectedInvalidMessage := r.rejectedInvalidMessage
	rejectedProviderFailure := r.rejectedProviderFailure
	rejectedUnregisteredToken := r.rejectedUnregisteredToken
	providerFailures := r.providerFailures
	providerTimeouts := r.providerTimeouts
	providerPanics := r.providerPanics
	requestDuration := histogram{
		buckets: append([]float64(nil), r.requestDuration.buckets...),
		counts:  append([]uint64(nil), r.requestDuration.counts...),
		count:   r.requestDuration.count,
		sum:     r.requestDuration.sum,
	}
	providerDuration := histogram{
		buckets: append([]float64(nil), r.providerDuration.buckets...),
		counts:  append([]uint64(nil), r.providerDuration.counts...),
		count:   r.providerDuration.count,
		sum:     r.providerDuration.sum,
	}
	r.mu.Unlock()

	providerLabel := fmt.Sprintf("provider=%q", providerName)

	fmt.Fprintf(w, "# HELP gateway_requests_total Total gateway requests.\n")
	fmt.Fprintf(w, "# TYPE gateway_requests_total counter\n")
	fmt.Fprintf(w, "gateway_requests_total{%s} %d\n", providerLabel, requestsTotal)

	fmt.Fprintf(w, "# HELP gateway_outcomes_total Gateway outcomes by status.\n")
	fmt.Fprintf(w, "# TYPE gateway_outcomes_total counter\n")
	fmt.Fprintf(w, "gateway_outcomes_total{%s,outcome=%q} %d\n", providerLabel, "accepted", acceptedTotal)
	fmt.Fprintf(w, "gateway_outcomes_total{%s,outcome=%q} %d\n", providerLabel, "rejected", rejectedTotal)

	fmt.Fprintf(w, "# HELP gateway_rejections_total Gateway rejections by reason.\n")
	fmt.Fprintf(w, "# TYPE gateway_rejections_total counter\n")
	fmt.Fprintf(w, "gateway_rejections_total{%s,reason=%q} %d\n", providerLabel, "invalid_request", rejectedInvalidRequest)
	fmt.Fprintf(w, "gateway_rejections_total{%s,reason=%q} %d\n", providerLabel, "duplicate_reference", rejectedDuplicateReference)
	fmt.Fprintf(w, "gateway_rejections_total{%s,reason=%q} %d\n", providerLabel, "invalid_recipient", rejectedInvalidRecipient)
	fmt.Fprintf(w, "gateway_rejections_total{%s,reason=%q} %d\n", providerLabel, "invalid_message", rejectedInvalidMessage)
	fmt.Fprintf(w, "gateway_rejections_total{%s,reason=%q} %d\n", providerLabel, "provider_failure", rejectedProviderFailure)
	fmt.Fprintf(w, "gateway_rejections_total{%s,reason=%q} %d\n", providerLabel, "unregistered_token", rejectedUnregisteredToken)

	fmt.Fprintf(w, "# HELP gateway_provider_failures_total Provider failures.\n")
	fmt.Fprintf(w, "# TYPE gateway_provider_failures_total counter\n")
	fmt.Fprintf(w, "gateway_provider_failures_total{%s} %d\n", providerLabel, providerFailures)

	fmt.Fprintf(w, "# HELP gateway_provider_timeouts_total Provider timeouts.\n")
	fmt.Fprintf(w, "# TYPE gateway_provider_timeouts_total counter\n")
	fmt.Fprintf(w, "gateway_provider_timeouts_total{%s} %d\n", providerLabel, providerTimeouts)

	fmt.Fprintf(w, "# HELP gateway_provider_panics_total Provider panics recovered.\n")
	fmt.Fprintf(w, "# TYPE gateway_provider_panics_total counter\n")
	fmt.Fprintf(w, "gateway_provider_panics_total{%s} %d\n", providerLabel, providerPanics)

	writeHistogram(w, "gateway_request_duration_seconds", "Gateway request duration in seconds.", providerLabel, requestDuration)
	writeHistogram(w, "gateway_provider_duration_seconds", "Provider call duration in seconds.", providerLabel, providerDuration)
}

func newHistogram(buckets []float64) histogram {
	return histogram{
		buckets: buckets,
		counts:  make([]uint64, len(buckets)),
	}
}

func (h *histogram) observe(value float64) {
	h.count++
	h.sum += value
	for i, bound := range h.buckets {
		if value <= bound {
			h.counts[i]++
		}
	}
}

func writeHistogram(w io.Writer, name, help, providerLabel string, h histogram) {
	fmt.Fprintf(w, "# HELP %s %s\n", name, help)
	fmt.Fprintf(w, "# TYPE %s histogram\n", name)
	for i, bound := range h.buckets {
		fmt.Fprintf(
			w,
			"%s_bucket{%s,le=%q} %d\n",
			name,
			providerLabel,
			formatFloat(bound),
			h.counts[i],
		)
	}
	fmt.Fprintf(w, "%s_bucket{%s,le=%q} %d\n", name, providerLabel, "+Inf", h.count)
	fmt.Fprintf(w, "%s_sum{%s} %s\n", name, providerLabel, formatFloat(h.sum))
	fmt.Fprintf(w, "%s_count{%s} %d\n", name, providerLabel, h.count)
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
