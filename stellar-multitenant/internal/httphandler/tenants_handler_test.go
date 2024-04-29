package httphandler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/internal/provisioning"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func runBadRequestPatchTest(t *testing.T, r *chi.Mux, url, fieldName, errorMsg string) {
	rr := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPatch, url, strings.NewReader(fmt.Sprintf(`{"%s": "invalid"}`, fieldName)))
	require.NoError(t, err)
	r.ServeHTTP(rr, req)

	resp := rr.Result()
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	expectedRespBody := fmt.Sprintf(`{
		"error": "invalid request body",
		"extras": {
			"%s": "%s"
		}
	}`, fieldName, errorMsg)
	assert.JSONEq(t, expectedRespBody, string(respBody))
}

func runRequestStatusUpdatePatchTest(t *testing.T, r *chi.Mux, ctx context.Context, dbConnectionPool db.DBConnectionPool, handler TenantsHandler, getEntries func() []logrus.Entry, originalStatus, reqStatus tenant.TenantStatus, errorMsg string) {
	tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)
	tnt := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "aid-org", "GCTNUNQVX7BNIP5AUWW2R4YC7G6R3JGUDNMGT7H62BGBUY4A4V6ROAAH")

	_, err := handler.Manager.UpdateTenantConfig(ctx, &tenant.TenantUpdate{ID: tnt.ID, Status: &originalStatus})
	require.NoError(t, err)

	url := fmt.Sprintf("/tenants/%s", tnt.ID)

	rr := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPatch, url, strings.NewReader(fmt.Sprintf(`{"status": "%s", "id": "%s"}`, reqStatus, tnt.ID)))
	require.NoError(t, err)
	r.ServeHTTP(rr, req)

	resp := rr.Result()
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	if originalStatus == reqStatus {
		assert.Contains(t, string(respBody), string(reqStatus))
		if getEntries != nil {
			entries := getEntries()
			if reqStatus == tenant.DeactivatedTenantStatus {
				assert.Contains(t, fmt.Sprintf("tenant %s is already deactivated", tnt.ID), entries[0].Message)
			} else if reqStatus == tenant.ActivatedTenantStatus {
				assert.Contains(t, fmt.Sprintf("tenant %s is already activated", tnt.ID), entries[0].Message)
			}
		}
	} else {
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		expectedRespBody := fmt.Sprintf(`{
			"error": "%s"
		}`, errorMsg)
		assert.JSONEq(t, expectedRespBody, string(respBody))
	}
}

