package httphandler

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"

	"github.com/go-chi/chi/v5"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpresponse"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"
)

func Test_DisbursementHandler_PostDisbursement(t *testing.T) {
	const url = "/disbursements"
	const method = "POST"

	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	token := "token"
	ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)
	user := &auth.User{
		ID:    "user-id",
		Email: "email@email.com",
	}
	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.
		On("GetUser", mock.Anything, token).
		Return(user, nil)

	mMonitorService := &monitor.MockMonitorService{}

	handler := &DisbursementHandler{
		Models:           models,
		MonitorService:   mMonitorService,
		DBConnectionPool: models.DBConnectionPool,
		AuthManager:      authManagerMock,
	}

	// setup fixtures
	wallets := data.ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)
	enabledWallet := wallets[0]
	disabledWallet := wallets[1]
	data.EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, false, disabledWallet.ID)

	enabledWallet.Assets = nil
	asset := data.GetAssetFixture(t, ctx, dbConnectionPool, data.FixtureAssetUSDC)
	country := data.GetCountryFixture(t, ctx, dbConnectionPool, data.FixtureCountryUKR)

	smsTemplate := "You have a new payment waiting for you from org x. Click on the link to register."

	t.Run("returns error when body is invalid", func(t *testing.T) {
		requestBody := `
       {
			"name": "My New Disbursement name 5",
       }`

		want := `{"error":"invalid request body"}`

		assertPOSTResponse(t, ctx, handler, method, url, requestBody, want, http.StatusBadRequest)
	})

	t.Run("returns error when name is not provided", func(t *testing.T) {
		requestBody := `
		{
		  "wallet_id": "aab4a4a9-2493-4f37-9741-01d5bd31d68b",
		  "asset_id": "61dbfa89-943a-413c-b862-a2177384d321",
		  "country_code": "UKR",
		  "verification_field": "date_of_birth"
		}`

		want := `
		{
			"error":"Request invalid", 
			"extras": {
				"name": "name is required"
			}
		}`

		assertPOSTResponse(t, ctx, handler, method, url, requestBody, want, http.StatusBadRequest)
	})

	t.Run("returns error when wallet_id is not provided", func(t *testing.T) {
		requestBody := `
		{
		   "name": "My New Disbursement name 5",
		   "asset_id": "61dbfa89-943a-413c-b862-a2177384d321",
		   "country_code": "UKR",
		   "verification_field": "date_of_birth"
		}`

		want := `{"error":"Request invalid", "extras": {"wallet_id": "wallet_id is required"}}`

		assertPOSTResponse(t, ctx, handler, method, url, requestBody, want, http.StatusBadRequest)
	})

	t.Run("returns error when asset_id is not provided", func(t *testing.T) {
		requestBody := `
		{
		   "name": "My New Disbursement name 5",
		   "wallet_id": "aab4a4a9-2493-4f37-9741-01d5bd31d68b",
		   "country_code": "UKR",
		   "verification_field": "date_of_birth"
		}`

		want := `{"error":"Request invalid", "extras": {"asset_id": "asset_id is required"}}`

		assertPOSTResponse(t, ctx, handler, method, url, requestBody, want, http.StatusBadRequest)
	})

	t.Run("returns error when country_code is not provided", func(t *testing.T) {
		requestBody := `
		{
		   "name": "My New Disbursement name 5",
		   "wallet_id": "aab4a4a9-2493-4f37-9741-01d5bd31d68b",
		   "asset_id": "61dbfa89-943a-413c-b862-a2177384d321",
		   "verification_field": "date_of_birth"
		}`

		want := `{"error":"Request invalid", "extras": {"country_code": "country_code is required"}}`

		assertPOSTResponse(t, ctx, handler, method, url, requestBody, want, http.StatusBadRequest)
	})

	t.Run("returns error when no verification field is provided", func(t *testing.T) {
		requestBody, err := json.Marshal(PostDisbursementRequest{
			Name:        "disbursement 1",
			CountryCode: country.Code,
			AssetID:     asset.ID,
			WalletID:    enabledWallet.ID,
		})
		require.NoError(t, err)

		want := `{"error":"Verification field invalid", "extras": {"verification_field": "invalid parameter. valid values are: DATE_OF_BIRTH, PIN, NATIONAL_ID_NUMBER"}}`

		assertPOSTResponse(t, ctx, handler, method, url, string(requestBody), want, http.StatusBadRequest)
	})

	t.Run("returns error when wallet_id is not valid", func(t *testing.T) {
		requestBody, err := json.Marshal(PostDisbursementRequest{
			Name:              "disbursement 1",
			CountryCode:       country.Code,
			AssetID:           asset.ID,
			WalletID:          "aab4a4a9-2493-4f37-9741-01d5bd31d68b",
			VerificationField: data.VerificationFieldDateOfBirth,
		})
		require.NoError(t, err)

		want := `{"error":"wallet ID is invalid"}`

		assertPOSTResponse(t, ctx, handler, method, url, string(requestBody), want, http.StatusBadRequest)
	})

	t.Run("returns error when wallet is not enabled", func(t *testing.T) {
		data.EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, false, disabledWallet.ID)
		requestBody, err := json.Marshal(PostDisbursementRequest{
			Name:              "disbursement 1",
			CountryCode:       country.Code,
			AssetID:           asset.ID,
			WalletID:          disabledWallet.ID,
			VerificationField: data.VerificationFieldDateOfBirth,
		})
		require.NoError(t, err)

		want := `{"error":"wallet is not enabled"}`

		assertPOSTResponse(t, ctx, handler, method, url, string(requestBody), want, http.StatusBadRequest)
	})

	t.Run("returns error when asset_id is not valid", func(t *testing.T) {
		requestBody, err := json.Marshal(PostDisbursementRequest{
			Name:              "disbursement 1",
			CountryCode:       country.Code,
			AssetID:           "aab4a4a9-2493-4f37-9741-01d5bd31d68b",
			WalletID:          enabledWallet.ID,
			VerificationField: data.VerificationFieldDateOfBirth,
		})
		require.NoError(t, err)

		want := `{"error":"asset ID is invalid"}`

		assertPOSTResponse(t, ctx, handler, method, url, string(requestBody), want, http.StatusBadRequest)
	})

	t.Run("returns error when country_code is not valid", func(t *testing.T) {
		requestBody, err := json.Marshal(PostDisbursementRequest{
			Name:              "disbursement 1",
			CountryCode:       "AAA",
			AssetID:           asset.ID,
			WalletID:          enabledWallet.ID,
			VerificationField: data.VerificationFieldDateOfBirth,
		})
		require.NoError(t, err)

		want := `{"error":"country code is invalid"}`

		assertPOSTResponse(t, ctx, handler, method, url, string(requestBody), want, http.StatusBadRequest)
	})

	labels := monitor.DisbursementLabels{
		Asset:   asset.Code,
		Country: country.Code,
		Wallet:  enabledWallet.Name,
	}

	t.Run("returns error when disbursement name is not unique", func(t *testing.T) {
		mMonitorService.On("MonitorCounters", monitor.DisbursementsCounterTag, labels.ToMap()).Return(nil).Once()

		requestBody, err := json.Marshal(PostDisbursementRequest{
			Name:              "disbursement 1",
			CountryCode:       country.Code,
			AssetID:           asset.ID,
			WalletID:          enabledWallet.ID,
			VerificationField: data.VerificationFieldDateOfBirth,
		})
		require.NoError(t, err)

		want := `{"error":"disbursement already exists"}`

		// create disbursement
		assertPOSTResponse(t, ctx, handler, method, url, string(requestBody), "", http.StatusCreated)
		mMonitorService.AssertExpectations(t)
		// try creating again
		assertPOSTResponse(t, ctx, handler, method, url, string(requestBody), want, http.StatusConflict)
	})

	t.Run("successfully create a disbursement", func(t *testing.T) {
		mMonitorService.On("MonitorCounters", monitor.DisbursementsCounterTag, labels.ToMap()).Return(nil).Once()

		expectedName := "disbursement 2"
		requestBody, err := json.Marshal(PostDisbursementRequest{
			Name:                           expectedName,
			CountryCode:                    country.Code,
			AssetID:                        asset.ID,
			WalletID:                       enabledWallet.ID,
			VerificationField:              data.VerificationFieldDateOfBirth,
			SMSRegistrationMessageTemplate: smsTemplate,
		})
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(ctx, method, url, strings.NewReader(string(requestBody)))
		http.HandlerFunc(handler.PostDisbursement).ServeHTTP(rr, req)

		resp := rr.Result()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var actualDisbursement data.Disbursement
		err = json.NewDecoder(resp.Body).Decode(&actualDisbursement)

		require.NoError(t, err)
		assert.Equal(t, expectedName, actualDisbursement.Name)
		assert.Equal(t, data.DraftDisbursementStatus, actualDisbursement.Status)
		assert.Equal(t, asset, actualDisbursement.Asset)
		assert.Equal(t, &enabledWallet, actualDisbursement.Wallet)
		assert.Equal(t, country, actualDisbursement.Country)
		assert.Equal(t, 1, len(actualDisbursement.StatusHistory))
		assert.Equal(t, data.DraftDisbursementStatus, actualDisbursement.StatusHistory[0].Status)
		assert.Equal(t, user.ID, actualDisbursement.StatusHistory[0].UserID)
		assert.Equal(t, smsTemplate, actualDisbursement.SMSRegistrationMessageTemplate)
		mMonitorService.AssertExpectations(t)
	})

	authManagerMock.AssertExpectations(t)
}

