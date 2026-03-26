package seputil

import (
	"context"
	"net/url"
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
)

func GetWebAuthDomain(ctx context.Context, baseURL string) string {
	currentTenant, err := sdpcontext.GetTenantFromContext(ctx)
	if err == nil && currentTenant != nil && currentTenant.BaseURL != nil {
		parsedURL, parseErr := url.Parse(*currentTenant.BaseURL)
		if parseErr == nil {
			return parsedURL.Host
		}
	}

	return GetBaseDomain(baseURL)
}

func GetBaseDomain(baseURL string) string {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return parsedURL.Host
}

func IsValidHomeDomain(baseURL, homeDomain string) bool {
	baseDomain := GetBaseDomain(baseURL)
	if baseDomain == "" || homeDomain == "" {
		return false
	}

	baseDomainLower := strings.ToLower(baseDomain)
	homeDomainLower := strings.ToLower(homeDomain)

	if homeDomainLower == baseDomainLower {
		return true
	}

	return strings.HasSuffix(homeDomainLower, "."+baseDomainLower)
}
