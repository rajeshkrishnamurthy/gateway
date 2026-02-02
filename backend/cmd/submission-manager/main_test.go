package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/microsoft/go-mssqldb"

	"gateway/submission"
	"gateway/submissionmanager"
)

type stubExecutor struct{}

func (s stubExecutor) Exec(ctx context.Context, input submissionmanager.AttemptInput) (submissionmanager.GatewayOutcome, error) {
	return submissionmanager.GatewayOutcome{}, nil
}

func newTestManager(t *testing.T, db *sql.DB) *submissionmanager.Manager {
	t.Helper()
	contract := submission.TargetContract{
		SubmissionTarget: "sms.realtime",
		GatewayType:      submission.GatewaySMS,
		GatewayURL:       "http://localhost:8080",
		Policy:           submission.PolicyMaxAttempts,
		MaxAttempts:      3,
		TerminalOutcomes: []string{"invalid_request"},
	}
	registry := submission.Registry{Targets: map[string]submission.TargetContract{contract.SubmissionTarget: contract}}
	manager, err := submissionmanager.NewManager(registry, stubExecutor{}.Exec, submissionmanager.Clock{}, db)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	return manager
}

func TestSubmitIntentIdempotent(t *testing.T) {
	db := newTestDB(t)
	manager := newTestManager(t, db)
	server := &apiServer{manager: manager}

	body := `{"intentId":"intent-1","submissionTarget":"sms.realtime","payload":{"to":"+1","message":"hello"}}`

	req := httptest.NewRequest(http.MethodPost, "/v1/intents", strings.NewReader(body))
	rr := httptest.NewRecorder()
	server.handleSubmit(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/intents", strings.NewReader(body))
	rr = httptest.NewRecorder()
	server.handleSubmit(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected idempotent 200, got %d", rr.Code)
	}

	conflictBody := `{"intentId":"intent-1","submissionTarget":"sms.realtime","payload":{"to":"+1","message":"changed"}}`
	req = httptest.NewRequest(http.MethodPost, "/v1/intents", strings.NewReader(conflictBody))
	rr = httptest.NewRecorder()
	server.handleSubmit(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rr.Code)
	}
}

