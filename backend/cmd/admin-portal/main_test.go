package main

import (
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRewriteUIPaths(t *testing.T) {
	input := `<a href="/ui" hx-get="/ui/send" hx-post="/ui/troubleshoot"></a>`
	got := string(rewriteUIPaths([]byte(input), "sms"))
	want := `<a href="/sms/ui" hx-get="/sms/ui/send" hx-post="/sms/ui/troubleshoot"></a>`
	if got != want {
		t.Fatalf("rewriteUIPaths mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestRewriteUIPathsNoPrefix(t *testing.T) {
	input := `<a href="/ui" hx-get="/ui/send"></a>`
	got := string(rewriteUIPaths([]byte(input), ""))
	if got != input {
		t.Fatalf("expected input unchanged, got %q", got)
	}
}

func TestRewriteSubmissionCopy(t *testing.T) {
	input := "Manual submission to the gateway. This mirrors POST /sms/send and returns the raw response.\n" +
		"No retry, no send again, no history. Use referenceId values you can trace in logs.\n" +
		"<label for=\"referenceId\">referenceId</label>\n" +
		"<h2>Gateway response</h2>\n" +
		"Submit a request to see status, reason, and gatewayMessageId.\n" +
		"Accepted means submitted, not delivered. This console does not infer delivery or retries."
	output := string(rewriteSubmissionCopy([]byte(input), "/sms/send"))
	if !strings.Contains(output, "SubmissionManager owns retries and history.") {
		t.Fatalf("expected submission manager copy, got %q", output)
	}
	if !strings.Contains(output, "intentId") {
		t.Fatalf("expected intentId label, got %q", output)
	}
	if strings.Contains(output, "Gateway response") {
		t.Fatalf("expected gateway response label removed, got %q", output)
	}
}

func TestParseHAProxyCSV(t *testing.T) {
	csvData := "# pxname,svname,scur,status,lastchg\n" +
		"sms_gateway,FRONTEND,2,OPEN,30\n" +
		"sms_backends,BACKEND,0,UP,10\n" +
		"sms_backends,sms1,0,UP,5\n" +
		"sms_backends,sms2,0,DOWN,8\n"

	frontends, backends, err := parseHAProxyCSV([]byte(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(frontends) != 1 {
		t.Fatalf("expected 1 frontend, got %d", len(frontends))
	}
	if frontends[0].Name != "sms_gateway" {
		t.Fatalf("unexpected frontend name: %q", frontends[0].Name)
	}
	if frontends[0].StatusClass != "status-up" {
		t.Fatalf("unexpected frontend status class: %q", frontends[0].StatusClass)
	}
	if len(backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(backends))
	}
	if backends[0].Name != "sms_backends" {
		t.Fatalf("unexpected backend name: %q", backends[0].Name)
	}
	if backends[0].ServersUp != 1 || backends[0].ServersTotal != 2 {
		t.Fatalf("unexpected backend server counts: %d/%d", backends[0].ServersUp, backends[0].ServersTotal)
	}
}

func TestParseHAProxyCSVEmpty(t *testing.T) {
	if _, _, err := parseHAProxyCSV([]byte("")); err == nil {
		t.Fatal("expected error for empty CSV")
	}
}

func TestParseHAProxyCSVMissingHeader(t *testing.T) {
	if _, _, err := parseHAProxyCSV([]byte("\n")); err == nil {
		t.Fatal("expected error for missing header")
	}
}

func TestStripThemeToggle(t *testing.T) {
	input := `<nav><button id="theme-toggle" class="toggle">Light</button><a href="/command-center/ui">Command Center</a></nav>`
	got := string(stripThemeToggle([]byte(input)))
	if strings.Contains(got, "theme-toggle") {
		t.Fatalf("expected theme toggle to be removed, got %q", got)
	}
	if !strings.Contains(got, "Command Center") {
		t.Fatalf("expected navigation content to remain, got %q", got)
	}
}

func TestStripThemeToggleMultiple(t *testing.T) {
	input := `<div><button id="theme-toggle" class="toggle">A</button><button id="theme-toggle" class="toggle">B</button></div>`
	got := string(stripThemeToggle([]byte(input)))
	if strings.Contains(got, "theme-toggle") {
		t.Fatalf("expected all theme toggles removed, got %q", got)
	}
}

func TestBuildTargetURLAddsEmbed(t *testing.T) {
	got, err := buildTargetURL("http://example.com/base", "/ui", "a=b", true)
	if err != nil {
		t.Fatalf("buildTargetURL: %v", err)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if parsed.Query().Get("embed") != "1" {
		t.Fatalf("expected embed=1, got %q", parsed.RawQuery)
	}
	if parsed.Query().Get("a") != "b" {
		t.Fatalf("expected query preserved, got %q", parsed.RawQuery)
	}
}

func TestBuildTargetURLNoEmbed(t *testing.T) {
	got, err := buildTargetURL("http://example.com/base", "/ui", "", false)
	if err != nil {
		t.Fatalf("buildTargetURL: %v", err)
	}
	if strings.Contains(got, "embed=") {
		t.Fatalf("unexpected embed query: %q", got)
	}
}

func TestNormalizeConfig(t *testing.T) {
	cfg := fileConfig{
		Title:                         "  Portal  ",
		SMSGatewayURL:                 "http://sms.example.com/",
		PushGatewayURL:                "http://push.example.com///",
		SubmissionManagerDashboardURL: " http://grafana.example.com/d/submission ",
		CommandCenterURL:              "http://cc.example.com/ ",
		HAProxyStatsURL:               " http://haproxy.example.com/stats;csv ",
	}
	got := normalizeConfig(cfg)
	if got.Title != "Portal" {
		t.Fatalf("expected trimmed title, got %q", got.Title)
	}
	if got.SMSGatewayURL != "http://sms.example.com" {
		t.Fatalf("unexpected sms url: %q", got.SMSGatewayURL)
	}
	if got.PushGatewayURL != "http://push.example.com" {
		t.Fatalf("unexpected push url: %q", got.PushGatewayURL)
	}
	if got.SubmissionManagerDashboardURL != "http://grafana.example.com/d/submission" {
		t.Fatalf("unexpected submission manager dashboard url: %q", got.SubmissionManagerDashboardURL)
	}
	if got.CommandCenterURL != "http://cc.example.com" {
		t.Fatalf("unexpected command center url: %q", got.CommandCenterURL)
	}
	if got.HAProxyStatsURL != "http://haproxy.example.com/stats;csv" {
		t.Fatalf("unexpected haproxy url: %q", got.HAProxyStatsURL)
	}
}

func TestResolveTitleDefault(t *testing.T) {
	if got := resolveTitle(" "); got != "Setu Admin Portal" {
		t.Fatalf("expected default title, got %q", got)
	}
}

func TestBuildConsoleViews(t *testing.T) {
	cfg := fileConfig{
		SMSGatewayURL:    "http://sms",
		PushGatewayURL:   "http://push",
		CommandCenterURL: "http://cc",
		HAProxyStatsURL:  "http://haproxy/stats;csv",
	}
	consoles := buildConsoleViews(cfg)
	if len(consoles) != 4 {
		t.Fatalf("expected 4 consoles, got %d", len(consoles))
	}
}

func TestCopyHeader(t *testing.T) {
	src := http.Header{
		"Content-Type": []string{"application/json"},
		"Accept":       []string{"text/plain"},
		"X-Ignore":     []string{"nope"},
	}
	dst := http.Header{}
	copyHeader(dst, src, []string{"Content-Type", "Accept"})
	if dst.Get("Content-Type") != "application/json" {
		t.Fatalf("expected Content-Type copied, got %q", dst.Get("Content-Type"))
	}
	if dst.Get("Accept") != "text/plain" {
		t.Fatalf("expected Accept copied, got %q", dst.Get("Accept"))
	}
	if dst.Get("X-Ignore") != "" {
		t.Fatalf("expected X-Ignore not copied, got %q", dst.Get("X-Ignore"))
	}
}

func TestHandleOverviewHTMX(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{Title: "Admin"})
	req := httptest.NewRequest(http.MethodGet, "/ui", nil)
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handleOverview(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", rr.Code)
	}
	if rr.Header().Get("Location") != "/command-center/ui" {
		t.Fatalf("expected command center redirect, got %q", rr.Header().Get("Location"))
	}
}

func TestHandleHAProxyNotConfigured(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{})
	req := httptest.NewRequest(http.MethodGet, "/haproxy", nil)
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handleHAProxy(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
}

func TestHandleHAProxyConfigured(t *testing.T) {
	statsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := "# pxname,svname,scur,status,lastchg\nsms_gateway,FRONTEND,1,OPEN,2\nsms_backends,BACKEND,0,UP,1\n"
		_, _ = io.WriteString(w, body)
	}))
	defer statsServer.Close()
	server := newTestPortalServer(t, fileConfig{HAProxyStatsURL: statsServer.URL})
	req := httptest.NewRequest(http.MethodGet, "/haproxy", nil)
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handleHAProxy(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "haproxy 1 1") {
		t.Fatalf("unexpected haproxy body: %q", rr.Body.String())
	}
}

func TestProxyUIRewrite(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<a href="/ui">home</a>`)
	}))
	defer upstream.Close()
	server := newTestPortalServer(t, fileConfig{SMSGatewayURL: upstream.URL})
	req := httptest.NewRequest(http.MethodGet, "/sms/ui", nil)
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handleSMSUI(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "/sms/ui") {
		t.Fatalf("expected rewritten ui paths, got %q", body)
	}
}

func TestProxyUIEmbedAddsQueryAndStripsTheme(t *testing.T) {
	var seenEmbed bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("embed") == "1" {
			seenEmbed = true
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<button id="theme-toggle">Theme</button><div>ok</div>`)
	}))
	defer upstream.Close()
	server := newTestPortalServer(t, fileConfig{CommandCenterURL: upstream.URL})
	req := httptest.NewRequest(http.MethodGet, "/command-center/ui", nil)
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handleCommandCenterUI(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if !seenEmbed {
		t.Fatal("expected embed=1 in upstream request")
	}
	if strings.Contains(rr.Body.String(), "theme-toggle") {
		t.Fatalf("expected theme toggle stripped, got %q", rr.Body.String())
	}
}