func Test_DisbursementHandler_GetDisbursements_Errors(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	handler := &DisbursementHandler{
		Models:           models,
		DBConnectionPool: dbConnectionPool,
	}

	ts := httptest.NewServer(http.HandlerFunc(handler.GetDisbursements))
	defer ts.Close()

	tests := []struct {
		name               string
		queryParams        map[string]string
		expectedStatusCode int
		expectedResponse   string
	}{
		{
			name: "returns error when sort parameter is invalid",
			queryParams: map[string]string{
				"sort": "invalid_sort",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"request invalid", "extras":{"sort":"invalid sort field name"}}`,
		},
		{
			name: "returns error when direction is invalid",
			queryParams: map[string]string{
				"direction": "invalid_direction",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"request invalid", "extras":{"direction":"invalid sort order. valid values are 'asc' and 'desc'"}}`,
		},
		{
			name: "returns error when page is invalid",
			queryParams: map[string]string{
				"page": "invalid_page",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"request invalid", "extras":{"page":"parameter must be an integer"}}`,
		},
		{
			name: "returns error when page_limit is invalid",
			queryParams: map[string]string{
				"page_limit": "invalid_page_limit",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"request invalid", "extras":{"page_limit":"parameter must be an integer"}}`,
		},
		{
			name: "returns error when status is invalid",
			queryParams: map[string]string{
				"status": "invalid_status",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"request invalid", "extras":{"status":"invalid parameter. valid value is a comma separate list of statuses: draft, ready, started, paused, completed"}}`,
		},
		{
			name: "returns error when created_at_after is invalid",
			queryParams: map[string]string{
				"created_at_after": "invalid_created_at_after",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"request invalid", "extras":{"created_at_after":"invalid date format. valid format is 'YYYY-MM-DD'"}}`,
		},
		{
			name: "returns error when created_at_before is invalid",
			queryParams: map[string]string{
				"created_at_before": "invalid_created_at_before",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"request invalid", "extras":{"created_at_before":"invalid date format. valid format is 'YYYY-MM-DD'"}}`,
		},
		{
			name:               "returns empty list when no expectedDisbursements are found",
			queryParams:        map[string]string{},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   `{"data":[], "pagination":{"pages":0, "total":0}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Build the URL for the test request
			url := buildURLWithQueryParams(ts.URL, "/disbursements", tc.queryParams)
			resp, err := http.Get(url)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedStatusCode, resp.StatusCode)
			assert.JSONEq(t, tc.expectedResponse, string(respBody))
		})
	}
}

