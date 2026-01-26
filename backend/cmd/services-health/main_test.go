package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"
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
