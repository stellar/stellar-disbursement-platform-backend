package utils

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/stellar/go-stellar-sdk/keypair"
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

func GetURLWithScheme(rawURL string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parsing url: %w", err)
	}

	if parsedURL.Scheme == "" || !strings.Contains("http https", parsedURL.Scheme) {
		rawURL, err = url.JoinPath("http://", rawURL)
		if err != nil {
			return "", fmt.Errorf("joining scheme to raw URL: %w", err)
		}
	}

	return rawURL, nil
}

func GenerateTenantURL(baseURL string, tenantID string) (string, error) {
	if tenantID == "" {
		return "", fmt.Errorf("tenantID is empty")
	}

	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL %s: %w", baseURL, err)
	}

	hostParts := strings.SplitN(parsedURL.Hostname(), ".", 2)
	if len(hostParts) != 2 {
		return "", fmt.Errorf("base URL must have at least two domain parts %s", baseURL)
	}

	newHostname := fmt.Sprintf("%s.%s", tenantID, parsedURL.Hostname())
	if parsedURL.Port() != "" {
		parsedURL.Host = newHostname + ":" + parsedURL.Port()
	} else {
		parsedURL.Host = newHostname
	}

	return parsedURL.String(), nil
}

// IsStaticAsset determines if a path refers to a static asset.
func IsStaticAsset(path string) bool {
	if path == "" {
		return false
	}

	path = strings.TrimPrefix(path, "/")

	lastSlashIndex := strings.LastIndex(path, "/")
	var lastPart string
	if lastSlashIndex == -1 {
		lastPart = path
	} else {
		lastPart = path[lastSlashIndex+1:]
	}

	// Check if the last part contains a dot (has an extension)
	return filepath.Ext(lastPart) != ""
}

// IsBaseURL checks if the provided URL is a base URL.
func IsBaseURL(urlStr string) (bool, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false, fmt.Errorf("parsing url: %w", err)
	}

	// Check if path is empty or just "/" AND no query params AND no fragment
	return (u.Path == "" || u.Path == "/") &&
		u.RawQuery == "" &&
		u.Fragment == "", nil
}