func newTestPortalServer(t *testing.T, cfg fileConfig) *portalServer {
	t.Helper()
	topbar := template.Must(template.New("portal_topbar.tmpl").Parse(`{{define "portal_topbar.tmpl"}}topbar{{end}}`))
	overview := template.Must(template.New("portal_overview.tmpl").Parse(`{{define "portal_overview.tmpl"}}overview {{.Title}}{{end}}`))
	haproxy := template.Must(template.New("portal_haproxy.tmpl").Parse(`{{define "portal_haproxy.tmpl"}}haproxy {{len .Frontends}} {{len .Backends}} {{.Error}}{{end}}`))
	errView := template.Must(template.New("portal_error.tmpl").Parse(`{{define "portal_error.tmpl"}}error {{.Title}} {{.Message}}{{end}}`))
	troubleshoot := template.Must(template.New("portal_troubleshoot.tmpl").Parse(`{{define "portal_troubleshoot.tmpl"}}troubleshoot {{.HistoryAction}}{{end}}`))
	dashboards := template.Must(template.New("portal_dashboards.tmpl").Parse(`{{define "portal_dashboards.tmpl"}}dashboards {{.SubmissionURL}} {{.SMSGatewayURL}} {{.PushGatewayURL}}{{end}}`))
	dashboardEmbed := template.Must(template.New("portal_dashboard_embed.tmpl").Parse(`{{define "portal_dashboard_embed.tmpl"}}dashboard {{.Title}} {{.DashboardURL}}{{end}}`))
	submissionResult := template.Must(template.New("submission_result.tmpl").Parse(`{{define "submission_result.tmpl"}}submission {{.IntentID}} {{.StatusEndpoint}} {{.Status}} {{.RejectedReason}} {{.ExhaustedReason}} {{.CompletedAt}} {{.Error}}{{end}}`))
	return &portalServer{
		config: normalizeConfig(cfg),
		templates: portalTemplates{
			topbar:           topbar,
			overview:         overview,
			haproxy:          haproxy,
			errView:          errView,
			troubleshoot:     troubleshoot,
			dashboards:       dashboards,
			dashboardEmbed:   dashboardEmbed,
			submissionResult: submissionResult,
		},
		client: &http.Client{},
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

func TestHandleOverviewFullPage(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{Title: "Admin", SMSGatewayURL: "http://sms"})
	req := httptest.NewRequest(http.MethodGet, "/ui", nil)
	rr := httptest.NewRecorder()
	server.handleOverview(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rr.Code)
	}
	if rr.Header().Get("Location") != "/command-center/ui" {
		t.Fatalf("expected command center redirect, got %q", rr.Header().Get("Location"))
	}
}

