package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"gateway"
	"gateway/adapter"
	"gateway/metrics"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const maxBodyBytes = 16 << 10

var configPath = flag.String("config", "config.json", "Gateway config file path")
var listenAddr = flag.String("addr", ":8081", "HTTP listen address")
var showHelp = flag.Bool("help", false, "show usage")
var showVersion = flag.Bool("version", false, "show version")

const version = "0.1.0"

const defaultGrafanaDashboardURL = "http://localhost:3000/d/gateway-overview-push"

const (
	minProviderConnectTimeout = 2 * time.Second
	maxProviderConnectTimeout = 10 * time.Second
)

const (
	defaultFCMScopeURL = "https://www.googleapis.com/auth/firebase.messaging"
	tokenRefreshSkew   = 2 * time.Minute
)

const (
	uiLogCapacity    = 1000
	uiLogResultLimit = 200
)

var latencyBuckets = []time.Duration{
	100 * time.Millisecond,
	250 * time.Millisecond,
	500 * time.Millisecond,
	1 * time.Second,
	2500 * time.Millisecond,
	5 * time.Second,
}

type fileConfig struct {
	PushProvider                      string `json:"pushProvider"`
	PushProviderURL                   string `json:"pushProviderUrl"`
	PushProviderTimeoutSeconds        int    `json:"pushProviderTimeoutSeconds"`
	PushProviderConnectTimeoutSeconds int    `json:"pushProviderConnectTimeoutSeconds"`
	GrafanaDashboardURL               string `json:"grafanaDashboardUrl"`
}

type uiTemplates struct {
	overview            *template.Template
	send                *template.Template
	sendResult          *template.Template
	troubleshoot        *template.Template
	troubleshootResults *template.Template
	metrics             *template.Template
}

type uiServer struct {
	templates       uiTemplates
	staticDir       string
	consoleTitle    string
	sendTitle       string
	sendNavLabel    string
	sendEndpoint    string
	metricsURL      string
	isPush          bool
	gatewayName     string
	version         string
	providerName    string
	providerTimeout time.Duration
	startTime       time.Time
	metricsRegistry *metrics.Registry
	logBuffer       *logBuffer
}

type overviewView struct {
	ConsoleTitle    string
	SendNavLabel    string
	MetricsURL      string
	GatewayName     string
	Version         string
	ProviderName    string
	ProviderTimeout string
	Uptime          string
}

type troubleshootView struct {
	SendNavLabel     string
	MetricsURL       string
	ReferenceID      string
	ProviderDecision string
	MappingDecision  string
	FinalOutcome     string
	Entries          []logEntry
}

type metricsView struct {
	SendNavLabel         string
	MetricsURL           string
	TotalRequests        string
	AcceptedTotal        string
	RejectedTotal        string
	ProviderFailureCount string
	Rejections           []rejectionCount
	RequestLatency       []latencyBucket
	ProviderLatency      []latencyBucket
}

type sendView struct {
	SendTitle    string
	SendEndpoint string
	SendNavLabel string
	MetricsURL   string
	IsPush       bool
}

type rejectionCount struct {
	Reason string
	Count  string
}

type latencyBucket struct {
	Label string
	Count string
}

type logEntry struct {
	Timestamp string
	Line      string
}

type logBuffer struct {
	mu       sync.Mutex
	capacity int
	entries  []logEntry
	partial  string
}

