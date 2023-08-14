package utils

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

// ParseECDSAPublicKey parses the given public key string and returns the *ecdsa.PublicKey.
func ParseECDSAPublicKey(publicKeyStr string) (*ecdsa.PublicKey, error) {
	// Decode PEM block
	block, _ := pem.Decode([]byte(publicKeyStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block containing public key")
	}

	// Parse the public key
	pkixPublicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse x509 PKIX public key: %w", err)
	}

	// Check if the public key is of type *ecdsa.PublicKey
	publicKey, ok := pkixPublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not of type ECDSA")
	}

	return publicKey, nil
}

// ParseECDSAPrivateKey parses the given private key string and returns the *ecdsa.PrivateKey.
func ParseECDSAPrivateKey(privateKeyStr string) (*ecdsa.PrivateKey, error) {
	// Decode PEM block
	block, _ := pem.Decode([]byte(privateKeyStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block containing private key")
	}

	// Parse the private key
	pkcsPrivateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse EC private key: %w", err)
	}

	// Check if the public key is of type *ecdsa.PublicKey
	privateKey, ok := pkcsPrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not of type ECDSA")
	}

	return privateKey, nil
}

// ValidateECDSAKeys validates if the given public and private keys are a valid ECDSA keypair.
func ValidateECDSAKeys(publicKeyStr, privateKeyStr string) error {
	publicKey, err := ParseECDSAPublicKey(publicKeyStr)
	if err != nil {
		return fmt.Errorf("validating ECDSA public key: %w", err)
	}

	privateKey, err := ParseECDSAPrivateKey(privateKeyStr)
	if err != nil {
		return fmt.Errorf("validating ECDSA private key: %w", err)
	}

	// Sign a test message using the private key
	msg := "test message"
	hash := sha256.Sum256([]byte(msg))
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hash[:])
	if err != nil {
		return fmt.Errorf("signing message for validation: %w", err)
	}

	// Verify the signature using the public key
	valid := ecdsa.Verify(publicKey, hash[:], r, s)
	if !valid {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}
