package validators

import (
	"fmt"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
)

// CAPTCHAValidatorFactory creates CAPTCHA validators based on the specified type.
type CAPTCHAValidatorFactory struct{}

func NewCAPTCHAValidatorFactory() *CAPTCHAValidatorFactory {
	return &CAPTCHAValidatorFactory{}
}

// CreateValidator creates a CAPTCHA validator based on the specified type and configuration.
func (f *CAPTCHAValidatorFactory) CreateValidator(captchaType CAPTCHAType, siteSecretKey string, minScore float64) (ReCAPTCHAValidator, error) {
	if !captchaType.IsValid() {
		return nil, fmt.Errorf("invalid CAPTCHA type: %s", captchaType)
	}

	if siteSecretKey == "" {
		return nil, fmt.Errorf("site secret key is required for CAPTCHA validation")
	}

	httpClient := httpclient.DefaultClient()

	switch captchaType {
	case GoogleReCAPTCHAV2:
		return NewGoogleReCAPTCHAValidator(siteSecretKey, httpClient), nil
	case GoogleReCAPTCHAV3:
		return NewGoogleReCAPTCHAV3Validator(siteSecretKey, minScore, httpClient), nil
	default:
		return nil, fmt.Errorf("unsupported CAPTCHA type: %s", captchaType)
	}
}

// CreateValidatorWithDefaults creates a CAPTCHA validator with default settings.
func (f *CAPTCHAValidatorFactory) CreateValidatorWithDefaults(captchaType CAPTCHAType, siteSecretKey string) (ReCAPTCHAValidator, error) {
	return f.CreateValidator(captchaType, siteSecretKey, defaultMinScore)
}