func main() {
	flag.Parse()
	if *showHelp {
		flag.Usage()
		return
	}
	if *showVersion {
		log.Printf("gateway version %s", version)
		return
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	startTime := time.Now()
	logBuffer := newLogBuffer(uiLogCapacity)
	log.SetOutput(io.MultiWriter(os.Stderr, logBuffer))

	providerTimeout := time.Duration(cfg.PushProviderTimeoutSeconds) * time.Second
	providerConnectTimeout := time.Duration(cfg.PushProviderConnectTimeoutSeconds) * time.Second
	providerCall, providerName, err := providerFromConfig(cfg, providerConnectTimeout)
	if err != nil {
		log.Fatal(err)
	}

	metricsRegistry := metrics.New(providerName, latencyBuckets)
	gw, err := gateway.NewPushGateway(gateway.PushConfig{
		ProviderCall:    providerCall,
		ProviderTimeout: providerTimeout,
		Metrics:         metricsRegistry,
	})
	if err != nil {
		log.Fatal(err)
	}

	ui, err := newUIServer(providerName, providerTimeout, cfg.GrafanaDashboardURL, metricsRegistry, logBuffer, startTime)
	if err != nil {
		log.Printf("ui disabled: %v", err)
	}

	server := &http.Server{
		Addr:    *listenAddr,
		Handler: newMux(gw, metricsRegistry, ui),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	log.Printf(
		"listening on %s configPath=%q pushProvider=%q pushProviderUrl=%q pushProviderTimeoutSeconds=%d pushProviderConnectTimeoutSeconds=%d grafanaDashboardUrl=%q",
		*listenAddr,
		*configPath,
		cfg.PushProvider,
		cfg.PushProviderURL,
		cfg.PushProviderTimeoutSeconds,
		cfg.PushProviderConnectTimeoutSeconds,
		cfg.GrafanaDashboardURL,
	)

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
		return
	case sig := <-sigCh:
		log.Printf("shutdown signal: %s", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Allow in-flight requests to finish before exit.
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}

	err = <-errCh
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("server error: %v", err)
	}
}

func loadConfig(path string) (fileConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return fileConfig{}, err
	}
	defer file.Close()

	var filtered bytes.Buffer
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		filtered.WriteString(line)
		filtered.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return fileConfig{}, err
	}

	dec := json.NewDecoder(&filtered)
	dec.DisallowUnknownFields()
	var cfg fileConfig
	if err := dec.Decode(&cfg); err != nil {
		return fileConfig{}, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fileConfig{}, errors.New("config has trailing data")
	}

	cfg.PushProvider = strings.TrimSpace(cfg.PushProvider)
	if cfg.PushProvider == "" {
		cfg.PushProvider = "fcm"
	}
	switch cfg.PushProvider {
	case "fcm":
	default:
		return fileConfig{}, errors.New("pushProvider must be one of: fcm")
	}
	if strings.TrimSpace(cfg.PushProviderURL) == "" {
		return fileConfig{}, errors.New("pushProviderUrl is required")
	}
	if cfg.PushProviderTimeoutSeconds < 15 || cfg.PushProviderTimeoutSeconds > 60 {
		return fileConfig{}, errors.New("pushProviderTimeoutSeconds must be between 15 and 60")
	}
	if cfg.PushProviderConnectTimeoutSeconds == 0 {
		cfg.PushProviderConnectTimeoutSeconds = int(minProviderConnectTimeout / time.Second)
	}
	connectTimeout := time.Duration(cfg.PushProviderConnectTimeoutSeconds) * time.Second
	if connectTimeout < minProviderConnectTimeout || connectTimeout > maxProviderConnectTimeout {
		return fileConfig{}, errors.New("pushProviderConnectTimeoutSeconds must be between 2 and 10")
	}

	return cfg, nil
}

func providerFromConfig(cfg fileConfig, providerConnectTimeout time.Duration) (gateway.PushProviderCall, string, error) {
	switch cfg.PushProvider {
	case "fcm":
		credentialPath := strings.TrimSpace(os.Getenv("PUSH_FCM_CREDENTIAL_JSON_PATH"))
		if credentialPath != "" {
			scope := strings.TrimSpace(os.Getenv("PUSH_FCM_SCOPE_URL"))
			tokenSource, err := newFCMTokenSource(credentialPath, scope, providerConnectTimeout)
			if err != nil {
				return nil, "", err
			}
			return adapter.PushFCMProviderCallWithTokenSource(
				cfg.PushProviderURL,
				tokenSource.Token,
				providerConnectTimeout,
			), adapter.PushFCMProviderName, nil
		}
		bearerToken := strings.TrimSpace(os.Getenv("PUSH_FCM_BEARER_TOKEN"))
		if bearerToken == "" {
			return nil, "", errors.New("PUSH_FCM_CREDENTIAL_JSON_PATH or PUSH_FCM_BEARER_TOKEN is required for fcm")
		}
		return adapter.PushFCMProviderCall(
			cfg.PushProviderURL,
			bearerToken,
			providerConnectTimeout,
		), adapter.PushFCMProviderName, nil
	default:
		return nil, "", errors.New("pushProvider must be one of: fcm")
	}
}

type serviceAccountJSON struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

type serviceAccount struct {
	email      string
	privateKey *rsa.PrivateKey
	tokenURI   string
}

