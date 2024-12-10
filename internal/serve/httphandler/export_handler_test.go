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
