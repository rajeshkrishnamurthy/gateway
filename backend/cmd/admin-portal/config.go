package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
	cfg.SubmissionManagerURL = strings.TrimRight(strings.TrimSpace(cfg.SubmissionManagerURL), "/")
	cfg.SMSSubmissionTarget = strings.TrimSpace(cfg.SMSSubmissionTarget)
	cfg.PushSubmissionTarget = strings.TrimSpace(cfg.PushSubmissionTarget)
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