type fcmTokenSource struct {
	mu      sync.Mutex
	token   string
	expiry  time.Time
	account serviceAccount
	scope   string
	client  *http.Client
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type jwtClaims struct {
	Iss   string `json:"iss"`
	Scope string `json:"scope"`
	Aud   string `json:"aud"`
	Exp   int64  `json:"exp"`
	Iat   int64  `json:"iat"`
}

func newFCMTokenSource(credentialsPath, scope string, connectTimeout time.Duration) (*fcmTokenSource, error) {
	raw, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, err
	}
	var creds serviceAccountJSON
	if err := json.Unmarshal(raw, &creds); err != nil {
		return nil, err
	}
	email := strings.TrimSpace(creds.ClientEmail)
	if email == "" {
		return nil, errors.New("client_email is required")
	}
	privateKey := strings.TrimSpace(creds.PrivateKey)
	if privateKey == "" {
		return nil, errors.New("private_key is required")
	}
	tokenURI := strings.TrimSpace(creds.TokenURI)
	if tokenURI == "" {
		return nil, errors.New("token_uri is required")
	}
	key, err := parsePrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = defaultFCMScopeURL
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   connectTimeout,
		KeepAlive: 30 * time.Second,
	}).DialContext
	client := &http.Client{Transport: transport}

	return &fcmTokenSource{
		account: serviceAccount{
			email:      email,
			privateKey: key,
			tokenURI:   tokenURI,
		},
		scope:  scope,
		client: client,
	}, nil
}

func (s *fcmTokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	if s.token != "" && time.Until(s.expiry) > tokenRefreshSkew {
		token := s.token
		s.mu.Unlock()
		return token, nil
	}
	s.mu.Unlock()

	token, expiry, err := s.fetchToken(ctx)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	s.token = token
	s.expiry = expiry
	s.mu.Unlock()

	return token, nil
}

func (s *fcmTokenSource) fetchToken(ctx context.Context) (string, time.Time, error) {
	now := time.Now()
	assertion, err := buildJWT(s.account, s.scope, now)
	if err != nil {
		return "", time.Time{}, err
	}

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.account.tokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", time.Time{}, fmt.Errorf("token request status=%d", resp.StatusCode)
	}

	var token tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return "", time.Time{}, err
	}
	if token.AccessToken == "" {
		return "", time.Time{}, errors.New("token response missing access_token")
	}
	if token.ExpiresIn <= 0 {
		return "", time.Time{}, errors.New("token response missing expires_in")
	}
	expiry := now.Add(time.Duration(token.ExpiresIn) * time.Second)
	return token.AccessToken, expiry, nil
}

func buildJWT(account serviceAccount, scope string, now time.Time) (string, error) {
	header, err := json.Marshal(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	})
	if err != nil {
		return "", err
	}
	claims := jwtClaims{
		Iss:   account.email,
		Scope: scope,
		Aud:   account.tokenURI,
		Exp:   now.Add(time.Hour).Unix(),
		Iat:   now.Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	encoder := base64.RawURLEncoding
	headerPart := encoder.EncodeToString(header)
	payloadPart := encoder.EncodeToString(payload)
	signingInput := headerPart + "." + payloadPart

	hash := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, account.privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", err
	}
	signaturePart := encoder.EncodeToString(signature)
	return signingInput + "." + signaturePart, nil
}

func parsePrivateKey(raw string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, errors.New("private key PEM not found")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("private key is not RSA")
		}
		return rsaKey, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, errors.New("private key parse failed")
}

