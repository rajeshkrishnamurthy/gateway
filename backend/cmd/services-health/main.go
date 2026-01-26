package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultConfigPath  = "conf/services_health.json"
	defaultListenAddr  = ":8070"
	statusDialTimeout  = 500 * time.Millisecond
	startWaitTimeout   = 10 * time.Second
	startPollInterval  = 200 * time.Millisecond
	maxStartOutput     = 400
	defaultStatusLabel = "down"
)

var configPath = flag.String("config", defaultConfigPath, "Services health config file path")
var listenAddr = flag.String("addr", defaultListenAddr, "HTTP listen address")

const (
	statusUpClass   = "status-up"
	statusDownClass = "status-down"
)

type fileConfig struct {
	Services []serviceConfig `json:"services"`
}

type serviceConfig struct {
	ID                string            `json:"id"`
	Label             string            `json:"label"`
	Instances         []serviceInstance `json:"instances"`
	StartCommand      []string          `json:"startCommand"`
	StopCommand       []string          `json:"stopCommand"`
	DefaultConfigPath string            `json:"defaultConfigPath"`
	SingleToggle      bool              `json:"singleToggle"`
	ToggleInstance    string            `json:"toggleInstance"`
}

type serviceInstance struct {
	Name       string `json:"name"`
	Addr       string `json:"addr"`
	UIURL      string `json:"uiUrl"`
	ConfigPath string `json:"configPath"`
}

type uiTemplates struct {
	overview *template.Template
	services *template.Template
	config   *template.Template
}

type uiServer struct {
	templates  uiTemplates
	staticDir  string
	config     fileConfig
	title      string
	workingDir string
	configDir  string
}

type overviewView struct {
	Title           string
	ShowThemeToggle bool
	Services        servicesView
}

type servicesView struct {
	Services []serviceView
	Notice   string
}

type configView struct {
	Hidden       bool
	ServiceLabel string
	InstanceName string
	DisplayPath  string
	Content      string
	Error        string
}

type actionResult struct {
	notice       string
	serviceID    string
	instanceName string
	desiredUp    *bool
}

type serviceView struct {
	ID                 string
	Label              string
	Instances          []instanceView
	HasStart           bool
	HasStop            bool
	NeedsConfig        bool
	NeedsAddr          bool
	SingleToggle       bool
	ToggleInstanceName string
	ToggleIsUp         bool
	ToggleConfigPath   string
}

type instanceView struct {
	Name        string
	Addr        string
	Port        string
	UIURL       string
	Status      string
	StatusClass string
	IsUp        bool
	ConfigPath  string
	AddrInput   string
}

func main() {
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	ui, err := newUIServer(cfg)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRoot)
	mux.HandleFunc("/ui", ui.handleOverview)
	mux.HandleFunc("/ui/services", ui.handleServices)
	mux.HandleFunc("/ui/services/start", ui.handleStart)
	mux.HandleFunc("/ui/services/stop", ui.handleStop)
	mux.HandleFunc("/ui/config", ui.handleConfig)
	mux.HandleFunc("/ui/config/clear", ui.handleConfigClear)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(ui.staticDir))))

	log.Printf("services health listening on %s configPath=%q", *listenAddr, *configPath)
	if err := http.ListenAndServe(*listenAddr, mux); err != nil {
		log.Fatal(err)
	}
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.Redirect(w, r, "/ui", http.StatusFound)
}

func newUIServer(cfg fileConfig) (*uiServer, error) {
	uiDir, err := findUIDir()
	if err != nil {
		return nil, err
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	configDir := filepath.Join(workingDir, "conf")
	info, err := os.Stat(configDir)
	if err != nil {
		return nil, fmt.Errorf("config dir not found: %s", configDir)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("config dir is not a directory: %s", configDir)
	}
	templates, err := loadUITemplates(uiDir)
	if err != nil {
		return nil, err
	}
	return &uiServer{
		templates:  templates,
		staticDir:  filepath.Join(uiDir, "static"),
		config:     cfg,
		title:      "Command Center",
		workingDir: workingDir,
		configDir:  configDir,
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
		if _, err := os.Stat(filepath.Join(candidate, "health_overview.tmpl")); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("ui templates not found")
}

func loadUITemplates(uiDir string) (uiTemplates, error) {
	overview, err := template.ParseFiles(filepath.Join(uiDir, "health_overview.tmpl"), filepath.Join(uiDir, "health_services.tmpl"))
	if err != nil {
		return uiTemplates{}, err
	}
	services, err := template.ParseFiles(filepath.Join(uiDir, "health_services.tmpl"))
	if err != nil {
		return uiTemplates{}, err
	}
	config, err := template.ParseFiles(filepath.Join(uiDir, "health_config.tmpl"))
	if err != nil {
		return uiTemplates{}, err
	}
	return uiTemplates{overview: overview, services: services, config: config}, nil
}

func (u *uiServer) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	embedValue := strings.TrimSpace(r.URL.Query().Get("embed"))
	embed := strings.EqualFold(embedValue, "1") || strings.EqualFold(embedValue, "true")
	view := overviewView{
		Title:           "Command Center",
		ShowThemeToggle: !embed,
		Services:        buildServicesView(u.config, "", nil),
	}
	u.renderPage(w, r, u.templates.overview, "health_overview.tmpl", view)
}

func (u *uiServer) handleServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	view := buildServicesView(u.config, "", nil)
	renderFragment(w, u.templates.services, "health_services.tmpl", view)
}

