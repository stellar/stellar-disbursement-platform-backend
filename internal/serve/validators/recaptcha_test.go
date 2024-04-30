package validators

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	httpclientMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient/mocks"
)

func Test_GoogleReCAPTCHAValidator(t *testing.T) {
	siteSecretKey := "secretKey"
	httpClientMock := &httpclientMocks.HttpClientMock{}

	grv := NewGoogleReCAPTCHAValidator(siteSecretKey, httpClientMock)
	ctx := context.Background()
	const token = "token"

	t.Run("returns error when requesting verify token URL fails", func(t *testing.T) {
		httpClientMock.
			On("Do", mock.AnythingOfType("*http.Request")).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				require.True(t, ok)

				require.Equal(t, verifyTokenURL, req.URL.String())
				require.Equal(t, http.MethodPost, req.Method)
				require.Equal(t, "application/x-www-form-urlencoded", req.Header.Get("Content-Type"))

				reqBody, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				defer req.Body.Close()

				assert.Equal(t, fmt.Sprintf(`secret=%s&response=%s`, siteSecretKey, token), string(reqBody))
			}).
			Return(nil, fmt.Errorf("unexpected error")).
			Once()

		isValid, err := grv.IsTokenValid(ctx, token)

		assert.False(t, isValid)
		assert.EqualError(t, err, "error requesting verify reCAPTCHA token: unexpected error")
		httpClientMock.AssertExpectations(t)
	})

	t.Run("returns error when an error code is returned", func(t *testing.T) {
		respBody := `{
			"success": false,
			"error-codes": [
				"bad-request"
			]
		}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(respBody)),
			StatusCode: http.StatusOK,
		}

		httpClientMock.
			On("Do", mock.AnythingOfType("*http.Request")).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				require.True(t, ok)

				assert.Equal(t, verifyTokenURL, req.URL.String())
				assert.Equal(t, http.MethodPost, req.Method)
				assert.Equal(t, "application/x-www-form-urlencoded", req.Header.Get("Content-Type"))

				reqBody, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				defer req.Body.Close()

				assert.Equal(t, fmt.Sprintf(`secret=%s&response=%s`, siteSecretKey, token), string(reqBody))
			}).
			Return(response, nil).
			Once()

		isValid, err := grv.IsTokenValid(ctx, token)

		assert.False(t, isValid)
		assert.EqualError(t, err, "error returned by verify reCAPTCHA token: [bad-request]")
	})

	t.Run("returns false when timeout-or-duplicate error code is returned", func(t *testing.T) {
		respBody := `{
			"success": false,
			"error-codes": [
				"bad-request",
				"timeout-or-duplicate"
			]
		}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(respBody)),
			StatusCode: http.StatusOK,
		}

		httpClientMock.
			On("Do", mock.AnythingOfType("*http.Request")).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				require.True(t, ok)

				assert.Equal(t, verifyTokenURL, req.URL.String())
				assert.Equal(t, http.MethodPost, req.Method)
				assert.Equal(t, "application/x-www-form-urlencoded", req.Header.Get("Content-Type"))

				reqBody, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				defer req.Body.Close()

				assert.Equal(t, fmt.Sprintf(`secret=%s&response=%s`, siteSecretKey, token), string(reqBody))
			}).
			Return(response, nil).
			Once()

		isValid, err := grv.IsTokenValid(ctx, token)

		assert.False(t, isValid)
		assert.NoError(t, err)
	})

	t.Run("returns whether the token is invalid or not", func(t *testing.T) {
		response1 := &http.Response{
			Body:       io.NopCloser(strings.NewReader(`{"success": false}`)),
			StatusCode: http.StatusOK,
		}

		httpClientMock.
			On("Do", mock.AnythingOfType("*http.Request")).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				require.True(t, ok)

				assert.Equal(t, verifyTokenURL, req.URL.String())
				assert.Equal(t, http.MethodPost, req.Method)
				assert.Equal(t, "application/x-www-form-urlencoded", req.Header.Get("Content-Type"))

				reqBody, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				defer req.Body.Close()

				assert.Equal(t, fmt.Sprintf(`secret=%s&response=%s`, siteSecretKey, token), string(reqBody))
			}).
			Return(response1, nil).
			Once()

		isValid, err := grv.IsTokenValid(ctx, token)
		assert.NoError(t, err)
		assert.False(t, isValid)

		// Token is valid
		response2 := &http.Response{
			Body:       io.NopCloser(strings.NewReader(`{"success": true}`)),
			StatusCode: http.StatusOK,
		}

		httpClientMock.
			On("Do", mock.AnythingOfType("*http.Request")).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				require.True(t, ok)

				assert.Equal(t, verifyTokenURL, req.URL.String())
				assert.Equal(t, http.MethodPost, req.Method)
				assert.Equal(t, "application/x-www-form-urlencoded", req.Header.Get("Content-Type"))

				reqBody, rErr := io.ReadAll(req.Body)
				require.NoError(t, rErr)
				defer req.Body.Close()

				assert.Equal(t, fmt.Sprintf(`secret=%s&response=%s`, siteSecretKey, token), string(reqBody))
			}).
			Return(response2, nil).
			Once()

		isValid, err = grv.IsTokenValid(ctx, token)
		assert.NoError(t, err)
		assert.True(t, isValid)
	})
}