func newMux(gw *gateway.PushGateway, metricsRegistry *metrics.Registry, ui *uiServer) *http.ServeMux {
	mux := http.NewServeMux()
	var sendResult *template.Template
	if ui != nil {
		sendResult = ui.templates.sendResult
	}
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/readyz", handleReadyz)
	mux.HandleFunc("/push/send", handlePushSend(gw, metricsRegistry, sendResult))
	mux.HandleFunc("/metrics", handleMetrics(metricsRegistry))
	if ui != nil {
		mux.Handle("/ui/static/", http.StripPrefix("/ui/static/", http.FileServer(http.Dir(ui.staticDir))))
		mux.HandleFunc("/ui", ui.handleOverview)
		mux.HandleFunc("/ui/send", ui.handleSend)
		mux.HandleFunc("/ui/troubleshoot", ui.handleTroubleshoot)
		mux.HandleFunc("/ui/metrics", ui.handleUIMetrics)
	}
	return mux
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func handleReadyz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func handlePushSend(gw *gateway.PushGateway, metricsRegistry *metrics.Registry, sendResult *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

		dec := json.NewDecoder(r.Body)
		var req gateway.PushRequest
		if err := dec.Decode(&req); err != nil {
			log.Printf("push decision referenceId=%q status=rejected reason=invalid_request source=validation detail=decode_error err=%v", "", err)
			writePushSendResponse(w, r, http.StatusBadRequest, gateway.PushResponse{
				Status: "rejected",
				Reason: "invalid_request",
			}, sendResult)
			if metricsRegistry != nil {
				metricsRegistry.ObserveRequest("rejected", "invalid_request", time.Since(start))
			}
			return
		}
		if err := dec.Decode(&struct{}{}); err != io.EOF {
			log.Printf("push decision referenceId=%q status=rejected reason=invalid_request source=validation detail=trailing_json", req.ReferenceID)
			writePushSendResponse(w, r, http.StatusBadRequest, gateway.PushResponse{
				Status: "rejected",
				Reason: "invalid_request",
			}, sendResult)
			if metricsRegistry != nil {
				metricsRegistry.ObserveRequest("rejected", "invalid_request", time.Since(start))
			}
			return
		}

		resp, err := gw.SendPush(r.Context(), req)
		status := http.StatusOK
		if err != nil && errors.Is(err, gateway.ErrInvalidRequest) {
			status = http.StatusBadRequest
		}
		source := "provider_result"
		if err != nil && errors.Is(err, gateway.ErrInvalidRequest) {
			source = "validation"
		} else if resp.Reason == "provider_failure" {
			source = "provider_failure"
		}
		log.Printf(
			"push decision referenceId=%q status=%q reason=%q source=%s gatewayMessageId=%q",
			resp.ReferenceID,
			resp.Status,
			resp.Reason,
			source,
			resp.GatewayMessageID,
		)
		writePushSendResponse(w, r, status, resp, sendResult)
		if metricsRegistry != nil {
			metricsRegistry.ObserveRequest(resp.Status, resp.Reason, time.Since(start))
		}
	}
}

func handleMetrics(metricsRegistry *metrics.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if metricsRegistry == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		metricsRegistry.WritePrometheus(w)
	}
}

