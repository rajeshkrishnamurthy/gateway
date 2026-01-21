package adapter

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func maskRecipient(recipient string) string {
	recipient = strings.TrimSpace(recipient)
	if recipient == "" {
		return ""
	}
	const keep = 4
	if len(recipient) <= keep {
		return recipient
	}
	return strings.Repeat("*", len(recipient)-keep) + recipient[len(recipient)-keep:]
}

func hashMessage(message string) string {
	sum := sha256.Sum256([]byte(message))
	return hex.EncodeToString(sum[:])
}
