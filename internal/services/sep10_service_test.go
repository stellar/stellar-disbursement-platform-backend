package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/txnbuild"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient/mocks"
)

type testKeypairs struct {
	client       *keypair.Full
	server       *keypair.Full
	clientDomain *keypair.Full
}

func newTestKeypairs() *testKeypairs {
	return &testKeypairs{
		client:       keypair.MustRandom(),
		server:       keypair.MustRandom(),
		clientDomain: keypair.MustRandom(),
	}
}

func createMockHorizonClient(accountID string, thresholds horizon.AccountThresholds, signers []horizon.Signer) *horizonclient.MockClient {
	mockClient := &horizonclient.MockClient{}
	account := horizon.Account{
		AccountID:  accountID,
		Thresholds: thresholds,
	}
	if len(signers) > 0 {
		account.Signers = signers
	}
	mockClient.On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).Return(account, nil)
	return mockClient
}

func createMockHTTPClient(t *testing.T, clientDomainKP *keypair.Full) *mocks.HttpClientMock {
	mockClient := mocks.NewHttpClientMock(t)
	mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(&http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("SIGNING_KEY = \"" + clientDomainKP.Address() + "\"\n")),
	}, nil)
	return mockClient
}

func createSEP10Service(t *testing.T, kps *testKeypairs, baseURL string, jwtManager *anchorplatform.JWTManager, setupHTTPMock bool) (*sep10Service, error) {
	if jwtManager == nil {
		jwtManager = &anchorplatform.JWTManager{}
	}

	service, err := NewSEP10Service(
		jwtManager,
		"Test SDF Network ; September 2015",
		kps.server.Seed(),
		baseURL,
		&data.Models{},
		true,
		createMockHorizonClient(kps.client.Address(), horizon.AccountThresholds{MedThreshold: 1}, []horizon.Signer{
			{Key: kps.client.Address(), Weight: 1, Type: "ed25519_public_key"},
		}),
	)
	if err != nil {
		return nil, err
	}

	sep10Service := service.(*sep10Service)
	if setupHTTPMock {
		sep10Service.HTTPClient = createMockHTTPClient(t, kps.clientDomain)
	}
	return sep10Service, nil
}

func assertTransactionStructure(t *testing.T, tx *txnbuild.Transaction, kps *testKeypairs, expectedHomeDomain, expectedClientDomain string) {
	operations := tx.Operations()
	assert.Len(t, operations, 3, "should have exactly 3 operations")

	authOp, ok := operations[0].(*txnbuild.ManageData)
	assert.True(t, ok, "first operation should be ManageData")
	assert.Equal(t, expectedHomeDomain+" auth", authOp.Name)
	assert.Equal(t, kps.client.Address(), authOp.SourceAccount)
	assert.Len(t, authOp.Value, 64, "auth operation value should be 64 bytes base64")

	webAuthOp, ok := operations[1].(*txnbuild.ManageData)
	assert.True(t, ok, "second operation should be ManageData")
	assert.Equal(t, "web_auth_domain", webAuthOp.Name)
	assert.Equal(t, expectedHomeDomain, string(webAuthOp.Value))
	assert.Equal(t, kps.server.Address(), webAuthOp.SourceAccount)

	clientDomainOp, ok := operations[2].(*txnbuild.ManageData)
	assert.True(t, ok, "third operation should be ManageData")
	assert.Equal(t, "client_domain", clientDomainOp.Name)
	assert.Equal(t, expectedClientDomain, string(clientDomainOp.Value))
	assert.Equal(t, kps.clientDomain.Address(), clientDomainOp.SourceAccount)
}

func assertTransactionProperties(t *testing.T, tx *txnbuild.Transaction, kps *testKeypairs) {
	assert.Equal(t, kps.server.Address(), tx.SourceAccount().AccountID, "transaction source account should be server account")
	assert.Equal(t, int64(0), tx.SourceAccount().Sequence, "sequence number should be 0")
	assert.NotNil(t, tx.Timebounds(), "transaction should have timebounds")
	assert.NotEqual(t, txnbuild.TimeoutInfinite, tx.Timebounds().MaxTime, "max time should not be infinite")
}

