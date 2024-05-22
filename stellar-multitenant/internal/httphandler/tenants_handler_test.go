package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/protocols/horizon/base"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/internal/provisioning"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_TenantHandler_Get(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	handler := TenantsHandler{
		Manager: tenant.NewManager(tenant.WithDatabase(dbConnectionPool)),
	}

	r := chi.NewRouter()
	r.Get("/tenants/{arg}", handler.GetByIDOrName)

	t.Run("GetAll successfully returns an empty list of tenants", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, getTntErr := http.NewRequest(http.MethodGet, "/tenants", nil)
		require.NoError(t, getTntErr)
		http.HandlerFunc(handler.GetAll).ServeHTTP(rr, req)

		resp := rr.Result()
		defer resp.Body.Close()

		respBody, readRespBodyErr := io.ReadAll(resp.Body)
		require.NoError(t, readRespBodyErr)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `[]`, string(respBody))
	})

	tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)
	tnt1 := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "myorg1", "GCTNUNQVX7BNIP5AUWW2R4YC7G6R3JGUDNMGT7H62BGBUY4A4V6ROAAH")
	tnt2 := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "myorg2", "GB37V3J5C3RAJY6BI52MAAWF6AVKJH7J4L2DVBMOP7WQJHQPNIBR3FKH")
	deactivatedTnt := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "dorg", "GBKXOCCQ5HXYOJ7NH5LXDKOBKU22TE6XOKHKYADZPRQFLR2F5KPFVILF")
	dStatus := tenant.DeactivatedTenantStatus
	deactivatedTnt, err = handler.Manager.UpdateTenantConfig(ctx, &tenant.TenantUpdate{
		ID:     deactivatedTnt.ID,
		Status: &dStatus,
	})
	require.NoError(t, err)
	t.Run("GetAll successfully returns a list of all tenants", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/tenants", nil)
		require.NoError(t, err)
		http.HandlerFunc(handler.GetAll).ServeHTTP(rr, req)

		resp := rr.Result()
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		expectedRespBody := fmt.Sprintf(`
			[
				{
					"id": %q,
					"name": %q,
					"base_url": null,
					"sdp_ui_base_url": null,
					"status": "TENANT_CREATED",
					"is_default": false,
					"created_at": %q,
					"updated_at": %q,
					"deleted_at": null,
					"distribution_account_address": %q,
					"distribution_account_type": %q,
					"distribution_account_status": %q
				},
				{
					"id": %q,
					"name": %q,
					"base_url": null,
					"sdp_ui_base_url": null,
					"status": "TENANT_CREATED",
					"is_default": false,
					"created_at": %q,
					"updated_at": %q,
					"deleted_at": null,
					"distribution_account_address": %q,
					"distribution_account_type": %q,
					"distribution_account_status": %q
				},
				{
					"id": %q,
					"name": %q,
					"base_url": null,
					"sdp_ui_base_url": null,
					"status": "TENANT_DEACTIVATED",
					"is_default": false,
					"created_at": %q,
					"updated_at": %q,
					"deleted_at": null,
					"distribution_account_address": %q,
					"distribution_account_type": %q,
					"distribution_account_status": %q
				}
			]
		`,
			tnt1.ID, tnt1.Name, tnt1.CreatedAt.Format(time.RFC3339Nano), tnt1.UpdatedAt.Format(time.RFC3339Nano),
			*tnt1.DistributionAccountAddress, schema.DistributionAccountStellarDBVault, schema.AccountStatusActive,
			tnt2.ID, tnt2.Name, tnt2.CreatedAt.Format(time.RFC3339Nano), tnt2.UpdatedAt.Format(time.RFC3339Nano),
			*tnt2.DistributionAccountAddress, schema.DistributionAccountStellarDBVault, schema.AccountStatusActive,
			deactivatedTnt.ID, deactivatedTnt.Name, deactivatedTnt.CreatedAt.Format(time.RFC3339Nano), deactivatedTnt.UpdatedAt.Format(time.RFC3339Nano),
			*deactivatedTnt.DistributionAccountAddress, schema.DistributionAccountStellarDBVault, schema.AccountStatusActive,
		)
		assert.JSONEq(t, expectedRespBody, string(respBody))
	})

	t.Run("successfully returns a tenant by ID", func(t *testing.T) {
		url := fmt.Sprintf("/tenants/%s", tnt1.ID)
		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, url, nil)
		require.NoError(t, err)
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		expectedRespBody := fmt.Sprintf(`
			{
				"id": %q,
				"name": %q,
				"base_url": null,
				"sdp_ui_base_url": null,
				"status": "TENANT_CREATED",
				"is_default": false,
				"created_at": %q,
				"updated_at": %q,
				"deleted_at": null,
				"distribution_account_address": %q,
				"distribution_account_type": %q,
				"distribution_account_status": %q
			}
		`, tnt1.ID, tnt1.Name, tnt1.CreatedAt.Format(time.RFC3339Nano), tnt1.UpdatedAt.Format(time.RFC3339Nano),
			*tnt1.DistributionAccountAddress, schema.DistributionAccountStellarDBVault, schema.AccountStatusActive,
		)
		assert.JSONEq(t, expectedRespBody, string(respBody))
	})

	t.Run("successfully returns a tenant by name", func(t *testing.T) {
		url := fmt.Sprintf("/tenants/%s", tnt2.Name)
		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, url, nil)
		require.NoError(t, err)
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		expectedRespBody := fmt.Sprintf(`
			{
				"id": %q,
				"name": %q,
				"base_url": null,
				"sdp_ui_base_url": null,
				"status": "TENANT_CREATED",
				"is_default": false,
				"created_at": %q,
				"updated_at": %q,
				"deleted_at": null,
				"distribution_account_address": %q,
				"distribution_account_type": %q,
				"distribution_account_status": %q
			}
		`, tnt2.ID, tnt2.Name, tnt2.CreatedAt.Format(time.RFC3339Nano), tnt2.UpdatedAt.Format(time.RFC3339Nano),
			*tnt2.DistributionAccountAddress, schema.DistributionAccountStellarDBVault, schema.AccountStatusActive,
		)
		assert.JSONEq(t, expectedRespBody, string(respBody))
	})

	t.Run("returns NotFound when tenant does not exist", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/tenants/unknown", nil)
		require.NoError(t, err)
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		expectedRespBody := `
			{"error": "tenant unknown does not exist"}
		`
		assert.JSONEq(t, expectedRespBody, string(respBody))
	})
}

