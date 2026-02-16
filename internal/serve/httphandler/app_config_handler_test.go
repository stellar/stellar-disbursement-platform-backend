package httphandler

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_AppConfigHandler_ServeHTTP(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	tnt := &schema.Tenant{ID: "tenant-id-1", Name: "test-org"}
	ctx := sdpcontext.SetTenantInContext(t.Context(), tnt)

	t.Run("returns correct captcha config when env-level DisableReCAPTCHA is true", func(t *testing.T) {
		handler := AppConfigHandler{
			Models:            models,
			CAPTCHAType:       validators.GoogleReCAPTCHAV2,
			ReCAPTCHASiteKey:  "test-site-key",
			ReCAPTCHADisabled: false,
		}

		r := chi.NewRouter()
		r.Get("/app-config", handler.ServeHTTP)

		rr := testutils.Request(t, ctx, r, "/app-config", http.MethodGet, nil)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.JSONEq(t, `{
			"captcha_type": "GOOGLE_RECAPTCHA_V2",
			"captcha_site_key": "test-site-key",
			"captcha_disabled": false
		}`, rr.Body.String())
	})

	t.Run("returns captcha disabled when env-level DisableReCAPTCHA is true", func(t *testing.T) {
		handler := AppConfigHandler{
			Models:            models,
			CAPTCHAType:       validators.GoogleReCAPTCHAV3,
			ReCAPTCHASiteKey:  "test-site-key",
			ReCAPTCHADisabled: true,
		}

		r := chi.NewRouter()
		r.Get("/app-config", handler.ServeHTTP)

		rr := testutils.Request(t, ctx, r, "/app-config", http.MethodGet, nil)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.JSONEq(t, `{
			"captcha_type": "GOOGLE_RECAPTCHA_V3",
			"captcha_site_key": "test-site-key",
			"captcha_disabled": true
		}`, rr.Body.String())
	})

	t.Run("org-level CAPTCHADisabled=false overrides env-level DisableReCAPTCHA=true", func(t *testing.T) {
		// Set org-level CAPTCHADisabled to false
		captchaEnabled := false
		err := models.Organizations.Update(ctx, &data.OrganizationUpdate{
			CAPTCHADisabled: &captchaEnabled,
		})
		require.NoError(t, err)

		handler := AppConfigHandler{
			Models:            models,
			CAPTCHAType:       validators.GoogleReCAPTCHAV2,
			ReCAPTCHASiteKey:  "test-site-key",
			ReCAPTCHADisabled: true,
		}

		r := chi.NewRouter()
		r.Get("/app-config", handler.ServeHTTP)

		rr := testutils.Request(t, ctx, r, "/app-config", http.MethodGet, nil)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.JSONEq(t, `{
			"captcha_type": "GOOGLE_RECAPTCHA_V2",
			"captcha_site_key": "test-site-key",
			"captcha_disabled": false
		}`, rr.Body.String())
	})
}
