package deliverytracking

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"sync"

	mssql "github.com/microsoft/go-mssqldb"

	"gateway/submission"
)

const (
	// Processing failure stage labels.
	ProcessingStageWebhookIngestion   = "webhook_ingestion"
	ProcessingStageCorrelationHandoff = "correlation_handoff"
	ProcessingStageCoreProcessing     = "core_processing"
	ProcessingStageFreshnessEvaluator = "freshness_evaluator"
	ProcessingStageDeliveryReadAPI    = "delivery_read_api"

	// Processing failure reason labels.
	ProcessingFailureReasonInvalidPayload        = "invalid_payload"
	ProcessingFailureReasonStorageUnavailable    = "storage_unavailable"
	ProcessingFailureReasonTimeout               = "timeout"
	ProcessingFailureReasonDeadlock              = "deadlock"
	ProcessingFailureReasonDependencyUnavailable = "dependency_unavailable"
	ProcessingFailureReasonInternalError         = "internal_error"
	ProcessingFailureReasonUnknownReason         = "unknown_reason"

	// Read API endpoint labels.
	DeliveryReadEndpointCurrent = "/v1/intents/{intentId}/delivery"
	DeliveryReadEndpointHistory = "/v1/intents/{intentId}/delivery/history"
)

var canonicalDeliveryStatuses = []string{
	string(DeliveryStatusUnknown),
	string(DeliveryStatusInProgress),
	string(DeliveryStatusDelivered),
	string(DeliveryStatusFailed),
}

var canonicalDeliveryFreshnesses = []string{
	string(DeliveryFreshnessFresh),
	string(DeliveryFreshnessStale),
	string(DeliveryFreshnessNotApplicable),
}

var canonicalIgnoredReasons = []string{
	deliveryNoOpReasonModeOff,
	deliveryNoOpReasonCorrelationUnmatched,
	deliveryNoOpReasonCorrelationInvalid,
	deliveryNoOpReasonDuplicateSource,
}

var canonicalFailureStages = []string{
	ProcessingStageWebhookIngestion,
	ProcessingStageCorrelationHandoff,
	ProcessingStageCoreProcessing,
	ProcessingStageFreshnessEvaluator,
	ProcessingStageDeliveryReadAPI,
}

var canonicalFailureReasons = []string{
	ProcessingFailureReasonInvalidPayload,
	ProcessingFailureReasonStorageUnavailable,
	ProcessingFailureReasonTimeout,
	ProcessingFailureReasonDeadlock,
	ProcessingFailureReasonDependencyUnavailable,
	ProcessingFailureReasonInternalError,
	ProcessingFailureReasonUnknownReason,
}

var canonicalReadCodeLabels = []string{"200", "400", "404", "405", "500", "503", "other"}
var canonicalCorrelationResults = []string{string(DeliveryCorrelationMatched), string(DeliveryCorrelationUnmatched), string(DeliveryCorrelationInvalid)}

type providerSignalKey struct {
	source            string
	correlationResult string
}

type statusTransitionKey struct {
	from string
	to   string
}

type freshnessTransitionKey struct {
	from string
	to   string
}

type processingFailureKey struct {
	stage  string
	reason string
}

type readAPIRequestKey struct {
	endpoint string
	code     string
}

// ProcessingStageError preserves the processing stage while retaining the original error.
type ProcessingStageError struct {
	Stage string
	Err   error
}

func (e ProcessingStageError) Error() string {
	if e.Err == nil {
		return "processing stage error"
	}
	return e.Err.Error()
}

func (e ProcessingStageError) Unwrap() error {
	return e.Err
}

// WrapProcessingStageError annotates errors with a bounded processing stage.
func WrapProcessingStageError(stage string, err error) error {
	if err == nil {
		return nil
	}
	return ProcessingStageError{
		Stage: canonicalFailureStage(stage),
		Err:   err,
	}
}

// ProcessingFailureStageFromError extracts a bounded stage from an error, falling back when absent.
func ProcessingFailureStageFromError(err error, fallback string) string {
	var stageErr ProcessingStageError
	if errors.As(err, &stageErr) {
		return canonicalFailureStage(stageErr.Stage)
	}
	return canonicalFailureStage(fallback)
}

