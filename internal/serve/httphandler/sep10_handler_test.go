package httphandler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

func Test_SEP10Handler_GetChallenge(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name               string
		queryParams        map[string]string
		mockSetup          func(mService *services.MockSEP10Service)
		expectedStatusCode int
		checkResponse      func(t *testing.T, body []byte)
	}{
		{
			name:               "missing account parameter",
			queryParams:        map[string]string{},
			expectedStatusCode: http.StatusBadRequest,
			checkResponse: func(t *testing.T, body []byte) {
				var errResp httperror.HTTPError
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)
				assert.Equal(t, "account is required", errResp.Message)
			},
		},
		{
			name: "invalid account format",
			queryParams: map[string]string{
				"account": "invalid_account",
			},
			expectedStatusCode: http.StatusBadRequest,
			checkResponse: func(t *testing.T, body []byte) {
				var errResp httperror.HTTPError
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)
				assert.Equal(t, "invalid account format", errResp.Message)
			},
		},
		{
			name: "successful challenge creation",
			queryParams: map[string]string{
				"account":       "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
				"memo":          "12345",
				"home_domain":   "example.com",
				"client_domain": "wallet.example.com",
			},
			mockSetup: func(mService *services.MockSEP10Service) {
				mService.On("CreateChallenge", mock.Anything, services.ChallengeRequest{
					Account:      "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
					Memo:         "12345",
					HomeDomain:   "example.com",
					ClientDomain: "wallet.example.com",
				}).Return(&services.ChallengeResponse{
					Transaction:       "challenge_xdr_here",
					NetworkPassphrase: "Test SDF Network ; September 2015",
				}, nil).Once()
			},
			expectedStatusCode: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var resp services.ChallengeResponse
				err := json.Unmarshal(body, &resp)
				require.NoError(t, err)
				assert.Equal(t, "challenge_xdr_here", resp.Transaction)
				assert.Equal(t, "Test SDF Network ; September 2015", resp.NetworkPassphrase)
			},
		},
		{
			name: "service returns error",
			queryParams: map[string]string{
				"account": "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			},
			mockSetup: func(mService *services.MockSEP10Service) {
				mService.On("CreateChallenge", mock.Anything, mock.Anything).
					Return(nil, fmt.Errorf("internal service error")).Once()
			},
			expectedStatusCode: http.StatusInternalServerError,
			checkResponse: func(t *testing.T, body []byte) {
				var errResp httperror.HTTPError
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)
				assert.Equal(t, "Failed to create challenge", errResp.Message)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mService := services.NewMockSEP10Service(t)
			if tc.mockSetup != nil {
				tc.mockSetup(mService)
			}

			handler := SEP10Handler{
				SEP10Service: mService,
			}

			u, err := url.Parse("/auth")
			require.NoError(t, err)
			q := u.Query()
			for k, v := range tc.queryParams {
				q.Set(k, v)
			}
			u.RawQuery = q.Encode()

			req := httptest.NewRequest(http.MethodGet, u.String(), nil)
			req = req.WithContext(ctx)
			rr := httptest.NewRecorder()

			handler.GetChallenge(rr, req)

			resp := rr.Result()
			defer resp.Body.Close()

			assert.Equal(t, tc.expectedStatusCode, resp.StatusCode)

			if tc.checkResponse != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tc.checkResponse(t, body)
			}
		})
	}
}

func Test_SEP10Handler_PostChallenge(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name               string
		body               any
		mockSetup          func(mService *services.MockSEP10Service)
		expectedStatusCode int
		checkResponse      func(t *testing.T, body []byte)
	}{
		{
			name:               "missing transaction",
			body:               map[string]string{},
			expectedStatusCode: http.StatusBadRequest,
			checkResponse: func(t *testing.T, body []byte) {
				var errResp httperror.HTTPError
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)
				assert.Equal(t, "transaction is required", errResp.Message)
			},
		},
		{
			name:               "invalid JSON body",
			body:               "invalid json",
			expectedStatusCode: http.StatusBadRequest,
			checkResponse: func(t *testing.T, body []byte) {
				var errResp httperror.HTTPError
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)
				assert.Equal(t, "invalid request body", errResp.Message)
			},
		},
		{
			name: "successful validation",
			body: map[string]string{
				"transaction": "valid_xdr_transaction",
			},
			mockSetup: func(mService *services.MockSEP10Service) {
				mService.On("ValidateChallenge", mock.Anything, services.ValidationRequest{
					Transaction: "valid_xdr_transaction",
				}).Return(&services.ValidationResponse{
					Token: "jwt_token_here",
				}, nil).Once()
			},
			expectedStatusCode: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var resp services.ValidationResponse
				err := json.Unmarshal(body, &resp)
				require.NoError(t, err)
				assert.Equal(t, "jwt_token_here", resp.Token)
			},
		},
		{
			name: "validation error",
			body: map[string]string{
				"transaction": "invalid_xdr",
			},
			mockSetup: func(mService *services.MockSEP10Service) {
				mService.On("ValidateChallenge", mock.Anything, mock.Anything).
					Return(nil, fmt.Errorf("invalid signature")).Once()
			},
			expectedStatusCode: http.StatusBadRequest,
			checkResponse: func(t *testing.T, body []byte) {
				var errResp httperror.HTTPError
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)
				assert.Equal(t, "challenge validation failed", errResp.Message)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mService := services.NewMockSEP10Service(t)
			if tc.mockSetup != nil {
				tc.mockSetup(mService)
			}

			handler := SEP10Handler{
				SEP10Service: mService,
			}

			var reqBody io.Reader
			if str, ok := tc.body.(string); ok {
				reqBody = strings.NewReader(str)
			} else {
				jsonBody, err := json.Marshal(tc.body)
				require.NoError(t, err)
				reqBody = bytes.NewReader(jsonBody)
			}

			req := httptest.NewRequest(http.MethodPost, "/auth", reqBody)
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(ctx)
			rr := httptest.NewRecorder()

			handler.PostChallenge(rr, req)

			resp := rr.Result()
			defer resp.Body.Close()

			assert.Equal(t, tc.expectedStatusCode, resp.StatusCode)

			if tc.checkResponse != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tc.checkResponse(t, body)
			}
		})
	}
}
