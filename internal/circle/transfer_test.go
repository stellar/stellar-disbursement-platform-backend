package circle

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func Test_TransferRequest_validate(t *testing.T) {
	tests := []struct {
		name    string
		tr      TransferRequest
		wantErr error
	}{
		{
			name:    "source type is not provided",
			tr:      TransferRequest{},
			wantErr: errors.New("source type must be provided"),
		},
		{
			name:    "source type is not wallet",
			tr:      TransferRequest{Source: TransferAccount{Type: TransferAccountTypeBlockchain}},
			wantErr: errors.New("source type must be wallet"),
		},
		{
			name: "source ID is not provided",
			tr: TransferRequest{
				Source: TransferAccount{Type: TransferAccountTypeWallet},
			},
			wantErr: errors.New("source ID must be provided for wallet transfers"),
		},
		{
			name: "destination type is not blockchain",
			tr: TransferRequest{
				Source:      TransferAccount{Type: TransferAccountTypeWallet, ID: "1014442536"},
				Destination: TransferAccount{Type: TransferAccountTypeWallet},
			},
			wantErr: errors.New("destination type must be blockchain"),
		},
		{
			name: "destination chain is not XLM",
			tr: TransferRequest{
				Source:      TransferAccount{Type: TransferAccountTypeWallet, ID: "1014442536"},
				Destination: TransferAccount{Type: TransferAccountTypeBlockchain},
			},
			wantErr: errors.New("destination chain must be Stellar (XLM)"),
		},
		{
			name: "destination address is not provided",
			tr: TransferRequest{
				Source:      TransferAccount{Type: TransferAccountTypeWallet, ID: "1014442536"},
				Destination: TransferAccount{Type: TransferAccountTypeBlockchain, Chain: "XLM"},
			},
			wantErr: errors.New("destination address must be provided"),
		},
		{
			name: "currency is not provided",
			tr: TransferRequest{
				Source:      TransferAccount{Type: TransferAccountTypeWallet, ID: "1014442536"},
				Destination: TransferAccount{Type: TransferAccountTypeBlockchain, Chain: "XLM", Address: "GBG2DFASN2E5ZZSOYH7SJ7HWBKR4M5LYQ5Q5ZVBWS3RI46GDSYTEA6YF"},
			},
			wantErr: errors.New("currency must be provided"),
		},
		{
			name: "amount is not a valid number",
			tr: TransferRequest{
				Source:         TransferAccount{Type: TransferAccountTypeWallet, ID: "1014442536"},
				Destination:    TransferAccount{Type: TransferAccountTypeBlockchain, Chain: "XLM", Address: "GBG2DFASN2E5ZZSOYH7SJ7HWBKR4M5LYQ5Q5ZVBWS3RI46GDSYTEA6YF"},
				Amount:         Money{Amount: "invalid", Currency: "USD"},
				IdempotencyKey: uuid.NewString(),
			},
			wantErr: errors.New("amount must be a valid number"),
		},
		{
			name: "idempotency key is not provided",
			tr: TransferRequest{
				Source:      TransferAccount{Type: TransferAccountTypeWallet, ID: "1014442536"},
				Destination: TransferAccount{Type: TransferAccountTypeBlockchain, Chain: "XLM", Address: "GBG2DFASN2E5ZZSOYH7SJ7HWBKR4M5LYQ5Q5ZVBWS3RI46GDSYTEA6YF"},
				Amount:      Money{Amount: "0.25", Currency: "USD"},
			},
			wantErr: nil,
		},
		{
			name: "valid transfer request",
			tr: TransferRequest{
				IdempotencyKey: uuid.NewString(),
				Source:         TransferAccount{Type: TransferAccountTypeWallet, ID: "1014442536"},
				Destination:    TransferAccount{Type: TransferAccountTypeBlockchain, Chain: "XLM", Address: "GBG2DFASN2E5ZZSOYH7SJ7HWBKR4M5LYQ5Q5ZVBWS3RI46GDSYTEA6YF"},
				Amount:         Money{Amount: "0.25", Currency: "USD"},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.tr.validate()
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			}
		})
	}
}
