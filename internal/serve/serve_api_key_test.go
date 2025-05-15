package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	monitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type TestResources struct {
	DBPool     db.DBConnectionPool
	Wallet     *data.Wallet
	Asset      *data.Asset
	TestUserID string
}

func Test_handleHTTP_APIKeyAuthentication(t *testing.T) {
	dbConnectionPool := getConnectionPool(t)

	serveOptions := getServeOptionsForTests(t, dbConnectionPool)

	handlerMux := handleHTTP(serveOptions)

	testUserID := "00000000-0000-0000-0000-000000000000"

	validAPIKey := createTestAPIKey(t, dbConnectionPool, "Valid Admin Key",
		[]data.APIKeyPermission{data.ReadAll, data.WriteAll}, nil, 30, testUserID)

	readOnlyAPIKey := createTestAPIKey(t, dbConnectionPool, "Read Only Key",
		[]data.APIKeyPermission{data.ReadAll}, nil, 30, testUserID)

	expiredAPIKey := createTestAPIKey(t, dbConnectionPool, "Expired Key",
		[]data.APIKeyPermission{data.ReadAll, data.WriteAll}, nil, -1, testUserID)

	limitedIPAPIKey := createTestAPIKey(t, dbConnectionPool, "Limited IP Key",
		[]data.APIKeyPermission{data.ReadAll, data.WriteAll}, []string{"192.168.1.1"}, 30, testUserID)

	disbursementOnlyAPIKey := createTestAPIKey(t, dbConnectionPool, "Disbursement Only Key",
		[]data.APIKeyPermission{data.ReadDisbursements, data.WriteDisbursements}, nil, 30, testUserID)

	testCases := []struct {
		name           string
		method         string
		path           string
		apiKey         string
		remoteAddr     string
		expectedStatus int
	}{
		{
			name:           "valid API key with full permissions can access disbursements",
			method:         http.MethodGet,
			path:           "/disbursements",
			apiKey:         validAPIKey.Key,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "read-only API key cannot create disbursement",
			method:         http.MethodPost,
			path:           "/disbursements",
			apiKey:         readOnlyAPIKey.Key,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "expired API key is rejected",
			method:         http.MethodGet,
			path:           "/disbursements",
			apiKey:         expiredAPIKey.Key,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "IP restricted key is rejected from unauthorized IP",
			method:         http.MethodGet,
			path:           "/disbursements",
			apiKey:         limitedIPAPIKey.Key,
			remoteAddr:     "127.0.0.1:8080", // Different than allowed IP
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "disbursement-only key can access disbursements",
			method:         http.MethodGet,
			path:           "/disbursements",
			apiKey:         disbursementOnlyAPIKey.Key,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "disbursement-only key cannot access wallets",
			method:         http.MethodGet,
			path:           "/wallets",
			apiKey:         disbursementOnlyAPIKey.Key,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "malformed API key is rejected",
			method:         http.MethodGet,
			path:           "/disbursements",
			apiKey:         "SDP_INVALID_KEY",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set(middleware.TenantHeaderKey, "aid-org")
			req.Header.Set("Authorization", tc.apiKey)

			if tc.remoteAddr != "" {
				req.RemoteAddr = tc.remoteAddr
			}

			w := httptest.NewRecorder()
			handlerMux.ServeHTTP(w, req)

			resp := w.Result()
			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
		})
	}
}

