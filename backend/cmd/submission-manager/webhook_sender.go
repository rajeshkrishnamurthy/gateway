package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"gateway/submissionmanager"
)

func newWebhookSender(client *http.Client) submissionmanager.WebhookSender {
	if client == nil {
		client = http.DefaultClient
	}
	return func(ctx context.Context, delivery submissionmanager.WebhookDelivery) error {
		urlValue := strings.TrimSpace(delivery.URL)
		if urlValue == "" {
			return fmt.Errorf("webhook url is required")
		}
		body := delivery.Body
		if body == nil {
			body = []byte{}
		}
		headers := http.Header{}
		for key, value := range delivery.Headers {
			headers.Set(key, value)
		}
		for headerName, envKey := range delivery.HeadersEnv {
			envValue := strings.TrimSpace(os.Getenv(envKey))
			if envValue == "" {
				return fmt.Errorf("webhook env %q is required for header %q", envKey, headerName)
			}
			headers.Set(headerName, envValue)
		}
		if secretEnv := strings.TrimSpace(delivery.SecretEnv); secretEnv != "" {
			secret := strings.TrimSpace(os.Getenv(secretEnv))
			if secret == "" {
				return fmt.Errorf("webhook secret env %q is required", secretEnv)
			}
			mac := hmac.New(sha256.New, []byte(secret))
			_, _ = mac.Write(body)
			signature := hex.EncodeToString(mac.Sum(nil))
			headers.Set("X-Setu-Signature", signature)
		}
		if headers.Get("Content-Type") == "" {
			headers.Set("Content-Type", "application/json")
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlValue, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header = headers

		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return fmt.Errorf("webhook returned status %d", resp.StatusCode)
		}
		return nil
	}
}
