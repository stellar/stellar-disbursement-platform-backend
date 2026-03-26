package httphandler

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	monitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpresponse"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	svcMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_DisbursementHandler_validateRequest(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	wallets := data.ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)
	wallet := wallets[0]

	embeddedWalletFixture := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Embedded Wallet", "https://embedded.example.com", "embedded.example.com", "embedded://")
	data.MakeWalletEmbedded(t, ctx, dbConnectionPool, embeddedWalletFixture.ID)
	embeddedWallet := data.GetWalletFixture(t, ctx, dbConnectionPool, embeddedWalletFixture.Name)

	handler := &DisbursementHandler{Models: models}

	type TestCase struct {
		name           string
		request        PostDisbursementRequest
		expectedErrors map[string]interface{}
	}

	testCases := []TestCase{
		{
			name:    "🔴 all fields are empty",
			request: PostDisbursementRequest{},
			expectedErrors: map[string]interface{}{
				"name":                      "name is required",
				"wallet_id":                 "wallet_id is required",
				"asset_id":                  "asset_id is required",
				"registration_contact_type": fmt.Sprintf("registration_contact_type must be one of %v", data.AllRegistrationContactTypes()),
				"verification_field":        fmt.Sprintf("verification_field must be one of %v", data.GetAllVerificationTypes()),
			},
		},
		{
			name: "🔴 wallet_id does not exist",
			request: PostDisbursementRequest{
				Name:                    "disbursement 1",
				AssetID:                 "61dbfa89-943a-413c-b862-a2177384d321",
				WalletID:                "non-existent-wallet-id",
				RegistrationContactType: data.RegistrationContactTypePhone,
				VerificationField:       data.VerificationTypeDateOfBirth,
			},
			expectedErrors: map[string]interface{}{
				"wallet_id": "wallet_id could not be retrieved",
			},
		},
		{
			name: "🔴 wallet_id and verification_field not allowed for user managed wallet",
			request: PostDisbursementRequest{
				Name:                    "disbursement 1",
				AssetID:                 "61dbfa89-943a-413c-b862-a2177384d321",
				WalletID:                wallet.ID,
				RegistrationContactType: data.RegistrationContactTypePhoneAndWalletAddress,
				VerificationField:       data.VerificationTypeDateOfBirth,
			},
			expectedErrors: map[string]interface{}{
				"wallet_id":          "wallet_id is not allowed for this registration contact type",
				"verification_field": "verification_field is not allowed for this registration contact type",
			},
		},
		{
			name: "🔴 registration_contact_type and verification_field are invalid",
			request: PostDisbursementRequest{
				Name:     "disbursement 1",
				AssetID:  "61dbfa89-943a-413c-b862-a2177384d321",
				WalletID: wallet.ID,
				RegistrationContactType: data.RegistrationContactType{
					ReceiverContactType: "invalid1",
				},
				VerificationField: "invalid2",
			},
			expectedErrors: map[string]interface{}{
				"registration_contact_type": fmt.Sprintf("registration_contact_type must be one of %v", data.AllRegistrationContactTypes()),
				"verification_field":        fmt.Sprintf("verification_field must be one of %v", data.GetAllVerificationTypes()),
			},
		},
		{
			name: "🔴 receiver_registration_message_template contains HTML",
			request: PostDisbursementRequest{
				Name:                                "disbursement 1",
				AssetID:                             "61dbfa89-943a-413c-b862-a2177384d321",
				WalletID:                            wallet.ID,
				RegistrationContactType:             data.RegistrationContactTypePhone,
				VerificationField:                   data.VerificationTypeDateOfBirth,
				ReceiverRegistrationMessageTemplate: "<a href='evil.com'>Redeem money</a>",
			},
			expectedErrors: map[string]interface{}{
				"receiver_registration_message_template": "receiver_registration_message_template cannot contain HTML, JS or CSS",
			},
		},
		{
			name: "🔴 receiver_registration_message_template contains JS",
			request: PostDisbursementRequest{
				Name:                                "disbursement 1",
				AssetID:                             "61dbfa89-943a-413c-b862-a2177384d321",
				WalletID:                            wallet.ID,
				RegistrationContactType:             data.RegistrationContactTypePhone,
				VerificationField:                   data.VerificationTypeDateOfBirth,
				ReceiverRegistrationMessageTemplate: "javascript:alert(localStorage.getItem('sdp_session'))",
			},
			expectedErrors: map[string]interface{}{
				"receiver_registration_message_template": "receiver_registration_message_template cannot contain HTML, JS or CSS",
			},
		},
		{
			name: "🟢 all fields are valid",
			request: PostDisbursementRequest{
				Name:                    "disbursement 1",
				AssetID:                 "61dbfa89-943a-413c-b862-a2177384d321",
				WalletID:                wallet.ID,
				RegistrationContactType: data.RegistrationContactTypePhone,
				VerificationField:       data.VerificationTypeDateOfBirth,
			},
		},
		{
			name: "🟢 all fields are valid w/ receiver_registration_message_template",
			request: PostDisbursementRequest{
				Name:                                "disbursement 1",
				AssetID:                             "61dbfa89-943a-413c-b862-a2177384d321",
				WalletID:                            wallet.ID,
				RegistrationContactType:             data.RegistrationContactTypePhone,
				VerificationField:                   data.VerificationTypeDateOfBirth,
				ReceiverRegistrationMessageTemplate: "My custom invitation message",
			},
		},
		{
			name: "🟢 embedded wallet allows empty verification_field",
			request: PostDisbursementRequest{
				Name:                    "embedded disbursement",
				AssetID:                 "asset-id",
				WalletID:                embeddedWallet.ID,
				RegistrationContactType: data.RegistrationContactTypePhone,
			},
		},
	}

	for _, rct := range data.AllRegistrationContactTypes() {
		var name string
		var expectedErrors map[string]interface{}
		if !rct.IncludesWalletAddress {
			name = fmt.Sprintf("🔴[%s]registration_contact_type without wallet address REQUIRES verification_field", rct)
			expectedErrors = map[string]interface{}{
				"verification_field": fmt.Sprintf("verification_field must be one of %v", data.GetAllVerificationTypes()),
			}
		} else {
			name = fmt.Sprintf("🟢[%s]registration_contact_type with wallet address DOES NOT REQUIRE registration_contact_type", rct)
		}
		newTestCase := TestCase{
			name: name,
			request: PostDisbursementRequest{
				Name:                    "disbursement 1",
				AssetID:                 "61dbfa89-943a-413c-b862-a2177384d321",
				RegistrationContactType: rct,
			},
			expectedErrors: expectedErrors,
		}
		if !rct.IncludesWalletAddress {
			newTestCase.request.WalletID = wallet.ID
		}

		testCases = append(testCases, newTestCase)
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			v := handler.validateRequest(ctx, tc.request)
			if len(tc.expectedErrors) == 0 {
				assert.False(t, v.HasErrors())
			} else {
				assert.True(t, v.HasErrors())
				assert.Equal(t, tc.expectedErrors, v.Errors)
			}
		})
	}
}

