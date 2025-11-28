package httphandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go-stellar-sdk/network"
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

	for _, contactType := range data.GetAllReceiverContactTypes() {
		t.Run(string(contactType), func(t *testing.T) {
			testCases := []struct {
				name              string
				networkPassphrase string
				getContactinfoFn  func(t *testing.T, receiver *data.Receiver) string
				wantStatusCode    int
				wantBody          string
			}{
				{
					name:              "🔴 return 404 if network passphrase is not testnet",
					networkPassphrase: network.PublicNetworkPassphrase,
					getContactinfoFn:  func(t *testing.T, receiver *data.Receiver) string { return receiver.ContactByType(contactType) },
					wantStatusCode:    http.StatusNotFound,
					wantBody:          `{"error": "Resource not found."}`,
				},
				{
					name:              "🔴 return 400 if the contact info is invalid",
					networkPassphrase: network.TestNetworkPassphrase,
					getContactinfoFn:  func(t *testing.T, receiver *data.Receiver) string { return "foobar" },
					wantStatusCode:    http.StatusBadRequest,
					wantBody: `{
						"error": "The request was invalid in some way.",
						"extras": {
							"contact_info": "not a valid phone number or email"
						}
					}`,
				},
				{
					name:              "🔴 return 404 if the contact info does not exist",
					networkPassphrase: network.TestNetworkPassphrase,
					getContactinfoFn: func(t *testing.T, receiver *data.Receiver) string {
						switch contactType {
						case data.ReceiverContactTypeEmail:
							return "foobar@test.com"
						case data.ReceiverContactTypeSMS:
							return "+14153333333"
						}
						t.Errorf("Unsupported contact type %s", contactType)
						panic("Unsupported contact type " + contactType)
					},
					wantStatusCode: http.StatusNotFound,
					wantBody:       `{"error":"Resource not found."}`,
				},
				{
					name:              "🟢 return 204 if the contact info exists",
					networkPassphrase: network.TestNetworkPassphrase,
					getContactinfoFn:  func(t *testing.T, receiver *data.Receiver) string { return receiver.ContactByType(contactType) },
					wantStatusCode:    http.StatusNoContent,
					wantBody:          "null",
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})

					h := DeleteContactInfoHandler{NetworkPassphrase: tc.networkPassphrase, Models: models}
					r := chi.NewRouter()
					r.Delete("/wallet-registration/contact-info/{contact_info}", h.ServeHTTP)

					// test
					req, err := http.NewRequest("DELETE", "/wallet-registration/contact-info/"+tc.getContactinfoFn(t, receiver), nil)
					require.NoError(t, err)
					rr := httptest.NewRecorder()
					r.ServeHTTP(rr, req)

					// assert response
					assert.Equal(t, tc.wantStatusCode, rr.Code)
					assert.JSONEq(t, tc.wantBody, rr.Body.String())
				})
			}
		})
	}
}
