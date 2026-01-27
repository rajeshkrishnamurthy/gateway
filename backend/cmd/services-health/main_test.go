package main

import (
	"errors"
	"fmt"
	"html"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadConfigWithComments(t *testing.T) {
	content := "# comment\n{\n  \"services\": [\n    {\n      \"id\": \"sms\",\n      \"label\": \"SMS\",\n      \"instances\": [\n        {\n          \"name\": \"one\",\n          \"addr\": \":18080\",\n          \"healthUrl\": \"http://localhost:18080/readyz\"\n        }\n      ]\n    }\n  ]\n}\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "services.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(cfg.Services))
	}
	if cfg.Services[0].ID != "sms" {
		t.Fatalf("expected service id sms, got %q", cfg.Services[0].ID)
	}
}

func TestLoadConfigRequiresHealthURL(t *testing.T) {
	content := "{\n  \"services\": [\n    {\n      \"id\": \"sms\",\n      \"label\": \"SMS\",\n      \"instances\": [\n        {\n          \"name\": \"one\",\n          \"addr\": \":18080\"\n        }\n      ]\n    }\n  ]\n}\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "services.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	if _, err := loadConfig(path); err == nil {
		t.Fatal("expected error for missing healthUrl")
	}
}

func TestLoadConfigRejectsInvalidHealthURL(t *testing.T) {
	content := "{\n  \"services\": [\n    {\n      \"id\": \"sms\",\n      \"label\": \"SMS\",\n      \"instances\": [\n        {\n          \"name\": \"one\",\n          \"addr\": \":18080\",\n          \"healthUrl\": \"localhost:18080/readyz\"\n        }\n      ]\n    }\n  ]\n}\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "services.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	if _, err := loadConfig(path); err == nil {
		t.Fatal("expected error for invalid healthUrl scheme")
	}
}

func TestIsHealthUp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	if !isHealthUp(server.URL) {
		t.Fatalf("expected health up for %s", server.URL)
	}
	server.Close()
	if isHealthUp(server.URL) {
		t.Fatalf("expected health down after close")
	}
}

func TestIsHealthUpNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	if isHealthUp(server.URL) {
		t.Fatalf("expected health down for non-2xx")
	}
}

func TestResolveHealthURL(t *testing.T) {
	instance := serviceInstance{Name: "sms-1", HealthURL: "http://localhost:18080/readyz"}
	if _, err := resolveHealthURL(instance); err != nil {
		t.Fatalf("expected valid healthUrl, got %v", err)
	}
	instance.HealthURL = ""
	if _, err := resolveHealthURL(instance); err == nil {
		t.Fatal("expected error for empty healthUrl")
	}
	instance.HealthURL = "localhost:18080/readyz"
	if _, err := resolveHealthURL(instance); err == nil {
		t.Fatal("expected error for invalid scheme")
	}
}

func TestWaitForHealthDown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	go func() {
		time.Sleep(50 * time.Millisecond)
		server.Close()
	}()
	if err := waitForHealthDown(server.URL, 500*time.Millisecond); err != nil {
		t.Fatalf("waitForHealthDown: %v", err)
	}
}

func TestWaitForHealthUp(t *testing.T) {
	ready := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	exitCh := make(chan error, 1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		ready = true
	}()
	if err := waitForHealthUp(server.URL, 500*time.Millisecond, exitCh); err != nil {
		t.Fatalf("waitForHealthUp: %v", err)
	}
}

func TestWaitForHealthUpExitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	exitCh := make(chan error, 1)
	exitCh <- errors.New("start failed")
	if err := waitForHealthUp(server.URL, 200*time.Millisecond, exitCh); err == nil {
		t.Fatal("expected error for exit failure")
	}
}

func TestSingleToggleUsesToggleInstance(t *testing.T) {
	upServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upServer.Close()
	downServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	downServer.Close()

	cfg := fileConfig{
		Services: []serviceConfig{
			{
				ID:             "haproxy",
				Label:          "HAProxy",
				SingleToggle:   true,
				ToggleInstance: "primary",
				Instances: []serviceInstance{
					{Name: "primary", Addr: ":18080", HealthURL: downServer.URL},
					{Name: "secondary", Addr: ":18081", HealthURL: upServer.URL},
				},
			},
		},
	}
	view := buildServicesView(cfg, "", nil)
	if len(view.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(view.Services))
	}
	if view.Services[0].ToggleIsUp {
		t.Fatalf("expected toggle to be down when toggle instance is down")
	}
}

