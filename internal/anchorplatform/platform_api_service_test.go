package anchorplatform

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_NewAnchorPlatformAPIService(t *testing.T) {
	testCases := []struct {
		name                          string
		httpClient                    httpclient.HttpClientInterface
		anchorPlatformBasePlatformURL string
		anchorPlatformOutgoingJWT     string
		wantErrContains               string
	}{
		{
			name:                          "returns error when http client is nil",
			httpClient:                    nil,
			anchorPlatformBasePlatformURL: "",
			wantErrContains:               "http client cannot be nil",
		},
		{
			name:                          "returns error when anchor platform base platform url is empty",
			httpClient:                    &http.Client{},
			anchorPlatformBasePlatformURL: "",
			wantErrContains:               "anchor platform base platform url cannot be empty",
		},
		{
			name:                          "returns error when anchor platform outgoing jwt secret is empty",
			httpClient:                    &http.Client{},
			anchorPlatformBasePlatformURL: "https://test.com",
			anchorPlatformOutgoingJWT:     "",
			wantErrContains:               "anchor platform outgoing jwt secret cannot be empty",
		},
		{
			name:                          "returns error when jwt manager cannot be created due to a small jwt secret",
			httpClient:                    &http.Client{},
			anchorPlatformBasePlatformURL: "https://test.com",
			anchorPlatformOutgoingJWT:     "small",
			wantErrContains:               "creating jwt manager: secret is required to have at least 12 characteres",
		},
		{
			name:                          "🎉 successfully creates Anchor Platform API service",
			httpClient:                    &http.Client{},
			anchorPlatformBasePlatformURL: "https://test.com",
			anchorPlatformOutgoingJWT:     "jwt_secret_1234567890",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			apService, err := NewAnchorPlatformAPIService(tc.httpClient, tc.anchorPlatformBasePlatformURL, tc.anchorPlatformOutgoingJWT)
			if tc.wantErrContains == "" {
				require.NoError(t, err)
				require.NotNil(t, apService)
			} else {
				require.EqualError(t, err, tc.wantErrContains)
				require.Nil(t, apService)
			}
		})
	}
}

func Test_updateAnchorTransactions(t *testing.T) {
	httpClientMock := httpclient.HttpClientMock{}
	anchorPlatformAPIService, err := NewAnchorPlatformAPIService(&httpClientMock, "http://mock_anchor.com/", "jwt_secret_1234567890")
	require.NoError(t, err)
	ctx := context.Background()

	apTxPatch := APSep24TransactionPatch{
		ID:     "test-transaction-id",
		Status: "pending_anchor",
		SEP:    "24",
	}
	mockMatchedByFn := func(req *http.Request) bool {
		assert := assert.New(t)

		// STEP 1: check method type and URL
		assert.Equal("PATCH", req.Method)
		assert.Equal("http://mock_anchor.com/transactions", req.URL.String())

		// STEP2: check request body
		bodyBytes, err := io.ReadAll(req.Body)
		assert.NoError(err)
		// Restore the io.ReadCloser to its original state
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		expectedJSON := `{
				"records": [
					{
						"transaction": {
							"id": "test-transaction-id",
							"status": "pending_anchor",
							"sep": "24"
						}
					}
				]
			}`
		assert.JSONEq(expectedJSON, string(bodyBytes))

		return true
	}

	t.Run("error calling httpClient.Do", func(t *testing.T) {
		httpClientMock.On("Do", mock.MatchedBy(mockMatchedByFn)).Return(nil, fmt.Errorf("error calling the request")).Once()
		err := anchorPlatformAPIService.updateAnchorTransactions(ctx, apTxPatch)
		require.EqualError(t, err, "error making request to anchor platform: error calling the request")

		httpClientMock.AssertExpectations(t)
	})

	t.Run("error trying to update transactions on anchor platform", func(t *testing.T) {
		transactionResponse := `{The 'id' of the transaction first determined to be invalid.}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(transactionResponse)),
			StatusCode: http.StatusBadRequest,
		}
		httpClientMock.On("Do", mock.MatchedBy(mockMatchedByFn)).Return(response, nil).Once()

		err := anchorPlatformAPIService.updateAnchorTransactions(ctx, apTxPatch)
		require.EqualError(t, err, "error updating transaction on anchor platform, response.StatusCode: 400")

		httpClientMock.AssertExpectations(t)
	})

	t.Run("succesfully update transaction on anchor platform", func(t *testing.T) {
		transactionResponse := `{
			"transaction":{
				"id": "test-transaction-id",
				"status": "pending_anchor",
				"sep": "24",
				"kind": "deposit",
				"destination_account": "stellar_address",
				"kyc_verified": true,
			}
		}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(transactionResponse)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.On("Do", mock.MatchedBy(mockMatchedByFn)).Return(response, nil).Once()

		err := anchorPlatformAPIService.updateAnchorTransactions(ctx, apTxPatch)
		require.NoError(t, err)

		httpClientMock.AssertExpectations(t)
	})
}

