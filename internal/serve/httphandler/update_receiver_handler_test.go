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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func Test_UpdateReceiverHandler_createVerificationInsert(t *testing.T) {
	receiverID := "mock_id"

	verificationDOB := data.ReceiverVerificationInsert{
		ReceiverID:        receiverID,
		VerificationField: data.VerificationFieldDateOfBirth,
		VerificationValue: "1999-01-01",
	}

	verificationYearMonth := data.ReceiverVerificationInsert{
		ReceiverID:        receiverID,
		VerificationField: data.VerificationFieldYearMonth,
		VerificationValue: "1999-01",
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
			name:                  "insert receiver verification year month",
			updateReceiverRequest: validators.UpdateReceiverRequest{YearMonth: "1999-01"},
			want:                  []data.ReceiverVerificationInsert{verificationYearMonth},
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
				name:    "invalid year/month",
				request: validators.UpdateReceiverRequest{YearMonth: "invalid"},
				want: `
				{
					"error": "request invalid",
					"extras": {
					  "year_month": "invalid year/month format. Correct format: 1990-12"
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
					  "pin": "invalid pin length. Cannot have less than 4 or more than 8 characters in pin"
					}
				  }
				`,
			},
			{
				name:    "invalid national ID - empty",
				request: validators.UpdateReceiverRequest{NationalID: "   "},
				want: `
				{
					"error": "request invalid",
					"extras": {
					  "national_id": "national id cannot be empty"
					}
				  }
				`,
			},
			{
				name:    "invalid national ID - too long",
				request: validators.UpdateReceiverRequest{NationalID: fmt.Sprintf("%0*d", utils.VerificationFieldMaxIdLength+1, 0)},
				want: `
				{
					"error": "request invalid",
					"extras": {
					  "national_id": "invalid national id. Cannot have more than 50 characters in national id"
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

	t.Run("receiver not found", func(t *testing.T) {
		request := validators.UpdateReceiverRequest{DateOfBirth: "1999-01-01"}

		route := fmt.Sprintf("/receivers/%s", "invalid_receiver_id")
		reqBody, err := json.Marshal(request)
		require.NoError(t, err)
		req, err := http.NewRequest("PATCH", route, strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
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

	t.Run("update year/month value", func(t *testing.T) {
		data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldYearMonth,
			VerificationValue: "2000-01",
		})

		request := validators.UpdateReceiverRequest{YearMonth: "1999-01"}

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
		err = dbConnectionPool.GetContext(ctx, &newReceiverVerification, query, receiver.ID, data.VerificationFieldYearMonth)
		require.NoError(t, err)

		assert.True(t, data.CompareVerificationValue(newReceiverVerification.HashedValue, "1999-01"))
		assert.False(t, data.CompareVerificationValue(newReceiverVerification.HashedValue, "2000-01"))

		receiverDB, err := models.Receiver.Get(ctx, dbConnectionPool, receiver.ID)
		require.NoError(t, err)
		assert.Equal(t, "receiver@email.com", *receiverDB.Email)
		assert.Equal(t, "externalID", receiverDB.ExternalID)
	})

	t.Run("update pin value", func(t *testing.T) {
		data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldPin,
			VerificationValue: "8901",
		})

		request := validators.UpdateReceiverRequest{Pin: "1234"}

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

		assert.True(t, data.CompareVerificationValue(newReceiverVerification.HashedValue, "1234"))
		assert.False(t, data.CompareVerificationValue(newReceiverVerification.HashedValue, "8901"))

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
			VerificationField: data.VerificationFieldYearMonth,
			VerificationValue: "2000-01",
		})

		data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldPin,
			VerificationValue: "8901",
		})

		data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldNationalID,
			VerificationValue: "OLDID890",
		})

		request := validators.UpdateReceiverRequest{
			DateOfBirth: "1999-01-01",
			YearMonth:   "1999-01",
			Pin:         "1234",
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
				verificationField:    data.VerificationFieldYearMonth,
				newVerificationValue: "1999-01",
				oldVerificationValue: "2000-01",
			},
			{
				verificationField:    data.VerificationFieldPin,
				newVerificationValue: "1234",
				oldVerificationValue: "8901",
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

	t.Run("updates and inserts receiver verifications values", func(t *testing.T) {
		data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)

		request := validators.UpdateReceiverRequest{
			DateOfBirth: "1999-01-01",
			YearMonth:   "1999-01",
			Pin:         "1234",
			NationalID:  "NEWID123",
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
				verificationField:    data.VerificationFieldYearMonth,
				newVerificationValue: "1999-01",
				oldVerificationValue: "",
			},
			{
				verificationField:    data.VerificationFieldPin,
				newVerificationValue: "1234",
				oldVerificationValue: "",
			},
			{
				verificationField:    data.VerificationFieldNationalID,
				newVerificationValue: "NEWID123",
				oldVerificationValue: "",
			},
		}
		for _, v := range receiverVerifications {
			newReceiverVerification := data.ReceiverVerification{}
			err = dbConnectionPool.GetContext(ctx, &newReceiverVerification, query, receiver.ID, v.verificationField)
			require.NoError(t, err)
			t.Logf("newReceiverVerification: %+v", newReceiverVerification)

			assert.True(t, data.CompareVerificationValue(newReceiverVerification.HashedValue, v.newVerificationValue))

			if v.oldVerificationValue != "" {
				assert.False(t, data.CompareVerificationValue(newReceiverVerification.HashedValue, v.oldVerificationValue))
			}

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
