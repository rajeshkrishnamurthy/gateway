package main

import (
	"encoding/json"
	"html/template"
	"net/http"
)

type fileConfig struct {
	Title                string `json:"title"`
	SMSGatewayURL        string `json:"smsGatewayUrl"`
	PushGatewayURL       string `json:"pushGatewayUrl"`
	SubmissionManagerURL string `json:"submissionManagerUrl"`
	SMSSubmissionTarget  string `json:"smsSubmissionTarget"`
	PushSubmissionTarget string `json:"pushSubmissionTarget"`
	CommandCenterURL     string `json:"commandCenterUrl"`
	HAProxyStatsURL      string `json:"haproxyStatsUrl"`
}

type portalTemplates struct {
	topbar           *template.Template
	overview         *template.Template
	haproxy          *template.Template
	errView          *template.Template
	submissionResult *template.Template
}

type portalServer struct {
	config    fileConfig
	templates portalTemplates
	staticDir string
	client    *http.Client
}

type submissionResultView struct {
	IntentID        string
	StatusEndpoint  string
	Status          string
	RejectedReason  string
	ExhaustedReason string
	CompletedAt     string
	Error           string
}

type submissionIntentRequest struct {
	IntentID         string          `json:"intentId"`
	SubmissionTarget string          `json:"submissionTarget"`
	Payload          json.RawMessage `json:"payload"`
}

type submissionIntentResponse struct {
	IntentID         string `json:"intentId"`
	SubmissionTarget string `json:"submissionTarget"`
	CreatedAt        string `json:"createdAt"`
	Status           string `json:"status"`
	CompletedAt      string `json:"completedAt,omitempty"`
	RejectedReason   string `json:"rejectedReason,omitempty"`
	ExhaustedReason  string `json:"exhaustedReason,omitempty"`
}

type submissionErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type smsTestRequest struct {
	ReferenceID string `json:"referenceId"`
	To          string `json:"to"`
	Message     string `json:"message"`
	TenantID    string `json:"tenantId"`
}

type pushTestRequest struct {
	ReferenceID string `json:"referenceId"`
	Token       string `json:"token"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	TenantID    string `json:"tenantId"`
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