func Test_TenantHandler_Post(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	messengerClientMock := &message.MessengerClientMock{}
	crashTrackerMock := &crashtracker.MockCrashTrackerClient{}
	tenantManager := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))

	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, sigRouter, distAccResolver := signing.NewMockSignatureService(t)

	distAccAddress := keypair.MustRandom().Address()

	submitterEngine := engine.SubmitterEngine{
		HorizonClient:       mHorizonClient,
		SignatureService:    sigService,
		LedgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100 * txnbuild.MinBaseFee,
	}

	p, err := provisioning.NewManager(provisioning.ManagerOptions{
		DBConnectionPool:           dbConnectionPool,
		TenantManager:              tenantManager,
		SubmitterEngine:            submitterEngine,
		NativeAssetBootstrapAmount: tenant.MinTenantDistributionAccountAmount,
	})
	require.NoError(t, err)

	handler := TenantsHandler{
		CrashTrackerClient:  crashTrackerMock,
		Manager:             tenantManager,
		MessengerClient:     messengerClientMock,
		ProvisioningManager: p,
		NetworkType:         utils.TestnetNetworkType,
		BaseURL:             "https://sdp-backend.stellar.org",
		SDPUIBaseURL:        "https://sdp-ui.stellar.org",
	}

	createMocks := func(t *testing.T, accountType schema.AccountType, msgClientErr error) {
		distAccToReturn := schema.TransactionAccount{
			Address: distAccAddress,
			Type:    accountType,
			Status:  schema.AccountStatusActive,
		}
		sigRouter.
			On("BatchInsert", ctx, accountType, 1).
			Return([]schema.TransactionAccount{distAccToReturn}, nil).
			Once()

		hostAccount := schema.NewDefaultHostAccount(distAccAddress)
		distAccResolver.
			On("HostDistributionAccount").
			Return(&hostAccount, nil).
			Once()

		messengerClientMock.
			On("SendMessage", mock.AnythingOfType("message.Message")).
			Run(func(args mock.Arguments) {
				msg, ok := args.Get(0).(message.Message)
				require.True(t, ok)

				assert.Equal(t, "Welcome to Stellar Disbursement Platform", msg.Title)
				assert.Equal(t, "owner@email.org", msg.ToEmail)
				assert.Empty(t, msg.ToPhoneNumber)
			}).
			Return(msgClientErr).
			Once()

		if msgClientErr != nil {
			crashTrackerMock.
				On("LogAndReportErrors", ctx, mock.Anything, "Cannot send invitation message").
				Once()
		}
	}

	makeRequest := func(t *testing.T, reqBody string, expectedStatus int) []byte {
		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/tenants", strings.NewReader(reqBody))
		require.NoError(t, err)
		http.HandlerFunc(handler.Post).ServeHTTP(rr, req)

		resp := rr.Result()
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, expectedStatus, resp.StatusCode)
		return respBody
	}

	assertMigrations := func(orgName string) {
		// Validating infrastructure
		expectedSchema := fmt.Sprintf("sdp_%s", orgName)
		expectedTablesAfterMigrationsApplied := []string{
			"assets",
			"auth_migrations",
			"auth_user_mfa_codes",
			"auth_user_password_reset",
			"auth_users",
			"countries",
			"disbursements",
			"sdp_migrations",
			"messages",
			"organizations",
			"payments",
			"receiver_verifications",
			"receiver_wallets",
			"receivers",
			"wallets",
			"wallets_assets",
		}
		tenant.CheckSchemaExistsFixture(t, ctx, dbConnectionPool, expectedSchema)
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, expectedSchema, expectedTablesAfterMigrationsApplied)

		dsn, err := tenantManager.GetDSNForTenant(ctx, orgName)
		require.NoError(t, err)

		tenantSchemaConnectionPool, err := db.OpenDBConnectionPool(dsn)
		require.NoError(t, err)
		defer tenantSchemaConnectionPool.Close()

		tenant.AssertRegisteredAssetsFixture(t, ctx, tenantSchemaConnectionPool, []string{"USDC:GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", "XLM:"})
		tenant.AssertRegisteredWalletsFixture(t, ctx, tenantSchemaConnectionPool, []string{"Demo Wallet", "Vibrant Assist"})
		tenant.AssertRegisteredUserFixture(t, ctx, tenantSchemaConnectionPool, "Owner", "Owner", "owner@email.org")
	}

	t.Run("returns BadRequest with invalid request body", func(t *testing.T) {
		respBody := makeRequest(t, `{}`, http.StatusBadRequest)

		expectedBody := `
			{
				"error": "invalid request body",
				"extras": {
					"name": "invalid tenant name. It should only contains lower case letters and dash (-)",
					"owner_email": "invalid email",
					"owner_first_name": "owner_first_name is required",
					"owner_last_name": "owner_last_name is required",
					"organization_name": "organization_name is required"
				}
			}
		`
		assert.JSONEq(t, expectedBody, string(respBody))
	})

	t.Run("provisions a new tenant successfully", func(t *testing.T) {
		// TODO: in SDP-1167, send the accountType in the request body
		accountType := schema.DistributionAccountStellarEnv
		createMocks(t, accountType, nil)

		orgName := "aid-org"
		reqBody := fmt.Sprintf(`
			{
				"name": %q,
				"owner_email": "owner@email.org",
				"owner_first_name": "Owner",
				"owner_last_name": "Owner",
				"organization_name": "My Aid Org",
				"base_url": "https://sdp-backend.stellar.org",
				"sdp_ui_base_url": "https://sdp-ui.stellar.org",
				"is_default": false
			}
		`, orgName)

		respBody := makeRequest(t, reqBody, http.StatusCreated)

		tnt, err := tenantManager.GetTenantByName(ctx, orgName)
		require.NoError(t, err)

		expectedRespBody := fmt.Sprintf(`
			{
				"id": %q,
				"name": %q,
				"base_url": "https://sdp-backend.stellar.org",
				"sdp_ui_base_url": "https://sdp-ui.stellar.org",
				"status": "TENANT_PROVISIONED",
				"is_default": false,
				"created_at": %q,
				"updated_at": %q,
				"deleted_at": null,
				"distribution_account_address": %q,
				"distribution_account_type": %q,
				"distribution_account_status": %q
			}
		`, tnt.ID, orgName, tnt.CreatedAt.Format(time.RFC3339Nano), tnt.UpdatedAt.Format(time.RFC3339Nano),
			distAccAddress, accountType, schema.AccountStatusActive)
		assert.JSONEq(t, expectedRespBody, string(respBody))

		assertMigrations(orgName)
	})

	t.Run("provisions a new tenant successfully - dynamically generates base URL and SDP UI base URL for tenant", func(t *testing.T) {
		// TODO: in SDP-1167, send the accountType in the request body
		accountType := schema.DistributionAccountStellarEnv
		createMocks(t, accountType, nil)

		orgName := "aid-org-two"
		reqBody := fmt.Sprintf(`
			{
				"name": %q,
				"owner_email": "owner@email.org",
				"owner_first_name": "Owner",
				"owner_last_name": "Owner",
				"organization_name": "My Aid Org 2",
				"is_default": false
			}
		`, orgName)

		respBody := makeRequest(t, reqBody, http.StatusCreated)

		tnt, err := tenantManager.GetTenantByName(ctx, orgName)
		require.NoError(t, err)

		generatedURL := fmt.Sprintf("https://%s.sdp-backend.stellar.org", orgName)
		generatedUIURL := fmt.Sprintf("https://%s.sdp-ui.stellar.org", orgName)
		expectedRespBody := fmt.Sprintf(`
			{
				"id": %q,
				"name": %q,
				"base_url": %q,
				"sdp_ui_base_url": %q,
				"status": "TENANT_PROVISIONED",
				"is_default": false,
				"created_at": %q,
				"updated_at": %q,
				"deleted_at": null,
				"distribution_account_address": %q,
				"distribution_account_type": %q,
				"distribution_account_status": %q
			}
		`, tnt.ID, orgName, generatedURL, generatedUIURL, tnt.CreatedAt.Format(time.RFC3339Nano), tnt.UpdatedAt.Format(time.RFC3339Nano),
			distAccAddress, accountType, schema.AccountStatusActive)
		assert.JSONEq(t, expectedRespBody, string(respBody))

		assertMigrations(orgName)
	})

	t.Run("provisions a new tenant successfully - dynamically generates only SDP UI base URL", func(t *testing.T) {
		// TODO: in SDP-1167, send the accountType in the request body
		accountType := schema.DistributionAccountStellarEnv
		createMocks(t, accountType, nil)

		orgName := "aid-org-three"
		reqBody := fmt.Sprintf(`
			{
				"name": %q,
				"owner_email": "owner@email.org",
				"owner_first_name": "Owner",
				"owner_last_name": "Owner",
				"organization_name": "My Aid Org 3",
				"base_url": %q,
				"is_default": false
			}
		`, orgName, handler.BaseURL)

		respBody := makeRequest(t, reqBody, http.StatusCreated)

		tnt, err := tenantManager.GetTenantByName(ctx, orgName)
		require.NoError(t, err)

		generatedUIURL := fmt.Sprintf("https://%s.sdp-ui.stellar.org", orgName)
		expectedRespBody := fmt.Sprintf(`
			{
				"id": %q,
				"name": %q,
				"base_url": %q,
				"sdp_ui_base_url": %q,
				"status": "TENANT_PROVISIONED",
				"is_default": false,
				"created_at": %q,
				"updated_at": %q,
				"deleted_at": null,
				"distribution_account_address": %q,
				"distribution_account_type": %q,
				"distribution_account_status": %q
			}
		`, tnt.ID, orgName, handler.BaseURL, generatedUIURL, tnt.CreatedAt.Format(time.RFC3339Nano), tnt.UpdatedAt.Format(time.RFC3339Nano),
			distAccAddress, accountType, schema.AccountStatusActive)
		assert.JSONEq(t, expectedRespBody, string(respBody))

		assertMigrations(orgName)
	})

	t.Run("provisions a new tenant successfully - dynamically generates only backend base URL", func(t *testing.T) {
		// TODO: in SDP-1167, send the accountType in the request body
		accountType := schema.DistributionAccountStellarEnv
		createMocks(t, accountType, nil)

		orgName := "aid-org-four"
		reqBody := fmt.Sprintf(`
			{
				"name": %q,
				"owner_email": "owner@email.org",
				"owner_first_name": "Owner",
				"owner_last_name": "Owner",
				"organization_name": "My Aid Org 4",
				"sdp_ui_base_url": %q,
				"is_default": false
			}
		`, orgName, handler.SDPUIBaseURL)

		respBody := makeRequest(t, reqBody, http.StatusCreated)

		tnt, err := tenantManager.GetTenantByName(ctx, orgName)
		require.NoError(t, err)

		generatedURL := fmt.Sprintf("https://%s.sdp-backend.stellar.org", orgName)
		expectedRespBody := fmt.Sprintf(`
			{
				"id": %q,
				"name": %q,
				"base_url": %q,
				"sdp_ui_base_url": %q,
				"status": "TENANT_PROVISIONED",
				"is_default": false,
				"created_at": %q,
				"updated_at": %q,
				"deleted_at": null,
				"distribution_account_address": %q,
				"distribution_account_type": %q,
				"distribution_account_status": %q
			}
		`, tnt.ID, orgName, generatedURL, handler.SDPUIBaseURL, tnt.CreatedAt.Format(time.RFC3339Nano), tnt.UpdatedAt.Format(time.RFC3339Nano),
			distAccAddress, accountType, schema.AccountStatusActive)
		assert.JSONEq(t, expectedRespBody, string(respBody))

		assertMigrations(orgName)
	})

	t.Run("returns badRequest for duplicate tenant name", func(t *testing.T) {
		// TODO: in SDP-1167, send the accountType in the request body
		accountType := schema.DistributionAccountStellarEnv
		createMocks(t, accountType, nil)

		reqBody := `
			{
				"name": "my-aid-org",
				"owner_email": "owner@email.org",
				"owner_first_name": "Owner",
				"owner_last_name": "Owner",
				"organization_name": "My Aid Org",
				"base_url": "https://backend.sdp.org",
				"sdp_ui_base_url": "https://aid-org.sdp.org"
			}
		`

		// make first request to create tenant
		_ = makeRequest(t, reqBody, http.StatusCreated)
		// attempt creating another tenant with the same name
		respBody := makeRequest(t, reqBody, http.StatusBadRequest)
		assert.JSONEq(t, `{"error": "Tenant name already exists"}`, string(respBody))
	})

	t.Run("logs and reports error when failing to send invitation message", func(t *testing.T) {
		accountType := schema.DistributionAccountStellarEnv
		createMocks(t, accountType, errors.New("foobar"))

		orgName := "aid-org-five"
		reqBody := fmt.Sprintf(`
			{
				"name": %q,
				"owner_email": "owner@email.org",
				"owner_first_name": "Owner",
				"owner_last_name": "Owner",
				"organization_name": "My Aid Org",
				"base_url": "https://sdp-backend.stellar.org",
				"sdp_ui_base_url": "https://sdp-ui.stellar.org",
				"is_default": false
			}
		`, orgName)

		respBody := makeRequest(t, reqBody, http.StatusCreated)

		tnt, err := tenantManager.GetTenantByName(ctx, orgName)
		require.NoError(t, err)

		expectedRespBody := fmt.Sprintf(`
			{
				"id": %q,
				"name": %q,
				"base_url": "https://sdp-backend.stellar.org",
				"sdp_ui_base_url": "https://sdp-ui.stellar.org",
				"status": "TENANT_PROVISIONED",
				"is_default": false,
				"created_at": %q,
				"updated_at": %q,
				"deleted_at": null,
				"distribution_account_address": %q,
				"distribution_account_type": %q,
				"distribution_account_status": %q
			}
		`, tnt.ID, orgName, tnt.CreatedAt.Format(time.RFC3339Nano), tnt.UpdatedAt.Format(time.RFC3339Nano),
			distAccAddress, schema.DistributionAccountStellarEnv, schema.AccountStatusActive)
		assert.JSONEq(t, expectedRespBody, string(respBody))

		assertMigrations(orgName)
	})

	messengerClientMock.AssertExpectations(t)
}

