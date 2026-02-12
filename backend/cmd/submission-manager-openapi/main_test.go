package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWritesStdout(t *testing.T) {
	var out bytes.Buffer
	if err := run(nil, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("expected stdout output")
	}
	if !strings.Contains(out.String(), `"openapi": "3.0.`) {
		t.Fatalf("expected openapi field in output, got %q", out.String())
	}
}

func TestRunWritesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openapi.json")
	if err := run([]string{"-out", path}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected generated file content")
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("output file must contain valid json: %v", err)
	}
}
