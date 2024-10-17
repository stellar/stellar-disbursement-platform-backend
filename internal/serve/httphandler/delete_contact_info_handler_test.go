package httphandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

func Test_DeleteContactInfoHandler(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	receiverPhoneNumber := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: "+14152222222"})

	t.Run("return 404 if network passphrase is not testnet", func(t *testing.T) {
		h := DeleteContactInfoHandler{NetworkPassphrase: network.PublicNetworkPassphrase, Models: models}
		r := chi.NewRouter()
		r.Delete("/wallet-registration/contact-info/{contact_info}", h.ServeHTTP)

		// test
		req, err := http.NewRequest("DELETE", "/wallet-registration/contact-info/"+receiverPhoneNumber.PhoneNumber, nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusNotFound, rr.Code)
		wantJson := `{"error": "Resource not found."}`
		assert.JSONEq(t, wantJson, rr.Body.String())
	})

	t.Run("return 400 if network passphrase is testnet but phone number is invalid", func(t *testing.T) {
		h := DeleteContactInfoHandler{NetworkPassphrase: network.TestNetworkPassphrase, Models: models}
		r := chi.NewRouter()
		r.Delete("/wallet-registration/contact-info/{contact_info}", h.ServeHTTP)

		// test
		req, err := http.NewRequest("DELETE", "/wallet-registration/contact-info/foobar", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		wantJson := `{
			"error": "The request was invalid in some way.",
			"extras": {
				"contact_info": "not a valid phone number or email"
			}
		}`
		assert.JSONEq(t, wantJson, rr.Body.String())
	})

	t.Run("return 404 if network passphrase is testnet but phone number does not exist", func(t *testing.T) {
		h := DeleteContactInfoHandler{NetworkPassphrase: network.TestNetworkPassphrase, Models: models}
		r := chi.NewRouter()
		r.Delete("/wallet-registration/contact-info/{contact_info}", h.ServeHTTP)

		// test
		req, err := http.NewRequest("DELETE", "/wallet-registration/contact-info/+14153333333", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusNotFound, rr.Code)
		wantJson := `{"error":"Resource not found."}`
		assert.JSONEq(t, wantJson, rr.Body.String())
	})

	t.Run("return 204 if network passphrase is testnet and phone number exists", func(t *testing.T) {
		h := DeleteContactInfoHandler{NetworkPassphrase: network.TestNetworkPassphrase, Models: models}
		r := chi.NewRouter()
		r.Delete("/wallet-registration/contact-info/{contact_info}", h.ServeHTTP)

		// test
		req, err := http.NewRequest("DELETE", "/wallet-registration/contact-info/"+receiverPhoneNumber.PhoneNumber, nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusNoContent, rr.Code)
		wantJson := "null"
		assert.JSONEq(t, wantJson, rr.Body.String())
	})
}
