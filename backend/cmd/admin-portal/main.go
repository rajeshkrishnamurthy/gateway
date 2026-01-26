package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const version = "0.1.0"

var configPath = flag.String("config", "conf/admin_portal.json", "Admin portal config file path")
var listenAddr = flag.String("addr", ":8090", "HTTP listen address")
var showHelp = flag.Bool("help", false, "show usage")
var showVersion = flag.Bool("version", false, "show version")

const proxyTimeout = 8 * time.Second

const (
	navOverview      = "overview"
	navSMS           = "sms"
	navPush          = "push"
	navHAProxy       = "haproxy"
	navCommandCenter = "command-center"
)

type fileConfig struct {
	Title            string `json:"title"`
	SMSGatewayURL    string `json:"smsGatewayUrl"`
	PushGatewayURL   string `json:"pushGatewayUrl"`
	CommandCenterURL string `json:"commandCenterUrl"`
	HAProxyStatsURL  string `json:"haproxyStatsUrl"`
}

type portalTemplates struct {
	topbar   *template.Template
	overview *template.Template
	haproxy  *template.Template
	errView  *template.Template
}

type portalServer struct {
	config    fileConfig
	templates portalTemplates
	staticDir string
	client    *http.Client
}

type topbarView struct {
	Active            string
	ShowSMS           bool
	ShowPush          bool
	ShowHAProxy       bool
	ShowCommandCenter bool
}

type overviewView struct {
	Title    string
	Consoles []consoleView
}

type consoleView struct {
	Label string
	Meta  string
	Href  string
}

type haproxyView struct {
	Frontends []haproxyFrontend
	Backends  []haproxyBackend
	Error     string
}

type haproxyFrontend struct {
	Name        string
	Status      string
	Sessions    string
	LastChange  string
	StatusClass string
}

type haproxyBackend struct {
	Name         string
	Status       string
	ServersUp    int
	ServersTotal int
	StatusClass  string
}

type errorView struct {
	Title   string
	Message string
}

func main() {
	flag.Parse()
	if *showHelp {
		flag.Usage()
		return
	}
	if *showVersion {
		log.Printf("admin-portal version %s", version)
		return
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	uiDir, err := findUIDir()
	if err != nil {
		log.Fatal(err)
	}
	templates, err := loadPortalTemplates(uiDir)
	if err != nil {
		log.Fatal(err)
	}

	server := &portalServer{
		config:    normalizeConfig(cfg),
		templates: templates,
		staticDir: filepath.Join(uiDir, "static"),
		client: &http.Client{
			Timeout: proxyTimeout,
		},
	}

	mux := http.NewServeMux()
	mux.Handle("/ui/static/", http.StripPrefix("/ui/static/", http.FileServer(http.Dir(server.staticDir))))
	mux.HandleFunc("/ui", server.handleOverview)
	mux.HandleFunc("/haproxy", server.handleHAProxy)
	mux.HandleFunc("/haproxy/", server.handleHAProxy)
	mux.HandleFunc("/sms/ui", server.handleSMSUI)
	mux.HandleFunc("/sms/ui/", server.handleSMSUI)
	mux.HandleFunc("/sms/send", server.handleSMSAPI)
	mux.HandleFunc("/push/ui", server.handlePushUI)
	mux.HandleFunc("/push/ui/", server.handlePushUI)
	mux.HandleFunc("/push/send", server.handlePushAPI)
	mux.HandleFunc("/command-center/ui", server.handleCommandCenterUI)
	mux.HandleFunc("/command-center/ui/", server.handleCommandCenterUI)

	log.Printf("listening on %s configPath=%q", *listenAddr, *configPath)
	if err := http.ListenAndServe(*listenAddr, mux); err != nil {
		log.Fatal(err)
	}
}

func (s *portalServer) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	view := overviewView{
		Title:    resolveTitle(s.config.Title),
		Consoles: buildConsoleViews(s.config),
	}
	s.renderPage(w, r, s.templates.overview, "portal_overview.tmpl", view, navOverview)
}

func (s *portalServer) handleHAProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.config.HAProxyStatsURL == "" {
		s.renderError(w, r, http.StatusNotFound, "HAProxy not configured", "haproxyStatsUrl is empty in the portal config.", navHAProxy)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, s.config.HAProxyStatsURL, nil)
	if err != nil {
		s.renderError(w, r, http.StatusBadGateway, "HAProxy request failed", err.Error(), navHAProxy)
		return
	}
	resp, err := s.client.Do(req)
	if err != nil {
		s.renderError(w, r, http.StatusBadGateway, "HAProxy request failed", err.Error(), navHAProxy)
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.renderError(w, r, http.StatusBadGateway, "HAProxy response failed", err.Error(), navHAProxy)
		return
	}
	frontends, backends, err := parseHAProxyCSV(body)
	view := haproxyView{Frontends: frontends, Backends: backends}
	if err != nil {
		view.Error = err.Error()
	}
	s.renderPage(w, r, s.templates.haproxy, "portal_haproxy.tmpl", view, navHAProxy)
}