func Test_DisbursementHandler_GetDisbursements_Success(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	authManagerMock := &auth.AuthManagerMock{}
	handler := &DisbursementHandler{
		Models:           models,
		DBConnectionPool: dbConnectionPool,
		AuthManager:      authManagerMock,
	}

	ts := httptest.NewServer(http.HandlerFunc(handler.GetDisbursements))
	defer ts.Close()

	ctx := context.Background()

	// create fixtures
	wallet := data.CreateDefaultWalletFixture(t, ctx, dbConnectionPool)
	asset := data.GetAssetFixture(t, ctx, dbConnectionPool, data.FixtureAssetUSDC)
	country := data.GetCountryFixture(t, ctx, dbConnectionPool, data.FixtureCountryUKR)

	createdByUser := auth.User{
		ID:        "User1",
		FirstName: "User",
		LastName:  "One",
	}
	startedByUser := auth.User{
		ID:        "User2",
		FirstName: "User",
		LastName:  "Two",
	}
	allUsers := []*auth.User{
		&startedByUser,
		&createdByUser,
	}

	authManagerMock.
		On("GetUsersByID", mock.Anything, []string{createdByUser.ID, startedByUser.ID}).
		Return(allUsers, nil)
	authManagerMock.
		On("GetUsersByID", mock.Anything, []string{startedByUser.ID, createdByUser.ID}).
		Return(allUsers, nil)
	authManagerMock.
		On("GetUsersByID", mock.Anything, []string{createdByUser.ID}).
		Return([]*auth.User{&createdByUser}, nil)

	createdByUserRef := services.UserReference{
		ID:        createdByUser.ID,
		FirstName: createdByUser.FirstName,
		LastName:  createdByUser.LastName,
	}
	startedByUserRef := services.UserReference{
		ID:        startedByUser.ID,
		FirstName: startedByUser.FirstName,
		LastName:  startedByUser.LastName,
	}

	draftStatusHistory := data.DisbursementStatusHistory{
		data.DisbursementStatusHistoryEntry{
			Status: data.DraftDisbursementStatus,
			UserID: createdByUser.ID,
		},
	}

	startedStatusHistory := data.DisbursementStatusHistory{
		data.DisbursementStatusHistoryEntry{
			Status: data.DraftDisbursementStatus,
			UserID: createdByUser.ID,
		},
		data.DisbursementStatusHistoryEntry{
			Status: data.StartedDisbursementStatus,
			UserID: startedByUser.ID,
		},
	}

	// create disbursements
	disbursement1 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:          "disbursement 1",
		Status:        data.DraftDisbursementStatus,
		StatusHistory: draftStatusHistory,
		Asset:         asset,
		Wallet:        wallet,
		Country:       country,
		CreatedAt:     time.Date(2022, 3, 21, 23, 40, 20, 1431, time.UTC),
	})
	disbursement2 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:          "disbursement 2",
		Status:        data.ReadyDisbursementStatus,
		StatusHistory: draftStatusHistory,
		Asset:         asset,
		Wallet:        wallet,
		Country:       country,
		CreatedAt:     time.Date(2023, 2, 20, 23, 40, 20, 1431, time.UTC),
	})
	disbursement3 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:          "disbursement 3",
		Status:        data.StartedDisbursementStatus,
		StatusHistory: startedStatusHistory,
		Asset:         asset,
		Wallet:        wallet,
		Country:       country,
		CreatedAt:     time.Date(2023, 3, 19, 23, 40, 20, 1431, time.UTC),
	})
	disbursement4 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:          "disbursement 4",
		Status:        data.DraftDisbursementStatus,
		StatusHistory: draftStatusHistory,
		Asset:         asset,
		Wallet:        wallet,
		Country:       country,
		CreatedAt:     time.Date(2023, 4, 19, 23, 40, 20, 1431, time.UTC),
	})

	tests := []struct {
		name                  string
		queryParams           map[string]string
		expectedStatusCode    int
		expectedPagination    httpresponse.PaginationInfo
		expectedDisbursements []services.DisbursementWithUserMetadata
	}{
		{
			name:               "fetch all disbursements without filters",
			queryParams:        map[string]string{},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "",
				Pages: 1,
				Total: 4,
			},
			expectedDisbursements: []services.DisbursementWithUserMetadata{
				{
					Disbursement: *disbursement4,
					CreatedBy:    createdByUserRef,
				},
				{
					Disbursement: *disbursement3,
					CreatedBy:    createdByUserRef,
					StartedBy:    startedByUserRef,
				},
				{
					Disbursement: *disbursement2,
					CreatedBy:    createdByUserRef,
				},
				{
					Disbursement: *disbursement1,
					CreatedBy:    createdByUserRef,
				},
			},
		},
		{
			name: "fetch first page of disbursements with limit 1 and sort by name",
			queryParams: map[string]string{
				"page":       "1",
				"page_limit": "1",
				"sort":       "name",
				"direction":  "asc",
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "/disbursements?direction=asc&page=2&page_limit=1&sort=name",
				Prev:  "",
				Pages: 4,
				Total: 4,
			},
			expectedDisbursements: []services.DisbursementWithUserMetadata{
				{
					Disbursement: *disbursement1,
					CreatedBy:    createdByUserRef,
				},
			},
		},
		{
			name: "fetch second page of disbursements with limit 1 and sort by name",
			queryParams: map[string]string{
				"page":       "2",
				"page_limit": "1",
				"sort":       "name",
				"direction":  "asc",
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "/disbursements?direction=asc&page=3&page_limit=1&sort=name",
				Prev:  "/disbursements?direction=asc&page=1&page_limit=1&sort=name",
				Pages: 4,
				Total: 4,
			},
			expectedDisbursements: []services.DisbursementWithUserMetadata{
				{
					Disbursement: *disbursement2,
					CreatedBy:    createdByUserRef,
				},
			},
		},
		{
			name: "fetch last page of disbursements with limit 1 and sort by name",
			queryParams: map[string]string{
				"page":       "4",
				"page_limit": "1",
				"sort":       "name",
				"direction":  "asc",
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "/disbursements?direction=asc&page=3&page_limit=1&sort=name",
				Pages: 4,
				Total: 4,
			},
			expectedDisbursements: []services.DisbursementWithUserMetadata{
				{
					Disbursement: *disbursement4,
					CreatedBy:    createdByUserRef,
				},
			},
		},
		{
			name: "fetch last page of disbursements with limit 1 and sort by name",
			queryParams: map[string]string{
				"page":       "4",
				"page_limit": "1",
				"sort":       "name",
				"direction":  "asc",
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "/disbursements?direction=asc&page=3&page_limit=1&sort=name",
				Pages: 4,
				Total: 4,
			},
			expectedDisbursements: []services.DisbursementWithUserMetadata{
				{
					Disbursement: *disbursement4,
					CreatedBy:    createdByUserRef,
				},
			},
		},
		{
			name: "fetch disbursements with status draft",
			queryParams: map[string]string{
				"status": "dRaFt",
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "",
				Pages: 1,
				Total: 2,
			},
			expectedDisbursements: []services.DisbursementWithUserMetadata{
				{
					Disbursement: *disbursement4,
					CreatedBy:    createdByUserRef,
				},
				{
					Disbursement: *disbursement1,
					CreatedBy:    createdByUserRef,
				},
			},
		},
		{
			name: "fetch disbursements with status draft and q=1",
			queryParams: map[string]string{
				"status": "draft",
				"q":      "1",
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "",
				Pages: 1,
				Total: 1,
			},
			expectedDisbursements: []services.DisbursementWithUserMetadata{
				{
					Disbursement: *disbursement1,
					CreatedBy:    createdByUserRef,
				},
			},
		},
		{
			name: "fetch disbursements after 2023-01-01",
			queryParams: map[string]string{
				"created_at_after": "2023-01-01",
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "",
				Pages: 1,
				Total: 3,
			},
			expectedDisbursements: []services.DisbursementWithUserMetadata{
				{
					Disbursement: *disbursement4,
					CreatedBy:    createdByUserRef,
				},
				{
					Disbursement: *disbursement3,
					CreatedBy:    createdByUserRef,
					StartedBy:    startedByUserRef,
				},
				{
					Disbursement: *disbursement2,
					CreatedBy:    createdByUserRef,
				},
			},
		},
		{
			name: "fetch disbursements after 2023-01-01 and before 2023-03-20",
			queryParams: map[string]string{
				"created_at_after":  "2023-01-01",
				"created_at_before": "2023-03-20",
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "",
				Pages: 1,
				Total: 2,
			},
			expectedDisbursements: []services.DisbursementWithUserMetadata{
				{
					Disbursement: *disbursement3,
					CreatedBy:    createdByUserRef,
					StartedBy:    startedByUserRef,
				},
				{
					Disbursement: *disbursement2,
					CreatedBy:    createdByUserRef,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Build the URL for the test request
			url := buildURLWithQueryParams(ts.URL, "/disbursements", tc.queryParams)
			resp, err := http.Get(url)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Parse the response body
			var actualResponse httpresponse.PaginatedResponse
			err = json.NewDecoder(resp.Body).Decode(&actualResponse)
			require.NoError(t, err)

			// Assert on the pagination data
			assert.Equal(t, tc.expectedPagination, actualResponse.Pagination)

			// Parse the response data
			var actualDisbursements []services.DisbursementWithUserMetadata
			err = json.Unmarshal(actualResponse.Data, &actualDisbursements)
			require.NoError(t, err)

			// Assert on the disbursements data
			assert.Equal(t, tc.expectedDisbursements, actualDisbursements)
		})
	}
}

