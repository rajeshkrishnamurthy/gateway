package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

type intentResponse struct {
	IntentID         string `json:"intentId"`
	SubmissionTarget string `json:"submissionTarget"`
	CreatedAt        string `json:"createdAt"`
	Status           string `json:"status"`
	CompletedAt      string `json:"completedAt,omitempty"`
	RejectedReason   string `json:"rejectedReason,omitempty"`
	ExhaustedReason  string `json:"exhaustedReason,omitempty"`
}

type historyResponse struct {
	Intent   intentResponse    `json:"intent"`
	Attempts []attemptResponse `json:"attempts"`
}

type attemptResponse struct {
	AttemptNumber int    `json:"attemptNumber"`
	OutcomeStatus string `json:"outcomeStatus"`
	OutcomeReason string `json:"outcomeReason"`
	Error         string `json:"error"`
}

type webhookLast struct {
	ReceivedAt string       `json:"receivedAt"`
	Event      webhookEvent `json:"event"`
}

type webhookEvent struct {
	EventID   string        `json:"eventId"`
	EventType string        `json:"eventType"`
	Intent    webhookIntent `json:"intent"`
}

type webhookIntent struct {
	IntentID         string `json:"intentId"`
	SubmissionTarget string `json:"submissionTarget"`
	Status           string `json:"status"`
	RejectedReason   string `json:"rejectedReason,omitempty"`
	ExhaustedReason  string `json:"exhaustedReason,omitempty"`
}