func Test_PatchAnchorTransactionsPostRegistration(t *testing.T) {
	httpClientMock := httpclient.HttpClientMock{}
	anchorPlatformAPIService, err := NewAnchorPlatformAPIService(&httpClientMock, "http://mock_anchor.com/", "jwt_secret_1234567890")
	require.NoError(t, err)
	ctx := context.Background()

	t.Run("succesfully update transaction on anchor platform", func(t *testing.T) {
		transactionResponse := `{
			"transaction":{
				"id": "test-transaction-id",
				"status": "pending_anchor",
				"sep": "24",
				"kind": "deposit",
				"destination_account": "stellar_address",
				"kyc_verified": true,
			}
		}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(transactionResponse)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.On("Do", mock.MatchedBy(func(req *http.Request) bool {
			assert := assert.New(t)

			// STEP 1: check method type and URL
			assert.Equal("PATCH", req.Method)
			assert.Equal("http://mock_anchor.com/transactions", req.URL.String())

			// STEP2: check request body
			bodyBytes, err := io.ReadAll(req.Body)
			assert.NoError(err)
			// Restore the io.ReadCloser to its original state
			req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			expectedJSON := `{
				"records": [
					{
						"transaction": {
							"id": "test-transaction-id",
							"status": "pending_anchor",
							"sep": "24"
						}
					}
				]
			}`
			assert.JSONEq(expectedJSON, string(bodyBytes))

			return true
		})).Return(response, nil).Twice()

		// PatchAnchorTransactionsPostRegistration and updateAnchorTransactions should result in the same request:
		err := anchorPlatformAPIService.PatchAnchorTransactionsPostRegistration(ctx, APSep24TransactionPatchPostRegistration{
			ID:     "test-transaction-id",
			Status: "pending_anchor",
			SEP:    "24",
		})
		require.NoError(t, err)

		err = anchorPlatformAPIService.updateAnchorTransactions(ctx, APSep24TransactionPatch{
			ID:     "test-transaction-id",
			Status: "pending_anchor",
			SEP:    "24",
		})
		require.NoError(t, err)

		httpClientMock.AssertExpectations(t)
	})
}

func Test_getAnchorTransactions(t *testing.T) {
	httpClientMock := httpclient.HttpClientMock{}
	apService, err := NewAnchorPlatformAPIService(&httpClientMock, "https://test.com/", "jwt_secret_1234567890")
	require.NoError(t, err)
	ctx := context.Background()

	t.Run("error calling httpClient.Do", func(t *testing.T) {
		httpClientMock.
			On("Do", mock.AnythingOfType("*http.Request")).
			Return(nil, fmt.Errorf("error calling the request")).
			Once()

		resp, err := apService.getAnchorTransactions(ctx, false, GetTransactionsQueryParams{})
		require.EqualError(t, err, "making getAnchorTransactions request to anchor platform: error calling the request")
		require.Nil(t, resp)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("🎉 successfully return the http response to the caller even when a non 2xx is returned", func(t *testing.T) {
		wantBody := `{"error": "authentication is required."}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(wantBody)),
			StatusCode: http.StatusUnauthorized,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		resp, err := apService.getAnchorTransactions(ctx, false, GetTransactionsQueryParams{})
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.JSONEq(t, wantBody, string(body))

		httpClientMock.AssertExpectations(t)
	})

	t.Run("🎉 successfully return the http response to the caller when a 2xx is returned", func(t *testing.T) {
		wantBody := `{"records": []}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(wantBody)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		resp, err := apService.getAnchorTransactions(ctx, false, GetTransactionsQueryParams{})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.JSONEq(t, wantBody, string(body))

		httpClientMock.AssertExpectations(t)
	})

	t.Run("🎉 successfully validate authentication ON/OFF", func(t *testing.T) {
		wantBody := `{"records": []}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(wantBody)),
			StatusCode: http.StatusOK,
		}

		// authentication ON:
		httpClientMock.
			On("Do", mock.AnythingOfType("*http.Request")).
			Return(response, nil).
			Run(func(args mock.Arguments) {
				gotRequest, ok := args.Get(0).(*http.Request)
				require.True(t, ok)
				require.NotNil(t, gotRequest)
				require.Equal(t, "https://test.com/transactions?sep=24", gotRequest.URL.String())
				require.Equal(t, "GET", gotRequest.Method)
				require.Equal(t, "application/json", gotRequest.Header.Get("Content-Type"))
				require.True(t, strings.HasPrefix(gotRequest.Header.Get("Authorization"), "Bearer "))
			}).Once()

		_, err := apService.getAnchorTransactions(ctx, false, GetTransactionsQueryParams{SEP: "24"})
		require.NoError(t, err)

		// authentication OFF:
		httpClientMock.
			On("Do", mock.AnythingOfType("*http.Request")).
			Return(response, nil).
			Run(func(args mock.Arguments) {
				gotRequest, ok := args.Get(0).(*http.Request)
				require.True(t, ok)
				require.NotNil(t, gotRequest)
				require.Equal(t, "https://test.com/transactions?sep=24", gotRequest.URL.String())
				require.Equal(t, "GET", gotRequest.Method)
				require.Equal(t, "application/json", gotRequest.Header.Get("Content-Type"))
				require.Empty(t, gotRequest.Header.Get("Authorization"))
			}).Once()

		_, err = apService.getAnchorTransactions(ctx, true, GetTransactionsQueryParams{SEP: "24"})
		require.NoError(t, err)

		httpClientMock.AssertExpectations(t)
	})
}

