package transaction

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// TestBuildPDF renders one notice per layout path and checks each builds. The row content itself
// is pinned by TestDetailRows; the PDF bytes are not inspected.
func TestBuildPDF(t *testing.T) {
	payoutType := data.CircleTransactionTypePayout
	minimal := &data.Payment{
		ID: "pay-1", Amount: "100.0000000", Status: data.DraftPaymentStatus, Type: data.PaymentTypeDisbursement,
		Asset:     data.Asset{Code: "USDC", Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"},
		CreatedAt: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 1, 15, 10, 5, 0, 0, time.UTC),
	}
	full := &data.Payment{
		ID: "pay-2", Amount: "50.5000000", StellarTransactionID: "tx-hash-456", Status: data.SuccessPaymentStatus, Type: data.PaymentTypeDisbursement,
		Asset: data.Asset{Code: "XLM"}, ExternalPaymentID: "ext-2", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		ReceiverWallet: &data.ReceiverWallet{
			StellarAddress: "GABCDEF1234567890ABCDEF1234567890ABCDEF12", StellarMemo: "memo-text-123",
			Receiver: data.Receiver{ID: "rec-1", ExternalID: "org-id-123"},
			Wallet:   data.Wallet{Name: "Vibrant Assist"},
		},
		Disbursement: &data.Disbursement{Name: "Q1 2026 Disbursement", StatusHistory: data.DisbursementStatusHistory{
			{Status: data.DraftDisbursementStatus, UserID: "user-draft", Timestamp: time.Now().Add(-24 * time.Hour)},
			{Status: data.StartedDisbursementStatus, UserID: "user-started", Timestamp: time.Now().Add(-time.Hour)},
		}},
		CircleTransactionID: utils.Ptr("payout-1"), CircleTransactionType: &payoutType,
	}
	enrichment := &Enrichment{
		SenderName: "Sender Org", SenderAccountName: "Main account", SenderAccountAddress: "GSENDER1234567890ABCDEF1234567890ABCDEF12",
		FeeCharged: "0.00001 XLM", MemoText: "horizon-memo", StellarExpertBaseURL: "https://stellar.expert/explorer/testnet/",
		DisbursementCreatedByUserName: "Alice", DisbursementCreatedByTimestamp: "Jan 1, 2026 · 10:00:00 UTC",
		DisbursementApprovedByUserName: "Bob", DisbursementApprovedByTimestamp: "Jan 2, 2026 · 10:00:00 UTC",
	}
	longNotes := strings.Repeat("x", internalNotesMaxLength)
	multilineNotes := "line one\nline two\n\nline four"

	testCases := []struct {
		name       string
		payment    *data.Payment
		enrichment *Enrichment
		notes      *string
		logo       []byte
	}{
		{name: "minimal draft payment, no enrichment"},
		{name: "success payment with every section filled", payment: full, enrichment: enrichment},
		{name: "internal notes at the maximum length", payment: full, enrichment: enrichment, notes: &longNotes},
		{name: "internal notes with line breaks", notes: &multilineNotes},
		{name: "organization logo", logo: []byte{0x89, 0x50, 0x4E, 0x47}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payment := tc.payment
			if payment == nil {
				payment = minimal
			}
			pdfBytes, err := BuildPDF(payment, "Test Organization", tc.logo, tc.enrichment, tc.notes, "example.com")
			require.NoError(t, err)
			assert.Greater(t, len(pdfBytes), 1000)
		})
	}
}