func main() {
	managerURL := flag.String("manager", "http://localhost:8082", "SubmissionManager base URL")
	adminURL := flag.String("admin", "http://localhost:8090", "Admin portal base URL")
	webhookURL := flag.String("webhook", "http://localhost:9999", "Webhook sink base URL")
	smsGatewayURL := flag.String("smsGateway", "http://localhost:18080", "SMS gateway base URL")
	pushGatewayURL := flag.String("pushGateway", "http://localhost:19080", "Push gateway base URL")
	runRobust := flag.Bool("robust", false, "run robustness scenarios (restart + exhaustion)")
	runUI := flag.Bool("ui", false, "run admin portal send/troubleshoot checks")
	flag.Parse()

	client := &http.Client{Timeout: 20 * time.Second}
	ctx := context.Background()

	if *runRobust {
		must(waitReady(client, *managerURL+"/readyz", 60*time.Second), "submission-manager ready")
		must(waitReady(client, *adminURL+"/readyz", 60*time.Second), "admin-portal ready")
		must(waitReady(client, *webhookURL+"/readyz", 60*time.Second), "webhook-sink ready")
		must(waitReady(client, *smsGatewayURL+"/readyz", 60*time.Second), "sms-gateway ready")
		must(waitReady(client, *pushGatewayURL+"/readyz", 60*time.Second), "push-gateway ready")
	} else {
		runScenario("Readiness", func() {
			must(waitReady(client, *managerURL+"/readyz", 60*time.Second), "submission-manager ready")
			must(waitReady(client, *adminURL+"/readyz", 60*time.Second), "admin-portal ready")
			must(waitReady(client, *webhookURL+"/readyz", 60*time.Second), "webhook-sink ready")
			must(waitReady(client, *smsGatewayURL+"/readyz", 60*time.Second), "sms-gateway ready")
			must(waitReady(client, *pushGatewayURL+"/readyz", 60*time.Second), "push-gateway ready")
		})
	}

	if *runRobust {
		runScenario("SMS exhausted + webhook + attempts", func() {
			exhaustedID := "sms-exhaust-" + stamp()
			resp := submitIntent(ctx, client, *managerURL, exhaustedID, "sms.realtime", map[string]string{
				"referenceId": exhaustedID,
				"to":          "+15551234567",
				"message":     "FAIL",
			}, 1)
			require(resp.Status == "pending" || resp.Status == "rejected" || resp.Status == "exhausted", "exhausted submit response")
			resp = waitForIntent(client, *managerURL, exhaustedID, "exhausted", 45*time.Second)
			require(resp.ExhaustedReason == "deadline_exceeded", "exhausted reason")
			waitWebhook(client, *webhookURL, exhaustedID, "exhausted", 20*time.Second)
			history := fetchHistory(client, *managerURL, exhaustedID)
			require(len(history.Attempts) >= 2, "exhausted attempts >= 2")
		})

		runScenario("Restart recovery", func() {
			recoveryID := "sms-restart-" + stamp()
			resp := submitIntent(ctx, client, *managerURL, recoveryID, "sms.realtime", map[string]string{
				"referenceId": recoveryID,
				"to":          "+15551234567",
				"message":     "FAIL",
			}, 1)
			require(resp.Status == "pending" || resp.Status == "rejected" || resp.Status == "exhausted", "restart submit response")
			history := waitForAttempts(client, *managerURL, recoveryID, 1, 20*time.Second)
			must(restartSubmissionManagers(), "restart submission-managers")
			must(waitReady(client, *managerURL+"/readyz", 60*time.Second), "submission-manager ready after restart")
			history = waitForAttempts(client, *managerURL, recoveryID, 2, 30*time.Second)
			require(len(history.Attempts) >= 2, "restart attempts >= 2")
		})

		fmt.Println("compose integration: all checks passed")
		return
	}

	var smsAcceptedID string
	runScenario("SMS accepted + webhook", func() {
		smsAcceptedID = "sms-accepted-" + stamp()
		resp := submitIntent(ctx, client, *managerURL, smsAcceptedID, "sms.realtime", map[string]string{
			"referenceId": smsAcceptedID,
			"to":          "+15551234567",
			"message":     "hello",
		}, 5)
		require(resp.Status == "accepted", "sms accepted status")
		waitWebhook(client, *webhookURL, smsAcceptedID, "accepted", 20*time.Second)
	})

	runScenario("SMS rejected + webhook", func() {
		smsRejectedID := "sms-rejected-" + stamp()
		resp := submitIntent(ctx, client, *managerURL, smsRejectedID, "sms.realtime", map[string]string{
			"referenceId": smsRejectedID,
			"to":          "abc",
			"message":     "hello",
		}, 5)
		require(resp.Status == "rejected", "sms rejected status")
		require(resp.RejectedReason == "invalid_recipient", "sms rejected reason")
		waitWebhook(client, *webhookURL, smsRejectedID, "rejected", 20*time.Second)
	})

	runScenario("Idempotency conflict", func() {
		idempotentID := "sms-idem-" + stamp()
		first := submitIntent(ctx, client, *managerURL, idempotentID, "sms.realtime", map[string]string{
			"referenceId": idempotentID,
			"to":          "+15551234567",
			"message":     "hello",
		}, 0)
		second := submitIntent(ctx, client, *managerURL, idempotentID, "sms.realtime", map[string]string{
			"referenceId": idempotentID,
			"to":          "+15551234567",
			"message":     "hello",
		}, 0)
		require(first.IntentID == second.IntentID, "idempotent reuse")
		statusCode := submitIntentRaw(ctx, client, *managerURL, idempotentID, "sms.realtime", map[string]string{
			"referenceId": idempotentID,
			"to":          "+15551234567",
			"message":     "hello again",
		}, 0)
		require(statusCode == http.StatusConflict, "idempotency conflict")
	})

	runScenario("Sync waitSeconds", func() {
		waitID := "sms-wait-" + stamp()
		resp := submitIntent(ctx, client, *managerURL, waitID, "sms.realtime", map[string]string{
			"referenceId": waitID,
			"to":          "+15551234567",
			"message":     "hello",
		}, 5)
		require(resp.Status != "pending", "waitSeconds returns terminal")
	})

	runScenario("Intent history", func() {
		_ = fetchIntent(client, *managerURL, smsAcceptedID)
		history := fetchHistory(client, *managerURL, smsAcceptedID)
		require(len(history.Attempts) >= 1, "history attempts >= 1")
	})

	if *runUI {
		runScenario("Admin portal send + troubleshoot", func() {
			adminSMSID := "admin-sms-" + stamp()
			adminResp := submitAdminSMS(ctx, client, *adminURL, adminSMSID)
			require(adminResp.IntentID == adminSMSID, "admin portal sms intent")
			adminPushID := "admin-push-" + stamp()
			adminResp = submitAdminPush(ctx, client, *adminURL, adminPushID)
			require(adminResp.IntentID == adminPushID, "admin portal push intent")

			must(checkContains(client, *adminURL+"/troubleshoot", "Intent history"), "troubleshoot page")
			must(checkTroubleshootHistory(client, *adminURL, smsAcceptedID), "troubleshoot history")
		})
	}

	runScenario("Health endpoints", func() {
		must(waitReady(client, *managerURL+"/healthz", 10*time.Second), "submission-manager healthz")
		must(waitReady(client, *adminURL+"/healthz", 10*time.Second), "admin-portal healthz")
		must(waitReady(client, *webhookURL+"/healthz", 10*time.Second), "webhook-sink healthz")
		must(waitReady(client, *smsGatewayURL+"/healthz", 10*time.Second), "sms-gateway healthz")
		must(waitReady(client, *pushGatewayURL+"/healthz", 10*time.Second), "push-gateway healthz")
	})

	fmt.Println("compose integration: all checks passed")
}