func TestHandlePushUI(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<a href="/ui">home</a>`)
	}))
	defer upstream.Close()
	server := newTestPortalServer(t, fileConfig{PushGatewayURL: upstream.URL})
	req := httptest.NewRequest(http.MethodGet, "/push/ui", nil)
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handlePushUI(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "/push/ui") {
		t.Fatalf("expected rewritten paths, got %q", rr.Body.String())
	}
}

func TestHandleSMSTroubleshootPage(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{
		SMSGatewayURL:        "http://sms",
		SubmissionManagerURL: "http://manager",
	})
	req := httptest.NewRequest(http.MethodGet, "/sms/ui/troubleshoot", nil)
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handleSMSTroubleshoot(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "/sms/ui/troubleshoot/history") {
		t.Fatalf("expected history action, got %q", body)
	}
}

func TestHandlePushTroubleshootPage(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{
		PushGatewayURL:       "http://push",
		SubmissionManagerURL: "http://manager",
	})
	req := httptest.NewRequest(http.MethodGet, "/push/ui/troubleshoot", nil)
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handlePushTroubleshoot(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "/push/ui/troubleshoot/history") {
		t.Fatalf("expected history action, got %q", body)
	}
}

func TestHandleTroubleshootPage(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{
		SubmissionManagerURL: "http://manager",
	})
	req := httptest.NewRequest(http.MethodGet, "/troubleshoot", nil)
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handleTroubleshoot(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "/troubleshoot/history") {
		t.Fatalf("expected history action, got %q", body)
	}
}

func TestHandleDashboardsPage(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{
		SMSGatewayURL:                 "http://sms",
		PushGatewayURL:                "http://push",
		SubmissionManagerDashboardURL: "http://grafana/submission-manager",
	})
	req := httptest.NewRequest(http.MethodGet, "/dashboards", nil)
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handleDashboards(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "/dashboards/submission-manager") {
		t.Fatalf("expected submission manager dashboard link, got %q", body)
	}
	if !strings.Contains(body, "/sms/ui/metrics") {
		t.Fatalf("expected sms dashboard link, got %q", body)
	}
	if !strings.Contains(body, "/push/ui/metrics") {
		t.Fatalf("expected push dashboard link, got %q", body)
	}
}

func TestHandleSubmissionManagerDashboard(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{
		SubmissionManagerDashboardURL: "http://grafana/submission-manager",
	})
	req := httptest.NewRequest(http.MethodGet, "/dashboards/submission-manager", nil)
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handleSubmissionManagerDashboard(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "http://grafana/submission-manager") {
		t.Fatalf("expected dashboard url in response, got %q", rr.Body.String())
	}
}

func TestHandleSubmissionManagerDashboardNotConfigured(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{})
	req := httptest.NewRequest(http.MethodGet, "/dashboards/submission-manager", nil)
	rr := httptest.NewRecorder()
	server.handleSubmissionManagerDashboard(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandleSMSTroubleshootHistoryProxy(t *testing.T) {
	var seenIntent string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ui/history" {
			t.Fatalf("expected /ui/history, got %q", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		seenIntent = r.FormValue("intentId")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "history ok")
	}))
	defer upstream.Close()

	server := newTestPortalServer(t, fileConfig{SubmissionManagerURL: upstream.URL})
	body := strings.NewReader("intentId=abc-123")
	req := httptest.NewRequest(http.MethodPost, "/sms/ui/troubleshoot/history", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handleSMSTroubleshootHistory(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if seenIntent != "abc-123" {
		t.Fatalf("expected intentId forwarded, got %q", seenIntent)
	}
	if !strings.Contains(rr.Body.String(), "history ok") {
		t.Fatalf("expected upstream body, got %q", rr.Body.String())
	}
}

func TestHandlePushTroubleshootHistoryProxy(t *testing.T) {
	var seenIntent string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ui/history" {
			t.Fatalf("expected /ui/history, got %q", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		seenIntent = r.FormValue("intentId")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "history ok")
	}))
	defer upstream.Close()

	server := newTestPortalServer(t, fileConfig{SubmissionManagerURL: upstream.URL})
	body := strings.NewReader("intentId=abc-123")
	req := httptest.NewRequest(http.MethodPost, "/push/ui/troubleshoot/history", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handlePushTroubleshootHistory(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if seenIntent != "abc-123" {
		t.Fatalf("expected intentId forwarded, got %q", seenIntent)
	}
	if !strings.Contains(rr.Body.String(), "history ok") {
		t.Fatalf("expected upstream body, got %q", rr.Body.String())
	}
}

func TestHandleTroubleshootHistoryProxy(t *testing.T) {
	var seenIntent string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ui/history" {
			t.Fatalf("expected /ui/history, got %q", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		seenIntent = r.FormValue("intentId")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "history ok")
	}))
	defer upstream.Close()

	server := newTestPortalServer(t, fileConfig{SubmissionManagerURL: upstream.URL})
	body := strings.NewReader("intentId=abc-123")
	req := httptest.NewRequest(http.MethodPost, "/troubleshoot/history", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handleTroubleshootHistory(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if seenIntent != "abc-123" {
		t.Fatalf("expected intentId forwarded, got %q", seenIntent)
	}
	if !strings.Contains(rr.Body.String(), "history ok") {
		t.Fatalf("expected upstream body, got %q", rr.Body.String())
	}
}

func TestHandleTroubleshootHistoryProxyError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "intent not found", http.StatusNotFound)
	}))
	defer upstream.Close()

	server := newTestPortalServer(t, fileConfig{SubmissionManagerURL: upstream.URL})
	body := strings.NewReader("intentId=missing")
	req := httptest.NewRequest(http.MethodPost, "/troubleshoot/history", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handleTroubleshootHistory(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "intent not found") {
		t.Fatalf("expected error message, got %q", rr.Body.String())
	}
}

func TestHandleTroubleshootHistoryNotConfiguredHTMX(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{})
	body := strings.NewReader("intentId=missing")
	req := httptest.NewRequest(http.MethodPost, "/troubleshoot/history", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handleTroubleshootHistory(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "SubmissionManager not configured") {
		t.Fatalf("expected error message, got %q", rr.Body.String())
	}
}

func TestHandleTroubleshootHistoryNotConfiguredNonHTMX(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{})
	body := strings.NewReader("intentId=missing")
	req := httptest.NewRequest(http.MethodPost, "/troubleshoot/history", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	server.handleTroubleshootHistory(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "SubmissionManager not configured") {
		t.Fatalf("expected error message, got %q", rr.Body.String())
	}
}

func TestHandleTroubleshootHistoryMissingIntent(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{SubmissionManagerURL: "http://manager"})
	req := httptest.NewRequest(http.MethodPost, "/troubleshoot/history", nil)
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handleTroubleshootHistory(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "intentId is required") {
		t.Fatalf("expected error message, got %q", rr.Body.String())
	}
}

func TestHandleTroubleshootHistoryBadMethod(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{SubmissionManagerURL: "http://manager"})
	req := httptest.NewRequest(http.MethodGet, "/troubleshoot/history", nil)
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handleTroubleshootHistory(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "method not allowed") {
		t.Fatalf("expected error message, got %q", rr.Body.String())
	}
}

func TestHandleTroubleshootHistoryBadBaseURL(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{SubmissionManagerURL: "http://[::1"})
	body := strings.NewReader("intentId=abc-123")
	req := httptest.NewRequest(http.MethodPost, "/troubleshoot/history", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handleTroubleshootHistory(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Invalid upstream URL") {
		t.Fatalf("expected error message, got %q", rr.Body.String())
	}
}

func TestProxyUIBadMethod(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer upstream.Close()
	server := newTestPortalServer(t, fileConfig{SMSGatewayURL: upstream.URL})
	req := httptest.NewRequest(http.MethodPut, "/sms/ui", nil)
	rr := httptest.NewRecorder()
	server.handleSMSUI(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestHandleSMSAPISubmissionManager(t *testing.T) {
	var got submissionIntentRequest
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/intents" {
			t.Fatalf("expected path /v1/intents, got %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %q", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode intent: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"intentId":"intent-1","submissionTarget":"sms.realtime","createdAt":"2026-01-30T00:00:00Z","status":"pending"}`)
	}))
	defer upstream.Close()

	server := newTestPortalServer(t, fileConfig{
		SMSGatewayURL:        "http://sms",
		SubmissionManagerURL: upstream.URL,
		SMSSubmissionTarget:  "sms.realtime",
	})

	req := httptest.NewRequest(http.MethodPost, "/sms/send", strings.NewReader(`{"referenceId":"intent-1","to":"+1","message":"hello","tenantId":"tenant-a"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handleSMSAPI(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "pending") {
		t.Fatalf("expected pending status, got %q", rr.Body.String())
	}

	if got.IntentID != "intent-1" {
		t.Fatalf("expected intentId intent-1, got %q", got.IntentID)
	}
	if got.SubmissionTarget != "sms.realtime" {
		t.Fatalf("expected submissionTarget sms.realtime, got %q", got.SubmissionTarget)
	}
	var payload map[string]string
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["referenceId"] != "intent-1" || payload["to"] != "+1" || payload["message"] != "hello" || payload["tenantId"] != "tenant-a" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestHandleSMSSubmissionInvalidJSON(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{
		SubmissionManagerURL: "http://sm",
		SMSSubmissionTarget:  "sms.realtime",
	})
	req := httptest.NewRequest(http.MethodPost, "/sms/send", strings.NewReader(`{"bad":`))
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handleSMSAPI(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid request body") {
		t.Fatalf("expected error message, got %q", rr.Body.String())
	}
}

func TestHandleSMSSubmissionMissingFields(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{
		SubmissionManagerURL: "http://sm",
		SMSSubmissionTarget:  "sms.realtime",
	})
	req := httptest.NewRequest(http.MethodPost, "/sms/send", strings.NewReader(`{"referenceId":"intent-1","to":"+1"}`))
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handleSMSAPI(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "referenceId, to, and message are required") {
		t.Fatalf("expected missing fields error, got %q", rr.Body.String())
	}
}

func TestHandlePushAPISubmissionManager(t *testing.T) {
	var got submissionIntentRequest
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/intents" {
			t.Fatalf("expected path /v1/intents, got %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %q", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode intent: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"intentId":"intent-1","submissionTarget":"push.realtime","createdAt":"2026-01-30T00:00:00Z","status":"pending"}`)
	}))
	defer upstream.Close()

	server := newTestPortalServer(t, fileConfig{
		PushGatewayURL:       "http://push",
		SubmissionManagerURL: upstream.URL,
		PushSubmissionTarget: "push.realtime",
	})

	req := httptest.NewRequest(http.MethodPost, "/push/send", strings.NewReader(`{"referenceId":"intent-1","token":"abc","title":"hi","body":"there","tenantId":"tenant-a"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handlePushAPI(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "pending") {
		t.Fatalf("expected pending status, got %q", rr.Body.String())
	}

	if got.IntentID != "intent-1" {
		t.Fatalf("expected intentId intent-1, got %q", got.IntentID)
	}
	if got.SubmissionTarget != "push.realtime" {
		t.Fatalf("expected submissionTarget push.realtime, got %q", got.SubmissionTarget)
	}
	var payload map[string]string
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["referenceId"] != "intent-1" || payload["token"] != "abc" || payload["title"] != "hi" || payload["body"] != "there" || payload["tenantId"] != "tenant-a" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestHandlePushSubmissionInvalidJSON(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{
		SubmissionManagerURL: "http://sm",
		PushSubmissionTarget: "push.realtime",
	})
	req := httptest.NewRequest(http.MethodPost, "/push/send", strings.NewReader(`{"bad":`))
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handlePushAPI(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid request body") {
		t.Fatalf("expected error message, got %q", rr.Body.String())
	}
}

func TestHandlePushSubmissionMissingFields(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{
		SubmissionManagerURL: "http://sm",
		PushSubmissionTarget: "push.realtime",
	})
	req := httptest.NewRequest(http.MethodPost, "/push/send", strings.NewReader(`{"referenceId":"intent-1"}`))
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handlePushAPI(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "referenceId and token are required") {
		t.Fatalf("expected missing fields error, got %q", rr.Body.String())
	}
}

func TestHandleSMSAPIProxy(t *testing.T) {
	var gotPath, gotMethod, gotContentType string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()
	server := newTestPortalServer(t, fileConfig{SMSGatewayURL: upstream.URL})
	req := httptest.NewRequest(http.MethodPost, "/sms/send", strings.NewReader(`{"msg":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.handleSMSAPI(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != `{"ok":true}` {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}
	if gotPath != "/sms/send" {
		t.Fatalf("expected path /sms/send, got %q", gotPath)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %q", gotMethod)
	}
	if gotContentType != "application/json" {
		t.Fatalf("expected content type forwarded, got %q", gotContentType)
	}
}

func TestHandleSMSStatusSubmissionManager(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/intents/intent-1" {
			t.Fatalf("expected path /v1/intents/intent-1, got %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"intentId":"intent-1","submissionTarget":"sms.realtime","createdAt":"2026-01-30T00:00:00Z","status":"accepted","completedAt":"2026-01-30T00:00:01Z"}`)
	}))
	defer upstream.Close()

	server := newTestPortalServer(t, fileConfig{
		SubmissionManagerURL: upstream.URL,
		SMSSubmissionTarget:  "sms.realtime",
	})

	req := httptest.NewRequest(http.MethodGet, "/sms/status?intentId=intent-1", nil)
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handleSMSStatus(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "accepted") {
		t.Fatalf("expected accepted status, got %q", rr.Body.String())
	}
}

func TestHandleSMSStatusMissingIntent(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{
		SubmissionManagerURL: "http://sm",
		SMSSubmissionTarget:  "sms.realtime",
	})
	req := httptest.NewRequest(http.MethodGet, "/sms/status", nil)
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handleSMSStatus(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "intentId is required") {
		t.Fatalf("expected missing intentId error, got %q", rr.Body.String())
	}
}

func TestHandleSMSStatusNotConfigured(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{})
	req := httptest.NewRequest(http.MethodGet, "/sms/status?intentId=intent-1", nil)
	rr := httptest.NewRecorder()
	server.handleSMSStatus(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandlePushStatusSubmissionManager(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/intents/intent-1" {
			t.Fatalf("expected path /v1/intents/intent-1, got %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"intentId":"intent-1","submissionTarget":"push.realtime","createdAt":"2026-01-30T00:00:00Z","status":"accepted","completedAt":"2026-01-30T00:00:01Z"}`)
	}))
	defer upstream.Close()

	server := newTestPortalServer(t, fileConfig{
		SubmissionManagerURL: upstream.URL,
		PushSubmissionTarget: "push.realtime",
	})

	req := httptest.NewRequest(http.MethodGet, "/push/status?intentId=intent-1", nil)
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handlePushStatus(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "accepted") {
		t.Fatalf("expected accepted status, got %q", rr.Body.String())
	}
}

