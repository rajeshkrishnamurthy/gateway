package main

import (
	"context"
	"encoding/json"
	"gateway/adapter"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const testServiceAccountKey = `-----BEGIN RSA PRIVATE KEY-----
MIICXQIBAAKBgQDdSqc0uEYXOkWnyboe07kNh66M+67dMMXYVt+11uzxsyAA6ZbY
iZCRkQr7gC9U0meFK0D+z7kJc8psNyFS0UwTQBNVhezqId36ZeZdjAOBaMOZRpHw
yGi+S2dkrdTMvJ/AV1fWG20snh5WxrJXud5wdEnatMCkUdbDsg6bjEpqlwIDAQAB
AoGAH5I1BLp9lXbE1UlcemVuc1W2O3r02a3JrDHIvOKq71jE6hxpXv9RVtNAo90H
46wZBNDE9xWfqo+Qg5vh7zTZC2oWSlPOcZVHR4g6oa9I2X7m00ovqD49hFrsiqb+
PU9BFBNKmSVEnCPlDV3lh9wx1+0m3+UHlmebqw3FI17PKiECQQDl7n86FjLT1vDN
tQfLs/LfF24WoLS5mV60ivmlEr0M2lRYo5vGnbguoXLPZzupgRl1mPMkIPU7fEMv
7RVRhOgpAkEA9mFjvK05o8HMqSnj2qKo/k0Z6Q7K+4qUT3wpqg4BFGiDegDvCnQv
6vADz1lokfz1xQ/wKi7lKCX4OkgLL84UvwJAXL3f70v44F0376DvLgi9E6Ldsp7L
hnkILAZKP3zZaA/AKaiEMo53NcfFCUb4V5xM6pPwrkfk4kNyzifwi1ryUQJBAKTX
tCtgmtf9qjjkVhbKDddXLqbHxvdVWLV1lUq54+8LnivaxBRyeDzwKRxp7ZT/clBO
wZj3l0qtXM9htFpfv3ECQQC3CBBul4EZYHdklwmxBN6k24DhfLIwFcYg6Akgu8rW
FVT6atOuC42ZpkUrQ5wJbyqPqOAKwInVTSae5qlCFXGU
-----END RSA PRIVATE KEY-----
`

func writeServiceAccountFile(t *testing.T, tokenURI string) string {
	t.Helper()
	creds := serviceAccountJSON{
		ClientEmail: "test@example.com",
		PrivateKey:  testServiceAccountKey,
		TokenURI:    tokenURI,
	}
	raw, err := json.Marshal(creds)
	if err != nil {
		t.Fatalf("marshal credentials: %v", err)
	}
	path := filepath.Join(t.TempDir(), "creds.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	return path
}

func TestLoadConfigAllowsHashComments(t *testing.T) {
	config := `# top comment
{
  "pushProvider": "fcm",
  "pushProviderUrl": "http://localhost:9095/push/send",
  "pushProviderConnectTimeoutSeconds": 2,
  "pushProviderTimeoutSeconds": 30
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
	if cfg.PushProvider != "fcm" {
		t.Fatalf("expected pushProvider fcm, got %q", cfg.PushProvider)
	}
	if cfg.PushProviderURL != "http://localhost:9095/push/send" {
		t.Fatalf("expected pushProviderUrl, got %q", cfg.PushProviderURL)
	}
}

func TestLoadConfigAllowsHashInString(t *testing.T) {
	config := `{
  "pushProvider": "fcm",
  "pushProviderUrl": "http://localhost:9095/push/send#frag",
  "pushProviderConnectTimeoutSeconds": 2,
  "pushProviderTimeoutSeconds": 30
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
	if cfg.PushProviderURL != "http://localhost:9095/push/send#frag" {
		t.Fatalf("expected pushProviderUrl with #, got %q", cfg.PushProviderURL)
	}
}

func TestLoadConfigInvalidJSON(t *testing.T) {
	config := `# comment
{
  "pushProvider": "fcm"
  "pushProviderUrl": "http://localhost:9095/push/send",
  "pushProviderConnectTimeoutSeconds": 2,
  "pushProviderTimeoutSeconds": 30
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

func TestProviderFromConfigPushFCMMissingEnv(t *testing.T) {
	t.Setenv("PUSH_FCM_CREDENTIAL_JSON_PATH", "")
	t.Setenv("PUSH_FCM_BEARER_TOKEN", "")
	cfg := fileConfig{
		PushProvider:    "fcm",
		PushProviderURL: "http://localhost",
	}
	_, _, err := providerFromConfig(cfg, time.Second)
	if err == nil {
		t.Fatal("expected error for missing PUSH_FCM_CREDENTIAL_JSON_PATH or PUSH_FCM_BEARER_TOKEN")
	}
}

func TestProviderFromConfigPushFCMWithEnv(t *testing.T) {
	t.Setenv("PUSH_FCM_CREDENTIAL_JSON_PATH", "")
	t.Setenv("PUSH_FCM_BEARER_TOKEN", "secret")
	cfg := fileConfig{
		PushProvider:    "fcm",
		PushProviderURL: "http://localhost",
	}
	providerCall, providerName, err := providerFromConfig(cfg, time.Second)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if providerCall == nil {
		t.Fatal("expected providerCall")
	}
	if providerName != adapter.PushFCMProviderName {
		t.Fatalf("expected provider name %q, got %q", adapter.PushFCMProviderName, providerName)
	}
}

func TestProviderFromConfigPushFCMWithCredentialPath(t *testing.T) {
	credsPath := writeServiceAccountFile(t, "http://localhost/token")
	t.Setenv("PUSH_FCM_CREDENTIAL_JSON_PATH", credsPath)
	t.Setenv("PUSH_FCM_BEARER_TOKEN", "")
	cfg := fileConfig{
		PushProvider:    "fcm",
		PushProviderURL: "http://localhost",
	}
	providerCall, providerName, err := providerFromConfig(cfg, time.Second)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if providerCall == nil {
		t.Fatal("expected providerCall")
	}
	if providerName != adapter.PushFCMProviderName {
		t.Fatalf("expected provider name %q, got %q", adapter.PushFCMProviderName, providerName)
	}
}

func TestFCMTokenSourceToken(t *testing.T) {
	var hitCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Errorf("expected content-type application/x-www-form-urlencoded, got %q", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if got := r.PostForm.Get("grant_type"); got != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			t.Errorf("unexpected grant_type: %q", got)
		}
		if got := r.PostForm.Get("assertion"); got == "" {
			t.Errorf("expected assertion")
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"access_token":"token-1","expires_in":3600}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	credsPath := writeServiceAccountFile(t, server.URL)
	source, err := newFCMTokenSource(credsPath, "", time.Second)
	if err != nil {
		t.Fatalf("new token source: %v", err)
	}

	token, err := source.Token(context.Background())
	if err != nil {
		t.Fatalf("first token: %v", err)
	}
	if token != "token-1" {
		t.Fatalf("expected token-1, got %q", token)
	}

	token, err = source.Token(context.Background())
	if err != nil {
		t.Fatalf("second token: %v", err)
	}
	if token != "token-1" {
		t.Fatalf("expected token-1, got %q", token)
	}
	if hitCount != 1 {
		t.Fatalf("expected 1 token request, got %d", hitCount)
	}
}