// MapProcessingFailureReason maps arbitrary errors into bounded processing failure reasons.
func MapProcessingFailureReason(err error) string {
	if err == nil {
		return ProcessingFailureReasonUnknownReason
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ProcessingFailureReasonTimeout
	}
	if errors.Is(err, sql.ErrConnDone) {
		return ProcessingFailureReasonStorageUnavailable
	}

	var sqlErr mssql.Error
	if errors.As(err, &sqlErr) {
		switch sqlErr.Number {
		case 1205:
			return ProcessingFailureReasonDeadlock
		case -2:
			return ProcessingFailureReasonTimeout
		case 53, 64, 233, 4060, 40197, 40501, 40613:
			return ProcessingFailureReasonDependencyUnavailable
		default:
			return ProcessingFailureReasonStorageUnavailable
		}
	}

	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "deadlock"):
		return ProcessingFailureReasonDeadlock
	case strings.Contains(message, "timeout"):
		return ProcessingFailureReasonTimeout
	case strings.Contains(message, "connection refused"),
		strings.Contains(message, "broken pipe"),
		strings.Contains(message, "no such host"),
		strings.Contains(message, "network is unreachable"),
		strings.Contains(message, "i/o timeout"):
		return ProcessingFailureReasonDependencyUnavailable
	default:
		return ProcessingFailureReasonInternalError
	}
}

// Metrics tracks delivery-tracking metrics for Prometheus exposition.
type Metrics struct {
	mu sync.Mutex

	db *sql.DB

	providerSignals      map[providerSignalKey]uint64
	statusTransitions    map[statusTransitionKey]uint64
	freshnessTransitions map[freshnessTransitionKey]uint64
	ignoredSignals       map[string]uint64
	processingFailures   map[processingFailureKey]uint64
	readAPIRequests      map[readAPIRequestKey]uint64
}

// NewMetrics constructs a delivery-tracking metrics registry.
func NewMetrics(db *sql.DB) *Metrics {
	return &Metrics{
		db:                   db,
		providerSignals:      make(map[providerSignalKey]uint64),
		statusTransitions:    make(map[statusTransitionKey]uint64),
		freshnessTransitions: make(map[freshnessTransitionKey]uint64),
		ignoredSignals:       make(map[string]uint64),
		processingFailures:   make(map[processingFailureKey]uint64),
		readAPIRequests:      make(map[readAPIRequestKey]uint64),
	}
}

// ObserveProviderSignal increments provider signal classification counters.
func (m *Metrics) ObserveProviderSignal(source string, correlationResult DeliveryCorrelationResult) {
	if m == nil {
		return
	}
	key := providerSignalKey{
		source:            canonicalIngressSource(source),
		correlationResult: canonicalCorrelationResult(correlationResult),
	}
	m.mu.Lock()
	m.providerSignals[key]++
	m.mu.Unlock()
}

// ObserveStatusTransition increments delivery status transition counters.
func (m *Metrics) ObserveStatusTransition(from, to DeliveryStatus) {
	if m == nil {
		return
	}
	fromValue := canonicalDeliveryStatus(from)
	toValue := canonicalDeliveryStatus(to)
	if fromValue == toValue {
		return
	}
	key := statusTransitionKey{from: fromValue, to: toValue}
	m.mu.Lock()
	m.statusTransitions[key]++
	m.mu.Unlock()
}

// ObserveFreshnessTransition increments delivery freshness transition counters.
func (m *Metrics) ObserveFreshnessTransition(from, to DeliveryFreshness) {
	m.ObserveFreshnessTransitions(from, to, 1)
}

// ObserveFreshnessTransitions increments freshness transitions by a caller-supplied count.
func (m *Metrics) ObserveFreshnessTransitions(from, to DeliveryFreshness, count uint64) {
	if m == nil || count == 0 {
		return
	}
	fromValue := canonicalDeliveryFreshness(from)
	toValue := canonicalDeliveryFreshness(to)
	if fromValue == toValue {
		return
	}
	key := freshnessTransitionKey{from: fromValue, to: toValue}
	m.mu.Lock()
	m.freshnessTransitions[key] += count
	m.mu.Unlock()
}

// ObserveIgnoredSignal increments ignored/no-op signal counters.
func (m *Metrics) ObserveIgnoredSignal(reason string) {
	if m == nil {
		return
	}
	reasonValue := canonicalIgnoredReason(reason)
	m.mu.Lock()
	m.ignoredSignals[reasonValue]++
	m.mu.Unlock()
}

