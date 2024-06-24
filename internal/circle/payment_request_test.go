package circle

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_PaymentRequest_getCircleAssetCode(t *testing.T) {
	tests := []struct {
		name              string
		stellarAssetCode  string
		expectedAssetCode string
		wantErr           string
	}{
		{
			name:              "USDC asset code",
			stellarAssetCode:  "USDC",
			expectedAssetCode: "USD",
			wantErr:           "",
		},
		{
			name:              "EURC asset code",
			stellarAssetCode:  "EURC",
			expectedAssetCode: "EUR",
			wantErr:           "",
		},
		{
			name:              "unsupported asset code for CIRCLE",
			stellarAssetCode:  "XYZ",
			expectedAssetCode: "",
			wantErr:           "unsupported asset code for CIRCLE: XYZ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PaymentRequest{
				StellarAssetCode: tt.stellarAssetCode,
			}

			actualAssetCode, actualErr := p.getCircleAssetCode()

			if tt.wantErr != "" {
				assert.ErrorContains(t, actualErr, tt.wantErr)
			} else {
				assert.NoError(t, actualErr)
			}

			assert.Equal(t, tt.expectedAssetCode, actualAssetCode)
		})
	}
}

func Test_PaymentRequest_Validate(t *testing.T) {
	validDestinationAddress := keypair.MustRandom().Address()

	tests := []struct {
		name       string
		paymentReq PaymentRequest
		wantErr    string
	}{
		{
			name: "missing source wallet ID",
			paymentReq: PaymentRequest{
				SourceWalletID:            "",
				DestinationStellarAddress: validDestinationAddress,
				Amount:                    "100.00",
				StellarAssetCode:          "USDC",
				IdempotencyKey:            uuid.New().String(),
			},
			wantErr: "source wallet ID is required",
		},
		{
			name: "invalid destination stellar address",
			paymentReq: PaymentRequest{
				SourceWalletID:            "source_wallet_123",
				DestinationStellarAddress: "invalid_address",
				Amount:                    "100.00",
				StellarAssetCode:          "USDC",
				IdempotencyKey:            uuid.New().String(),
			},
			wantErr: "destination stellar address is not a valid public key",
		},
		{
			name: "invalid amount",
			paymentReq: PaymentRequest{
				SourceWalletID:            "source_wallet_123",
				DestinationStellarAddress: validDestinationAddress,
				Amount:                    "invalid_amount",
				StellarAssetCode:          "USDC",
				IdempotencyKey:            uuid.New().String(),
			},
			wantErr: "amount is not valid",
		},
		{
			name: "missing stellar asset code",
			paymentReq: PaymentRequest{
				SourceWalletID:            "source_wallet_123",
				DestinationStellarAddress: validDestinationAddress,
				Amount:                    "100.00",
				StellarAssetCode:          "",
				IdempotencyKey:            uuid.New().String(),
			},
			wantErr: "stellar asset code is required",
		},
		{
			name: "invalid idempotency key",
			paymentReq: PaymentRequest{
				SourceWalletID:            "source_wallet_123",
				DestinationStellarAddress: validDestinationAddress,
				Amount:                    "100.00",
				StellarAssetCode:          "USDC",
				IdempotencyKey:            "invalid_uuid",
			},
			wantErr: "idempotency key is not valid",
		},
		{
			name: "valid payment request",
			paymentReq: PaymentRequest{
				SourceWalletID:            "source_wallet_123",
				DestinationStellarAddress: validDestinationAddress,
				Amount:                    "100.00",
				StellarAssetCode:          "USDC",
				IdempotencyKey:            uuid.New().String(),
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.paymentReq.Validate()

			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
