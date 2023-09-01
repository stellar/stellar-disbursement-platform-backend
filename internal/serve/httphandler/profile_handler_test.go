package httphandler

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/publicfiles"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createOrganizationProfileMultipartRequest(t *testing.T, url, fieldName, filename, body string, fileContent io.Reader) (*http.Request, error) {
	buf := new(bytes.Buffer)
	writer := multipart.NewWriter(buf)
	defer writer.Close()

	if fieldName == "" {
		fieldName = "logo"
	}

	part, err := writer.CreateFormFile(fieldName, filename)
	require.NoError(t, err)

	_, err = io.Copy(part, fileContent)
	require.NoError(t, err)

	// adding the data
	err = writer.WriteField("data", body)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch, url, buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, nil
}

func Test_PatchOrganizationProfileRequest_AreAllFieldsEmpty(t *testing.T) {
	r := &PatchOrganizationProfileRequest{
		OrganizationName:  "",
		TimezoneUTCOffset: "",
	}
	res := r.AreAllFieldsEmpty()
	assert.True(t, res)

	r = &PatchOrganizationProfileRequest{
		OrganizationName:  "MyAid",
		TimezoneUTCOffset: "",
	}
	res = r.AreAllFieldsEmpty()
	assert.False(t, res)

	r = &PatchOrganizationProfileRequest{
		OrganizationName:  "",
		TimezoneUTCOffset: "-03:00",
	}
	res = r.AreAllFieldsEmpty()
	assert.False(t, res)
}