func Test_DisbursementHandler_PostDisbursementInstructions(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	mMonitorService := &monitor.MockMonitorService{}

	token := "token"
	ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)

	user := &auth.User{
		ID:    "user-id",
		Email: "email@email.com",
	}
	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.
		On("GetUser", mock.Anything, token).
		Return(user, nil).
		Run(func(args mock.Arguments) {
			mockCtx := args.Get(0).(context.Context)
			val := mockCtx.Value(middleware.TokenContextKey)
			assert.Equal(t, token, val)
		})

	handler := &DisbursementHandler{
		Models:           models,
		MonitorService:   mMonitorService,
		DBConnectionPool: models.DBConnectionPool,
		AuthManager:      authManagerMock,
	}

	// create fixtures
	wallet := data.CreateDefaultWalletFixture(t, ctx, dbConnectionPool)
	asset := data.GetAssetFixture(t, ctx, dbConnectionPool, data.FixtureAssetUSDC)
	country := data.GetCountryFixture(t, ctx, dbConnectionPool, data.FixtureCountryUKR)

	// create disbursement
	draftDisbursement := data.CreateDraftDisbursementFixture(t, ctx, dbConnectionPool, handler.Models.Disbursements, data.Disbursement{
		Name:    "disbursement1",
		Asset:   asset,
		Country: country,
		Wallet:  wallet,
	})

	startedDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, handler.Models.Disbursements, &data.Disbursement{
		Name:      "disbursement 1",
		Status:    data.StartedDisbursementStatus,
		CreatedAt: time.Date(2022, 3, 21, 23, 40, 20, 1431, time.UTC),
	})

	maxCSVRecords := [][]string{
		{"phone", "id", "amount", "verification"},
	}
	for i := 0; i < 10001; i++ {
		maxCSVRecords = append(maxCSVRecords, []string{
			"+380445555555", "123456789", "100.5", "1990-01-01",
		})
	}

	testCases := []struct {
		name            string
		disbursementID  string
		fieldName       string
		csvRecords      [][]string
		expectedStatus  int
		expectedMessage string
	}{
		{
			name:           "valid input",
			disbursementID: draftDisbursement.ID,
			csvRecords: [][]string{
				{"phone", "id", "amount", "verification"},
				{"+380445555555", "123456789", "100.5", "1990-01-01"},
			},
			expectedStatus:  http.StatusOK,
			expectedMessage: "File uploaded successfully",
		},
		{
			name:           "invalid date of birth",
			disbursementID: draftDisbursement.ID,
			csvRecords: [][]string{
				{"phone", "id", "amount", "verification"},
				{"+380445555555", "123456789", "100.5", "1990/01/01"},
			},
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "invalid date of birth format. Correct format: 1990-01-01",
		},
		{
			name:           "invalid phone number",
			disbursementID: draftDisbursement.ID,
			csvRecords: [][]string{
				{"phone", "id", "amount", "verification"},
				{"380-12-345-678", "123456789", "100.5", "1990-01-01"},
			},
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "invalid phone format. Correct format: +380445555555",
		},
		{
			name:            "invalid disbursement id",
			disbursementID:  "invalid",
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "disbursement ID is invalid",
		},
		{
			name:            "valid input",
			disbursementID:  draftDisbursement.ID,
			fieldName:       "instructions",
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "could not parse file",
		},
		{
			name:            "disbursement not in draft/ready starte",
			disbursementID:  startedDisbursement.ID,
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "disbursement is not in draft or ready status",
		},
		{
			name:            "disbursement not in draft/ready state",
			disbursementID:  startedDisbursement.ID,
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "disbursement is not in draft or ready status",
		},
		{
			name:           "error parsing header",
			disbursementID: draftDisbursement.ID,
			csvRecords: [][]string{
				{},
			},
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "could not parse file",
		},
		{
			name:           "no instructions found in file",
			disbursementID: draftDisbursement.ID,
			csvRecords: [][]string{
				{"phone", "id", "amount", "date-of-birth"},
			},
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "no valid instructions found",
		},
		{
			name:            "max instructions exceeded",
			disbursementID:  draftDisbursement.ID,
			csvRecords:      maxCSVRecords,
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "number of instructions exceeds maximum of : 10000",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fileContent, err := createCSVFile(t, tc.csvRecords)
			require.NoError(t, err)

			req, err := createInstructionsMultipartRequest(t, ctx, tc.fieldName, tc.disbursementID, fileContent)
			require.NoError(t, err)

			// Record the response
			rr := httptest.NewRecorder()
			router := chi.NewRouter()
			router.Post("/disbursements/{id}/instructions", handler.PostDisbursementInstructions)
			router.ServeHTTP(rr, req)

			// Check the response status and message
			assert.Equal(t, tc.expectedStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tc.expectedMessage)
		})

		authManagerMock.AssertExpectations(t)
	}
}