func writePushResponse(w http.ResponseWriter, status int, resp gateway.PushResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writePushSendResponse(w http.ResponseWriter, r *http.Request, status int, resp gateway.PushResponse, sendResult *template.Template) {
	if sendResult != nil && isHTMX(r) {
		fragmentStatus := status
		if fragmentStatus >= http.StatusBadRequest {
			fragmentStatus = http.StatusOK
		}
		writePushResponseFragment(w, fragmentStatus, resp, sendResult)
		return
	}
	writePushResponse(w, status, resp)
}

func writePushResponseFragment(w http.ResponseWriter, status int, resp gateway.PushResponse, tmpl *template.Template) {
	fragment, err := executeTemplate(tmpl, "send_result.tmpl", resp)
	if err != nil {
		log.Printf("render send result: %v", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(fragment); err != nil {
		log.Printf("write send result: %v", err)
	}
}

func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func newUIServer(providerName string, providerTimeout time.Duration, grafanaDashboardURL string, metricsRegistry *metrics.Registry, logBuffer *logBuffer, startTime time.Time) (*uiServer, error) {
	uiDir, err := findUIDir()
	if err != nil {
		return nil, err
	}
	templates, err := loadUITemplates(uiDir)
	if err != nil {
		return nil, err
	}
	metricsURL := strings.TrimSpace(grafanaDashboardURL)
	if metricsURL == "" {
		metricsURL = defaultGrafanaDashboardURL
	}
	return &uiServer{
		templates:       templates,
		staticDir:       filepath.Join(uiDir, "static"),
		consoleTitle:    "Push Gateway Console",
		sendTitle:       "Send Test Push",
		sendNavLabel:    "Send Test Push",
		sendEndpoint:    "/push/send",
		metricsURL:      metricsURL,
		isPush:          true,
		gatewayName:     "push-gateway",
		version:         version,
		providerName:    providerName,
		providerTimeout: providerTimeout,
		startTime:       startTime,
		metricsRegistry: metricsRegistry,
		logBuffer:       logBuffer,
	}, nil
}

func findUIDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	candidates := []string{
		filepath.Join(wd, "..", "ui"),
		filepath.Join(wd, "..", "..", "ui"),
		filepath.Join(wd, "..", "..", "..", "ui"),
		filepath.Join(wd, "ui"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(candidate, "overview.tmpl")); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("ui templates not found")
}

func loadUITemplates(uiDir string) (uiTemplates, error) {
	overview, err := template.ParseFiles(filepath.Join(uiDir, "nav.tmpl"), filepath.Join(uiDir, "overview.tmpl"))
	if err != nil {
		return uiTemplates{}, err
	}
	send, err := template.ParseFiles(filepath.Join(uiDir, "nav.tmpl"), filepath.Join(uiDir, "send.tmpl"))
	if err != nil {
		return uiTemplates{}, err
	}
	sendResult, err := template.ParseFiles(filepath.Join(uiDir, "send_result.tmpl"))
	if err != nil {
		return uiTemplates{}, err
	}
	troubleshoot, err := template.ParseFiles(filepath.Join(uiDir, "nav.tmpl"), filepath.Join(uiDir, "troubleshoot.tmpl"))
	if err != nil {
		return uiTemplates{}, err
	}
	troubleshootResults, err := template.ParseFiles(filepath.Join(uiDir, "troubleshoot_results.tmpl"))
	if err != nil {
		return uiTemplates{}, err
	}
	metrics, err := template.ParseFiles(filepath.Join(uiDir, "nav.tmpl"), filepath.Join(uiDir, "metrics.tmpl"))
	if err != nil {
		return uiTemplates{}, err
	}
	return uiTemplates{
		overview:            overview,
		send:                send,
		sendResult:          sendResult,
		troubleshoot:        troubleshoot,
		troubleshootResults: troubleshootResults,
		metrics:             metrics,
	}, nil
}

func (u *uiServer) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	view := overviewView{
		ConsoleTitle:    u.consoleTitle,
		SendNavLabel:    u.sendNavLabel,
		MetricsURL:      u.metricsURL,
		GatewayName:     u.gatewayName,
		Version:         u.version,
		ProviderName:    u.providerName,
		ProviderTimeout: u.providerTimeout.String(),
		Uptime:          formatUptime(time.Since(u.startTime)),
	}
	u.renderPage(w, r, u.templates.overview, "overview.tmpl", view)
}

func (u *uiServer) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	view := sendView{
		SendTitle:    u.sendTitle,
		SendEndpoint: u.sendEndpoint,
		SendNavLabel: u.sendNavLabel,
		MetricsURL:   u.metricsURL,
		IsPush:       u.isPush,
	}
	u.renderPage(w, r, u.templates.send, "send.tmpl", view)
}

func (u *uiServer) handleTroubleshoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		view := troubleshootView{
			SendNavLabel: u.sendNavLabel,
			MetricsURL:   u.metricsURL,
		}
		u.renderPage(w, r, u.templates.troubleshoot, "troubleshoot.tmpl", view)
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		referenceID := strings.TrimSpace(r.FormValue("referenceId"))
		if referenceID == "" {
			http.Error(w, "referenceId is required", http.StatusBadRequest)
			return
		}
		entries := u.logBuffer.entriesForReferenceID(referenceID, uiLogResultLimit)
		providerDecision, mappingDecision, finalOutcome := summarizeLogEntries(entries)
		view := troubleshootView{
			SendNavLabel:     u.sendNavLabel,
			ReferenceID:      referenceID,
			ProviderDecision: providerDecision,
			MappingDecision:  mappingDecision,
			FinalOutcome:     finalOutcome,
			Entries:          entries,
		}
		renderFragment(w, u.templates.troubleshootResults, "troubleshoot_results.tmpl", view)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (u *uiServer) handleUIMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	view := buildMetricsView(u.metricsRegistry)
	view.SendNavLabel = u.sendNavLabel
	view.MetricsURL = u.metricsURL
	u.renderPage(w, r, u.templates.metrics, "metrics.tmpl", view)
}