func createSignedChallenge(t *testing.T, service *sep10Service, kps *testKeypairs, homeDomain, clientDomain string) string {
	challengeReq := ChallengeRequest{
		Account:      kps.client.Address(),
		HomeDomain:   homeDomain,
		ClientDomain: clientDomain,
	}

	challengeResp, err := service.CreateChallenge(context.Background(), challengeReq)
	require.NoError(t, err)
	require.NotEmpty(t, challengeResp.Transaction)

	parsed, err := txnbuild.TransactionFromXDR(challengeResp.Transaction)
	require.NoError(t, err)

	tx, isSimple := parsed.Transaction()
	require.True(t, isSimple)

	signedTx, err := tx.Sign("Test SDF Network ; September 2015", kps.client)
	require.NoError(t, err)

	signedTxBase64, err := signedTx.Base64()
	require.NoError(t, err)
	return signedTxBase64
}

func TestChallengeRequest_Validate(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name        string
		req         ChallengeRequest
		expectError bool
		errMsg      string
	}{
		{
			name: "valid request",
			req: ChallengeRequest{
				Account:      keypair.MustRandom().Address(),
				HomeDomain:   "fenris.imperium.com",
				ClientDomain: "chaos.cadia.com",
			},
			expectError: false,
		},
		{
			name: "valid request with memo",
			req: ChallengeRequest{
				Account:      keypair.MustRandom().Address(),
				HomeDomain:   "fenris.imperium.com",
				ClientDomain: "chaos.cadia.com",
				Memo:         "12345",
			},
			expectError: false,
		},
		{
			name: "empty account",
			req: ChallengeRequest{
				Account:    "",
				HomeDomain: "fenris.imperium.com",
			},
			expectError: true,
			errMsg:      "account is required",
		},
		{
			name: "invalid account",
			req: ChallengeRequest{
				Account:    "invalid-account",
				HomeDomain: "fenris.imperium.com",
			},
			expectError: true,
			errMsg:      "invalid account not a valid ed25519 public key",
		},
		{
			name: "missing client_domain",
			req: ChallengeRequest{
				Account:    keypair.MustRandom().Address(),
				HomeDomain: "fenris.imperium.com",
			},
			expectError: true,
			errMsg:      "client_domain is required",
		},
		{
			name: "invalid memo",
			req: ChallengeRequest{
				Account:      keypair.MustRandom().Address(),
				HomeDomain:   "fenris.imperium.com",
				ClientDomain: "chaos.cadia.com",
				Memo:         "invalid-memo",
			},
			expectError: true,
			errMsg:      "invalid memo must be a positive integer",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSEP10Service_CreateChallenge(t *testing.T) {
	kps := newTestKeypairs()

	testCases := []struct {
		name          string
		req           ChallengeRequest
		expectError   bool
		errMsg        string
		checkResponse func(t *testing.T, resp *ChallengeResponse)
	}{
		{
			name: "valid challenge request",
			req: ChallengeRequest{
				Account:      kps.client.Address(),
				HomeDomain:   "ultramar.imperium.com",
				ClientDomain: "chaos.cadia.com",
			},
			expectError: false,
			checkResponse: func(t *testing.T, resp *ChallengeResponse) {
				assert.NotEmpty(t, resp.Transaction)
				assert.Equal(t, "Test SDF Network ; September 2015", resp.NetworkPassphrase)

				parsed, err := txnbuild.TransactionFromXDR(resp.Transaction)
				require.NoError(t, err)

				tx, isSimple := parsed.Transaction()
				require.True(t, isSimple, "transaction should be a simple transaction")

				assertTransactionProperties(t, tx, kps)
				assertTransactionStructure(t, tx, kps, "ultramar.imperium.com", "chaos.cadia.com")
			},
		},
		{
			name: "valid challenge with memo",
			req: ChallengeRequest{
				Account:      kps.client.Address(),
				HomeDomain:   "ultramar.imperium.com",
				ClientDomain: "chaos.cadia.com",
				Memo:         "12345",
			},
			expectError: false,
			checkResponse: func(t *testing.T, resp *ChallengeResponse) {
				assert.NotEmpty(t, resp.Transaction)
				assert.Equal(t, "Test SDF Network ; September 2015", resp.NetworkPassphrase)

				parsed, err := txnbuild.TransactionFromXDR(resp.Transaction)
				require.NoError(t, err)

				tx, isSimple := parsed.Transaction()
				require.True(t, isSimple, "transaction should be a simple transaction")

				assert.NotNil(t, tx.Memo(), "transaction should have memo")
				memoID, ok := tx.Memo().(txnbuild.MemoID)
				assert.True(t, ok, "memo should be MemoID type")
				assert.Equal(t, int64(12345), int64(memoID), "memo value should match")

				assertTransactionProperties(t, tx, kps)
				assertTransactionStructure(t, tx, kps, "ultramar.imperium.com", "chaos.cadia.com")
			},
		},
		{
			name: "valid challenge with subdomain home domain",
			req: ChallengeRequest{
				Account:      kps.client.Address(),
				HomeDomain:   "api.ultramar.imperium.com",
				ClientDomain: "chaos.cadia.com",
			},
			expectError: false,
			checkResponse: func(t *testing.T, resp *ChallengeResponse) {
				assert.NotEmpty(t, resp.Transaction)
				assert.Equal(t, "Test SDF Network ; September 2015", resp.NetworkPassphrase)

				parsed, err := txnbuild.TransactionFromXDR(resp.Transaction)
				require.NoError(t, err)

				tx, isSimple := parsed.Transaction()
				require.True(t, isSimple, "transaction should be a simple transaction")

				assertTransactionProperties(t, tx, kps)
				operations := tx.Operations()
				authOp, ok := operations[0].(*txnbuild.ManageData)
				assert.True(t, ok, "first operation should be ManageData")
				assert.Equal(t, "api.ultramar.imperium.com auth", authOp.Name)
			},
		},
		{
			name: "invalid account",
			req: ChallengeRequest{
				Account:      "invalid-account",
				HomeDomain:   "ultramar.imperium.com",
				ClientDomain: "chaos.cadia.com",
			},
			expectError: true,
			errMsg:      "invalid-account is not a valid account id",
		},
		{
			name: "invalid home domain",
			req: ChallengeRequest{
				Account:      kps.client.Address(),
				HomeDomain:   "chaos.galaxy.com",
				ClientDomain: "chaos.cadia.com",
			},
			expectError: true,
			errMsg:      "invalid home_domain must match ultramar.imperium.com",
		},
		{
			name: "invalid signing key from client domain",
			req: ChallengeRequest{
				Account:      kps.client.Address(),
				HomeDomain:   "ultramar.imperium.com",
				ClientDomain: "invalid-domain.com",
			},
			expectError: true,
			errMsg:      "fetching client domain signing key",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			service, err := createSEP10Service(t, kps, "https://ultramar.imperium.com", nil, !tc.expectError)
			require.NoError(t, err)

			resp, err := service.CreateChallenge(context.Background(), tc.req)

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
				assert.Nil(t, resp)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, resp)
				if tc.checkResponse != nil {
					tc.checkResponse(t, resp)
				}
			}
		})
	}
}