func (s *portalServer) handleSMSUI(w http.ResponseWriter, r *http.Request) {
	s.proxyUI(w, r, s.config.SMSGatewayURL, "/sms", navSMS, false)
}

func (s *portalServer) handlePushUI(w http.ResponseWriter, r *http.Request) {
	s.proxyUI(w, r, s.config.PushGatewayURL, "/push", navPush, false)
}

func (s *portalServer) handleCommandCenterUI(w http.ResponseWriter, r *http.Request) {
	s.proxyUI(w, r, s.config.CommandCenterURL, "/command-center", navCommandCenter, true)
}

func (s *portalServer) handleSMSAPI(w http.ResponseWriter, r *http.Request) {
	s.proxyAPI(w, r, s.config.SMSGatewayURL)
}

func (s *portalServer) handlePushAPI(w http.ResponseWriter, r *http.Request) {
	s.proxyAPI(w, r, s.config.PushGatewayURL)
}

func (s *portalServer) proxyUI(w http.ResponseWriter, r *http.Request, baseURL, prefix, active string, embed bool) {
	if baseURL == "" {
		s.renderError(w, r, http.StatusNotFound, "Console not configured", "The upstream URL is not set in the portal config.", active)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	remotePath := strings.TrimPrefix(r.URL.Path, prefix)
	if remotePath == "" {
		remotePath = "/"
	}
	if !strings.HasPrefix(remotePath, "/") {
		remotePath = "/" + remotePath
	}

	remoteURL, err := buildTargetURL(baseURL, remotePath, r.URL.RawQuery, embed)
	if err != nil {
		s.renderError(w, r, http.StatusBadGateway, "Invalid upstream URL", err.Error(), active)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, remoteURL, r.Body)
	if err != nil {
		s.renderError(w, r, http.StatusBadGateway, "Upstream request failed", err.Error(), active)
		return
	}
	copyHeader(req.Header, r.Header, []string{"Content-Type", "Accept"})
	req.Header.Set("HX-Request", "true")

	resp, err := s.client.Do(req)
	if err != nil {
		s.renderError(w, r, http.StatusBadGateway, "Upstream request failed", err.Error(), active)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.renderError(w, r, http.StatusBadGateway, "Upstream response failed", err.Error(), active)
		return
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "text/html") {
		body = rewriteUIPaths(body, prefix)
		if prefix == "/sms" {
			body = bytes.ReplaceAll(body, []byte("Troubleshoot by ReferenceId"), []byte("Troubleshoot"))
		}
		if embed {
			body = stripThemeToggle(body)
		}
		if !isHTMX(r) {
			s.renderShell(w, body, active, resp.StatusCode)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(resp.StatusCode)
		if _, err := w.Write(body); err != nil {
			log.Printf("write proxy fragment: %v", err)
		}
		return
	}

	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(body); err != nil {
		log.Printf("write proxy response: %v", err)
	}
}

func (s *portalServer) proxyAPI(w http.ResponseWriter, r *http.Request, baseURL string) {
	if baseURL == "" {
		http.Error(w, "upstream not configured", http.StatusNotFound)
		return
	}
	remoteURL, err := buildTargetURL(baseURL, r.URL.Path, r.URL.RawQuery, false)
	if err != nil {
		http.Error(w, "invalid upstream URL", http.StatusBadGateway)
		return
	}
	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, remoteURL, r.Body)
	if err != nil {
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	copyHeader(proxyReq.Header, r.Header, []string{"Content-Type", "Accept", "HX-Request"})

	resp, err := s.client.Do(proxyReq)
	if err != nil {
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "" {
		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("write proxy api: %v", err)
	}
}

func (s *portalServer) renderPage(w http.ResponseWriter, r *http.Request, tmpl *template.Template, name string, data any, active string) {
	fragment, err := executeTemplate(tmpl, name, data)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Render failed", err.Error(), active)
		return
	}
	if isHTMX(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := w.Write(fragment); err != nil {
			log.Printf("write fragment: %v", err)
		}
		return
	}
	s.renderShell(w, fragment, active, http.StatusOK)
}

func (s *portalServer) renderShell(w http.ResponseWriter, fragment []byte, active string, status int) {
	topbar, err := executeTemplate(s.templates.topbar, "portal_topbar.tmpl", topbarView{
		Active:            active,
		ShowSMS:           s.config.SMSGatewayURL != "",
		ShowPush:          s.config.PushGatewayURL != "",
		ShowHAProxy:       s.config.HAProxyStatsURL != "",
		ShowCommandCenter: s.config.CommandCenterURL != "",
	})
	if err != nil {
		log.Printf("render topbar: %v", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}

	if status <= 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := fmt.Fprintf(w, "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><link rel=\"stylesheet\" href=\"/ui/static/ui.css\"><title>%s</title></head><body>", template.HTMLEscapeString(resolveTitle(s.config.Title))); err != nil {
		log.Printf("write shell start: %v", err)
		return
	}
	if _, err := w.Write(topbar); err != nil {
		log.Printf("write topbar: %v", err)
		return
	}
	if _, err := io.WriteString(w, "<div id=\"ui-root\" class=\"portal-root\">"); err != nil {
		log.Printf("write shell root: %v", err)
		return
	}
	if _, err := w.Write(fragment); err != nil {
		log.Printf("write shell fragment: %v", err)
		return
	}
	if _, err := io.WriteString(w, "</div><script src=\"/ui/static/htmx.min.js\"></script><script src=\"/ui/static/json-enc.js\"></script><script src=\"/ui/static/theme.js\"></script></body></html>"); err != nil {
		log.Printf("write shell end: %v", err)
	}
}

func (s *portalServer) renderError(w http.ResponseWriter, r *http.Request, status int, title, message, active string) {
	fragment, err := executeTemplate(s.templates.errView, "portal_error.tmpl", errorView{Title: title, Message: message})
	if err != nil {
		http.Error(w, message, status)
		return
	}
	if isHTMX(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		if _, err := w.Write(fragment); err != nil {
			log.Printf("write error fragment: %v", err)
		}
		return
	}
	s.renderShell(w, fragment, active, status)
}

func executeTemplate(tmpl *template.Template, name string, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func loadPortalTemplates(uiDir string) (portalTemplates, error) {
	topbar, err := template.ParseFiles(filepath.Join(uiDir, "portal_topbar.tmpl"))
	if err != nil {
		return portalTemplates{}, err
	}
	overview, err := template.ParseFiles(filepath.Join(uiDir, "portal_overview.tmpl"))
	if err != nil {
		return portalTemplates{}, err
	}
	haproxy, err := template.ParseFiles(filepath.Join(uiDir, "portal_haproxy.tmpl"))
	if err != nil {
		return portalTemplates{}, err
	}
	errView, err := template.ParseFiles(filepath.Join(uiDir, "portal_error.tmpl"))
	if err != nil {
		return portalTemplates{}, err
	}
	return portalTemplates{topbar: topbar, overview: overview, haproxy: haproxy, errView: errView}, nil
}

func buildConsoleViews(cfg fileConfig) []consoleView {
	var consoles []consoleView
	if cfg.SMSGatewayURL != "" {
		consoles = append(consoles, consoleView{Label: "SMS Gateway", Meta: cfg.SMSGatewayURL, Href: "/sms/ui"})
	}
	if cfg.PushGatewayURL != "" {
		consoles = append(consoles, consoleView{Label: "Push Gateway", Meta: cfg.PushGatewayURL, Href: "/push/ui"})
	}
	if cfg.CommandCenterURL != "" {
		consoles = append(consoles, consoleView{Label: "Command Center", Meta: cfg.CommandCenterURL, Href: "/command-center/ui"})
	}
	if cfg.HAProxyStatsURL != "" {
		consoles = append(consoles, consoleView{Label: "HAProxy", Meta: cfg.HAProxyStatsURL, Href: "/haproxy"})
	}
	return consoles
}

func rewriteUIPaths(input []byte, prefix string) []byte {
	if prefix == "" {
		return input
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	output := string(input)
	output = strings.ReplaceAll(output, "=\"/ui", "=\""+prefix+"/ui")
	output = strings.ReplaceAll(output, "='/ui", "='"+prefix+"/ui")
	return []byte(output)
}

func stripThemeToggle(input []byte) []byte {
	text := string(input)
	needle := "id=\"theme-toggle\""
	for {
		idx := strings.Index(text, needle)
		if idx == -1 {
			break
		}
		start := strings.LastIndex(text[:idx], "<button")
		if start == -1 {
			break
		}
		end := strings.Index(text[idx:], "</button>")
		if end == -1 {
			break
		}
		end = idx + end + len("</button>")
		text = text[:start] + text[end:]
	}
	return []byte(text)
}

func parseHAProxyCSV(data []byte) ([]haproxyFrontend, []haproxyBackend, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(records) == 0 {
		return nil, nil, errors.New("empty HAProxy stats")
	}

	header := records[0]
	if len(header) == 0 {
		return nil, nil, errors.New("missing HAProxy header")
	}
	header[0] = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(header[0]), "#"))
	columns := make(map[string]int)
	for i, name := range header {
		columns[strings.TrimSpace(name)] = i
	}

	get := func(record []string, key string) string {
		idx, ok := columns[key]
		if !ok || idx >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[idx])
	}

	var frontends []haproxyFrontend
	backendOrder := make([]string, 0)
	backendMap := make(map[string]*haproxyBackend)

	for _, record := range records[1:] {
		pxname := get(record, "pxname")
		svname := get(record, "svname")
		status := get(record, "status")
		if pxname == "" || svname == "" {
			continue
		}
		if svname == "FRONTEND" {
			frontends = append(frontends, haproxyFrontend{
				Name:        pxname,
				Status:      status,
				Sessions:    fallbackZero(get(record, "scur")),
				LastChange:  formatSeconds(get(record, "lastchg")),
				StatusClass: statusClass(status),
			})
			continue
		}

		backend, ok := backendMap[pxname]
		if !ok {
			backend = &haproxyBackend{Name: pxname}
			backendMap[pxname] = backend
			backendOrder = append(backendOrder, pxname)
		}

		if svname == "BACKEND" {
			backend.Status = status
			backend.StatusClass = statusClass(status)
			continue
		}

		backend.ServersTotal++
		if isUpStatus(status) {
			backend.ServersUp++
		}
	}

	backends := make([]haproxyBackend, 0, len(backendOrder))
	for _, name := range backendOrder {
		backend := backendMap[name]
		if backend.Status == "" {
			backend.Status = "unknown"
			backend.StatusClass = statusClass(backend.Status)
		}
		backends = append(backends, *backend)
	}

	return frontends, backends, nil
}

func statusClass(status string) string {
	if isUpStatus(status) {
		return "status-up"
	}
	return "status-down"
}

func isUpStatus(status string) bool {
	status = strings.ToUpper(strings.TrimSpace(status))
	return status == "UP" || status == "OPEN"
}

func fallbackZero(value string) string {
	if strings.TrimSpace(value) == "" {
		return "0"
	}
	return value
}

func formatSeconds(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value + "s"
}

func buildTargetURL(base, path, rawQuery string, embed bool) (string, error) {
	parsedBase, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	ref := &url.URL{Path: path, RawQuery: rawQuery}
	resolved := parsedBase.ResolveReference(ref)
	if embed {
		query := resolved.Query()
		query.Set("embed", "1")
		resolved.RawQuery = query.Encode()
	}
	return resolved.String(), nil
}

func copyHeader(dst, src http.Header, keys []string) {
	for _, key := range keys {
		if value := src.Get(key); value != "" {
			dst.Set(key, value)
		}
	}
}

func isHTMX(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("HX-Request"), "true")
}

func loadConfig(path string) (fileConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return fileConfig{}, err
	}
	defer file.Close()

	var buf bytes.Buffer
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return fileConfig{}, err
	}

	decoder := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	decoder.DisallowUnknownFields()
	var cfg fileConfig
	if err := decoder.Decode(&cfg); err != nil {
		return fileConfig{}, err
	}
	if decoder.More() {
		return fileConfig{}, errors.New("config has trailing data")
	}
	return cfg, nil
}

func normalizeConfig(cfg fileConfig) fileConfig {
	cfg.Title = strings.TrimSpace(cfg.Title)
	cfg.SMSGatewayURL = strings.TrimRight(strings.TrimSpace(cfg.SMSGatewayURL), "/")
	cfg.PushGatewayURL = strings.TrimRight(strings.TrimSpace(cfg.PushGatewayURL), "/")
	cfg.CommandCenterURL = strings.TrimRight(strings.TrimSpace(cfg.CommandCenterURL), "/")
	cfg.HAProxyStatsURL = strings.TrimSpace(cfg.HAProxyStatsURL)
	return cfg
}

func resolveTitle(title string) string {
	if strings.TrimSpace(title) == "" {
		return "Setu Admin Portal"
	}
	return title
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
		if _, err := os.Stat(filepath.Join(candidate, "portal_overview.tmpl")); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("ui templates not found")
}