func (u *uiServer) renderPage(w http.ResponseWriter, r *http.Request, tmpl *template.Template, name string, data any) {
	if isHTMX(r) {
		renderFragment(w, tmpl, name, data)
		return
	}
	fragment, err := executeTemplate(tmpl, name, data)
	if err != nil {
		log.Printf("render page: %v", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	renderShell(w, fragment, u.consoleTitle)
}

func renderFragment(w http.ResponseWriter, tmpl *template.Template, name string, data any) {
	fragment, err := executeTemplate(tmpl, name, data)
	if err != nil {
		log.Printf("render fragment: %v", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(fragment); err != nil {
		log.Printf("write fragment: %v", err)
	}
}

func renderShell(w http.ResponseWriter, fragment []byte, title string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := io.WriteString(w, "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><link rel=\"stylesheet\" href=\"/ui/static/ui.css\"><title>"+title+"</title></head><body><div class=\"topbar\"><div class=\"topbar-brand\"><svg class=\"topbar-logo\" viewBox=\"0 0 48 24\" aria-hidden=\"true\" focusable=\"false\"><path d=\"M2 18c6-10 12-14 22-14s16 4 22 14\" fill=\"none\" stroke=\"currentColor\" stroke-width=\"2\" stroke-linecap=\"round\"/><path d=\"M8 18v-6M40 18v-6M16 18v-4M32 18v-4\" stroke=\"currentColor\" stroke-width=\"2\" stroke-linecap=\"round\"/><path d=\"M2 18h44\" stroke=\"currentColor\" stroke-width=\"2\" stroke-linecap=\"round\"/></svg><span class=\"topbar-title\">Setu</span></div></div><div id=\"ui-root\">"); err != nil {
		log.Printf("write shell start: %v", err)
		return
	}
	if _, err := w.Write(fragment); err != nil {
		log.Printf("write shell fragment: %v", err)
		return
	}
	if _, err := io.WriteString(w, "</div><script src=\"/ui/static/htmx.min.js\"></script><script src=\"/ui/static/json-enc.js\"></script></body></html>"); err != nil {
		log.Printf("write shell end: %v", err)
	}
}

func executeTemplate(tmpl *template.Template, name string, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func formatUptime(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	return d.Truncate(time.Second).String()
}

func buildMetricsView(metricsRegistry *metrics.Registry) metricsView {
	if metricsRegistry == nil {
		return metricsView{}
	}
	var buf bytes.Buffer
	metricsRegistry.WritePrometheus(&buf)
	return parseMetrics(buf.String())
}

func parseMetrics(text string) metricsView {
	view := metricsView{}
	rejections := make(map[string]string)
	var requestLatency []latencyBucket
	var providerLatency []latencyBucket
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		name, labels, value, ok := parseMetricLine(line)
		if !ok {
			continue
		}
		switch name {
		case "gateway_requests_total":
			view.TotalRequests = value
		case "gateway_outcomes_total":
			switch labels["outcome"] {
			case "accepted":
				view.AcceptedTotal = value
			case "rejected":
				view.RejectedTotal = value
			}
		case "gateway_rejections_total":
			if reason := labels["reason"]; reason != "" {
				rejections[reason] = value
			}
		case "gateway_provider_failures_total":
			view.ProviderFailureCount = value
		case "gateway_request_duration_seconds_bucket":
			if le := labels["le"]; le != "" && le != "+Inf" {
				requestLatency = append(requestLatency, latencyBucket{
					Label: "<= " + le + "s",
					Count: value,
				})
			}
		case "gateway_provider_duration_seconds_bucket":
			if le := labels["le"]; le != "" && le != "+Inf" {
				providerLatency = append(providerLatency, latencyBucket{
					Label: "<= " + le + "s",
					Count: value,
				})
			}
		}
	}
	if len(rejections) > 0 {
		reasonOrder := []string{
			"invalid_request",
			"duplicate_reference",
			"provider_failure",
		}
		for _, reason := range reasonOrder {
			count := rejections[reason]
			if count == "" {
				count = "0"
			}
			view.Rejections = append(view.Rejections, rejectionCount{
				Reason: reason,
				Count:  count,
			})
		}
	}
	view.RequestLatency = requestLatency
	view.ProviderLatency = providerLatency
	return view
}

func parseMetricLine(line string) (string, map[string]string, string, bool) {
	if line == "" || strings.HasPrefix(line, "#") {
		return "", nil, "", false
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", nil, "", false
	}
	metric := fields[0]
	value := fields[1]
	name := metric
	labels := map[string]string{}
	if idx := strings.Index(metric, "{"); idx != -1 && strings.HasSuffix(metric, "}") {
		name = metric[:idx]
		labelPart := strings.TrimSuffix(metric[idx+1:], "}")
		labels = parseLabels(labelPart)
	}
	return name, labels, value, true
}

func parseLabels(labelPart string) map[string]string {
	labels := make(map[string]string)
	if labelPart == "" {
		return labels
	}
	parts := strings.Split(labelPart, ",")
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		value := strings.Trim(kv[1], "\"")
		labels[key] = value
	}
	return labels
}

func newLogBuffer(capacity int) *logBuffer {
	return &logBuffer{capacity: capacity}
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.partial += string(p)
	for {
		idx := strings.IndexByte(b.partial, '\n')
		if idx == -1 {
			break
		}
		line := strings.TrimRight(b.partial[:idx], "\r")
		b.partial = b.partial[idx+1:]
		if line == "" {
			continue
		}
		entry := parseLogEntry(line)
		b.append(entry)
	}
	return len(p), nil
}

func (b *logBuffer) append(entry logEntry) {
	if b.capacity <= 0 {
		return
	}
	if len(b.entries) < b.capacity {
		b.entries = append(b.entries, entry)
		return
	}
	copy(b.entries, b.entries[1:])
	b.entries[len(b.entries)-1] = entry
}

func (b *logBuffer) entriesForReferenceID(referenceID string, limit int) []logEntry {
	if b == nil || referenceID == "" || limit <= 0 {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	needle := fmt.Sprintf("referenceId=%q", referenceID)
	matches := make([]logEntry, 0, limit)
	for i := len(b.entries) - 1; i >= 0 && len(matches) < limit; i-- {
		if strings.Contains(b.entries[i].Line, needle) {
			matches = append(matches, b.entries[i])
		}
	}
	for i, j := 0, len(matches)-1; i < j; i, j = i+1, j-1 {
		matches[i], matches[j] = matches[j], matches[i]
	}
	return matches
}

func parseLogEntry(line string) logEntry {
	timestamp, msg := splitLogTimestamp(line)
	return logEntry{
		Timestamp: timestamp,
		Line:      msg,
	}
}

func splitLogTimestamp(line string) (string, string) {
	if len(line) >= 20 &&
		line[4] == '/' &&
		line[7] == '/' &&
		line[10] == ' ' &&
		line[13] == ':' &&
		line[16] == ':' {
		return line[:19], strings.TrimSpace(line[19:])
	}
	return "", line
}

func summarizeLogEntries(entries []logEntry) (string, string, string) {
	var providerDecision string
	var mappingDecision string
	var finalOutcome string
	for i := len(entries) - 1; i >= 0; i-- {
		line := entries[i].Line
		if finalOutcome == "" && strings.Contains(line, "push decision") {
			status := parseQuotedField(line, "status")
			reason := parseQuotedField(line, "reason")
			finalOutcome = formatFinalOutcome(status, reason, line)
		}
		if providerDecision == "" && strings.Contains(line, "push provider decision") {
			providerDecision = line
			if mappingDecision == "" {
				mappingDecision = parseField(line, "mapped")
			}
		}
		if providerDecision == "" && strings.Contains(line, "push provider response") {
			providerDecision = line
		}
		if providerDecision == "" && strings.Contains(line, "push provider error") {
			providerDecision = line
		}
		if mappingDecision == "" && strings.Contains(line, "mapped=") {
			mappingDecision = parseField(line, "mapped")
		}
	}
	return providerDecision, mappingDecision, finalOutcome
}

func parseQuotedField(line, field string) string {
	needle := field + "=\""
	idx := strings.Index(line, needle)
	if idx == -1 {
		return ""
	}
	start := idx + len(needle)
	end := strings.Index(line[start:], "\"")
	if end == -1 {
		return ""
	}
	return line[start : start+end]
}

func parseField(line, field string) string {
	needle := field + "="
	idx := strings.Index(line, needle)
	if idx == -1 {
		return ""
	}
	start := idx + len(needle)
	end := strings.IndexFunc(line[start:], func(r rune) bool {
		return r == ' ' || r == '\t'
	})
	if end == -1 {
		return line[start:]
	}
	return line[start : start+end]
}

func formatFinalOutcome(status, reason, fallback string) string {
	if status == "" {
		return fallback
	}
	if status == "accepted" {
		return "accepted"
	}
	if status == "rejected" {
		if reason != "" {
			return "rejected (" + reason + ")"
		}
		return "rejected"
	}
	if reason != "" {
		return status + " (" + reason + ")"
	}
	return status
}
