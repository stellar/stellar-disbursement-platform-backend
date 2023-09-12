package anchorplatform

import (
	"fmt"
	"strings"
	"time"
)

// GetTransactionsQueryParams are the query parameters that can be used in the `GET {PlatformAPIBaseURL}/transactions`
// request.
type GetTransactionsQueryParams struct {
	SEP        string   `schema:"sep,required,omitempty"`
	Order      string   `schema:"order,omitempty"`
	OrderBy    string   `schema:"order_by,omitempty"`
	PageNumber int      `schema:"page_number,omitempty"`
	PageSize   int      `schema:"page_size,omitempty"`
	Statuses   []string `schema:"statuses,omitempty"`
}

// APSep24TransactionRecords is a struct used for composing the HTTP body of a request or response to
// `{PlatformAPIBaseURL}/transactions`. It structures the body in the following format:
//
//		{
//		  "records": [
//	       {
//	         "transaction": {...}
//	       },
//	       ...
//	     ]
//		}
//
// The `records` field contains a slice of [APSep24TransactionWrapper], each wrapping an [APSep24Transaction].
type APSep24TransactionRecords struct {
	Records []APSep24TransactionWrapper `json:"records"`
}

// APSep24TransactionWrapper is a struct that wraps an [APSep24Transaction] for use in a request or response to
// `{PlatformAPIBaseURL}/transactions`. It structures the "transaction" field in the following format within a record:
//
//	{
//	  "transaction": {...}
//	}
//
// The `APSep24Transaction` field contains the transaction data.
type APSep24TransactionWrapper struct {
	APSep24Transaction `json:"transaction"`
}

// APSep24Transaction is the transaction object used in the `{PlatformAPIBaseURL}/transactions` requests.
type APSep24Transaction struct {
	ID          string `json:"id"`
	KYCVerified bool   `json:"kyc_verified,omitempty"`

	// Kind can be "deposit" or "withdrawal". It's a read-only field.
	Kind           string    `json:"kind,omitempty"`
	AmountExpected *APAmount `json:"amount_expected,omitempty"`

	SourceAccount      string `json:"source_account,omitempty"`
	DestinationAccount string `json:"destination_account,omitempty"`

	// These fields are patchable but they are already set by the AP, so I'm leaving them out of the patch:
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	Memo      string     `json:"memo,omitempty"`
	MemoType  string     `json:"memo_type,omitempty"`
	AmountIn  *APAmount  `json:"amount_in,omitempty"`

	// These fields are patchable:
	ExternalTransactionID string `json:"external_transaction_id,omitempty"`

	// Status
	SEP                 string                 `json:"sep,omitempty"`
	Status              APTransactionStatus    `json:"status,omitempty"`
	StellarTransactions []APStellarTransaction `json:"stellar_transactions,omitempty"`
	Message             string                 `json:"message,omitempty"`

	// Amounts
	AmountOut *APAmount `json:"amount_out,omitempty"`
	AmountFee *APAmount `json:"amount_fee,omitempty"`

	// Dates
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	TransferReceivedAt *time.Time `json:"transfer_received_at,omitempty"`
}

func NewAPSep24TransactionRecordsFromPatches(patches ...APSep24Transaction) APSep24TransactionRecords {
	var records APSep24TransactionRecords
	for _, patch := range patches {
		newEntry := APSep24TransactionWrapper{
			APSep24Transaction: patch,
		}
		records.Records = append(records.Records, newEntry)
	}

	return records
}

// type APSep24TransactionPostSuccess struct {
// 	ID                  string                 `json:"id"`
// 	SEP                 string                 `json:"sep,omitempty"`
// 	Status              APTransactionStatus    `json:"status,omitempty"` // Success
// 	StellarTransactions []APStellarTransaction `json:"stellar_transactions,omitempty"`
// 	Message             string                 `json:"message,omitempty"`
// 	CompletedAt         *time.Time             `json:"completed_at,omitempty"`
// 	AmountOut           APAmount               `json:"amount_out,omitempty"`
// }

// type APSep24TransactionPostError struct {
// 	ID                  string                 `json:"id"`
// 	SEP                 string                 `json:"sep,omitempty"`
// 	Status              APTransactionStatus    `json:"status,omitempty"` // Error
// 	StellarTransactions []APStellarTransaction `json:"stellar_transactions,omitempty"`
// 	Message             string                 `json:"message,omitempty"` // Error message
// }

// APTransactionStatus is the body of the Stellar transaction stored in the Anchor Platform.
type APStellarTransaction struct {
	ID        string             `json:"id"`
	Memo      string             `json:"memo,omitempty"`
	MemoType  string             `json:"memo_type,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
	Envelope  string             `json:"envelope"`
	Payments  []APStellarPayment `json:"payments,omitempty"`
}

// APStellarPayment is the body of the Stellar payment stored in the Anchor Platform.
type APStellarPayment struct {
	ID                 string   `json:"id"`
	PaymentType        string   `json:"payment_type"`
	SourceAccount      string   `json:"source_account"`
	DestinationAccount string   `json:"destination_account"`
	Amount             APAmount `json:"amount"`
}

// APAmount is the body of the Stellar amount stored in the Anchor Platform.
type APAmount struct {
	Amount string `json:"amount"`
	Asset  string `json:"asset"`
}

// NewAnchorPlatformStellarAsset creates a stellar asset using the [Asset Identification Format](https://stellar.org/protocol/sep-38#asset-identification-format)
func NewStellarAssetInAIF(assetCode, assetIssuer string) string {
	assetIssuer = strings.TrimSpace(assetIssuer)
	if assetIssuer != "" {
		assetIssuer = ":" + assetIssuer
	}
	return fmt.Sprintf("stellar:%s%s", strings.TrimSpace(assetCode), assetIssuer)
}
