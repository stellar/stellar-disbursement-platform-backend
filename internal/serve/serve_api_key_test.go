package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	monitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
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

	// The API keys' creator must exist as a real (owner) user: read/write paths resolve the
	// acting user for wallet-scoped visibility and authorization.
	data.EnsureDefaultDistributionWalletFixture(t, context.Background(), dbConnectionPool)
	_, seedErr := dbConnectionPool.ExecContext(context.Background(), `
		INSERT INTO auth_users (id, encrypted_password, email, first_name, last_name, is_owner, roles)
		VALUES ($1, 'x', 'api-key-auth-owner@test.com', 'API', 'Key', TRUE, ARRAY['owner'])
		ON CONFLICT (id) DO NOTHING`, "00000000-0000-0000-0000-000000000000")
	require.NoError(t, seedErr)

	// A deactivated creator: GetUser's is_active=true filter makes this user invisible to
	// GetUserFromContext, so any request made with a key this user created should 401 rather
	// than 500 (deactivation is normal offboarding, not an internal error).
	_, seedErr = dbConnectionPool.ExecContext(context.Background(), `
		INSERT INTO auth_users (id, encrypted_password, email, first_name, last_name, is_owner, roles, is_active)
		VALUES ($1, 'x', 'api-key-auth-deactivated@test.com', 'Deactivated', 'Owner', TRUE, ARRAY['owner'], FALSE)
		ON CONFLICT (id) DO NOTHING`, "00000000-0000-0000-0000-000000000099")
	require.NoError(t, seedErr)

	handlerMux := handleHTTP(serveOptions)

	testUserID := "00000000-0000-0000-0000-000000000000"
	deactivatedUserID := "00000000-0000-0000-0000-000000000099"

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

	deactivatedCreatorAPIKey := createTestAPIKey(t, dbConnectionPool, "Deactivated Creator Key",
		[]data.APIKeyPermission{data.ReadAll, data.WriteAll}, nil, 30, deactivatedUserID)

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
		{
			name:           "key whose creator was deactivated is rejected, not a 500",
			method:         http.MethodGet,
			path:           "/disbursements",
			apiKey:         deactivatedCreatorAPIKey.Key,
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
	usr := &auth.User{ID: res.TestUserID, Email: "inquisitor@ordohereticus.gov", IsOwner: true} // wallet authz covered by Test_WalletScopedAuthorization
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
		On("MonitorHTTPRequestDuration",
			mock.AnythingOfType("time.Duration"),
			mock.AnythingOfType("monitor.HTTPRequestLabels"),
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
	usr := &auth.User{ID: res.TestUserID, Email: "chapter.master@ultramar.gov", IsOwner: true} // wallet authz covered by Test_WalletScopedAuthorization
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
		On("MonitorHTTPRequestDuration",
			mock.AnythingOfType("time.Duration"),
			mock.AnythingOfType("monitor.HTTPRequestLabels"),
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
			if resp.StatusCode != tc.expectedStatus {
				b, readErr := io.ReadAll(resp.Body)
				require.NoError(t, readErr)
				t.Logf("unexpected status %d body: %s", resp.StatusCode, string(b))
			}
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
	usr := &auth.User{ID: res.TestUserID, Email: "roboute.guilliman@imperium.gov", IsOwner: true}
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
		On("MonitorHTTPRequestDuration",
			mock.AnythingOfType("time.Duration"),
			mock.AnythingOfType("monitor.HTTPRequestLabels"),
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
	usr := &auth.User{ID: res.TestUserID, Email: "logistics@munitorum.gov", IsOwner: true}
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
		On("MonitorHTTPRequestDuration",
			mock.AnythingOfType("time.Duration"),
			mock.AnythingOfType("monitor.HTTPRequestLabels"),
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

// Test_handleHTTP_APIKeyPermissionChangeTakesEffectImmediately reproduces (and
// guards against regressing) a bug where the API-key auth cache kept serving a
// key's pre-PATCH permissions/allowed_ips: a revoked scope kept being accepted
// and a newly-granted one kept being rejected, for as long as the cache entry's
// TTL window lasted, unless the exact same PATCH was resubmitted a second time.
//
// This exercises the real, fully-wired mux (handleHTTP) so the PATCH goes
// through the same APIKeyAuthenticator instance that authenticates every other
// request - a test that only called the model/handler layer directly could not
// have caught the missing cache invalidation.
func Test_handleHTTP_APIKeyPermissionChangeTakesEffectImmediately(t *testing.T) {
	res := setupAPIKeyTestResources(t)

	authMock := &auth.AuthManagerMock{}
	usr := &auth.User{ID: res.TestUserID, Email: "inquisitor.fiscus@ordo-hereticus.gov", IsOwner: true}
	authMock.On("GetUserByID", mock.Anything, mock.Anything).Return(usr, nil)

	monitorMock := monitorMocks.NewMockMonitorService(t)
	monitorMock.
		On("MonitorCounters", mock.Anything, mock.AnythingOfType("map[string]string")).
		Return(nil).
		Maybe()
	monitorMock.
		On("MonitorHTTPRequestDuration", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("monitor.HTTPRequestLabels")).
		Return(nil).
		Maybe()

	mux := createHandler(t, res, authMock, monitorMock)

	// adminKey only exists to authenticate the PATCH call itself. In production
	// this would be an owner's JWT session; an API key with WriteAll satisfies
	// the same RequirePermission check and keeps this test self-contained.
	adminKey := createTestAPIKey(t, res.DBPool, "Ordo Fiscus Admin Key",
		[]data.APIKeyPermission{data.WriteAll}, nil, 30, res.TestUserID)

	// The key under test: starts with read:statistics only, and an allowed_ips
	// restriction that will be reverted back to its original value later.
	targetKey := createTestAPIKey(t, res.DBPool, "Rotatable Munitorum Key",
		[]data.APIKeyPermission{data.ReadStatistics}, []string{"192.168.1.0"}, 30, res.TestUserID)

	// Sanity check before any PATCH: granted scope works, ungranted scope doesn't.
	resp := executeRequest(t, mux, http.MethodGet, "/statistics", nil, targetKey.Key)
	require.Equal(t, http.StatusOK, resp.StatusCode, "granted read:statistics should work before the PATCH")

	resp = executeRequest(t, mux, http.MethodGet, "/receivers", nil, targetKey.Key)
	require.Equal(t, http.StatusForbidden, resp.StatusCode, "not-yet-granted read:receivers should be rejected before the PATCH")

	t.Run("permission change enforced on the very next request", func(t *testing.T) {
		// Revoke read:statistics, grant read:receivers instead - in a single PATCH.
		patchResp := executeRequest(t, mux, http.MethodPatch, "/api-keys/"+targetKey.ID, map[string]any{
			"permissions": []string{"read:receivers"},
			"allowed_ips": []string{"192.168.1.0"},
		}, adminKey.Key)
		body, readErr := io.ReadAll(patchResp.Body)
		require.NoError(t, readErr)
		require.Equal(t, http.StatusOK, patchResp.StatusCode, "PATCH /api-keys/%s failed: %s", targetKey.ID, string(body))

		// No sleep, no retry, no second identical PATCH: the very next request
		// using this exact key must already reflect the change.
		oldScopeResp := executeRequest(t, mux, http.MethodGet, "/statistics", nil, targetKey.Key)
		assert.Equal(t, http.StatusForbidden, oldScopeResp.StatusCode,
			"revoked read:statistics must be rejected on the very next request, not after a cache TTL window or a duplicate PATCH")

		newScopeResp := executeRequest(t, mux, http.MethodGet, "/receivers", nil, targetKey.Key)
		assert.Equal(t, http.StatusOK, newScopeResp.StatusCode,
			"newly granted read:receivers must be accepted on the very next request")
	})

	t.Run("allowed_ips revert enforced on the very next request", func(t *testing.T) {
		// Tighten the allowlist to an IP that won't match the test's RemoteAddr...
		tightenResp := executeRequest(t, mux, http.MethodPatch, "/api-keys/"+targetKey.ID, map[string]any{
			"permissions": []string{"read:receivers"},
			"allowed_ips": []string{"10.10.10.10"},
		}, adminKey.Key)
		require.Equal(t, http.StatusOK, tightenResp.StatusCode)

		blockedResp := executeRequest(t, mux, http.MethodGet, "/receivers", nil, targetKey.Key)
		require.Equal(t, http.StatusForbidden, blockedResp.StatusCode,
			"the tightened allowed_ips must be enforced on the very next request")

		// ...then immediately revert it back to the original value. This is the
		// exact "revert to original value" case from the live repro: a second,
		// different-looking PATCH, not a duplicate of the previous one.
		revertResp := executeRequest(t, mux, http.MethodPatch, "/api-keys/"+targetKey.ID, map[string]any{
			"permissions": []string{"read:receivers"},
			"allowed_ips": []string{"192.168.1.0"},
		}, adminKey.Key)
		require.Equal(t, http.StatusOK, revertResp.StatusCode)

		// No sleep, no duplicate PATCH: access from the original, now-restored IP
		// must work again immediately.
		restoredResp := executeRequest(t, mux, http.MethodGet, "/receivers", nil, targetKey.Key)
		assert.Equal(t, http.StatusOK, restoredResp.StatusCode,
			"the reverted allowed_ips must be enforced on the very next request, not after a cache TTL window or a duplicate PATCH")
	})
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
		ExternalID:  externalIDPtr,
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
	data.EnsureDefaultDistributionWalletFixture(t, ctx, dbPool)

	// The API key's creator must exist as a real (owner) user: the disbursement-creation
	// path resolves the acting user for wallet-scoped authorization.
	_, err = dbPool.ExecContext(ctx, `
		INSERT INTO auth_users (id, encrypted_password, email, first_name, last_name, is_owner, roles)
		VALUES ($1, 'x', 'api-key-owner@test.com', 'API', 'Key', TRUE, ARRAY['owner'])
		ON CONFLICT (id) DO NOTHING`, testUserID)
	require.NoError(t, err)

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
	if expiryDays != 0 {
		exp := time.Now().AddDate(0, 0, expiryDays)
		expiry = &exp
	}

	apiKey, err := models.APIKeys.Insert(ctx, name, perms, allowedIPs, expiry, createdBy)
	require.NoError(t, err)
	require.NotNil(t, apiKey)

	return apiKey
}
