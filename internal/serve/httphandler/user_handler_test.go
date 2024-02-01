package httphandler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	urllib "net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/htmltemplate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_UserHandler_UserActivation(t *testing.T) {
	type testMocks struct {
		AuthenticatorMock *auth.AuthenticatorMock
		JWTManagerMock    *auth.JWTManagerMock
		Handler           UserHandler
	}

	setupMocks := func(t *testing.T) *testMocks {
		t.Helper()
		authenticatorMock := &auth.AuthenticatorMock{}
		jwtManagerMock := &auth.JWTManagerMock{}

		authManager := auth.NewAuthManager(
			auth.WithCustomAuthenticatorOption(authenticatorMock),
			auth.WithCustomJWTManagerOption(jwtManagerMock),
		)

		return &testMocks{
			AuthenticatorMock: authenticatorMock,
			JWTManagerMock:    jwtManagerMock,
			Handler:           UserHandler{AuthManager: authManager},
		}
	}

	executePatchRequest := func(t *testing.T, handler UserHandler, token string, body string) *http.Response {
		t.Helper()
		const url = "/users/activation"
		r := chi.NewRouter()
		r.Patch(url, handler.UserActivation)

		ctx := context.Background()
		if token != "" {
			ctx = context.WithValue(ctx, middleware.TokenContextKey, token)
		}

		var bodyReader io.Reader
		if body != "" {
			bodyReader = strings.NewReader(body)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bodyReader)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Result()
	}

	t.Run("returns Unauthorized when no token is in the request context", func(t *testing.T) {
		mocks := setupMocks(t)
		resp := executePatchRequest(t, mocks.Handler, "", "")
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	t.Run("returns error when request body is invalid", func(t *testing.T) {
		// is_active and user_id are required
		mocks := setupMocks(t)
		resp := executePatchRequest(t, mocks.Handler, "mytoken", "{}")
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody := `{
			"error": "Request invalid",
			"extras": {
				"user_id": "user_id is required",
				"is_active": "is_active is required"
			}
		}`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))

		// is_active is required
		resp = executePatchRequest(t, mocks.Handler, "mytoken", `{"user_id": "user-id"}`)
		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody = `{
			"error": "Request invalid",
			"extras": {
				"is_active": "is_active is required"
			}
		}`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))

		// user_id is required
		resp = executePatchRequest(t, mocks.Handler, "mytoken", `{"is_active": true}`)
		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody = `{
			"error": "Request invalid",
			"extras": {
				"user_id": "user_id is required"
			}
		}`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))

		// invalid body format:
		getLogEntries := log.DefaultLogger.StartTest(log.InfoLevel)
		resp = executePatchRequest(t, mocks.Handler, "mytoken", `"invalid"`)
		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody = `{"error": "The request was invalid in some way."}`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
		assert.Contains(t, getLogEntries()[0].Message, "decoding the request body")
	})

	t.Run("returns Unauthorized when token is invalid", func(t *testing.T) {
		mocks := setupMocks(t)
		token := "mytoken"

		mocks.JWTManagerMock.
			On("ValidateToken", mock.Anything, token).
			Return(false, nil).
			Twice()
		defer mocks.JWTManagerMock.AssertExpectations(t)

		// Activating the user
		reqBody := `{
			"user_id": "user-id",
			"is_active": true
		}`
		resp := executePatchRequest(t, mocks.Handler, token, reqBody)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))

		// Deactivating the user
		reqBody = `{
			"user_id": "user-id",
			"is_active": false
		}`
		resp = executePatchRequest(t, mocks.Handler, token, reqBody)
		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	t.Run("returns BadRequest when trying to update self activation state", func(t *testing.T) {
		mocks := setupMocks(t)
		token := "mytoken"

		mocks.JWTManagerMock.
			On("ValidateToken", mock.Anything, token).
			Return(true, nil).
			Twice().
			On("GetUserFromToken", mock.Anything, token).
			Return(&auth.User{ID: "user-id"}, nil).
			Twice()
		defer mocks.JWTManagerMock.AssertExpectations(t)

		// Activating the user
		reqBody := `{
			"user_id": "user-id",
			"is_active": true
		}`
		resp := executePatchRequest(t, mocks.Handler, token, reqBody)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "The request was invalid in some way.", "extras": {"user_id":"cannot update your own activation"}}`, string(respBody))

		// Deactivating the user
		reqBody = `{
			"user_id": "user-id",
			"is_active": false
		}`
		resp = executePatchRequest(t, mocks.Handler, token, reqBody)
		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "The request was invalid in some way.", "extras": {"user_id":"cannot update your own activation"}}`, string(respBody))
	})

	t.Run("returns BadRequest when user doesn't exist", func(t *testing.T) {
		mocks := setupMocks(t)
		token := "mytoken"

		mocks.JWTManagerMock.
			On("ValidateToken", mock.Anything, token).
			Return(true, nil).
			Times(4).
			On("GetUserFromToken", mock.Anything, token).
			Return(&auth.User{}, nil).
			Twice()
		defer mocks.JWTManagerMock.AssertExpectations(t)

		mocks.AuthenticatorMock.
			On("ActivateUser", mock.Anything, "user-id").
			Return(auth.ErrNoRowsAffected).
			Once().
			On("DeactivateUser", mock.Anything, "user-id").
			Return(auth.ErrNoRowsAffected).
			Once()
		defer mocks.AuthenticatorMock.AssertExpectations(t)

		// Activating the user
		reqBody := `{
			"user_id": "user-id",
			"is_active": true
		}`
		resp := executePatchRequest(t, mocks.Handler, token, reqBody)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "The request was invalid in some way.", "extras": {"user_id":"user_id is invalid"}}`, string(respBody))

		// Deactivating the user
		reqBody = `{
			"user_id": "user-id",
			"is_active": false
		}`
		resp = executePatchRequest(t, mocks.Handler, token, reqBody)
		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "The request was invalid in some way.", "extras": {"user_id":"user_id is invalid"}}`, string(respBody))
	})

	t.Run("returns InternalServerError when a unexpected error occurs", func(t *testing.T) {
		mocks := setupMocks(t)
		token := "mytoken"

		mocks.JWTManagerMock.
			On("ValidateToken", mock.Anything, token).
			Return(false, errors.New("unexpected error")).
			Once()
		defer mocks.JWTManagerMock.AssertExpectations(t)

		getTestEntries := log.DefaultLogger.StartTest(log.InfoLevel)
		reqBody := `{
			"user_id": "user-id",
			"is_active": true
		}`
		resp := executePatchRequest(t, mocks.Handler, token, reqBody)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, `{"error": "An internal error occurred while processing this request."}`, string(respBody))
		assert.Contains(t, getTestEntries()[0].Message, "getting user from token: validating token: validating token: unexpected error")
	})

	t.Run("updates the user activation correctly", func(t *testing.T) {
		mocks := setupMocks(t)
		token := "mytoken"

		mocks.JWTManagerMock.
			On("ValidateToken", mock.Anything, token).
			Return(true, nil).
			Times(4).
			On("GetUserFromToken", mock.Anything, token).
			Return(&auth.User{ID: "authenticated-user-id"}, nil).
			Twice()
		defer mocks.JWTManagerMock.AssertExpectations(t)

		mocks.AuthenticatorMock.
			On("ActivateUser", mock.Anything, "user-id").
			Return(nil).
			Once().
			On("DeactivateUser", mock.Anything, "user-id").
			Return(nil).
			Once()
		defer mocks.AuthenticatorMock.AssertExpectations(t)

		getTestEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		// Activating the user
		reqBody := `{
			"user_id": "user-id",
			"is_active": true
		}`
		resp := executePatchRequest(t, mocks.Handler, token, reqBody)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "user activation was updated successfully"}`, string(respBody))

		// Deactivating the user
		reqBody = `{
			"user_id": "user-id",
			"is_active": false
		}`
		resp = executePatchRequest(t, mocks.Handler, token, reqBody)
		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "user activation was updated successfully"}`, string(respBody))

		// validate logs
		require.Contains(t, getTestEntries()[0].Message, "[ActivateUserAccount] - User ID authenticated-user-id activating user with account ID user-id")
	})
}