func Test_ProfileHandler_PatchOrganizationProfile(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	handler := &ProfileHandler{Models: models, MaxMemoryAllocation: DefaultMaxMemoryAllocation}
	url := "/profile/organization"

	resetOrganizationInfo := func(t *testing.T, ctx context.Context) {
		const q = "UPDATE organizations SET name = 'MyCustomAid', logo = NULL, timezone_utc_offset = '+00:00'"
		_, err := dbConnectionPool.ExecContext(ctx, q)
		require.NoError(t, err)
	}

	ctx := context.Background()

	t.Run("returns Unauthorized error when no token is found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, nil)
		require.NoError(t, err)

		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Not authorized."}`, string(respBody))
	})

	t.Run("returns BadRequest error when the request is invalid", func(t *testing.T) {
		ctx = context.WithValue(ctx, middleware.TokenContextKey, "token")

		// Invalid JSON data
		img := data.CreateMockImage(t, 300, 300, data.ImageSizeSmall)
		imgBuf := new(bytes.Buffer)
		err := png.Encode(imgBuf, img)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req, err := createOrganizationProfileMultipartRequest(t, url, "logo", "logo.png", `invalid`, imgBuf)
		require.NoError(t, err)

		req = req.WithContext(ctx)

		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "The request was invalid in some way."}`, string(respBody))

		// Invalid file format
		csvBuf := new(bytes.Buffer)
		csvWriter := csv.NewWriter(csvBuf)
		err = csvWriter.WriteAll([][]string{
			{"name", "age"},
			{"foo", "99"},
			{"bar", "99"},
		})
		require.NoError(t, err)

		w = httptest.NewRecorder()
		req, err = createOrganizationProfileMultipartRequest(t, url, "logo", "logo.csv", `{}`, csvBuf)
		require.NoError(t, err)

		req = req.WithContext(ctx)

		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp = w.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody := `
			{
				"error": "The request was invalid in some way.",
				"extras": {
					"logo": "invalid file type provided. Expected png or jpeg."
				}
			}
		`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))

		// Neither logo and organization_name isn't present.
		w = httptest.NewRecorder()
		req, err = createOrganizationProfileMultipartRequest(t, url, "wrong", "logo.png", `{}`, new(bytes.Buffer))
		require.NoError(t, err)

		req = req.WithContext(ctx)

		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp = w.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "request is invalid", "extras": {"details": "data or logo is required"}}`, string(respBody))
	})

	t.Run("returns BadRequest error when the request size is too large", func(t *testing.T) {
		ctx = context.WithValue(ctx, middleware.TokenContextKey, "token")

		img := data.CreateMockImage(t, 3840, 2160, data.ImageSizeMedium)
		imgBuf := new(bytes.Buffer)
		err := jpeg.Encode(imgBuf, img, &jpeg.Options{Quality: jpeg.DefaultQuality})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req, err := createOrganizationProfileMultipartRequest(t, url, "logo", "logo.jpeg", `{}`, imgBuf)
		require.NoError(t, err)

		req = req.WithContext(ctx)

		getEntries := log.DefaultLogger.StartTest(log.ErrorLevel)

		profileHandler := &ProfileHandler{Models: models, MaxMemoryAllocation: 1024 * 1024}
		http.HandlerFunc(profileHandler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "could not parse multipart form data", "extras": {"details": "request too large. Max size 2MB."}}`, string(respBody))

		entries := getEntries()
		assert.Equal(t, "error parsing multipart form: http: request body too large", entries[0].Message)
	})

	t.Run("updates the organization's name successfully", func(t *testing.T) {
		resetOrganizationInfo(t, ctx)

		ctx = context.WithValue(ctx, middleware.TokenContextKey, "token")

		org, err := models.Organizations.Get(ctx)
		require.NoError(t, err)

		assert.Equal(t, "MyCustomAid", org.Name)
		assert.Nil(t, org.Logo)

		w := httptest.NewRecorder()
		req, err := createOrganizationProfileMultipartRequest(t, url, "", "", `{"organization_name": "My Org Name"}`, new(bytes.Buffer))
		require.NoError(t, err)

		req = req.WithContext(ctx)

		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "organization profile updated successfully"}`, string(respBody))

		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)

		assert.Equal(t, "My Org Name", org.Name)
		assert.Nil(t, org.Logo)
	})

	t.Run("updates the organization's timezone UTC offset successfully", func(t *testing.T) {
		resetOrganizationInfo(t, ctx)

		ctx = context.WithValue(ctx, middleware.TokenContextKey, "token")

		org, err := models.Organizations.Get(ctx)
		require.NoError(t, err)

		assert.Equal(t, "+00:00", org.TimezoneUTCOffset)
		assert.Equal(t, "MyCustomAid", org.Name)
		assert.Nil(t, org.Logo)

		w := httptest.NewRecorder()
		req, err := createOrganizationProfileMultipartRequest(t, url, "", "", `{"timezone_utc_offset": "-03:00"}`, new(bytes.Buffer))
		require.NoError(t, err)

		req = req.WithContext(ctx)

		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "organization profile updated successfully"}`, string(respBody))

		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)

		assert.Equal(t, "-03:00", org.TimezoneUTCOffset)
		assert.Equal(t, "MyCustomAid", org.Name)
		assert.Nil(t, org.Logo)
	})

	t.Run("updates the organization's IsApprovalRequired successfully", func(t *testing.T) {
		resetOrganizationInfo(t, ctx)

		ctx = context.WithValue(ctx, middleware.TokenContextKey, "token")

		org, err := models.Organizations.Get(ctx)
		require.NoError(t, err)
		assert.False(t, org.IsApprovalRequired)

		w := httptest.NewRecorder()
		req, err := createOrganizationProfileMultipartRequest(t, url, "", "", `{"is_approval_required": true}`, new(bytes.Buffer))
		require.NoError(t, err)

		req = req.WithContext(ctx)

		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "organization profile updated successfully"}`, string(respBody))

		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)
		require.True(t, org.IsApprovalRequired)
	})

	t.Run("updates the organization's logo successfully", func(t *testing.T) {
		resetOrganizationInfo(t, ctx)

		ctx = context.WithValue(ctx, middleware.TokenContextKey, "token")

		// PNG logo
		org, err := models.Organizations.Get(ctx)
		require.NoError(t, err)

		assert.Nil(t, org.Logo)
		assert.Equal(t, "MyCustomAid", org.Name)

		img := data.CreateMockImage(t, 300, 300, data.ImageSizeSmall)
		imgBuf := new(bytes.Buffer)
		err = png.Encode(imgBuf, img)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req, err := createOrganizationProfileMultipartRequest(t, url, "logo", "logo.png", `{}`, imgBuf)
		require.NoError(t, err)

		req = req.WithContext(ctx)

		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "organization profile updated successfully"}`, string(respBody))

		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)

		// renew buffer
		imgBuf = new(bytes.Buffer)
		err = png.Encode(imgBuf, img)
		require.NoError(t, err)

		assert.Equal(t, imgBuf.Bytes(), org.Logo)
		assert.Equal(t, "MyCustomAid", org.Name)

		// JPEG logo
		resetOrganizationInfo(t, ctx)

		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)

		assert.Nil(t, org.Logo)
		assert.Equal(t, "MyCustomAid", org.Name)

		img = data.CreateMockImage(t, 300, 300, data.ImageSizeSmall)
		imgBuf = new(bytes.Buffer)
		err = jpeg.Encode(imgBuf, img, &jpeg.Options{Quality: jpeg.DefaultQuality})
		require.NoError(t, err)

		w = httptest.NewRecorder()
		req, err = createOrganizationProfileMultipartRequest(t, url, "logo", "logo.jpeg", `{}`, imgBuf)
		require.NoError(t, err)

		req = req.WithContext(ctx)

		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp = w.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "organization profile updated successfully"}`, string(respBody))

		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)

		// renew buffer
		imgBuf = new(bytes.Buffer)
		err = jpeg.Encode(imgBuf, img, &jpeg.Options{Quality: jpeg.DefaultQuality})
		require.NoError(t, err)

		assert.Equal(t, imgBuf.Bytes(), org.Logo)
		assert.Equal(t, "MyCustomAid", org.Name)
	})

	t.Run("updates both organization name, timezone UTC offset and logo successfully", func(t *testing.T) {
		resetOrganizationInfo(t, ctx)

		ctx = context.WithValue(ctx, middleware.TokenContextKey, "token")

		org, err := models.Organizations.Get(ctx)
		require.NoError(t, err)

		assert.Equal(t, "MyCustomAid", org.Name)
		assert.Equal(t, "+00:00", org.TimezoneUTCOffset)
		assert.Nil(t, org.Logo)

		img := data.CreateMockImage(t, 300, 300, data.ImageSizeSmall)
		imgBuf := new(bytes.Buffer)
		err = png.Encode(imgBuf, img)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req, err := createOrganizationProfileMultipartRequest(t, url, "logo", "logo.png", `{"organization_name": "My Org Name", "timezone_utc_offset": "-03:00"}`, imgBuf)
		require.NoError(t, err)

		req = req.WithContext(ctx)

		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "organization profile updated successfully"}`, string(respBody))

		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)

		// renew buffer
		imgBuf = new(bytes.Buffer)
		err = png.Encode(imgBuf, img)
		require.NoError(t, err)

		assert.Equal(t, "My Org Name", org.Name)
		assert.Equal(t, "-03:00", org.TimezoneUTCOffset)
		assert.Equal(t, imgBuf.Bytes(), org.Logo)
	})
}

func Test_ProfileHandler_PatchUserProfile(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	authenticatorMock := &auth.AuthenticatorMock{}
	jwtManagerMock := &auth.JWTManagerMock{}
	authManager := auth.NewAuthManager(
		auth.WithCustomAuthenticatorOption(authenticatorMock),
		auth.WithCustomJWTManagerOption(jwtManagerMock),
	)

	handler := &ProfileHandler{AuthManager: authManager}
	url := "/profile/user"

	ctx := context.Background()

	t.Run("returns Unauthorized error when no token is found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, nil)
		require.NoError(t, err)

		http.HandlerFunc(handler.PatchUserProfile).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Not authorized."}`, string(respBody))
	})

	t.Run("returns BadRequest error when the request is invalid", func(t *testing.T) {
		ctx = context.WithValue(ctx, middleware.TokenContextKey, "token")

		// Invalid JSON
		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(`invalid`))
		require.NoError(t, err)

		http.HandlerFunc(handler.PatchUserProfile).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "The request was invalid in some way."}`, string(respBody))

		// Invalid email
		w = httptest.NewRecorder()
		req, err = http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(`{"email": "invalid"}`))
		require.NoError(t, err)

		req = req.WithContext(ctx)

		http.HandlerFunc(handler.PatchUserProfile).ServeHTTP(w, req)

		resp = w.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "The request was invalid in some way.", "extras": {"email": "invalid email provided"}}`, string(respBody))

		// Password too short
		w = httptest.NewRecorder()
		req, err = http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(`{"password": "short"}`))
		require.NoError(t, err)

		http.HandlerFunc(handler.PatchUserProfile).ServeHTTP(w, req)

		resp = w.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "The request was invalid in some way.", "extras": {"password": "password should have at least 8 characters"}}`, string(respBody))

		// None of values provided
		w = httptest.NewRecorder()
		req, err = http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(`{}`))
		require.NoError(t, err)

		http.HandlerFunc(handler.PatchUserProfile).ServeHTTP(w, req)

		resp = w.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "The request was invalid in some way.", "extras": {"details":"provide at least first_name, last_name, email or password."}}`, string(respBody))
	})

	t.Run("returns InternalServerError when AuthManager fails", func(t *testing.T) {
		ctx = context.WithValue(ctx, middleware.TokenContextKey, "token")

		reqBody := `
			{
				"first_name": "First",
				"last_name": "Last",
				"email": "email@email.com",
				"password": "mypassword"
			}
		`

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(reqBody))
		require.NoError(t, err)

		jwtManagerMock.
			On("ValidateToken", req.Context(), "token").
			Return(true, nil).
			Once().
			On("GetUserFromToken", req.Context(), "token").
			Return(&auth.User{ID: "user-id"}, nil).
			Once()

		authenticatorMock.
			On("UpdateUser", req.Context(), "user-id", "First", "Last", "email@email.com", "mypassword").
			Return(errors.New("unexpected error")).
			Once()

		http.HandlerFunc(handler.PatchUserProfile).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Cannot update user profiles"}`, string(respBody))
	})

	t.Run("updates the user profile successfully", func(t *testing.T) {
		ctx = context.WithValue(ctx, middleware.TokenContextKey, "token")

		reqBody := `
			{
				"first_name": "First",
				"last_name": "Last",
				"email": "email@email.com",
				"password": "mypassword"
			}
		`

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(reqBody))
		require.NoError(t, err)

		jwtManagerMock.
			On("ValidateToken", req.Context(), "token").
			Return(true, nil).
			Once().
			On("GetUserFromToken", req.Context(), "token").
			Return(&auth.User{ID: "user-id"}, nil).
			Once()

		authenticatorMock.
			On("UpdateUser", req.Context(), "user-id", "First", "Last", "email@email.com", "mypassword").
			Return(nil).
			Once()

		http.HandlerFunc(handler.PatchUserProfile).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "user profile updated successfully"}`, string(respBody))
	})

	authenticatorMock.AssertExpectations(t)
	jwtManagerMock.AssertExpectations(t)
}

func Test_ProfileHandler_PatchUserPassword(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	authenticatorMock := &auth.AuthenticatorMock{}
	jwtManagerMock := &auth.JWTManagerMock{}
	authManager := auth.NewAuthManager(
		auth.WithCustomAuthenticatorOption(authenticatorMock),
		auth.WithCustomJWTManagerOption(jwtManagerMock),
	)
	handler := &ProfileHandler{AuthManager: authManager}

	url := "/profile/reset-password"
	ctx := context.Background()

	user := &auth.User{
		ID:        "user-id",
		FirstName: "First",
		LastName:  "Last",
		Email:     "email@email.com",
	}

	t.Run("returns Unauthorized error when no token is found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, nil)
		require.NoError(t, err)

		http.HandlerFunc(handler.PatchUserPassword).ServeHTTP(w, req)

		resp := w.Result()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.JSONEq(t, `{"error": "Not authorized."}`, string(respBody))
	})

	t.Run("returns BadRequest error when JSON decoding fails", func(t *testing.T) {
		ctx = context.WithValue(ctx, middleware.TokenContextKey, "token")

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(`invalid`))
		require.NoError(t, err)

		http.HandlerFunc(handler.PatchUserPassword).ServeHTTP(w, req)

		resp := w.Result()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.JSONEq(t, `{"error": "The request was invalid in some way."}`, string(respBody))
	})

	t.Run("returns BadRequest error when current_password and new_password are not provided", func(t *testing.T) {
		ctx = context.WithValue(ctx, middleware.TokenContextKey, "token")

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(`{}`))
		require.NoError(t, err)

		http.HandlerFunc(handler.PatchUserPassword).ServeHTTP(w, req)

		resp := w.Result()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()
		wantBody := `{
			"error": "The request was invalid in some way.",
			"extras": {
				"current_password":"current_password is required",
				"new_password":"new_password should be different from current_password"
			}
		}`
		assert.JSONEq(t, wantBody, string(respBody))
	})

	t.Run("returns BadRequest error when current_password and new_password are equal", func(t *testing.T) {
		ctx = context.WithValue(ctx, middleware.TokenContextKey, "token")
		reqBody := `{"current_password": "currentpassword", "new_password": "currentpassword"}`

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(reqBody))
		require.NoError(t, err)

		http.HandlerFunc(handler.PatchUserPassword).ServeHTTP(w, req)

		resp := w.Result()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()
		wantBody := `{
			"error": "The request was invalid in some way.",
			"extras": {
				"new_password":"new_password should be different from current_password"
			}
		}`
		assert.JSONEq(t, wantBody, string(respBody))
	})

	t.Run("returns BadRequest error when password does not match all the criteria", func(t *testing.T) {
		ctx = context.WithValue(ctx, middleware.TokenContextKey, "token")
		reqBody := `{"current_password": "currentpassword", "new_password": "1Az2By3Cx"}`

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(reqBody))
		require.NoError(t, err)

		http.HandlerFunc(handler.PatchUserPassword).ServeHTTP(w, req)

		resp := w.Result()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()
		wantBody := `{
			"error": "The request was invalid in some way.",
			"extras": {
				"length":"password length must be between 12 and 36 characters",
				"special character":"password must contain at least one special character"
			}
		}`
		assert.JSONEq(t, wantBody, string(respBody))
	})

	t.Run("returns InternalServerError when AuthManager fails", func(t *testing.T) {
		ctx = context.WithValue(ctx, middleware.TokenContextKey, "token")
		reqBody := `{"current_password": "currentpassword", "new_password": "!1Az?2By.3Cx"}`

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(reqBody))
		require.NoError(t, err)

		jwtManagerMock.
			On("ValidateToken", req.Context(), "token").
			Return(true, nil).
			Once().
			On("GetUserFromToken", req.Context(), "token").
			Return(user, nil).
			Once()

		authenticatorMock.
			On("UpdatePassword", req.Context(), user, "currentpassword", "!1Az?2By.3Cx").
			Return(errors.New("unexpected error")).
			Once()

		http.HandlerFunc(handler.PatchUserPassword).ServeHTTP(w, req)

		resp := w.Result()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"Cannot update user password"}`, string(respBody))
	})

	t.Run("updates the user password successfully", func(t *testing.T) {
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.SetLevel(log.InfoLevel)

		ctx = context.WithValue(ctx, middleware.TokenContextKey, "token")
		reqBody := `{"current_password": "currentpassword", "new_password": "!1Az?2By.3Cx"}`

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(reqBody))
		require.NoError(t, err)

		jwtManagerMock.
			On("ValidateToken", req.Context(), "token").
			Return(true, nil).
			Twice().
			On("GetUserFromToken", req.Context(), "token").
			Return(user, nil).
			Twice()

		authenticatorMock.
			On("UpdatePassword", req.Context(), user, "currentpassword", "!1Az?2By.3Cx").
			Return(nil).
			Once()

		http.HandlerFunc(handler.PatchUserPassword).ServeHTTP(w, req)

		resp := w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"message": "user password updated successfully"}`, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "[UpdateUserPassword] - Updated password for user with account ID user-id")
	})

	authenticatorMock.AssertExpectations(t)
	jwtManagerMock.AssertExpectations(t)
}

func Test_ProfileHandler_GetProfile(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	authManagerMock := &auth.AuthManagerMock{}

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	handler := &ProfileHandler{Models: models, AuthManager: authManagerMock}
	url := "/profile"

	ctx := context.Background()

	t.Run("returns Unauthorized error when no token is found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		http.HandlerFunc(handler.GetProfile).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Not authorized."}`, string(respBody))
	})

	t.Run("returns Unauthorized when AuthManager fails with ErrInvalidToken", func(t *testing.T) {
		token := "mytoken"
		ctx = context.WithValue(ctx, middleware.TokenContextKey, token)

		expectedErr := auth.ErrInvalidToken
		authManagerMock.
			On("GetUser", ctx, token).
			Return(nil, expectedErr).
			Once()

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		getEntries := log.DefaultLogger.StartTest(log.ErrorLevel)

		http.HandlerFunc(handler.GetProfile).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Not authorized."}`, string(respBody))

		entries := getEntries()
		expectedLog := fmt.Sprintf("getting user profile: %s", expectedErr)
		assert.Equal(t, expectedLog, entries[0].Message)
	})

	t.Run("returns BadRequest when user is not found", func(t *testing.T) {
		token := "mytoken"
		ctx = context.WithValue(ctx, middleware.TokenContextKey, token)
		expectedErr := fmt.Errorf("error getting user ID %s: %w", "user-id", auth.ErrUserNotFound)

		authManagerMock.
			On("GetUser", ctx, token).
			Return(nil, expectedErr).
			Once()

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		getEntries := log.DefaultLogger.StartTest(log.ErrorLevel)

		http.HandlerFunc(handler.GetProfile).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "The request was invalid in some way."}`, string(respBody))

		entries := getEntries()
		expectedLog := fmt.Sprintf("user from token mytoken not found: %s", expectedErr)
		assert.Equal(t, expectedLog, entries[0].Message)
	})

	t.Run("returns InternalServerError when AuthManager fails", func(t *testing.T) {
		token := "mytoken"
		ctx = context.WithValue(ctx, middleware.TokenContextKey, token)

		expectedErr := errors.New("error getting user ID user-id: unexpected error")
		authManagerMock.
			On("GetUser", ctx, token).
			Return(nil, expectedErr).
			Once()

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		getEntries := log.DefaultLogger.StartTest(log.ErrorLevel)

		http.HandlerFunc(handler.GetProfile).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Cannot get user"}`, string(respBody))

		entries := getEntries()
		expectedLog := fmt.Sprintf("Cannot get user: %s", expectedErr)
		assert.Equal(t, expectedLog, entries[0].Message)
	})

	t.Run("returns the profile info successfully", func(t *testing.T) {
		token := "mytoken"
		ctx = context.WithValue(ctx, middleware.TokenContextKey, token)

		u := &auth.User{
			ID:        "user-id",
			FirstName: "First",
			LastName:  "Last",
			Email:     "email@email.com",
			Roles:     []string{data.DeveloperUserRole.String()},
		}

		authManagerMock.
			On("GetUser", ctx, token).
			Return(u, nil).
			Once()

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		http.HandlerFunc(handler.GetProfile).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody := `
			{
				"id":"user-id",
				"email": "email@email.com",
				"first_name": "First",
				"last_name": "Last",
				"organization_name": "MyCustomAid",
				"roles": ["developer"]
			}
		`

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
	})

	authManagerMock.AssertExpectations(t)
}