func Test_DisbursementHandler_GetDisbursement(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	authManagerMock := &auth.AuthManagerMock{}
	createdByUser := auth.User{
		ID:        "User1",
		FirstName: "User",
		LastName:  "One",
	}
	startedByUser := auth.User{
		ID:        "User2",
		FirstName: "User",
		LastName:  "Two",
	}

	allUsers := []*auth.User{
		&createdByUser,
		&startedByUser,
	}

	authManagerMock.
		On("GetUsersByID", mock.Anything, []string{createdByUser.ID, startedByUser.ID}).
		Return(allUsers, nil)
	authManagerMock.
		On("GetUsersByID", mock.Anything, []string{startedByUser.ID, createdByUser.ID}).
		Return(allUsers, nil)

	handler := &DisbursementHandler{
		Models:           models,
		DBConnectionPool: models.DBConnectionPool,
		AuthManager:      authManagerMock,
	}

	r := chi.NewRouter()
	r.Get("/disbursements/{id}", handler.GetDisbursement)

	// create disbursements
	disbursement := data.CreateDisbursementFixture(t, context.Background(), dbConnectionPool, handler.Models.Disbursements, &data.Disbursement{
		Name:   "disbursement 1",
		Status: data.StartedDisbursementStatus,
		StatusHistory: data.DisbursementStatusHistory{
			data.DisbursementStatusHistoryEntry{
				Status: data.DraftDisbursementStatus,
				UserID: createdByUser.ID,
			},
			data.DisbursementStatusHistoryEntry{
				Status: data.StartedDisbursementStatus,
				UserID: startedByUser.ID,
			},
		},
		CreatedAt: time.Date(2022, 3, 21, 23, 40, 20, 1431, time.UTC),
	})

	response := services.DisbursementWithUserMetadata{
		Disbursement: *disbursement,
		CreatedBy: services.UserReference{
			ID:        createdByUser.ID,
			FirstName: createdByUser.FirstName,
			LastName:  createdByUser.LastName,
		},
		StartedBy: services.UserReference{
			ID:        startedByUser.ID,
			FirstName: startedByUser.FirstName,
			LastName:  startedByUser.LastName,
		},
	}

	tests := []struct {
		name                 string
		id                   string
		expectedStatusCode   int
		expectedDisbursement services.DisbursementWithUserMetadata
		expectedErrorMessage string
	}{
		{
			name:                 "disbursement not found",
			id:                   "invalid",
			expectedStatusCode:   http.StatusNotFound,
			expectedErrorMessage: "disbursement not found",
		},
		{
			name:                 "success",
			id:                   disbursement.ID,
			expectedStatusCode:   http.StatusOK,
			expectedDisbursement: response,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/disbursements/%s", tc.id), nil)
			require.NoError(t, err)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code == http.StatusOK {
				var actualDisbursement services.DisbursementWithUserMetadata
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &actualDisbursement))
				require.Equal(t, tc.expectedDisbursement, actualDisbursement)
			} else {
				var actualErrorMessage httperror.HTTPError
				require.Equal(t, tc.expectedStatusCode, rr.Code)
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &actualErrorMessage))
				require.Equal(t, tc.expectedErrorMessage, actualErrorMessage.Message)
			}
		})
	}
}

