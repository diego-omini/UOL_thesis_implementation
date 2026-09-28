package persistence

// This file removes account identifiers, credentials, and opaque tokens before JSON persistence.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"unicode"
)

func accountFingerprint(accountID string) string {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return ""
	}
	digest := sha256.Sum256([]byte("uol-portfolio-tracker/account/" + accountID))
	return hex.EncodeToString(digest[:])
}

func sanitizedJSON(value any) ([]byte, error) {
	contents, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var generic any
	if err := json.Unmarshal(contents, &generic); err != nil {
		return nil, err
	}
	removeSensitiveFields(generic)
	return json.Marshal(generic)
}

func removeSensitiveFields(value any) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if sensitiveKey(key) {
				delete(current, key)
				continue
			}
			removeSensitiveFields(child)
		}
	case []any:
		for _, child := range current {
			removeSensitiveFields(child)
		}
	}
}

func sensitiveKey(key string) bool {
	var normalized strings.Builder
	for _, character := range strings.ToLower(key) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			normalized.WriteRune(character)
		}
	}
	switch normalized.String() {
	case "accountid", "apikey", "token", "previewtoken", "warningtoken", "replyid", "secret", "password":
		return true
	default:
		return false
	}
}
