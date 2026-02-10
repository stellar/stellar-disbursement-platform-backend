package transaction

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

func TestBuildPDF(t *testing.T) {
	orgName := "Test Organization"
	orgLogo := []byte{}
	operatedByBaseURL := "https://example.com"

	t.Run("successfully generates PDF with minimal payment", func(t *testing.T) {
		payment := &data.Payment{
			ID:                   "pay-1",
			Amount:               "100.0000000",
			StellarTransactionID: "tx-hash-123",
			StellarOperationID:   "op-456",
			Status:               data.SuccessPaymentStatus,
			Type:                 data.PaymentTypeDisbursement,
			Asset: data.Asset{
				ID:     "asset-1",
				Code:   "USDC",
				Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN",
			},
			ExternalPaymentID: "ext-1",
			CreatedAt:         time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
			UpdatedAt:         time.Date(2026, 1, 15, 10, 5, 0, 0, time.UTC),
		}

		pdfBytes, err := BuildPDF(payment, orgName, orgLogo, nil, nil, operatedByBaseURL)

		require.NoError(t, err)
		assert.NotEmpty(t, pdfBytes)
		assert.Greater(t, len(pdfBytes), 1000, "PDF should be substantial size")
	})

	t.Run("successfully generates PDF with receiver wallet and enrichment", func(t *testing.T) {
		payment := &data.Payment{
			ID:                   "pay-2",
			Amount:               "50.5000000",
			StellarTransactionID: "tx-hash-456",
			StellarOperationID:   "op-789",
			Status:               data.SuccessPaymentStatus,
			Type:                 data.PaymentTypeDirect,
			Asset: data.Asset{
				ID:     "asset-2",
				Code:   "XLM",
				Issuer: "",
			},
			ReceiverWallet: &data.ReceiverWallet{
				ID:             "rw-1",
				StellarAddress: "GABCDEF1234567890ABCDEF1234567890ABCDEF12",
				StellarMemo:    "memo-text-123",
				Receiver: data.Receiver{
					ID:         "rec-1",
					ExternalID: "org-id-123",
				},
				Wallet: data.Wallet{
					ID:   "wal-1",
					Name: "Vibrant Assist",
				},
			},
			ExternalPaymentID: "ext-2",
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		}

		enrichment := &Enrichment{
			SenderName:           "Sender Org",
			SenderWalletAddress:  "GSENDER1234567890ABCDEF1234567890ABCDEF12",
			FeeCharged:           "0.00001 XLM",
			StellarExpertBaseURL: GetStellarExpertBaseURL(),
		}

		pdfBytes, err := BuildPDF(payment, orgName, orgLogo, enrichment, nil, operatedByBaseURL)

		require.NoError(t, err)
		assert.NotEmpty(t, pdfBytes)
	})

	t.Run("successfully generates PDF with internal notes", func(t *testing.T) {
		payment := &data.Payment{
			ID:                   "pay-3",
			Amount:               "25.0000000",
			StellarTransactionID: "tx-notes",
			Status:               data.SuccessPaymentStatus,
			Type:                 data.PaymentTypeDisbursement,
			Asset:                data.Asset{Code: "USDC"},
			ExternalPaymentID:    "ext-3",
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}

		internalNotes := "Internal reference note for this transaction."
		pdfBytes, err := BuildPDF(payment, orgName, orgLogo, nil, &internalNotes, operatedByBaseURL)

		require.NoError(t, err)
		assert.NotEmpty(t, pdfBytes)
	})

	t.Run("successfully generates PDF with disbursement details section", func(t *testing.T) {
		payment := &data.Payment{
			ID:                   "pay-4",
			Amount:               "75.0000000",
			StellarTransactionID: "tx-disp",
			Status:               data.SuccessPaymentStatus,
			Type:                 data.PaymentTypeDisbursement,
			Asset:                data.Asset{Code: "USDC"},
			Disbursement: &data.Disbursement{
				ID:   "disp-1",
				Name: "Q1 2026 Disbursement",
				StatusHistory: data.DisbursementStatusHistory{
					{Status: data.DraftDisbursementStatus, UserID: "user-draft", Timestamp: time.Now().Add(-24 * time.Hour)},
					{Status: data.ReadyDisbursementStatus, UserID: "user-ready", Timestamp: time.Now().Add(-1 * time.Hour)},
				},
			},
			ExternalPaymentID: "ext-4",
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		}

		enrichment := &Enrichment{
			DisbursementCreatedByUserName:   "Alice",
			DisbursementCreatedByTimestamp:  "Jan 9, 2026 · 10:00:00 UTC",
			DisbursementApprovedByUserName:  "Bob",
			DisbursementApprovedByTimestamp: "Jan 10, 2026 · 14:30:00 UTC",
		}

		pdfBytes, err := BuildPDF(payment, orgName, orgLogo, enrichment, nil, operatedByBaseURL)

		require.NoError(t, err)
		assert.NotEmpty(t, pdfBytes)
	})

	t.Run("handles nil internal notes pointer", func(t *testing.T) {
		payment := &data.Payment{
			ID:                "pay-5",
			Amount:            "10.0000000",
			Status:            data.SuccessPaymentStatus,
			Type:              data.PaymentTypeDirect,
			Asset:             data.Asset{Code: "XLM"},
			ExternalPaymentID: "ext-5",
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		}

		pdfBytes, err := BuildPDF(payment, orgName, orgLogo, nil, nil, operatedByBaseURL)

		require.NoError(t, err)
		assert.NotEmpty(t, pdfBytes)
	})

	t.Run("handles empty internal notes string", func(t *testing.T) {
		payment := &data.Payment{
			ID:                "pay-6",
			Amount:            "10.0000000",
			Status:            data.SuccessPaymentStatus,
			Type:              data.PaymentTypeDirect,
			Asset:             data.Asset{Code: "XLM"},
			ExternalPaymentID: "ext-6",
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		}

		emptyNotes := ""
		pdfBytes, err := BuildPDF(payment, orgName, orgLogo, nil, &emptyNotes, operatedByBaseURL)

		require.NoError(t, err)
		assert.NotEmpty(t, pdfBytes)
	})

	t.Run("handles payment with SUCCESS status for green styling", func(t *testing.T) {
		payment := &data.Payment{
			ID:                   "pay-7",
			Amount:               "99.9900000",
			StellarTransactionID: "tx-success",
			Status:               data.SuccessPaymentStatus,
			Type:                 data.PaymentTypeDisbursement,
			Asset:                data.Asset{Code: "USDC"},
			ExternalPaymentID:    "ext-7",
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}

		pdfBytes, err := BuildPDF(payment, orgName, orgLogo, nil, nil, operatedByBaseURL)

		require.NoError(t, err)
		assert.NotEmpty(t, pdfBytes)
	})

	t.Run("handles organization logo", func(t *testing.T) {
		payment := &data.Payment{
			ID:                "pay-8",
			Amount:            "1.0000000",
			Status:            data.SuccessPaymentStatus,
			Type:              data.PaymentTypeDirect,
			Asset:             data.Asset{Code: "XLM"},
			ExternalPaymentID: "ext-8",
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		}

		logoPNG := []byte{0x89, 0x50, 0x4E, 0x47} // PNG header
		pdfBytes, err := BuildPDF(payment, orgName, logoPNG, nil, nil, operatedByBaseURL)

		require.NoError(t, err)
		assert.NotEmpty(t, pdfBytes)
	})
}