func (u *uiServer) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	result := u.runAction(r.FormValue("serviceId"), r.FormValue("instanceName"), r.FormValue("configPath"), r.FormValue("addr"), true)
	overrides := buildOverrides(result)
	view := buildServicesView(u.config, result.notice, overrides)
	renderFragment(w, u.templates.services, "health_services.tmpl", view)
}

func (u *uiServer) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	result := u.runAction(r.FormValue("serviceId"), r.FormValue("instanceName"), r.FormValue("configPath"), r.FormValue("addr"), false)
	overrides := buildOverrides(result)
	view := buildServicesView(u.config, result.notice, overrides)
	renderFragment(w, u.templates.services, "health_services.tmpl", view)
}

func (u *uiServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	serviceID := strings.TrimSpace(r.URL.Query().Get("serviceId"))
	instanceName := strings.TrimSpace(r.URL.Query().Get("instanceName"))
	if serviceID == "" || instanceName == "" {
		u.renderConfigError(w, "service and instance are required")
		return
	}
	service, instance, err := findServiceInstance(u.config, serviceID, instanceName)
	if err != nil {
		u.renderConfigError(w, err.Error())
		return
	}
	configPath := strings.TrimSpace(defaultConfigPathFor(service, instance))
	if configPath == "" {
		u.renderConfigError(w, "config not configured for this instance")
		return
	}
	fullPath, displayPath, err := resolveConfigPath(u.workingDir, u.configDir, configPath)
	if err != nil {
		u.renderConfigError(w, err.Error())
		return
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		u.renderConfigError(w, fmt.Sprintf("read failed: %v", err))
		return
	}
	view := configView{
		ServiceLabel: service.Label,
		InstanceName: instance.Name,
		DisplayPath:  displayPath,
		Content:      string(data),
	}
	renderFragment(w, u.templates.config, "health_config.tmpl", view)
}

func (u *uiServer) handleConfigClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	view := configView{Hidden: true}
	renderFragment(w, u.templates.config, "health_config.tmpl", view)
}

func (u *uiServer) renderConfigError(w http.ResponseWriter, message string) {
	view := configView{Error: message}
	renderFragment(w, u.templates.config, "health_config.tmpl", view)
}

func (u *uiServer) runAction(serviceID, instanceName, configInput, addrInput string, isStart bool) actionResult {
	service, instance, err := findServiceInstance(u.config, serviceID, instanceName)
	if err != nil {
		return actionResult{notice: err.Error(), serviceID: serviceID, instanceName: instanceName}
	}
	configPath := strings.TrimSpace(configInput)
	addr := strings.TrimSpace(addrInput)
	if configPath == "" {
		configPath = defaultConfigPathFor(service, instance)
	}
	if addr == "" {
		addr = instance.Addr
	}
	host, port, err := splitAddr(addr)
	if err != nil {
		return actionResult{notice: err.Error(), serviceID: serviceID, instanceName: instanceName}
	}
	var cmdArgs []string
	if isStart {
		cmdArgs, err = buildCommand(service.StartCommand, configPath, addr, port)
	} else {
		cmdArgs, err = buildCommand(service.StopCommand, configPath, addr, port)
	}
	if err != nil {
		return actionResult{notice: err.Error(), serviceID: serviceID, instanceName: instanceName}
	}
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	var output bytes.Buffer
	writer := io.MultiWriter(os.Stderr, &output)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if isStart {
		if err := cmd.Start(); err != nil {
			return actionResult{notice: formatStartError(err, output.String()), serviceID: serviceID, instanceName: instanceName}
		}
		exitCh := make(chan error, 1)
		go func() {
			exitCh <- cmd.Wait()
		}()
		addrToCheck := net.JoinHostPort(host, port)
		if err := waitForAddrUp(addrToCheck, startWaitTimeout, exitCh); err != nil {
			return actionResult{notice: formatStartError(err, output.String()), serviceID: serviceID, instanceName: instanceName}
		}
		log.Printf("started %s/%s: %s", service.ID, instance.Name, strings.Join(cmdArgs, " "))
		return actionResult{notice: fmt.Sprintf("started %s (%s)", service.Label, instance.Name), serviceID: service.ID, instanceName: instance.Name, desiredUp: boolPtr(true)}
	}
	if err := cmd.Run(); err != nil {
		return actionResult{notice: fmt.Sprintf("stop failed: %v", err), serviceID: serviceID, instanceName: instanceName}
	}
	addrToCheck := net.JoinHostPort(host, port)
	if err := waitForAddrDown(addrToCheck, startWaitTimeout); err != nil {
		return actionResult{notice: fmt.Sprintf("stop failed: %v", err), serviceID: serviceID, instanceName: instanceName}
	}
	log.Printf("stopped %s/%s: %s", service.ID, instance.Name, strings.Join(cmdArgs, " "))
	return actionResult{notice: fmt.Sprintf("stopped %s (%s)", service.Label, instance.Name), serviceID: service.ID, instanceName: instance.Name, desiredUp: boolPtr(false)}
}

