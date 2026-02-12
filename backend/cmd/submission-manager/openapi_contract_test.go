package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gateway/submissionmanager"
	"gateway/submissionmanagerapi"
)

func TestSubmissionManagerOpenAPIArtifactMatchesGeneratedOutput(t *testing.T) {
	path := openAPIArtifactPath(t)
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read OpenAPI artifact %s: %v", path, err)
	}
	generated, err := submissionmanagerapi.MarshalOpenAPIJSON()
	if err != nil {
		t.Fatalf("generate OpenAPI JSON: %v", err)
	}
	generated = append(generated, '\n')
	if !bytes.Equal(onDisk, generated) {
		t.Fatalf("OpenAPI artifact is stale; regenerate with: (cd backend && go run ./cmd/submission-manager-openapi -out ../specs/submission-tracking/submission-manager-openapi.yaml)")
	}
}

func TestSubmissionManagerOpenAPIContractCoverage(t *testing.T) {
	doc := loadSubmissionManagerOpenAPI(t)

	if !strings.HasPrefix(doc.OpenAPI, "3.0.") {
		t.Fatalf("expected OpenAPI 3.0.x, got %q", doc.OpenAPI)
	}

	expectedOps := map[string]map[string]bool{
		"/healthz":                       {"get": true},
		"/readyz":                        {"get": true},
		"/metrics":                       {"get": true},
		submissionmanagerapi.OpenAPIPath: {"get": true},
		submissionmanagerapi.DocsPath:    {"get": true},
		"/v1/intents":                    {"post": true},
		"/v1/intents/{intentId}":         {"get": true},
		"/v1/intents/{intentId}/history": {"get": true},
		"/ui/history":                    {"post": true},
	}
	assertExactOperationSet(t, doc, expectedOps)

	nonOwned := []string{
		"/v1/delivery/provider-signal-webhook",
		"/v1/intents/{intentId}/delivery",
		"/v1/intents/{intentId}/delivery/history",
	}
	for _, path := range nonOwned {
		if _, exists := doc.Paths[path]; exists {
			t.Fatalf("non-owned path must not be in submission-manager OpenAPI: %s", path)
		}
	}

	requiredSchemas := []string{
		"SubmitIntentRequest",
		"IntentResponse",
		"AttemptResponse",
		"IntentHistoryResponse",
		"ErrorBody",
		"ErrorResponse",
	}
	for _, name := range requiredSchemas {
		if _, ok := doc.Components.Schemas[name]; !ok {
			t.Fatalf("missing component schema %q", name)
		}
	}

	postIntent := doc.Paths["/v1/intents"]["post"]
	waitParam, ok := findParameter(postIntent.Parameters, "waitSeconds", "query")
	if !ok {
		t.Fatal("missing waitSeconds query parameter on POST /v1/intents")
	}
	if waitParam.Required {
		t.Fatal("waitSeconds must be optional")
	}
	if got, ok := waitParam.Schema["type"].(string); !ok || got != "integer" {
		t.Fatalf("waitSeconds type must be integer, got %#v", waitParam.Schema["type"])
	}
	if got, ok := waitParam.Schema["minimum"].(float64); !ok || got != 0 {
		t.Fatalf("waitSeconds minimum must be 0, got %#v", waitParam.Schema["minimum"])
	}

	if postIntent.RequestBody == nil || !postIntent.RequestBody.Required {
		t.Fatal("POST /v1/intents must define a required requestBody")
	}
	if _, ok := postIntent.RequestBody.Content["application/json"]; !ok {
		t.Fatal("POST /v1/intents must accept application/json")
	}

	submitSchema := doc.Components.Schemas["SubmitIntentRequest"]
	required, ok := submitSchema["required"].([]any)
	if !ok {
		t.Fatalf("SubmitIntentRequest.required must be an array, got %#v", submitSchema["required"])
	}
	if !containsString(required, "intentId") || !containsString(required, "submissionTarget") {
		t.Fatalf("SubmitIntentRequest.required must contain intentId and submissionTarget, got %#v", required)
	}

	assertResponseMediaType(t, doc, "/healthz", "get", "200", "text/plain")
	assertResponseMediaType(t, doc, "/readyz", "get", "200", "text/plain")
	assertResponseMediaType(t, doc, "/metrics", "get", "200", "text/plain")
	assertResponseNoContent(t, doc, "/metrics", "get", "404")
	assertResponseMediaType(t, doc, submissionmanagerapi.OpenAPIPath, "get", "200", "application/json")
	assertResponseMediaType(t, doc, submissionmanagerapi.DocsPath, "get", "200", "text/html")
	assertResponseMediaType(t, doc, "/v1/intents", "post", "200", "application/json")
	assertResponseMediaType(t, doc, "/v1/intents/{intentId}", "get", "200", "application/json")
	assertResponseMediaType(t, doc, "/v1/intents/{intentId}/history", "get", "200", "application/json")
	assertResponseMediaType(t, doc, "/ui/history", "post", "200", "text/html")
}

