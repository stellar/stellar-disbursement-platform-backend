package serve

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/network"
	supporthttp "github.com/stellar/go/support/http"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type mockHTTPServer struct {
	mock.Mock
}

func (m *mockHTTPServer) Run(conf supporthttp.Config) {
	m.Called(conf)
}

var _ HTTPServerInterface = new(mockHTTPServer)

func Test_SetupDependencies(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mTenantManager := &tenant.TenantManagerMock{}
	mMessengerClient := &message.MessengerClientMock{}
	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, _, _ := signing.NewMockSignatureService(t)
	submitterEngine := engine.SubmitterEngine{
		HorizonClient:       mHorizonClient,
		SignatureService:    sigService,
		LedgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100 * txnbuild.MinBaseFee,
	}

	testCases := []struct {
		name            string
		opts            ServeOptions
		wantErrContains string
	}{
		{
			name: "handle errors when creating a provisioning manager",
			opts: ServeOptions{
				AdminDBConnectionPool: dbConnectionPool,
			},
			wantErrContains: "creating provisioning manager",
		},
		{
			name: "handle errors when parsing the network type",
			opts: ServeOptions{
				AdminDBConnectionPool:                   dbConnectionPool,
				tenantManager:                           mTenantManager,
				EmailMessengerClient:                    mMessengerClient,
				SubmitterEngine:                         submitterEngine,
				TenantAccountNativeAssetBootstrapAmount: tenant.MinTenantDistributionAccountAmount,
			},
			wantErrContains: "parsing network type",
		},
		{
			name: "handle errors when creating a data.Models instance",
			opts: ServeOptions{
				AdminDBConnectionPool:                   dbConnectionPool,
				tenantManager:                           mTenantManager,
				EmailMessengerClient:                    mMessengerClient,
				SubmitterEngine:                         submitterEngine,
				TenantAccountNativeAssetBootstrapAmount: tenant.MinTenantDistributionAccountAmount,
				NetworkPassphrase:                       network.TestNetworkPassphrase,
			},
			wantErrContains: "creating models",
		},
		{
			name: "🎉 successfully setup the dependencies",
			opts: ServeOptions{
				AdminDBConnectionPool:                   dbConnectionPool,
				tenantManager:                           mTenantManager,
				EmailMessengerClient:                    mMessengerClient,
				SubmitterEngine:                         submitterEngine,
				TenantAccountNativeAssetBootstrapAmount: tenant.MinTenantDistributionAccountAmount,
				NetworkPassphrase:                       network.TestNetworkPassphrase,
				MTNDBConnectionPool:                     dbConnectionPool,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.opts.SetupDependencies()
			if tc.wantErrContains != "" {
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_Serve(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mMessengerClient := &message.MessengerClientMock{}
	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)

	sigService, _, _ := signing.NewMockSignatureService(t)

	submitterEngine := engine.SubmitterEngine{
		HorizonClient:       mHorizonClient,
		SignatureService:    sigService,
		LedgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100 * txnbuild.MinBaseFee,
	}

	opts := ServeOptions{
		AdminDBConnectionPool:                   dbConnectionPool,
		MTNDBConnectionPool:                     dbConnectionPool,
		Environment:                             "test",
		GitCommit:                               "1234567890abcdef",
		NetworkPassphrase:                       network.TestNetworkPassphrase,
		Port:                                    8003,
		Version:                                 "x.y.z",
		SubmitterEngine:                         submitterEngine,
		TenantAccountNativeAssetBootstrapAmount: tenant.MinTenantDistributionAccountAmount,
		EmailMessengerClient:                    mMessengerClient,
	}

	// Mock supportHTTPRun
	mHTTPServer := mockHTTPServer{}
	mHTTPServer.On("Run", mock.AnythingOfType("http.Config")).Run(func(args mock.Arguments) {
		conf, ok := args.Get(0).(supporthttp.Config)
		require.True(t, ok, "should be of type supporthttp.Config")
		assert.Equal(t, ":8003", conf.ListenAddr)
		assert.Equal(t, time.Minute*3, conf.TCPKeepAlive)
		assert.Equal(t, time.Second*50, conf.ShutdownGracePeriod)
		assert.Equal(t, time.Second*5, conf.ReadTimeout)
		assert.Equal(t, time.Second*50, conf.WriteTimeout)
		assert.Equal(t, time.Minute*2, conf.IdleTimeout)
		assert.Nil(t, conf.TLS)
		assert.ObjectsAreEqualValues(handleHTTP(&opts), conf.Handler)
		conf.OnStopping()
	}).Once()

	// test and assert
	err = StartServe(opts, &mHTTPServer)
	require.NoError(t, err)
	mHTTPServer.AssertExpectations(t)
}

func Test_handleHTTP_authenticatedAdminEndpoints(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	serveOptions := ServeOptions{
		AdminAccount: "SDP-admin",
		AdminApiKey:  "api_key_1234567890",
	}

	handlerMux := handleHTTP(&serveOptions)

	// Authenticated endpoints
	authenticatedEndpoints := []struct { // TODO: body to requests
		method string
		path   string
	}{
		// Tenants
		{http.MethodGet, "/tenants"},
		{http.MethodPost, "/tenants"},
		{http.MethodGet, "/tenants/1234"},
		{http.MethodPatch, "/tenants/1234"},
	}

	// Expect 401 as a response:
	for _, endpoint := range authenticatedEndpoints {
		t.Run(fmt.Sprintf("expect 401 for %s %s", endpoint.method, endpoint.path), func(t *testing.T) {
			req := httptest.NewRequest(endpoint.method, endpoint.path, nil)

			w := httptest.NewRecorder()
			handlerMux.ServeHTTP(w, req)

			resp := w.Result()
			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		})
	}
}

func Test_handleHTTP_unauthenticatedAdminEndpoints(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	handlerMux := handleHTTP(&ServeOptions{})

	// Unauthenticated endpoints
	unauthenticatedEndpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/health"},
	}

	for _, endpoint := range unauthenticatedEndpoints {
		t.Run(fmt.Sprintf("%s %s", endpoint.method, endpoint.path), func(t *testing.T) {
			req := httptest.NewRequest(endpoint.method, endpoint.path, nil)

			w := httptest.NewRecorder()
			handlerMux.ServeHTTP(w, req)

			resp := w.Result()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}
