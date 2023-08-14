package httphandler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_UpdateReceiverHandler_createVerificationInsert(t *testing.T) {
	receiverID := "mock_id"

	verificationDOB := data.ReceiverVerificationInsert{
		ReceiverID:        receiverID,
		VerificationField: data.VerificationFieldDateOfBirth,
		VerificationValue: "1999-01-01",
	}

	verificationPIN := data.ReceiverVerificationInsert{
		ReceiverID:        receiverID,
		VerificationField: data.VerificationFieldPin,
		VerificationValue: "123",
	}

	verificationNationalID := data.ReceiverVerificationInsert{
		ReceiverID:        receiverID,
		VerificationField: data.VerificationFieldNationalID,
		VerificationValue: "12345CODE",
	}

	testCases := []struct {
		name                  string
		updateReceiverRequest validators.UpdateReceiverRequest
		want                  []data.ReceiverVerificationInsert
	}{
		{
			name:                  "empty update request",
			updateReceiverRequest: validators.UpdateReceiverRequest{},
			want:                  []data.ReceiverVerificationInsert{},
		},
		{
			name:                  "insert receiver verification date of birth",
			updateReceiverRequest: validators.UpdateReceiverRequest{DateOfBirth: "1999-01-01"},
			want:                  []data.ReceiverVerificationInsert{verificationDOB},
		},
		{
			name:                  "insert receiver verification pin",
			updateReceiverRequest: validators.UpdateReceiverRequest{Pin: "123"},
			want:                  []data.ReceiverVerificationInsert{verificationPIN},
		},
		{
			name:                  "insert receiver verification national ID",
			updateReceiverRequest: validators.UpdateReceiverRequest{NationalID: "12345CODE"},
			want:                  []data.ReceiverVerificationInsert{verificationNationalID},
		},
		{
			name: "insert multipes receiver verification values",
			updateReceiverRequest: validators.UpdateReceiverRequest{
				DateOfBirth: "1999-01-01",
				Pin:         "123",
				NationalID:  "12345CODE",
			},
			want: []data.ReceiverVerificationInsert{verificationDOB, verificationPIN, verificationNationalID},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			updateReceiverRequest := tc.updateReceiverRequest
			receiverVerifications := createVerificationInsert(&updateReceiverRequest, receiverID)

			assert.Equal(t, tc.want, receiverVerifications)
		})
	}
}

