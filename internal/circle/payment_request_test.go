package circle

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ParseAPIType(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		apiType string
		wantErr error
	}{
		{wantErr: fmt.Errorf(`invalid Circle API type "", must be one of [PAYOUTS TRANSFERS]`)},
		{apiType: "foo_BAR", wantErr: fmt.Errorf(`invalid Circle API type "FOO_BAR", must be one of [PAYOUTS TRANSFERS]`)},
		{apiType: "PAYOUTS"},
		{apiType: "TRANSFERS"},
		{apiType: "pAyOuTs"},
		{apiType: "tRaNsFeRs"},
	}

	for _, tc := range testCases {
		t.Run("apiType: "+tc.apiType, func(t *testing.T) {
			got, err := ParseAPIType(tc.apiType)
			if tc.wantErr != nil {
				assert.Equal(t, tc.wantErr, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, APIType(strings.ToUpper(tc.apiType)), got)
			}
		})
	}
}

func Test_PaymentRequest_GetCircleAssetCode(t *testing.T) {
	t.Parallel()

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

			actualAssetCode, actualErr := p.GetCircleAssetCode()

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
	t.Parallel()

	tests := []struct {
		name       string
		paymentReq PaymentRequest
		wantErr    string
	}{
		{
			name: "🔴 invalid API type",
			paymentReq: PaymentRequest{
				APIType:          "",
				SourceWalletID:   "source_wallet_123",
				RecipientID:      "recipient_id_123",
				Amount:           "100.00",
				StellarAssetCode: "USDC",
				IdempotencyKey:   uuid.New().String(),
			},
			wantErr: `invalid Circle API type "", must be one of [PAYOUTS TRANSFERS]`,
		},
		{
			name: "🔴 missing source wallet ID",
			paymentReq: PaymentRequest{
				APIType:          APITypePayouts,
				SourceWalletID:   "",
				Amount:           "100.00",
				StellarAssetCode: "USDC",
				IdempotencyKey:   uuid.New().String(),
			},
			wantErr: "source wallet ID is required",
		},
		{
			name: "🔴 missing recipient id",
			paymentReq: PaymentRequest{
				APIType:          APITypePayouts,
				SourceWalletID:   "source_wallet_123",
				Amount:           "100.00",
				StellarAssetCode: "USDC",
				IdempotencyKey:   uuid.New().String(),
			},
			wantErr: "recipient ID is required",
		},
		{
			name: "🔴 invalid amount",
			paymentReq: PaymentRequest{
				APIType:          APITypePayouts,
				SourceWalletID:   "source_wallet_123",
				RecipientID:      "recipient_id_123",
				Amount:           "invalid_amount",
				StellarAssetCode: "USDC",
				IdempotencyKey:   uuid.New().String(),
			},
			wantErr: "amount is not valid",
		},
		{
			name: "🔴 missing stellar asset code",
			paymentReq: PaymentRequest{
				APIType:          APITypePayouts,
				SourceWalletID:   "source_wallet_123",
				RecipientID:      "recipient_id_123",
				Amount:           "100.00",
				StellarAssetCode: "",
				IdempotencyKey:   uuid.New().String(),
			},
			wantErr: "stellar asset code is required",
		},
		{
			name: "🔴 invalid idempotency key",
			paymentReq: PaymentRequest{
				APIType:          APITypePayouts,
				SourceWalletID:   "source_wallet_123",
				RecipientID:      "recipient_id_123",
				Amount:           "100.00",
				StellarAssetCode: "USDC",
				IdempotencyKey:   "invalid_uuid",
			},
			wantErr: "idempotency key is not valid",
		},
		{
			name: "🔴 invalid destination stellar address for transfers",
			paymentReq: PaymentRequest{
				APIType:                   APITypeTransfers,
				SourceWalletID:            "source_wallet_123",
				RecipientID:               "recipient_id_123",
				Amount:                    "100.00",
				StellarAssetCode:          "USDC",
				IdempotencyKey:            uuid.New().String(),
				DestinationStellarAddress: "invalid-stellar-address",
			},
			wantErr: "destination stellar address is not a valid public key",
		},
		{
			name: "🟢 valid payout payment request",
			paymentReq: PaymentRequest{
				APIType:          APITypePayouts,
				SourceWalletID:   "source_wallet_123",
				RecipientID:      "recipient_id_123",
				Amount:           "100.00",
				StellarAssetCode: "USDC",
				IdempotencyKey:   uuid.New().String(),
			},
			wantErr: "",
		},
		{
			name: "🟢 valid transfer payment request",
			paymentReq: PaymentRequest{
				APIType:                   APITypeTransfers,
				SourceWalletID:            "source_wallet_123",
				DestinationStellarAddress: "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
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
