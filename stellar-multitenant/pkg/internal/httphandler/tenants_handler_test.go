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
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/internal/provisioning"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_Get(t *testing.T) {
	dbt := dbtest.OpenWithTenantMigrationsOnly(t)
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
					"sep10_signing_public_key": null,
					"distribution_public_key": null,
					"enable_mfa": true,
					"enable_recaptcha": true,
					"cors_allowed_origins": null,
					"base_url": null,
					"sdp_ui_base_url": null,
					"status": "TENANT_CREATED",
					"created_at": %q,
					"updated_at": %q
				},
				{
					"id": %q,
					"name": %q,
					"email_sender_type": "DRY_RUN",
					"sms_sender_type": "DRY_RUN",
					"sep10_signing_public_key": null,
					"distribution_public_key": null,
					"enable_mfa": true,
					"enable_recaptcha": true,
					"cors_allowed_origins": null,
					"base_url": null,
					"sdp_ui_base_url": null,
					"status": "TENANT_CREATED",
					"created_at": %q,
					"updated_at": %q
				}
			]
		`, tnt1.ID, tnt1.Name, tnt1.CreatedAt.Format(time.RFC3339Nano), tnt1.UpdatedAt.Format(time.RFC3339Nano), tnt2.ID, tnt2.Name, tnt2.CreatedAt.Format(time.RFC3339Nano), tnt2.UpdatedAt.Format(time.RFC3339Nano))
		assert.JSONEq(t, string(expectedRespBody), string(respBody))
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
				"sep10_signing_public_key": null,
				"distribution_public_key": null,
				"enable_mfa": true,
				"enable_recaptcha": true,
				"cors_allowed_origins": null,
				"base_url": null,
				"sdp_ui_base_url": null,
				"status": "TENANT_CREATED",
				"created_at": %q,
				"updated_at": %q
			}
		`, tnt1.ID, tnt1.Name, tnt1.CreatedAt.Format(time.RFC3339Nano), tnt1.UpdatedAt.Format(time.RFC3339Nano))
		assert.JSONEq(t, string(expectedRespBody), string(respBody))
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
				"sep10_signing_public_key": null,
				"distribution_public_key": null,
				"enable_mfa": true,
				"enable_recaptcha": true,
				"cors_allowed_origins": null,
				"base_url": null,
				"sdp_ui_base_url": null,
				"status": "TENANT_CREATED",
				"created_at": %q,
				"updated_at": %q
			}
		`, tnt2.ID, tnt2.Name, tnt2.CreatedAt.Format(time.RFC3339Nano), tnt2.UpdatedAt.Format(time.RFC3339Nano))
		assert.JSONEq(t, string(expectedRespBody), string(respBody))
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
		assert.JSONEq(t, string(expectedRespBody), string(respBody))
	})
}

func Test_TenantHandler_Post(t *testing.T) {
	dbt := dbtest.OpenWithTenantMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	messengerClientMock := message.MessengerClientMock{}
	m := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
	p := provisioning.NewManager(
		provisioning.WithDatabase(dbConnectionPool),
		provisioning.WithTenantManager(m),
		provisioning.WithMessengerClient(&messengerClientMock),
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
		http.HandlerFunc(handler.PostTenants).ServeHTTP(rr, req)

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
					"cors_allowed_origins": "provide at least one CORS allowed origins",
					"sdp_ui_base_url": "invalid SDP UI base URL value",
					"sep10_signing_public_key": "invalid public key",
					"distribution_public_key": "invalid public key"
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

		pubKey := keypair.MustRandom().Address()
		reqBody := fmt.Sprintf(`
			{
				"name": "aid-org",
				"owner_email": "owner@email.org",
				"owner_first_name": "Owner",
				"owner_last_name": "Owner",
				"organization_name": "My Aid Org",
				"email_sender_type": "DRY_RUN",
				"sms_sender_type": "DRY_RUN",
				"enable_recaptcha": true,
				"enable_mfa": false,
				"cors_allowed_origins": ["*"],
				"base_url": "https://backend.sdp.org",
				"sdp_ui_base_url": "https://aid-org.sdp.org",
				"sep10_signing_public_key": %q,
				"distribution_public_key": %q
			}
		`, pubKey, pubKey)

		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/tenants", strings.NewReader(reqBody))
		require.NoError(t, err)
		http.HandlerFunc(handler.PostTenants).ServeHTTP(rr, req)

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
				"sep10_signing_public_key": %q,
				"distribution_public_key": %q,
				"enable_mfa": false,
				"enable_recaptcha": true,
				"cors_allowed_origins": ["*"],
				"base_url": "https://backend.sdp.org",
				"sdp_ui_base_url": "https://aid-org.sdp.org",
				"status": "TENANT_PROVISIONED",
				"created_at": %q,
				"updated_at": %q
			}
		`, tnt.ID, pubKey, pubKey, tnt.CreatedAt.Format(time.RFC3339Nano), tnt.UpdatedAt.Format(time.RFC3339Nano))
		assert.JSONEq(t, string(expectedRespBody), string(respBody))

		// Validating infrastructure
		expectedSchema := "sdp_aid-org"
		expectedTablesAfterMigrationsApplied := []string{
			"assets",
			"auth_migrations",
			"auth_user_mfa_codes",
			"auth_user_password_reset",
			"auth_users",
			"channel_accounts",
			"countries",
			"disbursements",
			"gorp_migrations",
			"messages",
			"organizations",
			"payments",
			"receiver_verifications",
			"receiver_wallets",
			"receivers",
			"submitter_transactions",
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

	messengerClientMock.AssertExpectations(t)
}
