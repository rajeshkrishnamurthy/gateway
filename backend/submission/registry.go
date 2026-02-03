package submission

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
)

// GatewayType identifies a gateway family with defined response semantics.
type GatewayType string

const (
	// GatewaySMS is the SMS gateway type.
	GatewaySMS GatewayType = "sms"
	// GatewayPush is the push gateway type.
	GatewayPush GatewayType = "push"
)

// ContractPolicy selects the retry termination rule for a submissionTarget.
type ContractPolicy string

const (
	// PolicyDeadline exhausts when the acceptance deadline is reached.
	PolicyDeadline ContractPolicy = "deadline"
	// PolicyMaxAttempts exhausts when the attempt count reaches the limit.
	PolicyMaxAttempts ContractPolicy = "max_attempts"
	// PolicyOneShot allows only a single attempt.
	PolicyOneShot ContractPolicy = "one_shot"
)

// TargetContract is the resolved contract snapshot for a submissionTarget.
type TargetContract struct {
	SubmissionTarget string
	GatewayType      GatewayType
	GatewayURL       string
	Policy           ContractPolicy
	// MaxAcceptanceSeconds is the cumulative wall-clock deadline from intent
	// creation within which the submission must be accepted. It is not a
	// per-attempt timeout. It is required when policy is "deadline".
	MaxAcceptanceSeconds int
	// MaxAttempts is required when policy is "max_attempts".
	MaxAttempts int
	// TerminalOutcomes lists gateway-reported outcomes that, under this
	// submission contract, must immediately complete the intent without
	// further attempts.
	TerminalOutcomes []string
	Webhook          *WebhookConfig
}

// Registry maps submissionTarget identifiers to validated TargetContracts.
type Registry struct {
	Targets map[string]TargetContract
}

type fileConfig struct {
	// Non-obvious constraint: unsigned webhooks are rejected unless this is true.
	AllowUnsignedWebhooks bool           `json:"allowUnsignedWebhooks"`
	Targets               []targetConfig `json:"targets"`
}

type targetConfig struct {
	SubmissionTarget     string         `json:"submissionTarget"`
	GatewayType          string         `json:"gatewayType"`
	GatewayURL           string         `json:"gatewayUrl"`
	Policy               string         `json:"policy"`
	MaxAcceptanceSeconds int            `json:"maxAcceptanceSeconds"`
	MaxAttempts          int            `json:"maxAttempts"`
	TerminalOutcomes     []string       `json:"terminalOutcomes"`
	Webhook              *webhookConfig `json:"webhook"`
}

// WebhookConfig defines the terminal webhook callback for a submissionTarget.
type WebhookConfig struct {
	URL        string
	Headers    map[string]string
	HeadersEnv map[string]string
	SecretEnv  string
}

type webhookConfig struct {
	URL        string            `json:"url"`
	Headers    map[string]string `json:"headers"`
	HeadersEnv map[string]string `json:"headersEnv"`
	SecretEnv  string            `json:"secretEnv"`
}

var allowedOutcomes = map[GatewayType]map[string]struct{}{
	GatewaySMS: {
		"invalid_request":     {},
		"duplicate_reference": {},
		"invalid_recipient":   {},
		"invalid_message":     {},
		"provider_failure":    {},
	},
	GatewayPush: {
		"invalid_request":     {},
		"duplicate_reference": {},
		"provider_failure":    {},
		"unregistered_token":  {},
	},
}

// IsKnownOutcome reports whether the reason is a known gateway outcome for the
// given gatewayType.
func IsKnownOutcome(gatewayType GatewayType, reason string) bool {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return false
	}
	allowed := allowedOutcomes[gatewayType]
	if allowed == nil {
		return false
	}
	_, ok := allowed[trimmed]
	return ok
}

// LoadRegistry loads and validates a SubmissionTarget registry JSON file.
func LoadRegistry(path string) (Registry, error) {
	file, err := os.Open(path)
	if err != nil {
		return Registry{}, err
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
		return Registry{}, err
	}

	dec := json.NewDecoder(&filtered)
	dec.DisallowUnknownFields()
	var cfg fileConfig
	if err := dec.Decode(&cfg); err != nil {
		return Registry{}, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return Registry{}, errors.New("config has trailing data")
	}

	return buildRegistry(cfg)
}

