package utils

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/stellar/go/hash"
)

type ContactType string

const (
	ContactTypeEmail       ContactType = "EMAIL"
	ContactTypePhoneNumber ContactType = "PHONE_NUMBER"
)

// normalizeContactForSalt normalizes receiver contact information for consistent salt generation.
// This ensures that the same receiver contact always produces the same salt regardless of formatting.
func normalizeContactForSalt(contact string, contactType ContactType) string {
	switch contactType {
	case ContactTypeEmail:
		return strings.ToLower(strings.TrimSpace(contact))
	case ContactTypePhoneNumber:
		digitsOnly := regexp.MustCompile(`\D`).ReplaceAllString(contact, "")
		if len(digitsOnly) > 10 {
			digitsOnly = digitsOnly[len(digitsOnly)-10:]
		}
		return digitsOnly
	default:
		return strings.ToLower(strings.TrimSpace(contact))
	}
}

// GenerateSalt generates a hex-encoded salt for storing in TSS transactions
func GenerateSalt(receiverContact string, contactType string) (string, error) {
	if receiverContact == "" {
		return "", fmt.Errorf("receiver contact cannot be empty")
	}
	if contactType == "" {
		return "", fmt.Errorf("contact type cannot be empty")
	}

	if err := ValidateContactType(contactType); err != nil {
		return "", err
	}

	normalizedContact := normalizeContactForSalt(receiverContact, ContactType(contactType))

	hashInput := contactType + ":" + normalizedContact
	salt := hash.Hash([]byte(hashInput))

	return hex.EncodeToString(salt[:]), nil
}
