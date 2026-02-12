package submissionmanagerapi

import (
	"encoding/json"
	"fmt"
)

const (
	OpenAPIPath = "/openapi.json"
	DocsPath    = "/docs"
)

type Document struct {
	OpenAPI    string                          `json:"openapi"`
	Info       Info                            `json:"info"`
	Paths      map[string]map[string]Operation `json:"paths"`
	Components Components                      `json:"components"`
}

type Info struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

type Components struct {
	Schemas map[string]map[string]any `json:"schemas"`
}

type Operation struct {
	Summary     string              `json:"summary,omitempty"`
	Parameters  []Parameter         `json:"parameters,omitempty"`
	RequestBody *RequestBody        `json:"requestBody,omitempty"`
	Responses   map[string]Response `json:"responses"`
}

type Parameter struct {
	Name        string         `json:"name"`
	In          string         `json:"in"`
	Required    bool           `json:"required"`
	Description string         `json:"description,omitempty"`
	Schema      map[string]any `json:"schema"`
}

type RequestBody struct {
	Required bool             `json:"required"`
	Content  map[string]Media `json:"content"`
}

type Response struct {
	Description string           `json:"description"`
	Content     map[string]Media `json:"content,omitempty"`
}

type Media struct {
	Schema map[string]any `json:"schema"`
}

func BuildOpenAPIDocument() Document {
	return Document{
		OpenAPI: "3.0.3",
		Info: Info{
			Title:       "SubmissionManager API",
			Version:     "1.0.0",
			Description: "Canonical HTTP contract for the SubmissionManager bounded context.",
		},
		Paths: buildPaths(),
		Components: Components{
			Schemas: buildSchemas(),
		},
	}
}

func MarshalOpenAPIJSON() ([]byte, error) {
	return json.MarshalIndent(BuildOpenAPIDocument(), "", "  ")
}

