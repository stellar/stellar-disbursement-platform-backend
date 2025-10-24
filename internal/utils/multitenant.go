package utils

import (
	"errors"
	"net"
	"strings"
)

var (
	ErrTenantNameNotFound  = errors.New("tenant name not found")
	ErrHostnameIsIPAddress = errors.New("hostname is an IP address")
)

func ExtractTenantNameFromHostName(hostname string) (string, error) {
	// Strip port if present
	if host, _, err := net.SplitHostPort(hostname); err == nil {
		hostname = host
	}

	// Check if hostname is an IP address
	if net.ParseIP(hostname) != nil {
		return "", ErrHostnameIsIPAddress
	}

	// Extract subdomain from hostname (e.g. aidorg.sdp.com -> aidorg)
	parts := strings.Split(hostname, ".")
	// If there's more than 2 parts, it means there's a subdomain
	if len(parts) > 2 {
		return parts[0], nil
	}
	return "", ErrTenantNameNotFound
}
