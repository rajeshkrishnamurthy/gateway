package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

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
