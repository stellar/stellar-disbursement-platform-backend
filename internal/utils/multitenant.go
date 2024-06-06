package utils

import (
	"errors"
	"strings"
)

var ErrTenantNameNotFound = errors.New("tenant name not found")

func ExtractTenantNameFromHostName(hostname string) (string, error) {
	// Remove port number if present (e.g. aidorg.sdp.com:8000 -> aidorg.sdp.com)
	hostname = strings.Split(hostname, ":")[0]
	// Split by dots (e.g. aidorg.sdp.com -> [aidorg, sdp, com])
	parts := strings.Split(hostname, ".")
	// If there's more than 2 parts, it means there's a subdomain
	if len(parts) > 2 {
		return parts[0], nil
	}
	return "", ErrTenantNameNotFound
}
