package validators

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

func TestNewReceiverValidator(t *testing.T) {
	validator := NewReceiverValidator()
	assert.NotNil(t, validator)
	assert.NotNil(t, validator.Validator)
}

func TestReceiverValidator_ValidateCreateReceiverRequest(t *testing.T) {
	testCases := []struct {
		name        string
		request     CreateReceiverRequest
		expectError bool
		errorFields []string
	}{
		{
			name: "valid request with email and verifications",
			request: CreateReceiverRequest{
				Email:      "frodo@shire.me",
				ExternalID: "Bag-End-001",
				Verifications: []ReceiverVerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "1990-01-01",
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid request with phone and wallet",
			request: CreateReceiverRequest{
				PhoneNumber: "+12345678901",
				ExternalID:  "Rivendell-456",
				Wallets: []ReceiverWalletRequest{
					{
						Address: "GCQFMQ7U33ICSLAVGBJNX6P66M5GGOTQWCRZ5Y3YXYK3EB3DNCWOAD5K",
						Memo:    "precious",
					},
				},
			},
			expectError: false,
		},
		{
			name: "missing contact information",
			request: CreateReceiverRequest{
				ExternalID: "Mordor-Mount-Doom",
				Verifications: []ReceiverVerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "1990-01-01",
					},
				},
			},
			expectError: true,
			errorFields: []string{"email", "phone_number"},
		},
		{
			name: "missing external ID",
			request: CreateReceiverRequest{
				Email: "gandalf@istari.me",
				Verifications: []ReceiverVerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "1990-01-01",
					},
				},
			},
			expectError: true,
			errorFields: []string{"external_id"},
		},
		{
			name: "missing verifications and wallets",
			request: CreateReceiverRequest{
				Email:      "aragorn@rangers.me",
				ExternalID: "Minas-Tirith-001",
			},
			expectError: true,
			errorFields: []string{"verifications", "wallets"},
		},
		{
			name: "invalid email format",
			request: CreateReceiverRequest{
				Email:      "@mordor.net",
				ExternalID: "Misty-Mountains-001",
				Verifications: []ReceiverVerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "1990-01-01",
					},
				},
			},
			expectError: true,
			errorFields: []string{"email"},
		},
		{
			name: "invalid phone number format",
			request: CreateReceiverRequest{
				PhoneNumber: "01-RING",
				ExternalID:  "Isengard-001",
				Verifications: []ReceiverVerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "1990-01-01",
					},
				},
			},
			expectError: true,
			errorFields: []string{"phone_number"},
		},
		{
			name: "multiple wallets not allowed",
			request: CreateReceiverRequest{
				Email:      "legolas@mirkwood.me",
				ExternalID: "Lothlorien-001",
				Wallets: []ReceiverWalletRequest{
					{
						Address: "GCQFMQ7U33ICSLAVGBJNX6P66M5GGOTQWCRZ5Y3YXYK3EB3DNCWOAD5K",
					},
					{
						Address: "GDQNY3PBOJOKYZSRMK2S7LHHGWZIUISD4QORETLMXEWXBI7KFZZMKTL3",
					},
				},
			},
			expectError: true,
			errorFields: []string{"wallets"},
		},
		{
			name: "invalid stellar address",
			request: CreateReceiverRequest{
				Email:      "gimli@erebor.me",
				ExternalID: "Moria-001",
				Wallets: []ReceiverWalletRequest{
					{
						Address: "invalid-address",
					},
				},
			},
			expectError: true,
			errorFields: []string{"wallets[0].address"},
		},
		{
			name: "invalid verification type",
			request: CreateReceiverRequest{
				Email:      "boromir@gondor.me",
				ExternalID: "Osgiliath-001",
				Verifications: []ReceiverVerificationRequest{
					{
						Type:  "INVALID_TYPE",
						Value: "test-value",
					},
				},
			},
			expectError: true,
			errorFields: []string{"verifications[0].type"},
		},
		{
			name: "invalid date of birth format",
			request: CreateReceiverRequest{
				Email:      "samwise@shire.me",
				ExternalID: "Hobbiton-001",
				Verifications: []ReceiverVerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "invalid-date",
					},
				},
			},
			expectError: true,
			errorFields: []string{"verifications[0].value"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validator := NewReceiverValidator()
			validator.ValidateCreateReceiverRequest(&tc.request)

			if tc.expectError {
				require.True(t, validator.HasErrors(), "Expected validation errors but none found")

				for _, field := range tc.errorFields {
					_, exists := validator.Errors[field]
					assert.True(t, exists, "Expected error field '%s' not found in validation errors", field)
				}
			} else {
				require.False(t, validator.HasErrors(), "Expected no validation errors but found: %v", validator.Errors)
			}
		})
	}
}

func TestReceiverValidator_TrimValues(t *testing.T) {
	validator := NewReceiverValidator()
	request := &CreateReceiverRequest{
		Email:       "  merry@buckland.me  ",
		PhoneNumber: "  +1234567890  ",
		ExternalID:  "  Bree-001  ",
		Verifications: []ReceiverVerificationRequest{
			{
				Type:  data.VerificationTypeDateOfBirth,
				Value: "  1990-01-01  ",
			},
		},
		Wallets: []ReceiverWalletRequest{
			{
				Address: "  GCQFMQ7U33ICSLAVGBJNX6P66M5GGOTQWCRZ5Y3YXYK3EB3DNCWOAD5K  ",
				Memo:    "  my-precious  ",
			},
		},
	}

	validator.ValidateCreateReceiverRequest(request)

	assert.Equal(t, "merry@buckland.me", request.Email)
	assert.Equal(t, "+1234567890", request.PhoneNumber)
	assert.Equal(t, "Bree-001", request.ExternalID)
	assert.Equal(t, "1990-01-01", request.Verifications[0].Value)
	assert.Equal(t, "GCQFMQ7U33ICSLAVGBJNX6P66M5GGOTQWCRZ5Y3YXYK3EB3DNCWOAD5K", request.Wallets[0].Address)
	assert.Equal(t, "my-precious", request.Wallets[0].Memo)
}
