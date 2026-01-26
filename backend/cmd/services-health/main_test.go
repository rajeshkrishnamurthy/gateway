package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigWithComments(t *testing.T) {
	content := "# comment\n{\n  \"services\": [\n    {\n      \"id\": \"sms\",\n      \"label\": \"SMS\",\n      \"instances\": [\n        {\n          \"name\": \"one\",\n          \"addr\": \":18080\"\n        }\n      ]\n    }\n  ]\n}\n"
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

func TestIsAddrUp(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	if !isAddrUp(addr) {
		_ = listener.Close()
		t.Fatalf("expected addr up: %s", addr)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if isAddrUp(addr) {
		t.Fatalf("expected addr down after close: %s", addr)
	}
}

func TestWaitForAddrDown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = listener.Close()
	}()
	if err := waitForAddrDown(addr, 500*time.Millisecond); err != nil {
		t.Fatalf("waitForAddrDown: %v", err)
	}
}

func TestSingleToggleUsesToggleInstance(t *testing.T) {
	upListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen up: %v", err)
	}
	defer upListener.Close()
	upAddr := upListener.Addr().String()

	downListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen down: %v", err)
	}
	downAddr := downListener.Addr().String()
	if err := downListener.Close(); err != nil {
		t.Fatalf("close down: %v", err)
	}

	cfg := fileConfig{
		Services: []serviceConfig{
			{
				ID:             "haproxy",
				Label:          "HAProxy",
				SingleToggle:   true,
				ToggleInstance: "primary",
				Instances: []serviceInstance{
					{Name: "primary", Addr: downAddr},
					{Name: "secondary", Addr: upAddr},
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
