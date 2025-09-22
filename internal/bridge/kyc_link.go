package bridge

import (
	"fmt"
	"strings"
)

// KYCLinkInfo represents the response from creating a KYC link
type KYCLinkInfo struct {
	ID               string       `json:"id"`
	FullName         string       `json:"full_name"`
	Email            string       `json:"email"`
	Type             CustomerType `json:"type"`
	KYCLink          string       `json:"kyc_link"`
	TOSLink          string       `json:"tos_link"`
	KYCStatus        KYCStatus    `json:"kyc_status"`
	TOSStatus        TOSStatus    `json:"tos_status"`
	RejectionReasons []string     `json:"rejection_reasons,omitempty"`
	CreatedAt        string       `json:"created_at,omitempty"`
	CustomerID       string       `json:"customer_id"`
}

func (kycType CustomerType) Validate() error {
	switch CustomerType(strings.ToLower(string(kycType))) {
	case CustomerTypeIndividual, CustomerTypeBusiness:
		return nil
	default:
		return fmt.Errorf("invalid KYC type %s, must be either 'individual' or 'business'", kycType)
	}
}

// KYCStatus represents the status of KYC verification
type KYCStatus string

const (
	KYCStatusNotStarted  KYCStatus = "not_started"
	KYCStatusIncomplete  KYCStatus = "incomplete"
	KYCStatusAwaitingUBO KYCStatus = "awaiting_ubo"
	KYCStatusUnderReview KYCStatus = "under_review"
	KYCStatusApproved    KYCStatus = "approved"
	KYCStatusRejected    KYCStatus = "rejected"
	KYCStatusPaused      KYCStatus = "paused"
	KYCStatusOffboarded  KYCStatus = "offboarded"
)

// TOSStatus represents the status of Terms of Service acceptance.
type TOSStatus string

const (
	TOSStatusPending  TOSStatus = "pending"
	TOSStatusApproved TOSStatus = "approved"
)

// KYCLinkRequest represents the request payload for creating a KYC link
type KYCLinkRequest struct {
	FullName     string       `json:"full_name"`
	Email        string       `json:"email"`
	Type         CustomerType `json:"type"`
	Endorsements []string     `json:"endorsements,omitempty"`
	RedirectURI  string       `json:"redirect_uri,omitempty"`
}

// Validate validates the KYC link request
func (r KYCLinkRequest) Validate() error {
	if r.FullName == "" {
		return fmt.Errorf("full_name is required")
	}
	if r.Email == "" {
		return fmt.Errorf("email is required")
	}
	if r.Type == "" {
		return fmt.Errorf("type is required")
	}
	if r.Type != CustomerTypeIndividual && r.Type != CustomerTypeBusiness {
		return fmt.Errorf("type must be either 'individual' or 'business'")
	}
	return nil
}
