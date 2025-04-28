package circle

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_PayoutRequest_validate(t *testing.T) {
	t.Parallel()

	idempotencyKey := uuid.NewString()
	source := TransferAccount{Type: TransferAccountTypeWallet, ID: "1014442536"}
	destination := TransferAccount{
		Type:  TransferAccountTypeAddressBook,
		Chain: StellarChainCode,
		ID:    uuid.NewString(),
	}

	testCases := []struct {
		name            string
		pr              *PayoutRequest
		wantErrContains string
	}{
		// IdempotencyKey:
		{
			name:            "🔴IdempotencyKey is required",
			pr:              &PayoutRequest{},
			wantErrContains: "idempotency key must be provided",
		},
		{
			name: "🔴IdempotencyKey must be a valid uuid",
			pr: &PayoutRequest{
				IdempotencyKey: "invalid-idempotency-key",
			},
			wantErrContains: "idempotency key is not a valid UUID",
		},
		// Source:
		{
			name:            "🔴Source.Type is not provided",
			pr:              &PayoutRequest{IdempotencyKey: idempotencyKey},
			wantErrContains: "source type must be provided",
		},
		{
			name: "🔴Source.Type is not wallet",
			pr: &PayoutRequest{
				IdempotencyKey: idempotencyKey,
				Source:         TransferAccount{Type: TransferAccountTypeBlockchain},
			},
			wantErrContains: "source type must be wallet",
		},
		{
			name: "🔴Source.ID is not provided",
			pr: &PayoutRequest{
				IdempotencyKey: idempotencyKey,
				Source:         TransferAccount{Type: TransferAccountTypeWallet},
			},
			wantErrContains: "source ID must be provided for wallet transfers",
		},
		// Destination:
		{
			name: "🔴Destination.Type is not address book",
			pr: &PayoutRequest{
				IdempotencyKey: idempotencyKey,
				Source:         source,
				Destination:    TransferAccount{Type: TransferAccountTypeWallet},
			},
			wantErrContains: "destination type must be address_book",
		},
		{
			name: "🔴Destination.Chain is not XLM",
			pr: &PayoutRequest{
				IdempotencyKey: idempotencyKey,
				Source:         source,
				Destination: TransferAccount{
					Type:  TransferAccountTypeAddressBook,
					Chain: "FOO",
				},
			},
			wantErrContains: `invalid destination chain provided "FOO"`,
		},
		{
			name: "🔴Destination.ID must be provided",
			pr: &PayoutRequest{
				IdempotencyKey: idempotencyKey,
				Source:         source,
				Destination: TransferAccount{
					Type:  TransferAccountTypeAddressBook,
					Chain: StellarChainCode,
					ID:    "",
				},
			},
			wantErrContains: "destination ID must be provided",
		},
		// Amount:
		{
			name: "🔴Amount.Currency must be provided",
			pr: &PayoutRequest{
				IdempotencyKey: idempotencyKey,
				Source:         source,
				Destination:    destination,
			},
			wantErrContains: "currency must be provided",
		},
		{
			name: "🔴Amount.Amount must be provided",
			pr: &PayoutRequest{
				IdempotencyKey: idempotencyKey,
				Source:         source,
				Destination:    destination,
				Amount:         Balance{Currency: "USD"},
			},
			wantErrContains: "amount cannot be empty",
		},
		{
			name: "🔴Amount.Amount must be valid",
			pr: &PayoutRequest{
				IdempotencyKey: idempotencyKey,
				Source:         source,
				Destination:    destination,
				Amount:         Balance{Currency: "USD", Amount: "not-a-number"},
			},
			wantErrContains: "the provided amount is not a valid number",
		},
		// ToAmount:
		{
			name: "🔴ToAmount.Currency must be provided",
			pr: &PayoutRequest{
				IdempotencyKey: idempotencyKey,
				Source:         source,
				Destination:    destination,
				Amount:         Balance{Currency: "USD", Amount: "1"},
			},
			wantErrContains: "toAmount.currency must be provided",
		},
		// 🎉 Successful:
		{
			name: "🟢valid without chain",
			pr: &PayoutRequest{
				IdempotencyKey: idempotencyKey,
				Source:         source,
				Destination:    TransferAccount{Type: destination.Type, ID: destination.ID, AddressTag: destination.AddressTag},
				Amount:         Balance{Currency: "USD", Amount: "1"},
				ToAmount:       ToAmount{Currency: "USD"},
			},
		},
		{
			name: "🟢valid with chain",
			pr: &PayoutRequest{
				IdempotencyKey: idempotencyKey,
				Source:         source,
				Destination:    destination,
				Amount:         Balance{Currency: "USD", Amount: "1"},
				ToAmount:       ToAmount{Currency: "USD"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.pr.validate()
			if tc.wantErrContains == "" {
				require.NoError(t, err)
				assert.Equal(t, StellarChainCode, tc.pr.Destination.Chain)
			} else {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			}
		})
	}
}
