package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"
)

const (
	minProviderConnectTimeout = 2 * time.Second
	maxProviderConnectTimeout = 10 * time.Second
)

type fileConfig struct {
	PushProvider                      string `json:"pushProvider"`
	PushProviderURL                   string `json:"pushProviderUrl"`
	PushProviderTimeoutSeconds        int    `json:"pushProviderTimeoutSeconds"`
	PushProviderConnectTimeoutSeconds int    `json:"pushProviderConnectTimeoutSeconds"`
	GrafanaDashboardURL               string `json:"grafanaDashboardUrl"`
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