func TestGetIntent(t *testing.T) {
	db := newTestDB(t)
	manager := newTestManager(t, db)
	server := &apiServer{manager: manager}

	body := `{"intentId":"intent-1","submissionTarget":"sms.realtime","payload":{"to":"+1","message":"hello"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/intents", strings.NewReader(body))
	rr := httptest.NewRecorder()
	server.handleSubmit(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/intents/intent-1", nil)
	rr = httptest.NewRecorder()
	server.handleGet(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp intentResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.IntentID != "intent-1" {
		t.Fatalf("expected intentId intent-1, got %q", resp.IntentID)
	}
	if resp.SubmissionTarget != "sms.realtime" {
		t.Fatalf("expected submissionTarget sms.realtime, got %q", resp.SubmissionTarget)
	}
	if resp.Status == "" {
		t.Fatalf("expected status to be set")
	}
}

func TestGetIntentHistory(t *testing.T) {
	db := newTestDB(t)
	manager := newTestManager(t, db)
	server := &apiServer{manager: manager}

	body := `{"intentId":"intent-1","submissionTarget":"sms.realtime","payload":{"to":"+1","message":"hello"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/intents", strings.NewReader(body))
	rr := httptest.NewRecorder()
	server.handleSubmit(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/intents/intent-1/history", nil)
	rr = httptest.NewRecorder()
	server.handleGet(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp intentHistoryResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Intent.IntentID != "intent-1" {
		t.Fatalf("expected intentId intent-1, got %q", resp.Intent.IntentID)
	}
	if resp.Intent.SubmissionTarget != "sms.realtime" {
		t.Fatalf("expected submissionTarget sms.realtime, got %q", resp.Intent.SubmissionTarget)
	}
	if len(resp.Attempts) != 0 {
		t.Fatalf("expected 0 attempts, got %d", len(resp.Attempts))
	}
}

func TestGetIntentNotFound(t *testing.T) {
	db := newTestDB(t)
	manager := newTestManager(t, db)
	server := &apiServer{manager: manager}

	req := httptest.NewRequest(http.MethodGet, "/v1/intents/missing", nil)
	rr := httptest.NewRecorder()
	server.handleGet(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestSubmitUnknownTarget(t *testing.T) {
	db := newTestDB(t)
	manager := newTestManager(t, db)
	server := &apiServer{manager: manager}

	body := `{"intentId":"intent-1","submissionTarget":"unknown.target","payload":{"to":"+1","message":"hello"}}`

	req := httptest.NewRequest(http.MethodPost, "/v1/intents", strings.NewReader(body))
	rr := httptest.NewRecorder()
	server.handleSubmit(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestSubmitWaitSecondsInvalid(t *testing.T) {
	db := newTestDB(t)
	manager := newTestManager(t, db)
	server := &apiServer{manager: manager}

	body := `{"intentId":"intent-1","submissionTarget":"sms.realtime","payload":{"to":"+1","message":"hello"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/intents?waitSeconds=bad", strings.NewReader(body))
	rr := httptest.NewRecorder()
	server.handleSubmit(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestSubmitWaitSecondsNegative(t *testing.T) {
	db := newTestDB(t)
	manager := newTestManager(t, db)
	server := &apiServer{manager: manager}

	body := `{"intentId":"intent-1","submissionTarget":"sms.realtime","payload":{"to":"+1","message":"hello"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/intents?waitSeconds=-1", strings.NewReader(body))
	rr := httptest.NewRecorder()
	server.handleSubmit(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestSubmitWaitSecondsTimeout(t *testing.T) {
	db := newTestDB(t)
	manager := newTestManager(t, db)
	server := &apiServer{manager: manager}

	body := `{"intentId":"intent-1","submissionTarget":"sms.realtime","payload":{"to":"+1","message":"hello"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/intents?waitSeconds=1", strings.NewReader(body))
	rr := httptest.NewRecorder()
	start := time.Now()
	server.handleSubmit(rr, req)
	elapsed := time.Since(start)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if elapsed < 900*time.Millisecond {
		t.Fatalf("expected wait before timeout, elapsed %s", elapsed)
	}

	var resp intentResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != string(submissionmanager.IntentPending) {
		t.Fatalf("expected pending status, got %q", resp.Status)
	}
}

func TestSubmitWaitSecondsEarlyReturn(t *testing.T) {
	db := newTestDB(t)
	manager := newTestManager(t, db)
	server := &apiServer{manager: manager}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go manager.Run(ctx)

	body := `{"intentId":"intent-1","submissionTarget":"sms.realtime","payload":{"to":"+1","message":"hello"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/intents?waitSeconds=5", strings.NewReader(body))
	rr := httptest.NewRecorder()
	start := time.Now()
	server.handleSubmit(rr, req)
	elapsed := time.Since(start)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if elapsed > 4*time.Second {
		t.Fatalf("expected early return, elapsed %s", elapsed)
	}

	var resp intentResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != string(submissionmanager.IntentPending) {
		t.Fatalf("expected pending status after first attempt, got %q", resp.Status)
	}
}

func TestHandleMetrics(t *testing.T) {
	metrics := submissionmanager.NewMetrics()
	metrics.ObserveIntentCreated()
	handler := handleMetrics(metrics)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "submission_intents_created_total") {
		t.Fatalf("expected metrics output, got %q", rr.Body.String())
	}
}

func TestHandleMetricsNotConfigured(t *testing.T) {
	handler := handleMetrics(nil)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandleHealthzAndReadyz(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	handleHealthz(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/healthz", nil)
	rr = httptest.NewRecorder()
	handleHealthz(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr = httptest.NewRecorder()
	handleReadyz(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleHistoryResults(t *testing.T) {
	db := newTestDB(t)
	manager := newTestManager(t, db)
	server := &apiServer{manager: manager}

	body := `{"intentId":"intent-1","submissionTarget":"sms.realtime","payload":{"to":"+1","message":"hello"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/intents", strings.NewReader(body))
	rr := httptest.NewRecorder()
	server.handleSubmit(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	ui := newTestUIServerWithManager(manager)
	form := strings.NewReader("intentId=intent-1")
	req = httptest.NewRequest(http.MethodPost, "/ui/history", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	ui.handleHistory(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "intent-1") {
		t.Fatalf("expected intentId in response, got %q", rr.Body.String())
	}
}

func newTestUIServer() *managerUIServer {
	history := template.Must(template.New("manager_history_results.tmpl").Parse(`{{define "manager_history_results.tmpl"}}history {{.IntentID}}{{end}}`))
	return &managerUIServer{
		templates: managerTemplates{
			historyResults: history,
		},
	}
}

func newTestUIServerWithManager(manager *submissionmanager.Manager) *managerUIServer {
	ui := newTestUIServer()
	ui.manager = manager
	return ui
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	password, ok, err := resolveSQLPassword(t)
	if err != nil {
		t.Fatalf("resolve sql password: %v", err)
	}
	if !ok {
		t.Skip("MSSQL_SA_PASSWORD not set; start docker compose and set env or backend/.env")
	}

	host := envOrDefault("MSSQL_HOST", "localhost")
	port := envOrDefault("MSSQL_PORT", "1433")

	masterDSN, err := buildSQLServerDSN(host, port, "sa", password, "master", "disable")
	if err != nil {
		t.Fatalf("build master dsn: %v", err)
	}
	masterDB, err := sql.Open("sqlserver", masterDSN)
	if err != nil {
		t.Fatalf("open master db: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	if err := masterDB.PingContext(ctx); err != nil {
		_ = masterDB.Close()
		t.Fatalf("ping master db: %v", err)
	}

	dbName := fmt.Sprintf("submissionmanager_http_test_%d", time.Now().UnixNano())
	if _, err := masterDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE [%s]", dbName)); err != nil {
		_ = masterDB.Close()
		t.Fatalf("create database: %v", err)
	}

	testDSN, err := buildSQLServerDSN(host, port, "sa", password, dbName, "disable")
	if err != nil {
		_ = dropTestDB(ctx, masterDB, dbName)
		t.Fatalf("build test dsn: %v", err)
	}
	db, err := sql.Open("sqlserver", testDSN)
	if err != nil {
		_ = dropTestDB(ctx, masterDB, dbName)
		t.Fatalf("open test db: %v", err)
	}

	schemaPath := filepath.Join(moduleRoot(t), "conf", "sql", "submissionmanager", "001_create_schema.sql")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		_ = db.Close()
		_ = dropTestDB(ctx, masterDB, dbName)
		t.Fatalf("read schema: %v", err)
	}
	if _, err := db.ExecContext(ctx, string(schema)); err != nil {
		_ = db.Close()
		_ = dropTestDB(ctx, masterDB, dbName)
		t.Fatalf("apply schema: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
		_ = dropTestDB(context.Background(), masterDB, dbName)
		_ = masterDB.Close()
	})

	return db
}

func dropTestDB(ctx context.Context, masterDB *sql.DB, dbName string) error {
	_, _ = masterDB.ExecContext(ctx, fmt.Sprintf("ALTER DATABASE [%s] SET SINGLE_USER WITH ROLLBACK IMMEDIATE", dbName))
	_, err := masterDB.ExecContext(ctx, fmt.Sprintf("DROP DATABASE [%s]", dbName))
	return err
}

func resolveSQLPassword(t *testing.T) (string, bool, error) {
	t.Helper()
	if value, ok := os.LookupEnv("MSSQL_SA_PASSWORD"); ok && strings.TrimSpace(value) != "" {
		return value, true, nil
	}

	data, err := os.ReadFile(filepath.Join(moduleRoot(t), ".env"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "MSSQL_SA_PASSWORD" && value != "" {
			return strings.Trim(value, "\"'"), true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", false, err
	}
	return "", false, nil
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve module root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
