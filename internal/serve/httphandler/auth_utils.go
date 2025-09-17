package httphandler

import (
	"context"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

// CAPTCHAConfig holds the configuration for CAPTCHA validation
type CAPTCHAConfig struct {
	Models            *data.Models
	ReCAPTCHADisabled bool
}

// IsCAPTCHADisabled checks if CAPTCHA is disabled considering both org and environment settings
func IsCAPTCHADisabled(ctx context.Context, config CAPTCHAConfig) bool {
	org, err := config.Models.Organizations.Get(ctx)
	if err != nil {
		return config.ReCAPTCHADisabled
	}

	return org.CAPTCHADisabled
}

// MFAConfig holds the configuration for MFA validation.
type MFAConfig struct {
	Models     *data.Models
	MFADisabled bool
}

// IsMFADisabled checks if MFA is disabled considering both org and environment settings.
func IsMFADisabled(ctx context.Context, config MFAConfig) bool {
	org, err := config.Models.Organizations.Get(ctx)
	if err != nil {
		return config.MFADisabled
	}

	return org.MFADisabled
}
