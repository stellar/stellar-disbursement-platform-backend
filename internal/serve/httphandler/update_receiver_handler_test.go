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
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

func Test_UpdateReceiverHandler_createVerificationInsert(t *testing.T) {
	receiverID := "mock_id"

	verificationDOB := data.ReceiverVerificationInsert{
		ReceiverID:        receiverID,
		VerificationField: data.VerificationTypeDateOfBirth,
		VerificationValue: "1999-01-01",
	}

	verificationYearMonth := data.ReceiverVerificationInsert{
		ReceiverID:        receiverID,
		VerificationField: data.VerificationTypeYearMonth,
		VerificationValue: "1999-01",
	}

	verificationPIN := data.ReceiverVerificationInsert{
		ReceiverID:        receiverID,
		VerificationField: data.VerificationTypePin,
		VerificationValue: "123",
	}

	verificationNationalID := data.ReceiverVerificationInsert{
		ReceiverID:        receiverID,
		VerificationField: data.VerificationTypeNationalID,
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

func Test_UpdateReceiverHandler_400(t *testing.T) {
	dbConnectionPool := getConnectionPool(t)

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()
	ctx = sdpcontext.SetUserIDInContext(ctx, "my-user-id")

	user := &auth.User{
		ID:    "my-user-id",
		Email: "email@email.com",
	}

	// setup
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, nil)
	authManager := &auth.AuthManagerMock{}
	authManager.On("GetUserByID", mock.Anything, "my-token").Return(user, nil)
	handler := &UpdateReceiverHandler{
		AuthManager:      authManager,
		Models:           models,
		DBConnectionPool: dbConnectionPool,
	}
	r := chi.NewRouter()
	r.Patch("/receivers/{id}", handler.UpdateReceiver)

	testCases := []struct {
		name         string
		request      validators.UpdateReceiverRequest
		expectedBody string
	}{
		{
			name:    "empty request body",
			request: validators.UpdateReceiverRequest{},
			expectedBody: `
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
			expectedBody: `
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
			expectedBody: `
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
			expectedBody: `
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
			expectedBody: `
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
			request: validators.UpdateReceiverRequest{NationalID: fmt.Sprintf("%0*d", utils.VerificationFieldMaxIDLength+1, 0)},
			expectedBody: `
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
			expectedBody: `
				{
					"error": "request invalid",
					"extras": {
					  "email": "invalid email format"
					}
				  }
				`,
		},
		{
			name:    "invalid phone number",
			request: validators.UpdateReceiverRequest{PhoneNumber: "invalid"},
			expectedBody: `
				{
					"error": "request invalid",
					"extras": {
					  "phone_number": "invalid phone number format"
					}
				  }
				`,
		},
		{
			name:    "invalid external ID",
			request: validators.UpdateReceiverRequest{ExternalID: "       "},
			expectedBody: `
				{
					"error": "request invalid",
					"extras": {
					  "external_id": "external_id cannot be set to empty"
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
			req, err := http.NewRequestWithContext(ctx, "PATCH", route, strings.NewReader(string(reqBody)))
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			resp := rr.Result()
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			assert.JSONEq(t, tc.expectedBody, string(respBody))
		})
	}
}

func Test_UpdateReceiverHandler_404(t *testing.T) {
	dbConnectionPool := getConnectionPool(t)

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()
	ctx = sdpcontext.SetUserIDInContext(ctx, "my-user-id")

	// setup
	authManager := &auth.AuthManagerMock{}
	handler := &UpdateReceiverHandler{
		AuthManager:      authManager,
		Models:           models,
		DBConnectionPool: dbConnectionPool,
	}

	// setup
	r := chi.NewRouter()
	r.Patch("/receivers/{id}", handler.UpdateReceiver)

	request := validators.UpdateReceiverRequest{DateOfBirth: "1999-01-01"}

	route := fmt.Sprintf("/receivers/%s", "invalid_receiver_id")
	reqBody, err := json.Marshal(request)
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(ctx, "PATCH", route, strings.NewReader(string(reqBody)))
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	resp := rr.Result()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func Test_UpdateReceiverHandler_409(t *testing.T) {
	dbConnectionPool := getConnectionPool(t)

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()
	ctx = sdpcontext.SetUserIDInContext(ctx, "my-user-id")

	// setup
	authManager := &auth.AuthManagerMock{}
	authManager.On("GetUserByID", mock.Anything, "my-token").Return("my-user-id", nil)
	handler := &UpdateReceiverHandler{
		AuthManager:      authManager,
		Models:           models,
		DBConnectionPool: dbConnectionPool,
	}

	// setup
	r := chi.NewRouter()
	r.Patch("/receivers/{id}", handler.UpdateReceiver)

	receiverStatic := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
		PhoneNumber: "+14155556666",
	})
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, nil)

	testCases := []struct {
		fieldName    string
		request      validators.UpdateReceiverRequest
		expectedBody string
	}{
		{
			fieldName: "email conflict",
			request: validators.UpdateReceiverRequest{
				Email: receiverStatic.Email,
			},
			expectedBody: `{
				"error": "The provided email is already associated with another user.",
				"extras": {
					"email": "email must be unique"
				}
			}`,
		},
		{
			fieldName: "phone_number",
			request: validators.UpdateReceiverRequest{
				PhoneNumber: receiverStatic.PhoneNumber,
			},
			expectedBody: `{
				"error": "The provided phone number is already associated with another user.",
				"extras": {
					"phone_number": "phone number must be unique"
				}
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.fieldName, func(t *testing.T) {
			route := fmt.Sprintf("/receivers/%s", receiver.ID)
			reqBody, err := json.Marshal(tc.request)
			require.NoError(t, err)
			req, err := http.NewRequestWithContext(ctx, http.MethodPatch, route, strings.NewReader(string(reqBody)))
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			resp := rr.Result()
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusConflict, resp.StatusCode)
			assert.JSONEq(t, tc.expectedBody, string(respBody))
		})
	}
}

func Test_UpdateReceiverHandler_200ok_updateReceiverFields(t *testing.T) {
	dbConnectionPool := getConnectionPool(t)

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()
	ctx = sdpcontext.SetUserIDInContext(ctx, "my-user-id")

	// setup
	authManager := &auth.AuthManagerMock{}
	authManager.On("GetUserByID", mock.Anything, "my-user-id").Return("my-user-id", nil)
	handler := &UpdateReceiverHandler{
		AuthManager:      authManager,
		Models:           models,
		DBConnectionPool: dbConnectionPool,
	}

	// setup
	r := chi.NewRouter()
	r.Patch("/receivers/{id}", handler.UpdateReceiver)

	testCases := []struct {
		fieldName string
		request   validators.UpdateReceiverRequest
		assertFn  func(t *testing.T, receiver *data.Receiver)
	}{
		{
			fieldName: "email",
			request: validators.UpdateReceiverRequest{
				Email: "update_receiver@email.com",
			},
			assertFn: func(t *testing.T, receiver *data.Receiver) {
				assert.Equal(t, "update_receiver@email.com", receiver.Email)
			},
		},
		{
			fieldName: "phone_number",
			request: validators.UpdateReceiverRequest{
				PhoneNumber: "+14155556666",
			},
			assertFn: func(t *testing.T, receiver *data.Receiver) {
				assert.Equal(t, "+14155556666", receiver.PhoneNumber)
			},
		},
		{
			fieldName: "external_id",
			request: validators.UpdateReceiverRequest{
				ExternalID: "newExternalID",
			},
			assertFn: func(t *testing.T, receiver *data.Receiver) {
				assert.Equal(t, "newExternalID", receiver.ExternalID)
			},
		},
		{
			fieldName: "ALL FIELDS",
			request: validators.UpdateReceiverRequest{
				Email:       "update_receiver@email.com",
				PhoneNumber: "+14155556666",
				ExternalID:  "newExternalID",
			},
			assertFn: func(t *testing.T, receiver *data.Receiver) {
				assert.Equal(t, "update_receiver@email.com", receiver.Email)
				assert.Equal(t, "+14155556666", receiver.PhoneNumber)
				assert.Equal(t, "newExternalID", receiver.ExternalID)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.fieldName, func(t *testing.T) {
			defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
			defer data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)

			receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, nil)
			data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
				ReceiverID:        receiver.ID,
				VerificationField: data.VerificationTypeDateOfBirth,
				VerificationValue: "2000-01-01",
			})
			rvSlice, err := models.ReceiverVerification.GetAllByReceiverID(ctx, dbConnectionPool, receiver.ID)
			require.NoError(t, err)
			require.Len(t, rvSlice, 1)
			rv := rvSlice[0]
			assert.Empty(t, rv.ConfirmedAt)
			assert.Empty(t, rv.ConfirmedByID)
			assert.Empty(t, rv.ConfirmedByType)

			route := fmt.Sprintf("/receivers/%s", receiver.ID)
			reqBody, err := json.Marshal(tc.request)
			require.NoError(t, err)
			req, err := http.NewRequestWithContext(ctx, http.MethodPatch, route, strings.NewReader(string(reqBody)))
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			resp := rr.Result()
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			receiverDB, err := models.Receiver.Get(ctx, dbConnectionPool, receiver.ID)
			require.NoError(t, err)

			tc.assertFn(t, receiverDB)
		})
	}
}

