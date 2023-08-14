package httphandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/network"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_DeletePhoneNumberHandler(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: "+14152222222"})

	t.Run("return 404 if network passphrase is not testnet", func(t *testing.T) {
		h := DeletePhoneNumberHandler{NetworkPassphrase: network.PublicNetworkPassphrase, Models: models}
		r := chi.NewRouter()
		r.Delete("/wallet-registration/phone-number/{phone_number}", h.ServeHTTP)

		// test
		req, err := http.NewRequest("DELETE", "/wallet-registration/phone-number/"+receiver.PhoneNumber, nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusNotFound, rr.Code)
		wantJson := `{"error": "Resource not found."}`
		assert.JSONEq(t, wantJson, rr.Body.String())
	})

	t.Run("return 400 if network passphrase is testnet but phone number is invalid", func(t *testing.T) {
		h := DeletePhoneNumberHandler{NetworkPassphrase: network.TestNetworkPassphrase, Models: models}
		r := chi.NewRouter()
		r.Delete("/wallet-registration/phone-number/{phone_number}", h.ServeHTTP)

		// test
		req, err := http.NewRequest("DELETE", "/wallet-registration/phone-number/foobar", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		wantJson := `{
			"error": "The request was invalid in some way.",
			"extras": {
				"phone_number": "invalid phone number"
			}
		}`
		assert.JSONEq(t, wantJson, rr.Body.String())
	})

	t.Run("return 404 if network passphrase is testnet but phone number does not exist", func(t *testing.T) {
		h := DeletePhoneNumberHandler{NetworkPassphrase: network.TestNetworkPassphrase, Models: models}
		r := chi.NewRouter()
		r.Delete("/wallet-registration/phone-number/{phone_number}", h.ServeHTTP)

		// test
		req, err := http.NewRequest("DELETE", "/wallet-registration/phone-number/+14153333333", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusNotFound, rr.Code)
		wantJson := `{"error":"Resource not found."}`
		assert.JSONEq(t, wantJson, rr.Body.String())
	})

	t.Run("return 204 if network passphrase is testnet and phone nymber exists", func(t *testing.T) {
		h := DeletePhoneNumberHandler{NetworkPassphrase: network.TestNetworkPassphrase, Models: models}
		r := chi.NewRouter()
		r.Delete("/wallet-registration/phone-number/{phone_number}", h.ServeHTTP)

		// test
		req, err := http.NewRequest("DELETE", "/wallet-registration/phone-number/"+receiver.PhoneNumber, nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusNoContent, rr.Code)
		wantJson := "null"
		assert.JSONEq(t, wantJson, rr.Body.String())
	})
}
