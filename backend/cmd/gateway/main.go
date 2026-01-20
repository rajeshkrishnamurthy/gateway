package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"gateway"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const maxBodyBytes = 16 << 10

var addr = flag.String("addr", ":8080", "HTTP listen address")
var smsProvider = flag.String("sms-provider", "", "SMS provider name")

func main() {
	flag.Parse()

	gw, err := gateway.New(gateway.Config{Provider: *smsProvider})
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

func newMux(gw *gateway.Gateway) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/sms/send", handleSMSSend(gw))
	return mux
}

func handleSMSSend(gw *gateway.Gateway) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

		var req gateway.SMSRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		_, err := gw.SendSMS(r.Context(), req)
		if err != nil {
			w.WriteHeader(http.StatusNotImplemented)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}