func Test_ProfileHandler_GetOrganizationInfo(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	distributionAccountPK := keypair.MustRandom().Address()
	handler := &ProfileHandler{Models: models, BaseURL: "http://localhost:8000", DistributionPublicKey: distributionAccountPK}
	url := "/profile/info"

	ctx := context.Background()

	t.Run("returns Unauthorized error when no token is found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		http.HandlerFunc(handler.GetOrganizationInfo).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Not authorized."}`, string(respBody))
	})

	t.Run("returns InternalServerError if getting logo URL fails", func(t *testing.T) {
		ctx = context.WithValue(ctx, middleware.TokenContextKey, "mytoken")

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		getEntries := log.DefaultLogger.StartTest(log.ErrorLevel)

		h := &ProfileHandler{Models: models, BaseURL: "%invalid%"}
		http.HandlerFunc(h.GetOrganizationInfo).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Cannot get logo URL"}`, string(respBody))

		entries := getEntries()
		assert.Equal(t, `Cannot get logo URL: parse "%invalid%": invalid URL escape "%in"`, entries[0].Message)
	})

	t.Run("returns the organization info successfully", func(t *testing.T) {
		ctx = context.WithValue(ctx, middleware.TokenContextKey, "mytoken")

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		http.HandlerFunc(handler.GetOrganizationInfo).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody := fmt.Sprintf(`
			{
				"logo_url": "http://localhost:8000/organization/logo?token=mytoken",
				"name": "MyCustomAid",
				"distribution_account_public_key": %q,
				"timezone_utc_offset": "+00:00",
				"is_approval_required":false
			}
		`, distributionAccountPK)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
	})
}

