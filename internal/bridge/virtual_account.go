package bridge

import "fmt"

// VirtualAccountInfo represents the response from creating/retrieving a virtual account
type VirtualAccountInfo struct {
	ID                        string                            `json:"id"`
	Status                    VirtualAccountStatus              `json:"status"`
	DeveloperFeePercent       string                            `json:"developer_fee_percent"`
	CustomerID                string                            `json:"customer_id"`
	SourceDepositInstructions VirtualAccountDepositInstructions `json:"source_deposit_instructions"`
	Destination               VirtualAccountDestination         `json:"destination"`
}

// VirtualAccountStatus represents the status of a virtual account.
type VirtualAccountStatus string

const (
	VirtualAccountActivated   VirtualAccountStatus = "activated"
	VirtualAccountDeactivated VirtualAccountStatus = "deactivated"
)

// VirtualAccountDepositInstructions represents bank deposit instructions for a virtual account
type VirtualAccountDepositInstructions struct {
	BankBeneficiaryName string   `json:"bank_beneficiary_name"`
	Currency            string   `json:"currency"`
	BankName            string   `json:"bank_name"`
	BankAddress         string   `json:"bank_address"`
	BankAccountNumber   string   `json:"bank_account_number"`
	BankRoutingNumber   string   `json:"bank_routing_number"`
	PaymentRails        []string `json:"payment_rails"`
}

// VirtualAccountDestination represents the destination configuration for a virtual account
type VirtualAccountDestination struct {
	PaymentRail    string `json:"payment_rail"`
	Currency       string `json:"currency"`
	Address        string `json:"address"`
	BlockchainMemo string `json:"blockchain_memo,omitempty"`
}

// VirtualAccountRequest represents the request payload for creating a virtual account
type VirtualAccountRequest struct {
	Source      VirtualAccountSource      `json:"source"`
	Destination VirtualAccountDestination `json:"destination"`
}

// VirtualAccountSource represents the source configuration for a virtual account
type VirtualAccountSource struct {
	Currency string `json:"currency"`
}

// Validate validates the virtual account request
func (r VirtualAccountRequest) Validate() error {
	if r.Source.Currency == "" {
		return fmt.Errorf("source currency is required")
	}
	if r.Destination.PaymentRail == "" {
		return fmt.Errorf("destination payment_rail is required")
	}
	if r.Destination.Currency == "" {
		return fmt.Errorf("destination currency is required")
	}
	if r.Destination.Address == "" {
		return fmt.Errorf("destination address is required")
	}
	return nil
}
