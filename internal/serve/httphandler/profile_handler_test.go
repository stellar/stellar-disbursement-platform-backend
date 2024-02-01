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
	"reflect"
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

func createOrganizationProfileMultipartRequest(t *testing.T, ctx context.Context, url, fieldName, filename, body string, fileContent io.Reader) *http.Request {
	buf := new(bytes.Buffer)
	writer := multipart.NewWriter(buf)
	defer writer.Close()

	if fieldName == "" {
		fieldName = "logo"
	}

	if fileContent == nil {
		fileContent = new(bytes.Buffer)
	}

	// Insert file into the Multipart form
	part, err := writer.CreateFormFile(fieldName, filename)
	require.NoError(t, err)
	_, err = io.Copy(part, fileContent)
	require.NoError(t, err)

	// Insert JSON body into the Multipart form
	err = writer.WriteField("data", body)
	require.NoError(t, err)

	// Create the request
	req, err := http.NewRequest(http.MethodPatch, url, buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req.WithContext(ctx)
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

func Test_ProfileHandler_PatchOrganizationProfile_Failures(t *testing.T) {
	// PNG file
	pngImg := data.CreateMockImage(t, 300, 300, data.ImageSizeSmall)
	pngImgBuf := new(bytes.Buffer)
	err := png.Encode(pngImgBuf, pngImg)
	require.NoError(t, err)

	// CSV file
	csvBuf := new(bytes.Buffer)
	csvWriter := csv.NewWriter(csvBuf)
	err = csvWriter.WriteAll([][]string{
		{"name", "age"},
		{"foo", "99"},
		{"bar", "99"},
	})
	require.NoError(t, err)

	// JPEG too big
	imgTooBig := data.CreateMockImage(t, 3840, 2160, data.ImageSizeMedium)
	imgTooBigBuf := new(bytes.Buffer)
	err = jpeg.Encode(imgTooBigBuf, imgTooBig, &jpeg.Options{Quality: jpeg.DefaultQuality})
	require.NoError(t, err)

	url := "/profile/organization"
	user := &auth.User{ID: "user-id"}
	testCases := []struct {
		name              string
		token             string
		getRequestFn      func(t *testing.T, ctx context.Context) *http.Request
		mockAuthManagerFn func(authManagerMock *auth.AuthManagerMock)
		wantStatusCode    int
		wantRespBody      string
	}{
		{
			name: "returns Unauthorized when no token is found",
			getRequestFn: func(t *testing.T, ctx context.Context) *http.Request {
				return httptest.NewRequest(http.MethodPatch, url, nil).WithContext(ctx)
			},
			wantStatusCode: http.StatusUnauthorized,
			wantRespBody:   `{"error": "Not authorized."}`,
		},
		{
			name:  "returns BadRequest when the request is not valid (invalid JSON)",
			token: "token",
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once()
			},
			getRequestFn: func(t *testing.T, ctx context.Context) *http.Request {
				return createOrganizationProfileMultipartRequest(t, ctx, url, "logo", "logo.png", `invalid`, pngImgBuf)
			},
			wantStatusCode: http.StatusBadRequest,
			wantRespBody:   `{"error": "The request was invalid in some way."}`,
		},
		{
			name:  "returns BadRequest when the request is not valid (invalid file format)",
			token: "token",
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once()
			},
			getRequestFn: func(t *testing.T, ctx context.Context) *http.Request {
				return createOrganizationProfileMultipartRequest(t, ctx, url, "logo", "logo.csv", `{}`, csvBuf)
			},
			wantStatusCode: http.StatusBadRequest,
			wantRespBody: `{
				"error": "The request was invalid in some way.",
				"extras": {
					"logo": "invalid file type provided. Expected png or jpeg."
				}
			}`,
		},
		{
			name:  "returns BadRequest when the request is not valid (both file and data are empty)",
			token: "token",
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once()
			},
			getRequestFn: func(t *testing.T, ctx context.Context) *http.Request {
				return createOrganizationProfileMultipartRequest(t, ctx, url, "invalidParameterName", "logo.csv", `{}`, pngImgBuf)
			},
			wantStatusCode: http.StatusBadRequest,
			wantRespBody: `{
				"error": "request is invalid",
				"extras": {
					"details": "data or logo is required"
				}
			}`,
		},
		{
			name:  "returns BadRequest error when the request size is too large",
			token: "token",
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once()
			},
			getRequestFn: func(t *testing.T, ctx context.Context) *http.Request {
				return createOrganizationProfileMultipartRequest(t, ctx, url, "logo", "logo.png", `{}`, imgTooBigBuf)
			},
			wantStatusCode: http.StatusBadRequest,
			wantRespBody: `{
				"error": "could not parse multipart form data",
				"extras": {
					"details": "request too large. Max size 2MB."
				}
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
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
			handler := &ProfileHandler{MaxMemoryAllocation: 1024 * 1024, PasswordValidator: pwValidator}
			if tc.mockAuthManagerFn != nil {
				authManagerMock := &auth.AuthManagerMock{}
				tc.mockAuthManagerFn(authManagerMock)
				handler.AuthManager = authManagerMock
				defer authManagerMock.AssertExpectations(t)
			}

			// Execute the request
			req := tc.getRequestFn(t, ctx)
			w := httptest.NewRecorder()
			http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

			// Assert response
			resp := w.Result()
			assert.Equal(t, tc.wantStatusCode, resp.StatusCode)
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.JSONEq(t, tc.wantRespBody, string(respBody))
		})
	}
}

func Test_ProfileHandler_PatchOrganizationProfile_Successful(t *testing.T) {
	// PNG file
	newPNGImgBuf := func() *bytes.Buffer {
		pngImg := data.CreateMockImage(t, 300, 300, data.ImageSizeSmall)
		pngImgBuf := new(bytes.Buffer)
		err := png.Encode(pngImgBuf, pngImg)
		require.NoError(t, err)
		return pngImgBuf
	}

	var nilInt64 *int64

	// JPEG file
	jpegImg := data.CreateMockImage(t, 300, 300, data.ImageSizeSmall)
	jpegImgBuf := new(bytes.Buffer)
	err := jpeg.Encode(jpegImgBuf, jpegImg, &jpeg.Options{Quality: jpeg.DefaultQuality})
	require.NoError(t, err)

	url := "/profile/organization"
	user := &auth.User{ID: "user-id"}
	testCases := []struct {
		name                     string
		token                    string
		updateOrgInitialValuesFn func(t *testing.T, ctx context.Context, models *data.Models)
		getRequestFn             func(t *testing.T, ctx context.Context) *http.Request
		mockAuthManagerFn        func(authManagerMock *auth.AuthManagerMock)
		resultingFieldsToCompare map[string]interface{}
		wantLogEntries           []string
	}{
		{
			name:  "🎉 successfully updates the organization's logo (PNG)",
			token: "token",
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once()
			},
			getRequestFn: func(t *testing.T, ctx context.Context) *http.Request {
				return createOrganizationProfileMultipartRequest(t, ctx, url, "logo", "logo.png", `{}`, newPNGImgBuf())
			},
			resultingFieldsToCompare: map[string]interface{}{
				"Logo": newPNGImgBuf().Bytes(),
			},
			wantLogEntries: []string{"[PatchOrganizationProfile] - userID user-id will update the organization fields [Logo='...']"},
		},
		{
			name:  "🎉 successfully updates the organization's logo (JPEG)",
			token: "token",
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once()
			},
			getRequestFn: func(t *testing.T, ctx context.Context) *http.Request {
				return createOrganizationProfileMultipartRequest(t, ctx, url, "logo", "logo.jpeg", `{}`, jpegImgBuf)
			},
			resultingFieldsToCompare: map[string]interface{}{
				"Logo": jpegImgBuf.Bytes(),
			},
			wantLogEntries: []string{"[PatchOrganizationProfile] - userID user-id will update the organization fields [Logo='...']"},
		},
		{
			name:  "🎉 successfully updates ALL the organization fields",
			token: "token",
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once()
			},
			getRequestFn: func(t *testing.T, ctx context.Context) *http.Request {
				reqBody := `{
					"is_approval_required": true,
					"organization_name": "My Org Name",
					"otp_message_template": "Here's your OTP Code to complete your registration. MyOrg 👋",
					"payment_cancellation_period_days": 2,
					"sms_registration_message_template": "My custom receiver wallet registration invite. MyOrg 👋",
					"sms_resend_interval": 2,
					"timezone_utc_offset": "-03:00"
				}`
				return createOrganizationProfileMultipartRequest(t, ctx, url, "logo", "logo.png", reqBody, newPNGImgBuf())
			},
			resultingFieldsToCompare: map[string]interface{}{
				"IsApprovalRequired":             true,
				"Name":                           "My Org Name",
				"Logo":                           newPNGImgBuf().Bytes(),
				"OTPMessageTemplate":             "Here's your OTP Code to complete your registration. MyOrg 👋",
				"PaymentCancellationPeriodDays":  int64(2),
				"SMSRegistrationMessageTemplate": "My custom receiver wallet registration invite. MyOrg 👋",
				"SMSResendInterval":              int64(2),
				"TimezoneUTCOffset":              "-03:00",
			},
			wantLogEntries: []string{"[PatchOrganizationProfile] - userID user-id will update the organization fields [IsApprovalRequired='true', Logo='...', Name='My Org Name', OTPMessageTemplate='Here's your OTP Code to complete your registration. MyOrg 👋', PaymentCancellationPeriodDays='2', SMSRegistrationMessageTemplate='My custom receiver wallet registration invite. MyOrg 👋', SMSResendInterval='2', TimezoneUTCOffset='-03:00']"},
		},
		{
			name:  "🎉 successfully updates organization back to its default values",
			token: "token",
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once()
			},
			updateOrgInitialValuesFn: func(t *testing.T, ctx context.Context, models *data.Models) {
				otpMessageTemplate := "custom OTPMessageTemplate"
				smsRegistrationMessageTemplate := "custom SMSRegistrationMessageTemplate"
				smsResendInterval := int64(123)
				paymentCancellationPeriodDays := int64(456)
				err := models.Organizations.Update(ctx, &data.OrganizationUpdate{
					SMSRegistrationMessageTemplate: &smsRegistrationMessageTemplate,
					OTPMessageTemplate:             &otpMessageTemplate,
					SMSResendInterval:              &smsResendInterval,
					PaymentCancellationPeriodDays:  &paymentCancellationPeriodDays,
				})
				require.NoError(t, err)
			},
			getRequestFn: func(t *testing.T, ctx context.Context) *http.Request {
				reqBody := `{
					"sms_registration_message_template": "",
					"otp_message_template": "",
					"sms_resend_interval": 0,
					"payment_cancellation_period_days": 0
				}`
				return createOrganizationProfileMultipartRequest(t, ctx, url, "", "", reqBody, new(bytes.Buffer))
			},
			resultingFieldsToCompare: map[string]interface{}{
				"SMSRegistrationMessageTemplate": "You have a payment waiting for you from the {{.OrganizationName}}. Click {{.RegistrationLink}} to register.",
				"OTPMessageTemplate":             "{{.OTP}} is your {{.OrganizationName}} phone verification code.",
				"SMSResendInterval":              nilInt64,
				"PaymentCancellationPeriodDays":  nilInt64,
			},
			wantLogEntries: []string{"[PatchOrganizationProfile] - userID user-id will update the organization fields [OTPMessageTemplate='', PaymentCancellationPeriodDays='0', SMSRegistrationMessageTemplate='', SMSResendInterval='0']"},
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
			models, err := data.NewModels(dbConnectionPool)
			require.NoError(t, err)

			// Inject authenticated token into context:
			ctx := context.Background()
			if tc.token != "" {
				ctx = context.WithValue(ctx, middleware.TokenContextKey, tc.token)
			}

			// Assert DB before
			if tc.updateOrgInitialValuesFn != nil {
				tc.updateOrgInitialValuesFn(t, ctx, models)
			}
			org, err := models.Organizations.Get(ctx)
			require.NoError(t, err)
			for k, expectedValue := range tc.resultingFieldsToCompare {
				fieldValue := reflect.ValueOf(org).Elem().FieldByName(k)
				if fieldValue.Kind() == reflect.Ptr && !fieldValue.IsNil() {
					fieldValue = fieldValue.Elem()
				}
				assert.NotEqual(t, expectedValue, fieldValue.Interface())
			}

			// Setup password validator
			pwValidator, err := utils.GetPasswordValidatorInstance()
			require.NoError(t, err)

			// Setup handler with mocked dependencies
			handler := &ProfileHandler{Models: models, MaxMemoryAllocation: 1024 * 1024, PasswordValidator: pwValidator}
			if tc.mockAuthManagerFn != nil {
				authManagerMock := &auth.AuthManagerMock{}
				tc.mockAuthManagerFn(authManagerMock)
				handler.AuthManager = authManagerMock
				defer authManagerMock.AssertExpectations(t)
			}

			// Execute the request
			req := tc.getRequestFn(t, ctx)
			w := httptest.NewRecorder()
			http.HandlerFunc(handler.PatchOrganizationProfile).ServeHTTP(w, req)

			// Assert response
			resp := w.Result()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.JSONEq(t, `{"message": "updated successfully"}`, string(respBody))

			// Assert DB after
			org, err = models.Organizations.Get(ctx)
			require.NoError(t, err)
			for k, expectedValue := range tc.resultingFieldsToCompare {
				fieldValue := reflect.ValueOf(org).Elem().FieldByName(k)
				if fieldValue.Kind() == reflect.Ptr && !fieldValue.IsNil() {
					fieldValue = fieldValue.Elem()
				}
				assert.Equal(t, expectedValue, fieldValue.Interface())
			}

			// Assert logs
			for _, logEntry := range tc.wantLogEntries {
				require.Contains(t, buf.String(), logEntry)
			}
		})
	}
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
		wantLogEntries    []string
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
					"details":"provide at least first_name, last_name or email."
				}
			}`,
		},
		{
			name:  "returns InternalServerError when AuthManager fails",
			token: "token",
			reqBody: `{
				"first_name": "First",
				"last_name": "Last",
				"email": "email@email.com"
			}`,
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once().
					On("UpdateUser", mock.Anything, "token", "First", "Last", "email@email.com", "").
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
				"email": "email@email.com"
			}`,
			mockAuthManagerFn: func(authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(user, nil).
					Once().
					On("UpdateUser", mock.Anything, "token", "First", "Last", "email@email.com", "").
					Return(nil).
					Once()
			},
			wantStatusCode: http.StatusOK,
			wantRespBody:   `{"message": "user profile updated successfully"}`,
			wantLogEntries: []string{
				"[PatchUserProfile] - Will update email for userID user-id to ema...com",
			},
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
			for _, logEntry := range tc.wantLogEntries {
				assert.Contains(t, buf.String(), logEntry)
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
		wantLogEntries    []string
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
			wantLogEntries: []string{
				"[PatchUserPassword] - Will update password for user account ID user-id",
			},
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
			for _, logEntry := range tc.wantLogEntries {
				assert.Contains(t, buf.String(), logEntry)
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