func runSuccessfulRequestPatchTest(t *testing.T, r *chi.Mux, ctx context.Context, dbConnectionPool db.DBConnectionPool, handler TenantsHandler, reqBody, expectedRespBody string, tntStatus *tenant.TenantStatus) {
	tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)
	tnt := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "aid-org", "GCTNUNQVX7BNIP5AUWW2R4YC7G6R3JGUDNMGT7H62BGBUY4A4V6ROAAH")
	url := fmt.Sprintf("/tenants/%s", tnt.ID)

	if tntStatus != nil {
		_, err := handler.Manager.UpdateTenantConfig(ctx, &tenant.TenantUpdate{ID: tnt.ID, Status: tntStatus})
		require.NoError(t, err)
	}

	rr := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPatch, url, strings.NewReader(reqBody))
	require.NoError(t, err)
	r.ServeHTTP(rr, req)

	resp := rr.Result()
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	tntDB, err := handler.Manager.GetTenant(ctx, &tenant.QueryParams{Filters: map[tenant.FilterKey]interface{}{
		tenant.FilterKeyName: tnt.Name,
	}})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	expectedRespBody = fmt.Sprintf(`
		{
			"id": %q,
			"name": "aid-org",
			%s
			"created_at": %q,
			"updated_at": %q,
			"deleted_at": null
		}
	`, tnt.ID, expectedRespBody, tnt.CreatedAt.Format(time.RFC3339Nano), tntDB.UpdatedAt.Format(time.RFC3339Nano))

	assert.JSONEq(t, expectedRespBody, string(respBody))
}

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
					"distribution_account": %q,
					"is_default": false,
					"created_at": %q,
					"updated_at": %q,
					"deleted_at": null
				},
				{
					"id": %q,
					"name": %q,
					"base_url": null,
					"sdp_ui_base_url": null,
					"status": "TENANT_CREATED",
					"distribution_account": %q,
					"is_default": false,
					"created_at": %q,
					"updated_at": %q,
					"deleted_at": null
				},
				{
					"id": %q,
					"name": %q,
					"base_url": null,
					"sdp_ui_base_url": null,
					"status": "TENANT_DEACTIVATED",
					"distribution_account": %q,
					"is_default": false,
					"created_at": %q,
					"updated_at": %q,
					"deleted_at": null
				}
			]
		`,
			tnt1.ID, tnt1.Name, *tnt1.DistributionAccount, tnt1.CreatedAt.Format(time.RFC3339Nano), tnt1.UpdatedAt.Format(time.RFC3339Nano),
			tnt2.ID, tnt2.Name, *tnt2.DistributionAccount, tnt2.CreatedAt.Format(time.RFC3339Nano), tnt2.UpdatedAt.Format(time.RFC3339Nano),
			deactivatedTnt.ID, deactivatedTnt.Name, *deactivatedTnt.DistributionAccount, deactivatedTnt.CreatedAt.Format(time.RFC3339Nano), deactivatedTnt.UpdatedAt.Format(time.RFC3339Nano))
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
				"distribution_account": %q,
				"is_default": false,
				"created_at": %q,
				"updated_at": %q,
				"deleted_at": null
			}
		`, tnt1.ID, tnt1.Name, *tnt1.DistributionAccount, tnt1.CreatedAt.Format(time.RFC3339Nano), tnt1.UpdatedAt.Format(time.RFC3339Nano))
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
				"distribution_account": %q,
				"is_default": false,
				"created_at": %q,
				"updated_at": %q,
				"deleted_at": null
			}
		`, tnt2.ID, tnt2.Name, *tnt2.DistributionAccount, tnt2.CreatedAt.Format(time.RFC3339Nano), tnt2.UpdatedAt.Format(time.RFC3339Nano))
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
	messengerClientMock := message.MessengerClientMock{}
	m := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))

	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, _, distAccSigClient, _, distAccResolver := signing.NewMockSignatureService(t)

	distAcc := keypair.MustRandom().Address()

	submitterEngine := engine.SubmitterEngine{
		HorizonClient:       mHorizonClient,
		SignatureService:    sigService,
		LedgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100 * txnbuild.MinBaseFee,
	}

	p := provisioning.NewManager(
		provisioning.WithDatabase(dbConnectionPool),
		provisioning.WithTenantManager(m),
		provisioning.WithMessengerClient(&messengerClientMock),
		provisioning.WithSubmitterEngine(submitterEngine),
		provisioning.WithNativeAssetBootstrapAmount(tenant.MinTenantDistributionAccountAmount),
	)

	handler := TenantsHandler{
		Manager:             m,
		ProvisioningManager: p,
		NetworkType:         utils.TestnetNetworkType,
	}

	t.Run("returns BadRequest with invalid request body", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/tenants", strings.NewReader(`{}`))
		require.NoError(t, err)
		http.HandlerFunc(handler.Post).ServeHTTP(rr, req)

		resp := rr.Result()
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		expectedBody := `
			{
				"error": "invalid request body",
				"extras": {
					"name": "invalid tenant name. It should only contains lower case letters and dash (-)",
					"owner_email": "invalid email",
					"owner_first_name": "owner_first_name is required",
					"owner_last_name": "owner_last_name is required",
					"organization_name": "organization_name is required",
					"base_url": "invalid base URL value",
					"sdp_ui_base_url": "invalid SDP UI base URL value"
				}
			}
		`
		assert.JSONEq(t, expectedBody, string(respBody))
	})

	t.Run("provisions a new tenant successfully", func(t *testing.T) {
		messengerClientMock.
			On("SendMessage", mock.AnythingOfType("message.Message")).
			Run(func(args mock.Arguments) {
				msg, ok := args.Get(0).(message.Message)
				require.True(t, ok)

				assert.Equal(t, "Welcome to Stellar Disbursement Platform", msg.Title)
				assert.Equal(t, "owner@email.org", msg.ToEmail)
				assert.Empty(t, msg.ToPhoneNumber)
			}).
			Return(nil).
			Once()

		distAccSigClient.
			On("BatchInsert", ctx, 1).
			Return([]string{distAcc}, nil).
			Once()

		distAccResolver.
			On("HostDistributionAccount").
			Return(distAcc, nil).
			Once()

		reqBody := `
			{
				"name": "aid-org",
				"owner_email": "owner@email.org",
				"owner_first_name": "Owner",
				"owner_last_name": "Owner",
				"organization_name": "My Aid Org",
				"base_url": "https://backend.sdp.org",
				"sdp_ui_base_url": "https://aid-org.sdp.org",
				"is_default": false
			}
		`

		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/tenants", strings.NewReader(reqBody))
		require.NoError(t, err)
		http.HandlerFunc(handler.Post).ServeHTTP(rr, req)

		resp := rr.Result()
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		tnt, err := m.GetTenantByName(ctx, "aid-org")
		require.NoError(t, err)

		expectedRespBody := fmt.Sprintf(`
			{
				"id": %q,
				"name": "aid-org",
				"base_url": "https://backend.sdp.org",
				"sdp_ui_base_url": "https://aid-org.sdp.org",
				"status": "TENANT_PROVISIONED",
				"distribution_account": %q,
				"is_default": false,
				"created_at": %q,
				"updated_at": %q,
				"deleted_at": null
			}
		`, tnt.ID, distAcc, tnt.CreatedAt.Format(time.RFC3339Nano), tnt.UpdatedAt.Format(time.RFC3339Nano))
		assert.JSONEq(t, expectedRespBody, string(respBody))

		// Validating infrastructure
		expectedSchema := "sdp_aid-org"
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

		dsn, err := m.GetDSNForTenant(ctx, "aid-org")
		require.NoError(t, err)

		tenantSchemaConnectionPool, err := db.OpenDBConnectionPool(dsn)
		require.NoError(t, err)
		defer tenantSchemaConnectionPool.Close()

		tenant.AssertRegisteredAssetsFixture(t, ctx, tenantSchemaConnectionPool, []string{"USDC:GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", "XLM:"})
		tenant.AssertRegisteredWalletsFixture(t, ctx, tenantSchemaConnectionPool, []string{"Demo Wallet", "Vibrant Assist"})
		tenant.AssertRegisteredUserFixture(t, ctx, tenantSchemaConnectionPool, "Owner", "Owner", "owner@email.org")
	})

	t.Run("returns badRequest for duplicate tenant name", func(t *testing.T) {
		messengerClientMock.
			On("SendMessage", mock.AnythingOfType("message.Message")).
			Run(func(args mock.Arguments) {
				msg, ok := args.Get(0).(message.Message)
				require.True(t, ok)

				assert.Equal(t, "Welcome to Stellar Disbursement Platform", msg.Title)
				assert.Equal(t, "owner@email.org", msg.ToEmail)
				assert.Empty(t, msg.ToPhoneNumber)
			}).
			Return(nil).
			Once()

		distAccSigClient.
			On("BatchInsert", ctx, 1).
			Return([]string{distAcc}, nil).
			Once()

		distAccResolver.
			On("HostDistributionAccount").
			Return(distAcc, nil).
			Once()

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

		createTenantReq := func() *http.Request {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/tenants", strings.NewReader(reqBody))
			require.NoError(t, err)
			return req
		}

		// create tenant
		rr := httptest.NewRecorder()
		http.HandlerFunc(handler.Post).ServeHTTP(rr, createTenantReq())

		resp := rr.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		// attempt creating another tenant with the same name
		rr = httptest.NewRecorder()
		http.HandlerFunc(handler.Post).ServeHTTP(rr, createTenantReq())

		resp = rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NotNil(t, respBody)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Tenant name already exists"}`, string(respBody))
	})

	messengerClientMock.AssertExpectations(t)
	distAccSigClient.AssertExpectations(t)
	distAccResolver.AssertExpectations(t)
}