func TestHandlePushStatusMissingIntent(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{
		SubmissionManagerURL: "http://sm",
		PushSubmissionTarget: "push.realtime",
	})
	req := httptest.NewRequest(http.MethodGet, "/push/status", nil)
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	server.handlePushStatus(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "intentId is required") {
		t.Fatalf("expected missing intentId error, got %q", rr.Body.String())
	}
}

func TestHandlePushStatusNotConfigured(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{})
	req := httptest.NewRequest(http.MethodGet, "/push/status?intentId=intent-1", nil)
	rr := httptest.NewRecorder()
	server.handlePushStatus(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandlePushAPINotConfigured(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{})
	req := httptest.NewRequest(http.MethodPost, "/push/send", nil)
	rr := httptest.NewRecorder()
	server.handlePushAPI(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandlePushAPIBadBaseURL(t *testing.T) {
	server := newTestPortalServer(t, fileConfig{PushGatewayURL: "http://[::1"})
	req := httptest.NewRequest(http.MethodPost, "/push/send", nil)
	rr := httptest.NewRecorder()
	server.handlePushAPI(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rr.Code)
	}
}

func TestLoadConfigWithComments(t *testing.T) {
	content := "# comment\n{\n  \"title\": \"Admin\",\n  \"smsGatewayUrl\": \"http://sms\"\n}\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Title != "Admin" {
		t.Fatalf("unexpected title %q", cfg.Title)
	}
	if cfg.SMSGatewayURL != "http://sms" {
		t.Fatalf("unexpected sms url %q", cfg.SMSGatewayURL)
	}
}