func Test_handleHTTP_APIKeyReadAllPermissions(t *testing.T) {
	res := setupAPIKeyTestResources(t)

	authMock := &auth.AuthManagerMock{}
	usr := &auth.User{ID: res.TestUserID, Email: "inquisitor@ordohereticus.gov"}
	authMock.On("GetUserByID", mock.Anything, mock.Anything).Return(usr, nil)

	monitorMock := monitorMocks.NewMockMonitorService(t)
	monitorMock.
		On("MonitorCounters",
			monitor.DisbursementsCounterTag,
			mock.AnythingOfType("map[string]string"),
		).
		Return(nil).
		Maybe()
	monitorMock.
		On("MonitorHttpRequestDuration",
			mock.AnythingOfType("time.Duration"),
			mock.AnythingOfType("monitor.HttpRequestLabels"),
		).
		Return(nil).
		Maybe()

	mux := createHandler(t, res, authMock, monitorMock)

	readAllKey := createTestAPIKey(t, res.DBPool, "Adeptus Custodes Read Access",
		[]data.APIKeyPermission{data.ReadAll}, nil, 30, res.TestUserID)

	tests := []struct {
		name           string
		method         string
		path           string
		body           map[string]any
		expectedStatus int
	}{
		{
			name:           "Can GET disbursements",
			method:         http.MethodGet,
			path:           "/disbursements",
			body:           nil,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Can GET receivers",
			method:         http.MethodGet,
			path:           "/receivers",
			body:           nil,
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Cannot POST disbursements",
			method: http.MethodPost,
			path:   "/disbursements",
			body: map[string]any{
				"name":                      "Imperial Guard Relief Fund",
				"country_code":              "UKR",
				"wallet_id":                 res.Wallet.ID,
				"asset_id":                  res.Asset.ID,
				"verification_field":        data.VerificationTypeNationalID,
				"registration_contact_type": data.RegistrationContactTypeEmail,
			},
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := executeRequest(t, mux, tc.method, tc.path, tc.body, readAllKey.Key)
			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
		})
	}
}

