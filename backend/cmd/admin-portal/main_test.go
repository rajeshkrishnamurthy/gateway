package main

import (
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

func TestStripThemeToggle(t *testing.T) {
	input := `<nav><button id="theme-toggle" class="toggle">Light</button><a href="/ui">Overview</a></nav>`
	got := string(stripThemeToggle([]byte(input)))
	if strings.Contains(got, "theme-toggle") {
		t.Fatalf("expected theme toggle to be removed, got %q", got)
	}
	if !strings.Contains(got, "Overview") {
		t.Fatalf("expected navigation content to remain, got %q", got)
	}
}
