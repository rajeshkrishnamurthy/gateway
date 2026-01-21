package pii

import (
	"crypto/sha256"
	"encoding/hex"
)

// Hash returns the SHA-256 hash of a value for safe correlation.
func Hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