func Test_DisbursementHandler_GetDisbursementReceivers(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	handler := &DisbursementHandler{
		Models:           models,
		DBConnectionPool: models.DBConnectionPool,
	}

	r := chi.NewRouter()
	r.Get("/disbursements/{id}/receivers", handler.GetDisbursementReceivers)

	// create fixtures
	wallet := data.CreateWalletFixture(t, context.Background(), dbConnectionPool,
		"My Wallet",
		"https://mywallet.com",
		"mywallet.com",
		"mywallet://")
	asset := data.CreateAssetFixture(t, context.Background(), dbConnectionPool,
		"USDC",
		"GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := data.CreateCountryFixture(t, context.Background(), dbConnectionPool,
		"FRA",
		"France")

	// create disbursements
	disbursementWithReceivers := data.CreateDisbursementFixture(t, context.Background(), dbConnectionPool, handler.Models.Disbursements, &data.Disbursement{
		Name:    "disbursement with receivers",
		Status:  data.DraftDisbursementStatus,
		Asset:   asset,
		Wallet:  wallet,
		Country: country,
	})
	disbursementWithoutReceivers := data.CreateDisbursementFixture(t, context.Background(), dbConnectionPool, handler.Models.Disbursements, &data.Disbursement{
		Name:    "disbursement without receivers",
		Status:  data.DraftDisbursementStatus,
		Asset:   asset,
		Wallet:  wallet,
		Country: country,
	})

	// create disbursement receivers
	ctx := context.Background()
	yesterday := time.Now().Add(-time.Hour * 24)
	twoDaysAgo := time.Now().Add(-time.Hour * 48)
	threeDaysAgo := time.Now().Add(-time.Hour * 72)

	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{CreatedAt: &yesterday})
	receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{CreatedAt: &twoDaysAgo})
	receiver3 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{CreatedAt: &threeDaysAgo})

	receiverWallet1 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, data.DraftReceiversWalletStatus)
	receiverWallet2 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, data.DraftReceiversWalletStatus)
	receiverWallet3 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver3.ID, wallet.ID, data.DraftReceiversWalletStatus)

	payment1 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, handler.Models.Payment, &data.Payment{
		ReceiverWallet: receiverWallet1,
		Disbursement:   disbursementWithReceivers,
		Asset:          *asset,
		Amount:         "100",
		Status:         data.SuccessPaymentStatus,
	})
	payment2 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, handler.Models.Payment, &data.Payment{
		ReceiverWallet: receiverWallet2,
		Disbursement:   disbursementWithReceivers,
		Asset:          *asset,
		Amount:         "200",
		Status:         data.SuccessPaymentStatus,
	})
	payment3 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, handler.Models.Payment, &data.Payment{
		ReceiverWallet: receiverWallet3,
		Disbursement:   disbursementWithReceivers,
		Asset:          *asset,
		Amount:         "300",
		Status:         data.SuccessPaymentStatus,
	})

	expectedDisbursementReceivers := []data.DisbursementReceiver{
		{
			ID:             receiver3.ID,
			PhoneNumber:    receiver3.PhoneNumber,
			Email:          *receiver3.Email,
			ExternalID:     receiver3.ExternalID,
			ReceiverWallet: receiverWallet3,
			Payment:        payment3,
			CreatedAt:      *receiver3.CreatedAt,
			UpdatedAt:      *receiver3.UpdatedAt,
		},
		{
			ID:             receiver2.ID,
			PhoneNumber:    receiver2.PhoneNumber,
			Email:          *receiver2.Email,
			ExternalID:     receiver2.ExternalID,
			ReceiverWallet: receiverWallet2,
			Payment:        payment2,
			CreatedAt:      *receiver2.CreatedAt,
			UpdatedAt:      *receiver2.UpdatedAt,
		},
		{
			ID:             receiver1.ID,
			PhoneNumber:    receiver1.PhoneNumber,
			Email:          *receiver1.Email,
			ExternalID:     receiver1.ExternalID,
			ReceiverWallet: receiverWallet1,
			Payment:        payment1,
			CreatedAt:      *receiver1.CreatedAt,
			UpdatedAt:      *receiver1.UpdatedAt,
		},
	}

	t.Run("disbursement doesn't exist", func(t *testing.T) {
		id := "5e1f1c7f5b6c9c0001c1b1b1"
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/disbursements/%s/receivers", id), nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("disbursement without receivers", func(t *testing.T) {
		id := disbursementWithoutReceivers.ID
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/disbursements/%s/receivers", id), nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)

		var actualResponse httpresponse.PaginatedResponse
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&actualResponse))
		require.Equal(t, httpresponse.NewEmptyPaginatedResponse(), actualResponse)
	})

	t.Run("disbursement with receivers", func(t *testing.T) {
		id := disbursementWithReceivers.ID
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/disbursements/%s/receivers", id), nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)

		var actualResponse httpresponse.PaginatedResponse
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&actualResponse))
		require.Equal(t, 3, actualResponse.Pagination.Total)
		require.Equal(t, 1, actualResponse.Pagination.Pages)

		var actualDisbursementReceivers []data.DisbursementReceiver
		require.NoError(t, json.NewDecoder(bytes.NewReader(actualResponse.Data)).Decode(&actualDisbursementReceivers))

		for i, actual := range actualDisbursementReceivers {
			require.Equal(t, expectedDisbursementReceivers[i].ID, actual.ID)
			require.Equal(t, expectedDisbursementReceivers[i].PhoneNumber, actual.PhoneNumber)
			require.Equal(t, expectedDisbursementReceivers[i].Email, actual.Email)
			require.Equal(t, expectedDisbursementReceivers[i].ExternalID, actual.ExternalID)
			require.Equal(t, expectedDisbursementReceivers[i].ReceiverWallet.ID, actual.ReceiverWallet.ID)
			require.Equal(t, expectedDisbursementReceivers[i].Payment.ID, actual.Payment.ID)
		}
	})
}

