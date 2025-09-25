package validators

import "slices"

type CAPTCHAType string

const (
	GoogleReCAPTCHAV2 CAPTCHAType = "GOOGLE_RECAPTCHA_V2"
	GoogleReCAPTCHAV3 CAPTCHAType = "GOOGLE_RECAPTCHA_V3"
)

func ValidCAPTCHATypes() []CAPTCHAType {
	return []CAPTCHAType{
		GoogleReCAPTCHAV2,
		GoogleReCAPTCHAV3,
	}
}

func (ct CAPTCHAType) IsValid() bool {
	return slices.Contains(ValidCAPTCHATypes(), ct)
}

func (ct CAPTCHAType) String() string {
	return string(ct)
}
