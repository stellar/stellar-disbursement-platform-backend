package httphandler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sepauth"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_ReceiverRegistrationHandler_ServeHTTP(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	receiverWalletModel := models.ReceiverWallet
	reCAPTCHASiteKey := "reCAPTCHASiteKey"

	r := chi.NewRouter()
	r.Get("/sep24-interactive-deposit/info", ReceiverRegistrationHandler{
		Models:              models,
		ReceiverWalletModel: receiverWalletModel,
		ReCAPTCHASiteKey:    reCAPTCHASiteKey,
		ReCAPTCHADisabled:   true,
		CAPTCHAType:         validators.GoogleReCAPTCHAV2,
	}.ServeHTTP)

	t.Run("returns 401 - Unauthorized if the token is not in the request context", func(t *testing.T) {
		req, reqErr := http.NewRequest("GET", "/sep24-interactive-deposit/info", nil)
		require.NoError(t, reqErr)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, respErr := io.ReadAll(resp.Body)
		require.NoError(t, respErr)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized.", "error_code":"401_0"}`, string(respBody))
	})

	t.Run("returns 401 - Unauthorized if the token is in the request context but it's not valid", func(t *testing.T) {
		req, reqErr := http.NewRequest("GET", "/sep24-interactive-deposit/info", nil)
		require.NoError(t, reqErr)

		rr := httptest.NewRecorder()
		invalidClaims := &sepauth.SEP24JWTClaims{}
		req = req.WithContext(context.WithValue(req.Context(), sepauth.SEP24ClaimsContextKey, invalidClaims))
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, respErr := io.ReadAll(resp.Body)
		require.NoError(t, respErr)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized.", "error_code":"401_0"}`, string(respBody))
	})

	ctx := context.Background()
	link := "http://www.stellar.org/privacy-policy"
	err = models.Organizations.Update(ctx, &data.OrganizationUpdate{
		PrivacyPolicyLink: &link,
	})
	require.NoError(t, err)

	_, err = models.Organizations.Get(ctx)
	require.NoError(t, err)

	validClaims := &sepauth.SEP24JWTClaims{
		ClientDomainClaim: "stellar.org",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "test-transaction-id",
			Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		},
	}
	currentTenant, ctx := tenant.LoadDefaultTenantInContext(t, dbConnectionPool)
	ctxWithClaims := context.WithValue(ctx, sepauth.SEP24ClaimsContextKey, validClaims)

	t.Run("returns 200 - Ok with JSON response for unregistered user", func(t *testing.T) {
		req, reqErr := http.NewRequestWithContext(ctxWithClaims, "GET", "/sep24-interactive-deposit/info", nil)
		require.NoError(t, reqErr)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, respErr := io.ReadAll(resp.Body)
		require.NoError(t, respErr)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))

		expectedJSON := fmt.Sprintf(`{
			"privacy_policy_link": "http://www.stellar.org/privacy-policy",
			"organization_name": "MyCustomAid",
			"organization_logo": "%s/organization/logo",
			"is_registered": false,
			"is_recaptcha_disabled": true,
			"recaptcha_site_key": "reCAPTCHASiteKey",
			"captcha_type": "GOOGLE_RECAPTCHA_V2"
		}`, *currentTenant.BaseURL)
		assert.JSONEq(t, expectedJSON, string(respBody))
	})

	// Create a receiver wallet
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool,
		"My Wallet",
		"https://mywallet.com",
		validClaims.ClientDomain(),
		"mywallet://")
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

	// Set OTPConfirmedWith field
	otpConfirmedWith := "user@example.com"
	err = receiverWalletModel.Update(ctx, receiverWallet.ID, data.ReceiverWalletUpdate{
		StellarAddress:   validClaims.Account(),
		OTPConfirmedWith: otpConfirmedWith,
		OTPConfirmedAt:   time.Now(),
	}, dbConnectionPool)
	require.NoError(t, err)

	t.Run("returns 200 - Ok with JSON response for registered user", func(t *testing.T) {
		req, reqErr := http.NewRequestWithContext(ctxWithClaims, "GET", "/sep24-interactive-deposit/info", nil)
		require.NoError(t, reqErr)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, respErr := io.ReadAll(resp.Body)
		require.NoError(t, respErr)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))

		truncatedContactInfo := utils.TruncateString(otpConfirmedWith, 3)
		expectedJSON := fmt.Sprintf(`{
			"privacy_policy_link": "http://www.stellar.org/privacy-policy",
			"organization_name": "MyCustomAid",
			"organization_logo": "%s/organization/logo",
			"truncated_contact_info": "`+truncatedContactInfo+`",
			"is_registered": true,
			"is_recaptcha_disabled": true,
			"recaptcha_site_key": "reCAPTCHASiteKey",
			"captcha_type": "GOOGLE_RECAPTCHA_V2"
		}`, *currentTenant.BaseURL)
		assert.JSONEq(t, expectedJSON, string(respBody))
	})

	t.Run("returns 200 - Ok with JSON response for unregistered user with same address but different wallet", func(t *testing.T) {
		otherWalletClaims := &sepauth.SEP24JWTClaims{
			ClientDomainClaim: "other.stellar.org",
			RegisteredClaims:  validClaims.RegisteredClaims,
		}
		ctxWithOtherWalletClaims := context.WithValue(ctx, sepauth.SEP24ClaimsContextKey, otherWalletClaims)
		req, reqErr := http.NewRequestWithContext(ctxWithOtherWalletClaims, "GET", "/sep24-interactive-deposit/info", nil)
		require.NoError(t, reqErr)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, respErr := io.ReadAll(resp.Body)
		require.NoError(t, respErr)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))

		expectedJSON := fmt.Sprintf(`{
			"privacy_policy_link": "http://www.stellar.org/privacy-policy",
			"organization_name": "MyCustomAid",
			"organization_logo": "%s/organization/logo",
			"is_registered": false,
			"is_recaptcha_disabled": true,
			"recaptcha_site_key": "reCAPTCHASiteKey",
			"captcha_type": "GOOGLE_RECAPTCHA_V2"
		}`, *currentTenant.BaseURL)
		assert.JSONEq(t, expectedJSON, string(respBody))
	})
}