// upsertAction is a helper type to define the action to be taken by the handler when upserting the receiver verification.
type upsertAction string

const (
	actionUpdate upsertAction = "UPDATE"
	actionInsert upsertAction = "INSERT"
)

// shouldPreInsert is a helper function to determine if the receiver verification should be inserted before the request is
// made, so we test if the handler is updating the verification value. Otherwise, the receiver verification will be inserted
// as a consequence of the request.
func (ua upsertAction) shouldPreInsert() bool {
	return ua == actionUpdate
}

func Test_UpdateReceiverHandler_200ok_upsertVerificationFields(t *testing.T) {
	dbConnectionPool := getConnectionPool(t)

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()
	ctx = sdpcontext.SetUserIDInContext(ctx, "my-user-id")

	// setup
	authManager := &auth.AuthManagerMock{}
	authManager.On("GetUserID", mock.Anything, "my-user-id").Return("my-user-id", nil)
	handler := &UpdateReceiverHandler{
		AuthManager:      authManager,
		Models:           models,
		DBConnectionPool: dbConnectionPool,
	}
	r := chi.NewRouter()
	r.Patch("/receivers/{id}", handler.UpdateReceiver)

	assertVerificationFieldsContains := func(t *testing.T, rvList []data.ReceiverVerification, vt data.VerificationType, verifValue string) {
		var rv data.ReceiverVerification
		for _, _rv := range rvList {
			if _rv.VerificationField == vt {
				rv = _rv
				break
			}
		}
		require.NotEmptyf(t, rv, "receiver verification of type %s not found", vt)

		assert.Equal(t, vt, rv.VerificationField)
		assert.True(t, data.CompareVerificationValue(rv.HashedValue, verifValue), "hashed value does not match")
	}

	testCases := []struct {
		fieldName string
		request   validators.UpdateReceiverRequest
		assertFn  func(t *testing.T, rvList []data.ReceiverVerification)
	}{
		{
			fieldName: "date_of_birth",
			request: validators.UpdateReceiverRequest{
				DateOfBirth: "2000-01-01",
			},
			assertFn: func(t *testing.T, rvList []data.ReceiverVerification) {
				assertVerificationFieldsContains(t, rvList, data.VerificationTypeDateOfBirth, "2000-01-01")
			},
		},
		{
			fieldName: "year_month",
			request: validators.UpdateReceiverRequest{
				YearMonth: "2000-01",
			},
			assertFn: func(t *testing.T, rvList []data.ReceiverVerification) {
				assertVerificationFieldsContains(t, rvList, data.VerificationTypeYearMonth, "2000-01")
			},
		},
		{
			fieldName: "pin",
			request: validators.UpdateReceiverRequest{
				Pin: "123456",
			},
			assertFn: func(t *testing.T, rvList []data.ReceiverVerification) {
				assertVerificationFieldsContains(t, rvList, data.VerificationTypePin, "123456")
			},
		},
		{
			fieldName: "national_id",
			request: validators.UpdateReceiverRequest{
				NationalID: "abcd1234",
			},
			assertFn: func(t *testing.T, rvList []data.ReceiverVerification) {
				assertVerificationFieldsContains(t, rvList, data.VerificationTypeNationalID, "abcd1234")
			},
		},
		{
			fieldName: "ALL FIELDS",
			request: validators.UpdateReceiverRequest{
				DateOfBirth: "2000-01-01",
				YearMonth:   "2000-01",
				Pin:         "123456",
				NationalID:  "abcd1234",
			},
			assertFn: func(t *testing.T, rvList []data.ReceiverVerification) {
				assertVerificationFieldsContains(t, rvList, data.VerificationTypeDateOfBirth, "2000-01-01")
				assertVerificationFieldsContains(t, rvList, data.VerificationTypeYearMonth, "2000-01")
				assertVerificationFieldsContains(t, rvList, data.VerificationTypePin, "123456")
				assertVerificationFieldsContains(t, rvList, data.VerificationTypeNationalID, "abcd1234")
			},
		},
	}

	for _, action := range []upsertAction{actionUpdate, actionInsert} {
		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%s", action, tc.fieldName), func(t *testing.T) {
				defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
				defer data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)

				receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, nil)

				if action.shouldPreInsert() {
					data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
						ReceiverID:        receiver.ID,
						VerificationField: data.VerificationTypeDateOfBirth,
						VerificationValue: "1999-01-01",
					})
					data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
						ReceiverID:        receiver.ID,
						VerificationField: data.VerificationTypeYearMonth,
						VerificationValue: "1999-01",
					})
					data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
						ReceiverID:        receiver.ID,
						VerificationField: data.VerificationTypePin,
						VerificationValue: "000000",
					})
					data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
						ReceiverID:        receiver.ID,
						VerificationField: data.VerificationTypeNationalID,
						VerificationValue: "aaaa0000",
					})
				}

				route := fmt.Sprintf("/receivers/%s", receiver.ID)
				reqBody, err := json.Marshal(tc.request)
				require.NoError(t, err)
				req, err := http.NewRequestWithContext(ctx, http.MethodPatch, route, strings.NewReader(string(reqBody)))
				require.NoError(t, err)

				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req)

				resp := rr.Result()
				assert.Equal(t, http.StatusOK, resp.StatusCode)

				rvSlice, err := models.ReceiverVerification.GetAllByReceiverID(ctx, dbConnectionPool, receiver.ID)
				require.NoError(t, err)

				tc.assertFn(t, rvSlice)
			})
		}
	}
}

func getConnectionPool(t *testing.T) db.DBConnectionPool {
	t.Helper()
	dbt := dbtest.Open(t)
	t.Cleanup(func() { dbt.Close() })

	pool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })
	return pool
}
