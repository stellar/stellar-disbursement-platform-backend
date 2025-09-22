package bridge

import "time"

// CustomerInfo represents customer information from the Bridge API.
type CustomerInfo struct {
	ID        string         `json:"id"`
	Email     string         `json:"email"`
	FirstName string         `json:"first_name"`
	LastName  string         `json:"last_name"`
	Type      CustomerType   `json:"type"`
	Status    CustomerStatus `json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// CustomerStatus represents the status of a customer in the Bridge system.
type CustomerStatus string

// Constants for CustomerStatus
const (
	CustomerStatusActive                CustomerStatus = "active"
	CustomerStatusAwaitingQuestionnaire CustomerStatus = "awaiting_questionnaire"
	CustomerStatusAwaitingUBO           CustomerStatus = "awaiting_ubo"
	CustomerStatusIncomplete            CustomerStatus = "incomplete"
	CustomerStatusNotStarted            CustomerStatus = "not_started"
	CustomerStatusOffboarded            CustomerStatus = "offboarded"
	CustomerStatusPaused                CustomerStatus = "paused"
	CustomerStatusRejected              CustomerStatus = "rejected"
	CustomerStatusUnderReview           CustomerStatus = "under_review"
)

// CustomerType represents the type of KYC verification
type CustomerType string

const (
	CustomerTypeIndividual CustomerType = "individual"
	CustomerTypeBusiness   CustomerType = "business"
)