func boolPtr(value bool) *bool {
	return &value
}

func buildOverrides(result actionResult) map[string]bool {
	if result.desiredUp == nil {
		return nil
	}
	key := serviceInstanceKey(result.serviceID, result.instanceName)
	return map[string]bool{key: *result.desiredUp}
}

func serviceInstanceKey(serviceID, instanceName string) string {
	return serviceID + "|" + instanceName
}

func buildServicesView(cfg fileConfig, notice string, overrides map[string]bool) servicesView {
	services := make([]serviceView, 0, len(cfg.Services))
	for _, service := range cfg.Services {
		needsConfig := hasPlaceholder(service.StartCommand, "{config}")
		needsAddr := hasPlaceholder(service.StartCommand, "{addr}")
		instances := make([]instanceView, 0, len(service.Instances))
		for _, instance := range service.Instances {
			addr := strings.TrimSpace(instance.Addr)
			host, port, err := splitAddr(addr)
			status := defaultStatusLabel
			statusClass := statusDownClass
			isUp := false
			overrideApplied := false
			if overrides != nil {
				key := serviceInstanceKey(service.ID, instance.Name)
				if override, ok := overrides[key]; ok {
					overrideApplied = true
					isUp = override
					if override {
						status = "up"
						statusClass = statusUpClass
					} else {
						status = "down"
						statusClass = statusDownClass
					}
				}
			}
			if err == nil && !overrideApplied {
				if isAddrUp(net.JoinHostPort(host, port)) {
					status = "up"
					statusClass = statusUpClass
					isUp = true
				}
			}
			instances = append(instances, instanceView{
				Name:        instance.Name,
				Addr:        instance.Addr,
				Port:        port,
				UIURL:       instance.UIURL,
				Status:      status,
				StatusClass: statusClass,
				IsUp:        isUp,
				ConfigPath:  defaultConfigPathFor(service, instance),
				AddrInput:   instance.Addr,
			})
		}
		toggleInstance := strings.TrimSpace(service.ToggleInstance)
		if toggleInstance == "" && len(service.Instances) > 0 {
			toggleInstance = service.Instances[0].Name
		}
		toggleConfigPath := ""
		for _, inst := range instances {
			if inst.Name == toggleInstance {
				toggleConfigPath = inst.ConfigPath
				break
			}
		}
		toggleIsUp := false
		if service.SingleToggle {
			for _, inst := range instances {
				if inst.Name == toggleInstance {
					toggleIsUp = inst.IsUp
					break
				}
			}
		} else {
			for _, inst := range instances {
				if inst.IsUp {
					toggleIsUp = true
					break
				}
			}
		}
		services = append(services, serviceView{
			ID:                 service.ID,
			Label:              service.Label,
			Instances:          instances,
			HasStart:           len(service.StartCommand) > 0,
			HasStop:            len(service.StopCommand) > 0,
			NeedsConfig:        needsConfig,
			NeedsAddr:          needsAddr,
			SingleToggle:       service.SingleToggle,
			ToggleInstanceName: toggleInstance,
			ToggleIsUp:         toggleIsUp,
			ToggleConfigPath:   toggleConfigPath,
		})
	}
	return servicesView{Services: services, Notice: notice}
}

func defaultConfigPathFor(service serviceConfig, instance serviceInstance) string {
	if strings.TrimSpace(instance.ConfigPath) != "" {
		return instance.ConfigPath
	}
	return service.DefaultConfigPath
}

func resolveConfigPath(workingDir, configDir, input string) (string, string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errors.New("config path is required")
	}
	if filepath.IsAbs(input) {
		return "", "", errors.New("config path must be relative")
	}
	cleaned := filepath.Clean(input)
	if cleaned == "." || strings.HasPrefix(cleaned, "..") {
		return "", "", errors.New("config path must be under conf/")
	}
	fullPath := filepath.Join(workingDir, cleaned)
	rel, err := filepath.Rel(configDir, fullPath)
	if err != nil {
		return "", "", errors.New("config path must be under conf/")
	}
	if rel == "." || strings.HasPrefix(rel, "..") {
		return "", "", errors.New("config path must be under conf/")
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		return "", "", err
	}
	if info.IsDir() {
		return "", "", errors.New("config path is a directory")
	}
	return fullPath, filepath.ToSlash(cleaned), nil
}

