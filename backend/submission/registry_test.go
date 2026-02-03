package submission

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRegistryValidConfig(t *testing.T) {
	config := `# top comment
{
  "allowUnsignedWebhooks": true,
  "targets": [
    {
      "submissionTarget": "sms.realtime",
      "gatewayType": "sms",
      "gatewayUrl": "http://localhost:8080",
      "policy": "deadline",
      "maxAcceptanceSeconds": 30,
      "terminalOutcomes": ["invalid_request", "invalid_message"],
      "webhook": {
        "url": "http://localhost:9999/webhook",
        "headers": {
          "X-Env": "dev"
        }
      }
    },
    {
      "submissionTarget": "push.realtime",
      "gatewayType": "push",
      "gatewayUrl": "http://localhost:8081",
      "policy": "max_attempts",
      "maxAttempts": 3,
      "terminalOutcomes": ["invalid_request", "unregistered_token"]
    }
  ]
}
`

	path := writeTempConfig(t, config)
	registry, err := LoadRegistry(path)
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if len(registry.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(registry.Targets))
	}

	contract, ok := registry.ContractFor(" sms.realtime ")
	if !ok {
		t.Fatal("expected sms.realtime contract")
	}
	if contract.GatewayType != GatewaySMS {
		t.Fatalf("expected gatewayType sms, got %q", contract.GatewayType)
	}
	if contract.GatewayURL != "http://localhost:8080" {
		t.Fatalf("expected gatewayUrl http://localhost:8080, got %q", contract.GatewayURL)
	}
	if contract.Policy != PolicyDeadline {
		t.Fatalf("expected policy deadline, got %q", contract.Policy)
	}
	if contract.MaxAcceptanceSeconds != 30 {
		t.Fatalf("expected maxAcceptanceSeconds 30, got %d", contract.MaxAcceptanceSeconds)
	}
	if contract.MaxAttempts != 0 {
		t.Fatalf("expected maxAttempts 0, got %d", contract.MaxAttempts)
	}
	if contract.Webhook == nil {
		t.Fatal("expected webhook config")
	}
	if contract.Webhook.URL != "http://localhost:9999/webhook" {
		t.Fatalf("expected webhook url, got %q", contract.Webhook.URL)
	}

	pushContract, ok := registry.ContractFor("push.realtime")
	if !ok {
		t.Fatal("expected push.realtime contract")
	}
	if pushContract.Policy != PolicyMaxAttempts {
		t.Fatalf("expected policy max_attempts, got %q", pushContract.Policy)
	}
	if pushContract.MaxAttempts != 3 {
		t.Fatalf("expected maxAttempts 3, got %d", pushContract.MaxAttempts)
	}
}

func TestLoadRegistryRejectsUnsignedWebhookByDefault(t *testing.T) {
	config := `{
  "targets": [
    {
      "submissionTarget": "sms.realtime",
      "gatewayType": "sms",
      "gatewayUrl": "http://localhost:8080",
      "policy": "deadline",
      "maxAcceptanceSeconds": 30,
      "terminalOutcomes": ["invalid_request"],
      "webhook": {
        "url": "http://localhost:9999/webhook"
      }
    }
  ]
}
`
	path := writeTempConfig(t, config)
	_, err := LoadRegistry(path)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "secretEnv") {
		t.Fatalf("expected secretEnv error, got %q", err.Error())
	}
}