func Test_DisbursementHandler_PatchDisbursementStatus(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	token := "token"
	ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)
	userID := "valid-user-id"
	user := &auth.User{
		ID:    userID,
		Email: "email@email.com",
	}
	require.NotNil(t, user)
	authManagerMock := &auth.AuthManagerMock{}

	handler := &DisbursementHandler{
		Models:           models,
		DBConnectionPool: models.DBConnectionPool,
		AuthManager:      authManagerMock,
	}

	r := chi.NewRouter()
	r.Patch("/disbursements/{id}/status", handler.PatchDisbursementStatus)

	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{})
	require.NotNil(t, disbursement)

	readyStatusHistory := []data.DisbursementStatusHistoryEntry{
		{
			Status: data.DraftDisbursementStatus,
			UserID: userID,
		},
		{
			Status: data.ReadyDisbursementStatus,
			UserID: userID,
		},
	}
	// create disbursements
	draftDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, handler.Models.Disbursements, &data.Disbursement{
		Name:   "draft disbursement",
		Status: data.DraftDisbursementStatus,
	})

	reqBody := bytes.NewBuffer(nil)
	t.Run("invalid body", func(t *testing.T) {
		id := draftDisbursement.ID
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, fmt.Sprintf("/disbursements/%s/status", id), reqBody)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Contains(t, rr.Body.String(), "invalid request body")
	})

	t.Run("invalid status", func(t *testing.T) {
		id := "5e1f1c7f5b6c9c0001c1b1b1"
		err := json.NewEncoder(reqBody).Encode(PatchDisbursementStatusRequest{Status: "INVALID"})
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, fmt.Sprintf("/disbursements/%s/status", id), reqBody)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Contains(t, rr.Body.String(), "invalid status")
	})

	t.Run("disbursement not ready to start", func(t *testing.T) {
		err := json.NewEncoder(reqBody).Encode(PatchDisbursementStatusRequest{Status: "Started"})
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, fmt.Sprintf("/disbursements/%s/status", draftDisbursement.ID), reqBody)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Contains(t, rr.Body.String(), services.ErrDisbursementNotReadyToStart.Error())
	})

	t.Run("disbursement can't be started by creator", func(t *testing.T) {
		data.EnableDisbursementApproval(t, ctx, handler.Models.Organizations)
		defer data.DisableDisbursementApproval(t, ctx, handler.Models.Organizations)

		readyDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, handler.Models.Disbursements, &data.Disbursement{
			Name:          "ready disbursement #1",
			Status:        data.ReadyDisbursementStatus,
			StatusHistory: readyStatusHistory,
		})

		authManagerMock.
			On("GetUser", mock.Anything, token).
			Return(user, nil).
			Once()

		err := json.NewEncoder(reqBody).Encode(PatchDisbursementStatusRequest{Status: "Started"})
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, fmt.Sprintf("/disbursements/%s/status", readyDisbursement.ID), reqBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusForbidden, rr.Code)
		require.Contains(t, rr.Body.String(), "Disbursement can't be started by its creator. Approval by another user is required")
	})

	t.Run("disbursement can be started by approver who is not a creator", func(t *testing.T) {
		data.EnableDisbursementApproval(t, ctx, handler.Models.Organizations)
		defer data.DisableDisbursementApproval(t, ctx, handler.Models.Organizations)

		readyDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, handler.Models.Disbursements, &data.Disbursement{
			Name:          "ready disbursement #2",
			Status:        data.ReadyDisbursementStatus,
			StatusHistory: readyStatusHistory,
		})

		approverUser := &auth.User{
			ID:    "valid-approver-user-id",
			Email: "approver@mail.org",
		}

		authManagerMock.
			On("GetUser", mock.Anything, token).
			Return(approverUser, nil).
			Once()

		err := json.NewEncoder(reqBody).Encode(PatchDisbursementStatusRequest{Status: "Started"})
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, fmt.Sprintf("/disbursements/%s/status", readyDisbursement.ID), reqBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		require.Contains(t, rr.Body.String(), "Disbursement started")
	})

	t.Run("disbursement started - then paused", func(t *testing.T) {
		authManagerMock.
			On("GetUser", mock.Anything, token).
			Return(user, nil).
			Twice()
		readyDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, handler.Models.Disbursements, &data.Disbursement{
			Name:          "ready disbursement #3",
			Status:        data.ReadyDisbursementStatus,
			StatusHistory: readyStatusHistory,
		})

		err := json.NewEncoder(reqBody).Encode(PatchDisbursementStatusRequest{Status: "Started"})
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, fmt.Sprintf("/disbursements/%s/status", readyDisbursement.ID), reqBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		require.Contains(t, rr.Body.String(), "Disbursement started")

		// check disbursement status
		disbursement, err := handler.Models.Disbursements.Get(context.Background(), models.DBConnectionPool, readyDisbursement.ID)
		require.NoError(t, err)
		require.Equal(t, data.StartedDisbursementStatus, disbursement.Status)

		// pause disbursement
		err = json.NewEncoder(reqBody).Encode(PatchDisbursementStatusRequest{Status: "Paused"})
		require.NoError(t, err)

		req, err = http.NewRequestWithContext(ctx, http.MethodPatch, fmt.Sprintf("/disbursements/%s/status", readyDisbursement.ID), reqBody)
		require.NoError(t, err)
		rr = httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		require.Contains(t, rr.Body.String(), "Disbursement paused")

		// check disbursement status
		disbursement, err = handler.Models.Disbursements.Get(context.Background(), models.DBConnectionPool, readyDisbursement.ID)
		require.NoError(t, err)
		require.Equal(t, data.PausedDisbursementStatus, disbursement.Status)
	})

	t.Run("disbursement can't be paused", func(t *testing.T) {
		err := json.NewEncoder(reqBody).Encode(PatchDisbursementStatusRequest{Status: "Paused"})
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPatch, fmt.Sprintf("/disbursements/%s/status", draftDisbursement.ID), reqBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Contains(t, rr.Body.String(), services.ErrDisbursementNotReadyToPause.Error())
	})

	t.Run("disbursement status can't be changed", func(t *testing.T) {
		err := json.NewEncoder(reqBody).Encode(PatchDisbursementStatusRequest{Status: "Completed"})
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPatch, fmt.Sprintf("/disbursements/%s/status", draftDisbursement.ID), reqBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Contains(t, rr.Body.String(), services.ErrDisbursementStatusCantBeChanged.Error())
	})

	t.Run("disbursement doesn't exist", func(t *testing.T) {
		id := "5e1f1c7f5b6c9c0001c1b1b1"
		err := json.NewEncoder(reqBody).Encode(PatchDisbursementStatusRequest{Status: "STARTED"})
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, fmt.Sprintf("/disbursements/%s/status", id), reqBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusNotFound, rr.Code)
		require.Contains(t, rr.Body.String(), services.ErrDisbursementNotFound.Error())
	})

	authManagerMock.AssertExpectations(t)
}