// ContractFor returns the contract for a submissionTarget if it exists.
func (r Registry) ContractFor(target string) (TargetContract, bool) {
	if r.Targets == nil {
		return TargetContract{}, false
	}
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return TargetContract{}, false
	}
	contract, ok := r.Targets[trimmed]
	return contract, ok
}

func buildRegistry(cfg fileConfig) (Registry, error) {
	if len(cfg.Targets) == 0 {
		return Registry{}, errors.New("targets must not be empty")
	}

	registry := Registry{
		Targets: make(map[string]TargetContract, len(cfg.Targets)),
	}

	for i, target := range cfg.Targets {
		submissionTarget := strings.TrimSpace(target.SubmissionTarget)
		if submissionTarget == "" {
			return Registry{}, fmt.Errorf("targets[%d].submissionTarget is required", i)
		}
		if _, exists := registry.Targets[submissionTarget]; exists {
			return Registry{}, fmt.Errorf("targets[%d].submissionTarget %q is duplicated", i, submissionTarget)
		}

		gatewayTypeValue := strings.TrimSpace(target.GatewayType)
		if gatewayTypeValue == "" {
			return Registry{}, fmt.Errorf("targets[%d].gatewayType is required", i)
		}
		var gatewayType GatewayType
		switch gatewayTypeValue {
		case string(GatewaySMS):
			gatewayType = GatewaySMS
		case string(GatewayPush):
			gatewayType = GatewayPush
		default:
			return Registry{}, fmt.Errorf("targets[%d].gatewayType must be one of: sms, push", i)
		}

		gatewayURL := strings.TrimSpace(target.GatewayURL)
		if gatewayURL == "" {
			return Registry{}, fmt.Errorf("targets[%d].gatewayUrl is required", i)
		}
		if err := validateGatewayURL(gatewayURL); err != nil {
			return Registry{}, fmt.Errorf("targets[%d].gatewayUrl %v", i, err)
		}

		if target.MaxAcceptanceSeconds < 0 {
			return Registry{}, fmt.Errorf("targets[%d].maxAcceptanceSeconds must be zero or greater", i)
		}
		if target.MaxAttempts < 0 {
			return Registry{}, fmt.Errorf("targets[%d].maxAttempts must be zero or greater", i)
		}

		policyValue := strings.TrimSpace(target.Policy)
		if policyValue == "" {
			return Registry{}, fmt.Errorf("targets[%d].policy is required", i)
		}
		var policy ContractPolicy
		switch policyValue {
		case string(PolicyDeadline):
			policy = PolicyDeadline
			if target.MaxAcceptanceSeconds <= 0 {
				return Registry{}, fmt.Errorf("targets[%d].maxAcceptanceSeconds must be greater than zero", i)
			}
			if target.MaxAttempts > 0 {
				return Registry{}, fmt.Errorf("targets[%d].maxAttempts must be empty when policy is deadline", i)
			}
		case string(PolicyMaxAttempts):
			policy = PolicyMaxAttempts
			if target.MaxAttempts <= 0 {
				return Registry{}, fmt.Errorf("targets[%d].maxAttempts must be greater than zero", i)
			}
			if target.MaxAcceptanceSeconds > 0 {
				return Registry{}, fmt.Errorf("targets[%d].maxAcceptanceSeconds must be empty when policy is max_attempts", i)
			}
		case string(PolicyOneShot):
			policy = PolicyOneShot
			if target.MaxAttempts > 0 {
				return Registry{}, fmt.Errorf("targets[%d].maxAttempts must be empty when policy is one_shot", i)
			}
			if target.MaxAcceptanceSeconds > 0 {
				return Registry{}, fmt.Errorf("targets[%d].maxAcceptanceSeconds must be empty when policy is one_shot", i)
			}
		default:
			return Registry{}, fmt.Errorf("targets[%d].policy must be one of: deadline, max_attempts, one_shot", i)
		}

		if len(target.TerminalOutcomes) == 0 {
			return Registry{}, fmt.Errorf("targets[%d].terminalOutcomes is required", i)
		}

		outcomes := make([]string, 0, len(target.TerminalOutcomes))
		seen := make(map[string]struct{}, len(target.TerminalOutcomes))
		allowed := allowedOutcomes[gatewayType]
		for _, outcome := range target.TerminalOutcomes {
			trimmed := strings.TrimSpace(outcome)
			if trimmed == "" {
				return Registry{}, fmt.Errorf("targets[%d].terminalOutcomes must not include empty values", i)
			}
			if _, ok := allowed[trimmed]; !ok {
				return Registry{}, fmt.Errorf("targets[%d].terminalOutcomes contains unknown outcome %q for gatewayType %q", i, trimmed, gatewayType)
			}
			if _, ok := seen[trimmed]; ok {
				return Registry{}, fmt.Errorf("targets[%d].terminalOutcomes contains duplicate value %q", i, trimmed)
			}
			seen[trimmed] = struct{}{}
			outcomes = append(outcomes, trimmed)
		}

		webhook, err := validateWebhook(target.Webhook, cfg.AllowUnsignedWebhooks, i)
		if err != nil {
			return Registry{}, err
		}

		registry.Targets[submissionTarget] = TargetContract{
			SubmissionTarget:     submissionTarget,
			GatewayType:          gatewayType,
			GatewayURL:           gatewayURL,
			Policy:               policy,
			MaxAcceptanceSeconds: target.MaxAcceptanceSeconds,
			MaxAttempts:          target.MaxAttempts,
			TerminalOutcomes:     outcomes,
			Webhook:              webhook,
		}
	}

	return registry, nil
}

func validateGatewayURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return errors.New("must be a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("must use http or https")
	}
	if parsed.Host == "" {
		return errors.New("must include host")
	}
	return nil
}

func validateWebhook(cfg *webhookConfig, allowUnsigned bool, idx int) (*WebhookConfig, error) {
	if cfg == nil {
		return nil, nil
	}

	urlValue := strings.TrimSpace(cfg.URL)
	if urlValue == "" {
		return nil, fmt.Errorf("targets[%d].webhook.url is required", idx)
	}
	if err := validateWebhookURL(urlValue); err != nil {
		return nil, fmt.Errorf("targets[%d].webhook.url %v", idx, err)
	}

	headers, err := normalizeHeaderMap(cfg.Headers, fmt.Sprintf("targets[%d].webhook.headers", idx))
	if err != nil {
		return nil, err
	}
	headersEnv, err := normalizeHeaderMap(cfg.HeadersEnv, fmt.Sprintf("targets[%d].webhook.headersEnv", idx))
	if err != nil {
		return nil, err
	}

	if len(headers) > 0 && len(headersEnv) > 0 {
		for name := range headers {
			if _, exists := headersEnv[name]; exists {
				return nil, fmt.Errorf("targets[%d].webhook header %q cannot be in both headers and headersEnv", idx, name)
			}
		}
	}

	secretEnv := strings.TrimSpace(cfg.SecretEnv)
	if secretEnv == "" && !allowUnsigned {
		// Non-obvious constraint: unsigned webhooks require explicit opt-in.
		return nil, fmt.Errorf("targets[%d].webhook.secretEnv is required unless allowUnsignedWebhooks is true", idx)
	}

	return &WebhookConfig{
		URL:        urlValue,
		Headers:    headers,
		HeadersEnv: headersEnv,
		SecretEnv:  secretEnv,
	}, nil
}

func validateWebhookURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return errors.New("must be a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("must use http or https")
	}
	if parsed.Host == "" {
		return errors.New("must include host")
	}
	return nil
}

func normalizeHeaderMap(input map[string]string, field string) (map[string]string, error) {
	if len(input) == 0 {
		return nil, nil
	}
	normalized := make(map[string]string, len(input))
	for name, value := range input {
		trimmedName := strings.TrimSpace(name)
		if trimmedName == "" {
			return nil, fmt.Errorf("%s must not include empty header names", field)
		}
		trimmedValue := strings.TrimSpace(value)
		if trimmedValue == "" {
			return nil, fmt.Errorf("%s must not include empty values for header %q", field, trimmedName)
		}
		if _, exists := normalized[trimmedName]; exists {
			return nil, fmt.Errorf("%s contains duplicate header %q", field, trimmedName)
		}
		normalized[trimmedName] = trimmedValue
	}
	return normalized, nil
}
