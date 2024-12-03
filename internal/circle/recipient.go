package circle

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/stellar/go/strkey"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type Recipient struct {
	ID         string            `json:"id"`
	Chain      string            `json:"chain"`
	Address    string            `json:"address"`
	Metadata   RecipientMetadata `json:"metadata"`
	Status     string            `json:"status"`
	CreateDate string            `json:"createDate"`
	UpdateDate string            `json:"updateDate"`
}

type RecipientMetadata struct {
	Nickname string `json:"nickname"`
	Email    string `json:"email"`
	// BNS stands for Blockchain Name Service (e.g. ENS) domain for the address.
	BNS string `json:"bns"`
}

type RecipientRequest struct {
	IdempotencyKey string            `json:"idempotencyKey"`
	Address        string            `json:"address"`
	Chain          string            `json:"chain"`
	Metadata       RecipientMetadata `json:"metadata"`
}

func (abr *RecipientRequest) validate() error {
	if abr.IdempotencyKey == "" {
		return errors.New("idempotency key must be provided")
	}
	if _, err := uuid.Parse(abr.IdempotencyKey); err != nil {
		return errors.New("idempotency key is not a valid UUID")
	}

	if abr.Address == "" {
		return errors.New("address must be provided")
	}
	if !strkey.IsValidEd25519PublicKey(abr.Address) {
		return errors.New("address is not a valid Stellar public key")
	}

	if abr.Chain != "" && abr.Chain != StellarChainCode {
		return fmt.Errorf("invalid chain provided %q", abr.Chain)
	} else if abr.Chain == "" {
		abr.Chain = StellarChainCode
	}

	if utils.IsEmpty(abr.Metadata) {
		return errors.New("metadata must be provided")
	}

	if abr.Metadata.Nickname == "" {
		return errors.New("metadata nickname must be provided")
	}

	if abr.Metadata.Email != "" {
		if err := utils.ValidateEmail(abr.Metadata.Email); err != nil {
			return errors.New("metadata email is not a valid email")
		}
	}

	return nil
}

// RecipientResponse represents the response from the Circle APIs.
type RecipientResponse struct {
	Data Recipient `json:"data"`
}

// parseRecipientResponse parses the response from the Circle APIs.
func parseRecipientResponse(resp *http.Response) (*Recipient, error) {
	var recipientResponse RecipientResponse
	if err := json.NewDecoder(resp.Body).Decode(&recipientResponse); err != nil {
		return nil, fmt.Errorf("decoding recipient response: %w", err)
	}

	return &recipientResponse.Data, nil
}