func Test_UpdateReceiverHandler(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	handler := &UpdateReceiverHandler{
		Models:           models,
		DBConnectionPool: dbConnectionPool,
	}

	ctx := context.Background()
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
		PhoneNumber: "+380445555555",
		Email:       &[]string{"receiver@email.com"}[0],
		ExternalID:  "externalID",
	})

	// setup
	r := chi.NewRouter()
	r.Patch("/receivers/{id}", handler.UpdateReceiver)

	t.Run("error invalid request body", func(t *testing.T) {
		testCases := []struct {
			name    string
			request validators.UpdateReceiverRequest
			want    string
		}{
			{
				name:    "empty request body",
				request: validators.UpdateReceiverRequest{},
				want: `
				{
					"error": "request invalid",
					"extras": {
					  "body": "request body is empty"
					}
				  }
				`,
			},
			{
				name:    "invalid date of birth",
				request: validators.UpdateReceiverRequest{DateOfBirth: "invalid"},
				want: `
				{
					"error": "request invalid",
					"extras": {
					  "date_of_birth": "invalid date of birth format. Correct format: 1990-01-30"
					}
				  }
				`,
			},
			{
				name:    "invalid pin",
				request: validators.UpdateReceiverRequest{Pin: "    "},
				want: `
				{
					"error": "request invalid",
					"extras": {
					  "pin": "invalid pin format"
					}
				  }
				`,
			},
			{
				name:    "invalid national ID",
				request: validators.UpdateReceiverRequest{NationalID: "   "},
				want: `
				{
					"error": "request invalid",
					"extras": {
					  "national_id": "invalid national ID format"
					}
				  }
				`,
			},
			{
				name:    "invalid email",
				request: validators.UpdateReceiverRequest{Email: "invalid"},
				want: `
				{
					"error": "request invalid",
					"extras": {
					  "email": "invalid email format"
					}
				  }
				`,
			},
			{
				name:    "invalid external ID",
				request: validators.UpdateReceiverRequest{ExternalID: "       "},
				want: `
				{
					"error": "request invalid",
					"extras": {
					  "external_id": "invalid external_id format"
					}
				  }
				`,
			},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				route := fmt.Sprintf("/receivers/%s", receiver.ID)
				reqBody, err := json.Marshal(tc.request)
				require.NoError(t, err)
				req, err := http.NewRequest("PATCH", route, strings.NewReader(string(reqBody)))
				require.NoError(t, err)

				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req)

				resp := rr.Result()
				respBody, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
				assert.JSONEq(t, tc.want, string(respBody))
			})
		}
	})

	t.Run("update date of birth value", func(t *testing.T) {
		data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldDateOfBirth,
			VerificationValue: "2000-01-01",
		})

		request := validators.UpdateReceiverRequest{DateOfBirth: "1999-01-01"}

		route := fmt.Sprintf("/receivers/%s", receiver.ID)
		reqBody, err := json.Marshal(request)
		require.NoError(t, err)
		req, err := http.NewRequest("PATCH", route, strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		query := `
			SELECT 
				hashed_value
			FROM 
				receiver_verifications
			WHERE 
				receiver_id = $1 AND 
				verification_field = $2
		`

		newReceiverVerification := data.ReceiverVerification{}
		err = dbConnectionPool.GetContext(ctx, &newReceiverVerification, query, receiver.ID, data.VerificationFieldDateOfBirth)
		require.NoError(t, err)

		assert.True(t, data.CompareVerificationValue(newReceiverVerification.HashedValue, "1999-01-01"))
		assert.False(t, data.CompareVerificationValue(newReceiverVerification.HashedValue, "2000-01-01"))

		receiverDB, err := models.Receiver.Get(ctx, dbConnectionPool, receiver.ID)
		require.NoError(t, err)
		assert.Equal(t, "receiver@email.com", *receiverDB.Email)
		assert.Equal(t, "externalID", receiverDB.ExternalID)
	})

	t.Run("update pin value", func(t *testing.T) {
		data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldPin,
			VerificationValue: "890",
		})

		request := validators.UpdateReceiverRequest{Pin: "123"}

		route := fmt.Sprintf("/receivers/%s", receiver.ID)
		reqBody, err := json.Marshal(request)
		require.NoError(t, err)
		req, err := http.NewRequest("PATCH", route, strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		query := `
			SELECT 
				hashed_value
			FROM 
				receiver_verifications
			WHERE 
				receiver_id = $1 AND 
				verification_field = $2
		`

		newReceiverVerification := data.ReceiverVerification{}
		err = dbConnectionPool.GetContext(ctx, &newReceiverVerification, query, receiver.ID, data.VerificationFieldPin)
		require.NoError(t, err)

		assert.True(t, data.CompareVerificationValue(newReceiverVerification.HashedValue, "123"))
		assert.False(t, data.CompareVerificationValue(newReceiverVerification.HashedValue, "890"))

		receiverDB, err := models.Receiver.Get(ctx, dbConnectionPool, receiver.ID)
		require.NoError(t, err)
		assert.Equal(t, "receiver@email.com", *receiverDB.Email)
		assert.Equal(t, "externalID", receiverDB.ExternalID)
	})

	t.Run("update national ID value", func(t *testing.T) {
		data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldNationalID,
			VerificationValue: "OLDID890",
		})

		request := validators.UpdateReceiverRequest{NationalID: "NEWID123"}

		route := fmt.Sprintf("/receivers/%s", receiver.ID)
		reqBody, err := json.Marshal(request)
		require.NoError(t, err)
		req, err := http.NewRequest("PATCH", route, strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		query := `
			SELECT 
				hashed_value
			FROM 
				receiver_verifications
			WHERE 
				receiver_id = $1 AND 
				verification_field = $2
		`

		newReceiverVerification := data.ReceiverVerification{}
		err = dbConnectionPool.GetContext(ctx, &newReceiverVerification, query, receiver.ID, data.VerificationFieldNationalID)
		require.NoError(t, err)

		assert.True(t, data.CompareVerificationValue(newReceiverVerification.HashedValue, "NEWID123"))
		assert.False(t, data.CompareVerificationValue(newReceiverVerification.HashedValue, "OLDID890"))

		receiverDB, err := models.Receiver.Get(ctx, dbConnectionPool, receiver.ID)
		require.NoError(t, err)
		assert.Equal(t, "receiver@email.com", *receiverDB.Email)
		assert.Equal(t, "externalID", receiverDB.ExternalID)
	})

	t.Run("update multiples receiver verifications values", func(t *testing.T) {
		data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)

		data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldDateOfBirth,
			VerificationValue: "2000-01-01",
		})

		data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldPin,
			VerificationValue: "890",
		})

		data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldNationalID,
			VerificationValue: "OLDID890",
		})

		request := validators.UpdateReceiverRequest{
			DateOfBirth: "1999-01-01",
			Pin:         "123",
			NationalID:  "NEWID123",
		}

		route := fmt.Sprintf("/receivers/%s", receiver.ID)
		reqBody, err := json.Marshal(request)
		require.NoError(t, err)
		req, err := http.NewRequest("PATCH", route, strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		query := `
			SELECT
				hashed_value
			FROM
				receiver_verifications
			WHERE
				receiver_id = $1 AND
				verification_field = $2
		`

		receiverVerifications := []struct {
			verificationField    data.VerificationField
			newVerificationValue string
			oldVerificationValue string
		}{
			{
				verificationField:    data.VerificationFieldDateOfBirth,
				newVerificationValue: "1999-01-01",
				oldVerificationValue: "2000-01-01",
			},
			{
				verificationField:    data.VerificationFieldPin,
				newVerificationValue: "123",
				oldVerificationValue: "890",
			},
			{
				verificationField:    data.VerificationFieldNationalID,
				newVerificationValue: "NEWID123",
				oldVerificationValue: "OLDID890",
			},
		}
		for _, v := range receiverVerifications {
			newReceiverVerification := data.ReceiverVerification{}
			err = dbConnectionPool.GetContext(ctx, &newReceiverVerification, query, receiver.ID, v.verificationField)
			require.NoError(t, err)

			assert.True(t, data.CompareVerificationValue(newReceiverVerification.HashedValue, v.newVerificationValue))
			assert.False(t, data.CompareVerificationValue(newReceiverVerification.HashedValue, v.oldVerificationValue))

			receiverDB, err := models.Receiver.Get(ctx, dbConnectionPool, receiver.ID)
			require.NoError(t, err)
			assert.Equal(t, "receiver@email.com", *receiverDB.Email)
			assert.Equal(t, "externalID", receiverDB.ExternalID)
		}
	})

	t.Run("updates receiver's email", func(t *testing.T) {
		request := validators.UpdateReceiverRequest{
			Email: "update_receiver@email.com",
		}

		route := fmt.Sprintf("/receivers/%s", receiver.ID)
		reqBody, err := json.Marshal(request)
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPatch, route, strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		receiverDB, err := models.Receiver.Get(ctx, dbConnectionPool, receiver.ID)
		require.NoError(t, err)
		assert.Equal(t, "update_receiver@email.com", *receiverDB.Email)
	})

	t.Run("updates receiver's external ID", func(t *testing.T) {
		request := validators.UpdateReceiverRequest{
			ExternalID: "newExternalID",
		}

		route := fmt.Sprintf("/receivers/%s", receiver.ID)
		reqBody, err := json.Marshal(request)
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPatch, route, strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		receiverDB, err := models.Receiver.Get(ctx, dbConnectionPool, receiver.ID)
		require.NoError(t, err)

		assert.Equal(t, "newExternalID", receiverDB.ExternalID)
	})
}
