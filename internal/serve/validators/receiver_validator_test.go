package validators

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/dto"
)

func TestNewReceiverValidator(t *testing.T) {
	validator := NewReceiverValidator()
	assert.NotNil(t, validator)
	assert.NotNil(t, validator.Validator)
}

func TestReceiverValidator_ValidateCreateReceiverRequest(t *testing.T) {
	testCases := []struct {
		name           string
		request        dto.CreateReceiverRequest
		expectError    bool
		errorFields    []string
		expectedErrors map[string]string
	}{
		{
			name: "valid request with email and verifications",
			request: dto.CreateReceiverRequest{
				Email:      "frodo@example.com",
				ExternalID: "Bag-End-001",
				Verifications: []dto.ReceiverVerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "1990-01-01",
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid request with phone and public key address",
			request: dto.CreateReceiverRequest{
				PhoneNumber: "+12345678901",
				ExternalID:  "Rivendell-456",
				Wallets: []dto.ReceiverWalletRequest{
					{
						Address: "GCQFMQ7U33ICSLAVGBJNX6P66M5GGOTQWCRZ5Y3YXYK3EB3DNCWOAD5K",
						Memo:    "precious",
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid request with phone and contract address",
			request: dto.CreateReceiverRequest{
				PhoneNumber: "+12345678901",
				ExternalID:  "Rivendell-456",
				Wallets: []dto.ReceiverWalletRequest{
					{
						Address: "CCWI7ARG6VMMREIKK2AOW6BYYDFCQQH52KW2J5AMPZKRRIFAMDQ44TUZ",
					},
				},
			},
		},
		{
			name: "missing contact information",
			request: dto.CreateReceiverRequest{
				ExternalID: "Mordor-Mount-Doom",
				Verifications: []dto.ReceiverVerificationRequest{
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
			request: dto.CreateReceiverRequest{
				Email: "gandalf@example.com",
				Verifications: []dto.ReceiverVerificationRequest{
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
			request: dto.CreateReceiverRequest{
				Email:      "aragorn@test.local",
				ExternalID: "Minas-Tirith-001",
			},
			expectError: true,
			errorFields: []string{"verifications", "wallets"},
		},
		{
			name: "invalid email format",
			request: dto.CreateReceiverRequest{
				Email:      "@mordor.net",
				ExternalID: "Misty-Mountains-001",
				Verifications: []dto.ReceiverVerificationRequest{
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
			request: dto.CreateReceiverRequest{
				PhoneNumber: "01-RING",
				ExternalID:  "Isengard-001",
				Verifications: []dto.ReceiverVerificationRequest{
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
			request: dto.CreateReceiverRequest{
				Email:      "legolas@example.com",
				ExternalID: "Lothlorien-001",
				Wallets: []dto.ReceiverWalletRequest{
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
			request: dto.CreateReceiverRequest{
				Email:      "gimli@test.local",
				ExternalID: "Moria-001",
				Wallets: []dto.ReceiverWalletRequest{
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
			request: dto.CreateReceiverRequest{
				Email:      "boromir@example.com",
				ExternalID: "Osgiliath-001",
				Verifications: []dto.ReceiverVerificationRequest{
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
			request: dto.CreateReceiverRequest{
				Email:      "samwise@test.local",
				ExternalID: "Hobbiton-001",
				Verifications: []dto.ReceiverVerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "invalid-date",
					},
				},
			},
			expectError: true,
			errorFields: []string{"verifications[0].value"},
		},
		{
			name: "invalid memo for contract address",
			request: dto.CreateReceiverRequest{
				PhoneNumber: "+12345678901",
				ExternalID:  "Rivendell-456",
				Wallets: []dto.ReceiverWalletRequest{
					{
						Address: "CCWI7ARG6VMMREIKK2AOW6BYYDFCQQH52KW2J5AMPZKRRIFAMDQ44TUZ",
						Memo:    "should-not-be-here",
					},
				},
			},
			expectError: true,
			errorFields: []string{"wallets[0].memo"},
			expectedErrors: map[string]string{
				"wallets[0].memo": "memos are not supported for contract addresses",
			},
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

					if expectedMsg, ok := tc.expectedErrors[field]; ok {
						assert.Equal(t, expectedMsg, validator.Errors[field])
					}
				}
			} else {
				require.False(t, validator.HasErrors(), "Expected no validation errors but found: %v", validator.Errors)
			}
		})
	}
}

func TestReceiverValidator_TrimValues(t *testing.T) {
	validator := NewReceiverValidator()
	request := &dto.CreateReceiverRequest{
		Email:       "  merry@example.com  ",
		PhoneNumber: "  +1234567890  ",
		ExternalID:  "  Bree-001  ",
		Verifications: []dto.ReceiverVerificationRequest{
			{
				Type:  data.VerificationTypeDateOfBirth,
				Value: "  1990-01-01  ",
			},
		},
		Wallets: []dto.ReceiverWalletRequest{
			{
				Address: "  GCQFMQ7U33ICSLAVGBJNX6P66M5GGOTQWCRZ5Y3YXYK3EB3DNCWOAD5K  ",
				Memo:    "  my-precious  ",
			},
		},
	}

	validator.ValidateCreateReceiverRequest(request)

	assert.Equal(t, "merry@example.com", request.Email)
	assert.Equal(t, "+1234567890", request.PhoneNumber)
	assert.Equal(t, "Bree-001", request.ExternalID)
	assert.Equal(t, "1990-01-01", request.Verifications[0].Value)
	assert.Equal(t, "GCQFMQ7U33ICSLAVGBJNX6P66M5GGOTQWCRZ5Y3YXYK3EB3DNCWOAD5K", request.Wallets[0].Address)
	assert.Equal(t, "my-precious", request.Wallets[0].Memo)
}
