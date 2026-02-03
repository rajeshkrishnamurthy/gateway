package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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

func waitForHealthUp(healthURL string, timeout time.Duration, exitCh <-chan error) error {
	ticker := time.NewTicker(startPollInterval)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		if isHealthUp(healthURL) {
			return nil
		}
		select {
		case err := <-exitCh:
			if err != nil {
				return fmt.Errorf("process exited: %v", err)
			}
			continue
		case <-ticker.C:
			continue
		case <-timer.C:
			return fmt.Errorf("not healthy at %s after %s", healthURL, timeout)
		}
	}
}

func waitForHealthDown(healthURL string, timeout time.Duration) error {
	ticker := time.NewTicker(startPollInterval)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		if !isHealthUp(healthURL) {
			return nil
		}
		select {
		case <-ticker.C:
			continue
		case <-timer.C:
			return fmt.Errorf("still healthy at %s after %s", healthURL, timeout)
		}
	}
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