func waitReady(client *http.Client, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", url)
}

func submitIntent(ctx context.Context, client *http.Client, baseURL, intentID, target string, payload map[string]string, waitSeconds int) intentResponse {
	status, body := submitIntentBody(ctx, client, baseURL, intentID, target, payload, waitSeconds)
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		panic(fmt.Sprintf("submit intent failed status=%d body=%s", status, string(body)))
	}
	var resp intentResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		panic(fmt.Sprintf("decode intent response: %v body=%s", err, string(body)))
	}
	return resp
}

func submitIntentRaw(ctx context.Context, client *http.Client, baseURL, intentID, target string, payload map[string]string, waitSeconds int) int {
	status, _ := submitIntentBody(ctx, client, baseURL, intentID, target, payload, waitSeconds)
	return status
}

func submitIntentBody(ctx context.Context, client *http.Client, baseURL, intentID, target string, payload map[string]string, waitSeconds int) (int, []byte) {
	body, _ := json.Marshal(map[string]interface{}{
		"intentId":         intentID,
		"submissionTarget": target,
		"payload":          payload,
	})
	url := strings.TrimRight(baseURL, "/") + "/v1/intents"
	if waitSeconds > 0 {
		url += fmt.Sprintf("?waitSeconds=%d", waitSeconds)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody
}

func fetchIntent(client *http.Client, baseURL, intentID string) intentResponse {
	resp, err := fetchIntentMaybe(client, baseURL, intentID)
	if err != nil {
		panic(err)
	}
	return resp
}

func fetchIntentMaybe(client *http.Client, baseURL, intentID string) (intentResponse, error) {
	url := strings.TrimRight(baseURL, "/") + "/v1/intents/" + intentID
	resp, err := client.Get(url)
	if err != nil {
		return intentResponse{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return intentResponse{}, fmt.Errorf("fetch intent failed status=%d body=%s", resp.StatusCode, string(body))
	}
	var out intentResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return intentResponse{}, err
	}
	return out, nil
}

func fetchHistory(client *http.Client, baseURL, intentID string) historyResponse {
	resp, err := fetchHistoryMaybe(client, baseURL, intentID)
	if err != nil {
		panic(err)
	}
	return resp
}

func fetchHistoryMaybe(client *http.Client, baseURL, intentID string) (historyResponse, error) {
	url := strings.TrimRight(baseURL, "/") + "/v1/intents/" + intentID + "/history"
	resp, err := client.Get(url)
	if err != nil {
		return historyResponse{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return historyResponse{}, fmt.Errorf("fetch history failed status=%d body=%s", resp.StatusCode, string(body))
	}
	var out historyResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return historyResponse{}, err
	}
	return out, nil
}

func waitForIntent(client *http.Client, baseURL, intentID, status string, timeout time.Duration) intentResponse {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := fetchIntentMaybe(client, baseURL, intentID)
		if err == nil {
			if resp.Status == status {
				return resp
			}
		}
		time.Sleep(1 * time.Second)
	}
	panic(fmt.Sprintf("timeout waiting for status %q", status))
}

