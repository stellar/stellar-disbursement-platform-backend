package circle

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Transfer represents a transfer of funds from a Circle Endpoint to another. A circle endpoint can be a wallet, card, wire, or blockchain address.
type Transfer struct {
	ID              string          `json:"id"`
	Source          TransferAccount `json:"source"`
	Destination     TransferAccount `json:"destination"`
	Amount          Money           `json:"amount"`
	TransactionHash string          `json:"transactionHash,omitempty"`
	Status          string          `json:"status"`
	CreateDate      time.Time       `json:"createDate"`
}

// TransferAccountType represents the type of the source or destination of the transfer.
type TransferAccountType string

const (
	TransferAccountTypeCard       TransferAccountType = "card"
	TransferAccountTypeWire       TransferAccountType = "wire"
	TransferAccountTypeBlockchain TransferAccountType = "blockchain"
	TransferAccountTypeWallet     TransferAccountType = "wallet"
)

// TransferAccount represents the source or destination of the transfer.
type TransferAccount struct {
	Type       TransferAccountType `json:"type"`
	ID         string              `json:"id,omitempty"`
	Chain      string              `json:"chain,omitempty"`
	Address    string              `json:"address,omitempty"`
	AddressTag string              `json:"addressTag,omitempty"`
}

// Money represents the amount transferred between source and destination.
type Money struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

// TransferResponse represents the response from the Circle APIs.
type TransferResponse struct {
	Data Transfer `json:"data"`
}

// TransferRequest represents the request to create a new transfer.
type TransferRequest struct {
	Source         TransferAccount `json:"source"`
	Destination    TransferAccount `json:"destination"`
	Amount         Money           `json:"amount"`
	IdempotencyKey string          `json:"idempotencyKey"`
}

func (tr TransferRequest) validate() error {
	if tr.Source.Type == "" {
		return fmt.Errorf("source type must be provided")
	}

	if tr.Source.Type != TransferAccountTypeWallet {
		return fmt.Errorf("source type must be wallet")
	}

	if tr.Source.ID == "" {
		return fmt.Errorf("source ID must be provided for wallet transfers")
	}

	if tr.Destination.Type != TransferAccountTypeBlockchain {
		return fmt.Errorf("destination type must be blockchain")
	}

	if tr.Destination.Chain != "XLM" {
		return fmt.Errorf("destination chain must be Stellar (XLM)")
	}

	if tr.Destination.Address == "" {
		return fmt.Errorf("destination address must be provided")
	}

	if tr.Amount.Currency == "" {
		return fmt.Errorf("currency must be provided")
	}

	if tr.Amount.Amount == "" {
		return fmt.Errorf("amount must be provided")
	}

	if tr.IdempotencyKey == "" {
		return fmt.Errorf("idempotency key must be provided")
	}

	if _, err := strconv.ParseFloat(tr.Amount.Amount, 64); err != nil {
		return fmt.Errorf("amount must be a valid number")
	}

	return nil
}

// parseTransferResponse parses the response from the Circle APIs
func parseTransferResponse(resp *http.Response) (*Transfer, error) {
	var transferResponse TransferResponse
	if err := json.NewDecoder(resp.Body).Decode(&transferResponse); err != nil {
		return nil, err
	}

	return &transferResponse.Data, nil
}
