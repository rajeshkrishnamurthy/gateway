package main

import (
	"gateway/adapter"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigAllowsHashComments(t *testing.T) {
	config := `# top comment
{
  "smsProvider": "model",
  "addr": ":8080",
  "smsProviderUrl": "http://localhost:9091/sms/send",
  "smsProviderConnectTimeoutSeconds": 2,
  "smsProviderTimeoutSeconds": 30
}
  # trailing comment
`
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.SMSProvider != "model" {
		t.Fatalf("expected smsProvider model, got %q", cfg.SMSProvider)
	}
	if cfg.SMSProviderURL != "http://localhost:9091/sms/send" {
		t.Fatalf("expected smsProviderUrl, got %q", cfg.SMSProviderURL)
	}
}

func TestLoadConfigAllowsHashInString(t *testing.T) {
	config := `{
  "smsProvider": "model",
  "addr": ":8080",
  "smsProviderUrl": "http://localhost:9091/sms/send#frag",
  "smsProviderConnectTimeoutSeconds": 2,
  "smsProviderTimeoutSeconds": 30
}
`
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.SMSProviderURL != "http://localhost:9091/sms/send#frag" {
		t.Fatalf("expected smsProviderUrl with #, got %q", cfg.SMSProviderURL)
	}
}

func TestLoadConfigInvalidJSON(t *testing.T) {
	config := `# comment
{
  "smsProvider": "model",
  "addr": ":8080"
  "smsProviderUrl": "http://localhost:9091/sms/send",
  "smsProviderConnectTimeoutSeconds": 2,
  "smsProviderTimeoutSeconds": 30
}
`
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := loadConfig(path); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestProviderFromConfigSms24X7MissingEnv(t *testing.T) {
	t.Setenv("SMS24X7_API_KEY", "")
	cfg := fileConfig{
		SMSProvider:            "sms24x7",
		SMSProviderURL:         "http://localhost",
		SMSProviderServiceName: "svc",
		SMSProviderSenderID:    "sender",
	}
	_, _, err := providerFromConfig(cfg, time.Second)
	if err == nil {
		t.Fatal("expected error for missing SMS24X7_API_KEY")
	}
}

func TestProviderFromConfigSms24X7WithEnv(t *testing.T) {
	t.Setenv("SMS24X7_API_KEY", "secret")
	cfg := fileConfig{
		SMSProvider:            "sms24x7",
		SMSProviderURL:         "http://localhost",
		SMSProviderServiceName: "svc",
		SMSProviderSenderID:    "sender",
	}
	providerCall, providerName, err := providerFromConfig(cfg, time.Second)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if providerCall == nil {
		t.Fatal("expected providerCall")
	}
	if providerName != adapter.Sms24X7ProviderName {
		t.Fatalf("expected provider name %q, got %q", adapter.Sms24X7ProviderName, providerName)
	}
}

func TestProviderFromConfigModelNoEnv(t *testing.T) {
	t.Setenv("SMS24X7_API_KEY", "")
	t.Setenv("SMSKARIX_API_KEY", "")
	cfg := fileConfig{
		SMSProvider:    "model",
		SMSProviderURL: "http://localhost",
	}
	providerCall, providerName, err := providerFromConfig(cfg, time.Second)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if providerCall == nil {
		t.Fatal("expected providerCall")
	}
	if providerName != adapter.ModelProviderName {
		t.Fatalf("expected provider name %q, got %q", adapter.ModelProviderName, providerName)
	}
}

func TestProviderFromConfigSmsKarixMissingEnv(t *testing.T) {
	t.Setenv("SMSKARIX_API_KEY", "")
	cfg := fileConfig{
		SMSProvider:         "smskarix",
		SMSProviderURL:      "http://localhost",
		SMSProviderVersion:  "v1",
		SMSProviderSenderID: "sender",
	}
	_, _, err := providerFromConfig(cfg, time.Second)
	if err == nil {
		t.Fatal("expected error for missing SMSKARIX_API_KEY")
	}
}

func TestProviderFromConfigSmsKarixWithEnv(t *testing.T) {
	t.Setenv("SMSKARIX_API_KEY", "secret")
	cfg := fileConfig{
		SMSProvider:         "smskarix",
		SMSProviderURL:      "http://localhost",
		SMSProviderVersion:  "v1",
		SMSProviderSenderID: "sender",
	}
	providerCall, providerName, err := providerFromConfig(cfg, time.Second)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if providerCall == nil {
		t.Fatal("expected providerCall")
	}
	if providerName != adapter.SmsKarixProviderName {
		t.Fatalf("expected provider name %q, got %q", adapter.SmsKarixProviderName, providerName)
	}
}
