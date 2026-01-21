package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"gateway"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const maxBodyBytes = 16 << 10

var addr = flag.String("addr", ":8080", "HTTP listen address")
var smsProviderURL = flag.String("sms-provider-url", "", "SMS provider URL")

func main() {
	flag.Parse()

	gw, err := gateway.New(gateway.Config{Provider: newProviderClient(*smsProviderURL)})
	if err != nil {
		log.Fatal(err)
	}

	server := &http.Server{
		Addr:    *addr,
		Handler: newMux(gw),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	log.Printf("listening on %s", *addr)

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
		return
	case sig := <-sigCh:
		log.Printf("shutdown signal: %s", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Allow in-flight requests to finish before exit.
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}

	err = <-errCh
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("server error: %v", err)
	}
}

func newMux(gw *gateway.SMSGateway) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/sms/send", handleSMSSend(gw))
	return mux
}

func handleSMSSend(gw *gateway.SMSGateway) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

		dec := json.NewDecoder(r.Body)
		var req gateway.SMSRequest
		if err := dec.Decode(&req); err != nil {
			writeSMSResponse(w, http.StatusBadRequest, gateway.SMSResponse{
				Status: "rejected",
				Reason: "invalid_request",
			})
			return
		}
		if err := dec.Decode(&struct{}{}); err != io.EOF {
			writeSMSResponse(w, http.StatusBadRequest, gateway.SMSResponse{
				Status: "rejected",
				Reason: "invalid_request",
			})
			return
		}

		resp, err := gw.SendSMS(r.Context(), req)
		status := http.StatusOK
		if err != nil && errors.Is(err, gateway.ErrInvalidRequest) {
			status = http.StatusBadRequest
		}
		writeSMSResponse(w, status, resp)
	}
}

func writeSMSResponse(w http.ResponseWriter, status int, resp gateway.SMSResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("encode response: %v", err)
	}
}

type providerRequest struct {
	ReferenceID string `json:"referenceId"`
	To          string `json:"to"`
	Message     string `json:"message"`
	TenantID    string `json:"tenantId,omitempty"`
}

type providerResponse struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

func newProviderClient(url string) gateway.ProviderCall {
	if url == "" {
		return nil
	}

	client := &http.Client{}
	return func(ctx context.Context, req gateway.SMSRequest) (gateway.ProviderResult, error) {
		payload := providerRequest{
			ReferenceID: req.ReferenceID,
			To:          req.To,
			Message:     req.Message,
			TenantID:    req.TenantID,
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return gateway.ProviderResult{}, err
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return gateway.ProviderResult{}, err
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(httpReq)
		if err != nil {
			return gateway.ProviderResult{}, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return gateway.ProviderResult{}, errors.New("provider non-200 response")
		}

		dec := json.NewDecoder(resp.Body)
		var providerResp providerResponse
		if err := dec.Decode(&providerResp); err != nil {
			return gateway.ProviderResult{}, err
		}
		if err := dec.Decode(&struct{}{}); err != io.EOF {
			return gateway.ProviderResult{}, errors.New("provider response has trailing data")
		}
		if providerResp.Status == "" {
			return gateway.ProviderResult{}, errors.New("provider status missing")
		}

		return gateway.ProviderResult{
			Status: providerResp.Status,
			Reason: providerResp.Reason,
		}, nil
	}
}
