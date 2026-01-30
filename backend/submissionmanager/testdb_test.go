package submissionmanager

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	password, ok, err := resolveSQLPassword(t)
	if err != nil {
		t.Fatalf("resolve sql password: %v", err)
	}
	if !ok {
		t.Skip("MSSQL_SA_PASSWORD not set; start docker compose and set env or backend/.env")
	}

	host := envOrDefault("MSSQL_HOST", "localhost")
	port := envOrDefault("MSSQL_PORT", "1433")

	masterDB, err := sql.Open("sqlserver", buildSQLServerDSN(host, port, password, "master"))
	if err != nil {
		t.Fatalf("open master db: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	if err := masterDB.PingContext(ctx); err != nil {
		_ = masterDB.Close()
		t.Fatalf("ping master db: %v", err)
	}

	dbName := fmt.Sprintf("submissionmanager_test_%d", time.Now().UnixNano())
	if _, err := masterDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE [%s]", dbName)); err != nil {
		_ = masterDB.Close()
		t.Fatalf("create database: %v", err)
	}

	db, err := sql.Open("sqlserver", buildSQLServerDSN(host, port, password, dbName))
	if err != nil {
		_ = dropTestDB(ctx, masterDB, dbName)
		t.Fatalf("open test db: %v", err)
	}

	schemaPath := filepath.Join(moduleRoot(t), "conf", "sql", "submissionmanager", "001_create_schema.sql")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		_ = db.Close()
		_ = dropTestDB(ctx, masterDB, dbName)
		t.Fatalf("read schema: %v", err)
	}
	if _, err := db.ExecContext(ctx, string(schema)); err != nil {
		_ = db.Close()
		_ = dropTestDB(ctx, masterDB, dbName)
		t.Fatalf("apply schema: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
		_ = dropTestDB(context.Background(), masterDB, dbName)
		_ = masterDB.Close()
	})

	return db
}

func dropTestDB(ctx context.Context, masterDB *sql.DB, dbName string) error {
	_, _ = masterDB.ExecContext(ctx, fmt.Sprintf("ALTER DATABASE [%s] SET SINGLE_USER WITH ROLLBACK IMMEDIATE", dbName))
	_, err := masterDB.ExecContext(ctx, fmt.Sprintf("DROP DATABASE [%s]", dbName))
	return err
}

func buildSQLServerDSN(host, port, password, database string) string {
	u := &url.URL{
		Scheme: "sqlserver",
		User:   url.UserPassword("sa", password),
		Host:   fmt.Sprintf("%s:%s", host, port),
	}
	query := url.Values{}
	query.Set("database", database)
	query.Set("encrypt", "disable")
	u.RawQuery = query.Encode()
	return u.String()
}

func resolveSQLPassword(t *testing.T) (string, bool, error) {
	t.Helper()
	if value, ok := os.LookupEnv("MSSQL_SA_PASSWORD"); ok && strings.TrimSpace(value) != "" {
		return value, true, nil
	}

	data, err := os.ReadFile(filepath.Join(moduleRoot(t), ".env"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "MSSQL_SA_PASSWORD" && value != "" {
			return strings.Trim(value, "\"'"), true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", false, err
	}
	return "", false, nil
}

func envOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return fallback
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve module root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), ".."))
}