func SwaggerUIHTML(specURL string) string {
	if specURL == "" {
		specURL = OpenAPIPath
	}
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>SubmissionManager API Docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = function() {
      SwaggerUIBundle({
        url: %q,
        dom_id: '#swagger-ui',
        deepLinking: true
      });
    };
  </script>
</body>
</html>
`, specURL)
}

func buildPaths() map[string]map[string]Operation {
	return map[string]map[string]Operation{
		"/healthz": {
			"get": {
				Summary: "Process health check",
				Responses: map[string]Response{
					"200": textResponse("Process is running."),
					"405": textResponse("Method not allowed."),
				},
			},
		},
		"/readyz": {
			"get": {
				Summary: "Readiness and role status",
				Responses: map[string]Response{
					"200": textResponse("Process is ready and reports role state."),
					"405": textResponse("Method not allowed."),
				},
			},
		},
		"/metrics": {
			"get": {
				Summary: "Prometheus metrics",
				Responses: map[string]Response{
					"200": textResponse("Prometheus metrics payload."),
					"404": {Description: "Metrics are not configured."},
					"405": textResponse("Method not allowed."),
				},
			},
		},
		OpenAPIPath: {
			"get": {
				Summary: "Live OpenAPI document generated from service code",
				Responses: map[string]Response{
					"200": {
						Description: "OpenAPI 3.0 JSON document.",
						Content: map[string]Media{
							"application/json": {
								Schema: map[string]any{"type": "object"},
							},
						},
					},
					"405": textResponse("Method not allowed."),
					"500": textResponse("Unable to generate OpenAPI document."),
				},
			},
		},
		DocsPath: {
			"get": {
				Summary: "Live Swagger UI documentation for SubmissionManager API",
				Responses: map[string]Response{
					"200": htmlResponse("Swagger UI HTML page."),
					"405": textResponse("Method not allowed."),
				},
			},
		},
		"/v1/intents": {
			"post": {
				Summary: "Create or query an intent (idempotent)",
				Parameters: []Parameter{
					{
						Name:        "waitSeconds",
						In:          "query",
						Required:    false,
						Description: "Optional synchronous wait duration in seconds. Must be a non-negative integer. Values greater than 30 are clamped to 30.",
						Schema: map[string]any{
							"type":    "integer",
							"minimum": 0,
						},
					},
				},
				RequestBody: &RequestBody{
					Required: true,
					Content: map[string]Media{
						"application/json": {
							Schema: map[string]any{"$ref": "#/components/schemas/SubmitIntentRequest"},
						},
					},
				},
				Responses: map[string]Response{
					"200": jsonRefResponse("Intent accepted for orchestration or current idempotent state.", "#/components/schemas/IntentResponse"),
					"400": jsonRefResponse("Invalid request body, invalid waitSeconds, missing required fields, or unknown submission target.", "#/components/schemas/ErrorResponse"),
					"404": jsonRefResponse("Intent not found after synchronous wait reconciliation.", "#/components/schemas/ErrorResponse"),
					"405": jsonRefResponse("Method not allowed.", "#/components/schemas/ErrorResponse"),
					"409": jsonRefResponse("Idempotency conflict for intentId.", "#/components/schemas/ErrorResponse"),
					"500": jsonRefResponse("Internal error.", "#/components/schemas/ErrorResponse"),
				},
			},
		},
		"/v1/intents/{intentId}": {
			"get": {
				Summary: "Get current intent state",
				Parameters: []Parameter{
					{
						Name:     "intentId",
						In:       "path",
						Required: true,
						Schema: map[string]any{
							"type": "string",
						},
					},
				},
				Responses: map[string]Response{
					"200": jsonRefResponse("Current intent state.", "#/components/schemas/IntentResponse"),
					"400": jsonRefResponse("Invalid request path.", "#/components/schemas/ErrorResponse"),
					"404": jsonRefResponse("Intent not found.", "#/components/schemas/ErrorResponse"),
					"405": jsonRefResponse("Method not allowed.", "#/components/schemas/ErrorResponse"),
				},
			},
		},
		"/v1/intents/{intentId}/history": {
			"get": {
				Summary: "Get intent state with ordered attempt history",
				Parameters: []Parameter{
					{
						Name:     "intentId",
						In:       "path",
						Required: true,
						Schema: map[string]any{
							"type": "string",
						},
					},
				},
				Responses: map[string]Response{
					"200": jsonRefResponse("Intent state and attempt history.", "#/components/schemas/IntentHistoryResponse"),
					"400": jsonRefResponse("Invalid request path.", "#/components/schemas/ErrorResponse"),
					"404": jsonRefResponse("Intent not found.", "#/components/schemas/ErrorResponse"),
					"405": jsonRefResponse("Method not allowed.", "#/components/schemas/ErrorResponse"),
				},
			},
		},
		"/ui/history": {
			"post": {
				Summary: "Render intent history HTML fragment (enabled when SubmissionManager UI templates are configured)",
				RequestBody: &RequestBody{
					Required: true,
					Content: map[string]Media{
						"application/x-www-form-urlencoded": {
							Schema: map[string]any{
								"type":     "object",
								"required": []any{"intentId"},
								"properties": map[string]any{
									"intentId": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
				Responses: map[string]Response{
					"200": htmlResponse("Rendered HTML history fragment."),
					"400": textResponse("Invalid form or missing intentId."),
					"404": textResponse("Intent not found."),
					"405": textResponse("Method not allowed."),
					"500": textResponse("Manager not configured or template render error."),
				},
			},
		},
	}
}

func buildSchemas() map[string]map[string]any {
	return map[string]map[string]any{
		"SubmitIntentRequest": {
			"type":     "object",
			"required": []any{"intentId", "submissionTarget"},
			"properties": map[string]any{
				"intentId":         map[string]any{"type": "string"},
				"submissionTarget": map[string]any{"type": "string"},
				"payload": map[string]any{
					"description": "Opaque JSON payload forwarded to the resolved gateway contract.",
				},
			},
		},
		"IntentResponse": {
			"type":     "object",
			"required": []any{"intentId", "submissionTarget", "createdAt", "status"},
			"properties": map[string]any{
				"intentId":         map[string]any{"type": "string"},
				"submissionTarget": map[string]any{"type": "string"},
				"createdAt":        map[string]any{"type": "string", "format": "date-time"},
				"status": map[string]any{
					"type": "string",
					"enum": []any{"pending", "accepted", "rejected", "exhausted"},
				},
				"completedAt":     map[string]any{"type": "string", "format": "date-time"},
				"rejectedReason":  map[string]any{"type": "string"},
				"exhaustedReason": map[string]any{"type": "string"},
			},
		},
		"AttemptResponse": {
			"type":     "object",
			"required": []any{"attemptNumber"},
			"properties": map[string]any{
				"attemptNumber": map[string]any{"type": "integer"},
				"startedAt":     map[string]any{"type": "string", "format": "date-time"},
				"finishedAt":    map[string]any{"type": "string", "format": "date-time"},
				"outcomeStatus": map[string]any{"type": "string"},
				"outcomeReason": map[string]any{"type": "string"},
				"error":         map[string]any{"type": "string"},
			},
		},
		"IntentHistoryResponse": {
			"type":     "object",
			"required": []any{"intent", "attempts"},
			"properties": map[string]any{
				"intent": map[string]any{
					"$ref": "#/components/schemas/IntentResponse",
				},
				"attempts": map[string]any{
					"type": "array",
					"items": map[string]any{
						"$ref": "#/components/schemas/AttemptResponse",
					},
				},
			},
		},
		"ErrorBody": {
			"type":     "object",
			"required": []any{"code", "message"},
			"properties": map[string]any{
				"code":    map[string]any{"type": "string"},
				"message": map[string]any{"type": "string"},
				"details": map[string]any{
					"type": "object",
					"additionalProperties": map[string]any{
						"type": "string",
					},
				},
			},
		},
		"ErrorResponse": {
			"type":     "object",
			"required": []any{"error"},
			"properties": map[string]any{
				"error": map[string]any{
					"$ref": "#/components/schemas/ErrorBody",
				},
			},
		},
	}
}

func jsonRefResponse(description, ref string) Response {
	return Response{
		Description: description,
		Content: map[string]Media{
			"application/json": {
				Schema: map[string]any{"$ref": ref},
			},
		},
	}
}

func textResponse(description string) Response {
	return Response{
		Description: description,
		Content: map[string]Media{
			"text/plain": {
				Schema: map[string]any{"type": "string"},
			},
		},
	}
}

func htmlResponse(description string) Response {
	return Response{
		Description: description,
		Content: map[string]Media{
			"text/html": {
				Schema: map[string]any{"type": "string"},
			},
		},
	}
}
