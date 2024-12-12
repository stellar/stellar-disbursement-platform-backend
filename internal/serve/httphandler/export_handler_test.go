package httphandler

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

func Test_ExportHandler_ExportDisbursements(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	ctx := context.Background()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	handler := &ExportHandler{
		Models: models,
	}

	r := chi.NewRouter()
	r.Get("/exports/disbursements", handler.ExportDisbursements)

	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	disbursement1 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, handler.Models.Disbursements, &data.Disbursement{
		Name:      "disbursement 1",
		Status:    data.StartedDisbursementStatus,
		Wallet:    wallet,
		Asset:     asset,
		CreatedAt: time.Date(2023, 3, 21, 23, 40, 20, 1431, time.UTC),
	})

	disbursement2 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, handler.Models.Disbursements, &data.Disbursement{
		Name:      "disbursement 2",
		Status:    data.DraftDisbursementStatus,
		Wallet:    wallet,
		Asset:     asset,
		CreatedAt: time.Date(2022, 3, 21, 23, 40, 20, 1431, time.UTC),
	})

	disbursement3 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, handler.Models.Disbursements, &data.Disbursement{
		Name:      "disbursement 3",
		Status:    data.ReadyDisbursementStatus,
		Wallet:    wallet,
		Asset:     asset,
		CreatedAt: time.Date(2021, 3, 21, 23, 40, 20, 1431, time.UTC),
	})

	tests := []struct {
		name                  string
		queryParams           string
		expectedStatusCode    int
		expectedDisbursements []*data.Disbursement
	}{
		{
			name:                  "success - returns CSV with no disbursements",
			queryParams:           "status=completed",
			expectedStatusCode:    http.StatusOK,
			expectedDisbursements: []*data.Disbursement{},
		},
		{
			name:                  "success - returns CSV with all disbursements",
			expectedStatusCode:    http.StatusOK,
			expectedDisbursements: []*data.Disbursement{disbursement1, disbursement2, disbursement3},
		},
		{
			name:                  "success - return CSV with reverse order of disbursements",
			expectedStatusCode:    http.StatusOK,
			queryParams:           "sort=created_at&direction=asc",
			expectedDisbursements: []*data.Disbursement{disbursement3, disbursement2, disbursement1},
		},
		{
			name:                  "success - return CSV with only started disbursements",
			expectedStatusCode:    http.StatusOK,
			queryParams:           "status=started",
			expectedDisbursements: []*data.Disbursement{disbursement1},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url := "/exports/disbursements"
			if tc.queryParams != "" {
				url += "?" + tc.queryParams
			}
			req, err := http.NewRequest(http.MethodGet, url, nil)
			require.NoError(t, err)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatusCode, rr.Code)
			csvReader := csv.NewReader(strings.NewReader(rr.Body.String()))

			header, err := csvReader.Read()
			require.NoError(t, err)
			assert.Contains(t, header, "Name")
			assert.Contains(t, header, "Status")
			assert.Contains(t, header, "CreatedAt")

			assert.Equal(t, "text/csv", rr.Header().Get("Content-Type"))
			today := time.Now().Format("2006-01-02")
			assert.Contains(t, rr.Header().Get("Content-Disposition"), fmt.Sprintf("attachment; filename=disbursements_%s", today))

			rows, err := csvReader.ReadAll()
			require.NoError(t, err)
			assert.Len(t, rows, len(tc.expectedDisbursements))

			for i, row := range rows {
				assert.Equal(t, tc.expectedDisbursements[i].Name, row[1])
				assert.Equal(t, string(tc.expectedDisbursements[i].Status), row[5])
			}
		})
	}
}