func Test_ProfileHandler_GetOrganizationLogo(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	models, outerErr := data.NewModels(dbConnectionPool)
	require.NoError(t, outerErr)

	handler := &ProfileHandler{Models: models, PublicFilesFS: publicfiles.PublicFiles}
	url := "/organization/logo"

	ctx := context.Background()

	t.Run("returns InternalServerError when can't find the default logo file", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		getEntries := log.DefaultLogger.StartTest(log.ErrorLevel)

		fsMap := fstest.MapFS{}
		h := &ProfileHandler{Models: models, PublicFilesFS: fsMap}
		http.HandlerFunc(h.GetOrganizationLogo).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Cannot open default logo"}`, string(respBody))

		entries := getEntries()
		assert.NotEmpty(t, entries)
		assert.Equal(t, `Cannot open default logo: open img/logo.png: file does not exist`, entries[0].Message)
	})

	t.Run("returns the default logo when no logo is set", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		http.HandlerFunc(handler.GetOrganizationLogo).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		expectedLogoBytes, err := fs.ReadFile(publicfiles.PublicFiles, "img/logo.png")
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, expectedLogoBytes, respBody)
	})

	t.Run("returns the organization logo stored in the database successfully", func(t *testing.T) {
		imgBuf := new(bytes.Buffer)
		img := data.CreateMockImage(t, 300, 300, data.ImageSizeSmall)
		err := png.Encode(imgBuf, img)
		require.NoError(t, err)

		err = models.Organizations.Update(ctx, &data.OrganizationUpdate{Logo: imgBuf.Bytes()})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		http.HandlerFunc(handler.GetOrganizationLogo).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		org, err := models.Organizations.Get(ctx)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, org.Logo, respBody)
	})
}
