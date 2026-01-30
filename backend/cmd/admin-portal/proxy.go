package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

func (s *portalServer) proxyUI(w http.ResponseWriter, r *http.Request, baseURL, prefix, active string, embed bool) {
	if baseURL == "" {
		s.renderError(w, r, http.StatusNotFound, "Console not configured", "The upstream URL is not set in the portal config.", active)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		s.renderError(w, r, http.StatusMethodNotAllowed, "Method not allowed", "method not allowed", active)
		return
	}

	remotePath := strings.TrimPrefix(r.URL.Path, prefix)
	if remotePath == "" {
		remotePath = "/"
	}
	if !strings.HasPrefix(remotePath, "/") {
		remotePath = "/" + remotePath
	}

	remoteURL, err := buildTargetURL(baseURL, remotePath, r.URL.RawQuery, embed)
	if err != nil {
		s.renderError(w, r, http.StatusBadGateway, "Invalid upstream URL", err.Error(), active)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, remoteURL, r.Body)
	if err != nil {
		s.renderError(w, r, http.StatusBadGateway, "Upstream request failed", err.Error(), active)
		return
	}
	copyHeader(req.Header, r.Header, []string{"Content-Type", "Accept"})
	req.Header.Set("HX-Request", "true")

	resp, err := s.client.Do(req)
	if err != nil {
		s.renderError(w, r, http.StatusBadGateway, "Upstream request failed", err.Error(), active)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.renderError(w, r, http.StatusBadGateway, "Upstream response failed", err.Error(), active)
		return
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "text/html") {
		body = rewriteUIPaths(body, prefix)
		if prefix == "/sms" && s.useSubmissionManagerSMS() {
			body = rewriteSubmissionCopy(body, "/sms/send")
		}
		if prefix == "/push" && s.useSubmissionManagerPush() {
			body = rewriteSubmissionCopy(body, "/push/send")
		}
		if prefix == "/sms" {
			body = bytes.ReplaceAll(body, []byte("Troubleshoot by ReferenceId"), []byte("Troubleshoot"))
		}
		if embed {
			body = stripThemeToggle(body)
		}
		if !isHTMX(r) {
			s.renderShell(w, body, active, resp.StatusCode)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(resp.StatusCode)
		if _, err := w.Write(body); err != nil {
			log.Printf("write proxy fragment: %v", err)
		}
		return
	}

	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(body); err != nil {
		log.Printf("write proxy response: %v", err)
	}
}

func (s *portalServer) proxyAPI(w http.ResponseWriter, r *http.Request, baseURL string) {
	if baseURL == "" {
		http.Error(w, "upstream not configured", http.StatusNotFound)
		return
	}
	remoteURL, err := buildTargetURL(baseURL, r.URL.Path, r.URL.RawQuery, false)
	if err != nil {
		http.Error(w, "invalid upstream URL", http.StatusBadGateway)
		return
	}
	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, remoteURL, r.Body)
	if err != nil {
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	copyHeader(proxyReq.Header, r.Header, []string{"Content-Type", "Accept", "HX-Request"})

	resp, err := s.client.Do(proxyReq)
	if err != nil {
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "" {
		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("write proxy api: %v", err)
	}
}

func buildTargetURL(base, path, rawQuery string, embed bool) (string, error) {
	parsedBase, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	ref := &url.URL{Path: path, RawQuery: rawQuery}
	resolved := parsedBase.ResolveReference(ref)
	if embed {
		query := resolved.Query()
		query.Set("embed", "1")
		resolved.RawQuery = query.Encode()
	}
	return resolved.String(), nil
}

func copyHeader(dst, src http.Header, keys []string) {
	for _, key := range keys {
		if value := src.Get(key); value != "" {
			dst.Set(key, value)
		}
	}
}

func isHTMX(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("HX-Request"), "true")
}

func rewriteUIPaths(input []byte, prefix string) []byte {
	if prefix == "" {
		return input
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	output := string(input)
	output = strings.ReplaceAll(output, "=\"/ui", "=\""+prefix+"/ui")
	output = strings.ReplaceAll(output, "='/ui", "='"+prefix+"/ui")
	return []byte(output)
}

func rewriteSubmissionCopy(input []byte, sendEndpoint string) []byte {
	text := string(input)
	manual := fmt.Sprintf("Manual submission to the gateway. This mirrors POST %s and returns the raw response.", sendEndpoint)
	text = strings.ReplaceAll(text, manual, "Manual submission via SubmissionManager. This creates an intent and shows the current status.")
	text = strings.ReplaceAll(text, "No retry, no send again, no history. Use referenceId values you can trace in logs.", "SubmissionManager owns retries and history. Use intentId values you can trace in logs.")
	text = strings.ReplaceAll(text, "<h2>Gateway response</h2>", "<h2>Submission response</h2>")
	text = strings.ReplaceAll(text, "Submit a request to see status, reason, and gatewayMessageId.", "Submit a request to see the current intent status.")
	text = strings.ReplaceAll(text, "Accepted means submitted, not delivered. This console does not infer delivery or retries.", "Accepted means submitted, not delivered. SubmissionManager does not infer delivery.")
	text = strings.ReplaceAll(text, `<label for="referenceId">referenceId</label>`, `<label for="referenceId">intentId</label>`)
	return []byte(text)
}

func stripThemeToggle(input []byte) []byte {
	text := string(input)
	needle := "id=\"theme-toggle\""
	for {
		idx := strings.Index(text, needle)
		if idx == -1 {
			break
		}
		start := strings.LastIndex(text[:idx], "<button")
		if start == -1 {
			break
		}
		end := strings.Index(text[idx:], "</button>")
		if end == -1 {
			break
		}
		end = idx + end + len("</button>")
		text = text[:start] + text[end:]
	}
	return []byte(text)
}
