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

	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/protocols/horizon"
	"github.com/stellar/go-stellar-sdk/support/render/problem"
	"github.com/stellar/go-stellar-sdk/txnbuild"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/sepauth"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient/mocks"
	servicesmocks "github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
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

func createMockSEP10NonceStore(t *testing.T) NonceStoreInterface {
	t.Helper()
	store := servicesmocks.NewMockNonceStore(t)
	store.On("Store", mock.Anything, mock.Anything).Return(nil).Maybe()
	store.On("Consume", mock.Anything, mock.Anything).Return(true, nil).Maybe()
	return store
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

func createMockHTTPClient(t *testing.T, clientDomainKP *keypair.Full) *mocks.HTTPClientMock {
	mockClient := mocks.NewHTTPClientMock(t)
	mockClient.
		On("Get", mock.AnythingOfType("string")).
		Return(func(_ string) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("SIGNING_KEY = \"" + clientDomainKP.Address() + "\"\n")),
			}
		}, nil)
	return mockClient
}

func createSEP10Service(t *testing.T, kps *testKeypairs, baseURL string, jwtManager *sepauth.JWTManager, setupHTTPMock bool) (*sep10Service, error) {
	if jwtManager == nil {
		jwtManager = &sepauth.JWTManager{}
	}

	service, err := NewSEP10Service(
		jwtManager,
		"Test SDF Network ; September 2015",
		kps.server.Seed(),
		baseURL,
		true,
		createMockHorizonClient(kps.client.Address(), horizon.AccountThresholds{MedThreshold: 1}, []horizon.Signer{
			{Key: kps.client.Address(), Weight: 1, Type: "ed25519_public_key"},
		}),
		true,
		createMockSEP10NonceStore(t),
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

	signedTx, err := tx.Sign("Test SDF Network ; September 2015", kps.server, kps.client, kps.clientDomain)
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
			expectError: false,
		},
		{
			name: "client_domain with only whitespace",
			req: ChallengeRequest{
				Account:      keypair.MustRandom().Address(),
				HomeDomain:   "fenris.imperium.com",
				ClientDomain: "   ",
			},
			expectError: false,
		},
		{
			name: "client_domain with leading/trailing whitespace",
			req: ChallengeRequest{
				Account:      keypair.MustRandom().Address(),
				HomeDomain:   "fenris.imperium.com",
				ClientDomain: "  chaos.cadia.com  ",
			},
			expectError: false,
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
			errMsg:      "invalid memo type: expected ID memo, got text",
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
			errMsg:      "unable to fetch stellar.toml from invalid-domain.com",
		},
		{
			name: "timeout validation",
			req: ChallengeRequest{
				Account:      kps.client.Address(),
				HomeDomain:   "ultramar.imperium.com",
				ClientDomain: "chaos.cadia.com",
			},
			expectError: true,
			errMsg:      "provided timebound must be at least 1s",
			checkResponse: func(t *testing.T, resp *ChallengeResponse) {
				// This test case will fail before reaching response check
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			service, err := createSEP10Service(t, kps, "https://ultramar.imperium.com", nil, !tc.expectError)
			require.NoError(t, err)

			// Set short timeout for timeout validation test
			if tc.name == "timeout validation" {
				service.AuthTimeout = time.Millisecond
				// Set up HTTP mock for timeout test to avoid network errors
				service.HTTPClient = createMockHTTPClient(t, kps.clientDomain)
			}

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

func TestSEP10Service_ValidateChallenge(t *testing.T) {
	t.Parallel()
	kps := newTestKeypairs()

	jwtManager, err := sepauth.NewJWTManager("emperors-light-123", 3600000)
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

		signedTx, err := tx.Sign("Test SDF Network ; September 2015", kps.client, kps.clientDomain)
		require.NoError(t, err)

		signedTxBase64, err := signedTx.Base64()
		require.NoError(t, err)

		validationReq := ValidationRequest{Transaction: signedTxBase64}
		validationResp, err := service.ValidateChallenge(context.Background(), validationReq)
		require.NoError(t, err)
		assert.NotNil(t, validationResp)
		assert.NotEmpty(t, validationResp.Token)
	})

	t.Run("nonce replay", func(t *testing.T) {
		nonceStore := servicesmocks.NewMockNonceStore(t)
		nonceStore.On("Store", mock.Anything, mock.Anything).Return(nil).Maybe()
		nonceStore.On("Consume", mock.Anything, mock.Anything).Return(true, nil).Once()
		nonceStore.On("Consume", mock.Anything, mock.Anything).Return(false, nil).Once()

		svc, err := createSEP10Service(t, kps, "https://stellar.local:8000", jwtManager, false)
		require.NoError(t, err)
		svc.nonceStore = nonceStore
		svc.HTTPClient = createMockHTTPClient(t, kps.clientDomain)

		signedTxBase64 := createSignedChallenge(t, svc, kps, "stellar.local:8000", "chaos.cadia.com")

		validationReq := ValidationRequest{Transaction: signedTxBase64}
		validationResp, err := svc.ValidateChallenge(context.Background(), validationReq)
		require.NoError(t, err)
		assert.NotNil(t, validationResp)

		validationResp, err = svc.ValidateChallenge(context.Background(), validationReq)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nonce is invalid or expired")
		assert.Nil(t, validationResp)
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
		assert.ErrorContains(t, err, "verifying client domain signature")
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
		assert.ErrorContains(t, err, "verifying client domain signature")
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

		signedTx, err := tx.Sign("Test SDF Network ; September 2015", kps.server, highThresholdKP, kps.clientDomain)
		require.NoError(t, err)

		signedTxBase64, err := signedTx.Base64()
		require.NoError(t, err)

		validationReq := ValidationRequest{Transaction: signedTxBase64}
		_, err = highThresholdService.ValidateChallenge(context.Background(), validationReq)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "verifying signature threshold")
	})

	t.Run("non-existent account without client_domain", func(t *testing.T) {
		// Create a new account that doesn't exist on Horizon
		nonExistentKP := keypair.MustRandom()

		// Create service with ClientAttributionRequired = false
		nonExistentService, err := NewSEP10Service(
			jwtManager,
			"Test SDF Network ; September 2015",
			kps.server.Seed(),
			"https://stellar.local:8000",
			true,
			nil,   // HorizonClient will be set below
			false, // ClientAttributionRequired = false
			createMockSEP10NonceStore(t),
		)
		require.NoError(t, err)

		sep10Svc := nonExistentService.(*sep10Service)

		// Mock Horizon client to return 404
		mockHorizonClient := &horizonclient.MockClient{}
		mockHorizonClient.On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).
			Return(horizon.Account{}, &horizonclient.Error{
				Problem: problem.P{
					Status: 404,
					Title:  "Resource Missing",
					Detail: "The resource at the url requested was not found.",
				},
			})
		sep10Svc.HorizonClient = mockHorizonClient

		// Create challenge without client_domain
		challengeReq := ChallengeRequest{
			Account:    nonExistentKP.Address(),
			HomeDomain: "stellar.local:8000",
		}

		challengeResp, err := sep10Svc.CreateChallenge(context.Background(), challengeReq)
		require.NoError(t, err)
		require.NotEmpty(t, challengeResp.Transaction)

		parsed, err := txnbuild.TransactionFromXDR(challengeResp.Transaction)
		require.NoError(t, err)
		tx, isSimple := parsed.Transaction()
		require.True(t, isSimple)

		// Sign with only client (no client_domain)
		signedTx, err := tx.Sign("Test SDF Network ; September 2015", nonExistentKP)
		require.NoError(t, err)

		signedTxBase64, err := signedTx.Base64()
		require.NoError(t, err)

		// Validate - should succeed for non-existent account
		validationReq := ValidationRequest{Transaction: signedTxBase64}
		validationResp, err := sep10Svc.ValidateChallenge(context.Background(), validationReq)
		assert.NoError(t, err)
		assert.NotNil(t, validationResp)
		assert.NotEmpty(t, validationResp.Token)
	})

	t.Run("non-existent account with client_domain", func(t *testing.T) {
		// Create a new account that doesn't exist on Horizon
		nonExistentKP := keypair.MustRandom()

		// Create service with ClientAttributionRequired = false
		nonExistentService, err := NewSEP10Service(
			jwtManager,
			"Test SDF Network ; September 2015",
			kps.server.Seed(),
			"https://stellar.local:8000",
			true,
			nil,   // HorizonClient will be set below
			false, // ClientAttributionRequired = false
			createMockSEP10NonceStore(t),
		)
		require.NoError(t, err)

		sep10Svc := nonExistentService.(*sep10Service)

		// Mock Horizon client to return 404
		mockHorizonClient := &horizonclient.MockClient{}
		mockHorizonClient.On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).
			Return(horizon.Account{}, &horizonclient.Error{
				Problem: problem.P{
					Status: 404,
					Title:  "Resource Missing",
					Detail: "The resource at the url requested was not found.",
				},
			})
		sep10Svc.HorizonClient = mockHorizonClient
		sep10Svc.HTTPClient = createMockHTTPClient(t, kps.clientDomain)

		// Create challenge with client_domain
		challengeReq := ChallengeRequest{
			Account:      nonExistentKP.Address(),
			HomeDomain:   "stellar.local:8000",
			ClientDomain: "chaos.cadia.com",
		}

		challengeResp, err := sep10Svc.CreateChallenge(context.Background(), challengeReq)
		require.NoError(t, err)
		require.NotEmpty(t, challengeResp.Transaction)

		parsed, err := txnbuild.TransactionFromXDR(challengeResp.Transaction)
		require.NoError(t, err)
		tx, isSimple := parsed.Transaction()
		require.True(t, isSimple)

		// Sign with client and client_domain
		signedTx, err := tx.Sign("Test SDF Network ; September 2015", nonExistentKP, kps.clientDomain)
		require.NoError(t, err)

		signedTxBase64, err := signedTx.Base64()
		require.NoError(t, err)

		// Validate - should succeed for non-existent account with client_domain
		validationReq := ValidationRequest{Transaction: signedTxBase64}
		validationResp, err := sep10Svc.ValidateChallenge(context.Background(), validationReq)
		assert.NoError(t, err)
		assert.NotNil(t, validationResp)
		assert.NotEmpty(t, validationResp.Token)
	})

	t.Run("succeeds with non-master signer signature", func(t *testing.T) {
		// Create account with a non-master signer
		clientKP := keypair.MustRandom()
		nonMasterSignerKP := keypair.MustRandom()

		nonMasterService, err := createSEP10Service(t, &testKeypairs{
			client:       clientKP,
			server:       kps.server,
			clientDomain: kps.clientDomain,
		}, "https://stellar.local:8000", jwtManager, false)
		require.NoError(t, err)

		// Mock Horizon to return account with non-master signer
		mockHorizonClient := &horizonclient.MockClient{}
		mockHorizonClient.On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).
			Return(horizon.Account{
				AccountID: clientKP.Address(),
				Thresholds: horizon.AccountThresholds{
					LowThreshold:  1,
					MedThreshold:  1,
					HighThreshold: 1,
				},
				Signers: []horizon.Signer{
					{Key: clientKP.Address(), Weight: 1, Type: "ed25519_public_key"},
					{Key: nonMasterSignerKP.Address(), Weight: 1, Type: "ed25519_public_key"},
				},
			}, nil)
		nonMasterService.HorizonClient = mockHorizonClient
		nonMasterService.HTTPClient = createMockHTTPClient(t, kps.clientDomain)

		// Create and sign challenge with non-master signer only
		challengeReq := ChallengeRequest{
			Account:      clientKP.Address(),
			HomeDomain:   "stellar.local:8000",
			ClientDomain: "chaos.cadia.com",
		}

		challengeResp, err := nonMasterService.CreateChallenge(context.Background(), challengeReq)
		require.NoError(t, err)
		require.NotEmpty(t, challengeResp.Transaction)

		parsed, err := txnbuild.TransactionFromXDR(challengeResp.Transaction)
		require.NoError(t, err)
		tx, isSimple := parsed.Transaction()
		require.True(t, isSimple)

		// Sign with non-master signer and client_domain (not master key)
		signedTx, err := tx.Sign("Test SDF Network ; September 2015", nonMasterSignerKP, kps.clientDomain)
		require.NoError(t, err)

		signedTxBase64, err := signedTx.Base64()
		require.NoError(t, err)

		// Should succeed - non-master signer has sufficient weight
		validationReq := ValidationRequest{Transaction: signedTxBase64}
		validationResp, err := nonMasterService.ValidateChallenge(context.Background(), validationReq)
		assert.NoError(t, err)
		assert.NotNil(t, validationResp)
		assert.NotEmpty(t, validationResp.Token)
	})

	t.Run("non-existent account with extra signatures should fail", func(t *testing.T) {
		// Create a new account that doesn't exist on Horizon
		nonExistentKP := keypair.MustRandom()
		extraKP := keypair.MustRandom()

		// Create service with ClientAttributionRequired = false
		nonExistentService, err := NewSEP10Service(
			jwtManager,
			"Test SDF Network ; September 2015",
			kps.server.Seed(),
			"https://stellar.local:8000",
			true,
			nil,   // HorizonClient will be set below
			false, // ClientAttributionRequired = false
			createMockSEP10NonceStore(t),
		)
		require.NoError(t, err)

		sep10Svc := nonExistentService.(*sep10Service)

		// Mock Horizon client to return 404
		mockHorizonClient := &horizonclient.MockClient{}
		mockHorizonClient.On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).
			Return(horizon.Account{}, &horizonclient.Error{
				Problem: problem.P{
					Status: 404,
					Title:  "Resource Missing",
					Detail: "The resource at the url requested was not found.",
				},
			})
		sep10Svc.HorizonClient = mockHorizonClient

		// Create challenge without client_domain
		challengeReq := ChallengeRequest{
			Account:    nonExistentKP.Address(),
			HomeDomain: "stellar.local:8000",
		}

		challengeResp, err := sep10Svc.CreateChallenge(context.Background(), challengeReq)
		require.NoError(t, err)
		require.NotEmpty(t, challengeResp.Transaction)

		parsed, err := txnbuild.TransactionFromXDR(challengeResp.Transaction)
		require.NoError(t, err)
		tx, isSimple := parsed.Transaction()
		require.True(t, isSimple)

		// Sign with client and extra signature
		signedTx, err := tx.Sign("Test SDF Network ; September 2015", nonExistentKP, extraKP)
		require.NoError(t, err)

		signedTxBase64, err := signedTx.Base64()
		require.NoError(t, err)

		// Validate - should fail due to extra signature
		validationReq := ValidationRequest{Transaction: signedTxBase64}
		_, err = sep10Svc.ValidateChallenge(context.Background(), validationReq)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "there is more than one client signer")
	})
}