func waitForAddrUp(addr string, timeout time.Duration, exitCh <-chan error) error {
	ticker := time.NewTicker(startPollInterval)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		if isAddrUp(addr) {
			return nil
		}
		select {
		case err := <-exitCh:
			if err != nil {
				return fmt.Errorf("process exited: %v", err)
			}
			return errors.New("process exited")
		case <-ticker.C:
			continue
		case <-timer.C:
			return fmt.Errorf("not listening on %s after %s", addr, timeout)
		}
	}
}

func waitForAddrDown(addr string, timeout time.Duration) error {
	ticker := time.NewTicker(startPollInterval)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		if !isAddrUp(addr) {
			return nil
		}
		select {
		case <-ticker.C:
			continue
		case <-timer.C:
			return fmt.Errorf("still listening on %s after %s", addr, timeout)
		}
	}
}

func formatStartError(err error, output string) string {
	message := strings.TrimSpace(output)
	if message != "" {
		message = summarizeOutput(message)
		return fmt.Sprintf("start failed: %v: %s", err, message)
	}
	return fmt.Sprintf("start failed: %v", err)
}

func summarizeOutput(output string) string {
	output = strings.ReplaceAll(output, "\n", " ")
	output = strings.TrimSpace(output)
	if len(output) > maxStartOutput {
		return output[:maxStartOutput] + "..."
	}
	return output
}

func isAddrUp(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, statusDialTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func findServiceInstance(cfg fileConfig, serviceID, instanceName string) (serviceConfig, serviceInstance, error) {
	for _, service := range cfg.Services {
		if service.ID != serviceID {
			continue
		}
		for _, instance := range service.Instances {
			if instance.Name == instanceName {
				return service, instance, nil
			}
		}
		return serviceConfig{}, serviceInstance{}, fmt.Errorf("instance not found: %s", instanceName)
	}
	return serviceConfig{}, serviceInstance{}, fmt.Errorf("service not found: %s", serviceID)
}

func buildCommand(args []string, configPath, addr, port string) ([]string, error) {
	if len(args) == 0 {
		return nil, errors.New("command not configured")
	}
	out := make([]string, 0, len(args))
	for _, arg := range args {
		replaced := strings.ReplaceAll(arg, "{config}", configPath)
		replaced = strings.ReplaceAll(replaced, "{addr}", addr)
		replaced = strings.ReplaceAll(replaced, "{port}", port)
		out = append(out, replaced)
	}
	if hasPlaceholder(args, "{config}") && strings.TrimSpace(configPath) == "" {
		return nil, errors.New("config path is required")
	}
	if hasPlaceholder(args, "{addr}") && strings.TrimSpace(addr) == "" {
		return nil, errors.New("addr is required")
	}
	if hasPlaceholder(args, "{port}") && strings.TrimSpace(port) == "" {
		return nil, errors.New("port is required")
	}
	return out, nil
}

func hasPlaceholder(args []string, token string) bool {
	for _, arg := range args {
		if strings.Contains(arg, token) {
			return true
		}
	}
	return false
}

func splitAddr(addr string) (string, string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", "", errors.New("addr is required")
	}
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1", strings.TrimPrefix(addr, ":"), nil
	}
	host, port, err := net.SplitHostPort(addr)
	if err == nil {
		if host == "" {
			host = "127.0.0.1"
		}
		return host, port, nil
	}
	if strings.Count(addr, ":") == 0 {
		return "127.0.0.1", addr, nil
	}
	return "", "", err
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
	renderShell(w, fragment, u.title)
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
	if _, err := fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><link rel="stylesheet" href="/static/ui.css"><title>%s</title></head><body><div class="topbar"><div class="topbar-brand"><svg class="topbar-logo" viewBox="0 0 48 24" aria-hidden="true" focusable="false"><path d="M2 18c6-10 12-14 22-14s16 4 22 14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/><path d="M8 18v-6M40 18v-6M16 18v-4M32 18v-4" stroke="currentColor" stroke-width="2" stroke-linecap="round"/><path d="M2 18h44" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg><span class="topbar-title">Setu</span></div></div><div id="ui-root">`, title); err != nil {
		log.Printf("write shell start: %v", err)
		return
	}
	if _, err := w.Write(fragment); err != nil {
		log.Printf("write shell fragment: %v", err)
		return
	}
	if _, err := io.WriteString(w, `</div><script src="/static/htmx.min.js"></script><script src="/static/theme.js"></script></body></html>`); err != nil {
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