func Test_DisbursementHandler_PostDisbursement(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	_, ctx := tenant.LoadDefaultTenantInContext(t, dbConnectionPool)
	ctx = sdpcontext.SetUserIDInContext(ctx, "user-id")
	user := &auth.User{
		ID:    "user-id",
		Email: "email@email.com",
	}

	// setup fixtures
	wallets := data.ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)
	enabledWallet := wallets[0]
	disabledWallet := wallets[1]
	data.EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, false, disabledWallet.ID)

	embeddedWallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Embedded Wallet", "https://embedded.example.com", "embedded.example.com", "embedded://")
	data.MakeWalletEmbedded(t, ctx, dbConnectionPool, embeddedWallet.ID)
	embeddedWallet = data.GetWalletFixture(t, ctx, dbConnectionPool, embeddedWallet.Name)

	userManagedWallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "User Managed Wallet", "stellar.org", "stellar.org", "stellar://")
	data.MakeWalletUserManaged(t, ctx, dbConnectionPool, userManagedWallet.ID)
	userManagedWallet = data.GetWalletFixture(t, ctx, dbConnectionPool, userManagedWallet.Name)

	enabledWallet.Assets = nil
	asset := data.GetAssetFixture(t, ctx, dbConnectionPool, data.FixtureAssetUSDC)

	existingDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:   "existing disbursement",
		Asset:  asset,
		Wallet: &enabledWallet,
	})

	type TestCase struct {
		name               string
		prepareMocksFn     func(t *testing.T, mMonitorService *monitorMocks.MockMonitorService)
		reqBody            map[string]interface{}
		wantStatusCode     int
		wantResponseBodyFn func(d *data.Disbursement) string
	}
	testCases := []TestCase{
		{
			name:           "🔴 body parameters are missing",
			wantStatusCode: http.StatusBadRequest,
			wantResponseBodyFn: func(d *data.Disbursement) string {
				return `{
					"error": "The request was invalid in some way.",
					"extras": {
						"name": "name is required",
						"wallet_id": "wallet_id is required",
						"asset_id": "asset_id is required",
						"registration_contact_type": "registration_contact_type must be one of [EMAIL PHONE_NUMBER EMAIL_AND_WALLET_ADDRESS PHONE_NUMBER_AND_WALLET_ADDRESS]",
						"verification_field": "verification_field must be one of [DATE_OF_BIRTH YEAR_MONTH PIN NATIONAL_ID_NUMBER]"
					}
				}`
			},
		},
		{
			name: "🔴 wallet_id could not be found",
			reqBody: map[string]interface{}{
				"name":                      "disbursement 1",
				"asset_id":                  asset.ID,
				"wallet_id":                 "not-found-wallet-id",
				"registration_contact_type": data.RegistrationContactTypePhone,
				"verification_field":        data.VerificationTypeDateOfBirth,
			},
			wantStatusCode: http.StatusBadRequest,
			wantResponseBodyFn: func(d *data.Disbursement) string {
				return `{
					"error": "The request was invalid in some way.",
					"extras": {
						"wallet_id": "wallet_id could not be retrieved"
					}
				}`
			},
		},
		{
			name: "🔴 wallet is not enabled",
			reqBody: map[string]interface{}{
				"name":                      "disbursement 1",
				"asset_id":                  asset.ID,
				"wallet_id":                 disabledWallet.ID,
				"registration_contact_type": data.RegistrationContactTypePhone,
				"verification_field":        data.VerificationTypeDateOfBirth,
			},
			wantStatusCode: http.StatusBadRequest,
			wantResponseBodyFn: func(d *data.Disbursement) string {
				return `{"error":"Wallet is not enabled"}`
			},
		},
		{
			name: "🔴 asset_id could not be found",
			reqBody: map[string]interface{}{
				"name":                      "disbursement 1",
				"asset_id":                  "not-found-asset-id",
				"wallet_id":                 enabledWallet.ID,
				"registration_contact_type": data.RegistrationContactTypePhone,
				"verification_field":        data.VerificationTypeDateOfBirth,
			},
			wantStatusCode: http.StatusBadRequest,
			wantResponseBodyFn: func(d *data.Disbursement) string {
				return `{"error":"asset ID could not be retrieved"}`
			},
		},
		{
			name: "🔴 non-unique disbursement name",
			reqBody: map[string]interface{}{
				"name":                      existingDisbursement.Name,
				"asset_id":                  asset.ID,
				"wallet_id":                 enabledWallet.ID,
				"registration_contact_type": data.RegistrationContactTypePhone,
				"verification_field":        data.VerificationTypeDateOfBirth,
			},
			wantStatusCode: http.StatusConflict,
			wantResponseBodyFn: func(d *data.Disbursement) string {
				return `{"error":"disbursement already exists"}`
			},
		},
	}

	// Add successful testCases
	for i, registrationContactType := range data.AllRegistrationContactTypes() {
		var customInviteTemplate string
		var testNameSuffix string
		var wallet data.Wallet
		if i%2 == 0 {
			customInviteTemplate = "You have a new payment waiting for you from org x. Click on the link to register."
			testNameSuffix = "(w/ custom invite template)"
		}
		if registrationContactType.IncludesWalletAddress {
			wallet = *userManagedWallet
		} else {
			wallet = enabledWallet
		}

		successfulTestCase := TestCase{
			name: fmt.Sprintf("🟢[%s]registration_contact_type%s", registrationContactType, testNameSuffix),
			prepareMocksFn: func(t *testing.T, mMonitorService *monitorMocks.MockMonitorService) {
				labels := monitor.DisbursementLabels{
					Asset:  asset.Code,
					Wallet: wallet.Name,
					CommonLabels: monitor.CommonLabels{
						TenantName: "default-tenant",
					},
				}
				mMonitorService.On("MonitorCounters", monitor.DisbursementsCounterTag, labels.ToMap()).Return(nil).Once()
			},
			reqBody: map[string]interface{}{
				"name":                                   fmt.Sprintf("successful disbursement %d", i),
				"asset_id":                               asset.ID,
				"registration_contact_type":              registrationContactType.String(),
				"receiver_registration_message_template": customInviteTemplate,
			},
			wantStatusCode: http.StatusCreated,
			wantResponseBodyFn: func(d *data.Disbursement) string {
				respMap := map[string]interface{}{
					"created_at":                             d.CreatedAt.Format(time.RFC3339Nano),
					"id":                                     d.ID,
					"name":                                   fmt.Sprintf("successful disbursement %d", i),
					"receiver_registration_message_template": customInviteTemplate,
					"registration_contact_type":              registrationContactType.String(),
					"updated_at":                             d.UpdatedAt.Format(time.RFC3339Nano),
					"status":                                 data.DraftDisbursementStatus,
					"status_history": []map[string]interface{}{
						{
							"status":    data.DraftDisbursementStatus,
							"timestamp": d.StatusHistory[0].Timestamp,
							"user_id":   user.ID,
						},
					},
					"asset": map[string]interface{}{
						"code":       asset.Code,
						"id":         asset.ID,
						"issuer":     asset.Issuer,
						"created_at": asset.CreatedAt.Format(time.RFC3339Nano),
						"updated_at": asset.UpdatedAt.Format(time.RFC3339Nano),
						"deleted_at": nil,
					},
					"wallet": map[string]interface{}{
						"id":                   wallet.ID,
						"name":                 wallet.Name,
						"deep_link_schema":     wallet.DeepLinkSchema,
						"homepage":             wallet.Homepage,
						"sep_10_client_domain": wallet.SEP10ClientDomain,
						"created_at":           wallet.CreatedAt.Format(time.RFC3339Nano),
						"updated_at":           wallet.UpdatedAt.Format(time.RFC3339Nano),
						"enabled":              true,
					},
				}

				if !registrationContactType.IncludesWalletAddress {
					respMap["verification_field"] = data.VerificationTypeDateOfBirth
				} else {
					respMap["wallet"].(map[string]interface{})["user_managed"] = true
				}

				resp, err := json.Marshal(respMap)
				require.NoError(t, err)
				return string(resp)
			},
		}

		if !registrationContactType.IncludesWalletAddress {
			successfulTestCase.reqBody["wallet_id"] = wallet.ID
			successfulTestCase.reqBody["verification_field"] = data.VerificationTypeDateOfBirth
		}
		testCases = append(testCases, successfulTestCase)
	}

	embeddedCases := []struct {
		name          string
		verification  data.VerificationType
		responseLabel string
	}{
		{
			name:          "🟢 embedded wallet allows no verification",
			verification:  data.VerificationType(""),
			responseLabel: "embedded empty verification",
		},
		{
			name:          "🟢 embedded wallet accepts verification",
			verification:  data.VerificationTypeDateOfBirth,
			responseLabel: "embedded with verification",
		},
	}

	for _, embeddedCase := range embeddedCases {
		caseCopy := embeddedCase
		prepare := func(t *testing.T, mMonitorService *monitorMocks.MockMonitorService) {
			labels := monitor.DisbursementLabels{
				Asset:  asset.Code,
				Wallet: embeddedWallet.Name,
				CommonLabels: monitor.CommonLabels{
					TenantName: "default-tenant",
				},
			}
			mMonitorService.On("MonitorCounters", monitor.DisbursementsCounterTag, labels.ToMap()).Return(nil).Once()
		}

		reqBody := map[string]interface{}{
			"name":                      caseCopy.responseLabel,
			"asset_id":                  asset.ID,
			"wallet_id":                 embeddedWallet.ID,
			"registration_contact_type": data.RegistrationContactTypePhone.String(),
		}
		if caseCopy.verification != "" {
			reqBody["verification_field"] = caseCopy.verification
		}

		wantFn := func(d *data.Disbursement) string {
			require.NotNil(t, d)
			require.Equal(t, caseCopy.verification, d.VerificationField)
			wallet := d.Wallet
			assetResp := d.Asset
			respMap := map[string]interface{}{
				"created_at":                             d.CreatedAt.Format(time.RFC3339Nano),
				"id":                                     d.ID,
				"name":                                   caseCopy.responseLabel,
				"receiver_registration_message_template": "",
				"registration_contact_type":              data.RegistrationContactTypePhone.String(),
				"updated_at":                             d.UpdatedAt.Format(time.RFC3339Nano),
				"status":                                 data.DraftDisbursementStatus,
				"status_history": []map[string]interface{}{
					{
						"status":    data.DraftDisbursementStatus,
						"timestamp": d.StatusHistory[0].Timestamp,
						"user_id":   user.ID,
					},
				},
				"asset": map[string]interface{}{
					"code":       assetResp.Code,
					"id":         assetResp.ID,
					"issuer":     assetResp.Issuer,
					"created_at": assetResp.CreatedAt.Format(time.RFC3339Nano),
					"updated_at": assetResp.UpdatedAt.Format(time.RFC3339Nano),
					"deleted_at": nil,
				},
				"wallet": map[string]interface{}{
					"id":                   wallet.ID,
					"name":                 wallet.Name,
					"deep_link_schema":     wallet.DeepLinkSchema,
					"homepage":             wallet.Homepage,
					"sep_10_client_domain": wallet.SEP10ClientDomain,
					"created_at":           wallet.CreatedAt.Format(time.RFC3339Nano),
					"updated_at":           wallet.UpdatedAt.Format(time.RFC3339Nano),
					"enabled":              wallet.Enabled,
					"embedded":             wallet.Embedded,
				},
			}
			if caseCopy.verification != "" {
				respMap["verification_field"] = caseCopy.verification
			}

			resp, err := json.Marshal(respMap)
			require.NoError(t, err)
			return string(resp)
		}

		testCases = append(testCases, TestCase{
			name:               caseCopy.name,
			prepareMocksFn:     prepare,
			reqBody:            reqBody,
			wantStatusCode:     http.StatusCreated,
			wantResponseBodyFn: wantFn,
		})
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mAuthManager := &auth.AuthManagerMock{}
			mAuthManager.
				On("GetUserByID", mock.Anything, "user-id").
				Return(user, nil)
			mMonitorService := monitorMocks.NewMockMonitorService(t)
			if tc.prepareMocksFn != nil {
				tc.prepareMocksFn(t, mMonitorService)
			}

			handler := &DisbursementHandler{
				Models:         models,
				AuthManager:    mAuthManager,
				MonitorService: mMonitorService,
			}

			requestBody, err := json.Marshal(tc.reqBody)
			require.NoError(t, err)
			rr := httptest.NewRecorder()
			req, err := http.NewRequestWithContext(ctx, "POST", "/disbursements", bytes.NewReader(requestBody))
			require.NoError(t, err)
			http.HandlerFunc(handler.PostDisbursement).ServeHTTP(rr, req)
			resp := rr.Result()
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			require.Equalf(t, tc.wantStatusCode, resp.StatusCode, "status code doesn't match and here's the response body: %s", respBody)
			var actualDisbursement *data.Disbursement
			if tc.wantResponseBodyFn != nil {
				require.NoError(t, json.Unmarshal(respBody, &actualDisbursement))
			}

			wantBody := tc.wantResponseBodyFn(actualDisbursement)
			assert.JSONEq(t, wantBody, string(respBody))
		})
	}
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
		Models:                        models,
		DisbursementManagementService: &services.DisbursementManagementService{Models: models},
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
			name: "returns error when page_limit is zero",
			queryParams: map[string]string{
				"page_limit": "0",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"request invalid", "extras":{"page_limit":"parameter must be a positive integer"}}`,
		},
		{
			name: "returns error when page_limit exceeds max",
			queryParams: map[string]string{
				"page_limit": fmt.Sprintf("%d", validators.MaxPageLimit+1),
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   fmt.Sprintf(`{"error":"request invalid", "extras":{"page_limit":"parameter must be less than or equal to %d"}}`, validators.MaxPageLimit),
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
		Models:      models,
		AuthManager: authManagerMock,
		DisbursementManagementService: &services.DisbursementManagementService{
			Models:      models,
			AuthManager: authManagerMock,
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(handler.GetDisbursements))
	defer ts.Close()

	ctx := context.Background()

	// create fixtures
	wallet := data.CreateDefaultWalletFixture(t, ctx, dbConnectionPool)
	asset := data.GetAssetFixture(t, ctx, dbConnectionPool, data.FixtureAssetUSDC)

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
		On("GetUsersByID", mock.Anything, []string{createdByUser.ID, startedByUser.ID}, false).
		Return(allUsers, nil)
	authManagerMock.
		On("GetUsersByID", mock.Anything, []string{startedByUser.ID, createdByUser.ID}, false).
		Return(allUsers, nil)
	authManagerMock.
		On("GetUsersByID", mock.Anything, []string{createdByUser.ID}, false).
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
		CreatedAt:     testutils.TimePtr(time.Date(2022, 3, 21, 23, 40, 20, 1431, time.UTC)),
	})
	disbursement2 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:          "disbursement 2",
		Status:        data.ReadyDisbursementStatus,
		StatusHistory: draftStatusHistory,
		Asset:         asset,
		Wallet:        wallet,
		CreatedAt:     testutils.TimePtr(time.Date(2023, 2, 20, 23, 40, 20, 1431, time.UTC)),
	})
	disbursement3 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:          "disbursement 3",
		Status:        data.StartedDisbursementStatus,
		StatusHistory: startedStatusHistory,
		Asset:         asset,
		Wallet:        wallet,
		CreatedAt:     testutils.TimePtr(time.Date(2023, 3, 19, 23, 40, 20, 1431, time.UTC)),
	})
	disbursement4 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:          "disbursement 4",
		Status:        data.DraftDisbursementStatus,
		StatusHistory: draftStatusHistory,
		Asset:         asset,
		Wallet:        wallet,
		CreatedAt:     testutils.TimePtr(time.Date(2023, 4, 19, 23, 40, 20, 1431, time.UTC)),
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

	mMonitorService := monitorMocks.NewMockMonitorService(t)

	_, ctx := tenant.LoadDefaultTenantInContext(t, dbConnectionPool)
	ctx = sdpcontext.SetUserIDInContext(ctx, "user-id")
	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.
		On("GetUserByID", mock.Anything, mock.Anything).
		Return(&auth.User{
			ID:    "user-id",
			Email: "email@email.com",
		}, nil).
		Run(func(args mock.Arguments) {
			mockCtx := args.Get(0).(context.Context)
			val, err := sdpcontext.GetUserIDFromContext(mockCtx)
			assert.NoError(t, err)
			assert.Equal(t, "user-id", val)
		})

	handler := &DisbursementHandler{
		Models:         models,
		MonitorService: mMonitorService,
		AuthManager:    authManagerMock,
	}

	// create fixtures
	wallet := data.CreateDefaultWalletFixture(t, ctx, dbConnectionPool)
	asset := data.GetAssetFixture(t, ctx, dbConnectionPool, data.FixtureAssetUSDC)

	// create disbursement
	phoneDraftDisbursement := data.CreateDraftDisbursementFixture(t, ctx, dbConnectionPool, handler.Models.Disbursements, data.Disbursement{
		Name:   "disbursement1",
		Asset:  asset,
		Wallet: wallet,
	})

	phoneWalletDraftDisbursement := data.CreateDraftDisbursementFixture(t, ctx, dbConnectionPool, handler.Models.Disbursements, data.Disbursement{
		Name:                    "disbursement with phone and wallet",
		Asset:                   asset,
		Wallet:                  wallet,
		RegistrationContactType: data.RegistrationContactTypePhoneAndWalletAddress,
	})

	emailDraftDisbursement := data.CreateDraftDisbursementFixture(t, ctx, dbConnectionPool, handler.Models.Disbursements, data.Disbursement{
		Name:                    "disbursement with emails",
		Asset:                   asset,
		Wallet:                  wallet,
		RegistrationContactType: data.RegistrationContactTypeEmail,
	})

	emailWalletDraftDisbursement := data.CreateDraftDisbursementFixture(t, ctx, dbConnectionPool, handler.Models.Disbursements, data.Disbursement{
		Name:                    "disbursement with emails and wallets",
		Asset:                   asset,
		Wallet:                  wallet,
		RegistrationContactType: data.RegistrationContactTypeEmailAndWalletAddress,
	})

	startedDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, handler.Models.Disbursements, &data.Disbursement{
		Name:      "disbursement 1",
		Status:    data.StartedDisbursementStatus,
		CreatedAt: testutils.TimePtr(time.Date(2022, 3, 21, 23, 40, 20, 1431, time.UTC)),
	})

	maxCSVRecords := [][]string{
		{"email", "id", "amount", "verification"},
	}
	for i := 0; i < 10001; i++ {
		email := fmt.Sprintf("user+%d@example.com", i)
		maxCSVRecords = append(maxCSVRecords, []string{
			email, "123456789", "100.5", "1990-01-01",
		})
	}

	type TestCase struct {
		name               string
		disbursementID     string
		multipartFieldName string
		actualFileName     string
		csvRecords         [][]string
		expectedStatus     int
		expectedMessage    string
	}
	testCases := []TestCase{
		{
			name:           fmt.Sprintf("🟢 valid input [%s]", data.RegistrationContactTypePhone),
			disbursementID: phoneDraftDisbursement.ID,
			csvRecords: [][]string{
				{"phone", "id", "amount", "verification"},
				{"+380445555555", "123456789", "100.5", "1990-01-01"},
			},
			expectedStatus:  http.StatusOK,
			expectedMessage: "File uploaded successfully",
		},
		{
			name:           fmt.Sprintf("🟢 valid input [%s]", data.RegistrationContactTypePhoneAndWalletAddress),
			disbursementID: phoneWalletDraftDisbursement.ID,
			csvRecords: [][]string{
				{"phone", "walletAddress", "walletAddressMemo", "id", "amount"},
				{"+380445555555", "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", "valid-memo-text", "123456789", "100.5"},
			},
			expectedStatus:  http.StatusOK,
			expectedMessage: "File uploaded successfully",
		},
		{
			name:           fmt.Sprintf("🟢 valid input [%s]", data.RegistrationContactTypeEmail),
			disbursementID: emailDraftDisbursement.ID,
			csvRecords: [][]string{
				{"email", "id", "amount", "verification"},
				{"foobar@test.com", "123456789", "100.5", "1990-01-01"},
			},
			expectedStatus:  http.StatusOK,
			expectedMessage: "File uploaded successfully",
		},
		{
			name:           fmt.Sprintf("🟢 valid input [%s]", data.RegistrationContactTypeEmailAndWalletAddress),
			disbursementID: emailWalletDraftDisbursement.ID,
			csvRecords: [][]string{
				{"email", "walletAddress", "walletAddressMemo", "id", "amount"},
				{"foobar@test.com", "GB3SAK22KSTIFQAV5GCDNPW7RTQCWGFDKALBY5KJ3JRF2DLSED3E7PVH", "123456789", "123456789", "100.5"},
			},
			expectedStatus:  http.StatusOK,
			expectedMessage: "File uploaded successfully",
		},
		{
			name:           "🟢 valid input with BOM",
			disbursementID: phoneDraftDisbursement.ID,
			csvRecords: [][]string{
				{"\xef\xbb\xbf" + "phone", "id", "amount", "verification"},
				{"+380445555555", "123456789", "100.5", "1990-01-01"},
			},
			expectedStatus:  http.StatusOK,
			expectedMessage: "File uploaded successfully",
		},
		{
			name:           "🔴 csv upload too large",
			disbursementID: phoneDraftDisbursement.ID,
			csvRecords: [][]string{
				{"phone", "id", "amount", "verification"},
				{strings.Repeat("a", int(DefaultMaxCSVUploadSizeBytes)+1024*1024), "123456789", "100.5", "1990-01-01"},
			},
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "request too large",
		},
		{
			name:           "🔴 .bat is rejected",
			disbursementID: phoneDraftDisbursement.ID,
			csvRecords: [][]string{
				{"phone", "id", "amount", "verification"},
				{"+380445555555", "123456789", "100.5", "1990-01-01"},
			},
			actualFileName:  "file.bat",
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "the file extension should be .csv",
		},
		{
			name:           "🔴 .sh file is rejected",
			disbursementID: phoneDraftDisbursement.ID,
			csvRecords: [][]string{
				{"phone", "id", "amount", "verification"},
				{"+380445555555", "123456789", "100.5", "1990-01-01"},
			},
			actualFileName:  "file.sh",
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "the file extension should be .csv",
		},
		{
			name:           "🔴 .bash file is rejected",
			disbursementID: phoneDraftDisbursement.ID,
			csvRecords: [][]string{
				{"phone", "id", "amount", "verification"},
				{"+380445555555", "123456789", "100.5", "1990-01-01"},
			},
			actualFileName:  "file.bash",
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "the file extension should be .csv",
		},
		{
			name:           "🔴 .csv file with transversal path ..\\.. is rejected",
			disbursementID: phoneDraftDisbursement.ID,
			csvRecords: [][]string{
				{"phone", "id", "amount", "verification"},
				{"+380445555555", "123456789", "100.5", "1990-01-01"},
			},
			actualFileName:  "..\\..\\file.csv",
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "file name contains invalid traversal pattern",
		},
		{
			name:           "🔴 invalid date of birth",
			disbursementID: phoneDraftDisbursement.ID,
			csvRecords: [][]string{
				{"phone", "id", "amount", "verification"},
				{"+380445555555", "123456789", "100.5", "1990/01/01"},
			},
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "invalid date of birth format. Correct format: 1990-01-30",
		},
		{
			name:           "🔴 invalid phone number",
			disbursementID: phoneDraftDisbursement.ID,
			csvRecords: [][]string{
				{"phone", "id", "amount", "verification"},
				{"380-12-345-678", "123456789", "100.5", "1990-01-01"},
			},
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "invalid phone format. Correct format: +380445555555",
		},
		{
			name:            "🔴 invalid disbursement id",
			disbursementID:  "invalid",
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "disbursement ID is invalid",
		},
		{
			name:               "🔴 invalid input",
			disbursementID:     phoneDraftDisbursement.ID,
			multipartFieldName: "instructions",
			expectedStatus:     http.StatusBadRequest,
			expectedMessage:    "could not parse file",
		},
		{
			name:            "🔴 disbursement not in draft/ready status",
			disbursementID:  startedDisbursement.ID,
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "disbursement is not in draft or ready status",
		},
		{
			name:            "🔴 disbursement not in draft/ready state",
			disbursementID:  startedDisbursement.ID,
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "disbursement is not in draft or ready status",
		},
		{
			name:           "🔴 no instructions found in file",
			disbursementID: phoneDraftDisbursement.ID,
			csvRecords: [][]string{
				{"phone", "id", "amount", "verification"},
			},
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "could not parse csv file",
		},
		{
			name:           "🔴 columns invalid - email column missing for email contact type",
			disbursementID: emailDraftDisbursement.ID,
			csvRecords: [][]string{
				{"id", "amount", "verification"},
				{"123456789", "100.5", "1990-01-01"},
			},
			expectedStatus: http.StatusBadRequest,
			expectedMessage: fmt.Sprintf(
				"CSV columns are not valid for registration contact type %s: email column is required",
				data.RegistrationContactTypeEmail),
		},
		{
			name:           "🔴 columns invalid - email column not allowed for phone contact type",
			disbursementID: phoneDraftDisbursement.ID,
			csvRecords: [][]string{
				{"phone", "email", "id", "amount", "verification"},
				{"+380445555555", "foobar@test.com", "123456789", "100.5", "1990-01-01"},
			},
			expectedStatus: http.StatusBadRequest,
			expectedMessage: fmt.Sprintf(
				"CSV columns are not valid for registration contact type %s: email column is not allowed for this registration contact type",
				data.RegistrationContactTypePhone),
		},
		{
			name:           "🔴 columns invalid - phone column not allowed for email contact type",
			disbursementID: emailDraftDisbursement.ID,
			csvRecords: [][]string{
				{"phone", "email", "id", "amount", "verification"},
				{"+380445555555", "foobar@test.com", "123456789", "100.5", "1990-01-01"},
			},
			expectedStatus: http.StatusBadRequest,
			expectedMessage: fmt.Sprintf(
				"CSV columns are not valid for registration contact type %s: phone column is not allowed for this registration contact type",
				data.RegistrationContactTypeEmail),
		},
		{
			name:           "🔴 columns invalid - walletAddressMemo column not allowed for email contact type",
			disbursementID: emailDraftDisbursement.ID,
			csvRecords: [][]string{
				{"walletAddressMemo", "email", "id", "amount", "verification"},
				{"123456789", "foobar@test.com", "123456789", "100.5", "1990-01-01"},
			},
			expectedStatus: http.StatusBadRequest,
			expectedMessage: fmt.Sprintf(
				"CSV columns are not valid for registration contact type %s: walletAddressMemo column is not allowed for this registration contact type",
				data.RegistrationContactTypeEmail),
		},
		{
			name:           "🔴 columns invalid - wallet column missing for email-wallet contact type",
			disbursementID: emailWalletDraftDisbursement.ID,
			csvRecords: [][]string{
				{"email", "id", "amount"},
				{"foobar@test.com", "123456789", "100.5"},
			},
			expectedStatus: http.StatusBadRequest,
			expectedMessage: fmt.Sprintf(
				"CSV columns are not valid for registration contact type %s: walletAddress column is required",
				data.RegistrationContactTypeEmailAndWalletAddress),
		},
		{
			name:           "🔴 columns invalid - verification column not allowed for wallet contact type",
			disbursementID: emailWalletDraftDisbursement.ID,
			csvRecords: [][]string{
				{"walletAddress", "email", "id", "amount", "verification"},
				{"GB3SAK22KSTIFQAV5GCDNPW7RTQCWGFDKALBY5KJ3JRF2DLSED3E7PVH", "foobar@test.com", "123456789", "100.5", "1990-01-01"},
			},
			expectedStatus: http.StatusBadRequest,
			expectedMessage: fmt.Sprintf(
				"CSV columns are not valid for registration contact type %s: verification column is not allowed for this registration contact type",
				data.RegistrationContactTypeEmailAndWalletAddress),
		},
		{
			name:           "🔴 instructions invalid - walletAddress is invalid",
			disbursementID: emailWalletDraftDisbursement.ID,
			csvRecords: [][]string{
				{"walletAddress", "email", "id", "amount"},
				{"GB3SAK22KSTIFQAV5GKALBY5KJ3JRF2DLSED3E7PVH", "foobar@test.com", "123456789", "100.5"},
			},
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "invalid wallet address",
		},
		{
			name:           "🔴 instructions invalid - walletAddressMemo is invalid",
			disbursementID: emailWalletDraftDisbursement.ID,
			csvRecords: [][]string{
				{"walletAddress", "walletAddressMemo", "email", "id", "amount"},
				{"GDBILPMQKKR3UKJDVZO6KZPJ4YP4HGAW67EHNMBEZ4DILI2YVFUI43ST", "this-string-is-not-a-valid-memo-because-it", "foobar@test.com", "123456789", "100.5"},
			},
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "invalid wallet address memo",
		},
		{
			name:            "🔴 max instructions exceeded",
			disbursementID:  emailDraftDisbursement.ID,
			csvRecords:      maxCSVRecords,
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "number of instructions exceeds maximum of 10000",
		},
		{
			name:           "🔴 wallet address already in use by another receiver",
			disbursementID: emailWalletDraftDisbursement.ID,
			csvRecords: [][]string{
				{"email", "walletAddress", "id", "amount"},
				{"user1@example.com", "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", "123456789", "100.5"},
				{"user2@example.com", "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", "987654321", "200.0"},
			},
			expectedStatus:  http.StatusConflict,
			expectedMessage: "wallet address GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5 is already registered to another receiver: wallet address already in use",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fileContent := createCSVFile(t, tc.csvRecords)

			req := createInstructionsMultipartRequest(t, ctx, tc.multipartFieldName, tc.actualFileName, tc.disbursementID, fileContent)

			// Record the response
			rr := httptest.NewRecorder()
			router := chi.NewRouter()
			router.Post("/disbursements/{id}/instructions", handler.PostDisbursementInstructions)
			router.ServeHTTP(rr, req)

			// Check the response status and message
			bodyStr := rr.Body.String()
			assert.Equal(t, tc.expectedStatus, rr.Code, bodyStr)
			assert.Contains(t, bodyStr, tc.expectedMessage)
		})
		authManagerMock.AssertExpectations(t)
	}
}

func Test_validateCSVHeaders(t *testing.T) {
	makeReader := func(headers []string) io.Reader {
		var buf bytes.Buffer
		writer := csv.NewWriter(&buf)
		require.NoError(t, writer.Write(headers))
		writer.Flush()
		return bytes.NewReader(buf.Bytes())
	}

	testCases := []struct {
		name                   string
		headers                []string
		rct                    data.RegistrationContactType
		skipVerification       bool
		expectedErrorSubstring string
	}{
		{
			name:                   "phone contact requires verification when not skipped",
			headers:                []string{"phone"},
			rct:                    data.RegistrationContactTypePhone,
			skipVerification:       false,
			expectedErrorSubstring: "verification column is required",
		},
		{
			name:                   "email contact disallows verification header when skipped",
			headers:                []string{"email", "verification"},
			rct:                    data.RegistrationContactTypeEmail,
			skipVerification:       true,
			expectedErrorSubstring: "verification column is not allowed",
		},
		{
			name:             "phone and wallet contact does not require verification",
			headers:          []string{"phone", "walletAddress"},
			rct:              data.RegistrationContactTypePhoneAndWalletAddress,
			skipVerification: false,
		},
		{
			name:                   "phone and wallet contact disallows verification header",
			headers:                []string{"phone", "walletAddress", "verification"},
			rct:                    data.RegistrationContactTypePhoneAndWalletAddress,
			skipVerification:       false,
			expectedErrorSubstring: "verification column is not allowed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reader := makeReader(tc.headers)
			err := validateCSVHeaders(reader, tc.rct, tc.skipVerification)
			if tc.expectedErrorSubstring == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedErrorSubstring)
		})
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
		On("GetUsersByID", mock.Anything, []string{createdByUser.ID, startedByUser.ID}, false).
		Return(allUsers, nil)
	authManagerMock.
		On("GetUsersByID", mock.Anything, []string{startedByUser.ID, createdByUser.ID}, false).
		Return(allUsers, nil)

	handler := &DisbursementHandler{
		Models:      models,
		AuthManager: authManagerMock,
		DisbursementManagementService: &services.DisbursementManagementService{
			Models:      models,
			AuthManager: authManagerMock,
		},
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
		CreatedAt: testutils.TimePtr(time.Date(2022, 3, 21, 23, 40, 20, 1431, time.UTC)),
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
		Models:                        models,
		DisbursementManagementService: &services.DisbursementManagementService{Models: models},
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

	// create disbursements
	disbursementWithReceivers := data.CreateDisbursementFixture(t, context.Background(), dbConnectionPool, handler.Models.Disbursements, &data.Disbursement{
		Name:   "disbursement with receivers",
		Status: data.DraftDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
	})
	disbursementWithoutReceivers := data.CreateDisbursementFixture(t, context.Background(), dbConnectionPool, handler.Models.Disbursements, &data.Disbursement{
		Name:   "disbursement without receivers",
		Status: data.DraftDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
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
			Email:          receiver3.Email,
			ExternalID:     receiver3.ExternalID,
			ReceiverWallet: receiverWallet3,
			Payment:        payment3,
			CreatedAt:      *receiver3.CreatedAt,
			UpdatedAt:      *receiver3.UpdatedAt,
		},
		{
			ID:             receiver2.ID,
			PhoneNumber:    receiver2.PhoneNumber,
			Email:          receiver2.Email,
			ExternalID:     receiver2.ExternalID,
			ReceiverWallet: receiverWallet2,
			Payment:        payment2,
			CreatedAt:      *receiver2.CreatedAt,
			UpdatedAt:      *receiver2.UpdatedAt,
		},
		{
			ID:             receiver1.ID,
			PhoneNumber:    receiver1.PhoneNumber,
			Email:          receiver1.Email,
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
	_, ctx := tenant.LoadDefaultTenantInContext(t, dbConnectionPool)
	ctx = sdpcontext.SetTokenInContext(ctx, token)
	userID := "valid-user-id"
	ctx = sdpcontext.SetUserIDInContext(ctx, userID)
	user := &auth.User{
		ID:    userID,
		Email: "email@email.com",
	}
	require.NotNil(t, user)

	authManagerMock := &auth.AuthManagerMock{}
	mockDistAccSvc := svcMocks.NewMockDistributionAccountService(t)
	asset := data.GetAssetFixture(t, ctx, dbConnectionPool, data.FixtureAssetUSDC)

	defaultTenantDistAcc := "GDIVVKL6QYF6C6K3C5PZZBQ2NQDLN2OSLMVIEQRHS6DZE7WRL33ZDNXL"
	distAcc := schema.NewStellarEnvTransactionAccount(defaultTenantDistAcc)
	mockDistAccResolver := sigMocks.NewMockDistributionAccountResolver(t)

	handler := &DisbursementHandler{
		Models:                      models,
		AuthManager:                 authManagerMock,
		DistributionAccountResolver: mockDistAccResolver,
		DisbursementManagementService: &services.DisbursementManagementService{
			Models:                     models,
			AuthManager:                authManagerMock,
			DistributionAccountService: mockDistAccSvc,
		},
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

	t.Run("cannot get distribution account", func(t *testing.T) {
		authManagerMock.
			On("GetUserByID", mock.Anything, userID).
			Return(user, nil).
			Once()

		mockDistAccResolver.
			On("DistributionAccountFromContext", mock.Anything).
			Return(schema.TransactionAccount{}, errors.New("unexpected error")).
			Once()

		httpRouter := chi.NewRouter()
		httpRouter.Patch("/disbursements/{id}/status", handler.PatchDisbursementStatus)

		err := json.NewEncoder(reqBody).Encode(PatchDisbursementStatusRequest{Status: "STARTED"})
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, fmt.Sprintf("/disbursements/%s/status", draftDisbursement.ID), reqBody)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		httpRouter.ServeHTTP(rr, req)

		require.Equal(t, http.StatusInternalServerError, rr.Code)
		require.Contains(t, rr.Body.String(), "Cannot get distribution account")
	})

	t.Run("disbursement not ready to start", func(t *testing.T) {
		authManagerMock.
			On("GetUserByID", mock.Anything, userID).
			Return(user, nil).
			Once()

		mockDistAccResolver.
			On("DistributionAccountFromContext", mock.Anything).
			Return(distAcc, nil).
			Once()

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
			On("GetUserByID", mock.Anything, userID).
			Return(user, nil).
			Once()

		mockDistAccResolver.
			On("DistributionAccountFromContext", mock.Anything).
			Return(distAcc, nil).
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
		wallet := data.CreateDefaultWalletFixture(t, ctx, dbConnectionPool)
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.DraftReceiversWalletStatus)
		data.CreatePaymentFixture(t, ctx, dbConnectionPool, handler.Models.Payment, &data.Payment{
			ReceiverWallet: receiverWallet,
			Disbursement:   readyDisbursement,
			Asset:          *asset,
			Amount:         "300",
			Status:         data.DraftPaymentStatus,
		})

		approverUser := &auth.User{
			ID:    "valid-approver-user-id",
			Email: "approver@mail.org",
		}

		// Create a context with the approver's userID for this test
		approverCtx := sdpcontext.SetUserIDInContext(ctx, approverUser.ID)

		authManagerMock.
			On("GetUserByID", mock.Anything, approverUser.ID).
			Return(approverUser, nil).
			Once()

		mockDistAccResolver.
			On("DistributionAccountFromContext", mock.Anything).
			Return(distAcc, nil).
			Once()

		mockDistAccSvc.On("GetBalance", mock.Anything, &distAcc, mock.AnythingOfType("data.Asset")).
			Return(decimal.NewFromFloat(10000.0), nil).Once()

		err := json.NewEncoder(reqBody).Encode(PatchDisbursementStatusRequest{Status: "Started"})
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(approverCtx, http.MethodPatch, fmt.Sprintf("/disbursements/%s/status", readyDisbursement.ID), reqBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		require.Contains(t, rr.Body.String(), "Disbursement started")
	})

	t.Run("disbursement started - then paused", func(t *testing.T) {
		authManagerMock.
			On("GetUserByID", mock.Anything, userID).
			Return(user, nil).
			Twice()

		mockDistAccResolver.
			On("DistributionAccountFromContext", mock.Anything).
			Return(distAcc, nil).
			Once()

		mockDistAccSvc.On("GetBalance", mock.Anything, &distAcc, mock.AnythingOfType("data.Asset")).
			Return(decimal.NewFromFloat(10000.0), nil).Once()

		readyDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, handler.Models.Disbursements, &data.Disbursement{
			Name:          "ready disbursement #3",
			Status:        data.ReadyDisbursementStatus,
			StatusHistory: readyStatusHistory,
		})
		wallet := data.CreateDefaultWalletFixture(t, ctx, dbConnectionPool)
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.DraftReceiversWalletStatus)
		data.CreatePaymentFixture(t, ctx, dbConnectionPool, handler.Models.Payment, &data.Payment{
			ReceiverWallet: receiverWallet,
			Disbursement:   readyDisbursement,
			Asset:          *asset,
			Amount:         "300",
			Status:         data.DraftPaymentStatus,
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
		authManagerMock.
			On("GetUserByID", mock.Anything, userID).
			Return(user, nil).
			Once()

		err := json.NewEncoder(reqBody).Encode(PatchDisbursementStatusRequest{Status: "Paused"})
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, fmt.Sprintf("/disbursements/%s/status", draftDisbursement.ID), reqBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Contains(t, rr.Body.String(), services.ErrDisbursementNotReadyToPause.Error())
	})

	t.Run("disbursement status can't be changed", func(t *testing.T) {
		authManagerMock.
			On("GetUserByID", mock.Anything, userID).
			Return(user, nil).
			Once()

		err := json.NewEncoder(reqBody).Encode(PatchDisbursementStatusRequest{Status: "Completed"})
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, fmt.Sprintf("/disbursements/%s/status", draftDisbursement.ID), reqBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Contains(t, rr.Body.String(), services.ErrDisbursementStatusCantBeChanged.Error())
	})

	t.Run("disbursement doesn't exist", func(t *testing.T) {
		authManagerMock.
			On("GetUserByID", mock.Anything, userID).
			Return(user, nil).
			Once()

		mockDistAccResolver.
			On("DistributionAccountFromContext", mock.Anything).
			Return(distAcc, nil).
			Once()

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
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, outerErr := data.NewModels(dbConnectionPool)
	require.NoError(t, outerErr)

	handler := &DisbursementHandler{Models: models}
	r := chi.NewRouter()
	r.Get("/disbursements/{id}/instructions", handler.GetDisbursementInstructions)

	disbursementFileContent := data.CreateInstructionsFixture(t, []*data.DisbursementInstruction{
		{Phone: "1234567890", ID: "1", Amount: "123.12", VerificationValue: "1995-02-20"},
		{Phone: "0987654321", ID: "2", Amount: "321", VerificationValue: "1974-07-19"},
		{Phone: "0987654321", ID: "3", Amount: "321", VerificationValue: "1974-07-19"},
	})
	d := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{})
	require.NotNil(t, d)

	testCases := []struct {
		name                 string
		updateDisbursementFn func(d *data.Disbursement) error
		getDisbursementIDFn  func(d *data.Disbursement) string
		expectedStatus       int
		expectedErrMessage   string
		wantFilename         string
	}{
		{
			name: "404-disbursement doesn't exist",
			getDisbursementIDFn: func(d *data.Disbursement) string {
				return "non-existent-disbursement-id"
			},
			expectedStatus:     http.StatusNotFound,
			expectedErrMessage: services.ErrDisbursementNotFound.Error(),
		},
		{
			name:                "404-disbursement has no instructions",
			getDisbursementIDFn: func(d *data.Disbursement) string { return d.ID },
			expectedStatus:      http.StatusNotFound,
			expectedErrMessage:  "disbursement " + d.ID + " has no instructions file",
		},
		{
			name: "200-disbursement has instructions",
			updateDisbursementFn: func(d *data.Disbursement) error {
				return models.Disbursements.Update(ctx, dbConnectionPool, &data.DisbursementUpdate{
					ID:          d.ID,
					FileContent: disbursementFileContent,
					FileName:    "instructions.csv",
				})
			},
			wantFilename:        "instructions.csv",
			getDisbursementIDFn: func(d *data.Disbursement) string { return d.ID },
			expectedStatus:      http.StatusOK,
		},
		{
			name: "200-disbursement has instructions but filename is missing .csv",
			updateDisbursementFn: func(d *data.Disbursement) error {
				return models.Disbursements.Update(ctx, dbConnectionPool, &data.DisbursementUpdate{
					ID:          d.ID,
					FileContent: disbursementFileContent,
					FileName:    "instructions.bat",
				})
			},
			wantFilename:        "instructions.bat.csv",
			getDisbursementIDFn: func(d *data.Disbursement) string { return d.ID },
			expectedStatus:      http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.updateDisbursementFn != nil {
				require.NoError(t, tc.updateDisbursementFn(d))
			}

			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/disbursements/%s/instructions", tc.getDisbursementIDFn(d)), nil)
			require.NoError(t, err)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			require.Equal(t, tc.expectedStatus, rr.Code)
			if tc.expectedStatus != http.StatusOK {
				require.Contains(t, rr.Body.String(), tc.expectedErrMessage)
			} else {
				t.Log(rr.Header())
				require.Equal(t, "text/csv", rr.Header().Get("Content-Type"))
				require.Equal(t, "attachment; filename=\""+tc.wantFilename+"\"", rr.Header().Get("Content-Disposition"))
				require.Equal(t, string(disbursementFileContent), rr.Body.String())
			}
		})
	}
}

func Test_DisbursementHandler_DeleteDisbursement(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	models, outerErr := data.NewModels(dbConnectionPool)
	require.NoError(t, outerErr)

	ctx := context.Background()
	handler := &DisbursementHandler{
		Models: models,
	}

	r := chi.NewRouter()
	r.Delete("/disbursements/{id}", handler.DeleteDisbursement)

	// Create test fixtures
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	t.Run("successfully deletes draft disbursement", func(t *testing.T) {
		disbursement := data.CreateDraftDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, data.Disbursement{
			Name:   uuid.NewString(),
			Asset:  asset,
			Wallet: wallet,
		})

		req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("/disbursements/%s", disbursement.ID), nil)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var response data.Disbursement
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
		assert.Equal(t, *disbursement, response)

		// Verify disbursement was deleted
		_, err = models.Disbursements.Get(ctx, dbConnectionPool, disbursement.ID)
		require.Error(t, err)
		assert.Equal(t, data.ErrRecordNotFound, err)
	})

	t.Run("successfully deletes ready disbursement", func(t *testing.T) {
		disbursement := data.CreateDraftDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, data.Disbursement{
			Name:   uuid.NewString(),
			Status: data.ReadyDisbursementStatus,
			Asset:  asset,
			Wallet: wallet,
		})

		req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("/disbursements/%s", disbursement.ID), nil)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var response data.Disbursement
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
		assert.Equal(t, *disbursement, response)

		// Verify disbursement was deleted
		_, err = models.Disbursements.Get(ctx, dbConnectionPool, disbursement.ID)
		require.Error(t, err)
		assert.Equal(t, data.ErrRecordNotFound, err)
	})

	t.Run("returns 404 when disbursement not found", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodDelete, "/disbursements/non-existent-id", nil)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("returns 400 when disbursement is not in draft status", func(t *testing.T) {
		disbursement := data.CreateDraftDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, data.Disbursement{
			Name:   uuid.NewString(),
			Status: data.StartedDisbursementStatus,
			Asset:  asset,
			Wallet: wallet,
		})

		req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("/disbursements/%s", disbursement.ID), nil)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "Cannot delete a disbursement that has started")

		// Verify disbursement still exists
		_, err = models.Disbursements.Get(ctx, dbConnectionPool, disbursement.ID)
		require.NoError(t, err)
	})

	t.Run("returns error when disbursement has associated payments", func(t *testing.T) {
		disbursement := data.CreateDraftDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, data.Disbursement{
			Name:   uuid.NewString(),
			Asset:  asset,
			Wallet: wallet,
		})

		// Create a receiver and receiver wallet
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.DraftReceiversWalletStatus)

		// Create an associated payment
		data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id",
			StellarOperationID:   "operation-id",
			Status:               data.SuccessPaymentStatus,
			Disbursement:         disbursement,
			Asset:                *asset,
			ReceiverWallet:       receiverWallet,
		})

		req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("/disbursements/%s", disbursement.ID), nil)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.Contains(t, rr.Body.String(), "Cannot delete disbursement")

		// Verify disbursement still exists
		_, err = models.Disbursements.Get(ctx, dbConnectionPool, disbursement.ID)
		require.NoError(t, err)
	})
}

func Test_DisbursementHandler_PostDisbursement_WithInstructions(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	_, ctx := tenant.LoadDefaultTenantInContext(t, dbConnectionPool)
	ctx = sdpcontext.SetUserIDInContext(ctx, "user-id")

	// Setup fixtures
	wallets := data.ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)
	enabledWallet := wallets[0]
	disabledWallet := wallets[1]
	data.EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, false, disabledWallet.ID)

	userManagedWallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "User Managed Wallet", "stellar.org", "stellar.org", "stellar://")
	data.MakeWalletUserManaged(t, ctx, dbConnectionPool, userManagedWallet.ID)

	asset := data.GetAssetFixture(t, ctx, dbConnectionPool, data.FixtureAssetUSDC)

	embeddedWallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Embedded Wallet", "https://embedded.example.com", "embedded.example.com", "embedded://")
	data.MakeWalletEmbedded(t, ctx, dbConnectionPool, embeddedWallet.ID)
	data.CreateWalletAssets(t, ctx, dbConnectionPool, embeddedWallet.ID, []string{asset.ID})

	walletNamesByID := map[string]string{
		enabledWallet.ID:     enabledWallet.Name,
		disabledWallet.ID:    disabledWallet.Name,
		userManagedWallet.ID: userManagedWallet.Name,
		embeddedWallet.ID:    embeddedWallet.Name,
	}

	// Setup Mocks
	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.
		On("GetUserByID", mock.Anything, mock.Anything).
		Return(&auth.User{
			ID:    "user-id",
			Email: "email@email.com",
		}, nil).
		Run(func(args mock.Arguments) {
			mockCtx := args.Get(0).(context.Context)
			val, err := sdpcontext.GetUserIDFromContext(mockCtx)
			assert.NoError(t, err)
			assert.Equal(t, "user-id", val)
		})

	mMonitorService := monitorMocks.NewMockMonitorService(t)

	// Setup handler
	handler := &DisbursementHandler{
		Models:         models,
		MonitorService: mMonitorService,
		AuthManager:    authManagerMock,
	}

	// Test cases combining disbursement creation and instruction validation
	testCases := []struct {
		name             string
		disbursementData map[string]interface{}
		csvRecords       [][]string
		expectedStatus   int
		expectedMessage  string
	}{
		{
			name: "🟢 embedded wallet without verification field",
			disbursementData: map[string]interface{}{
				"name":                      "embedded wallet without verification",
				"asset_id":                  asset.ID,
				"wallet_id":                 embeddedWallet.ID,
				"registration_contact_type": data.RegistrationContactTypePhone,
			},
			csvRecords: [][]string{
				{"phone", "id", "amount"},
				{"+380445555555", "123456789", "100.5"},
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "🟢 successful creation with phone verification",
			disbursementData: map[string]interface{}{
				"name":                      "disbursement with phone",
				"asset_id":                  asset.ID,
				"wallet_id":                 enabledWallet.ID,
				"registration_contact_type": data.RegistrationContactTypePhone,
				"verification_field":        data.VerificationTypeDateOfBirth,
			},
			csvRecords: [][]string{
				{"phone", "id", "amount", "verification"},
				{"+380445555555", "123456789", "100.5", "1990-01-01"},
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "🟢 successful creation with email verification",
			disbursementData: map[string]interface{}{
				"name":                      "disbursement with email",
				"asset_id":                  asset.ID,
				"wallet_id":                 enabledWallet.ID,
				"registration_contact_type": data.RegistrationContactTypeEmail,
				"verification_field":        data.VerificationTypeDateOfBirth,
			},
			csvRecords: [][]string{
				{"email", "id", "amount", "verification"},
				{"test@example.com", "123456789", "100.5", "1990-01-01"},
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "🔴 disabled wallet",
			disbursementData: map[string]interface{}{
				"name":                      "disbursement with disabled wallet",
				"asset_id":                  asset.ID,
				"wallet_id":                 disabledWallet.ID,
				"registration_contact_type": data.RegistrationContactTypePhone,
				"verification_field":        data.VerificationTypeDateOfBirth,
			},
			csvRecords: [][]string{
				{"phone", "id", "amount", "verification"},
				{"+380445555555", "123456789", "100.5", "1990-01-01"},
			},
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "Wallet is not enabled",
		},
		{
			name: "🔴 invalid asset ID",
			disbursementData: map[string]interface{}{
				"name":                      "disbursement with invalid asset",
				"asset_id":                  "invalid-asset-id",
				"wallet_id":                 enabledWallet.ID,
				"registration_contact_type": data.RegistrationContactTypePhone,
				"verification_field":        data.VerificationTypeDateOfBirth,
			},
			csvRecords: [][]string{
				{"phone", "id", "amount", "verification"},
				{"+380445555555", "123456789", "100.5", "1990-01-01"},
			},
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "asset ID could not be retrieved",
		},
		{
			name: "🔴 invalid CSV format - missing required columns",
			disbursementData: map[string]interface{}{
				"name":                      "disbursement with invalid CSV",
				"asset_id":                  asset.ID,
				"wallet_id":                 enabledWallet.ID,
				"registration_contact_type": data.RegistrationContactTypePhone,
				"verification_field":        data.VerificationTypeDateOfBirth,
			},
			csvRecords: [][]string{
				{"id", "amount"}, // missing phone and verification
				{"123456789", "100.5"},
			},
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "CSV columns are not valid for registration contact type PHONE_NUMBER: phone column is required",
		},
		{
			name: "🔴 invalid phone format",
			disbursementData: map[string]interface{}{
				"name":                      "disbursement with invalid phone",
				"asset_id":                  asset.ID,
				"wallet_id":                 enabledWallet.ID,
				"registration_contact_type": data.RegistrationContactTypePhone,
				"verification_field":        data.VerificationTypeDateOfBirth,
			},
			csvRecords: [][]string{
				{"phone", "id", "amount", "verification"},
				{"123-456-7890", "123456789", "100.5", "1990-01-01"}, // invalid phone format
			},
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: "invalid phone format",
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			walletName := enabledWallet.Name
			if walletID, ok := tc.disbursementData["wallet_id"].(string); ok {
				if name, exists := walletNamesByID[walletID]; exists {
					walletName = name
				}
			}

			labels := monitor.DisbursementLabels{
				Asset:        asset.Code,
				Wallet:       walletName,
				CommonLabels: monitor.CommonLabels{TenantName: "default-tenant"},
			}

			mMonitorService.
				On("MonitorCounters", monitor.DisbursementsCounterTag, labels.ToMap()).
				Return(nil).
				Maybe()

			// Create multipart form data
			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)

			// Add disbursement data
			disbursementJSON, err := json.Marshal(tc.disbursementData)
			require.NoError(t, err)
			err = writer.WriteField("data", string(disbursementJSON))
			require.NoError(t, err)

			// Add CSV file if records are provided
			addInstructionsIfNeeded(t, tc.csvRecords, writer)

			err = writer.Close()
			require.NoError(t, err)

			// Create request
			req, err := http.NewRequestWithContext(ctx, "POST", "/disbursements", body)
			require.NoError(t, err)
			req.Header.Set("Content-Type", writer.FormDataContentType())

			// Record response
			rr := httptest.NewRecorder()
			router := chi.NewRouter()
			router.Post("/disbursements", handler.PostDisbursement)
			router.ServeHTTP(rr, req)

			// Verify response
			assert.Equal(t, tc.expectedStatus, rr.Code)

			// ❌Verify error response
			if tc.expectedMessage != "" {
				assert.Contains(t, rr.Body.String(), tc.expectedMessage)
			}

			// ✅Verify successful response
			if tc.expectedStatus == http.StatusCreated {
				var createdDisbursement data.Disbursement
				err = json.Unmarshal(rr.Body.Bytes(), &createdDisbursement)
				require.NoError(t, err)

				// Verify disbursement properties
				assert.Equal(t, tc.disbursementData["name"], createdDisbursement.Name)
				assert.Equal(t, tc.disbursementData["asset_id"], createdDisbursement.Asset.ID)
				assert.Equal(t, tc.disbursementData["wallet_id"], createdDisbursement.Wallet.ID)
				assert.Equal(t, data.DraftDisbursementStatus, createdDisbursement.Status)

				// Verify instructions were uploaded
				actualPayments, err := models.Payment.GetAll(ctx, &data.QueryParams{
					Query: createdDisbursement.Name,
				}, dbConnectionPool, data.QueryTypeSelectAll)
				require.NoError(t, err)
				expectedPayments := len(tc.csvRecords) - 1 // excluding header
				assert.Len(t, actualPayments, expectedPayments)
			}
		})
	}

	authManagerMock.AssertExpectations(t)
}

func addInstructionsIfNeeded(t *testing.T, csvRecords [][]string, writer *multipart.Writer) {
	t.Helper()

	if len(csvRecords) > 0 {
		part, err := writer.CreateFormFile("file", "instructions.csv")
		require.NoError(t, err)

		csvContent := createCSVFile(t, csvRecords)
		_, err = io.Copy(part, csvContent)
		require.NoError(t, err)
	}
}

func createCSVFile(t *testing.T, records [][]string) io.Reader {
	t.Helper()

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	for _, record := range records {
		err := writer.Write(record)
		require.NoError(t, err)
	}
	writer.Flush()
	return &buf
}

func createInstructionsMultipartRequest(t *testing.T, ctx context.Context, multipartFieldName, fileName, disbursementID string, fileContent io.Reader) *http.Request {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	if multipartFieldName == "" {
		multipartFieldName = "file"
	}

	if fileName == "" {
		fileName = "instructions.csv"
	}

	part, err := writer.CreateFormFile(multipartFieldName, fileName)
	require.NoError(t, err)

	_, err = io.Copy(part, fileContent)
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	url := fmt.Sprintf("/disbursements/%s/instructions", disbursementID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func buildURLWithQueryParams(baseURL, endpoint string, queryParams map[string]string) string {
	u, err := url.Parse(baseURL + endpoint)
	if err != nil {
		panic(fmt.Sprintf("invalid URL: %v", err))
	}

	q := u.Query()
	for k, v := range queryParams {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
