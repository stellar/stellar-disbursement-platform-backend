package utils

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// dekBytes is the size of the random per-wallet data-encryption key (DEK).
const dekBytes = 32

// EnvelopeEncrypter implements two-layer (envelope) encryption on top of the existing AES-GCM
// primitives: a message is encrypted with a random per-use DEK, and the DEK is wrapped with the
// caller's key-encryption key (KEK). Each envelope is independent — re-encrypting or corrupting
// one envelope can never affect another, which is the isolation property required for
// per-distribution-wallet secret material.
type EnvelopeEncrypter struct{}

// EncryptWithNewDEK encrypts message under a freshly generated random DEK and wraps the DEK with
// kek. It returns (ciphertext, encryptedDEK).
func (EnvelopeEncrypter) EncryptWithNewDEK(message, kek string) (ciphertext string, encryptedDEK string, err error) {
	dek := make([]byte, dekBytes)
	if _, err = rand.Read(dek); err != nil {
		return "", "", fmt.Errorf("generating random DEK: %w", err)
	}
	dekStr := base64.StdEncoding.EncodeToString(dek)

	ciphertext, err = Encrypt(message, dekStr)
	if err != nil {
		return "", "", fmt.Errorf("encrypting message with DEK: %w", err)
	}

	encryptedDEK, err = Encrypt(dekStr, kek)
	if err != nil {
		return "", "", fmt.Errorf("wrapping DEK with KEK: %w", err)
	}

	return ciphertext, encryptedDEK, nil
}

// DecryptWithDEK unwraps encryptedDEK with kek and uses it to decrypt ciphertext.
func (EnvelopeEncrypter) DecryptWithDEK(ciphertext, encryptedDEK, kek string) (string, error) {
	dekStr, err := Decrypt(encryptedDEK, kek)
	if err != nil {
		return "", fmt.Errorf("unwrapping DEK with KEK: %w", err)
	}

	message, err := Decrypt(ciphertext, dekStr)
	if err != nil {
		return "", fmt.Errorf("decrypting message with DEK: %w", err)
	}

	return message, nil
}

// RotateDEK decrypts the envelope and re-encrypts its message under a brand-new random DEK.
// Only the given envelope is affected; sibling envelopes' ciphertext is untouched by design.
func (e EnvelopeEncrypter) RotateDEK(ciphertext, encryptedDEK, kek string) (newCiphertext string, newEncryptedDEK string, err error) {
	message, err := e.DecryptWithDEK(ciphertext, encryptedDEK, kek)
	if err != nil {
		return "", "", fmt.Errorf("decrypting envelope before rotation: %w", err)
	}

	return e.EncryptWithNewDEK(message, kek)
}
