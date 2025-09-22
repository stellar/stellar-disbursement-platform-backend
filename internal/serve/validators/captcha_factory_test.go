package validators

import (
	"slices"
	"testing"
)

func TestCAPTCHAValidatorFactory_CreateValidator(t *testing.T) {
	factory := NewCAPTCHAValidatorFactory()

	tests := []struct {
		name        string
		captchaType CAPTCHAType
		siteKey     string
		minScore    float64
		wantErr     bool
	}{
		{
			name:        "valid reCAPTCHA v2",
			captchaType: GoogleReCAPTCHAV2,
			siteKey:     "test-site-key",
			minScore:    0.5,
			wantErr:     false,
		},
		{
			name:        "valid reCAPTCHA v3",
			captchaType: GoogleReCAPTCHAV3,
			siteKey:     "test-site-key",
			minScore:    0.7,
			wantErr:     false,
		},
		{
			name:        "invalid CAPTCHA type",
			captchaType: CAPTCHAType("INVALID_TYPE"),
			siteKey:     "test-site-key",
			minScore:    0.5,
			wantErr:     true,
		},
		{
			name:        "empty site key",
			captchaType: GoogleReCAPTCHAV2,
			siteKey:     "",
			minScore:    0.5,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator, err := factory.CreateValidator(tt.captchaType, tt.siteKey, tt.minScore)

			if tt.wantErr {
				if err == nil {
					t.Errorf("CreateValidator() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("CreateValidator() unexpected error: %v", err)
				return
			}

			if validator == nil {
				t.Errorf("CreateValidator() returned nil validator")
			}
		})
	}
}

func TestCAPTCHAValidatorFactory_CreateValidatorWithDefaults(t *testing.T) {
	factory := NewCAPTCHAValidatorFactory()

	validator, err := factory.CreateValidatorWithDefaults(GoogleReCAPTCHAV3, "test-site-key")
	if err != nil {
		t.Errorf("CreateValidatorWithDefaults() unexpected error: %v", err)
	}

	if validator == nil {
		t.Errorf("CreateValidatorWithDefaults() returned nil validator")
	}
}

func TestCAPTCHAType_IsValid(t *testing.T) {
	tests := []struct {
		name        string
		captchaType CAPTCHAType
		want        bool
	}{
		{
			name:        "valid v2",
			captchaType: GoogleReCAPTCHAV2,
			want:        true,
		},
		{
			name:        "valid v3",
			captchaType: GoogleReCAPTCHAV3,
			want:        true,
		},
		{
			name:        "invalid type",
			captchaType: CAPTCHAType("malevelon creek"),
			want:        false,
		},
		{
			name:        "empty type",
			captchaType: CAPTCHAType(""),
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.captchaType.IsValid(); got != tt.want {
				t.Errorf("CAPTCHAType.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidCAPTCHATypes(t *testing.T) {
	validTypes := ValidCAPTCHATypes()

	expectedTypes := []CAPTCHAType{GoogleReCAPTCHAV2, GoogleReCAPTCHAV3}

	if len(validTypes) != len(expectedTypes) {
		t.Errorf("ValidCAPTCHATypes() returned %d types, want %d", len(validTypes), len(expectedTypes))
	}

	for _, expectedType := range expectedTypes {
		if !slices.Contains(validTypes, expectedType) {
			t.Errorf("ValidCAPTCHATypes() missing expected type: %v", expectedType)
		}
	}
}