// ObserveProcessingFailure increments processing failure counters.
func (m *Metrics) ObserveProcessingFailure(stage, reason string) {
	if m == nil {
		return
	}
	key := processingFailureKey{
		stage:  canonicalFailureStage(stage),
		reason: canonicalFailureReason(reason),
	}
	m.mu.Lock()
	m.processingFailures[key]++
	m.mu.Unlock()
}

// ObserveProcessingFailureFromError maps and records processing failures from an error.
func (m *Metrics) ObserveProcessingFailureFromError(stage string, err error) {
	if m == nil || err == nil {
		return
	}
	m.ObserveProcessingFailure(stage, MapProcessingFailureReason(err))
}

// ObserveReadAPIRequest increments request/response telemetry counters for delivery read endpoints.
func (m *Metrics) ObserveReadAPIRequest(endpoint string, statusCode int) {
	if m == nil {
		return
	}
	key := readAPIRequestKey{
		endpoint: canonicalReadEndpoint(endpoint),
		code:     canonicalReadCode(statusCode),
	}
	m.mu.Lock()
	m.readAPIRequests[key]++
	m.mu.Unlock()
}

// WritePrometheus writes delivery-tracking metrics in Prometheus exposition format.
func (m *Metrics) WritePrometheus(w io.Writer) {
	if m == nil {
		return
	}

	m.mu.Lock()
	providerSignals := make(map[providerSignalKey]uint64, len(m.providerSignals))
	for k, v := range m.providerSignals {
		providerSignals[k] = v
	}
	statusTransitions := make(map[statusTransitionKey]uint64, len(m.statusTransitions))
	for k, v := range m.statusTransitions {
		statusTransitions[k] = v
	}
	freshnessTransitions := make(map[freshnessTransitionKey]uint64, len(m.freshnessTransitions))
	for k, v := range m.freshnessTransitions {
		freshnessTransitions[k] = v
	}
	ignoredSignals := make(map[string]uint64, len(m.ignoredSignals))
	for k, v := range m.ignoredSignals {
		ignoredSignals[k] = v
	}
	processingFailures := make(map[processingFailureKey]uint64, len(m.processingFailures))
	for k, v := range m.processingFailures {
		processingFailures[k] = v
	}
	readAPIRequests := make(map[readAPIRequestKey]uint64, len(m.readAPIRequests))
	for k, v := range m.readAPIRequests {
		readAPIRequests[k] = v
	}
	m.mu.Unlock()

	statusCounts := m.loadCurrentStatusCounts(context.Background())

	fmt.Fprintf(w, "# HELP delivery_provider_signals_total Total provider signal classifications.\n")
	fmt.Fprintf(w, "# TYPE delivery_provider_signals_total counter\n")
	for _, correlationResult := range canonicalCorrelationResults {
		key := providerSignalKey{
			source:            webhookIngressSourceProviderSignalWebhook,
			correlationResult: correlationResult,
		}
		fmt.Fprintf(
			w,
			"delivery_provider_signals_total{source=%q,correlation_result=%q} %d\n",
			key.source,
			key.correlationResult,
			providerSignals[key],
		)
	}

	fmt.Fprintf(w, "# HELP delivery_status_current_count Current delivery status counts from durable state.\n")
	fmt.Fprintf(w, "# TYPE delivery_status_current_count gauge\n")
	for _, status := range canonicalDeliveryStatuses {
		fmt.Fprintf(w, "delivery_status_current_count{delivery_status=%q} %d\n", status, statusCounts[status])
	}

	fmt.Fprintf(w, "# HELP delivery_status_transitions_total Total committed delivery status transitions.\n")
	fmt.Fprintf(w, "# TYPE delivery_status_transitions_total counter\n")
	for _, from := range canonicalDeliveryStatuses {
		for _, to := range canonicalDeliveryStatuses {
			key := statusTransitionKey{from: from, to: to}
			fmt.Fprintf(
				w,
				"delivery_status_transitions_total{from_delivery_status=%q,to_delivery_status=%q} %d\n",
				from,
				to,
				statusTransitions[key],
			)
		}
	}

	fmt.Fprintf(w, "# HELP delivery_freshness_transitions_total Total committed delivery freshness transitions.\n")
	fmt.Fprintf(w, "# TYPE delivery_freshness_transitions_total counter\n")
	for _, from := range canonicalDeliveryFreshnesses {
		for _, to := range canonicalDeliveryFreshnesses {
			key := freshnessTransitionKey{from: from, to: to}
			fmt.Fprintf(
				w,
				"delivery_freshness_transitions_total{from_delivery_freshness=%q,to_delivery_freshness=%q} %d\n",
				from,
				to,
				freshnessTransitions[key],
			)
		}
	}

	fmt.Fprintf(w, "# HELP delivery_ignored_signals_total Total ignored delivery signals.\n")
	fmt.Fprintf(w, "# TYPE delivery_ignored_signals_total counter\n")
	for _, reason := range canonicalIgnoredReasons {
		fmt.Fprintf(w, "delivery_ignored_signals_total{reason=%q} %d\n", reason, ignoredSignals[reason])
	}

	fmt.Fprintf(w, "# HELP delivery_processing_failures_total Total delivery processing failures by stage and reason.\n")
	fmt.Fprintf(w, "# TYPE delivery_processing_failures_total counter\n")
	for _, stage := range canonicalFailureStages {
		for _, reason := range canonicalFailureReasons {
			key := processingFailureKey{stage: stage, reason: reason}
			fmt.Fprintf(w, "delivery_processing_failures_total{stage=%q,reason=%q} %d\n", stage, reason, processingFailures[key])
		}
	}

	fmt.Fprintf(w, "# HELP delivery_read_api_requests_total Total delivery read API requests by endpoint and response code.\n")
	fmt.Fprintf(w, "# TYPE delivery_read_api_requests_total counter\n")
	for _, endpoint := range []string{DeliveryReadEndpointCurrent, DeliveryReadEndpointHistory} {
		for _, code := range canonicalReadCodeLabels {
			key := readAPIRequestKey{endpoint: endpoint, code: code}
			fmt.Fprintf(w, "delivery_read_api_requests_total{endpoint=%q,code=%q} %d\n", endpoint, code, readAPIRequests[key])
		}
	}
}

