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
	SMSProvider                      string `json:"smsProvider"`
	SMSProviderURL                   string `json:"smsProviderUrl"`
	SMSProviderVersion               string `json:"smsProviderVersion"`
	SMSProviderServiceName           string `json:"smsProviderServiceName"`
	SMSProviderSenderID              string `json:"smsProviderSenderId"`
	SMSProviderTimeoutSeconds        int    `json:"smsProviderTimeoutSeconds"`
	SMSProviderConnectTimeoutSeconds int    `json:"smsProviderConnectTimeoutSeconds"`
	GrafanaDashboardURL              string `json:"grafanaDashboardUrl"`
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

	cfg.SMSProvider = strings.TrimSpace(cfg.SMSProvider)
	if cfg.SMSProvider == "" {
		cfg.SMSProvider = "default"
	}
	switch cfg.SMSProvider {
	case "default", "model", "sms24x7", "smskarix", "smsinfobip":
	default:
		return fileConfig{}, errors.New("smsProvider must be one of: default, model, sms24x7, smskarix, smsinfobip")
	}
	if strings.TrimSpace(cfg.SMSProviderURL) == "" {
		return fileConfig{}, errors.New("smsProviderUrl is required")
	}
	if cfg.SMSProviderTimeoutSeconds < 15 || cfg.SMSProviderTimeoutSeconds > 60 {
		return fileConfig{}, errors.New("smsProviderTimeoutSeconds must be between 15 and 60")
	}
	if cfg.SMSProviderConnectTimeoutSeconds == 0 {
		cfg.SMSProviderConnectTimeoutSeconds = int(minProviderConnectTimeout / time.Second)
	}
	connectTimeout := time.Duration(cfg.SMSProviderConnectTimeoutSeconds) * time.Second
	if connectTimeout < minProviderConnectTimeout || connectTimeout > maxProviderConnectTimeout {
		return fileConfig{}, errors.New("smsProviderConnectTimeoutSeconds must be between 2 and 10")
	}

	return cfg, nil
}