func TestSEP10Service_SignatureValidation(t *testing.T) {
	t.Parallel()
	kps := newTestKeypairs()
	wrongKP := keypair.MustRandom()

	jwtManager, outerErr := sepauth.NewJWTManager("emperors-light-123", 3600000)
	require.NoError(t, outerErr)

	service, outerErr := createSEP10Service(t, kps, "https://stellar.local:8000", jwtManager, false)
	require.NoError(t, outerErr)

	t.Run("wrong server signature (extra wrong signature is ignored)", func(t *testing.T) {
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

		signedTx, err := tx.Sign("Test SDF Network ; September 2015", wrongKP, kps.client, kps.clientDomain)
		require.NoError(t, err)

		signedTxBase64, err := signedTx.Base64()
		require.NoError(t, err)

		validationReq := ValidationRequest{Transaction: signedTxBase64}
		resp, err := service.ValidateChallenge(context.Background(), validationReq)
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("wrong client domain signature", func(t *testing.T) {
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

		signedTx, err := tx.Sign("Test SDF Network ; September 2015", kps.server, kps.client, wrongKP)
		require.NoError(t, err)

		signedTxBase64, err := signedTx.Base64()
		require.NoError(t, err)

		validationReq := ValidationRequest{Transaction: signedTxBase64}
		_, err = service.ValidateChallenge(context.Background(), validationReq)
		assert.Error(t, err)
		if err != nil {
			assert.Contains(t, err.Error(), "verifying client domain signature")
		}
	})

	t.Run("missing server signature", func(t *testing.T) {
		// Build a challenge transaction manually without a server signature
		sa := txnbuild.SimpleAccount{AccountID: kps.server.Address(), Sequence: -1}
		currentTime := time.Now().UTC()
		operations := []txnbuild.Operation{
			&txnbuild.ManageData{
				SourceAccount: kps.client.Address(),
				Name:          "stellar.local:8000 auth",
				Value:         []byte(base64.StdEncoding.EncodeToString(make([]byte, 48))),
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
				TimeBounds: txnbuild.NewTimebounds(currentTime.Unix(), currentTime.Add(15*time.Minute).Unix()),
			},
		}
		tx, err := txnbuild.NewTransaction(txParams)
		require.NoError(t, err)

		// Only client and client domain signatures; missing server signature
		signedTx, err := tx.Sign("Test SDF Network ; September 2015", kps.client, kps.clientDomain)
		require.NoError(t, err)
		signedTxBase64, err := signedTx.Base64()
		require.NoError(t, err)

		validationReq := ValidationRequest{Transaction: signedTxBase64}
		_, err = service.ValidateChallenge(context.Background(), validationReq)
		assert.Error(t, err)
		if err != nil {
			assert.Contains(t, err.Error(), "reading challenge transaction")
		}
	})

	t.Run("missing client domain signature", func(t *testing.T) {
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

		signedTx, err := tx.Sign("Test SDF Network ; September 2015", kps.server, kps.client)
		require.NoError(t, err)

		signedTxBase64, err := signedTx.Base64()
		require.NoError(t, err)

		validationReq := ValidationRequest{Transaction: signedTxBase64}
		_, err = service.ValidateChallenge(context.Background(), validationReq)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "verifying client domain signature")
	})

	t.Run("all wrong signatures", func(t *testing.T) {
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

		signedTx, err := tx.Sign("Test SDF Network ; September 2015", wrongKP, wrongKP, wrongKP)
		require.NoError(t, err)

		signedTxBase64, err := signedTx.Base64()
		require.NoError(t, err)

		validationReq := ValidationRequest{Transaction: signedTxBase64}
		_, err = service.ValidateChallenge(context.Background(), validationReq)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "verifying client domain signature")
	})

	t.Run("valid signatures but wrong client domain account", func(t *testing.T) {
		mockClient := mocks.NewHTTPClientMock(t)
		mockClient.On("Get", mock.AnythingOfType("string")).Return(&http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("SIGNING_KEY = \"" + wrongKP.Address() + "\"\n")),
		}, nil)
		service.HTTPClient = mockClient

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

		signedTx, err := tx.Sign("Test SDF Network ; September 2015", kps.server, kps.client, kps.clientDomain)
		require.NoError(t, err)

		signedTxBase64, err := signedTx.Base64()
		require.NoError(t, err)

		validationReq := ValidationRequest{Transaction: signedTxBase64}
		_, err = service.ValidateChallenge(context.Background(), validationReq)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "fetching client domain signing key")
	})
}
