package validators

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_GoogleReCAPTCHAValidator(t *testing.T) {
	siteSecretKey := "secretKey"
	httpClientMock := &httpClientMock{}

	grv := NewGoogleReCAPTCHAValidator(siteSecretKey, httpClientMock)

	ctx := context.Background()
	t.Run("returns error when requesting verify token URL fails", func(t *testing.T) {
		token := "token"

		httpClientMock.mockDo = func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, verifyTokenURL, req.URL.String())
			assert.Equal(t, http.MethodPost, req.Method)
			assert.Equal(t, "application/x-www-form-urlencoded", req.Header.Get("Content-Type"))

			reqBody, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			defer req.Body.Close()

			assert.Equal(t, fmt.Sprintf(`secret=%s&response=%s`, siteSecretKey, token), string(reqBody))

			return &http.Response{
				Body:       io.NopCloser(strings.NewReader("{}")),
				StatusCode: http.StatusOK,
			}, fmt.Errorf("unexpected error")
		}

		isValid, err := grv.IsTokenValid(ctx, token)

		assert.False(t, isValid)
		assert.EqualError(t, err, "error requesting verify reCAPTCHA token: unexpected error")
	})

	t.Run("returns error when an error code is returned", func(t *testing.T) {
		token := "token"

		httpClientMock.mockDo = func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, verifyTokenURL, req.URL.String())
			assert.Equal(t, http.MethodPost, req.Method)
			assert.Equal(t, "application/x-www-form-urlencoded", req.Header.Get("Content-Type"))

			reqBody, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			defer req.Body.Close()

			assert.Equal(t, fmt.Sprintf(`secret=%s&response=%s`, siteSecretKey, token), string(reqBody))

			respBody := `
				{
					"success": false,
					"error-codes": [
						"bad-request"
					]
				}
			`

			return &http.Response{
				Body:       io.NopCloser(strings.NewReader(respBody)),
				StatusCode: http.StatusOK,
			}, nil
		}

		isValid, err := grv.IsTokenValid(ctx, token)

		assert.False(t, isValid)
		assert.EqualError(t, err, "error returned by verify reCAPTCHA token: [bad-request]")
	})

	t.Run("returns false when timeout-or-duplicate error code is returned", func(t *testing.T) {
		token := "token"

		httpClientMock.mockDo = func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, verifyTokenURL, req.URL.String())
			assert.Equal(t, http.MethodPost, req.Method)
			assert.Equal(t, "application/x-www-form-urlencoded", req.Header.Get("Content-Type"))

			reqBody, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			defer req.Body.Close()

			assert.Equal(t, fmt.Sprintf(`secret=%s&response=%s`, siteSecretKey, token), string(reqBody))

			respBody := `
				{
					"success": false,
					"error-codes": [
						"bad-request",
						"timeout-or-duplicate"
					]
				}
			`

			return &http.Response{
				Body:       io.NopCloser(strings.NewReader(respBody)),
				StatusCode: http.StatusOK,
			}, nil
		}

		isValid, err := grv.IsTokenValid(ctx, token)

		assert.False(t, isValid)
		assert.NoError(t, err)
	})

	t.Run("returns whether the token is invalid or not", func(t *testing.T) {
		token := "token"

		// Token invalid
		httpClientMock.mockDo = func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, verifyTokenURL, req.URL.String())
			assert.Equal(t, http.MethodPost, req.Method)
			assert.Equal(t, "application/x-www-form-urlencoded", req.Header.Get("Content-Type"))

			reqBody, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			defer req.Body.Close()

			assert.Equal(t, fmt.Sprintf(`secret=%s&response=%s`, siteSecretKey, token), string(reqBody))

			respBody := `{"success": false}`

			return &http.Response{
				Body:       io.NopCloser(strings.NewReader(respBody)),
				StatusCode: http.StatusOK,
			}, nil
		}

		isValid, err := grv.IsTokenValid(ctx, token)

		assert.False(t, isValid)
		assert.NoError(t, err)

		// Token is valid
		httpClientMock.mockDo = func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, verifyTokenURL, req.URL.String())
			assert.Equal(t, http.MethodPost, req.Method)
			assert.Equal(t, "application/x-www-form-urlencoded", req.Header.Get("Content-Type"))

			reqBody, rErr := io.ReadAll(req.Body)
			require.NoError(t, rErr)
			defer req.Body.Close()

			assert.Equal(t, fmt.Sprintf(`secret=%s&response=%s`, siteSecretKey, token), string(reqBody))

			respBody := `{"success": true}`

			return &http.Response{
				Body:       io.NopCloser(strings.NewReader(respBody)),
				StatusCode: http.StatusOK,
			}, nil
		}

		isValid, err = grv.IsTokenValid(ctx, token)

		assert.True(t, isValid)
		assert.NoError(t, err)
	})
}