func Test_handleHTTP_APIKeyWriteAllPermissions(t *testing.T) {
	res := setupAPIKeyTestResources(t)

	receiver, err := createTestReceiver(t, res.DBPool)
	require.NoError(t, err)
	receiverID := receiver.ID

	authMock := &auth.AuthManagerMock{}
	usr := &auth.User{ID: res.TestUserID, Email: "chapter.master@ultramar.gov"}
	authMock.On("GetUserByID", mock.Anything, mock.Anything).Return(usr, nil)

	monitorMock := monitorMocks.NewMockMonitorService(t)
	monitorMock.
		On("MonitorCounters",
			monitor.DisbursementsCounterTag,
			mock.AnythingOfType("map[string]string"),
		).
		Return(nil).
		Maybe()
	monitorMock.
		On("MonitorHttpRequestDuration",
			mock.AnythingOfType("time.Duration"),
			mock.AnythingOfType("monitor.HttpRequestLabels"),
		).
		Return(nil).
		Maybe()

	mux := createHandler(t, res, authMock, monitorMock)

	writeAllKey := createTestAPIKey(t, res.DBPool, "Tech-Priest Dominus Write Access",
		[]data.APIKeyPermission{data.WriteAll}, nil, 30, res.TestUserID)

	tests := []struct {
		name           string
		method         string
		path           string
		body           map[string]any
		expectedStatus int
	}{
		{
			name:           "Cannot GET disbursements",
			method:         http.MethodGet,
			path:           "/disbursements",
			body:           nil,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:   "Can POST disbursements",
			method: http.MethodPost,
			path:   "/disbursements",
			body: map[string]any{
				"name":                      "Cadian Defense Fund",
				"country_code":              "UKR",
				"wallet_id":                 res.Wallet.ID,
				"asset_id":                  res.Asset.ID,
				"verification_field":        data.VerificationTypeNationalID,
				"registration_contact_type": data.RegistrationContactTypeEmail,
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:   "Can PATCH receivers",
			method: http.MethodPatch,
			path:   "/receivers/" + receiverID,
			body: map[string]any{
				"email":         "marneus.calgar@ultramar.gov",
				"phone_number":  "+380931234567",
				"national_id":   "PRIMARIS-123",
				"date_of_birth": "1990-01-01",
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := executeRequest(t, mux, tc.method, tc.path, tc.body, writeAllKey.Key)
			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
		})
	}
}

func Test_handleHTTP_APIKeyFullAccessPermissions(t *testing.T) {
	res := setupAPIKeyTestResources(t)

	receiver, err := createTestReceiver(t, res.DBPool)
	require.NoError(t, err)
	receiverID := receiver.ID

	authMock := &auth.AuthManagerMock{}
	usr := &auth.User{ID: res.TestUserID, Email: "roboute.guilliman@imperium.gov"}
	authMock.On("GetUserByID", mock.Anything, mock.Anything).Return(usr, nil)

	monitorMock := monitorMocks.NewMockMonitorService(t)
	monitorMock.
		On("MonitorCounters",
			monitor.DisbursementsCounterTag,
			mock.AnythingOfType("map[string]string"),
		).
		Return(nil).
		Maybe()
	monitorMock.
		On("MonitorHttpRequestDuration",
			mock.AnythingOfType("time.Duration"),
			mock.AnythingOfType("monitor.HttpRequestLabels"),
		).
		Return(nil).
		Maybe()

	mux := createHandler(t, res, authMock, monitorMock)

	fullAccessKey := createTestAPIKey(t, res.DBPool, "Lord Commander Access",
		[]data.APIKeyPermission{data.ReadAll, data.WriteAll}, nil, 30, res.TestUserID)

	tests := []struct {
		name           string
		method         string
		path           string
		body           map[string]any
		expectedStatus int
	}{
		{
			name:           "Can GET disbursements",
			method:         http.MethodGet,
			path:           "/disbursements",
			body:           nil,
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Can POST disbursements",
			method: http.MethodPost,
			path:   "/disbursements",
			body: map[string]any{
				"name":                      "Indomitus Crusade Fund",
				"country_code":              "UKR",
				"wallet_id":                 res.Wallet.ID,
				"asset_id":                  res.Asset.ID,
				"verification_field":        data.VerificationTypeNationalID,
				"registration_contact_type": data.RegistrationContactTypeEmail,
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "Can GET receivers",
			method:         http.MethodGet,
			path:           "/receivers",
			body:           nil,
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Can PATCH receivers",
			method: http.MethodPatch,
			path:   "/receivers/" + receiverID,
			body: map[string]any{
				"email":         "tushan@nocturne.gov",
				"phone_number":  "+380931234567",
				"national_id":   "DRAKE-HUNTER-777",
				"date_of_birth": "1990-01-01",
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := executeRequest(t, mux, tc.method, tc.path, tc.body, fullAccessKey.Key)
			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
		})
	}
}

func Test_handleHTTP_APIKeySpecificPermissions(t *testing.T) {
	res := setupAPIKeyTestResources(t)

	receiver, err := createTestReceiver(t, res.DBPool)
	require.NoError(t, err)
	receiverID := receiver.ID

	authMock := &auth.AuthManagerMock{}
	usr := &auth.User{ID: res.TestUserID, Email: "logistics@munitorum.gov"}
	authMock.On("GetUserByID", mock.Anything, mock.Anything).Return(usr, nil)

	monitorMock := monitorMocks.NewMockMonitorService(t)
	monitorMock.
		On("MonitorCounters",
			monitor.DisbursementsCounterTag,
			mock.AnythingOfType("map[string]string"),
		).
		Return(nil).
		Maybe()
	monitorMock.
		On("MonitorHttpRequestDuration",
			mock.AnythingOfType("time.Duration"),
			mock.AnythingOfType("monitor.HttpRequestLabels"),
		).
		Return(nil).
		Maybe()

	mux := createHandler(t, res, authMock, monitorMock)

	specificKey := createTestAPIKey(t, res.DBPool, "Lord General Access",
		[]data.APIKeyPermission{data.ReadDisbursements, data.ReadReceivers, data.WriteReceivers},
		nil, 30, res.TestUserID)

	tests := []struct {
		name           string
		method         string
		path           string
		body           map[string]any
		expectedStatus int
	}{
		{
			name:           "Can GET disbursements",
			method:         http.MethodGet,
			path:           "/disbursements",
			body:           nil,
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Cannot POST disbursements",
			method: http.MethodPost,
			path:   "/disbursements",
			body: map[string]any{
				"name":                      "Catachan Jungle Fighter Supplies",
				"country_code":              "UKR",
				"wallet_id":                 res.Wallet.ID,
				"asset_id":                  res.Asset.ID,
				"verification_field":        data.VerificationTypeNationalID,
				"registration_contact_type": data.RegistrationContactTypeEmail,
			},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Can GET receivers",
			method:         http.MethodGet,
			path:           "/receivers",
			body:           nil,
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Can PATCH receivers",
			method: http.MethodPatch,
			path:   "/receivers/" + receiverID,
			body: map[string]any{
				"email":         "ibram.gaunt@tanith.gov",
				"phone_number":  "+380931234567",
				"national_id":   "FIRST-AND-ONLY",
				"date_of_birth": "1990-01-01",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Cannot GET wallets",
			method:         http.MethodGet,
			path:           "/wallets",
			body:           nil,
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := executeRequest(t, mux, tc.method, tc.path, tc.body, specificKey.Key)
			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
		})
	}
}

func createTestReceiver(t *testing.T, dbPool db.DBConnectionPool) (*data.Receiver, error) {
	t.Helper()

	ctx := context.Background()
	models, err := data.NewModels(dbPool)
	require.NoError(t, err)

	receiver, err := models.Receiver.Get(ctx, dbPool, "123")
	if err == nil {
		return receiver, nil
	}

	phoneNumber := "+380931234567"
	email := "ultramarines@macragge.imperium"
	externalID := "PRIMARCH-GUILLIMAN"

	phonePtr := &phoneNumber
	emailPtr := &email
	externalIDPtr := &externalID

	insert := data.ReceiverInsert{
		PhoneNumber: phonePtr,
		Email:       emailPtr,
		ExternalId:  externalIDPtr,
	}

	return models.Receiver.Insert(ctx, dbPool, insert)
}

func executeRequest(t *testing.T, mux http.Handler, method, path string, body map[string]any, apiKey string) *http.Response {
	var req *http.Request
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req = httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}

	req.Header.Set(middleware.TenantHeaderKey, "aid-org")
	req.Header.Set("Authorization", apiKey)
	req.RemoteAddr = "192.168.1.0:4000"

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w.Result()
}

func setupAPIKeyTestResources(t *testing.T) *TestResources {
	dbPool := getConnectionPool(t)

	testUserID := "00000000-0000-0000-0000-000000000000"

	_, err := data.NewModels(dbPool)
	require.NoError(t, err)
	ctx := context.Background()
	wallet := data.CreateDefaultWalletFixture(t, ctx, dbPool)
	asset := data.GetAssetFixture(t, ctx, dbPool, data.FixtureAssetUSDC)

	return &TestResources{
		DBPool:     dbPool,
		Wallet:     wallet,
		Asset:      asset,
		TestUserID: testUserID,
	}
}

func createHandler(t *testing.T, res *TestResources, authMock *auth.AuthManagerMock, monitorMock *monitorMocks.MockMonitorService) http.Handler {
	srvOpts := getServeOptionsForTests(t, res.DBPool)

	srvOpts.authManager = authMock
	srvOpts.MonitorService = monitorMock

	return handleHTTP(srvOpts)
}

func createTestAPIKey(t *testing.T, db db.DBConnectionPool, name string, perms []data.APIKeyPermission,
	allowedIPs []string, expiryDays int, createdBy string,
) *data.APIKey {
	t.Helper()

	ctx := context.Background()
	models, err := data.NewModels(db)
	require.NoError(t, err)

	var expiry *time.Time
	if expiryDays > 0 {
		exp := time.Now().AddDate(0, 0, expiryDays)
		expiry = &exp
	} else if expiryDays < 0 {
		exp := time.Now().AddDate(0, 0, expiryDays) // Past date for expired keys
		expiry = &exp
	}

	apiKey, err := models.APIKeys.Insert(ctx, name, perms, allowedIPs, expiry, createdBy)
	require.NoError(t, err)
	require.NotNil(t, apiKey)

	return apiKey
}
