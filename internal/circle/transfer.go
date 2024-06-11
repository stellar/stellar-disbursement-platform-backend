package circle

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Transfer represents a transfer of funds from a Circle wallet to a blockchain address, from a blockchain address to a Circle wallet, or between two Circle wallets
type Transfer struct {
	ID              string           `json:"id"`
	Source          TransferEndpoint `json:"source"`
	Destination     TransferEndpoint `json:"destination"`
	Amount          Money            `json:"amount"`
	TransactionHash string           `json:"transactionHash,omitempty"`
	Status          string           `json:"status"`
	CreateDate      time.Time        `json:"createDate"`
}

// TransferEndpointType represents the type of the source
type TransferEndpointType string

const (
	TransferEndpointTypeCard       TransferEndpointType = "card"
	TransferEndpointTypeWire       TransferEndpointType = "wire"
	TransferEndpointTypeBlockchain TransferEndpointType = "blockchain"
	TransferEndpointTypeWallet     TransferEndpointType = "wallet"
)

// TransferEndpoint represents the source or destination of the transfer
type TransferEndpoint struct {
	Type       TransferEndpointType `json:"type"`
	ID         string               `json:"id,omitempty"`
	Chain      string               `json:"chain,omitempty"`
	Address    string               `json:"address,omitempty"`
	AddressTag string               `json:"addressTag,omitempty"`
}

// Money represents the amount transferred between source and destination
type Money struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

// TransferResponse represents the response from the Circle APIs
type TransferResponse struct {
	Data Transfer `json:"data"`
}

// TransferRequest represents the request to create a new transfer
type TransferRequest struct {
	Source         TransferEndpoint `json:"source"`
	Destination    TransferEndpoint `json:"destination"`
	Amount         Money            `json:"amount"`
	IdempotencyKey string           `json:"idempotencyKey"`
}

func (tr TransferRequest) validate() error {
	if tr.Source.Type == "" {
		return fmt.Errorf("source type must be provided")
	}

	if tr.Source.Type != TransferEndpointTypeWallet {
		return fmt.Errorf("source type must be wallet")
	}

	if tr.Source.ID == "" {
		return fmt.Errorf("source ID must be provided for wallet transfers")
	}

	if tr.Destination.Type != TransferEndpointTypeBlockchain {
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
