package submissionmanager

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"strings"
	"time"

	mssql "github.com/microsoft/go-mssqldb"
)

func normalizeDBTime(value time.Time) time.Time {
	return time.Date(
		value.Year(),
		value.Month(),
		value.Day(),
		value.Hour(),
		value.Minute(),
		value.Second(),
		value.Nanosecond(),
		time.UTC,
	)
}

func payloadHash(payload []byte) []byte {
	sum := sha256.Sum256(payload)
	return sum[:]
}

func nullString(value string) sql.NullString {
	if strings.TrimSpace(value) == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func nullInt(value int) sql.NullInt32 {
	if value <= 0 {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: int32(value), Valid: true}
}

func isUniqueViolation(err error) bool {
	var mssqlErr mssql.Error
	if !errors.As(err, &mssqlErr) {
		return false
	}
	return mssqlErr.Number == 2627 || mssqlErr.Number == 2601
}