func Test_DisbursementHandler_GetDisbursementInstructions(t *testing.T) {
	ctx := context.Background()

	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	models, outerErr := data.NewModels(dbConnectionPool)
	require.NoError(t, outerErr)

	handler := &DisbursementHandler{
		Models:           models,
		DBConnectionPool: models.DBConnectionPool,
	}

	r := chi.NewRouter()
	r.Get("/disbursements/{id}/instructions", handler.GetDisbursementInstructions)

	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{})
	require.NotNil(t, disbursement)

	t.Run("disbursement doesn't exist", func(t *testing.T) {
		id := "9e0ff65f-f6e9-46e9-bf03-dc46723e3bfb"

		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/disbursements/%s/instructions", id), nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusNotFound, rr.Code)
		require.Contains(t, rr.Body.String(), services.ErrDisbursementNotFound.Error())
	})

	t.Run("disbursement has no instructions", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/disbursements/%s/instructions", disbursement.ID), nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusNotFound, rr.Code)
		require.Contains(t, rr.Body.String(), fmt.Sprintf("disbursement %s has no instructions file", disbursement.ID))
	})

	t.Run("disbursement has instructions", func(t *testing.T) {
		disbursementFileContent := data.CreateInstructionsFixture(t, []*data.DisbursementInstruction{
			{Phone: "1234567890", ID: "1", Amount: "123.12", VerificationValue: "1995-02-20"},
			{Phone: "0987654321", ID: "2", Amount: "321", VerificationValue: "1974-07-19"},
			{Phone: "0987654321", ID: "3", Amount: "321", VerificationValue: "1974-07-19"},
		})

		err := models.Disbursements.Update(ctx, &data.DisbursementUpdate{
			ID:          disbursement.ID,
			FileContent: disbursementFileContent,
			FileName:    "instructions.csv",
		})
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/disbursements/%s/instructions", disbursement.ID), nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		require.Equal(t, "text/csv", rr.Header().Get("Content-Type"))
		require.Equal(t, "attachment; filename=\"instructions.csv\"", rr.Header().Get("Content-Disposition"))
		require.Equal(t, string(disbursementFileContent), rr.Body.String())
	})
}

func createCSVFile(t *testing.T, records [][]string) (io.Reader, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	for _, record := range records {
		err := writer.Write(record)
		require.NoError(t, err)
	}
	writer.Flush()
	return &buf, nil
}

func createInstructionsMultipartRequest(t *testing.T, ctx context.Context, fieldName, disbursementID string, fileContent io.Reader) (*http.Request, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	if fieldName == "" {
		fieldName = "file"
	}

	part, err := writer.CreateFormFile(fieldName, "instructions.csv")
	require.NoError(t, err)

	_, err = io.Copy(part, fileContent)
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	url := fmt.Sprintf("/disbursements/%s/instructions", disbursementID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, nil
}

func assertPOSTResponse(t *testing.T, ctx context.Context, handler *DisbursementHandler, method, url, requestBody, want string, expectedStatus int) {
	rr := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(ctx, method, url, strings.NewReader(requestBody))
	http.HandlerFunc(handler.PostDisbursement).ServeHTTP(rr, req)

	resp := rr.Result()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, expectedStatus, resp.StatusCode)

	if want != "" {
		assert.JSONEq(t, want, string(respBody))
	}
}

func buildURLWithQueryParams(baseURL, endpoint string, queryParams map[string]string) string {
	url := baseURL + endpoint
	if len(queryParams) > 0 {
		url += "?"
		for k, v := range queryParams {
			url += fmt.Sprintf("%s=%s&", k, v)
		}
		url = strings.TrimSuffix(url, "&")
	}
	return url
}