func Test_ExportHandler_ExportPayments(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	ctx := context.Background()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	handler := &ExportHandler{
		Models: models,
	}

	r := chi.NewRouter()
	r.Get("/exports/payments", handler.ExportPayments)

	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	receiverReady := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	rwReady := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverReady.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, handler.Models.Disbursements, &data.Disbursement{
		Name:   "disbursement 1",
		Status: data.StartedDisbursementStatus,
		Wallet: wallet,
		Asset:  asset,
	})

	pendingPayment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet:    rwReady,
		Disbursement:      disbursement,
		Asset:             *asset,
		Amount:            "100",
		Status:            data.PendingPaymentStatus,
		CreatedAt:         time.Date(2024, 3, 21, 23, 40, 20, 1431, time.UTC),
		ExternalPaymentID: "PAY01",
	})
	successfulPayment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet:    rwReady,
		Disbursement:      disbursement,
		Asset:             *asset,
		Amount:            "200",
		Status:            data.SuccessPaymentStatus,
		CreatedAt:         time.Date(2023, 3, 21, 23, 40, 20, 1431, time.UTC),
		ExternalPaymentID: "PAY02",
	})
	failedPayment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet:    rwReady,
		Disbursement:      disbursement,
		Asset:             *asset,
		Amount:            "300",
		Status:            data.FailedPaymentStatus,
		CreatedAt:         time.Date(2022, 3, 21, 23, 40, 20, 1431, time.UTC),
		ExternalPaymentID: "PAY03",
	})

	tests := []struct {
		name               string
		queryParams        string
		expectedStatusCode int
		expectedPayments   []*data.Payment
	}{
		{
			name:               "success - returns CSV with no payments",
			queryParams:        "status=draft",
			expectedStatusCode: http.StatusOK,
			expectedPayments:   []*data.Payment{},
		},
		{
			name:               "success - returns CSV with all payments",
			expectedStatusCode: http.StatusOK,
			queryParams:        "sort=created_at",
			expectedPayments:   []*data.Payment{pendingPayment, successfulPayment, failedPayment},
		},
		{
			name:               "success - return CSV with reverse order of payments",
			expectedStatusCode: http.StatusOK,
			queryParams:        "sort=created_at&direction=asc",
			expectedPayments:   []*data.Payment{failedPayment, successfulPayment, pendingPayment},
		},
		{
			name:               "success - return CSV with only successful payments",
			expectedStatusCode: http.StatusOK,
			queryParams:        "status=success",
			expectedPayments:   []*data.Payment{successfulPayment},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url := "/exports/payments"
			if tc.queryParams != "" {
				url += "?" + tc.queryParams
			}
			req, err := http.NewRequest(http.MethodGet, url, nil)
			require.NoError(t, err)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatusCode, rr.Code)
			csvReader := csv.NewReader(strings.NewReader(rr.Body.String()))

			header, err := csvReader.Read()
			require.NoError(t, err)

			expectedHeaders := []string{
				"ID", "Amount", "StellarTransactionID", "Status",
				"Disbursement.ID", "Asset.Code", "Asset.Issuer", "Wallet.Name", "Receiver.ID",
				"ReceiverWallet.Address", "ReceiverWallet.Status", "CreatedAt", "UpdatedAt",
				"ExternalPaymentID", "CircleTransferRequestID",
			}
			assert.Equal(t, expectedHeaders, header)

			assert.Equal(t, "text/csv", rr.Header().Get("Content-Type"))
			today := time.Now().Format("2006-01-02")
			assert.Contains(t, rr.Header().Get("Content-Disposition"), fmt.Sprintf("attachment; filename=payments_%s", today))

			rows, err := csvReader.ReadAll()
			require.NoError(t, err)
			assert.Len(t, rows, len(tc.expectedPayments))

			for i, row := range rows {
				assert.Equal(t, tc.expectedPayments[i].ID, row[0])
				assert.Equal(t, tc.expectedPayments[i].Amount, row[1])
				assert.Equal(t, tc.expectedPayments[i].StellarTransactionID, row[2])
				assert.Equal(t, string(tc.expectedPayments[i].Status), row[3])
				assert.Equal(t, tc.expectedPayments[i].Disbursement.ID, row[4])
				assert.Equal(t, tc.expectedPayments[i].Asset.Code, row[5])
				assert.Equal(t, tc.expectedPayments[i].Asset.Issuer, row[6])
				assert.Equal(t, tc.expectedPayments[i].ReceiverWallet.Wallet.Name, row[7])
				assert.Equal(t, tc.expectedPayments[i].ReceiverWallet.Receiver.ID, row[8])
				assert.Equal(t, tc.expectedPayments[i].ReceiverWallet.StellarAddress, row[9])
				assert.Equal(t, string(tc.expectedPayments[i].ReceiverWallet.Status), row[10])
				assert.Equal(t, tc.expectedPayments[i].ExternalPaymentID, row[13])
			}
		})
	}
}