func TestResolveConfigPath(t *testing.T) {
	baseDir := t.TempDir()
	configDir := filepath.Join(baseDir, "conf")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	configFile := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(configFile, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	full, display, err := resolveConfigPath(baseDir, configDir, "conf/config.json")
	if err != nil {
		t.Fatalf("resolveConfigPath: %v", err)
	}
	if full != configFile {
		t.Fatalf("expected full path %q, got %q", configFile, full)
	}
	if display != filepath.ToSlash("conf/config.json") {
		t.Fatalf("expected display path conf/config.json, got %q", display)
	}
	if _, _, err := resolveConfigPath(baseDir, configDir, "other/config.json"); err == nil {
		t.Fatalf("expected error for path outside conf")
	}
}

func TestResolveConfigPathRejectsAbsolute(t *testing.T) {
	baseDir := t.TempDir()
	configDir := filepath.Join(baseDir, "conf")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	if _, _, err := resolveConfigPath(baseDir, configDir, filepath.Join(baseDir, "conf", "x.json")); err == nil {
		t.Fatal("expected error for absolute path")
	}
}

func TestResolveConfigPathRejectsDir(t *testing.T) {
	baseDir := t.TempDir()
	configDir := filepath.Join(baseDir, "conf")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	if _, _, err := resolveConfigPath(baseDir, configDir, "conf"); err == nil {
		t.Fatal("expected error for config dir path")
	}
}

func TestBuildCommandReplacesPlaceholders(t *testing.T) {
	args := []string{"run", "--config", "{config}", "--addr", "{addr}", "--port", "{port}"}
	out, err := buildCommand(args, "conf/x.json", ":8080", "8080")
	if err != nil {
		t.Fatalf("buildCommand: %v", err)
	}
	if strings.Join(out, " ") != "run --config conf/x.json --addr :8080 --port 8080" {
		t.Fatalf("unexpected command: %v", out)
	}
}

func TestBuildCommandMissingPlaceholders(t *testing.T) {
	if _, err := buildCommand([]string{"run", "{config}"}, "", ":1", "1"); err == nil {
		t.Fatal("expected error for missing config")
	}
	if _, err := buildCommand([]string{"run", "{addr}"}, "conf/x.json", "", "1"); err == nil {
		t.Fatal("expected error for missing addr")
	}
	if _, err := buildCommand([]string{"run", "{port}"}, "conf/x.json", ":1", ""); err == nil {
		t.Fatal("expected error for missing port")
	}
}

func TestSplitAddr(t *testing.T) {
	host, port, err := splitAddr(":8080")
	if err != nil {
		t.Fatalf("splitAddr: %v", err)
	}
	if host != "127.0.0.1" || port != "8080" {
		t.Fatalf("unexpected host/port: %s/%s", host, port)
	}
	host, port, err = splitAddr("localhost:9090")
	if err != nil {
		t.Fatalf("splitAddr: %v", err)
	}
	if host != "localhost" || port != "9090" {
		t.Fatalf("unexpected host/port: %s/%s", host, port)
	}
}

func TestHandleRoot(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handleRoot(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect, got %d", rr.Code)
	}
	if rr.Header().Get("Location") != "/ui" {
		t.Fatalf("expected /ui redirect, got %q", rr.Header().Get("Location"))
	}

	req = httptest.NewRequest(http.MethodPost, "/", nil)
	rr = httptest.NewRecorder()
	handleRoot(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
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

func TestHandleOverviewRenderShell(t *testing.T) {
	ui := newTestUIServer(t, fileConfig{}, t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/ui", nil)
	rr := httptest.NewRecorder()
	ui.handleOverview(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "<!doctype html>") {
		t.Fatalf("expected full page shell, got %q", body)
	}
	if !strings.Contains(body, "overview Command Center") {
		t.Fatalf("expected overview content, got %q", body)
	}
	if !strings.Contains(body, "toggle") {
		t.Fatalf("expected theme toggle in non-embed view")
	}
}

func TestHandleOverviewEmbed(t *testing.T) {
	ui := newTestUIServer(t, fileConfig{}, t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/ui?embed=1", nil)
	rr := httptest.NewRecorder()
	ui.handleOverview(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if strings.Contains(rr.Body.String(), "toggle") {
		t.Fatalf("expected theme toggle stripped in embed view")
	}
}

func TestHandleServices(t *testing.T) {
	ui := newTestUIServer(t, fileConfig{}, t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/ui/services", nil)
	rr := httptest.NewRecorder()
	ui.handleServices(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "services 0") {
		t.Fatalf("expected services fragment, got %q", rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("expected html content type, got %q", ct)
	}
}

func TestHandleConfigSuccess(t *testing.T) {
	dir := t.TempDir()
	confDir := filepath.Join(dir, "conf")
	if err := os.Mkdir(confDir, 0o755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	configFile := filepath.Join(confDir, "svc.json")
	if err := os.WriteFile(configFile, []byte("{\"ok\":true}"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg := fileConfig{
		Services: []serviceConfig{
			{
				ID:                "svc",
				Label:             "Service",
				DefaultConfigPath: "conf/svc.json",
				Instances: []serviceInstance{
					{Name: "one", Addr: ":18080", HealthURL: "http://localhost/readyz"},
				},
			},
		},
	}
	ui := newTestUIServer(t, cfg, dir)
	req := httptest.NewRequest(http.MethodGet, "/ui/config?serviceId=svc&instanceName=one", nil)
	rr := httptest.NewRecorder()
	ui.handleConfig(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "conf/svc.json") {
		t.Fatalf("expected display path, got %q", body)
	}
	if !strings.Contains(body, html.EscapeString("{\"ok\":true}")) {
		t.Fatalf("expected config content, got %q", body)
	}
}

func TestHandleConfigMissingParams(t *testing.T) {
	ui := newTestUIServer(t, fileConfig{}, t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/ui/config", nil)
	rr := httptest.NewRecorder()
	ui.handleConfig(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "service and instance are required") {
		t.Fatalf("expected error message, got %q", rr.Body.String())
	}
}

func TestHandleConfigInvalidPath(t *testing.T) {
	dir := t.TempDir()
	confDir := filepath.Join(dir, "conf")
	if err := os.Mkdir(confDir, 0o755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	cfg := fileConfig{
		Services: []serviceConfig{
			{
				ID:                "svc",
				Label:             "Service",
				DefaultConfigPath: "config.json",
				Instances: []serviceInstance{
					{Name: "one", Addr: ":18080", HealthURL: "http://localhost/readyz"},
				},
			},
		},
	}
	ui := newTestUIServer(t, cfg, dir)
	req := httptest.NewRequest(http.MethodGet, "/ui/config?serviceId=svc&instanceName=one", nil)
	rr := httptest.NewRecorder()
	ui.handleConfig(rr, req)
	if !strings.Contains(rr.Body.String(), "config path must be under conf/") {
		t.Fatalf("expected config path error, got %q", rr.Body.String())
	}
}

func TestHandleConfigClear(t *testing.T) {
	ui := newTestUIServer(t, fileConfig{}, t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/ui/config/clear", nil)
	rr := httptest.NewRecorder()
	ui.handleConfigClear(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "hidden") {
		t.Fatalf("expected hidden response, got %q", rr.Body.String())
	}
}

func TestHandleStartAndStopWithoutCommands(t *testing.T) {
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer health.Close()
	cfg := fileConfig{
		Services: []serviceConfig{
			{
				ID:        "svc",
				Label:     "Service",
				Instances: []serviceInstance{{Name: "one", Addr: ":18080", HealthURL: health.URL}},
			},
		},
	}
	ui := newTestUIServer(t, cfg, t.TempDir())

	req := httptest.NewRequest(http.MethodPost, "/ui/services/start", strings.NewReader("serviceId=svc&instanceName=one"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	ui.handleStart(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "command not configured") {
		t.Fatalf("expected command error, got %q", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/ui/services/stop", strings.NewReader("serviceId=svc&instanceName=one"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	ui.handleStop(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "command not configured") {
		t.Fatalf("expected command error, got %q", rr.Body.String())
	}
}

func TestRunActionStartSuccess(t *testing.T) {
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer health.Close()
	cfg := fileConfig{
		Services: []serviceConfig{
			{
				ID:           "sms",
				Label:        "SMS Gateway",
				StartCommand: helperCommand("start"),
				Instances: []serviceInstance{
					{Name: "one", Addr: ":18080", HealthURL: health.URL},
				},
			},
		},
	}
	ui := newTestUIServer(t, cfg, t.TempDir())
	result := ui.runAction("sms", "one", "", "", true)
	if result.desiredUp == nil || !*result.desiredUp {
		t.Fatalf("expected desired up true, got %#v", result.desiredUp)
	}
	if !strings.Contains(result.notice, "started") {
		t.Fatalf("expected started notice, got %q", result.notice)
	}
}

func TestRunActionStopSuccess(t *testing.T) {
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer health.Close()
	cfg := fileConfig{
		Services: []serviceConfig{
			{
				ID:          "sms",
				Label:       "SMS Gateway",
				StopCommand: helperCommand("stop"),
				Instances: []serviceInstance{
					{Name: "one", Addr: ":18080", HealthURL: health.URL},
				},
			},
		},
	}
	ui := newTestUIServer(t, cfg, t.TempDir())
	result := ui.runAction("sms", "one", "", "", false)
	if result.desiredUp == nil || *result.desiredUp {
		t.Fatalf("expected desired up false, got %#v", result.desiredUp)
	}
	if !strings.Contains(result.notice, "stopped") {
		t.Fatalf("expected stopped notice, got %q", result.notice)
	}
}

func TestRunActionStartFailure(t *testing.T) {
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer health.Close()
	cfg := fileConfig{
		Services: []serviceConfig{
			{
				ID:           "sms",
				Label:        "SMS Gateway",
				StartCommand: helperCommand("fail"),
				Instances: []serviceInstance{
					{Name: "one", Addr: ":18080", HealthURL: health.URL},
				},
			},
		},
	}
	ui := newTestUIServer(t, cfg, t.TempDir())
	result := ui.runAction("sms", "one", "", "", true)
	if result.desiredUp != nil {
		t.Fatalf("expected no desired state on failure")
	}
	if !strings.Contains(result.notice, "start failed") {
		t.Fatalf("expected start failed notice, got %q", result.notice)
	}
}

func TestFormatStartError(t *testing.T) {
	long := strings.Repeat("x", maxStartOutput+10)
	got := formatStartError(errors.New("boom"), long)
	if !strings.Contains(got, "start failed") {
		t.Fatalf("unexpected format: %q", got)
	}
	if !strings.Contains(got, "...") {
		t.Fatalf("expected truncated output, got %q", got)
	}
}

func TestSummarizeOutput(t *testing.T) {
	got := summarizeOutput(" a\nb\n")
	if got != "a b" {
		t.Fatalf("unexpected summarize output: %q", got)
	}
}

func TestFindServiceInstanceErrors(t *testing.T) {
	cfg := fileConfig{Services: []serviceConfig{{ID: "svc", Instances: []serviceInstance{{Name: "one"}}}}}
	if _, _, err := findServiceInstance(cfg, "missing", "one"); err == nil {
		t.Fatal("expected service not found error")
	}
	if _, _, err := findServiceInstance(cfg, "svc", "missing"); err == nil {
		t.Fatal("expected instance not found error")
	}
}

func TestSplitAddrInvalid(t *testing.T) {
	if _, _, err := splitAddr("bad:addr:thing"); err == nil {
		t.Fatal("expected error for invalid address")
	}
}

func TestRenderFragmentError(t *testing.T) {
	bad := template.Must(template.New("health_services.tmpl").Parse(`{{template "missing"}}`))
	rr := httptest.NewRecorder()
	renderFragment(rr, bad, "health_services.tmpl", servicesView{})
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestNewUIServer(t *testing.T) {
	dir := t.TempDir()
	uiDir := filepath.Join(dir, "ui")
	if err := os.Mkdir(uiDir, 0o755); err != nil {
		t.Fatalf("mkdir ui: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "conf"), 0o755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	writeTemplate := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(uiDir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write template %s: %v", name, err)
		}
	}
	writeTemplate("health_overview.tmpl", `{{define "health_overview.tmpl"}}ok{{end}}`)
	writeTemplate("health_services.tmpl", `{{define "health_services.tmpl"}}ok{{end}}`)
	writeTemplate("health_config.tmpl", `{{define "health_config.tmpl"}}ok{{end}}`)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(cwd)
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	ui, err := newUIServer(fileConfig{})
	if err != nil {
		t.Fatalf("newUIServer: %v", err)
	}
	if ui.staticDir == "" {
		t.Fatal("expected staticDir to be set")
	}
}

func newTestUIServer(t *testing.T, cfg fileConfig, workingDir string) *uiServer {
	t.Helper()
	overview := template.Must(template.New("health_overview.tmpl").Parse(`{{define "health_overview.tmpl"}}overview {{.Title}} {{if .ShowThemeToggle}}toggle{{end}}{{end}}`))
	services := template.Must(template.New("health_services.tmpl").Parse(`{{define "health_services.tmpl"}}services {{len .Services}} {{.Notice}}{{end}}`))
	config := template.Must(template.New("health_config.tmpl").Parse(`{{define "health_config.tmpl"}}config {{if .Hidden}}hidden{{end}} {{.ServiceLabel}} {{.InstanceName}} {{.DisplayPath}} {{.Content}} {{.Error}}{{end}}`))
	return &uiServer{
		templates: uiTemplates{
			overview: overview,
			services: services,
			config:   config,
		},
		staticDir:  filepath.Join(workingDir, "static"),
		config:     cfg,
		title:      "Command Center",
		workingDir: workingDir,
		configDir:  filepath.Join(workingDir, "conf"),
	}
}

func helperCommand(mode string) []string {
	return []string{os.Args[0], "-test.run=TestHelperProcess", "--", mode}
}

func TestHelperProcess(t *testing.T) {
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			switch os.Args[i+1] {
			case "start":
				fmt.Fprint(os.Stdout, "started")
				os.Exit(0)
			case "stop":
				fmt.Fprint(os.Stdout, "stopped")
				os.Exit(0)
			case "fail":
				fmt.Fprint(os.Stderr, "boom")
				os.Exit(1)
			}
		}
	}
}
