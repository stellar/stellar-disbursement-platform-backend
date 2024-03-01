package httphandler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/internal/provisioning"
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
	assert.JSONEq(t, string(expectedRespBody), string(respBody))
}

func runSuccessfulRequestPatchTest(t *testing.T, r *chi.Mux, ctx context.Context, dbConnectionPool db.DBConnectionPool, handler TenantsHandler, reqBody, expectedRespBody string) {
	tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)
	tnt := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "aid-org")
	url := fmt.Sprintf("/tenants/%s", tnt.ID)

	rr := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPatch, url, strings.NewReader(reqBody))
	require.NoError(t, err)
	r.ServeHTTP(rr, req)

	resp := rr.Result()
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	tntDB, err := handler.Manager.GetTenantByName(ctx, "aid-org")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	expectedRespBody = fmt.Sprintf(`
		{
			"id": %q,
			"name": "aid-org",
			%s
			"created_at": %q,
			"updated_at": %q
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
		req, err := http.NewRequest(http.MethodGet, "/tenants", nil)
		require.NoError(t, err)
		http.HandlerFunc(handler.GetAll).ServeHTTP(rr, req)

		resp := rr.Result()
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `[]`, string(respBody))
	})

	tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)
	tnt1 := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "myorg1")
	tnt2 := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "myorg2")

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
					"email_sender_type": "DRY_RUN",
					"sms_sender_type": "DRY_RUN",
					"base_url": null,
					"sdp_ui_base_url": null,
					"status": "TENANT_CREATED",
					"distribution_account": null,
					"created_at": %q,
					"updated_at": %q
				},
				{
					"id": %q,
					"name": %q,
					"email_sender_type": "DRY_RUN",
					"sms_sender_type": "DRY_RUN",
					"base_url": null,
					"sdp_ui_base_url": null,
					"status": "TENANT_CREATED",
					"distribution_account": null,
					"created_at": %q,
					"updated_at": %q
				}
			]
		`, tnt1.ID, tnt1.Name, tnt1.CreatedAt.Format(time.RFC3339Nano), tnt1.UpdatedAt.Format(time.RFC3339Nano), tnt2.ID, tnt2.Name, tnt2.CreatedAt.Format(time.RFC3339Nano), tnt2.UpdatedAt.Format(time.RFC3339Nano))
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
				"email_sender_type": "DRY_RUN",
				"sms_sender_type": "DRY_RUN",
				"base_url": null,
				"sdp_ui_base_url": null,
				"status": "TENANT_CREATED",
				"distribution_account": null,
				"created_at": %q,
				"updated_at": %q
			}
		`, tnt1.ID, tnt1.Name, tnt1.CreatedAt.Format(time.RFC3339Nano), tnt1.UpdatedAt.Format(time.RFC3339Nano))
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
				"email_sender_type": "DRY_RUN",
				"sms_sender_type": "DRY_RUN",
				"base_url": null,
				"sdp_ui_base_url": null,
				"status": "TENANT_CREATED",
				"distribution_account": null,
				"created_at": %q,
				"updated_at": %q
			}
		`, tnt2.ID, tnt2.Name, tnt2.CreatedAt.Format(time.RFC3339Nano), tnt2.UpdatedAt.Format(time.RFC3339Nano))
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

	distAcc := keypair.MustRandom()
	t.Setenv("DISTRIBUTION_SEED", distAcc.Seed())
	distAccSigClientMock := sigMocks.NewMockSignatureClient(t)

	ctx := context.Background()
	messengerClientMock := message.MessengerClientMock{}
	m := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
	p := provisioning.NewManager(
		provisioning.WithDatabase(dbConnectionPool),
		provisioning.WithTenantManager(m),
		provisioning.WithMessengerClient(&messengerClientMock),
		provisioning.WithDistributionAccountSignatureClient(distAccSigClientMock),
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
					"email_sender_type": "invalid email sender type. Expected one of these values: [AWS_EMAIL DRY_RUN]",
					"sms_sender_type": "invalid sms sender type. Expected one of these values: [TWILIO_SMS AWS_SMS DRY_RUN]",
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

		distAccSigClientMock.
			On("BatchInsert", ctx, 1).
			Return([]string{distAcc.Address()}, nil).
			Once()

		reqBody := `
			{
				"name": "aid-org",
				"owner_email": "owner@email.org",
				"owner_first_name": "Owner",
				"owner_last_name": "Owner",
				"organization_name": "My Aid Org",
				"email_sender_type": "DRY_RUN",
				"sms_sender_type": "DRY_RUN",
				"base_url": "https://backend.sdp.org",
				"sdp_ui_base_url": "https://aid-org.sdp.org"
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
				"email_sender_type": "DRY_RUN",
				"sms_sender_type": "DRY_RUN",
				"base_url": "https://backend.sdp.org",
				"sdp_ui_base_url": "https://aid-org.sdp.org",
				"status": "TENANT_PROVISIONED",
				"distribution_account": %q,
				"created_at": %q,
				"updated_at": %q
			}
		`, tnt.ID, distAcc.Address(), tnt.CreatedAt.Format(time.RFC3339Nano), tnt.UpdatedAt.Format(time.RFC3339Nano))
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

		distAccSigClientMock.
			On("BatchInsert", ctx, 1).
			Return([]string{distAcc.Address()}, nil).
			Once()

		reqBody := `
			{
				"name": "my-aid-org",
				"owner_email": "owner@email.org",
				"owner_first_name": "Owner",
				"owner_last_name": "Owner",
				"organization_name": "My Aid Org",
				"email_sender_type": "DRY_RUN",
				"sms_sender_type": "DRY_RUN",
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
}

func Test_TenantHandler_Patch(t *testing.T) {
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
	r.Patch("/tenants/{id}", handler.Patch)

	tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)
	tnt := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "aid-org")
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
		req, err := http.NewRequest(http.MethodPatch, "/tenants/unknown", strings.NewReader(`{"email_sender_type": "AWS_EMAIL"}`))
		require.NoError(t, err)
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		expectedRespBody := `{"error":"updating tenant: tenant unknown does not exist"}`
		assert.JSONEq(t, string(expectedRespBody), string(respBody))
	})

	t.Run("returns BadRequest when EmailSenderType is not valid", func(t *testing.T) {
		runBadRequestPatchTest(t, r, url, "email_sender_type", "invalid email sender type. Expected one of these values: [AWS_EMAIL DRY_RUN]")
	})

	t.Run("returns BadRequest when SMSSenderType is not valid", func(t *testing.T) {
		runBadRequestPatchTest(t, r, url, "sms_sender_type", "invalid sms sender type. Expected one of these values: [TWILIO_SMS AWS_SMS DRY_RUN]")
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

	t.Run("successfully updates EmailSenderType of a tenant", func(t *testing.T) {
		reqBody := `{"email_sender_type": "AWS_EMAIL"}`
		expectedRespBody := `
			"email_sender_type": "AWS_EMAIL",
			"sms_sender_type": "DRY_RUN",
			"base_url": null,
			"sdp_ui_base_url": null,
			"status": "TENANT_CREATED",
			"distribution_account": null,
		`

		runSuccessfulRequestPatchTest(t, r, ctx, dbConnectionPool, handler, reqBody, expectedRespBody)
	})

	t.Run("successfully updates SMSSenderType of a tenant", func(t *testing.T) {
		reqBody := `{"SMS_sender_type": "TWILIO_SMS"}`
		expectedRespBody := `
			"email_sender_type": "DRY_RUN",
			"sms_sender_type": "TWILIO_SMS",
			"base_url": null,
			"sdp_ui_base_url": null,
			"status": "TENANT_CREATED",
			"distribution_account": null,
		`

		runSuccessfulRequestPatchTest(t, r, ctx, dbConnectionPool, handler, reqBody, expectedRespBody)
	})

	t.Run("successfully updates BaseURL of a tenant", func(t *testing.T) {
		reqBody := `{"base_url": "http://valid.com"}`
		expectedRespBody := `
			"email_sender_type": "DRY_RUN",
			"sms_sender_type": "DRY_RUN",
			"base_url": "http://valid.com",
			"sdp_ui_base_url": null,
			"status": "TENANT_CREATED",
			"distribution_account": null,
		`

		runSuccessfulRequestPatchTest(t, r, ctx, dbConnectionPool, handler, reqBody, expectedRespBody)
	})

	t.Run("successfully updates SDPUIBaseURL of a tenant", func(t *testing.T) {
		reqBody := `{"sdp_ui_base_url": "http://valid.com"}`
		expectedRespBody := `
			"email_sender_type": "DRY_RUN",
			"sms_sender_type": "DRY_RUN",
			"base_url": null,
			"sdp_ui_base_url": "http://valid.com",
			"status": "TENANT_CREATED",
			"distribution_account": null,
		`

		runSuccessfulRequestPatchTest(t, r, ctx, dbConnectionPool, handler, reqBody, expectedRespBody)
	})

	t.Run("successfully updates Status of a tenant", func(t *testing.T) {
		reqBody := `{"status": "TENANT_ACTIVATED"}`
		expectedRespBody := `
			"email_sender_type": "DRY_RUN",
			"sms_sender_type": "DRY_RUN",
			"base_url": null,
			"sdp_ui_base_url": null,
			"status": "TENANT_ACTIVATED",
			"distribution_account": null,
		`

		runSuccessfulRequestPatchTest(t, r, ctx, dbConnectionPool, handler, reqBody, expectedRespBody)
	})

	t.Run("successfully updates Distribution Account of a tenant", func(t *testing.T) {
		reqBody := `{"distribution_account": "GAAFQ2NZRRELBKLNLRZ5CT5RENLQGJPHDM6YY5UKV5UDAFNZ6KD6J4W7"}`
		expectedRespBody := `
			"email_sender_type": "DRY_RUN",
			"sms_sender_type": "DRY_RUN",
			"base_url": null,
			"sdp_ui_base_url": null,
			"status": "TENANT_CREATED",
			"distribution_account": "GAAFQ2NZRRELBKLNLRZ5CT5RENLQGJPHDM6YY5UKV5UDAFNZ6KD6J4W7",
		`

		runSuccessfulRequestPatchTest(t, r, ctx, dbConnectionPool, handler, reqBody, expectedRespBody)
	})

	t.Run("successfully updates all fields of a tenant", func(t *testing.T) {
		reqBody := `{
			"email_sender_type": "AWS_EMAIL",
			"sms_sender_type": "AWS_SMS",
			"base_url": "http://valid.com",
			"sdp_ui_base_url": "http://valid.com",
			"status": "TENANT_ACTIVATED",
			"distribution_account": "GAAFQ2NZRRELBKLNLRZ5CT5RENLQGJPHDM6YY5UKV5UDAFNZ6KD6J4W7"
		}`

		expectedRespBody := `
			"email_sender_type": "AWS_EMAIL",
			"sms_sender_type": "AWS_SMS",
			"base_url": "http://valid.com",
			"sdp_ui_base_url": "http://valid.com",
			"status": "TENANT_ACTIVATED",
			"distribution_account": "GAAFQ2NZRRELBKLNLRZ5CT5RENLQGJPHDM6YY5UKV5UDAFNZ6KD6J4W7",
		`

		runSuccessfulRequestPatchTest(t, r, ctx, dbConnectionPool, handler, reqBody, expectedRespBody)
	})
}
