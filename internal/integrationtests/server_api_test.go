package integrationtests

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

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	httpclientMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httphandler"
)

func Test_Login(t *testing.T) {
	httpClientMock := httpclientMocks.HTTPClientMock{}

	sa := ServerAPIIntegrationTests{
		HTTPClient:       &httpClientMock,
		ServerAPIBaseURL: "http://mock_server.com/",
		UserEmail:        "user_mock@email.com",
		UserPassword:     "userPass123",
	}

	ctx := context.Background()

	t.Run("error calling httpClient.Do", func(t *testing.T) {
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(nil, fmt.Errorf("error calling the request")).Once()
		at, err := sa.Login(ctx)
		require.EqualError(t, err, "error making request to server API post LOGIN: error calling the request")
		assert.Empty(t, at)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("error trying to login on server api", func(t *testing.T) {
		loginResponse := `{Invalid credentials.}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(loginResponse)),
			StatusCode: http.StatusBadRequest,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		at, err := sa.Login(ctx)
		require.EqualError(t, err, "error trying to login on the server API")
		assert.Empty(t, at)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("error invalid response body", func(t *testing.T) {
		loginResponse := ``
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(loginResponse)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		at, err := sa.Login(ctx)
		require.EqualError(t, err, "error decoding response body: EOF")
		assert.Empty(t, at)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("successfully logging on server api", func(t *testing.T) {
		loginResponse := `{
			"token": "valid_token"
		}`

		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(loginResponse)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		at, err := sa.Login(ctx)
		require.NoError(t, err)

		assert.Equal(t, "valid_token", at.Token)

		httpClientMock.AssertExpectations(t)
	})
}

func Test_CreateDisbursement(t *testing.T) {
	httpClientMock := httpclientMocks.HTTPClientMock{}

	sa := ServerAPIIntegrationTests{
		HTTPClient:       &httpClientMock,
		ServerAPIBaseURL: "http://mock_server.com/",
	}

	ctx := context.Background()

	authToken := &ServerAPIAuthToken{
		Token: "valid_token",
	}

	reqBody := &httphandler.PostDisbursementRequest{
		Name:     "mockDisbursement",
		WalletID: "123",
		AssetID:  "890",
	}

	t.Run("error calling httpClient.Do", func(t *testing.T) {
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(nil, fmt.Errorf("error calling the request")).Once()
		d, err := sa.CreateDisbursement(ctx, authToken, reqBody)
		require.EqualError(t, err, "error making request to server API post DISBURSEMENT: error calling the request")
		assert.Empty(t, d)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("error trying to create a disbursement on server api", func(t *testing.T) {
		disbursementResponse := `{Invalid credentials.}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(disbursementResponse)),
			StatusCode: http.StatusBadRequest,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		d, err := sa.CreateDisbursement(ctx, authToken, reqBody)
		require.EqualError(t, err, "trying to create a new disbursement on the server API")
		assert.Empty(t, d)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("error invalid response body", func(t *testing.T) {
		disbursementResponse := ``
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(disbursementResponse)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		d, err := sa.CreateDisbursement(ctx, authToken, reqBody)
		require.EqualError(t, err, "error decoding response body: EOF")
		assert.Empty(t, d)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("succesfully creating a disbursement on server api", func(t *testing.T) {
		disbursementResponse := `{
                "id": "619da857-8725-4c58-933d-c120a458e0f5",
                "name": "mockDisbursement",
                "status": "DRAFT"
		}`

		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(disbursementResponse)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		d, err := sa.CreateDisbursement(ctx, authToken, reqBody)
		require.NoError(t, err)

		assert.Equal(t, "mockDisbursement", d.Name)
		assert.Equal(t, "619da857-8725-4c58-933d-c120a458e0f5", d.ID)
		assert.Equal(t, "DRAFT", string(d.Status))

		httpClientMock.AssertExpectations(t)
	})
}

func Test_ProcessDisbursement(t *testing.T) {
	httpClientMock := httpclientMocks.HTTPClientMock{}

	sa := ServerAPIIntegrationTests{
		HTTPClient:              &httpClientMock,
		ServerAPIBaseURL:        "http://mock_server.com/",
		DisbursementCSVFilePath: "resources",
		DisbursementCSVFileName: "disbursement_instructions_phone.csv",
	}

	ctx := context.Background()

	authToken := &ServerAPIAuthToken{
		Token: "valid_token",
	}

	mockDisbursementID := "disbursement_id"

	t.Run("error calling httpClient.Do", func(t *testing.T) {
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(nil, fmt.Errorf("error calling the request")).Once()
		err := sa.ProcessDisbursement(ctx, authToken, mockDisbursementID)
		require.EqualError(t, err, "error making request to server API post DISBURSEMENT INSTRUCTIONS: error calling the request")

		httpClientMock.AssertExpectations(t)
	})

	t.Run("error trying to process the disbursement on server api", func(t *testing.T) {
		disbursementResponse := `{error processing disbursement.}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(disbursementResponse)),
			StatusCode: http.StatusBadRequest,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		err := sa.ProcessDisbursement(ctx, authToken, mockDisbursementID)
		require.EqualError(t, err, "error trying to process the disbursement CSV file on the server API")

		httpClientMock.AssertExpectations(t)
	})

	t.Run("succesfully creating a disbursement on server api", func(t *testing.T) {
		disbursementResponse := `{
			"message": "File uploaded successfully"
		}`

		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(disbursementResponse)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		err := sa.ProcessDisbursement(ctx, authToken, mockDisbursementID)
		require.NoError(t, err)

		httpClientMock.AssertExpectations(t)
	})
}

func Test_StartDisbursement(t *testing.T) {
	httpClientMock := httpclientMocks.HTTPClientMock{}

	sa := ServerAPIIntegrationTests{
		HTTPClient:       &httpClientMock,
		ServerAPIBaseURL: "http://mock_server.com/",
	}

	ctx := context.Background()

	authToken := &ServerAPIAuthToken{
		Token: "valid_token",
	}

	mockDisbursementID := "disbursement_id"
	reqBody := &httphandler.PatchDisbursementStatusRequest{
		Status: "STARTED",
	}

	t.Run("error calling httpClient.Do", func(t *testing.T) {
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(nil, fmt.Errorf("error calling the request")).Once()
		err := sa.StartDisbursement(ctx, authToken, mockDisbursementID, reqBody)
		require.EqualError(t, err, "error making request to server API patch DISBURSEMENT: error calling the request")

		httpClientMock.AssertExpectations(t)
	})

	t.Run("error trying to start the disbursement on server api", func(t *testing.T) {
		disbursementResponse := `{error starting disbursement.}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(disbursementResponse)),
			StatusCode: http.StatusBadRequest,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		err := sa.StartDisbursement(ctx, authToken, mockDisbursementID, reqBody)
		require.EqualError(t, err, "error trying to start the disbursement on the server API (statusCode=400)")

		httpClientMock.AssertExpectations(t)
	})

	t.Run("succesfully creating a disbursement on server api", func(t *testing.T) {
		disbursementResponse := `{
			"message": "Disbursement started"
		}`

		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(disbursementResponse)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		err := sa.StartDisbursement(ctx, authToken, mockDisbursementID, reqBody)
		require.NoError(t, err)

		httpClientMock.AssertExpectations(t)
	})
}

func Test_ReceiverRegistration(t *testing.T) {
	httpClientMock := httpclientMocks.HTTPClientMock{}

	sa := ServerAPIIntegrationTests{
		HTTPClient:       &httpClientMock,
		ServerAPIBaseURL: "http://mock_server.com/",
	}

	ctx := context.Background()

	authToken := &SEP24AuthToken{
		Token: "valid_token",
	}

	reqBody := &data.ReceiverRegistrationRequest{
		PhoneNumber:       "+18554212274",
		OTP:               "123456",
		VerificationValue: "1999-01-30",
		VerificationField: "date_of_birth",
		ReCAPTCHAToken:    "captchtoken",
	}

	t.Run("error calling httpClient.Do", func(t *testing.T) {
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(nil, fmt.Errorf("error calling the request")).Once()
		err := sa.ReceiverRegistration(ctx, authToken, reqBody)
		require.EqualError(t, err, "error making request to server API post WALLET REGISTRATION VERIFICATION: error calling the request")

		httpClientMock.AssertExpectations(t)
	})

	t.Run("error trying to registrate receiver on server api", func(t *testing.T) {
		disbursementResponse := `{error registrating receiver.}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(disbursementResponse)),
			StatusCode: http.StatusBadRequest,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		err := sa.ReceiverRegistration(ctx, authToken, reqBody)

		require.EqualError(t, err, "trying to complete receiver registration on the server API (statusCode=400)")

		httpClientMock.AssertExpectations(t)
	})

	t.Run("succesfully registrating receiver on server api", func(t *testing.T) {
		disbursementResponse := `{
			"message": "ok"
		}`

		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(disbursementResponse)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		err := sa.ReceiverRegistration(ctx, authToken, reqBody)

		require.NoError(t, err)

		httpClientMock.AssertExpectations(t)
	})
}

func Test_CreateEmbeddedWallet(t *testing.T) {
	httpClientMock := httpclientMocks.HTTPClientMock{}

	sa := ServerAPIIntegrationTests{
		HTTPClient:       &httpClientMock,
		ServerAPIBaseURL: "http://mock_server.com/",
		TenantName:       "test-tenant",
	}

	ctx := context.Background()

	reqBody := &httphandler.CreateWalletRequest{
		Token:        "test-token-123",
		PublicKey:    "04" + strings.Repeat("ab", 64), // 65 bytes hex-encoded P256 public key
		CredentialID: "credential-id-123",
	}

	t.Run("error calling httpClient.Do", func(t *testing.T) {
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(nil, fmt.Errorf("error calling the request")).Once()
		wallet, err := sa.CreateEmbeddedWallet(ctx, reqBody)
		require.EqualError(t, err, "making request to POST /embedded-wallets: error calling the request")
		assert.Nil(t, wallet)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("error registering embedded wallet on server api", func(t *testing.T) {
		errorResponse := `{"error": "Invalid token"}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(errorResponse)),
			StatusCode: http.StatusBadRequest,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		wallet, err := sa.CreateEmbeddedWallet(ctx, reqBody)
		require.EqualError(t, err, "error registering embedded wallet (statusCode=400)")
		assert.Nil(t, wallet)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("error invalid response body", func(t *testing.T) {
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader("")),
			StatusCode: http.StatusAccepted,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		wallet, err := sa.CreateEmbeddedWallet(ctx, reqBody)
		require.EqualError(t, err, "decoding response body: EOF")
		assert.Nil(t, wallet)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("successfully registering embedded wallet", func(t *testing.T) {
		walletResponse := `{
			"status": "PENDING"
		}`

		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(walletResponse)),
			StatusCode: http.StatusAccepted,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		wallet, err := sa.CreateEmbeddedWallet(ctx, reqBody)
		require.NoError(t, err)
		assert.Equal(t, data.PendingWalletStatus, wallet.Status)
		assert.Empty(t, wallet.ContractAddress)

		httpClientMock.AssertExpectations(t)
	})
}
