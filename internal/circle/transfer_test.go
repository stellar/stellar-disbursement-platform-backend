package circle

import (
	"errors"
	"testing"

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
			tr:      TransferRequest{Source: TransferEndpoint{Type: TransferEndpointTypeBlockchain}},
			wantErr: errors.New("source type must be wallet"),
		},
		{
			name: "source ID is not provided",
			tr: TransferRequest{
				Source: TransferEndpoint{Type: TransferEndpointTypeWallet},
			},
			wantErr: errors.New("source ID must be provided for wallet transfers"),
		},
		{
			name: "destination type is not blockchain",
			tr: TransferRequest{
				Source:      TransferEndpoint{Type: TransferEndpointTypeWallet, ID: "1014442536"},
				Destination: TransferEndpoint{Type: TransferEndpointTypeWallet},
			},
			wantErr: errors.New("destination type must be blockchain"),
		},
		{
			name: "destination chain is not XLM",
			tr: TransferRequest{
				Source:      TransferEndpoint{Type: TransferEndpointTypeWallet, ID: "1014442536"},
				Destination: TransferEndpoint{Type: TransferEndpointTypeBlockchain},
			},
			wantErr: errors.New("destination chain must be Stellar (XLM)"),
		},
		{
			name: "destination address is not provided",
			tr: TransferRequest{
				Source:      TransferEndpoint{Type: TransferEndpointTypeWallet, ID: "1014442536"},
				Destination: TransferEndpoint{Type: TransferEndpointTypeBlockchain, Chain: "XLM"},
			},
			wantErr: errors.New("destination address must be provided"),
		},
		{
			name: "currency is not provided",
			tr: TransferRequest{
				Source:      TransferEndpoint{Type: TransferEndpointTypeWallet, ID: "1014442536"},
				Destination: TransferEndpoint{Type: TransferEndpointTypeBlockchain, Chain: "XLM", Address: "GBG2DFASN2E5ZZSOYH7SJ7HWBKR4M5LYQ5Q5ZVBWS3RI46GDSYTEA6YF"},
			},
			wantErr: errors.New("currency must be provided"),
		},
		{
			name: "amount is not a valid number",
			tr: TransferRequest{
				Source:      TransferEndpoint{Type: TransferEndpointTypeWallet, ID: "1014442536"},
				Destination: TransferEndpoint{Type: TransferEndpointTypeBlockchain, Chain: "XLM", Address: "GBG2DFASN2E5ZZSOYH7SJ7HWBKR4M5LYQ5Q5ZVBWS3RI46GDSYTEA6YF"},
				Amount:      Money{Amount: "invalid", Currency: "USD"},
			},
			wantErr: errors.New("amount must be a valid number"),
		},
		{
			name: "valid transfer request",
			tr: TransferRequest{
				Source:      TransferEndpoint{Type: TransferEndpointTypeWallet, ID: "1014442536"},
				Destination: TransferEndpoint{Type: TransferEndpointTypeBlockchain, Chain: "XLM", Address: "GBG2DFASN2E5ZZSOYH7SJ7HWBKR4M5LYQ5Q5ZVBWS3RI46GDSYTEA6YF"},
				Amount:      Money{Amount: "0.25", Currency: "USD"},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.tr.validate()
			if err != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			}
		})
	}
}