func Test_CreateUserRequest_validate(t *testing.T) {
	cur := CreateUserRequest{
		FirstName: "",
		LastName:  "",
		Email:     "",
		Roles:     []data.UserRole{},
	}

	extras := map[string]interface{}{
		"email":     "email is required",
		"fist_name": "fist_name is required",
		"last_name": "last_name is required",
		"roles":     "the number of roles required is exactly one",
	}
	expectedErr := httperror.BadRequest("Request invalid", nil, extras)

	err := cur.validate()
	assert.Equal(t, expectedErr, err)

	cur = CreateUserRequest{
		FirstName: "First",
		LastName:  "Last",
		Email:     "email@email.com",
		Roles:     []data.UserRole{data.BusinessUserRole, data.DeveloperUserRole},
	}

	extras = map[string]interface{}{
		"roles": "the number of roles required is exactly one",
	}
	expectedErr = httperror.BadRequest("Request invalid", nil, extras)

	err = cur.validate()
	assert.Equal(t, expectedErr, err)

	cur = CreateUserRequest{
		FirstName: "First",
		LastName:  "Last",
		Email:     "email@email.com",
		Roles:     []data.UserRole{data.DeveloperUserRole},
	}

	err = cur.validate()
	assert.Nil(t, err)
}

func Test_UserHandler_CreateUser(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	r := chi.NewRouter()

	authManagerMock := &auth.AuthManagerMock{}

	messengerClientMock := &message.MessengerClientMock{}
	uiBaseURL := "https://sdp.com"
	handler := &UserHandler{
		AuthManager:     authManagerMock,
		MessengerClient: messengerClientMock,
		UIBaseURL:       uiBaseURL,
		Models:          models,
	}

	const url = "/users"

	r.Post(url, handler.CreateUser)

	t.Run("returns Unauthorized when token is invalid", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(`{}`))
		require.NoError(t, err)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	t.Run("returns error when request body is invalid", func(t *testing.T) {
		token := "mytoken"
		ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(`{}`))
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody := `
			{
				"error": "Request invalid",
				"extras": {
					"email": "email is required",
					"fist_name": "fist_name is required",
					"last_name": "last_name is required",
					"roles": "the number of roles required is exactly one"
				}
			}
		`

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))

		body := `
			{
				"first_name": "First",
				"last_name": "Last",
				"email": "email@email.com",
				"roles": ["role1", "role2"]
			}
		`
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
		require.NoError(t, err)

		w = httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp = w.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody = `
			{
				"error": "Request invalid",
				"extras": {
					"roles": "the number of roles required is exactly one"
				}
			}
		`

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))

		body = `
			{
				"first_name": "First",
				"last_name": "Last",
				"email": "email@email.com",
				"roles": ["role1"]
			}
		`
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
		require.NoError(t, err)

		w = httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp = w.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody = `
			{
				"error": "Request invalid",
				"extras": {
					"roles": "unexpected value for roles[0]=role1. Expect one of these values: [owner financial_controller developer business]"
				}
			}
		`

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))

		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		req, err = http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(`"invalid"`))
		require.NoError(t, err)

		w = httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp = w.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody = `
			{
				"error": "The request was invalid in some way."
			}
		`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
		assert.Contains(t, buf.String(), "decoding the request body")
	})

	t.Run("returns error when Auth Manager fails", func(t *testing.T) {
		token := "mytoken"
		ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)

		authManagerMock.
			On("GetUserID", mock.Anything, token).
			Return("", errors.New("unexpected error")).
			Once()

		body := `
			{
				"first_name": "First",
				"last_name": "Last",
				"email": "email@email.com",
				"roles": ["developer"]
			}
		`

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody := `
			{
				"error": "Not authorized."
			}
		`

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))

		u := &auth.User{
			FirstName: "First",
			LastName:  "Last",
			Email:     "email@email.com",
			Roles:     []string{data.DeveloperUserRole.String()},
		}

		authManagerMock.
			On("GetUserID", mock.Anything, token).
			Return("authenticated-user-id", nil).
			Once().
			On("CreateUser", mock.Anything, u, "").
			Return(nil, errors.New("unexpected error")).
			Once()

		req, err = http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
		require.NoError(t, err)

		w = httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp = w.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody = `
			{
				"error": "Cannot create user"
			}
		`

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
	})

	t.Run("returns Bad Request when user is duplicated", func(t *testing.T) {
		token := "mytoken"
		ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)

		u := &auth.User{
			FirstName: "First",
			LastName:  "Last",
			Email:     "email@email.com",
			Roles:     []string{data.DeveloperUserRole.String()},
		}

		authManagerMock.
			On("GetUserID", mock.Anything, token).
			Return("authenticated-user-id", nil).
			Once().
			On("CreateUser", mock.Anything, u, "").
			Return(nil, auth.ErrUserEmailAlreadyExists).
			Once()

		body := `
			{
				"first_name": "First",
				"last_name": "Last",
				"email": "email@email.com",
				"roles": ["developer"]
			}
		`
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "a user with this email already exists"}`, string(respBody))
	})

	t.Run("returns error when sending email fails", func(t *testing.T) {
		token := "mytoken"
		ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)

		u := &auth.User{
			FirstName: "First",
			LastName:  "Last",
			Email:     "email@email.com",
			Roles:     []string{data.DeveloperUserRole.String()},
		}

		expectedUser := &auth.User{
			ID:        "user-id",
			FirstName: u.FirstName,
			LastName:  u.LastName,
			Email:     u.Email,
			Roles:     u.Roles,
		}

		authManagerMock.
			On("GetUserID", mock.Anything, token).
			Return("authenticated-user-id", nil).
			Once().
			On("CreateUser", mock.Anything, u, "").
			Return(expectedUser, nil).
			Once()

		forgotPasswordLink, err := urllib.JoinPath(uiBaseURL, "forgot-password")
		require.NoError(t, err)

		content, err := htmltemplate.ExecuteHTMLTemplateForInvitationMessage(htmltemplate.InvitationMessageTemplate{
			FirstName:          u.FirstName,
			Role:               u.Roles[0],
			ForgotPasswordLink: forgotPasswordLink,
			OrganizationName:   "MyCustomAid",
		})
		require.NoError(t, err)

		msg := message.Message{
			ToEmail: u.Email,
			Title:   invitationMessageTitle,
			Message: content,
		}
		messengerClientMock.
			On("SendMessage", msg).
			Return(errors.New("unexpected error")).
			Once()

		body := `
			{
				"first_name": "First",
				"last_name": "Last",
				"email": "email@email.com",
				"roles": ["developer"]
			}
		`
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody := `
			{
				"error": "Cannot send invitation email for user user-id"
			}
		`

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
	})

	t.Run("returns error when joining the forgot password link", func(t *testing.T) {
		token := "mytoken"
		ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)

		u := &auth.User{
			FirstName: "First",
			LastName:  "Last",
			Email:     "email@email.com",
			Roles:     []string{data.DeveloperUserRole.String()},
		}

		expectedUser := &auth.User{
			ID:        "user-id",
			FirstName: u.FirstName,
			LastName:  u.LastName,
			Email:     u.Email,
			Roles:     u.Roles,
		}

		authManagerMock.
			On("GetUserID", mock.Anything, token).
			Return("authenticated-user-id", nil).
			Once().
			On("CreateUser", mock.Anything, u, "").
			Return(expectedUser, nil).
			Once()

		body := `
			{
				"first_name": "First",
				"last_name": "Last",
				"email": "email@email.com",
				"roles": ["developer"]
			}
		`
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
		require.NoError(t, err)

		w := httptest.NewRecorder()

		http.HandlerFunc(UserHandler{
			AuthManager:     authManagerMock,
			MessengerClient: messengerClientMock,
			UIBaseURL:       "%invalid%",
			Models:          models,
		}.CreateUser).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody := `
			{
				"error": "Cannot get forgot password link"
			}
		`

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
	})

	t.Run("creates user successfully", func(t *testing.T) {
		token := "mytoken"
		ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)

		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.SetLevel(log.InfoLevel)

		u := &auth.User{
			FirstName: "First",
			LastName:  "Last",
			Email:     "email@email.com",
			Roles:     []string{data.DeveloperUserRole.String()},
		}

		expectedUser := &auth.User{
			ID:        "user-id",
			FirstName: u.FirstName,
			LastName:  u.LastName,
			Email:     u.Email,
			Roles:     u.Roles,
			IsActive:  true,
		}

		authManagerMock.
			On("GetUserID", mock.Anything, token).
			Return("authenticated-user-id", nil).
			Once().
			On("CreateUser", mock.Anything, u, "").
			Return(expectedUser, nil).
			Once()

		forgotPasswordLink, err := urllib.JoinPath(uiBaseURL, "forgot-password")
		require.NoError(t, err)

		content, err := htmltemplate.ExecuteHTMLTemplateForInvitationMessage(htmltemplate.InvitationMessageTemplate{
			FirstName:          u.FirstName,
			Role:               u.Roles[0],
			ForgotPasswordLink: forgotPasswordLink,
			OrganizationName:   "MyCustomAid",
		})
		require.NoError(t, err)

		msg := message.Message{
			ToEmail: u.Email,
			Title:   invitationMessageTitle,
			Message: content,
		}
		messengerClientMock.
			On("SendMessage", msg).
			Return(nil).
			Once()

		body := `
			{
				"first_name": "First",
				"last_name": "Last",
				"email": "email@email.com",
				"roles": ["developer"]
			}
		`
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody := `
			{
				"id": "user-id",
				"first_name": "First",
				"last_name": "Last",
				"email": "email@email.com",
				"is_active": true,
				"roles": ["developer"]
			}
		`

		assert.Equal(t, http.StatusCreated, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "[CreateUserAccount] - User ID authenticated-user-id created user with account ID user-id")
	})

	authManagerMock.AssertExpectations(t)
	messengerClientMock.AssertExpectations(t)
}

func Test_UpdateRolesRequest_validate(t *testing.T) {
	upr := UpdateRolesRequest{
		UserID: "",
		Roles:  []data.UserRole{},
	}

	extras := map[string]interface{}{
		"user_id": "user_id is required",
		"roles":   "the number of roles required is exactly one",
	}
	expectedErr := httperror.BadRequest("Request invalid", nil, extras)

	err := upr.validate()
	assert.Equal(t, expectedErr, err)

	upr = UpdateRolesRequest{
		UserID: "user_id",
		Roles:  []data.UserRole{data.BusinessUserRole, data.DeveloperUserRole},
	}

	extras = map[string]interface{}{
		"roles": "the number of roles required is exactly one",
	}
	expectedErr = httperror.BadRequest("Request invalid", nil, extras)

	err = upr.validate()
	assert.Equal(t, expectedErr, err)

	upr = UpdateRolesRequest{
		UserID: "user_id",
		Roles:  []data.UserRole{data.DeveloperUserRole},
	}

	err = upr.validate()
	assert.Nil(t, err)
}

func Test_UserHandler_UpdateUserRoles(t *testing.T) {
	r := chi.NewRouter()

	jwtManagerMock := &auth.JWTManagerMock{}
	roleManagerMock := &auth.RoleManagerMock{}
	authManager := auth.NewAuthManager(
		auth.WithCustomJWTManagerOption(jwtManagerMock),
		auth.WithCustomRoleManagerOption(roleManagerMock),
	)

	handler := &UserHandler{AuthManager: authManager}

	const url = "/users/roles"
	r.Patch(url, handler.UpdateUserRoles)

	t.Run("returns Unauthorized when no token is in the request context", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPatch, url, nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	t.Run("returns error when request body is invalid", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), middleware.TokenContextKey, "mytoken")

		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(`{}`))
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody := `
			{
				"error": "Request invalid",
				"extras": {
					"user_id": "user_id is required",
					"roles": "the number of roles required is exactly one"
				}
			}
		`

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))

		body := `
			{
				"user_id": "user-id",
				"roles": ["role1", "role2"]
			}
		`
		req, err = http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(body))
		require.NoError(t, err)

		w = httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp = w.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody = `
			{
				"error": "Request invalid",
				"extras": {
					"roles": "the number of roles required is exactly one"
				}
			}
		`

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))

		body = `
			{
				"user_id": "user-id",
				"roles": ["role1"]
			}
		`
		req, err = http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(body))
		require.NoError(t, err)

		w = httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp = w.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody = `
			{
				"error": "Request invalid",
				"extras": {
					"roles": "unexpected value for roles[0]=role1. Expect one of these values: [owner financial_controller developer business]"
				}
			}
		`

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))

		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		req, err = http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(`"invalid"`))
		require.NoError(t, err)

		w = httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp = w.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody = `
			{
				"error": "The request was invalid in some way."
			}
		`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
		assert.Contains(t, buf.String(), "decoding the request body")
	})

	t.Run("returns Unauthorized when token is invalid", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", mock.Anything, token).
			Return(false, nil).
			Once()

		ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)

		reqBody := `
			{	
				"user_id": "user-id",
				"roles": ["developer"]
			}
		`
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(reqBody))
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	t.Run("returns BadRequest when user doesn't exist", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", mock.Anything, token).
			Return(true, nil).
			Twice().
			On("GetUserFromToken", mock.Anything, token).
			Return(&auth.User{ID: "authenticated-user-id"}, nil).
			Once()

		roleManagerMock.
			On("UpdateRoles", mock.Anything, &auth.User{ID: "user-id"}, []string{data.DeveloperUserRole.String()}).
			Return(auth.ErrNoRowsAffected).
			Once()

		ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)

		reqBody := `
			{
				"user_id": "user-id",
				"roles": ["developer"]
			}
		`
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(reqBody))
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "The request was invalid in some way.", "extras": {"user_id":"user_id is invalid"}}`, string(respBody))
	})

	t.Run("returns InternalServerError when a unexpected error occurs", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", mock.Anything, token).
			Return(true, nil).
			Once().
			On("GetUserFromToken", mock.Anything, token).
			Return(&auth.User{ID: "authenticated-user-id"}, nil).
			Once().
			On("ValidateToken", mock.Anything, token).
			Return(false, errors.New("unexpected error")).
			Once()

		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)

		reqBody := `
			{
				"user_id": "user-id",
				"roles": ["developer"]
			}
		`
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(reqBody))
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Cannot update user activation"}`, string(respBody))
		assert.Contains(t, buf.String(), "Cannot update user activation")
	})

	t.Run("updates the user activation correctly", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", mock.Anything, token).
			Return(true, nil).
			Twice().
			On("GetUserFromToken", mock.Anything, token).
			Return(&auth.User{ID: "authenticated-user-id"}, nil).
			Once()

		roleManagerMock.
			On("UpdateRoles", mock.Anything, &auth.User{ID: "user-id"}, []string{data.DeveloperUserRole.String()}).
			Return(nil).
			Once()

		ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)

		reqBody := `
			{
				"user_id": "user-id",
				"roles": ["developer"]
			}
		`
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(reqBody))
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "user roles were updated successfully"}`, string(respBody))
	})
}

func Test_UserHandler_GetAllUsers(t *testing.T) {
	jwtManagerMock := &auth.JWTManagerMock{}
	authenticatorMock := &auth.AuthenticatorMock{}
	authManager := auth.NewAuthManager(
		auth.WithCustomJWTManagerOption(jwtManagerMock),
		auth.WithCustomAuthenticatorOption(authenticatorMock),
	)

	handler := &UserHandler{AuthManager: authManager}

	const url = "/users"

	t.Run("returns Unauthorized when no token is in the request context", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()

		http.HandlerFunc(handler.GetAllUsers).ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	t.Run("returns Unauthorized when token is invalid", func(t *testing.T) {
		token := "mytoken"

		ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		jwtManagerMock.
			On("ValidateToken", req.Context(), token).
			Return(false, nil).
			Once()

		w := httptest.NewRecorder()

		http.HandlerFunc(handler.GetAllUsers).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	t.Run("returns InternalServerError when a unexpected error occurs", func(t *testing.T) {
		token := "mytoken"

		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		jwtManagerMock.
			On("ValidateToken", req.Context(), token).
			Return(false, errors.New("unexpected error")).
			Once()

		w := httptest.NewRecorder()

		http.HandlerFunc(handler.GetAllUsers).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Cannot get all users"}`, string(respBody))
		assert.Contains(t, buf.String(), "Cannot get all users")
	})

	const orderByEmailAscURL = "/users?sort=email&direction=ASC"
	const orderByEmailDescURL = "/users?sort=email&direction=DESC"
	const orderByIsActiveAscURL = "/users?sort=is_active&direction=ASC"
	const orderByIsActiveDescURL = "/users?sort=is_active&direction=DESC"

	t.Run("returns all users ordered by email ASC", func(t *testing.T) {
		token := "mytoken"

		ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, orderByEmailAscURL, nil)
		require.NoError(t, err)

		jwtManagerMock.
			On("ValidateToken", req.Context(), token).
			Return(true, nil).
			Once()

		authenticatorMock.
			On("GetAllUsers", req.Context()).
			Return([]auth.User{
				{
					ID:        "user1-ID",
					FirstName: "First",
					LastName:  "Last",
					Email:     "userA@email.com",
					IsOwner:   false,
					IsActive:  false,
					Roles:     []string{data.BusinessUserRole.String()},
				},
				{
					ID:        "user2-ID",
					FirstName: "First",
					LastName:  "Last",
					Email:     "userC@email.com",
					IsOwner:   true,
					IsActive:  true,
					Roles:     []string{data.OwnerUserRole.String()},
				},
				{
					ID:        "user3-ID",
					FirstName: "First",
					LastName:  "Last",
					Email:     "userB@email.com",
					IsOwner:   true,
					IsActive:  true,
					Roles:     []string{data.OwnerUserRole.String()},
				},
			}, nil).
			Once()

		w := httptest.NewRecorder()

		http.HandlerFunc(handler.GetAllUsers).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody := `
			[
				{
					"id": "user1-ID",
					"first_name": "First",
					"last_name": "Last",
					"email": "userA@email.com",
					"is_active": false,
					"roles": [
						"business"
					]
				},
				{
					"id":        "user3-ID",
					"first_name": "First",
					"last_name":  "Last",
					"email":     "userB@email.com",
					"is_active":  true,
					"roles": [
						"owner"
					]
				},
				{
					"id": "user2-ID",
					"first_name": "First",
					"last_name": "Last",
					"email": "userC@email.com",
					"is_active": true,
					"roles": [
						"owner"
					]
				}
			]
		`

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
	})

	t.Run("returns all users ordered by email DESC", func(t *testing.T) {
		token := "mytoken"

		ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, orderByEmailDescURL, nil)
		require.NoError(t, err)

		jwtManagerMock.
			On("ValidateToken", req.Context(), token).
			Return(true, nil).
			Once()

		authenticatorMock.
			On("GetAllUsers", req.Context()).
			Return([]auth.User{
				{
					ID:        "user1-ID",
					FirstName: "First",
					LastName:  "Last",
					Email:     "userA@email.com",
					IsOwner:   false,
					IsActive:  false,
					Roles:     []string{data.BusinessUserRole.String()},
				},
				{
					ID:        "user2-ID",
					FirstName: "First",
					LastName:  "Last",
					Email:     "userC@email.com",
					IsOwner:   true,
					IsActive:  true,
					Roles:     []string{data.OwnerUserRole.String()},
				},
				{
					ID:        "user3-ID",
					FirstName: "First",
					LastName:  "Last",
					Email:     "userB@email.com",
					IsOwner:   true,
					IsActive:  true,
					Roles:     []string{data.OwnerUserRole.String()},
				},
			}, nil).
			Once()

		w := httptest.NewRecorder()

		http.HandlerFunc(handler.GetAllUsers).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody := `
			[
				{
					"id": "user2-ID",
					"first_name": "First",
					"last_name": "Last",
					"email": "userC@email.com",
					"is_active": true,
					"roles": [
						"owner"
					]
				},
				{
					"id":        "user3-ID",
					"first_name": "First",
					"last_name":  "Last",
					"email":     "userB@email.com",
					"is_active":  true,
					"roles": [
						"owner"
					]
				},
				{
					"id": "user1-ID",
					"first_name": "First",
					"last_name": "Last",
					"email": "userA@email.com",
					"is_active": false,
					"roles": [
						"business"
					]
				}
			]
		`

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
	})

	t.Run("returns all users ordered by is_active ASC", func(t *testing.T) {
		token := "mytoken"

		ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, orderByIsActiveAscURL, nil)
		require.NoError(t, err)

		jwtManagerMock.
			On("ValidateToken", req.Context(), token).
			Return(true, nil).
			Once()

		authenticatorMock.
			On("GetAllUsers", req.Context()).
			Return([]auth.User{
				{
					ID:        "user1-ID",
					FirstName: "First",
					LastName:  "Last",
					Email:     "userA@email.com",
					IsOwner:   false,
					IsActive:  false,
					Roles:     []string{data.BusinessUserRole.String()},
				},
				{
					ID:        "user2-ID",
					FirstName: "First",
					LastName:  "Last",
					Email:     "userB@email.com",
					IsOwner:   true,
					IsActive:  true,
					Roles:     []string{data.OwnerUserRole.String()},
				},
			}, nil).
			Once()

		w := httptest.NewRecorder()

		http.HandlerFunc(handler.GetAllUsers).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody := `
			[
				{
					"id": "user2-ID",
					"first_name": "First",
					"last_name": "Last",
					"email": "userB@email.com",
					"is_active": true,
					"roles": [
						"owner"
					]
				},
				{
					"id": "user1-ID",
					"first_name": "First",
					"last_name": "Last",
					"email": "userA@email.com",
					"is_active": false,
					"roles": [
						"business"
					]
				}
			]
		`

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
	})

	t.Run("returns all users ordered by is_active DESC", func(t *testing.T) {
		token := "mytoken"

		ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, orderByIsActiveDescURL, nil)
		require.NoError(t, err)

		jwtManagerMock.
			On("ValidateToken", req.Context(), token).
			Return(true, nil).
			Once()

		authenticatorMock.
			On("GetAllUsers", req.Context()).
			Return([]auth.User{
				{
					ID:        "user1-ID",
					FirstName: "First",
					LastName:  "Last",
					Email:     "userA@email.com",
					IsOwner:   false,
					IsActive:  false,
					Roles:     []string{data.BusinessUserRole.String()},
				},
				{
					ID:        "user2-ID",
					FirstName: "First",
					LastName:  "Last",
					Email:     "userB@email.com",
					IsOwner:   true,
					IsActive:  true,
					Roles:     []string{data.OwnerUserRole.String()},
				},
			}, nil).
			Once()

		w := httptest.NewRecorder()

		http.HandlerFunc(handler.GetAllUsers).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody := `
			[
				{
					"id": "user1-ID",
					"first_name": "First",
					"last_name": "Last",
					"email": "userA@email.com",
					"is_active": false,
					"roles": [
						"business"
					]
				},
				{
					"id":        "user2-ID",
					"first_name": "First",
					"last_name":  "Last",
					"email":     "userB@email.com",
					"is_active":  true,
					"roles": [
						"owner"
					]
				}
			]
		`

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
	})
}
