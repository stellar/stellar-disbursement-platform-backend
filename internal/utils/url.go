package utils

import (
	"encoding/hex"
	"fmt"
	"net/url"

	"github.com/stellar/go/keypair"
)

func SignURL(stellarSecretKey string, rawURL string) (string, error) {
	// Validate stellar private key
	kp, err := keypair.ParseFull(stellarSecretKey)
	if err != nil {
		return "", fmt.Errorf("error parsing stellar private key: %w", err)
	}

	// Validate raw url
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("error parsing raw url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("raw url %q should have both a scheme and a host", rawURL)
	}

	// Sign url
	u.RawQuery = u.Query().Encode()
	signature, err := kp.Sign([]byte(u.String()))
	if err != nil {
		return "", fmt.Errorf("error signing url: %w", err)
	}
	signatureHex := hex.EncodeToString(signature)
	signedURL := u.String() + "&signature=" + signatureHex

	return signedURL, nil
}

func VerifySignedURL(signedURL string, expectedPublicKey string) (bool, error) {
	// Validate expected public key
	pubKey, err := keypair.ParseAddress(expectedPublicKey)
	if err != nil {
		return false, fmt.Errorf("error parsing expected public key: %w", err)
	}

	// Validate signed URL
	u, err := url.Parse(signedURL)
	if err != nil {
		return false, fmt.Errorf("error parsing signed url: %w", err)
	}

	// Extract signature from signed URL
	query := u.Query()
	signatureHex := query.Get("signature")
	if signatureHex == "" {
		return false, fmt.Errorf("signed url does not contain a signature")
	}
	signature, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false, fmt.Errorf("error decoding signature: %w", err)
	}

	// Remove signature from URL
	query.Del("signature")
	u.RawQuery = query.Encode()

	// Verify signature
	err = pubKey.Verify([]byte(u.String()), signature)
	if err != nil {
		return false, fmt.Errorf("error verifying URL signature: %w", err)
	}

	return true, nil
}
