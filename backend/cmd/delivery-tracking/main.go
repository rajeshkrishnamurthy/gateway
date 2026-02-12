package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/microsoft/go-mssqldb"

	"gateway/deliverytracking"
)

var (
	addrFlag          = flag.String("addr", ":8083", "HTTP listen address")
	mssqlHostFlag     = flag.String("sql-host", envOrDefault("MSSQL_HOST", "localhost"), "SQL Server host")
	mssqlPortFlag     = flag.String("sql-port", envOrDefault("MSSQL_PORT", "1433"), "SQL Server port")
	mssqlUserFlag     = flag.String("sql-user", envOrDefault("MSSQL_USER", "sa"), "SQL Server user")
	mssqlPasswordFlag = flag.String("sql-password", envOrDefault("MSSQL_SA_PASSWORD", ""), "SQL Server password")
	mssqlDBFlag       = flag.String("sql-db", envOrDefault("MSSQL_DATABASE", "setu"), "SQL Server database")
	mssqlEncryptFlag  = flag.String("sql-encrypt", envOrDefault("MSSQL_ENCRYPT", "disable"), "SQL Server encrypt setting")
)

func main() {
	flag.Parse()

	dsn, err := buildSQLServerDSN(*mssqlHostFlag, *mssqlPortFlag, *mssqlUserFlag, *mssqlPasswordFlag, *mssqlDBFlag, *mssqlEncryptFlag)
	if err != nil {
		log.Fatalf("build SQL Server DSN: %v", err)
	}

	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		log.Fatalf("open SQL Server: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := db.PingContext(ctx); err != nil {
		cancel()
		log.Fatalf("ping SQL Server: %v", err)
	}
	cancel()

	webhookIngestor, err := deliverytracking.NewWebhookIngestor(db)
	if err != nil {
		log.Fatalf("construct delivery webhook ingestor: %v", err)
	}
	reader, err := deliverytracking.NewReader(db)
	if err != nil {
		log.Fatalf("construct delivery reader: %v", err)
	}
	metrics := deliverytracking.NewMetrics(db)
	processor, err := deliverytracking.NewProcessorWithMetrics(db, metrics)
	if err != nil {
		log.Fatalf("construct delivery processor: %v", err)
	}

	server := &apiServer{
		webhookIngestor: webhookIngestor,
		processor:       processor,
		reader:          reader,
		metrics:         metrics,
	}
	httpServer := &http.Server{
		Addr:    *addrFlag,
		Handler: newMux(server),
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	log.Printf("delivery-tracking listening on %s", *addrFlag)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("http server: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func buildSQLServerDSN(host, port, user, password, database, encrypt string) (string, error) {
	if password == "" {
		return "", fmt.Errorf("sql password is required")
	}
	uri := &url.URL{
		Scheme: "sqlserver",
		User:   url.UserPassword(user, password),
		Host:   fmt.Sprintf("%s:%s", host, port),
	}
	query := url.Values{}
	query.Set("database", database)
	query.Set("encrypt", encrypt)
	uri.RawQuery = query.Encode()
	return uri.String(), nil
}
