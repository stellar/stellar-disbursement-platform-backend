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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/publicfiles"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/utils"
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

func resetOrganizationInfo(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool) {
	t.Helper()

	const q = `
		UPDATE
			organizations
		SET
			name = 'MyCustomAid', logo = NULL, timezone_utc_offset = '+00:00',
			sms_registration_message_template = DEFAULT, otp_message_template = DEFAULT,
			sms_resend_interval = NULL`
	_, err := dbConnectionPool.ExecContext(ctx, q)
	require.NoError(t, err)
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
		token := "token"
		ctx = context.WithValue(ctx, middleware.TokenContextKey, token)

		// Setup handler
		user := &auth.User{ID: "user-id"}
		authManagerMock := &auth.AuthManagerMock{}
		authManagerMock.
			On("GetUser", mock.Anything, token).
			Return(user, nil).
			Times(3)
		handler.AuthManager = authManagerMock
		defer func() { handler.AuthManager = nil }()
		defer authManagerMock.AssertExpectations(t)

		// Case 1: invalid JSON data
		img := data.CreateMockImage(t, 300, 300, data.ImageSizeSmall)
		imgBuf := new(bytes.Buffer)
		err := png.Encode(imgBuf, img)
		require.NoError(t, err)

		// Execute the request
		w := httptest.NewRecorder()
		req, err := createOrganizationProfileMultipartRequest(t, url, "logo", "logo.png", `invalid`, imgBuf)
		require.NoError(t, err)
		req = req.WithContext(ctx)
		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		// Assert response
		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "The request was invalid in some way."}`, string(respBody))

		// Case 2: invalid file format
		csvBuf := new(bytes.Buffer)
		csvWriter := csv.NewWriter(csvBuf)
		err = csvWriter.WriteAll([][]string{
			{"name", "age"},
			{"foo", "99"},
			{"bar", "99"},
		})
		require.NoError(t, err)

		// Execute the request
		w = httptest.NewRecorder()
		req, err = createOrganizationProfileMultipartRequest(t, url, "logo", "logo.csv", `{}`, csvBuf)
		require.NoError(t, err)
		req = req.WithContext(ctx)
		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		// Assert response
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

		// Case 3: Neither logo and organization_name isn't present.
		// Execute the request:
		w = httptest.NewRecorder()
		req, err = createOrganizationProfileMultipartRequest(t, url, "wrong", "logo.png", `{}`, new(bytes.Buffer))
		require.NoError(t, err)
		req = req.WithContext(ctx)
		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		// Assert response:
		resp = w.Result()
		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "request is invalid", "extras": {"details": "data or logo is required"}}`, string(respBody))
	})

	t.Run("returns BadRequest error when the request size is too large", func(t *testing.T) {
		token := "token"
		ctx = context.WithValue(ctx, middleware.TokenContextKey, token)
		authManagerMock := &auth.AuthManagerMock{}
		authManagerMock.
			On("GetUser", mock.Anything, token).
			Return(&auth.User{ID: "user-id"}, nil).
			Once()
		getEntries := log.DefaultLogger.StartTest(log.ErrorLevel)
		defer authManagerMock.AssertExpectations(t)

		profileHandler := &ProfileHandler{AuthManager: authManagerMock, Models: models, MaxMemoryAllocation: 1024 * 1024}
		profileHandler.AuthManager = authManagerMock

		img := data.CreateMockImage(t, 3840, 2160, data.ImageSizeMedium)
		imgBuf := new(bytes.Buffer)
		err := jpeg.Encode(imgBuf, img, &jpeg.Options{Quality: jpeg.DefaultQuality})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req, err := createOrganizationProfileMultipartRequest(t, url, "logo", "logo.jpeg", `{}`, imgBuf)
		require.NoError(t, err)
		req = req.WithContext(ctx)

		http.HandlerFunc(profileHandler.PatchOrganizationProfile).ServeHTTP(w, req)
		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "could not parse multipart form data", "extras": {"details": "request too large. Max size 2MB."}}`, string(respBody))

		entries := getEntries()
		assert.Equal(t, "error parsing multipart form: http: request body too large", entries[0].Message)
	})

	t.Run("🎉 successfully updates the organization's name", func(t *testing.T) {
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.SetLevel(log.InfoLevel)

		resetOrganizationInfo(t, ctx, dbConnectionPool)
		token := "token"
		ctx = context.WithValue(ctx, middleware.TokenContextKey, token)

		// Setup handler
		user := &auth.User{ID: "user-id"}
		authManagerMock := &auth.AuthManagerMock{}
		authManagerMock.
			On("GetUser", mock.Anything, token).
			Return(user, nil).
			Once()
		handler.AuthManager = authManagerMock
		defer func() { handler.AuthManager = nil }()
		defer authManagerMock.AssertExpectations(t)

		// org before the PATCH request
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
		assert.JSONEq(t, `{"message": "updated successfully"}`, string(respBody))

		// org after the PATCH request
		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, "My Org Name", org.Name)
		assert.Nil(t, org.Logo)

		// validate logs
		require.Contains(t, buf.String(), "[PatchOrganizationProfile] - userID user-id will update the organization fields [Name]")
	})

	t.Run("🎉 successfully updates the organization's timezone UTC offset", func(t *testing.T) {
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.SetLevel(log.InfoLevel)

		resetOrganizationInfo(t, ctx, dbConnectionPool)
		token := "token"
		ctx = context.WithValue(ctx, middleware.TokenContextKey, token)

		// Setup handler
		user := &auth.User{ID: "user-id"}
		authManagerMock := &auth.AuthManagerMock{}
		authManagerMock.
			On("GetUser", mock.Anything, token).
			Return(user, nil).
			Once()
		handler.AuthManager = authManagerMock
		defer func() { handler.AuthManager = nil }()
		defer authManagerMock.AssertExpectations(t)

		// org before the PATCH request
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
		assert.JSONEq(t, `{"message": "updated successfully"}`, string(respBody))

		// org after the PATCH request
		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, "-03:00", org.TimezoneUTCOffset)
		assert.Equal(t, "MyCustomAid", org.Name)
		assert.Nil(t, org.Logo)

		// validate logs
		require.Contains(t, buf.String(), "[PatchOrganizationProfile] - userID user-id will update the organization fields [TimezoneUTCOffset]")
	})

	t.Run("🎉 successfully updates the organization's IsApprovalRequired", func(t *testing.T) {
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.SetLevel(log.InfoLevel)

		resetOrganizationInfo(t, ctx, dbConnectionPool)
		token := "token"
		ctx = context.WithValue(ctx, middleware.TokenContextKey, token)

		// Setup handler
		user := &auth.User{ID: "user-id"}
		authManagerMock := &auth.AuthManagerMock{}
		authManagerMock.
			On("GetUser", mock.Anything, token).
			Return(user, nil).
			Once()
		handler.AuthManager = authManagerMock
		defer func() { handler.AuthManager = nil }()
		defer authManagerMock.AssertExpectations(t)

		// org before the PATCH request
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
		assert.JSONEq(t, `{"message": "updated successfully"}`, string(respBody))

		// org after the PATCH request
		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)
		require.True(t, org.IsApprovalRequired)

		// validate logs
		require.Contains(t, buf.String(), "[PatchOrganizationProfile] - userID user-id will update the organization fields [IsApprovalRequired]")
	})

	t.Run("🎉 successfully updates the organization's logo", func(t *testing.T) {
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.SetLevel(log.InfoLevel)

		resetOrganizationInfo(t, ctx, dbConnectionPool)
		token := "token"
		ctx = context.WithValue(ctx, middleware.TokenContextKey, token)

		// Setup handler
		user := &auth.User{ID: "user-id"}
		authManagerMock := &auth.AuthManagerMock{}
		authManagerMock.
			On("GetUser", mock.Anything, token).
			Return(user, nil).
			Twice()
		handler.AuthManager = authManagerMock
		defer func() { handler.AuthManager = nil }()
		defer authManagerMock.AssertExpectations(t)

		// org before the PATCH request (PNG)
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
		assert.JSONEq(t, `{"message": "updated successfully"}`, string(respBody))

		// org after the PATCH request (PNG)
		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)
		imgBuf = new(bytes.Buffer)
		err = png.Encode(imgBuf, img)
		require.NoError(t, err)
		assert.Equal(t, imgBuf.Bytes(), org.Logo)
		assert.Equal(t, "MyCustomAid", org.Name)

		// JPEG logo
		resetOrganizationInfo(t, ctx, dbConnectionPool)

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
		assert.JSONEq(t, `{"message": "updated successfully"}`, string(respBody))

		// org after the PATCH request (JPEG)
		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)
		imgBuf = new(bytes.Buffer)
		err = jpeg.Encode(imgBuf, img, &jpeg.Options{Quality: jpeg.DefaultQuality})
		require.NoError(t, err)
		assert.Equal(t, imgBuf.Bytes(), org.Logo)
		assert.Equal(t, "MyCustomAid", org.Name)

		// validate logs
		require.Contains(t, buf.String(), "[PatchOrganizationProfile] - userID user-id will update the organization fields [Logo]")
	})

	t.Run("🎉 successfully updates organization name, timezone UTC offset and logo", func(t *testing.T) {
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.SetLevel(log.InfoLevel)

		resetOrganizationInfo(t, ctx, dbConnectionPool)
		token := "token"
		ctx = context.WithValue(ctx, middleware.TokenContextKey, token)

		// Setup handler
		user := &auth.User{ID: "user-id"}
		authManagerMock := &auth.AuthManagerMock{}
		authManagerMock.
			On("GetUser", mock.Anything, token).
			Return(user, nil).
			Once()
		handler.AuthManager = authManagerMock
		defer func() { handler.AuthManager = nil }()
		defer authManagerMock.AssertExpectations(t)

		// org before the PATCH request
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
		assert.JSONEq(t, `{"message": "updated successfully"}`, string(respBody))

		// org after the PATCH request
		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)
		imgBuf = new(bytes.Buffer)
		err = png.Encode(imgBuf, img)
		require.NoError(t, err)

		assert.Equal(t, "My Org Name", org.Name)
		assert.Equal(t, "-03:00", org.TimezoneUTCOffset)
		assert.Equal(t, imgBuf.Bytes(), org.Logo)

		// validate logs
		require.Contains(t, buf.String(), "[PatchOrganizationProfile] - userID user-id will update the organization fields [Logo Name TimezoneUTCOffset]")
	})

	t.Run("🎉 successfully updates organization's SMS Registration Message Template", func(t *testing.T) {
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.SetLevel(log.InfoLevel)

		resetOrganizationInfo(t, ctx, dbConnectionPool)
		token := "token"
		ctx = context.WithValue(ctx, middleware.TokenContextKey, token)

		// Setup handler
		user := &auth.User{ID: "user-id"}
		authManagerMock := &auth.AuthManagerMock{}
		authManagerMock.
			On("GetUser", mock.Anything, token).
			Return(user, nil).
			Times(3)
		handler.AuthManager = authManagerMock
		defer func() { handler.AuthManager = nil }()
		defer authManagerMock.AssertExpectations(t)

		org, err := models.Organizations.Get(ctx)
		require.NoError(t, err)
		defaultMessage := "You have a payment waiting for you from the {{.OrganizationName}}. Click {{.RegistrationLink}} to register."
		assert.Equal(t, defaultMessage, org.SMSRegistrationMessageTemplate)

		// Custom message
		w := httptest.NewRecorder()
		req, err := createOrganizationProfileMultipartRequest(t, url, "", "", `{"sms_registration_message_template": "My custom receiver wallet registration invite. MyOrg 👋"}`, new(bytes.Buffer))
		require.NoError(t, err)
		req = req.WithContext(ctx)
		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "updated successfully"}`, string(respBody))

		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, "My custom receiver wallet registration invite. MyOrg 👋", org.SMSRegistrationMessageTemplate)

		// Don't update the message
		w = httptest.NewRecorder()
		req, err = createOrganizationProfileMultipartRequest(t, url, "", "", `{"organization_name": "MyOrg"}`, new(bytes.Buffer))
		require.NoError(t, err)
		req = req.WithContext(ctx)
		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp = w.Result()
		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "updated successfully"}`, string(respBody))

		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, "My custom receiver wallet registration invite. MyOrg 👋", org.SMSRegistrationMessageTemplate)

		// Back to default message
		w = httptest.NewRecorder()
		req, err = createOrganizationProfileMultipartRequest(t, url, "", "", `{"sms_registration_message_template": ""}`, new(bytes.Buffer))
		require.NoError(t, err)
		req = req.WithContext(ctx)
		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp = w.Result()
		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "updated successfully"}`, string(respBody))
		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, defaultMessage, org.SMSRegistrationMessageTemplate)

		// validate logs
		require.Contains(t, buf.String(), "[PatchOrganizationProfile] - userID user-id will update the organization fields [SMSRegistrationMessageTemplate]")
		require.Contains(t, buf.String(), "[PatchOrganizationProfile] - userID user-id will update the organization fields [Name]")
	})

	t.Run("🎉 successfully updates organization's OTP Message Template", func(t *testing.T) {
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.SetLevel(log.InfoLevel)

		resetOrganizationInfo(t, ctx, dbConnectionPool)
		token := "token"
		ctx = context.WithValue(ctx, middleware.TokenContextKey, token)

		// Setup handler
		user := &auth.User{ID: "user-id"}
		authManagerMock := &auth.AuthManagerMock{}
		authManagerMock.
			On("GetUser", mock.Anything, token).
			Return(user, nil).
			Times(3)
		handler.AuthManager = authManagerMock
		defer func() { handler.AuthManager = nil }()
		defer authManagerMock.AssertExpectations(t)

		org, err := models.Organizations.Get(ctx)
		require.NoError(t, err)

		defaultMessage := "{{.OTP}} is your {{.OrganizationName}} phone verification code."
		assert.Equal(t, defaultMessage, org.OTPMessageTemplate)

		// Custom message
		w := httptest.NewRecorder()
		req, err := createOrganizationProfileMultipartRequest(t, url, "", "", `{"otp_message_template": "Here's your OTP Code to complete your registration. MyOrg 👋"}`, new(bytes.Buffer))
		require.NoError(t, err)
		req = req.WithContext(ctx)
		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "updated successfully"}`, string(respBody))

		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, "Here's your OTP Code to complete your registration. MyOrg 👋", org.OTPMessageTemplate)

		// Don't update the message
		w = httptest.NewRecorder()
		req, err = createOrganizationProfileMultipartRequest(t, url, "", "", `{"organization_name": "MyOrg"}`, new(bytes.Buffer))
		require.NoError(t, err)
		req = req.WithContext(ctx)
		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp = w.Result()
		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "updated successfully"}`, string(respBody))

		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, "Here's your OTP Code to complete your registration. MyOrg 👋", org.OTPMessageTemplate)

		// Back to default message
		w = httptest.NewRecorder()
		req, err = createOrganizationProfileMultipartRequest(t, url, "", "", `{"otp_message_template": ""}`, new(bytes.Buffer))
		require.NoError(t, err)
		req = req.WithContext(ctx)
		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp = w.Result()
		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "updated successfully"}`, string(respBody))

		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, defaultMessage, org.OTPMessageTemplate)

		// validate logs
		require.Contains(t, buf.String(), "[PatchOrganizationProfile] - userID user-id will update the organization fields [OTPMessageTemplate]")
		require.Contains(t, buf.String(), "[PatchOrganizationProfile] - userID user-id will update the organization fields [Name]")
	})

	t.Run("🎉 successfully updates organization's SMS Resend Interval", func(t *testing.T) {
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.SetLevel(log.InfoLevel)

		resetOrganizationInfo(t, ctx, dbConnectionPool)
		token := "token"
		ctx = context.WithValue(ctx, middleware.TokenContextKey, token)

		// Setup handler
		user := &auth.User{ID: "user-id"}
		authManagerMock := &auth.AuthManagerMock{}
		authManagerMock.
			On("GetUser", mock.Anything, token).
			Return(user, nil).
			Times(3)
		handler.AuthManager = authManagerMock
		defer func() { handler.AuthManager = nil }()
		defer authManagerMock.AssertExpectations(t)

		org, err := models.Organizations.Get(ctx)
		require.NoError(t, err)
		assert.Nil(t, org.SMSResendInterval)

		// Custom interval
		w := httptest.NewRecorder()
		req, err := createOrganizationProfileMultipartRequest(t, url, "", "", `{"sms_resend_interval": 2}`, new(bytes.Buffer))
		require.NoError(t, err)
		req = req.WithContext(ctx)
		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "updated successfully"}`, string(respBody))

		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(2), *org.SMSResendInterval)

		// Don't update the interval
		w = httptest.NewRecorder()
		req, err = createOrganizationProfileMultipartRequest(t, url, "", "", `{"organization_name": "MyOrg"}`, new(bytes.Buffer))
		require.NoError(t, err)
		req = req.WithContext(ctx)
		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp = w.Result()
		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "updated successfully"}`, string(respBody))

		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(2), *org.SMSResendInterval)

		// Back to default interval
		w = httptest.NewRecorder()
		req, err = createOrganizationProfileMultipartRequest(t, url, "", "", `{"sms_resend_interval": 0}`, new(bytes.Buffer))
		require.NoError(t, err)
		req = req.WithContext(ctx)
		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp = w.Result()
		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "updated successfully"}`, string(respBody))

		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)
		assert.Nil(t, org.SMSResendInterval)

		// validate logs
		require.Contains(t, buf.String(), "[PatchOrganizationProfile] - userID user-id will update the organization fields [SMSResendInterval]")
		require.Contains(t, buf.String(), "[PatchOrganizationProfile] - userID user-id will update the organization fields [Name]")
	})

	t.Run("🎉 successfully updates organization's Payment Cancellation Period", func(t *testing.T) {
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.SetLevel(log.InfoLevel)

		resetOrganizationInfo(t, ctx, dbConnectionPool)
		token := "token"
		ctx = context.WithValue(ctx, middleware.TokenContextKey, token)

		// Setup handler
		user := &auth.User{ID: "user-id"}
		authManagerMock := &auth.AuthManagerMock{}
		authManagerMock.
			On("GetUser", mock.Anything, token).
			Return(user, nil).
			Times(3)
		handler.AuthManager = authManagerMock
		defer func() { handler.AuthManager = nil }()
		defer authManagerMock.AssertExpectations(t)

		org, err := models.Organizations.Get(ctx)
		require.NoError(t, err)
		assert.Nil(t, org.PaymentCancellationPeriodDays)

		// Custom period
		w := httptest.NewRecorder()
		req, err := createOrganizationProfileMultipartRequest(t, url, "", "", `{"payment_cancellation_period_days": 2}`, new(bytes.Buffer))
		require.NoError(t, err)
		req = req.WithContext(ctx)
		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "updated successfully"}`, string(respBody))

		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(2), *org.PaymentCancellationPeriodDays)

		// Don't update the period
		w = httptest.NewRecorder()
		req, err = createOrganizationProfileMultipartRequest(t, url, "", "", `{"organization_name": "MyOrg"}`, new(bytes.Buffer))
		require.NoError(t, err)
		req = req.WithContext(ctx)
		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp = w.Result()
		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "updated successfully"}`, string(respBody))

		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(2), *org.PaymentCancellationPeriodDays)

		// Back to default period
		w = httptest.NewRecorder()
		req, err = createOrganizationProfileMultipartRequest(t, url, "", "", `{"payment_cancellation_period_days": 0}`, new(bytes.Buffer))
		require.NoError(t, err)
		req = req.WithContext(ctx)
		http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

		resp = w.Result()
		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "updated successfully"}`, string(respBody))

		org, err = models.Organizations.Get(ctx)
		require.NoError(t, err)
		assert.Nil(t, org.PaymentCancellationPeriodDays)

		// validate logs
		require.Contains(t, buf.String(), "[PatchOrganizationProfile] - userID user-id will update the organization fields [PaymentCancellationPeriodDays]")
		require.Contains(t, buf.String(), "[PatchOrganizationProfile] - userID user-id will update the organization fields [Name]")
	})
}

func Test_ProfileHandler_PatchUserProfile(t *testing.T) {
	user := &auth.User{ID: "user-id"}
	testCases := []struct {
		name              string
		token             string
		reqBody           string
		mockAuthManagerFn func(authManagerMock *auth.AuthManagerMock)
		wantStatusCode    int
		wantRespBody      string
	}{
		{
			name:           "returns Unauthorized when no token is found",
			wantStatusCode: http.StatusUnauthorized,
			wantRespBody:   `{"error": "Not authorized."}`,
		},
		{
			name:    "returns BadRequest when the request has an invalid JSON body",
			token:   "token",
			reqBody: `invalid`,
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once()
			},
			wantStatusCode: http.StatusBadRequest,
			wantRespBody:   `{"error": "The request was invalid in some way."}`,
		},
		{
			name:    "returns BadRequest when the request has an invalid email",
			token:   "token",
			reqBody: `{"email": "invalid"}`,
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once()
			},
			wantStatusCode: http.StatusBadRequest,
			wantRespBody: `{
				"error": "The request was invalid in some way.", 
				"extras": {
					"email": "invalid email provided"
				}
			}`,
		},
		{
			name:    "returns BadRequest when the request has an invalid password",
			token:   "token",
			reqBody: `{"password": "invalid"}`,
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once()
			},
			wantStatusCode: http.StatusBadRequest,
			wantRespBody: `{
				"error": "The request was invalid in some way.",
				"extras": {
					"password": {
						"digit": "password must contain at least one numberical digit",
						"length":"password length must be between 12 and 36 characters",
						"special character":"password must contain at least one special character",
						"uppercase":"password must contain at least one uppercase letter"
					}
				}
			}`,
		},
		{
			name:    "returns BadRequest if none of the fields are provided",
			token:   "token",
			reqBody: `{}`,
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once()
			},
			wantStatusCode: http.StatusBadRequest,
			wantRespBody: `{
				"error": "The request was invalid in some way.",
				"extras": {
					"details":"provide at least first_name, last_name, email or password."
				}
			}`,
		},
		{
			name:  "returns InternalServerError when AuthManager fails",
			token: "token",
			reqBody: `{
				"first_name": "First",
				"last_name": "Last",
				"email": "email@email.com",
				"password": "!1Az?2By.3Cx"
			}`,
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once().
					On("UpdateUser", mock.Anything, "token", "First", "Last", "email@email.com", "!1Az?2By.3Cx").
					Return(errors.New("unexpected error")).
					Once()
			},
			wantStatusCode: http.StatusInternalServerError,
			wantRespBody:   `{"error":"Cannot update user profiles"}`,
		},
		{
			name:  "🎉 successfully updates user profile",
			token: "token",
			reqBody: `{
				"first_name": "First",
				"last_name": "Last",
				"email": "email@email.com",
				"password": "!1Az?2By.3Cx"
			}`,
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once().
					On("UpdateUser", mock.Anything, "token", "First", "Last", "email@email.com", "!1Az?2By.3Cx").
					Return(nil).
					Once()
			},
			wantStatusCode: http.StatusOK,
			wantRespBody:   `{"message": "user profile updated successfully"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			buf := new(strings.Builder)
			log.DefaultLogger.SetOutput(buf)
			log.SetLevel(log.InfoLevel)

			// Setup DB
			dbt := dbtest.Open(t)
			defer dbt.Close()
			dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
			require.NoError(t, err)
			defer dbConnectionPool.Close()

			// Inject authenticated token into context:
			ctx := context.Background()
			if tc.token != "" {
				ctx = context.WithValue(ctx, middleware.TokenContextKey, tc.token)
			}

			// Setup password validator
			pwValidator, err := utils.GetPasswordValidatorInstance()
			require.NoError(t, err)

			// Setup handler with mocked dependencies
			handler := &ProfileHandler{PasswordValidator: pwValidator}
			if tc.mockAuthManagerFn != nil {
				authManagerMock := &auth.AuthManagerMock{}
				tc.mockAuthManagerFn(authManagerMock)
				handler.AuthManager = authManagerMock
				defer authManagerMock.AssertExpectations(t)
			}

			// Execute the request
			var body io.Reader
			if tc.reqBody != "" {
				body = strings.NewReader(tc.reqBody)
			}
			w := httptest.NewRecorder()
			req, err := http.NewRequestWithContext(ctx, http.MethodPatch, "/profile/user", body)
			require.NoError(t, err)
			http.HandlerFunc(handler.PatchUserProfile).ServeHTTP(w, req)

			// Assert response
			resp := w.Result()
			assert.Equal(t, tc.wantStatusCode, resp.StatusCode)
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.JSONEq(t, tc.wantRespBody, string(respBody))

			// Validate logs
			if tc.wantStatusCode == http.StatusOK {
				assert.Contains(t, buf.String(), "[PatchUserProfile] - Will update email for userID user-id to ema...com")
				assert.Contains(t, buf.String(), "[PatchUserProfile] - Will update password for userID user-id")
			}
		})
	}
}