func Test_IsAnchorProtectedByAuth(t *testing.T) {
	httpClientMock := httpclient.HttpClientMock{}
	anchorPlatformAPIService, err := NewAnchorPlatformAPIService(&httpClientMock, "https://test.com/", "jwt_secret_1234567890")
	require.NoError(t, err)
	ctx := context.Background()

	t.Run("error calling httpClient.Do", func(t *testing.T) {
		httpClientMock.
			On("Do", mock.AnythingOfType("*http.Request")).
			Return(nil, fmt.Errorf("error calling the request")).
			Once()

		isProtected, err := anchorPlatformAPIService.IsAnchorProtectedByAuth(ctx)
		require.EqualError(t, err, "getting anchor transactions from platform API: making getAnchorTransactions request to anchor platform: error calling the request")
		require.False(t, isProtected)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("🎉 successfully identifies an unprotected 🔴 anchor platform server", func(t *testing.T) {
		wantBody := `{"records": []}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(wantBody)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.
			On("Do", mock.AnythingOfType("*http.Request")).
			Return(response, nil).
			Run(func(args mock.Arguments) {
				gotRequest, ok := args.Get(0).(*http.Request)
				require.True(t, ok)
				require.NotNil(t, gotRequest)
				require.Equal(t, "https://test.com/transactions?sep=24", gotRequest.URL.String())
				require.Equal(t, "GET", gotRequest.Method)
				require.Equal(t, "application/json", gotRequest.Header.Get("Content-Type"))
				require.Empty(t, gotRequest.Header.Get("Authorization"))
			}).
			Once()

		isProtected, err := anchorPlatformAPIService.IsAnchorProtectedByAuth(ctx)
		require.NoError(t, err)
		require.False(t, isProtected)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("🎉 successfully identifies a protected 🟢 anchor platform server", func(t *testing.T) {
		wantBody := `{"error": "authentication is required."}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(wantBody)),
			StatusCode: http.StatusUnauthorized,
		}
		httpClientMock.
			On("Do", mock.AnythingOfType("*http.Request")).
			Return(response, nil).
			Run(func(args mock.Arguments) {
				gotRequest, ok := args.Get(0).(*http.Request)
				require.True(t, ok)
				require.NotNil(t, gotRequest)
				require.Equal(t, "https://test.com/transactions?sep=24", gotRequest.URL.String())
				require.Equal(t, "GET", gotRequest.Method)
				require.Equal(t, "application/json", gotRequest.Header.Get("Content-Type"))
				require.Empty(t, gotRequest.Header.Get("Authorization"))
			}).
			Once()

		isProtected, err := anchorPlatformAPIService.IsAnchorProtectedByAuth(ctx)
		require.NoError(t, err)
		require.True(t, isProtected)

		httpClientMock.AssertExpectations(t)
	})
}

func Test_GetJWTToken(t *testing.T) {
	t.Run("returns ErrJWTSecretNotSet when a JWT secret is not set", func(t *testing.T) {
		apService := AnchorPlatformAPIService{}
		transactions := []APSep24TransactionPatch{{ID: "1"}, {ID: "2"}}
		token, err := apService.GetJWTToken(transactions...)
		require.ErrorIs(t, err, ErrJWTManagerNotSet)
		require.Empty(t, token)
	})

	t.Run("returns token successfully 🎉", func(t *testing.T) {
		jwtManager, err := NewJWTManager("1234567890ab", 5000)
		require.NoError(t, err)

		apService := AnchorPlatformAPIService{jwtManager: jwtManager}
		transactions := []APSep24TransactionPatch{{ID: "1"}, {ID: "2"}}
		token, err := apService.GetJWTToken(transactions...)
		require.NoError(t, err)
		require.NotEmpty(t, token)

		// verify the token
		claims, err := jwtManager.ParseDefaultTokenClaims(token)
		require.NoError(t, err)
		assert.Nil(t, claims.Valid())
		assert.Equal(t, "1,2", claims.ID)
		assert.Equal(t, "stellar-disbursement-platform-backend", claims.Subject)
	})

	t.Run("returns token successfully (zero transactions passed) 🎉", func(t *testing.T) {
		jwtManager, err := NewJWTManager("1234567890ab", 5000)
		require.NoError(t, err)

		apService := AnchorPlatformAPIService{jwtManager: jwtManager}
		token, err := apService.GetJWTToken()
		require.NoError(t, err)
		require.NotEmpty(t, token)

		// verify the token
		claims, err := jwtManager.ParseDefaultTokenClaims(token)
		require.NoError(t, err)
		assert.Nil(t, claims.Valid())
		assert.Empty(t, claims.ID)
		assert.Equal(t, "stellar-disbursement-platform-backend", claims.Subject)
	})
}
