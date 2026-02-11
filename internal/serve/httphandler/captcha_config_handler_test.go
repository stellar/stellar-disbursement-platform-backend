package httphandler

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_CAPTCHAConfigHandler_ServeHTTP(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("returns 400 when organization_name is missing", func(t *testing.T) {
		mTenantManager := tenant.NewTenantManagerMock(t)
		handler := CAPTCHAConfigHandler{
			TenantManager:     mTenantManager,
			Models:            models,
			CAPTCHAType:       validators.GoogleReCAPTCHAV2,
			ReCAPTCHADisabled: false,
		}

		r := chi.NewRouter()
		r.Get("/organization/captcha-config", handler.ServeHTTP)

		rr := testutils.Request(t, ctx, r, "/organization/captcha-config", http.MethodGet, nil)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"organization_name query parameter is required"}`, rr.Body.String())
	})

	t.Run("returns 404 when organization is not found", func(t *testing.T) {
		mTenantManager := tenant.NewTenantManagerMock(t)
		mTenantManager.
			On("GetTenantByName", mock.Anything, "unknown-org").
			Return(nil, fmt.Errorf("tenant not found")).
			Once()

		handler := CAPTCHAConfigHandler{
			TenantManager:     mTenantManager,
			Models:            models,
			CAPTCHAType:       validators.GoogleReCAPTCHAV2,
			ReCAPTCHADisabled: false,
		}

		r := chi.NewRouter()
		r.Get("/organization/captcha-config", handler.ServeHTTP)

		rr := testutils.Request(t, ctx, r, "/organization/captcha-config?organization_name=unknown-org", http.MethodGet, nil)
		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.JSONEq(t, `{"error":"organization not found"}`, rr.Body.String())
	})

	t.Run("returns correct captcha config when captcha is enabled", func(t *testing.T) {
		tnt := &schema.Tenant{ID: "tenant-id-1", Name: "test-org"}
		mTenantManager := tenant.NewTenantManagerMock(t)
		mTenantManager.
			On("GetTenantByName", mock.Anything, "test-org").
			Return(tnt, nil).
			Once()

		handler := CAPTCHAConfigHandler{
			TenantManager:     mTenantManager,
			Models:            models,
			CAPTCHAType:       validators.GoogleReCAPTCHAV2,
			ReCAPTCHADisabled: false,
		}

		r := chi.NewRouter()
		r.Get("/organization/captcha-config", handler.ServeHTTP)

		rr := testutils.Request(t, ctx, r, "/organization/captcha-config?organization_name=test-org", http.MethodGet, nil)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.JSONEq(t, `{
			"captcha_type": "GOOGLE_RECAPTCHA_V2",
			"captcha_disabled": false
		}`, rr.Body.String())
	})

	t.Run("returns captcha disabled when env-level DisableReCAPTCHA is true", func(t *testing.T) {
		tnt := &schema.Tenant{ID: "tenant-id-1", Name: "test-org"}
		mTenantManager := tenant.NewTenantManagerMock(t)
		mTenantManager.
			On("GetTenantByName", mock.Anything, "test-org").
			Return(tnt, nil).
			Once()

		handler := CAPTCHAConfigHandler{
			TenantManager:     mTenantManager,
			Models:            models,
			CAPTCHAType:       validators.GoogleReCAPTCHAV3,
			ReCAPTCHADisabled: true,
		}

		r := chi.NewRouter()
		r.Get("/organization/captcha-config", handler.ServeHTTP)

		rr := testutils.Request(t, ctx, r, "/organization/captcha-config?organization_name=test-org", http.MethodGet, nil)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.JSONEq(t, `{
			"captcha_type": "GOOGLE_RECAPTCHA_V3",
			"captcha_disabled": true
		}`, rr.Body.String())
	})

	t.Run("org-level CAPTCHADisabled=false overrides env-level disabled", func(t *testing.T) {
		tnt := &schema.Tenant{ID: "tenant-id-1", Name: "test-org"}
		mTenantManager := tenant.NewTenantManagerMock(t)
		mTenantManager.
			On("GetTenantByName", mock.Anything, "test-org").
			Return(tnt, nil).
			Once()

		// Set org-level CAPTCHADisabled to false
		captchaEnabled := false
		err := models.Organizations.Update(ctx, &data.OrganizationUpdate{
			CAPTCHADisabled: &captchaEnabled,
		})
		require.NoError(t, err)

		handler := CAPTCHAConfigHandler{
			TenantManager:     mTenantManager,
			Models:            models,
			CAPTCHAType:       validators.GoogleReCAPTCHAV2,
			ReCAPTCHADisabled: true,
		}

		r := chi.NewRouter()
		r.Get("/organization/captcha-config", handler.ServeHTTP)

		rr := testutils.Request(t, ctx, r, "/organization/captcha-config?organization_name=test-org", http.MethodGet, nil)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.JSONEq(t, `{
			"captcha_type": "GOOGLE_RECAPTCHA_V2",
			"captcha_disabled": false
		}`, rr.Body.String())
	})
}