func Test_TenantHandler_Patch_error(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	tnt := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "aid-org", "GCTNUNQVX7BNIP5AUWW2R4YC7G6R3JGUDNMGT7H62BGBUY4A4V6ROAAH")
	defaultURL := "/tenants/" + tnt.ID

	testCases := []struct {
		name           string
		urlOverride    string
		prepareMocksFn func()
		reqBody        string
		initialStatus  tenant.TenantStatus
		expectedStatus int
		expectedBody   map[string]interface{}
	}{
		{
			name:           "400 response when the request body is empty",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   map[string]interface{}{"error": "The request was invalid in some way."},
		},
		{
			name:           "404 response when the tenant does not exist",
			urlOverride:    "/tenants/unknown",
			reqBody:        `{"base_url": "http://localhost"}`,
			expectedStatus: http.StatusNotFound,
			expectedBody:   map[string]interface{}{"error": "updating tenant: tenant unknown does not exist"},
		},
		{
			name:           "400 response when BaseURL is not valid",
			reqBody:        `{"base_url": "invalid base_url"}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody: map[string]interface{}{
				"error": "invalid request body",
				"extras": map[string]interface{}{
					"base_url": "invalid base URL value",
				},
			},
		},
		{
			name:           "400 response when SDPUIBaseURL is not valid",
			reqBody:        `{"sdp_ui_base_url": "invalid sdp_ui_base_url"}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody: map[string]interface{}{
				"error": "invalid request body",
				"extras": map[string]interface{}{
					"sdp_ui_base_url": "invalid SDP UI base URL value",
				},
			},
		},
		{
			name:           "400 response when Status is not valid",
			reqBody:        `{"status": "invalid status"}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody: map[string]interface{}{
				"error": "invalid request body",
				"extras": map[string]interface{}{
					"status": "invalid status value",
				},
			},
		},
		// status transition errors
		{
			name:           "400 response on status transition forbidden (deactivated->created)",
			initialStatus:  tenant.DeactivatedTenantStatus,
			reqBody:        fmt.Sprintf(`{"status": %q}`, tenant.CreatedTenantStatus),
			expectedStatus: http.StatusBadRequest,
			expectedBody:   map[string]interface{}{"error": "cannot perform update on tenant to requested status"},
		},
		{
			name:           "200 response on NOOP transition (deactivated->deactivated)",
			initialStatus:  tenant.DeactivatedTenantStatus,
			reqBody:        fmt.Sprintf(`{"status": %q}`, tenant.DeactivatedTenantStatus),
			expectedStatus: http.StatusOK,
			expectedBody: map[string]interface{}{
				"id":     tnt.ID,
				"status": string(tenant.DeactivatedTenantStatus),
			},
		},
		{
			name:           "200 response on NOOP transition (activated->activated)",
			initialStatus:  tenant.ActivatedTenantStatus,
			reqBody:        fmt.Sprintf(`{"status": %q}`, tenant.ActivatedTenantStatus),
			expectedStatus: http.StatusOK,
			expectedBody: map[string]interface{}{
				"id":     tnt.ID,
				"status": string(tenant.ActivatedTenantStatus),
			},
		},
		{
			name:           "400 response on status transition forbidden (activated->created)",
			initialStatus:  tenant.ActivatedTenantStatus,
			reqBody:        fmt.Sprintf(`{"status": %q}`, tenant.CreatedTenantStatus),
			expectedStatus: http.StatusBadRequest,
			expectedBody:   map[string]interface{}{"error": "cannot perform update on tenant to requested status"},
		},
		{
			name:          "400 response if attempting to deactivate a tenant with active payments",
			initialStatus: tenant.ActivatedTenantStatus,
			prepareMocksFn: func() {
				country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
				wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
				asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
				disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
					Country: country,
					Wallet:  wallet,
					Status:  data.ReadyDisbursementStatus,
					Asset:   asset,
				})
				receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
				rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.DraftReceiversWalletStatus)
				_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
					Amount:         "50",
					Status:         data.ReadyPaymentStatus, // <----- active payment will cause it to fail!
					Disbursement:   disbursement,
					Asset:          *asset,
					ReceiverWallet: rw,
				})
			},
			reqBody:        fmt.Sprintf(`{"status": %q}`, tenant.DeactivatedTenantStatus),
			expectedStatus: http.StatusBadRequest,
			expectedBody:   map[string]interface{}{"error": services.ErrCannotDeactivateTenantWithActivePayments.Error()},
		},
	}

	handler := TenantsHandler{
		Manager:               tenant.NewManager(tenant.WithDatabase(dbConnectionPool)),
		Models:                models,
		AdminDBConnectionPool: dbConnectionPool,
	}
	r := chi.NewRouter()
	r.Patch("/tenants/{id}", handler.Patch)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Enforce initial status
			if !utils.IsEmpty(tc.initialStatus) {
				_, err = handler.Manager.UpdateTenantConfig(ctx, &tenant.TenantUpdate{
					ID:     tnt.ID,
					Status: &tc.initialStatus,
				})
				require.NoError(t, err)
			}

			if tc.prepareMocksFn != nil {
				tc.prepareMocksFn()
			}

			// PATCH request
			url := defaultURL
			if tc.urlOverride != "" {
				url = tc.urlOverride
			}
			rr := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPatch, url, strings.NewReader(tc.reqBody))
			require.NoError(t, err)

			r.ServeHTTP(rr, req)

			resp := rr.Result()
			defer resp.Body.Close()
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			respMap := map[string]interface{}{}
			err = json.Unmarshal(respBody, &respMap)
			require.NoError(t, err)

			// Assert
			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
			assert.Subset(t, respMap, tc.expectedBody)
		})
	}
}

func Test_TenantHandler_Patch_success(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	testCases := []struct {
		name           string
		reqBody        string
		expectedBodyFn func(tnt *tenant.Tenant) map[string]interface{}
	}{
		{
			name:    "🎉 successfully updates the status",
			reqBody: `{"status": "TENANT_DEACTIVATED"}`,
			expectedBodyFn: func(tnt *tenant.Tenant) map[string]interface{} {
				return map[string]interface{}{
					"id":                           tnt.ID,
					"base_url":                     nil,
					"sdp_ui_base_url":              nil,
					"status":                       string(tenant.DeactivatedTenantStatus),
					"distribution_account_address": "GCTNUNQVX7BNIP5AUWW2R4YC7G6R3JGUDNMGT7H62BGBUY4A4V6ROAAH",
					"is_default":                   false,
				}
			},
		},
		{
			name:    "🎉 successfully updates the base_url",
			reqBody: `{"base_url": "http://valid.com"}`,
			expectedBodyFn: func(tnt *tenant.Tenant) map[string]interface{} {
				return map[string]interface{}{
					"id":                           tnt.ID,
					"base_url":                     "http://valid.com",
					"sdp_ui_base_url":              nil,
					"status":                       string(tenant.ActivatedTenantStatus),
					"distribution_account_address": "GCTNUNQVX7BNIP5AUWW2R4YC7G6R3JGUDNMGT7H62BGBUY4A4V6ROAAH",
					"is_default":                   false,
				}
			},
		},
		{
			name:    "🎉 successfully updates the sdp_ui_base_url",
			reqBody: `{"sdp_ui_base_url": "http://ui.valid.com"}`,
			expectedBodyFn: func(tnt *tenant.Tenant) map[string]interface{} {
				return map[string]interface{}{
					"id":                           tnt.ID,
					"base_url":                     nil,
					"sdp_ui_base_url":              "http://ui.valid.com",
					"status":                       string(tenant.ActivatedTenantStatus),
					"distribution_account_address": "GCTNUNQVX7BNIP5AUWW2R4YC7G6R3JGUDNMGT7H62BGBUY4A4V6ROAAH",
					"is_default":                   false,
				}
			},
		},
		{
			name: "🎉 successfully updates ALL fields",
			reqBody: `{
				"status": "TENANT_DEACTIVATED",
				"base_url": "http://valid.com",
				"sdp_ui_base_url": "http://ui.valid.com"
			}`,
			expectedBodyFn: func(tnt *tenant.Tenant) map[string]interface{} {
				return map[string]interface{}{
					"id":                           tnt.ID,
					"base_url":                     "http://valid.com",
					"sdp_ui_base_url":              "http://ui.valid.com",
					"status":                       string(tenant.DeactivatedTenantStatus),
					"distribution_account_address": "GCTNUNQVX7BNIP5AUWW2R4YC7G6R3JGUDNMGT7H62BGBUY4A4V6ROAAH",
					"is_default":                   false,
				}
			},
		},
	}

	handler := TenantsHandler{
		Manager:               tenant.NewManager(tenant.WithDatabase(dbConnectionPool)),
		Models:                models,
		AdminDBConnectionPool: dbConnectionPool,
	}
	r := chi.NewRouter()
	r.Patch("/tenants/{id}", handler.Patch)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tnt := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "aid-org", "GCTNUNQVX7BNIP5AUWW2R4YC7G6R3JGUDNMGT7H62BGBUY4A4V6ROAAH")
			defer tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

			// Enforce initial status
			initialStatus := tenant.ActivatedTenantStatus
			_, err = handler.Manager.UpdateTenantConfig(ctx, &tenant.TenantUpdate{
				ID:     tnt.ID,
				Status: &initialStatus,
			})
			require.NoError(t, err)

			// PATCH request
			rr := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPatch, "/tenants/"+tnt.ID, strings.NewReader(tc.reqBody))
			require.NoError(t, err)

			r.ServeHTTP(rr, req)
			resp := rr.Result()
			defer resp.Body.Close()
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			respMap := map[string]interface{}{}
			err = json.Unmarshal(respBody, &respMap)
			require.NoError(t, err)

			// Assert
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			expectedBody := tc.expectedBodyFn(tnt)
			assert.Subset(t, respMap, expectedBody)
		})
	}
}

func Test_TenantHandler_SetDefault(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	tenantManager := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
	handler := TenantsHandler{
		Manager:               tenantManager,
		AdminDBConnectionPool: dbConnectionPool,
		SingleTenantMode:      false,
	}

	updateTenantIsDefault := func(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, tenantID string, isDefault bool) {
		const q = "UPDATE tenants SET is_default = $1 WHERE id = $2"
		_, err = dbConnectionPool.ExecContext(ctx, q, isDefault, tenantID)
		require.NoError(t, err)
	}

	t.Run("returns Forbidden when default tenant feature is disabled", func(t *testing.T) {
		body := `{"id": "some-id"}`
		req := httptest.NewRequest(http.MethodPost, "/default-tenant", strings.NewReader(body))
		r := httptest.NewRecorder()

		http.HandlerFunc(handler.SetDefault).ServeHTTP(r, req)

		resp := r.Result()
		respBody, rErr := io.ReadAll(resp.Body)
		require.NoError(t, rErr)

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Single Tenant Mode feature is disabled. Please, enable it before setting a tenant as default."}`, string(respBody))
	})

	handler.SingleTenantMode = true
	t.Run("returns BadRequest when body is invalid", func(t *testing.T) {
		body := `invalid`
		req := httptest.NewRequest(http.MethodPost, "/default-tenant", strings.NewReader(body))
		r := httptest.NewRecorder()

		http.HandlerFunc(handler.SetDefault).ServeHTTP(r, req)

		resp := r.Result()
		respBody, rErr := io.ReadAll(resp.Body)
		require.NoError(t, rErr)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "The request was invalid in some way."}`, string(respBody))

		body = `{"id": "    "}`
		req = httptest.NewRequest(http.MethodPost, "/default-tenant", strings.NewReader(body))
		r = httptest.NewRecorder()

		http.HandlerFunc(handler.SetDefault).ServeHTTP(r, req)

		resp = r.Result()
		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Invalid request body", "extras": {"id": "id is required"}}`, string(respBody))
	})

	// creating tenants. tnt2 is the default.
	tnt1 := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "redcorp", keypair.MustRandom().Address())
	tnt2 := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "bluecorp", keypair.MustRandom().Address())
	updateTenantIsDefault(t, ctx, dbConnectionPool, tnt2.ID, true)

	t.Run("returns NotFound when tenant does not exist", func(t *testing.T) {
		body := `{"id": "some-id"}`
		req := httptest.NewRequest(http.MethodPost, "/default-tenant", strings.NewReader(body))
		r := httptest.NewRecorder()

		http.HandlerFunc(handler.SetDefault).ServeHTTP(r, req)

		resp := r.Result()
		respBody, rErr := io.ReadAll(resp.Body)
		require.NoError(t, rErr)

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		assert.JSONEq(t, `{"error": "tenant id some-id does not exist"}`, string(respBody))

		// Ensure the tnt2 still the default one
		tnt2DB, dErr := tenantManager.GetTenantByID(ctx, tnt2.ID)
		require.NoError(t, dErr)
		assert.True(t, tnt2DB.IsDefault)
	})

	t.Run("successfully updates the default tenant", func(t *testing.T) {
		body := fmt.Sprintf(`{"id": %q}`, tnt1.ID)
		req := httptest.NewRequest(http.MethodPost, "/default-tenant", strings.NewReader(body))
		r := httptest.NewRecorder()

		http.HandlerFunc(handler.SetDefault).ServeHTTP(r, req)

		resp := r.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		tnt1DB, err := tenantManager.GetTenantByID(ctx, tnt1.ID)
		require.NoError(t, err)
		assert.True(t, tnt1DB.IsDefault)

		tnt2DB, err := tenantManager.GetTenantByID(ctx, tnt2.ID)
		require.NoError(t, err)
		assert.False(t, tnt2DB.IsDefault)
	})
}