func TestSEP10Service_CreateChallenge_TimeoutValidation(t *testing.T) {
	kps := newTestKeypairs()

	service, err := createSEP10Service(t, kps, "https://ultramar.imperium.com", nil, true)
	require.NoError(t, err)

	service.AuthTimeout = time.Millisecond

	req := ChallengeRequest{
		Account:      kps.client.Address(),
		HomeDomain:   "ultramar.imperium.com",
		ClientDomain: "chaos.cadia.com",
	}

	resp, err := service.CreateChallenge(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provided timebound must be at least 1s")
	assert.Nil(t, resp)
}

func TestSEP10Service_ValidateChallenge(t *testing.T) {
	t.Parallel()
	kps := newTestKeypairs()

	jwtManager, err := anchorplatform.NewJWTManager("emperors-light-123", 3600000)
	require.NoError(t, err)

	service, err := createSEP10Service(t, kps, "https://stellar.local:8000", jwtManager, false)
	require.NoError(t, err)

	t.Run("valid challenge validation", func(t *testing.T) {
		service.HTTPClient = createMockHTTPClient(t, kps.clientDomain)

		signedTxBase64 := createSignedChallenge(t, service, kps, "stellar.local:8000", "chaos.cadia.com")

		validationReq := ValidationRequest{Transaction: signedTxBase64}
		validationResp, err := service.ValidateChallenge(context.Background(), validationReq)
		require.NoError(t, err)
		assert.NotNil(t, validationResp)
		assert.NotEmpty(t, validationResp.Token)
	})

	t.Run("valid challenge validation with memo", func(t *testing.T) {
		service.HTTPClient = createMockHTTPClient(t, kps.clientDomain)

		challengeReq := ChallengeRequest{
			Account:      kps.client.Address(),
			HomeDomain:   "stellar.local:8000",
			ClientDomain: "chaos.cadia.com",
			Memo:         "54321",
		}

		challengeResp, err := service.CreateChallenge(context.Background(), challengeReq)
		require.NoError(t, err)
		require.NotEmpty(t, challengeResp.Transaction)

		parsed, err := txnbuild.TransactionFromXDR(challengeResp.Transaction)
		require.NoError(t, err)
		tx, isSimple := parsed.Transaction()
		require.True(t, isSimple)

		signedTx, err := tx.Sign("Test SDF Network ; September 2015", kps.client)
		require.NoError(t, err)

		signedTxBase64, err := signedTx.Base64()
		require.NoError(t, err)

		validationReq := ValidationRequest{Transaction: signedTxBase64}
		validationResp, err := service.ValidateChallenge(context.Background(), validationReq)
		require.NoError(t, err)
		assert.NotNil(t, validationResp)
		assert.NotEmpty(t, validationResp.Token)
	})

	t.Run("valid challenge validation with different client domain", func(t *testing.T) {
		service.HTTPClient = createMockHTTPClient(t, kps.clientDomain)

		signedTxBase64 := createSignedChallenge(t, service, kps, "stellar.local:8000", "orks.waaagh.com")

		validationReq := ValidationRequest{Transaction: signedTxBase64}
		validationResp, err := service.ValidateChallenge(context.Background(), validationReq)
		require.NoError(t, err)
		assert.NotNil(t, validationResp)
		assert.NotEmpty(t, validationResp.Token)
	})

	t.Run("invalid transaction", func(t *testing.T) {
		req := ValidationRequest{Transaction: "invalid-transaction"}
		_, err := service.ValidateChallenge(context.Background(), req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not parse challenge")
	})

	t.Run("unsigned transaction", func(t *testing.T) {
		service.HTTPClient = createMockHTTPClient(t, kps.clientDomain)

		challengeReq := ChallengeRequest{
			Account:      kps.client.Address(),
			HomeDomain:   "stellar.local:8000",
			ClientDomain: "chaos.cadia.com",
		}

		challengeResp, err := service.CreateChallenge(context.Background(), challengeReq)
		require.NoError(t, err)
		require.NotEmpty(t, challengeResp.Transaction)

		validationReq := ValidationRequest{Transaction: challengeResp.Transaction}
		_, err = service.ValidateChallenge(context.Background(), validationReq)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "verifying client signature")
	})

	t.Run("wrong client signature", func(t *testing.T) {
		service.HTTPClient = createMockHTTPClient(t, kps.clientDomain)

		challengeReq := ChallengeRequest{
			Account:      kps.client.Address(),
			HomeDomain:   "stellar.local:8000",
			ClientDomain: "chaos.cadia.com",
		}

		challengeResp, err := service.CreateChallenge(context.Background(), challengeReq)
		require.NoError(t, err)
		require.NotEmpty(t, challengeResp.Transaction)

		parsed, err := txnbuild.TransactionFromXDR(challengeResp.Transaction)
		require.NoError(t, err)
		tx, isSimple := parsed.Transaction()
		require.True(t, isSimple)

		wrongKP := keypair.MustRandom()
		signedTx, err := tx.Sign("Test SDF Network ; September 2015", wrongKP)
		require.NoError(t, err)

		signedTxBase64, err := signedTx.Base64()
		require.NoError(t, err)

		validationReq := ValidationRequest{Transaction: signedTxBase64}
		_, err = service.ValidateChallenge(context.Background(), validationReq)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "verifying client signature")
	})

	t.Run("expired transaction", func(t *testing.T) {
		pastTime := time.Now().UTC().Add(-time.Hour)
		expiredTime := pastTime.Add(time.Minute * 15)

		sa := txnbuild.SimpleAccount{
			AccountID: kps.server.Address(),
			Sequence:  -1,
		}

		randomNonce := make([]byte, 48)
		_, err := rand.Read(randomNonce)
		require.NoError(t, err)
		randomNonceB64 := base64.StdEncoding.EncodeToString(randomNonce)

		operations := []txnbuild.Operation{
			&txnbuild.ManageData{
				SourceAccount: kps.client.Address(),
				Name:          "stellar.local:8000 auth",
				Value:         []byte(randomNonceB64),
			},
			&txnbuild.ManageData{
				SourceAccount: kps.server.Address(),
				Name:          "web_auth_domain",
				Value:         []byte("stellar.local:8000"),
			},
			&txnbuild.ManageData{
				SourceAccount: kps.clientDomain.Address(),
				Name:          "client_domain",
				Value:         []byte("chaos.cadia.com"),
			},
		}

		txParams := txnbuild.TransactionParams{
			SourceAccount:        &sa,
			IncrementSequenceNum: true,
			Operations:           operations,
			BaseFee:              txnbuild.MinBaseFee,
			Preconditions: txnbuild.Preconditions{
				TimeBounds: txnbuild.NewTimebounds(pastTime.Unix(), expiredTime.Unix()),
			},
		}

		tx, err := txnbuild.NewTransaction(txParams)
		require.NoError(t, err)

		signedTx, err := tx.Sign("Test SDF Network ; September 2015", kps.server)
		require.NoError(t, err)
		signedTx, err = signedTx.Sign("Test SDF Network ; September 2015", kps.client)
		require.NoError(t, err)

		signedTxBase64, err := signedTx.Base64()
		require.NoError(t, err)

		validationReq := ValidationRequest{Transaction: signedTxBase64}
		_, err = service.ValidateChallenge(context.Background(), validationReq)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not within range of the specified timebounds")
	})

	t.Run("empty transaction", func(t *testing.T) {
		req := ValidationRequest{Transaction: ""}
		_, err := service.ValidateChallenge(context.Background(), req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not parse challenge")
	})

	t.Run("insufficient signature threshold", func(t *testing.T) {
		highThresholdKP := keypair.MustRandom()
		highThresholdService, err := createSEP10Service(t, &testKeypairs{
			client:       highThresholdKP,
			server:       kps.server,
			clientDomain: kps.clientDomain,
		}, "https://stellar.local:8000", jwtManager, false)
		require.NoError(t, err)

		highThresholdService.HorizonClient = createMockHorizonClient(
			highThresholdKP.Address(),
			horizon.AccountThresholds{MedThreshold: 3},
			[]horizon.Signer{
				{Key: highThresholdKP.Address(), Weight: 1, Type: "ed25519_public_key"},
			},
		)
		highThresholdService.HTTPClient = createMockHTTPClient(t, kps.clientDomain)

		challengeReq := ChallengeRequest{
			Account:      highThresholdKP.Address(),
			HomeDomain:   "stellar.local:8000",
			ClientDomain: "chaos.cadia.com",
		}

		challengeResp, err := highThresholdService.CreateChallenge(context.Background(), challengeReq)
		require.NoError(t, err)
		require.NotEmpty(t, challengeResp.Transaction)

		parsed, err := txnbuild.TransactionFromXDR(challengeResp.Transaction)
		require.NoError(t, err)
		tx, isSimple := parsed.Transaction()
		require.True(t, isSimple)

		signedTx, err := tx.Sign("Test SDF Network ; September 2015", highThresholdKP)
		require.NoError(t, err)

		signedTxBase64, err := signedTx.Base64()
		require.NoError(t, err)

		validationReq := ValidationRequest{Transaction: signedTxBase64}
		_, err = highThresholdService.ValidateChallenge(context.Background(), validationReq)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "verifying signature threshold")
	})
}