func TestSubmissionManagerOpenAPIRuntimeDriftCheck(t *testing.T) {
	doc := loadSubmissionManagerOpenAPI(t)
	server := &apiServer{}

	muxNoUI := newMux(server, nil, nil, nil)
	muxWithMetrics := newMux(server, nil, submissionmanager.NewMetrics(), nil)
	muxWithUI := newMux(server, newTestUIServer(), nil, nil)

	type scenario struct {
		name           string
		mux            *http.ServeMux
		method         string
		path           string
		body           string
		contentType    string
		status         int
		mediaType      string
		contractPath   string
		contractMethod string
	}

	scenarios := []scenario{
		{
			name:           "healthz get ok",
			mux:            muxNoUI,
			method:         http.MethodGet,
			path:           "/healthz",
			status:         http.StatusOK,
			mediaType:      "text/plain",
			contractPath:   "/healthz",
			contractMethod: "get",
		},
		{
			name:           "healthz method not allowed",
			mux:            muxNoUI,
			method:         http.MethodPost,
			path:           "/healthz",
			status:         http.StatusMethodNotAllowed,
			mediaType:      "text/plain",
			contractPath:   "/healthz",
			contractMethod: "get",
		},
		{
			name:           "readyz get ok",
			mux:            muxNoUI,
			method:         http.MethodGet,
			path:           "/readyz",
			status:         http.StatusOK,
			mediaType:      "text/plain",
			contractPath:   "/readyz",
			contractMethod: "get",
		},
		{
			name:           "readyz method not allowed",
			mux:            muxNoUI,
			method:         http.MethodPost,
			path:           "/readyz",
			status:         http.StatusMethodNotAllowed,
			mediaType:      "text/plain",
			contractPath:   "/readyz",
			contractMethod: "get",
		},
		{
			name:           "metrics get configured",
			mux:            muxWithMetrics,
			method:         http.MethodGet,
			path:           "/metrics",
			status:         http.StatusOK,
			mediaType:      "text/plain",
			contractPath:   "/metrics",
			contractMethod: "get",
		},
		{
			name:           "metrics get not configured",
			mux:            muxNoUI,
			method:         http.MethodGet,
			path:           "/metrics",
			status:         http.StatusNotFound,
			mediaType:      "",
			contractPath:   "/metrics",
			contractMethod: "get",
		},
		{
			name:           "metrics method not allowed",
			mux:            muxWithMetrics,
			method:         http.MethodPost,
			path:           "/metrics",
			status:         http.StatusMethodNotAllowed,
			mediaType:      "text/plain",
			contractPath:   "/metrics",
			contractMethod: "get",
		},
		{
			name:           "openapi get",
			mux:            muxNoUI,
			method:         http.MethodGet,
			path:           submissionmanagerapi.OpenAPIPath,
			status:         http.StatusOK,
			mediaType:      "application/json",
			contractPath:   submissionmanagerapi.OpenAPIPath,
			contractMethod: "get",
		},
		{
			name:           "openapi method not allowed",
			mux:            muxNoUI,
			method:         http.MethodPost,
			path:           submissionmanagerapi.OpenAPIPath,
			status:         http.StatusMethodNotAllowed,
			mediaType:      "text/plain",
			contractPath:   submissionmanagerapi.OpenAPIPath,
			contractMethod: "get",
		},
		{
			name:           "docs get",
			mux:            muxNoUI,
			method:         http.MethodGet,
			path:           submissionmanagerapi.DocsPath,
			status:         http.StatusOK,
			mediaType:      "text/html",
			contractPath:   submissionmanagerapi.DocsPath,
			contractMethod: "get",
		},
		{
			name:           "docs method not allowed",
			mux:            muxNoUI,
			method:         http.MethodPost,
			path:           submissionmanagerapi.DocsPath,
			status:         http.StatusMethodNotAllowed,
			mediaType:      "text/plain",
			contractPath:   submissionmanagerapi.DocsPath,
			contractMethod: "get",
		},
		{
			name:           "submit invalid json",
			mux:            muxNoUI,
			method:         http.MethodPost,
			path:           "/v1/intents",
			body:           "{",
			contentType:    "application/json",
			status:         http.StatusBadRequest,
			mediaType:      "application/json",
			contractPath:   "/v1/intents",
			contractMethod: "post",
		},
		{
			name:           "submit method not allowed",
			mux:            muxNoUI,
			method:         http.MethodGet,
			path:           "/v1/intents",
			status:         http.StatusMethodNotAllowed,
			mediaType:      "application/json",
			contractPath:   "/v1/intents",
			contractMethod: "post",
		},
		{
			name:           "get intent method not allowed",
			mux:            muxNoUI,
			method:         http.MethodPost,
			path:           "/v1/intents/intent-1",
			status:         http.StatusMethodNotAllowed,
			mediaType:      "application/json",
			contractPath:   "/v1/intents/{intentId}",
			contractMethod: "get",
		},
		{
			name:      "delivery route not owned",
			mux:       muxNoUI,
			method:    http.MethodGet,
			path:      "/v1/intents/intent-1/delivery",
			status:    http.StatusNotFound,
			mediaType: "application/json",
		},
		{
			name:      "delivery history route not owned",
			mux:       muxNoUI,
			method:    http.MethodGet,
			path:      "/v1/intents/intent-1/delivery/history",
			status:    http.StatusNotFound,
			mediaType: "application/json",
		},
		{
			name:      "ui history disabled route",
			mux:       muxNoUI,
			method:    http.MethodPost,
			path:      "/ui/history",
			body:      "intentId=intent-1",
			status:    http.StatusNotFound,
			mediaType: "text/plain",
		},
		{
			name:           "ui history enabled method not allowed",
			mux:            muxWithUI,
			method:         http.MethodGet,
			path:           "/ui/history",
			status:         http.StatusMethodNotAllowed,
			mediaType:      "text/plain",
			contractPath:   "/ui/history",
			contractMethod: "post",
		},
		{
			name:           "ui history enabled missing form value",
			mux:            muxWithUI,
			method:         http.MethodPost,
			path:           "/ui/history",
			body:           "",
			contentType:    "application/x-www-form-urlencoded",
			status:         http.StatusBadRequest,
			mediaType:      "text/plain",
			contractPath:   "/ui/history",
			contractMethod: "post",
		},
	}

	for _, tc := range scenarios {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			rr := httptest.NewRecorder()
			tc.mux.ServeHTTP(rr, req)

			if rr.Code != tc.status {
				t.Fatalf("expected status=%d, got %d body=%q", tc.status, rr.Code, rr.Body.String())
			}
			gotMedia := normalizeMediaType(rr.Header().Get("Content-Type"))
			if gotMedia != tc.mediaType {
				t.Fatalf("expected media type %q, got %q", tc.mediaType, gotMedia)
			}

			if tc.contractPath == "" {
				return
			}
			assertResponseDeclared(t, doc, tc.contractPath, tc.contractMethod, fmt.Sprintf("%d", tc.status), tc.mediaType)
		})
	}

	for _, path := range []string{
		"/v1/delivery/provider-signal-webhook",
		"/v1/intents/{intentId}/delivery",
		"/v1/intents/{intentId}/delivery/history",
	} {
		if _, ok := doc.Paths[path]; ok {
			t.Fatalf("non-owned delivery path must not be declared in OpenAPI: %s", path)
		}
	}
}

func openAPIArtifactPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(moduleRoot(t), "..", "specs", "submission-tracking", "submission-manager-openapi.yaml")
}

func loadSubmissionManagerOpenAPI(t *testing.T) submissionmanagerapi.Document {
	t.Helper()
	path := openAPIArtifactPath(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read OpenAPI artifact %s: %v", path, err)
	}
	var doc submissionmanagerapi.Document
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse OpenAPI artifact %s: %v", path, err)
	}
	return doc
}

func assertExactOperationSet(t *testing.T, doc submissionmanagerapi.Document, expected map[string]map[string]bool) {
	t.Helper()
	if len(doc.Paths) != len(expected) {
		t.Fatalf("unexpected path count: got=%d expected=%d paths=%v", len(doc.Paths), len(expected), sortedPathKeys(doc.Paths))
	}
	for path, methods := range expected {
		ops, ok := doc.Paths[path]
		if !ok {
			t.Fatalf("missing path %s", path)
		}
		if len(ops) != len(methods) {
			t.Fatalf("unexpected method count for %s: got=%v expected=%v", path, sortedMethodKeys(ops), sortedBoolKeys(methods))
		}
		for method := range methods {
			if _, ok := ops[method]; !ok {
				t.Fatalf("missing method %s %s", strings.ToUpper(method), path)
			}
		}
		for method := range ops {
			if !methods[method] {
				t.Fatalf("unexpected method %s on path %s", strings.ToUpper(method), path)
			}
		}
	}
}