func Test_TenantHandler_Delete(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	tenantManagerMock := tenant.TenantManagerMock{}
	horizonClientMock := horizonclient.MockClient{}
	_, _, distAccResolver := signing.NewMockSignatureService(t)
	hostAccount := schema.NewDefaultHostAccount(keypair.MustRandom().Address())

	handler := TenantsHandler{
		Manager:                     &tenantManagerMock,
		NetworkType:                 utils.TestnetNetworkType,
		HorizonClient:               &horizonClientMock,
		DistributionAccountResolver: distAccResolver,
	}

	r := chi.NewRouter()
	r.Delete("/tenants/{id}", handler.Delete)
	tntID := "tntID"
	tntDistributionAcc := keypair.MustRandom().Address()
	deletedAt := time.Now()

	testCases := []struct {
		name             string
		id               string
		mockTntManagerFn func(tntManagerMock *tenant.TenantManagerMock, horizonClientMock *horizonclient.MockClient)
		expectedStatus   int
	}{
		{
			name: "tenant does not exist",
			id:   tntID,
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock, _ *horizonclient.MockClient) {
				tntManagerMock.On("GetTenant", mock.Anything, &tenant.QueryParams{
					Filters: map[tenant.FilterKey]interface{}{tenant.FilterKeyID: tntID},
				}).
					Return(nil, tenant.ErrTenantDoesNotExist).
					Once()
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name: "failed to retrieve tenant",
			id:   tntID,
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock, _ *horizonclient.MockClient) {
				tntManagerMock.On("GetTenant", mock.Anything, &tenant.QueryParams{
					Filters: map[tenant.FilterKey]interface{}{tenant.FilterKeyID: tntID},
				}).
					Return(nil, errors.New("foobar")).
					Once()
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "tenant is already deleted",
			id:   tntID,
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock, _ *horizonclient.MockClient) {
				tntManagerMock.On("GetTenant", mock.Anything, &tenant.QueryParams{
					Filters: map[tenant.FilterKey]interface{}{tenant.FilterKeyID: tntID},
				}).
					Return(&tenant.Tenant{ID: tntID, Status: tenant.CreatedTenantStatus, DeletedAt: &deletedAt}, nil).
					Once()
			},
			expectedStatus: http.StatusNotModified,
		},
		{
			name: "tenant is not deactivated",
			id:   tntID,
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock, _ *horizonclient.MockClient) {
				tntManagerMock.On("GetTenant", mock.Anything, &tenant.QueryParams{
					Filters: map[tenant.FilterKey]interface{}{tenant.FilterKeyID: tntID},
				}).
					Return(&tenant.Tenant{ID: tntID, Status: tenant.CreatedTenantStatus}, nil).
					Once()
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "failed to get Horizon account details for tenant distribution account",
			id:   tntID,
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock, _ *horizonclient.MockClient) {
				tntManagerMock.On("GetTenant", mock.Anything, &tenant.QueryParams{
					Filters: map[tenant.FilterKey]interface{}{tenant.FilterKeyID: tntID},
				}).
					Return(&tenant.Tenant{ID: tntID, Status: tenant.DeactivatedTenantStatus, DistributionAccountAddress: &tntDistributionAcc}, nil).
					Once()
				distAccResolver.On("HostDistributionAccount").Return(&hostAccount).Once()
				horizonClientMock.On("AccountDetail", horizonclient.AccountRequest{AccountID: tntDistributionAcc}).
					Return(horizon.Account{}, errors.New("foobar")).
					Once()
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "tenant distribution account still has non-zero non-native balance",
			id:   tntID,
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock, _ *horizonclient.MockClient) {
				tntManagerMock.On("GetTenant", mock.Anything, &tenant.QueryParams{
					Filters: map[tenant.FilterKey]interface{}{tenant.FilterKeyID: tntID},
				}).Return(&tenant.Tenant{ID: tntID, Status: tenant.DeactivatedTenantStatus, DistributionAccountAddress: &tntDistributionAcc}, nil).
					Once()
				distAccResolver.On("HostDistributionAccount").Return(&hostAccount).Once()
				horizonClientMock.On("AccountDetail", horizonclient.AccountRequest{AccountID: tntDistributionAcc}).
					Return(horizon.Account{
						Balances: []horizon.Balance{
							{Asset: base.Asset{Type: "credit_alphanum4"}, Balance: "100.0000000"},
						},
					}, nil).Once()
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "tenant distribution account still has native balance above the minimum threshold",
			id:   tntID,
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock, _ *horizonclient.MockClient) {
				tntManagerMock.On("GetTenant", mock.Anything, &tenant.QueryParams{
					Filters: map[tenant.FilterKey]interface{}{tenant.FilterKeyID: tntID},
				}).Return(&tenant.Tenant{ID: tntID, Status: tenant.DeactivatedTenantStatus, DistributionAccountAddress: &tntDistributionAcc}, nil).
					Once()
				distAccResolver.On("HostDistributionAccount").Return(&hostAccount).Once()
				horizonClientMock.On("AccountDetail", horizonclient.AccountRequest{AccountID: tntDistributionAcc}).
					Return(horizon.Account{
						Balances: []horizon.Balance{
							{Asset: base.Asset{Type: "native"}, Balance: "120.0000000"},
						},
					}, nil).Once()
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "failed to soft delete tenant",
			id:   tntID,
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock, horizonClientMock *horizonclient.MockClient) {
				tntManagerMock.On("GetTenant", mock.Anything, &tenant.QueryParams{
					Filters: map[tenant.FilterKey]interface{}{tenant.FilterKeyID: tntID},
				}).
					Return(&tenant.Tenant{ID: tntID, Status: tenant.DeactivatedTenantStatus, DistributionAccountAddress: &tntDistributionAcc}, nil).
					Once()
				distAccResolver.On("HostDistributionAccount").Return(&hostAccount).Once()
				horizonClientMock.On("AccountDetail", horizonclient.AccountRequest{AccountID: tntDistributionAcc}).
					Return(horizon.Account{Balances: make([]horizon.Balance, 0)}, nil).
					Once()
				tntManagerMock.On("SoftDeleteTenantByID", mock.Anything, tntID).
					Return(nil, errors.New("foobar")).
					Once()
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "soft deletes tenant: host and tenant distribution accounts are the same",
			id:   tntID,
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock, horizonClientMock *horizonclient.MockClient) {
				tntManagerMock.On("GetTenant", mock.Anything, &tenant.QueryParams{
					Filters: map[tenant.FilterKey]interface{}{tenant.FilterKeyID: tntID},
				}).
					Return(&tenant.Tenant{ID: tntID, Status: tenant.DeactivatedTenantStatus, DistributionAccountAddress: &tntDistributionAcc}, nil).
					Once()
				hAcc := schema.NewDefaultHostAccount(tntDistributionAcc)
				distAccResolver.On("HostDistributionAccount").Return(&hAcc).Once()
				tntManagerMock.On("SoftDeleteTenantByID", mock.Anything, tntID).
					Return(&tenant.Tenant{
						ID:                         tntID,
						Status:                     tenant.DeactivatedTenantStatus,
						DistributionAccountAddress: &tntDistributionAcc,
						DeletedAt:                  &deletedAt,
					}, nil).
					Once()
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "soft deletes tenant: host and tenant distribution accounts are different",
			id:   tntID,
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock, horizonClientMock *horizonclient.MockClient) {
				tntManagerMock.On("GetTenant", mock.Anything, &tenant.QueryParams{
					Filters: map[tenant.FilterKey]interface{}{tenant.FilterKeyID: tntID},
				}).
					Return(&tenant.Tenant{ID: tntID, Status: tenant.DeactivatedTenantStatus, DistributionAccountAddress: &tntDistributionAcc}, nil).
					Once()
				distAccResolver.On("HostDistributionAccount").Return(&hostAccount).Once()
				horizonClientMock.On("AccountDetail", horizonclient.AccountRequest{AccountID: tntDistributionAcc}).
					Return(horizon.Account{Balances: make([]horizon.Balance, 0)}, nil).
					Once()
				tntManagerMock.On("SoftDeleteTenantByID", mock.Anything, tntID).
					Return(&tenant.Tenant{
						ID:                         tntID,
						Status:                     tenant.DeactivatedTenantStatus,
						DistributionAccountAddress: &tntDistributionAcc,
						DeletedAt:                  &deletedAt,
					}, nil).
					Once()
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockTntManagerFn(&tenantManagerMock, &horizonClientMock)

			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/tenants/%s", tc.id), strings.NewReader(`{}`))
			r.ServeHTTP(rr, req)

			resp := rr.Result()
			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
			if tc.expectedStatus == http.StatusOK {
				respBody, rErr := io.ReadAll(resp.Body)
				require.NoError(t, rErr)

				respMap := map[string]interface{}{}
				err := json.Unmarshal(respBody, &respMap)
				require.NoError(t, err)

				assert.Subset(t, respMap, map[string]interface{}{"deleted_at": deletedAt.Format(time.RFC3339Nano)})
			}

			tenantManagerMock.AssertExpectations(t)
			horizonClientMock.AssertExpectations(t)
		})
	}
}