func Test_TenantHandler_Patch(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	handler := TenantsHandler{
		Manager: tenant.NewManager(tenant.WithDatabase(dbConnectionPool)),
		Models:  models,
	}

	r := chi.NewRouter()
	r.Patch("/tenants/{id}", handler.Patch)

	tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)
	tnt := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "aid-org", "GCTNUNQVX7BNIP5AUWW2R4YC7G6R3JGUDNMGT7H62BGBUY4A4V6ROAAH")
	url := fmt.Sprintf("/tenants/%s", tnt.ID)

	t.Run("returns BadRequest with empty body", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodPatch, url, strings.NewReader(`{}`))
		require.NoError(t, err)
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		expectedBody := fmt.Sprintf(`{"error": "updating tenant %s: provide at least one field to be updated"}`, tnt.ID)
		assert.JSONEq(t, expectedBody, string(respBody))
	})

	t.Run("returns NotFound when tenant does not exist", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodPatch, "/tenants/unknown", strings.NewReader(`{"base_url": "http://localhost"}`))
		require.NoError(t, err)
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		expectedRespBody := `{"error":"updating tenant: tenant unknown does not exist"}`
		assert.JSONEq(t, expectedRespBody, string(respBody))
	})

	t.Run("returns BadRequest when BaseURL is not valid", func(t *testing.T) {
		runBadRequestPatchTest(t, r, url, "base_url", "invalid base URL value")
	})

	t.Run("returns BadRequest when SDPUIBaseURL is not valid", func(t *testing.T) {
		runBadRequestPatchTest(t, r, url, "sdp_ui_base_url", "invalid SDP UI base URL value")
	})

	t.Run("returns BadRequest when Status is not valid", func(t *testing.T) {
		runBadRequestPatchTest(t, r, url, "status", "invalid status value")
	})

	t.Run("successfully updates status of a tenant to be deactivated", func(t *testing.T) {
		reqBody := `{"status": "TENANT_DEACTIVATED"}`
		expectedRespBody := `
			"base_url": null,
			"sdp_ui_base_url": null,
			"status": "TENANT_DEACTIVATED",
			"distribution_account": "GCTNUNQVX7BNIP5AUWW2R4YC7G6R3JGUDNMGT7H62BGBUY4A4V6ROAAH",
			"is_default": false,
		`

		runSuccessfulRequestPatchTest(t, r, ctx, dbConnectionPool, handler, reqBody, expectedRespBody, nil)
	})

	t.Run("successfully updates BaseURL of a tenant", func(t *testing.T) {
		reqBody := `{"base_url": "http://valid.com"}`
		expectedRespBody := `
			"base_url": "http://valid.com",
			"sdp_ui_base_url": null,
			"status": "TENANT_CREATED",
			"distribution_account": "GCTNUNQVX7BNIP5AUWW2R4YC7G6R3JGUDNMGT7H62BGBUY4A4V6ROAAH",
			"is_default": false,
		`

		runSuccessfulRequestPatchTest(t, r, ctx, dbConnectionPool, handler, reqBody, expectedRespBody, nil)
	})

	t.Run("successfully updates SDPUIBaseURL of a tenant", func(t *testing.T) {
		reqBody := `{"sdp_ui_base_url": "http://valid.com"}`
		expectedRespBody := `
			"base_url": null,
			"sdp_ui_base_url": "http://valid.com",
			"status": "TENANT_CREATED",
			"distribution_account": "GCTNUNQVX7BNIP5AUWW2R4YC7G6R3JGUDNMGT7H62BGBUY4A4V6ROAAH",
			"is_default": false,
		`

		runSuccessfulRequestPatchTest(t, r, ctx, dbConnectionPool, handler, reqBody, expectedRespBody, nil)
	})

	t.Run("successfully updates status of a tenant", func(t *testing.T) {
		reqBody := `{"status": "TENANT_DEACTIVATED"}`
		expectedRespBody := `
			"base_url": null,
			"sdp_ui_base_url": null,
			"status": "TENANT_DEACTIVATED",
			"distribution_account": "GCTNUNQVX7BNIP5AUWW2R4YC7G6R3JGUDNMGT7H62BGBUY4A4V6ROAAH",
			"is_default": false,
		`

		tntStatus := tenant.ActivatedTenantStatus
		runSuccessfulRequestPatchTest(t, r, ctx, dbConnectionPool, handler, reqBody, expectedRespBody, &tntStatus)
	})

	t.Run("cannot update status of a tenant - invalid status", func(t *testing.T) {
		runRequestStatusUpdatePatchTest(t, r, ctx, dbConnectionPool, handler, nil, tenant.DeactivatedTenantStatus, tenant.CreatedTenantStatus, services.ErrCannotPerformStatusUpdate.Error())
	})

	t.Run("cannot update status of a tenant - same status (deactivated)", func(t *testing.T) {
		getEntries := log.DefaultLogger.StartTest(log.WarnLevel)
		runRequestStatusUpdatePatchTest(t, r, ctx, dbConnectionPool, handler, getEntries, tenant.DeactivatedTenantStatus, tenant.DeactivatedTenantStatus, "")
	})

	t.Run("cannot update status of a tenant - same status (activated)", func(t *testing.T) {
		getEntries := log.DefaultLogger.StartTest(log.WarnLevel)
		runRequestStatusUpdatePatchTest(t, r, ctx, dbConnectionPool, handler, getEntries, tenant.ActivatedTenantStatus, tenant.ActivatedTenantStatus, "")
	})

	t.Run("cannot update status of a tenant from activated to another status other than deactivated", func(t *testing.T) {
		runRequestStatusUpdatePatchTest(t, r, ctx, dbConnectionPool, handler, nil, tenant.ActivatedTenantStatus, tenant.CreatedTenantStatus, services.ErrCannotPerformStatusUpdate.Error())
	})

	t.Run("cannot update status of tenant from activated to deactivated if payments are not in terminal state", func(t *testing.T) {
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
			Status:         data.ReadyPaymentStatus,
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: rw,
		})

		runRequestStatusUpdatePatchTest(t, r, ctx, dbConnectionPool, handler, nil, tenant.ActivatedTenantStatus, tenant.DeactivatedTenantStatus, services.ErrCannotDeactivateTenantWithActivePayments.Error())
	})

	t.Run("successfully updates all fields of a tenant", func(t *testing.T) {
		reqBody := `{
			"base_url": "http://valid.com",
			"sdp_ui_base_url": "http://valid.com",
			"status": "TENANT_ACTIVATED"
		}`

		expectedRespBody := `
			"base_url": "http://valid.com",
			"sdp_ui_base_url": "http://valid.com",
			"status": "TENANT_ACTIVATED",
			"distribution_account": "GCTNUNQVX7BNIP5AUWW2R4YC7G6R3JGUDNMGT7H62BGBUY4A4V6ROAAH",
			"is_default": false,
		`

		tntStatus := tenant.DeactivatedTenantStatus
		runSuccessfulRequestPatchTest(t, r, ctx, dbConnectionPool, handler, reqBody, expectedRespBody, &tntStatus)
	})
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
	_, _, _, _, distAccResolver := signing.NewMockSignatureService(t)

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
					Return(&tenant.Tenant{ID: tntID, Status: tenant.DeactivatedTenantStatus, DistributionAccount: &tntDistributionAcc}, nil).
					Once()
				distAccResolver.On("HostDistributionAccount").Return("host-dist-account").Once()
				horizonClientMock.On("AccountDetail", horizonclient.AccountRequest{AccountID: tntDistributionAcc}).
					Return(horizon.Account{}, errors.New("foobar")).
					Once()
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "tenant distribution account still has valid balance",
			id:   tntID,
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock, _ *horizonclient.MockClient) {
				tntManagerMock.On("GetTenant", mock.Anything, &tenant.QueryParams{
					Filters: map[tenant.FilterKey]interface{}{tenant.FilterKeyID: tntID},
				}).Return(&tenant.Tenant{ID: tntID, Status: tenant.DeactivatedTenantStatus, DistributionAccount: &tntDistributionAcc}, nil).
					Once()
				distAccResolver.On("HostDistributionAccount").Return("host-dist-account").Once()
				horizonClientMock.On("AccountDetail", horizonclient.AccountRequest{AccountID: tntDistributionAcc}).
					Return(horizon.Account{
						Balances: []horizon.Balance{
							{Balance: "100.0000000"},
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
					Return(&tenant.Tenant{ID: tntID, Status: tenant.DeactivatedTenantStatus, DistributionAccount: &tntDistributionAcc}, nil).
					Once()
				distAccResolver.On("HostDistributionAccount").Return("host-dist-account").Once()
				horizonClientMock.On("AccountDetail", horizonclient.AccountRequest{AccountID: tntDistributionAcc}).
					Return(horizon.Account{Balances: make([]horizon.Balance, 0)}, nil).
					Once()
				tntManagerMock.On("SoftDeleteTenantByID", mock.Anything, tntID).
					Return(errors.New("foobar")).
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
					Return(&tenant.Tenant{ID: tntID, Status: tenant.DeactivatedTenantStatus, DistributionAccount: &tntDistributionAcc}, nil).
					Once()
				distAccResolver.On("HostDistributionAccount").Return(tntDistributionAcc).Once()
				tntManagerMock.On("SoftDeleteTenantByID", mock.Anything, tntID).
					Return(nil).
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
					Return(&tenant.Tenant{ID: tntID, Status: tenant.DeactivatedTenantStatus, DistributionAccount: &tntDistributionAcc}, nil).
					Once()
				distAccResolver.On("HostDistributionAccount").Return("host-dist-account").Once()
				horizonClientMock.On("AccountDetail", horizonclient.AccountRequest{AccountID: tntDistributionAcc}).
					Return(horizon.Account{Balances: make([]horizon.Balance, 0)}, nil).
					Once()
				tntManagerMock.On("SoftDeleteTenantByID", mock.Anything, tntID).
					Return(nil).
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

			tenantManagerMock.AssertExpectations(t)
			horizonClientMock.AssertExpectations(t)
		})
	}
}
