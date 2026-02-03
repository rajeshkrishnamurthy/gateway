package main

import (
	"net/http"
	"time"
)

const statusHTTPTimeout = 2 * time.Second

func isHealthUp(healthURL string) bool {
	client := &http.Client{Timeout: statusHTTPTimeout}
	req, err := http.NewRequest(http.MethodGet, healthURL, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}
