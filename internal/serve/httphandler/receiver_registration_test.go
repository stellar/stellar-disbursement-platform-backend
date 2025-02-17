package httphandler

import (
	"context"
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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
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
	r.Get("/receiver-registration/start", ReceiverRegistrationHandler{Models: models, ReceiverWalletModel: receiverWalletModel, ReCAPTCHASiteKey: reCAPTCHASiteKey}.ServeHTTP)

	t.Run("returns 401 - Unauthorized if the token is not in the request context", func(t *testing.T) {
		req, reqErr := http.NewRequest("GET", "/receiver-registration/start", nil)
		require.NoError(t, reqErr)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, respErr := io.ReadAll(resp.Body)
		require.NoError(t, respErr)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	t.Run("returns 401 - Unauthorized if the token is in the request context but it's not valid", func(t *testing.T) {
		req, reqErr := http.NewRequest("GET", "/receiver-registration/start", nil)
		require.NoError(t, reqErr)

		rr := httptest.NewRecorder()
		invalidClaims := &anchorplatform.SEP24JWTClaims{}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, invalidClaims))
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, respErr := io.ReadAll(resp.Body)
		require.NoError(t, respErr)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	ctx := context.Background()
	link := "http://www.test.com/privacy-policy"
	err = models.Organizations.Update(ctx, &data.OrganizationUpdate{
		PrivacyPolicyLink: &link,
	})
	require.NoError(t, err)

	_, err = models.Organizations.Get(ctx)
	require.NoError(t, err)

	t.Run("returns 200 - Ok (And show the Wallet Registration page) if the token is in the request context and it's valid 🎉", func(t *testing.T) {
		req, reqErr := http.NewRequest("GET", "/receiver-registration/start?token=test-token", nil)
		require.NoError(t, reqErr)

		validClaims := &anchorplatform.SEP24JWTClaims{
			ClientDomainClaim: "test.com",
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			},
		}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, respErr := io.ReadAll(resp.Body)
		require.NoError(t, respErr)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))
		assert.Contains(t, string(respBody), "<title>Wallet Registration</title>")
		assert.Contains(t, string(respBody), `<span id="recaptcha-site-key" data-sitekey="reCAPTCHASiteKey" style="display: none"/>`)
		assert.Contains(t, string(respBody), `<link rel="preload" href="https://www.google.com/recaptcha/api.js" as="script" />`)
		assert.Contains(t, string(respBody), `<p>Your data is processed by MyCustomAid in accordance with their <a href="http://www.test.com/privacy-policy"><b>Privacy Policy</b></a></p>`)
	})

	// Create a receiver wallet
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool,
		"My Wallet",
		"https://mywallet.com",
		"mywallet.com",
		"mywallet://")
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.DraftReceiversWalletStatus)
	err = receiverWalletModel.Update(ctx, receiverWallet.ID, data.ReceiverWalletUpdate{
		StellarAddress: "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
	}, dbConnectionPool)
	require.NoError(t, err)

	t.Run("returns 200 - Ok (And show the Registration Success page) if the token is in the request context and it's valid and the user was already registered 🎉", func(t *testing.T) {
		req, reqErr := http.NewRequest("GET", "/receiver-registration/start?token=test-token", nil)
		require.NoError(t, reqErr)

		validClaims := &anchorplatform.SEP24JWTClaims{
			ClientDomainClaim: "mywallet.com",
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			},
		}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, respErr := io.ReadAll(resp.Body)
		require.NoError(t, respErr)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))
		assert.Contains(t, string(respBody), "<title>Wallet Registration Confirmation</title>")
		assert.Contains(t, string(respBody), `<p>Your data is processed by MyCustomAid in accordance with their <a href="http://www.test.com/privacy-policy"><b>Privacy Policy</b></a></p>`)
	})

	t.Run("returns 200 - Ok (And show the Wallet Registration page) if the token is in the request context and wants to register second wallet in the same address", func(t *testing.T) {
		req, reqErr := http.NewRequest("GET", "/receiver-registration/start?token=test-token", nil)
		require.NoError(t, reqErr)

		validClaims := &anchorplatform.SEP24JWTClaims{
			ClientDomainClaim: "newwallet.com",
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			},
		}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, respErr := io.ReadAll(resp.Body)
		require.NoError(t, respErr)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))
		assert.Contains(t, string(respBody), "<title>Wallet Registration</title>")
		assert.Contains(t, string(respBody), `<span id="recaptcha-site-key" data-sitekey="reCAPTCHASiteKey" style="display: none"/>`)
		assert.Contains(t, string(respBody), `<link rel="preload" href="https://www.google.com/recaptcha/api.js" as="script" />`)
		assert.Contains(t, string(respBody), `<p>Your data is processed by MyCustomAid in accordance with their <a href="http://www.test.com/privacy-policy"><b>Privacy Policy</b></a></p>`)
	})
}