func (m *Metrics) loadCurrentStatusCounts(ctx context.Context) map[string]int64 {
	counts := map[string]int64{
		string(DeliveryStatusUnknown):    0,
		string(DeliveryStatusInProgress): 0,
		string(DeliveryStatusDelivered):  0,
		string(DeliveryStatusFailed):     0,
	}
	if m == nil || m.db == nil {
		return counts
	}

	row := m.db.QueryRowContext(
		ctx,
		`SELECT
       COUNT(1) AS tracked_total,
       SUM(CASE WHEN s.delivery_status = @p2 THEN 1 ELSE 0 END) AS in_progress_count,
       SUM(CASE WHEN s.delivery_status = @p3 THEN 1 ELSE 0 END) AS delivered_count,
       SUM(CASE WHEN s.delivery_status = @p4 THEN 1 ELSE 0 END) AS failed_count
     FROM dbo.submission_intents i
     LEFT JOIN dbo.intent_delivery_state s ON s.intent_id = i.intent_id
     WHERE i.delivery_tracking_mode = @p1`,
		string(submission.DeliveryTrackingModeOn),
		string(DeliveryStatusInProgress),
		string(DeliveryStatusDelivered),
		string(DeliveryStatusFailed),
	)

	var (
		trackedTotal    int64
		inProgressCount sql.NullInt64
		deliveredCount  sql.NullInt64
		failedCount     sql.NullInt64
	)
	if err := row.Scan(&trackedTotal, &inProgressCount, &deliveredCount, &failedCount); err != nil {
		log.Printf(
			"event=delivery_metrics_snapshot_failed stage=%q reason=%q source=%q endpoint=%q",
			ProcessingStageCoreProcessing,
			MapProcessingFailureReason(err),
			webhookIngressSourceProviderSignalWebhook,
			"/metrics",
		)
		return counts
	}

	inProgress := nullInt64(inProgressCount)
	delivered := nullInt64(deliveredCount)
	failed := nullInt64(failedCount)
	unknown := trackedTotal - inProgress - delivered - failed
	if unknown < 0 {
		unknown = 0
	}
	counts[string(DeliveryStatusUnknown)] = unknown
	counts[string(DeliveryStatusInProgress)] = inProgress
	counts[string(DeliveryStatusDelivered)] = delivered
	counts[string(DeliveryStatusFailed)] = failed
	return counts
}

