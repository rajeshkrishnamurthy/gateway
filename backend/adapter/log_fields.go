package adapter

import "strings"

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
