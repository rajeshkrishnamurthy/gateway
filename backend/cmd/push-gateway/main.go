package main

import (
	"context"
	"errors"
	"flag"
	"gateway"
	"gateway/metrics"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var configPath = flag.String("config", "conf/config_push.json", "Gateway config file path")
var listenAddr = flag.String("addr", ":8081", "HTTP listen address")
var showHelp = flag.Bool("help", false, "show usage")
var showVersion = flag.Bool("version", false, "show version")

const version = "0.1.0"

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

	startTime := time.Now()

	providerTimeout := time.Duration(cfg.PushProviderTimeoutSeconds) * time.Second
	providerConnectTimeout := time.Duration(cfg.PushProviderConnectTimeoutSeconds) * time.Second
	providerCall, providerName, err := providerFromConfig(cfg, providerConnectTimeout)
	if err != nil {
		log.Fatal(err)
	}

	metricsRegistry := metrics.New(providerName, latencyBuckets)
	gw, err := gateway.NewPushGateway(gateway.PushConfig{
		ProviderCall:    providerCall,
		ProviderTimeout: providerTimeout,
		Metrics:         metricsRegistry,
	})
	if err != nil {
		log.Fatal(err)
	}

	ui, err := newUIServer(providerName, providerTimeout, cfg.GrafanaDashboardURL, metricsRegistry, startTime)
	if err != nil {
		log.Printf("ui disabled: %v", err)
	}

	server := &http.Server{
		Addr:    *listenAddr,
		Handler: newMux(gw, metricsRegistry, ui),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	log.Printf(
		"listening on %s configPath=%q pushProvider=%q pushProviderUrl=%q pushProviderTimeoutSeconds=%d pushProviderConnectTimeoutSeconds=%d grafanaDashboardUrl=%q",
		*listenAddr,
		*configPath,
		cfg.PushProvider,
		cfg.PushProviderURL,
		cfg.PushProviderTimeoutSeconds,
		cfg.PushProviderConnectTimeoutSeconds,
		cfg.GrafanaDashboardURL,
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
