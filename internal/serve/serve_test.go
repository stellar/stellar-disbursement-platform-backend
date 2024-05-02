package serve

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/network"
	supporthttp "github.com/stellar/go/support/http"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	monitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/publicfiles"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type mockHTTPServer struct {
	mock.Mock
}

func (m *mockHTTPServer) Run(conf supporthttp.Config) {
	m.Called(conf)
}

const (
	publicKeyStr = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAER88h7AiQyVDysRTxKvBB6CaiO/kS
cvGyimApUE/12gFhNTRf37SE19CSCllKxstnVFOpLLWB7Qu5OJ0Wvcz3hg==
-----END PUBLIC KEY-----`
	privateKeyStr = `-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgIqI1MzMZIw2pQDLx
Jn0+FcNT/hNjwtn2TW43710JKZqhRANCAARHzyHsCJDJUPKxFPEq8EHoJqI7+RJy
8bKKYClQT/XaAWE1NF/ftITX0JIKWUrGy2dUU6kstYHtC7k4nRa9zPeG
-----END PRIVATE KEY-----`
	distAccPublicKey = "GBQQ7ATXREG5PXUTZ6UXR6LQRWVKVRTXLJKMN6UJCN6TGTFY7FKFUCBC"
)

func Test_Serve(t *testing.T) {
	dbt := dbtest.OpenWithoutMigrations(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	mockCrashTrackerClient := &crashtracker.MockCrashTrackerClient{}

	opts := ServeOptions{
		CrashTrackerClient:              mockCrashTrackerClient,
		MtnDBConnectionPool:             dbConnectionPool,
		AdminDBConnectionPool:           dbConnectionPool,
		EC256PrivateKey:                 privateKeyStr,
		EC256PublicKey:                  publicKeyStr,
		Environment:                     "test",
		GitCommit:                       "1234567890abcdef",
		Models:                          models,
		Port:                            8000,
		ResetTokenExpirationHours:       1,
		SEP24JWTSecret:                  "jwt_secret_1234567890",
		AnchorPlatformBasePlatformURL:   "https://test.com",
		AnchorPlatformOutgoingJWTSecret: "jwt_secret_1234567890",
		Version:                         "x.y.z",
		NetworkPassphrase:               network.TestNetworkPassphrase,
	}

	// Mock supportHTTPRun
	mHTTPServer := mockHTTPServer{}
	mHTTPServer.On("Run", mock.AnythingOfType("http.Config")).Run(func(args mock.Arguments) {
		conf, ok := args.Get(0).(supporthttp.Config)
		require.True(t, ok, "should be of type supporthttp.Config")
		assert.Equal(t, ":8000", conf.ListenAddr)
		assert.Equal(t, time.Minute*3, conf.TCPKeepAlive)
		assert.Equal(t, time.Second*50, conf.ShutdownGracePeriod)
		assert.Equal(t, time.Second*5, conf.ReadTimeout)
		assert.Equal(t, time.Second*35, conf.WriteTimeout)
		assert.Equal(t, time.Minute*2, conf.IdleTimeout)
		assert.Nil(t, conf.TLS)
		assert.ObjectsAreEqualValues(handleHTTP(opts), conf.Handler)
		conf.OnStopping()
	}).Once()
	mockCrashTrackerClient.On("FlushEvents", 2*time.Second).Return(false).Once()
	mockCrashTrackerClient.On("Recover").Once()

	// test and assert
	err = Serve(opts, &mHTTPServer)
	require.NoError(t, err)
	mHTTPServer.AssertExpectations(t)
	mockCrashTrackerClient.AssertExpectations(t)
}

func Test_Serve_callsValidateSecurity(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	serveOptions := getServeOptionsForTests(t, dbConnectionPool)

	mHTTPServer := mockHTTPServer{}
	serveOptions.NetworkPassphrase = network.PublicNetworkPassphrase

	// Make sure MFA is enforced in pubnet
	serveOptions.DisableMFA = true
	err = Serve(serveOptions, &mHTTPServer)
	require.EqualError(t, err, "validating security options: MFA cannot be disabled in pubnet")

	// Make sure reCAPTCHA is enforced in pubnet
	serveOptions.DisableMFA = false
	serveOptions.DisableReCAPTCHA = true
	err = Serve(serveOptions, &mHTTPServer)
	require.EqualError(t, err, "validating security options: reCAPTCHA cannot be disabled in pubnet")
}

func Test_ServeOptions_ValidateSecurity(t *testing.T) {
	t.Run("Pubnet + DisableMFA: should return error", func(t *testing.T) {
		serveOptions := ServeOptions{
			NetworkPassphrase: network.PublicNetworkPassphrase,
			DisableMFA:        true,
		}

		err := serveOptions.ValidateSecurity()
		require.EqualError(t, err, "MFA cannot be disabled in pubnet")
	})

	t.Run("Pubnet + DisableReCAPTCHA: should return error", func(t *testing.T) {
		// Pubnet + DisableReCAPTCHA: should return error
		serveOptions := ServeOptions{
			NetworkPassphrase: network.PublicNetworkPassphrase,
			DisableReCAPTCHA:  true,
		}

		err := serveOptions.ValidateSecurity()
		require.EqualError(t, err, "reCAPTCHA cannot be disabled in pubnet")
	})

	t.Run("Testnet + DisableMFA: should not return error", func(t *testing.T) {
		// Testnet + DisableMFA: should not return error
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		serveOptions := ServeOptions{
			NetworkPassphrase: network.TestNetworkPassphrase,
			DisableMFA:        true,
		}

		err := serveOptions.ValidateSecurity()
		require.NoError(t, err)
		require.Contains(t, buf.String(), "MFA is disabled in network 'Test SDF Network ; September 2015'")
	})

	t.Run("Testnet + DisableReCAPTCHA: should not return error", func(t *testing.T) {
		// Testnet + DisableReCAPTCHA: should not return error
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		serveOptions := ServeOptions{
			NetworkPassphrase: network.TestNetworkPassphrase,
			DisableReCAPTCHA:  true,
		}

		err := serveOptions.ValidateSecurity()
		require.NoError(t, err)
		require.Contains(t, buf.String(), "reCAPTCHA is disabled in network 'Test SDF Network ; September 2015'")
	})
}

func Test_handleHTTP_Health(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	mMonitorService := monitorMocks.NewMockMonitorService(t)
	mLabels := monitor.HttpRequestLabels{
		Status: "200",
		Route:  "/health",
		Method: "GET",
	}
	mMonitorService.On("MonitorHttpRequestDuration", mock.AnythingOfType("time.Duration"), mLabels).Return(nil).Once()

	handlerMux := handleHTTP(ServeOptions{
		EC256PrivateKey: privateKeyStr,
		EC256PublicKey:  publicKeyStr,
		Environment:     "test",
		GitCommit:       "1234567890abcdef",
		Models:          models,
		MonitorService:  mMonitorService,
		SEP24JWTSecret:  "jwt_secret_1234567890",
		Version:         "x.y.z",
		tenantManager:   tenant.NewManager(tenant.WithDatabase(dbConnectionPool)),
	})

	req := httptest.NewRequest("GET", "/health", nil)
	req.Header.Set(middleware.TenantHeaderKey, "aid-org")
	w := httptest.NewRecorder()
	handlerMux.ServeHTTP(w, req)

	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	wantBody := `{
		"status": "pass",
		"version": "x.y.z",
		"service_id": "serve",
		"release_id": "1234567890abcdef"
	}`
	assert.JSONEq(t, wantBody, string(body))
}

func Test_staticFileServer(t *testing.T) {
	r := chi.NewMux()

	staticFileServer(r, publicfiles.PublicFiles)

	t.Run("Should return not found when tryig to access a folder", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/static/", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.Equal(t, "text/plain; charset=utf-8", rr.Header().Get("Content-Type"))
		assert.Equal(t, "404 page not found\n", string(data))
	})

	t.Run("Should return file contents on a valid file", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/static/js/test_mock.js", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Header().Get("Content-Type"), "javascript")
		assert.Equal(t, "console.log(\"test mock file.\");\n", string(data))
	})
}

// getServeOptionsForTests returns an instance of ServeOptions for testing purposes.
// 🚨 Don't forget to call `defer serveOptions.dbConnectionPool.Close()` in your test 🚨.
func getServeOptionsForTests(t *testing.T, dbConnectionPool db.DBConnectionPool) ServeOptions {
	t.Helper()

	mMonitorService := monitorMocks.NewMockMonitorService(t)
	mMonitorService.On("MonitorHttpRequestDuration", mock.AnythingOfType("time.Duration"), mock.Anything).Return(nil).Maybe()

	messengerClientMock := message.MessengerClientMock{}
	messengerClientMock.On("SendMessage", mock.Anything).Return(nil)

	crashTrackerClient, err := crashtracker.NewDryRunClient()
	require.NoError(t, err)

	mTenantManager := &tenant.TenantManagerMock{}
	mTenantManager.
		On("GetTenantByName", mock.Anything, "aid-org").
		Return(&tenant.Tenant{ID: "tenant1"}, nil)

	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, _, _, _, distAccResolver := signing.NewMockSignatureService(t)
	submitterEngine := engine.SubmitterEngine{
		HorizonClient:       mHorizonClient,
		SignatureService:    sigService,
		LedgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100 * txnbuild.MinBaseFee,
	}
	distAccResolver.
		On("DistributionAccountFromContext", mock.Anything).
		Return(schema.NewStellarDistributionAccount(distAccPublicKey), nil).
		Maybe()

	serveOptions := ServeOptions{
		CrashTrackerClient:              crashTrackerClient,
		MtnDBConnectionPool:             dbConnectionPool,
		AdminDBConnectionPool:           dbConnectionPool,
		EC256PrivateKey:                 privateKeyStr,
		EC256PublicKey:                  publicKeyStr,
		EmailMessengerClient:            &messengerClientMock,
		Environment:                     "test",
		GitCommit:                       "1234567890abcdef",
		MonitorService:                  mMonitorService,
		ResetTokenExpirationHours:       1,
		SEP24JWTSecret:                  "jwt_secret_1234567890",
		AnchorPlatformOutgoingJWTSecret: "jwt_secret_1234567890",
		AnchorPlatformBasePlatformURL:   "https://test.com",
		SMSMessengerClient:              &messengerClientMock,
		Version:                         "x.y.z",
		NetworkPassphrase:               network.TestNetworkPassphrase,
		SubmitterEngine:                 submitterEngine,
	}
	err = serveOptions.SetupDependencies()
	require.NoError(t, err)

	serveOptions.tenantManager = mTenantManager

	return serveOptions
}

func Test_handleHTTP_unauthenticatedEndpoints(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	serveOptions := getServeOptionsForTests(t, dbConnectionPool)

	handlerMux := handleHTTP(serveOptions)

	// Unauthenticated endpoints
	unauthenticatedEndpoints := []struct { // TODO: body to requests
		method string
		path   string
	}{
		{http.MethodGet, "/health"},
		{http.MethodGet, "/.well-known/stellar.toml"},
		{http.MethodPost, "/login"},
		{http.MethodPost, "/mfa"},
		{http.MethodPost, "/forgot-password"},
		{http.MethodPost, "/reset-password"},
	}
	for _, endpoint := range unauthenticatedEndpoints {
		t.Run(fmt.Sprintf("%s %s", endpoint.method, endpoint.path), func(t *testing.T) {
			req := httptest.NewRequest(endpoint.method, endpoint.path, nil)
			req.Header.Set("SDP-Tenant-Name", "aid-org")

			w := httptest.NewRecorder()
			handlerMux.ServeHTTP(w, req)

			resp := w.Result()
			assert.Contains(t, []int{http.StatusOK, http.StatusBadRequest}, resp.StatusCode)
		})
	}
}

func Test_handleHTTP_authenticatedEndpoints(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	serveOptions := getServeOptionsForTests(t, dbConnectionPool)

	handlerMux := handleHTTP(serveOptions)

	// Authenticated endpoints
	authenticatedEndpoints := []struct { // TODO: body to requests
		method string
		path   string
	}{
		// Statistics
		{http.MethodGet, "/statistics"},
		{http.MethodGet, "/statistics/1234"},
		// Users
		{http.MethodGet, "/users"},
		{http.MethodPost, "/users"},
		{http.MethodGet, "/users/roles"},
		{http.MethodPatch, "/users/roles"},
		{http.MethodPatch, "/users/activation"},
		// Refresh Token
		{http.MethodPost, "/refresh-token"},
		// Disbursements
		{http.MethodPost, "/disbursements"},
		{http.MethodPost, "/disbursements/1234/instructions"},
		{http.MethodGet, "/disbursements/1234/instructions"},
		{http.MethodGet, "/disbursements"},
		{http.MethodGet, "/disbursements/1234"},
		{http.MethodGet, "/disbursements/1234/receivers"},
		{http.MethodPatch, "/disbursements/1234/status"},
		// Payments
		{http.MethodGet, "/payments"},
		{http.MethodGet, "/payments/1234"},
		{http.MethodPatch, "/payments/retry"},
		{http.MethodPatch, "/payments/1234/status"},
		// Receivers
		{http.MethodGet, "/receivers"},
		{http.MethodGet, "/receivers/1234"},
		{http.MethodPatch, "/receivers/1234"},
		{http.MethodPatch, "/receivers/wallets/1234"},
		{http.MethodGet, "/receivers/verification-types"},
		// Countries
		{http.MethodGet, "/countries"},
		// Assets
		{http.MethodGet, "/assets"},
		{http.MethodPost, "/assets"},
		{http.MethodPatch, "/assets/1234"},
		{http.MethodDelete, "/assets/1234"},
		// Wallets
		{http.MethodGet, "/wallets"},
		{http.MethodPost, "/wallets"},
		{http.MethodDelete, "/wallets/1234"},
		{http.MethodPatch, "/wallets/1234"},
		// Profile
		{http.MethodGet, "/profile"},
		{http.MethodPatch, "/profile"},
		{http.MethodPatch, "/profile/reset-password"},
		// Organization
		{http.MethodGet, "/organization"},
		{http.MethodPatch, "/organization"},
		{http.MethodGet, "/organization/logo"},
	}

	// Expect 401 as a response:
	for _, endpoint := range authenticatedEndpoints {
		t.Run(fmt.Sprintf("expect 401 for %s %s", endpoint.method, endpoint.path), func(t *testing.T) {
			req := httptest.NewRequest(endpoint.method, endpoint.path, nil)
			req.Header.Set(middleware.TenantHeaderKey, "aid-org")

			w := httptest.NewRecorder()
			handlerMux.ServeHTTP(w, req)

			resp := w.Result()
			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		})
	}
}

func Test_handleHTTP_rateLimit(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	serveOptions := getServeOptionsForTests(t, dbConnectionPool)

	handlerMux := handleHTTP(serveOptions)

	// 1. The first n requests to /health should return 200
	// 2. the n+1 request to /health should return 429
	// 3. an additional request to another endpoint should return something other than 429
	expectedEndpoints := make([]string, rateLimitPer20Seconds)
	expectedResponseCodes := make([]int, rateLimitPer20Seconds)
	for i := 0; i < rateLimitPer20Seconds; i++ {
		expectedResponseCodes[i] = http.StatusOK
		expectedEndpoints[i] = "/health"
	}
	expectedResponseCodes = append(expectedResponseCodes, http.StatusTooManyRequests, http.StatusNotFound)
	expectedEndpoints = append(expectedEndpoints, "/health", "/not-found")
	require.Len(t, expectedResponseCodes, rateLimitPer20Seconds+2)
	require.Len(t, expectedEndpoints, rateLimitPer20Seconds+2)

	actualResponseCodes := make([]int, len(expectedResponseCodes))
	for i := 0; i < len(expectedResponseCodes); i++ {
		req := httptest.NewRequest(http.MethodGet, expectedEndpoints[i], nil)
		w := httptest.NewRecorder()
		handlerMux.ServeHTTP(w, req)
		resp := w.Result()
		actualResponseCodes[i] = resp.StatusCode
	}

	require.Equal(t, expectedResponseCodes, actualResponseCodes)
}

func Test_createAuthManager(t *testing.T) {
	dbt := dbtest.OpenWithoutMigrations(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	// creates the expected auth manager
	passwordEncrypter := auth.NewDefaultPasswordEncrypter()
	wantAuthManager := auth.NewAuthManager(
		auth.WithDefaultAuthenticatorOption(dbConnectionPool, passwordEncrypter, time.Hour*time.Duration(1)),
		auth.WithDefaultJWTManagerOption(publicKeyStr, privateKeyStr),
		auth.WithDefaultRoleManagerOption(dbConnectionPool, data.OwnerUserRole.String()),
		auth.WithDefaultMFAManagerOption(dbConnectionPool),
	)

	testCases := []struct {
		name                      string
		dbConnectionPool          db.DBConnectionPool
		ec256PublicKey            string
		ec256PrivateKey           string
		resetTokenExpirationHours int
		wantErrContains           string
		wantAuthManager           auth.AuthManager
	}{
		{
			name:            "returns error if dbConnectionPool is nil",
			wantErrContains: "db connection pool cannot be nil",
		},
		{
			name:             "returns error if dbConnectionPool is valid but the keypair is not",
			dbConnectionPool: dbConnectionPool,
			wantErrContains:  "validating auth manager keys: validating EC public key: failed to decode PEM block containing public key",
		},
		{
			name:             "returns error if dbConnectionPool and keypair is valid but the resetTokenExpirationHours is not",
			dbConnectionPool: dbConnectionPool,
			ec256PublicKey:   publicKeyStr,
			ec256PrivateKey:  privateKeyStr,
			wantErrContains:  "reset token expiration hours must be greater than 0",
		},
		{
			name:                      "🎉 successfully create the auth manager",
			dbConnectionPool:          dbConnectionPool,
			ec256PublicKey:            publicKeyStr,
			ec256PrivateKey:           privateKeyStr,
			resetTokenExpirationHours: 1,
			wantAuthManager:           wantAuthManager,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotAuthManager, err := createAuthManager(
				tc.dbConnectionPool, tc.ec256PublicKey, tc.ec256PrivateKey, tc.resetTokenExpirationHours,
			)
			if tc.wantErrContains != "" {
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Empty(t, gotAuthManager)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantAuthManager, gotAuthManager)
			}
		})
	}
}