func canonicalIngressSource(source string) string {
	if strings.TrimSpace(source) == webhookIngressSourceProviderSignalWebhook {
		return webhookIngressSourceProviderSignalWebhook
	}
	return webhookIngressSourceProviderSignalWebhook
}

func canonicalCorrelationResult(value DeliveryCorrelationResult) string {
	switch value {
	case DeliveryCorrelationMatched:
		return string(DeliveryCorrelationMatched)
	case DeliveryCorrelationUnmatched:
		return string(DeliveryCorrelationUnmatched)
	case DeliveryCorrelationInvalid:
		return string(DeliveryCorrelationInvalid)
	default:
		return string(DeliveryCorrelationInvalid)
	}
}

func canonicalDeliveryStatus(value DeliveryStatus) string {
	switch value {
	case DeliveryStatusUnknown:
		return string(DeliveryStatusUnknown)
	case DeliveryStatusInProgress:
		return string(DeliveryStatusInProgress)
	case DeliveryStatusDelivered:
		return string(DeliveryStatusDelivered)
	case DeliveryStatusFailed:
		return string(DeliveryStatusFailed)
	default:
		return string(DeliveryStatusUnknown)
	}
}

func canonicalDeliveryFreshness(value DeliveryFreshness) string {
	switch value {
	case DeliveryFreshnessFresh:
		return string(DeliveryFreshnessFresh)
	case DeliveryFreshnessStale:
		return string(DeliveryFreshnessStale)
	case DeliveryFreshnessNotApplicable:
		return string(DeliveryFreshnessNotApplicable)
	default:
		return string(DeliveryFreshnessNotApplicable)
	}
}

func canonicalIgnoredReason(reason string) string {
	switch reason {
	case deliveryNoOpReasonModeOff:
		return deliveryNoOpReasonModeOff
	case deliveryNoOpReasonCorrelationUnmatched:
		return deliveryNoOpReasonCorrelationUnmatched
	case deliveryNoOpReasonCorrelationInvalid:
		return deliveryNoOpReasonCorrelationInvalid
	case deliveryNoOpReasonDuplicateSource:
		return deliveryNoOpReasonDuplicateSource
	default:
		return deliveryNoOpReasonCorrelationInvalid
	}
}

func canonicalFailureStage(stage string) string {
	switch stage {
	case ProcessingStageWebhookIngestion:
		return ProcessingStageWebhookIngestion
	case ProcessingStageCorrelationHandoff:
		return ProcessingStageCorrelationHandoff
	case ProcessingStageCoreProcessing:
		return ProcessingStageCoreProcessing
	case ProcessingStageFreshnessEvaluator:
		return ProcessingStageFreshnessEvaluator
	case ProcessingStageDeliveryReadAPI:
		return ProcessingStageDeliveryReadAPI
	default:
		return ProcessingStageCoreProcessing
	}
}

func canonicalFailureReason(reason string) string {
	switch reason {
	case ProcessingFailureReasonInvalidPayload:
		return ProcessingFailureReasonInvalidPayload
	case ProcessingFailureReasonStorageUnavailable:
		return ProcessingFailureReasonStorageUnavailable
	case ProcessingFailureReasonTimeout:
		return ProcessingFailureReasonTimeout
	case ProcessingFailureReasonDeadlock:
		return ProcessingFailureReasonDeadlock
	case ProcessingFailureReasonDependencyUnavailable:
		return ProcessingFailureReasonDependencyUnavailable
	case ProcessingFailureReasonInternalError:
		return ProcessingFailureReasonInternalError
	case ProcessingFailureReasonUnknownReason:
		return ProcessingFailureReasonUnknownReason
	default:
		return ProcessingFailureReasonUnknownReason
	}
}

func canonicalReadEndpoint(endpoint string) string {
	switch endpoint {
	case DeliveryReadEndpointCurrent:
		return DeliveryReadEndpointCurrent
	case DeliveryReadEndpointHistory:
		return DeliveryReadEndpointHistory
	default:
		return DeliveryReadEndpointCurrent
	}
}

func canonicalReadCode(statusCode int) string {
	switch statusCode {
	case 200, 400, 404, 405, 500, 503:
		return strconv.Itoa(statusCode)
	default:
		return "other"
	}
}

func nullInt64(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}
