package main

import (
	"fmt"
	"strings"
)

const (
	statusUpClass   = "status-up"
	statusDownClass = "status-down"
	defaultStatus   = "down"
)

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
			_, port, err := splitAddr(addr)
			healthURL, healthErr := resolveHealthURL(instance)
			status := defaultStatus
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
			if err == nil && healthErr == nil && !overrideApplied {
				if isHealthUp(healthURL) {
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

func resolveHealthURL(instance serviceInstance) (string, error) {
	healthURL := strings.TrimSpace(instance.HealthURL)
	if healthURL == "" {
		return "", fmt.Errorf("healthUrl is required for %s", instance.Name)
	}
	if !strings.HasPrefix(healthURL, "http://") && !strings.HasPrefix(healthURL, "https://") {
		return "", fmt.Errorf("healthUrl must start with http:// or https:// for %s", instance.Name)
	}
	return healthURL, nil
}