func waitForAttempts(client *http.Client, baseURL, intentID string, count int, timeout time.Duration) historyResponse {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := fetchHistoryMaybe(client, baseURL, intentID)
		if err == nil {
			if len(resp.Attempts) >= count {
				return resp
			}
		}
		time.Sleep(1 * time.Second)
	}
	panic(fmt.Sprintf("timeout waiting for %d attempts", count))
}

func waitWebhook(client *http.Client, baseURL, intentID, status string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	url := strings.TrimRight(baseURL, "/") + "/last"
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var last webhookLast
				if err := json.Unmarshal(body, &last); err == nil {
					if last.Event.EventID == intentID && last.Event.Intent.Status == status {
						return
					}
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	panic(fmt.Sprintf("timeout waiting for webhook intentId=%s status=%s", intentID, status))
}

func submitAdminSMS(ctx context.Context, client *http.Client, baseURL, intentID string) intentResponse {
	body, _ := json.Marshal(map[string]string{
		"referenceId": intentID,
		"to":          "+15551234567",
		"message":     "hello",
	})
	url := strings.TrimRight(baseURL, "/") + "/sms/send"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		panic(fmt.Sprintf("admin sms failed status=%d body=%s", resp.StatusCode, string(respBody)))
	}
	var out intentResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		panic(err)
	}
	return out
}

func submitAdminPush(ctx context.Context, client *http.Client, baseURL, intentID string) intentResponse {
	body, _ := json.Marshal(map[string]string{
		"referenceId": intentID,
		"token":       "test-token",
		"title":       "hello",
		"body":        "there",
	})
	url := strings.TrimRight(baseURL, "/") + "/push/send"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		panic(fmt.Sprintf("admin push failed status=%d body=%s", resp.StatusCode, string(respBody)))
	}
	var out intentResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		panic(err)
	}
	return out
}

func checkContains(client *http.Client, url, needle string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), needle) {
		return fmt.Errorf("expected %q", needle)
	}
	return nil
}

func checkTroubleshootHistory(client *http.Client, adminURL, intentID string) error {
	form := "intentId=" + intentID
	req, _ := http.NewRequest(http.MethodPost, strings.TrimRight(adminURL, "/")+"/troubleshoot/history", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "Intent summary") {
		return fmt.Errorf("missing intent summary")
	}
	if !strings.Contains(string(body), intentID) {
		return fmt.Errorf("missing intentId")
	}
	return nil
}

func restartSubmissionManagers() error {
	cmd := exec.Command("docker", "compose", "restart", "submission-manager-1", "submission-manager-2")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("restart submission-managers: %v output=%s", err, string(out))
	}
	return nil
}

func must(err error, label string) {
	if err != nil {
		panic(fmt.Sprintf("%s failed: %v", label, err))
	}
}

func require(ok bool, label string) {
	if !ok {
		panic(fmt.Sprintf("check failed: %s", label))
	}
}

func stamp() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func runScenario(name string, fn func()) {
	fmt.Printf("Scenario: %s\n", name)
	defer func() {
		if r := recover(); r != nil {
			panic(fmt.Sprintf("scenario %s failed: %v", name, r))
		}
	}()
	fn()
	fmt.Println("PASSED")
	fmt.Println()
}
