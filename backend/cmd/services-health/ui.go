package main

import (
	"fmt"
	"os"
	"path/filepath"
)

type uiServer struct {
	templates  uiTemplates
	staticDir  string
	config     fileConfig
	title      string
	workingDir string
	configDir  string
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