func TestLoadRegistryRejectsInvalidConfig(t *testing.T) {
	cases := []struct {
		name        string
		config      string
		wantContain string
	}{
		{
			name: "unknown gateway type",
			config: `{
  "targets": [
    {
      "submissionTarget": "sms.realtime",
      "gatewayType": "email",
      "gatewayUrl": "http://localhost:8080",
      "policy": "deadline",
      "maxAcceptanceSeconds": 30,
      "terminalOutcomes": ["invalid_request"]
    }
  ]
}
`,
			wantContain: "gatewayType",
		},
		{
			name: "unknown terminal outcome",
			config: `{
  "targets": [
    {
      "submissionTarget": "sms.realtime",
      "gatewayType": "sms",
      "gatewayUrl": "http://localhost:8080",
      "policy": "deadline",
      "maxAcceptanceSeconds": 30,
      "terminalOutcomes": ["unregistered_token"]
    }
  ]
}
`,
			wantContain: "terminalOutcomes",
		},
		{
			name: "invalid url",
			config: `{
  "targets": [
    {
      "submissionTarget": "sms.realtime",
      "gatewayType": "sms",
      "gatewayUrl": "localhost:8080",
      "policy": "deadline",
      "maxAcceptanceSeconds": 30,
      "terminalOutcomes": ["invalid_request"]
    }
  ]
}
`,
			wantContain: "gatewayUrl",
		},
		{
			name: "missing submissionTarget",
			config: `{
  "targets": [
    {
      "submissionTarget": " ",
      "gatewayType": "sms",
      "gatewayUrl": "http://localhost:8080",
      "policy": "deadline",
      "maxAcceptanceSeconds": 30,
      "terminalOutcomes": ["invalid_request"]
    }
  ]
}
`,
			wantContain: "submissionTarget",
		},
		{
			name: "duplicate submissionTarget",
			config: `{
  "targets": [
    {
      "submissionTarget": "sms.realtime",
      "gatewayType": "sms",
      "gatewayUrl": "http://localhost:8080",
      "policy": "deadline",
      "maxAcceptanceSeconds": 30,
      "terminalOutcomes": ["invalid_request"]
    },
    {
      "submissionTarget": "sms.realtime",
      "gatewayType": "sms",
      "gatewayUrl": "http://localhost:8080",
      "policy": "deadline",
      "maxAcceptanceSeconds": 30,
      "terminalOutcomes": ["invalid_request"]
    }
  ]
}
`,
			wantContain: "duplicated",
		},
		{
			name: "non-positive maxAcceptanceSeconds",
			config: `{
  "targets": [
    {
      "submissionTarget": "sms.realtime",
      "gatewayType": "sms",
      "gatewayUrl": "http://localhost:8080",
      "policy": "deadline",
      "maxAcceptanceSeconds": 0,
      "terminalOutcomes": ["invalid_request"]
    }
  ]
}
`,
			wantContain: "maxAcceptanceSeconds",
		},
		{
			name: "negative maxAcceptanceSeconds",
			config: `{
  "targets": [
    {
      "submissionTarget": "sms.realtime",
      "gatewayType": "sms",
      "gatewayUrl": "http://localhost:8080",
      "policy": "deadline",
      "maxAcceptanceSeconds": -1,
      "terminalOutcomes": ["invalid_request"]
    }
  ]
}
`,
			wantContain: "maxAcceptanceSeconds",
		},
		{
			name: "negative maxAttempts",
			config: `{
  "targets": [
    {
      "submissionTarget": "sms.realtime",
      "gatewayType": "sms",
      "gatewayUrl": "http://localhost:8080",
      "policy": "max_attempts",
      "maxAttempts": -2,
      "terminalOutcomes": ["invalid_request"]
    }
  ]
}
`,
			wantContain: "maxAttempts",
		},
		{
			name: "missing policy",
			config: `{
  "targets": [
    {
      "submissionTarget": "sms.realtime",
      "gatewayType": "sms",
      "gatewayUrl": "http://localhost:8080",
      "maxAcceptanceSeconds": 30,
      "terminalOutcomes": ["invalid_request"]
    }
  ]
}
`,
			wantContain: "policy",
		},
		{
			name: "invalid policy",
			config: `{
  "targets": [
    {
      "submissionTarget": "sms.realtime",
      "gatewayType": "sms",
      "gatewayUrl": "http://localhost:8080",
      "policy": "deadline_plus",
      "maxAcceptanceSeconds": 30,
      "terminalOutcomes": ["invalid_request"]
    }
  ]
}
`,
			wantContain: "policy",
		},
		{
			name: "deadline with maxAttempts",
			config: `{
  "targets": [
    {
      "submissionTarget": "sms.realtime",
      "gatewayType": "sms",
      "gatewayUrl": "http://localhost:8080",
      "policy": "deadline",
      "maxAcceptanceSeconds": 30,
      "maxAttempts": 2,
      "terminalOutcomes": ["invalid_request"]
    }
  ]
}
`,
			wantContain: "maxAttempts",
		},
		{
			name: "max_attempts with deadline",
			config: `{
  "targets": [
    {
      "submissionTarget": "sms.realtime",
      "gatewayType": "sms",
      "gatewayUrl": "http://localhost:8080",
      "policy": "max_attempts",
      "maxAcceptanceSeconds": 30,
      "maxAttempts": 2,
      "terminalOutcomes": ["invalid_request"]
    }
  ]
}
`,
			wantContain: "maxAcceptanceSeconds",
		},
		{
			name: "one_shot with maxAttempts",
			config: `{
  "targets": [
    {
      "submissionTarget": "sms.realtime",
      "gatewayType": "sms",
      "gatewayUrl": "http://localhost:8080",
      "policy": "one_shot",
      "maxAttempts": 2,
      "terminalOutcomes": ["invalid_request"]
    }
  ]
}
`,
			wantContain: "maxAttempts",
		},
		{
			name: "missing terminalOutcomes",
			config: `{
  "targets": [
    {
      "submissionTarget": "sms.realtime",
      "gatewayType": "sms",
      "gatewayUrl": "http://localhost:8080",
      "policy": "deadline",
      "maxAcceptanceSeconds": 30,
      "terminalOutcomes": []
    }
  ]
}
`,
			wantContain: "terminalOutcomes",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTempConfig(t, tc.config)
			_, err := LoadRegistry(path)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantContain) {
				t.Fatalf("expected error containing %q, got %q", tc.wantContain, err.Error())
			}
		})
	}
}

func writeTempConfig(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