func TestLoadConfigRejectsUnknownField(t *testing.T) {
	content := "{\n  \"unknown\": \"value\"\n}\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	if _, err := loadConfig(path); err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestFindUIDir(t *testing.T) {
	dir := t.TempDir()
	uiDir := filepath.Join(dir, "ui")
	if err := os.Mkdir(uiDir, 0o755); err != nil {
		t.Fatalf("mkdir ui: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uiDir, "portal_overview.tmpl"), []byte("ok"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
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
	got, err := findUIDir()
	if err != nil {
		t.Fatalf("findUIDir: %v", err)
	}
	gotResolved, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("resolve got: %v", err)
	}
	wantResolved, err := filepath.EvalSymlinks(uiDir)
	if err != nil {
		t.Fatalf("resolve want: %v", err)
	}
	if gotResolved != wantResolved {
		t.Fatalf("expected ui dir %q, got %q", wantResolved, gotResolved)
	}
}

func TestLoadPortalTemplates(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write template %s: %v", name, err)
		}
	}
	write("portal_topbar.tmpl", `{{define "portal_topbar.tmpl"}}top{{end}}`)
	write("portal_overview.tmpl", `{{define "portal_overview.tmpl"}}overview{{end}}`)
	write("portal_haproxy.tmpl", `{{define "portal_haproxy.tmpl"}}haproxy{{end}}`)
	write("portal_error.tmpl", `{{define "portal_error.tmpl"}}error{{end}}`)
	write("portal_troubleshoot.tmpl", `{{define "portal_troubleshoot.tmpl"}}troubleshoot{{end}}`)
	write("portal_dashboards.tmpl", `{{define "portal_dashboards.tmpl"}}dashboards{{end}}`)
	write("portal_dashboard_embed.tmpl", `{{define "portal_dashboard_embed.tmpl"}}dashboard{{end}}`)
	write("submission_result.tmpl", `{{define "submission_result.tmpl"}}submission{{end}}`)

	templates, err := loadPortalTemplates(dir)
	if err != nil {
		t.Fatalf("loadPortalTemplates: %v", err)
	}
	if templates.topbar == nil || templates.overview == nil || templates.haproxy == nil || templates.errView == nil || templates.troubleshoot == nil || templates.dashboards == nil || templates.dashboardEmbed == nil || templates.submissionResult == nil {
		t.Fatal("expected templates to be loaded")
	}
}
