package utils

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
)

var (
	ErrInvalidECPrivateKey = fmt.Errorf("invalid private key, make sure your private key is generated with a curve at least as strong as prime256v1")
	ErrInvalidECPublicKey  = fmt.Errorf("invalid public key, make sure your public key is generated with a curve at least as strong as prime256v1")
)

// ParseStrongECPublicKey parses a strong elliptic curve public key from a PEM-encoded string.
// It returns the parsed public key or an error if the key is invalid or not strong enough.
func ParseStrongECPublicKey(publicKeyStr string) (*ecdsa.PublicKey, error) {
	// Decode PEM block
	block, _ := pem.Decode([]byte(publicKeyStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block containing public key: %w", ErrInvalidECPublicKey)
	}

	// Parse the public key
	publicKeyInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse EC public key: %w", ErrInvalidECPublicKey)
	}

	// Check if the parsed public key is of type *ecdsa.PublicKey
	publicKey, ok := publicKeyInterface.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not a valid elliptic curve public key: %w", ErrInvalidECPublicKey)
	}

	// Check if the public key is using a curve that's at least as strong as prime256v1 (P-256)
	if publicKey.Curve.Params().BitSize < elliptic.P256().Params().BitSize {
		return nil, fmt.Errorf("public key curve is not at least as strong as prime256v1: %w", ErrInvalidECPublicKey)
	}

	return publicKey, nil
}

// ParseStrongECPrivateKey parses a strong elliptic curve private key from a PEM-encoded string.
// It returns the parsed private key or an error if the key is invalid or not strong enough.
func ParseStrongECPrivateKey(privateKeyStr string) (*ecdsa.PrivateKey, error) {
	// Decode PEM block
	block, _ := pem.Decode([]byte(privateKeyStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block containing private key: %w", ErrInvalidECPrivateKey)
	}

	// Attempts to parse using ParseECPrivateKey or ParsePKCS8PrivateKey
	var err error
	var privateKey *ecdsa.PrivateKey
	privateKey, err = x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		// Parse the private key
		pkcsPrivateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse EC private key: %w", ErrInvalidECPrivateKey)
		}

		// Check if the public key is of type *ecdsa.PublicKey
		var ok bool
		if privateKey, ok = pkcsPrivateKey.(*ecdsa.PrivateKey); !ok {
			return nil, fmt.Errorf("not a valid elliptic curve private key: %w", ErrInvalidECPrivateKey)
		}
	}

	// Check if the public key is using a curve that's at least as strong as prime256v1 (P-256)
	if privateKey.Curve.Params().BitSize < elliptic.P256().Params().BitSize {
		return nil, fmt.Errorf("private key curve is not at least as strong as prime256v1: %w", ErrInvalidECPrivateKey)
	}

	return privateKey, nil
}

// GetEC256PublicKeyFromPrivateKey returns the public key of the given EC private key.
func GetEC256PublicKeyFromPrivateKey(key string) (string, error) {
	privateKey, err := ParseStrongECPrivateKey(key)
	if err != nil {
		return "", fmt.Errorf("validating EC private key: %w", err)
	}

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", fmt.Errorf("marshaling public key: %w", err)
	}

	publicKeyPEMEncoded := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	return strings.TrimSpace(string(publicKeyPEMEncoded)), nil
}
