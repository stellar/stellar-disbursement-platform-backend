package circle

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"
)

type RiskEvaluation struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

type Payout struct {
	ID              string            `json:"id"`
	Destination     TransferAccount   `json:"destination"`
	Amount          Balance           `json:"amount"`
	ToAmount        Balance           `json:"toAmount"`
	TransactionHash string            `json:"externalRef,omitempty"`
	CreateDate      string            `json:"createDate"`
	UpdateDate      string            `json:"updateDate"`
	SourceWalletID  string            `json:"sourceWalletId"`
	Fees            Balance           `json:"fees,omitempty"`
	Status          TransferStatus    `json:"status"`
	ErrorCode       TransferErrorCode `json:"errorCode,omitempty"`
	RiskEvaluation  RiskEvaluation    `json:"riskEvaluation,omitempty"`
}

type ToAmount struct {
	Currency string `json:"currency"`
}

// PayoutRequest represents the request to create a new transfer.
type PayoutRequest struct {
	IdempotencyKey string          `json:"idempotencyKey"`
	Source         TransferAccount `json:"source"`
	Destination    TransferAccount `json:"destination"`
	Amount         Balance         `json:"amount"`
	ToAmount       ToAmount        `json:"toAmount"`
}

func (pr *PayoutRequest) validate() error {
	if pr.IdempotencyKey == "" {
		return errors.New("idempotency key must be provided")
	}
	if _, err := uuid.Parse(pr.IdempotencyKey); err != nil {
		return errors.New("idempotency key is not a valid UUID")
	}

	if pr.Source.Type == "" {
		return fmt.Errorf("source type must be provided")
	}
	if pr.Source.Type != TransferAccountTypeWallet {
		return fmt.Errorf("source type must be wallet")
	}
	if pr.Source.ID == "" {
		return fmt.Errorf("source ID must be provided for wallet transfers")
	}

	if pr.Destination.Type != TransferAccountTypeAddressBook {
		return fmt.Errorf("destination type must be address_book")
	}
	if pr.Destination.Chain != "" && pr.Destination.Chain != StellarChainCode {
		return fmt.Errorf("invalid destination chain provided %q", pr.Destination.Chain)
	} else if pr.Destination.Chain == "" {
		pr.Destination.Chain = StellarChainCode
	}
	if pr.Destination.ID == "" {
		return fmt.Errorf("destination ID must be provided")
	}

	if pr.Amount.Currency == "" {
		return fmt.Errorf("currency must be provided")
	}
	if pr.Amount.Amount == "" {
		return fmt.Errorf("amount must be provided")
	}
	if _, err := strconv.ParseFloat(pr.Amount.Amount, 64); err != nil {
		return fmt.Errorf("amount must be a valid number")
	}

	if pr.ToAmount.Currency == "" {
		return fmt.Errorf("toAmount.currency must be provided")
	}

	return nil
}

// PayoutResponse represents the response from the Circle APIs.
type PayoutResponse struct {
	Data Payout `json:"data"`
}

// parsePayoutResponse parses the response from the Circle APIs.
func parsePayoutResponse(resp *http.Response) (*Payout, error) {
	var payoutResponse PayoutResponse
	if err := json.NewDecoder(resp.Body).Decode(&payoutResponse); err != nil {
		return nil, fmt.Errorf("decoding transfer response: %w", err)
	}

	return &payoutResponse.Data, nil
}