func assertResponseDeclared(t *testing.T, doc submissionmanagerapi.Document, path, method, status, mediaType string) {
	t.Helper()
	ops, ok := doc.Paths[path]
	if !ok {
		t.Fatalf("OpenAPI missing path %s", path)
	}
	op, ok := ops[method]
	if !ok {
		t.Fatalf("OpenAPI missing method %s %s", strings.ToUpper(method), path)
	}
	reply, ok := op.Responses[status]
	if !ok {
		t.Fatalf("OpenAPI missing response status %s for %s %s", status, strings.ToUpper(method), path)
	}
	if mediaType == "" {
		if len(reply.Content) != 0 {
			t.Fatalf("expected no response content for %s %s %s, got %v", strings.ToUpper(method), path, status, sortedMediaKeys(reply.Content))
		}
		return
	}
	if _, ok := reply.Content[mediaType]; !ok {
		t.Fatalf("expected media type %s for %s %s %s, got %v", mediaType, strings.ToUpper(method), path, status, sortedMediaKeys(reply.Content))
	}
}

func assertResponseMediaType(t *testing.T, doc submissionmanagerapi.Document, path, method, status, mediaType string) {
	t.Helper()
	assertResponseDeclared(t, doc, path, method, status, mediaType)
}

func assertResponseNoContent(t *testing.T, doc submissionmanagerapi.Document, path, method, status string) {
	t.Helper()
	assertResponseDeclared(t, doc, path, method, status, "")
}

func findParameter(params []submissionmanagerapi.Parameter, name, location string) (submissionmanagerapi.Parameter, bool) {
	for _, param := range params {
		if param.Name == name && param.In == location {
			return param, true
		}
	}
	return submissionmanagerapi.Parameter{}, false
}

func containsString(values []any, target string) bool {
	for _, value := range values {
		text, ok := value.(string)
		if ok && text == target {
			return true
		}
	}
	return false
}

func normalizeMediaType(contentType string) string {
	base, _, _ := strings.Cut(contentType, ";")
	return strings.TrimSpace(base)
}

func sortedPathKeys(paths map[string]map[string]submissionmanagerapi.Operation) []string {
	out := make([]string, 0, len(paths))
	for key := range paths {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func sortedMethodKeys(methods map[string]submissionmanagerapi.Operation) []string {
	out := make([]string, 0, len(methods))
	for key := range methods {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func sortedBoolKeys(methods map[string]bool) []string {
	out := make([]string, 0, len(methods))
	for key := range methods {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func sortedMediaKeys(media map[string]submissionmanagerapi.Media) []string {
	out := make([]string, 0, len(media))
	for key := range media {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
