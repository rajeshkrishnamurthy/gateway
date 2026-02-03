package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
)

const defaultConfigPath = "conf/docker/services_health.json"

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
	HealthURL  string `json:"healthUrl"`
	ConfigPath string `json:"configPath"`
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
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fileConfig{}, errors.New("config has trailing data")
	}
	if err := validateConfig(cfg); err != nil {
		return fileConfig{}, err
	}
	return cfg, nil
}

func validateConfig(cfg fileConfig) error {
	for _, service := range cfg.Services {
		for _, instance := range service.Instances {
			if _, err := resolveHealthURL(instance); err != nil {
				return err
			}
		}
	}
	return nil
}
