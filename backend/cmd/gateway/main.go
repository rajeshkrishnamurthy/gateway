package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"gateway"
	"gateway/adapter"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const maxBodyBytes = 16 << 10

var configPath = flag.String("config", "config.json", "Gateway config file path")
var showHelp = flag.Bool("help", false, "show usage")
var showVersion = flag.Bool("version", false, "show version")

const version = "0.1.0"

const (
	minProviderConnectTimeout = 2 * time.Second
	maxProviderConnectTimeout = 10 * time.Second
)

type fileConfig struct {
	SMSProvider                      string `json:"smsProvider"`
	Addr                             string `json:"addr"`
	SMSProviderURL                   string `json:"smsProviderUrl"`
	SMSProviderTimeoutSeconds        int    `json:"smsProviderTimeoutSeconds"`
	SMSProviderConnectTimeoutSeconds int    `json:"smsProviderConnectTimeoutSeconds"`
}

func main() {
	flag.Parse()
	if *showHelp {
		flag.Usage()
		return
	}
	if *showVersion {
		log.Printf("gateway version %s", version)
		return
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	providerTimeout := time.Duration(cfg.SMSProviderTimeoutSeconds) * time.Second
	providerConnectTimeout := time.Duration(cfg.SMSProviderConnectTimeoutSeconds) * time.Second
	var providerCall gateway.ProviderCall
	switch cfg.SMSProvider {
	case "default":
		providerCall = adapter.DefaultProviderCall(cfg.SMSProviderURL, providerConnectTimeout)
	case "model":
		providerCall = adapter.ModelProviderCall(cfg.SMSProviderURL, providerConnectTimeout)
	default:
		log.Fatalf("smsProvider must be one of: default, model")
	}
	gw, err := gateway.New(gateway.Config{
		ProviderCall:    providerCall,
		ProviderTimeout: providerTimeout,
	})
	if err != nil {
		log.Fatal(err)
	}

	server := &http.Server{
		Addr:    cfg.Addr,
		Handler: newMux(gw),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	log.Printf(
		"listening on %s configPath=%q smsProvider=%q smsProviderUrl=%q smsProviderTimeoutSeconds=%d smsProviderConnectTimeoutSeconds=%d",
		cfg.Addr,
		*configPath,
		cfg.SMSProvider,
		cfg.SMSProviderURL,
		cfg.SMSProviderTimeoutSeconds,
		cfg.SMSProviderConnectTimeoutSeconds,
	)

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

func loadConfig(path string) (fileConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return fileConfig{}, err
	}
	defer file.Close()

	dec := json.NewDecoder(file)
	dec.DisallowUnknownFields()
	var cfg fileConfig
	if err := dec.Decode(&cfg); err != nil {
		return fileConfig{}, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fileConfig{}, errors.New("config has trailing data")
	}

	if strings.TrimSpace(cfg.Addr) == "" {
		return fileConfig{}, errors.New("addr is required")
	}
	cfg.SMSProvider = strings.TrimSpace(cfg.SMSProvider)
	if cfg.SMSProvider == "" {
		cfg.SMSProvider = "default"
	}
	switch cfg.SMSProvider {
	case "default", "model":
	default:
		return fileConfig{}, errors.New("smsProvider must be one of: default, model")
	}
	if strings.TrimSpace(cfg.SMSProviderURL) == "" {
		return fileConfig{}, errors.New("smsProviderUrl is required")
	}
	if cfg.SMSProviderTimeoutSeconds < 15 || cfg.SMSProviderTimeoutSeconds > 60 {
		return fileConfig{}, errors.New("smsProviderTimeoutSeconds must be between 15 and 60")
	}
	if cfg.SMSProviderConnectTimeoutSeconds == 0 {
		cfg.SMSProviderConnectTimeoutSeconds = int(minProviderConnectTimeout / time.Second)
	}
	connectTimeout := time.Duration(cfg.SMSProviderConnectTimeoutSeconds) * time.Second
	if connectTimeout < minProviderConnectTimeout || connectTimeout > maxProviderConnectTimeout {
		return fileConfig{}, errors.New("smsProviderConnectTimeoutSeconds must be between 2 and 10")
	}

	return cfg, nil
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
			log.Printf("sms decision referenceId=%q status=rejected reason=invalid_request source=validation detail=decode_error err=%v", "", err)
			writeSMSResponse(w, http.StatusBadRequest, gateway.SMSResponse{
				Status: "rejected",
				Reason: "invalid_request",
			})
			return
		}
		if err := dec.Decode(&struct{}{}); err != io.EOF {
			log.Printf("sms decision referenceId=%q status=rejected reason=invalid_request source=validation detail=trailing_json", req.ReferenceID)
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
		source := "provider_result"
		if err != nil && errors.Is(err, gateway.ErrInvalidRequest) {
			switch resp.Reason {
			case "invalid_recipient", "invalid_message":
				source = "provider_result"
			default:
				source = "validation"
			}
		} else if resp.Reason == "provider_failure" {
			source = "provider_failure"
		}
		log.Printf(
			"sms decision referenceId=%q status=%q reason=%q source=%s gatewayMessageId=%q",
			resp.ReferenceID,
			resp.Status,
			resp.Reason,
			source,
			resp.GatewayMessageID,
		)
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