func Test_ProfileHandler_PatchUserPassword(t *testing.T) {
	user := &auth.User{ID: "user-id"}
	testCases := []struct {
		name              string
		token             string
		reqBody           string
		mockAuthManagerFn func(authManagerMock *auth.AuthManagerMock)
		wantStatusCode    int
		wantRespBody      string
	}{
		{
			name:           "returns Unauthorized error when no token is found",
			token:          "",
			wantStatusCode: http.StatusUnauthorized,
			wantRespBody:   `{"error": "Not authorized."}`,
		},
		{
			name:    "returns BadRequest error when JSON decoding fails",
			token:   "token",
			reqBody: `invalid`,
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once()
			},
			wantStatusCode: http.StatusBadRequest,
			wantRespBody:   `{"error": "The request was invalid in some way."}`,
		},
		{
			name:    "returns BadRequest error when current_password and new_password are not provided",
			token:   "token",
			reqBody: `{}`,
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once()
			},
			wantStatusCode: http.StatusBadRequest,
			wantRespBody: `{
				"error": "The request was invalid in some way.",
				"extras": {
					"current_password":"current_password is required",
					"new_password":"new_password should be different from current_password"
				}
			}`,
		},
		{
			name:    "returns BadRequest error when current_password and new_password are equal",
			token:   "token",
			reqBody: `{"current_password": "currentpassword", "new_password": "currentpassword"}`,
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once()
			},
			wantStatusCode: http.StatusBadRequest,
			wantRespBody: `{
				"error": "The request was invalid in some way.",
				"extras": {
					"new_password":"new_password should be different from current_password"
				}
			}`,
		},
		{
			name:    "returns BadRequest error when password does not match all the criteria",
			token:   "token",
			reqBody: `{"current_password": "currentpassword", "new_password": "1Az2By3Cx"}`,
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once()
			},
			wantStatusCode: http.StatusBadRequest,
			wantRespBody: `{
				"error": "The request was invalid in some way.",
				"extras": {
					"length":"password length must be between 12 and 36 characters",
					"special character":"password must contain at least one special character"
				}
			}`,
		},
		{
			name:    "returns InternalServerError when AuthManager fails",
			token:   "token",
			reqBody: `{"current_password": "currentpassword", "new_password": "!1Az?2By.3Cx"}`,
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once().
					On("UpdatePassword", mock.Anything, "token", "currentpassword", "!1Az?2By.3Cx").
					Return(errors.New("unexpected error")).
					Once()
			},
			wantStatusCode: http.StatusInternalServerError,
			wantRespBody:   `{"error":"Cannot update user password"}`,
		},
		{
			name:    "🎉 successfully updates the user password",
			token:   "token",
			reqBody: `{"current_password": "currentpassword", "new_password": "!1Az?2By.3Cx"}`,
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once().
					On("UpdatePassword", mock.Anything, "token", "currentpassword", "!1Az?2By.3Cx").
					Return(nil).
					Once()
			},
			wantStatusCode: http.StatusOK,
			wantRespBody:   `{"message": "user password updated successfully"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			buf := new(strings.Builder)
			log.DefaultLogger.SetOutput(buf)
			log.SetLevel(log.InfoLevel)

			// Setup DB
			dbt := dbtest.Open(t)
			defer dbt.Close()
			dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
			require.NoError(t, err)
			defer dbConnectionPool.Close()

			// Inject authenticated token into context:
			ctx := context.Background()
			if tc.token != "" {
				ctx = context.WithValue(ctx, middleware.TokenContextKey, tc.token)
			}

			// Setup password validator
			pwValidator, err := utils.GetPasswordValidatorInstance()
			require.NoError(t, err)

			// Setup handler with mocked dependencies
			handler := &ProfileHandler{PasswordValidator: pwValidator}
			if tc.mockAuthManagerFn != nil {
				authManagerMock := &auth.AuthManagerMock{}
				tc.mockAuthManagerFn(authManagerMock)
				handler.AuthManager = authManagerMock
				defer authManagerMock.AssertExpectations(t)
			}

			// Execute the request
			var body io.Reader
			if tc.reqBody != "" {
				body = strings.NewReader(tc.reqBody)
			}
			w := httptest.NewRecorder()
			req, err := http.NewRequestWithContext(ctx, http.MethodPatch, "/profile/reset-password", body)
			require.NoError(t, err)
			http.HandlerFunc(handler.PatchUserPassword).ServeHTTP(w, req)

			// Assert response
			resp := w.Result()
			assert.Equal(t, tc.wantStatusCode, resp.StatusCode)
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.JSONEq(t, tc.wantRespBody, string(respBody))

			// Validate logs
			if tc.wantStatusCode == http.StatusOK {
				require.Contains(t, buf.String(), "[PatchUserPassword] - Will update password for user account ID user-id")
			}
		})
	}
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
		defer resp.Body.Close()

		wantsBody := fmt.Sprintf(`
			{
				"logo_url": "http://localhost:8000/organization/logo?token=mytoken",
				"name": "MyCustomAid",
				"distribution_account_public_key": %q,
				"timezone_utc_offset": "+00:00",
				"is_approval_required": false,
				"sms_resend_interval": 0,
				"payment_cancellation_period_days": 0
			}
		`, distributionAccountPK)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
	})

	t.Run("returns the sms_registration_message_template and otp_message_template when they aren't the default values", func(t *testing.T) {
		ctx = context.WithValue(ctx, middleware.TokenContextKey, "mytoken")

		msg := "My custom receiver wallet registration invite. MyOrg 👋"
		err := models.Organizations.Update(ctx, &data.OrganizationUpdate{
			SMSRegistrationMessageTemplate: &msg,
		})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)
		http.HandlerFunc(handler.GetOrganizationInfo).ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		wantsBody := fmt.Sprintf(`
			{
				"logo_url": "http://localhost:8000/organization/logo?token=mytoken",
				"name": "MyCustomAid",
				"distribution_account_public_key": %q,
				"timezone_utc_offset": "+00:00",
				"is_approval_required":false,
				"sms_registration_message_template": "My custom receiver wallet registration invite. MyOrg 👋",
				"sms_resend_interval": 0,
				"payment_cancellation_period_days": 0
			}
		`, distributionAccountPK)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))

		msg = "Here's your OTP Code to complete your registration. MyOrg 👋"
		err = models.Organizations.Update(ctx, &data.OrganizationUpdate{
			OTPMessageTemplate: &msg,
		})
		require.NoError(t, err)

		w = httptest.NewRecorder()
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)
		http.HandlerFunc(handler.GetOrganizationInfo).ServeHTTP(w, req)

		resp = w.Result()
		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		wantsBody = fmt.Sprintf(`
			{
				"logo_url": "http://localhost:8000/organization/logo?token=mytoken",
				"name": "MyCustomAid",
				"distribution_account_public_key": %q,
				"timezone_utc_offset": "+00:00",
				"is_approval_required":false,
				"sms_registration_message_template": "My custom receiver wallet registration invite. MyOrg 👋",
				"otp_message_template": "Here's your OTP Code to complete your registration. MyOrg 👋",
				"sms_resend_interval": 0,
				"payment_cancellation_period_days": 0
			}
		`, distributionAccountPK)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
	})

	t.Run("returns the custom sms_resend_interval", func(t *testing.T) {
		resetOrganizationInfo(t, ctx, dbConnectionPool)

		ctx = context.WithValue(ctx, middleware.TokenContextKey, "mytoken")

		var smsResendInterval int64 = 2
		err := models.Organizations.Update(ctx, &data.OrganizationUpdate{
			SMSResendInterval: &smsResendInterval,
		})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)
		http.HandlerFunc(handler.GetOrganizationInfo).ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		wantsBody := fmt.Sprintf(`
			{
				"logo_url": "http://localhost:8000/organization/logo?token=mytoken",
				"name": "MyCustomAid",
				"distribution_account_public_key": %q,
				"timezone_utc_offset": "+00:00",
				"is_approval_required":false,
				"sms_resend_interval": 2,
				"payment_cancellation_period_days": 0
			}
		`, distributionAccountPK)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
	})

	t.Run("returns the custom payment_cancellation_period_days", func(t *testing.T) {
		resetOrganizationInfo(t, ctx, dbConnectionPool)

		ctx = context.WithValue(ctx, middleware.TokenContextKey, "mytoken")

		var paymentCancellationPeriodDays int64 = 5
		err := models.Organizations.Update(ctx, &data.OrganizationUpdate{
			PaymentCancellationPeriodDays: &paymentCancellationPeriodDays,
		})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)
		http.HandlerFunc(handler.GetOrganizationInfo).ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		wantsBody := fmt.Sprintf(`
			{
				"logo_url": "http://localhost:8000/organization/logo?token=mytoken",
				"name": "MyCustomAid",
				"distribution_account_public_key": %q,
				"timezone_utc_offset": "+00:00",
				"is_approval_required":false,
				"sms_resend_interval": 0,
				"payment_cancellation_period_days": 5
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