func TestSEP10Service_Integration_WithRealDB(t *testing.T) {
	models := data.SetupModels(t)

	kp := keypair.MustRandom()

	mockHTTPClient := mocks.NewHttpClientMock(t)
	mockHTTPClient.On("Do", mock.AnythingOfType("*http.Request")).Return(&http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("SIGNING_KEY = \"" + kp.Address() + "\"\n")),
	}, nil)

	mockHorizonClient := &horizonclient.MockClient{}
	mockHorizonClient.On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).Return(horizon.Account{
		AccountID: kp.Address(),
		Thresholds: horizon.AccountThresholds{
			LowThreshold:  1,
			MedThreshold:  2,
			HighThreshold: 3,
		},
	}, nil)

	service, err := NewSEP10Service(
		&anchorplatform.JWTManager{},
		"Test SDF Network ; September 2015",
		kp.Seed(),
		"https://cadia.imperium.com",
		models,
		true,
		mockHorizonClient,
	)
	require.NoError(t, err)

	sep10Service := service.(*sep10Service)
	sep10Service.HTTPClient = mockHTTPClient

	t.Run("integration test with real DB", func(t *testing.T) {
		req := ChallengeRequest{
			Account:      kp.Address(),
			HomeDomain:   "cadia.imperium.com",
			ClientDomain: "chaos.cadia.com",
		}

		resp, err := service.CreateChallenge(context.Background(), req)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.Transaction)
		assert.Equal(t, "Test SDF Network ; September 2015", resp.NetworkPassphrase)
	})
}
