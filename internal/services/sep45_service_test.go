package services

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/stellar/go-stellar-sdk/clients/stellartoml"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	protocol "github.com/stellar/go-stellar-sdk/protocols/rpc"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/sepauth"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar"
	stellarMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/stellar/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

const (
	testWebAuthVerifyContractID = "CD3LA6RKF5D2FN2R2L57MWXLBRSEWWENE74YBEFZSSGNJRJGICFGQXMX"
	testClientContractAddress   = "CCYU2FUIMK23K34U3SWCN2O2JVI6JBGUGQUILYK7GRPCIDABVVTCS7R4"
	testJWTSecret               = "test_jwt_secret_sep45"
)

var (
	testWebAuthVerifyContract = decodeContractID(testWebAuthVerifyContractID)
	testClientContractID      = decodeContractID(testClientContractAddress)
)

func newTestJWTManager(t *testing.T) *sepauth.JWTManager {
	t.Helper()
	mgr, err := sepauth.NewJWTManager(testJWTSecret, 300000)
	require.NoError(t, err)
	return mgr
}

func Test_SEP45ChallengeRequest_Validate(t *testing.T) {
	validAccount := testClientContractAddress

	testCases := []struct {
		name        string
		req         SEP45ChallengeRequest
		expectError bool
		errMsg      string
	}{
		{
			name: "valid contract address",
			req: SEP45ChallengeRequest{
				Account:    validAccount,
				HomeDomain: "home.example.com",
			},
		},
		{
			name: "missing home domain",
			req: SEP45ChallengeRequest{
				Account: validAccount,
			},
			expectError: true,
			errMsg:      "home_domain is required",
		},
		{
			name:        "missing account",
			req:         SEP45ChallengeRequest{},
			expectError: true,
			errMsg:      "account is required",
		},
		{
			name: "invalid account type",
			req: SEP45ChallengeRequest{
				Account: keypair.MustRandom().Address(),
			},
			expectError: true,
			errMsg:      "account must be a valid contract address",
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

func Test_SEP45Service_CreateChallenge(t *testing.T) {
	testCases := []struct {
		name        string
		build       func(t *testing.T) (context.Context, SEP45ServiceOptions, SEP45ChallengeRequest, func(*testing.T, *SEP45ChallengeResponse))
		expectError bool
		errContains string
	}{
		{
			name: "valid challenge request with client domain",
			build: func(t *testing.T) (context.Context, SEP45ServiceOptions, SEP45ChallengeRequest, func(*testing.T, *SEP45ChallengeResponse)) {
				t.Helper()

				ctx := context.Background()
				serverKP := keypair.MustRandom()
				clientDomainKP := keypair.MustRandom()
				clientContractAddress := testClientContractAddress
				clientContractID := testClientContractID

				rpcMock := stellarMocks.NewMockRPCClient(t)
				tomlMock := &stellartoml.MockClient{}

				clientDomain := "wallet.example.com"
				homeDomain := "home.example.com"
				baseHost := "example.com"

				argEntries := xdr.ScMap{
					utils.NewSymbolStringEntry("account", clientContractAddress),
					utils.NewSymbolStringEntry("client_domain", clientDomain),
					utils.NewSymbolStringEntry("client_domain_account", clientDomainKP.Address()),
					utils.NewSymbolStringEntry("home_domain", homeDomain),
					utils.NewSymbolStringEntry("nonce", "nonce-value"),
					utils.NewSymbolStringEntry("web_auth_domain", baseHost),
					utils.NewSymbolStringEntry("web_auth_domain_account", serverKP.Address()),
				}

				serverAccountID := xdr.MustAddress(serverKP.Address())
				clientDomainAccountID := xdr.MustAddress(clientDomainKP.Address())
				entries := []xdr.SorobanAuthorizationEntry{
					makeAuthorizationEntry(t, testWebAuthVerifyContract, xdr.ScAddress{Type: xdr.ScAddressTypeScAddressTypeAccount, AccountId: &serverAccountID}, argEntries),
					makeAuthorizationEntry(t, testWebAuthVerifyContract, xdr.ScAddress{Type: xdr.ScAddressTypeScAddressTypeContract, ContractId: &clientContractID}, argEntries),
					makeAuthorizationEntry(t, testWebAuthVerifyContract, xdr.ScAddress{Type: xdr.ScAddressTypeScAddressTypeAccount, AccountId: &clientDomainAccountID}, argEntries),
				}
				entriesBase64 := make([]string, len(entries))
				for i, entry := range entries {
					bytes, err := entry.MarshalBinary()
					require.NoError(t, err)
					entriesBase64[i] = base64.StdEncoding.EncodeToString(bytes)
				}

				var capturedTx string
				rpcMock.
					On("SimulateTransaction", mock.Anything, mock.MatchedBy(func(req protocol.SimulateTransactionRequest) bool {
						capturedTx = req.Transaction
						return true
					})).
					Return(&stellar.SimulationResult{
						Response: protocol.SimulateTransactionResponse{
							Results: []protocol.SimulateHostFunctionResult{{AuthXDR: &entriesBase64}},
						},
					}, (*stellar.SimulationError)(nil)).
					Once()
				rpcMock.
					On("GetLatestLedgerSequence", mock.Anything).
					Return(uint32(100), nil).
					Once()

				tomlMock.
					On("GetStellarToml", clientDomain).
					Return(&stellartoml.Response{SigningKey: clientDomainKP.Address()}, nil).
					Once()

				clientDomainCopy := clientDomain
				assertFn := func(t *testing.T, resp *SEP45ChallengeResponse) {
					require.Equal(t, network.TestNetworkPassphrase, resp.NetworkPassphrase)

					rawEntries, err := base64.StdEncoding.DecodeString(resp.AuthorizationEntries)
					require.NoError(t, err)

					var signedEntries xdr.SorobanAuthorizationEntries
					require.NoError(t, signedEntries.UnmarshalBinary(rawEntries))
					require.Len(t, signedEntries, 3)
					require.Equal(t, xdr.Uint32(100+signatureExpirationLedgers), signedEntries[0].Credentials.Address.SignatureExpirationLedger)

					sigVec, ok := signedEntries[0].Credentials.Address.Signature.GetVec()
					require.True(t, ok)
					require.NotNil(t, sigVec)
					require.NotZero(t, len(*sigVec))

					argsMap := extractInvokeArgs(t, capturedTx)
					assert.Equal(t, clientContractAddress, argsMap["account"])
					assert.Equal(t, clientDomain, argsMap["client_domain"])
					assert.Equal(t, clientDomainKP.Address(), argsMap["client_domain_account"])
					assert.Equal(t, homeDomain, argsMap["home_domain"])
					assert.Equal(t, baseHost, argsMap["web_auth_domain"])
					assert.Equal(t, serverKP.Address(), argsMap["web_auth_domain_account"])
					assert.NotEmpty(t, argsMap["nonce"])
				}

				return ctx, SEP45ServiceOptions{
						RPCClient:               rpcMock,
						TOMLClient:              tomlMock,
						JWTManager:              newTestJWTManager(t),
						NetworkPassphrase:       network.TestNetworkPassphrase,
						WebAuthVerifyContractID: testWebAuthVerifyContractID,
						ServerSigningKeypair:    serverKP,
						BaseURL:                 "https://" + baseHost,
						AllowHTTPRetry:          true,
					}, SEP45ChallengeRequest{
						Account:      clientContractAddress,
						HomeDomain:   homeDomain,
						ClientDomain: &clientDomainCopy,
					}, assertFn
			},
		},
		{
			name: "valid challenge request without client domain",
			build: func(t *testing.T) (context.Context, SEP45ServiceOptions, SEP45ChallengeRequest, func(*testing.T, *SEP45ChallengeResponse)) {
				t.Helper()
				serverKP := keypair.MustRandom()
				rpcMock := stellarMocks.NewMockRPCClient(t)

				ctx := context.Background()
				clientContractAddress := testClientContractAddress
				homeDomain := "home.example.com"
				baseHost := "home.example.com"

				argEntries := xdr.ScMap{
					utils.NewSymbolStringEntry("account", clientContractAddress),
					utils.NewSymbolStringEntry("home_domain", homeDomain),
					utils.NewSymbolStringEntry("nonce", "nonce-value"),
					utils.NewSymbolStringEntry("web_auth_domain", baseHost),
					utils.NewSymbolStringEntry("web_auth_domain_account", serverKP.Address()),
				}

				serverAccountID := xdr.MustAddress(serverKP.Address())
				entries := []xdr.SorobanAuthorizationEntry{
					makeAuthorizationEntry(t, testWebAuthVerifyContract, xdr.ScAddress{Type: xdr.ScAddressTypeScAddressTypeAccount, AccountId: &serverAccountID}, argEntries),
					makeAuthorizationEntry(t, testWebAuthVerifyContract, xdr.ScAddress{Type: xdr.ScAddressTypeScAddressTypeContract, ContractId: &testClientContractID}, argEntries),
				}
				entriesBase64 := make([]string, len(entries))
				for i, entry := range entries {
					bytes, err := entry.MarshalBinary()
					require.NoError(t, err)
					entriesBase64[i] = base64.StdEncoding.EncodeToString(bytes)
				}

				var capturedTx string
				rpcMock.
					On("SimulateTransaction", mock.Anything, mock.MatchedBy(func(req protocol.SimulateTransactionRequest) bool {
						capturedTx = req.Transaction
						return true
					})).
					Return(&stellar.SimulationResult{
						Response: protocol.SimulateTransactionResponse{
							Results: []protocol.SimulateHostFunctionResult{{AuthXDR: &entriesBase64}},
						},
					}, (*stellar.SimulationError)(nil)).
					Once()
				rpcMock.
					On("GetLatestLedgerSequence", mock.Anything).
					Return(uint32(100), nil).
					Once()

				assertFn := func(t *testing.T, resp *SEP45ChallengeResponse) {
					require.Equal(t, network.TestNetworkPassphrase, resp.NetworkPassphrase)
					rawEntries, err := base64.StdEncoding.DecodeString(resp.AuthorizationEntries)
					require.NoError(t, err)

					var signedEntries xdr.SorobanAuthorizationEntries
					require.NoError(t, signedEntries.UnmarshalBinary(rawEntries))
					require.Len(t, signedEntries, 2)

					argsMap := extractInvokeArgs(t, capturedTx)
					assert.Equal(t, clientContractAddress, argsMap["account"])
					_, hasClientDomain := argsMap["client_domain"]
					assert.False(t, hasClientDomain)
					_, hasClientDomainAccount := argsMap["client_domain_account"]
					assert.False(t, hasClientDomainAccount)
					assert.Equal(t, homeDomain, argsMap["home_domain"])
					assert.Equal(t, baseHost, argsMap["web_auth_domain"])
					assert.Equal(t, serverKP.Address(), argsMap["web_auth_domain_account"])
				}

				return ctx, SEP45ServiceOptions{
						RPCClient:               rpcMock,
						TOMLClient:              nil,
						JWTManager:              newTestJWTManager(t),
						NetworkPassphrase:       network.TestNetworkPassphrase,
						WebAuthVerifyContractID: testWebAuthVerifyContractID,
						ServerSigningKeypair:    serverKP,
						BaseURL:                 "https://" + baseHost,
						AllowHTTPRetry:          true,
					}, SEP45ChallengeRequest{
						Account:    clientContractAddress,
						HomeDomain: homeDomain,
					}, assertFn
			},
		},
		{
			name:        "invalid account",
			expectError: true,
			errContains: "account",
			build: func(t *testing.T) (context.Context, SEP45ServiceOptions, SEP45ChallengeRequest, func(*testing.T, *SEP45ChallengeResponse)) {
				t.Helper()
				serverKP := keypair.MustRandom()
				ctx := context.Background()
				opts := SEP45ServiceOptions{
					RPCClient:               stellarMocks.NewMockRPCClient(t),
					TOMLClient:              stellartoml.DefaultClient,
					JWTManager:              newTestJWTManager(t),
					NetworkPassphrase:       network.TestNetworkPassphrase,
					WebAuthVerifyContractID: testWebAuthVerifyContractID,
					ServerSigningKeypair:    serverKP,
					BaseURL:                 "https://home.example.com",
					AllowHTTPRetry:          true,
				}
				req := SEP45ChallengeRequest{Account: "invalid-account", HomeDomain: "home.example.com"}
				return ctx, opts, req, nil
			},
		},
		{
			name:        "invalid home domain",
			expectError: true,
			errContains: "home_domain",
			build: func(t *testing.T) (context.Context, SEP45ServiceOptions, SEP45ChallengeRequest, func(*testing.T, *SEP45ChallengeResponse)) {
				t.Helper()
				serverKP := keypair.MustRandom()
				clientDomain := "wallet.example.com"
				clientDomainCopy := clientDomain
				ctx := context.Background()
				opts := SEP45ServiceOptions{
					RPCClient:               stellarMocks.NewMockRPCClient(t),
					TOMLClient:              stellartoml.DefaultClient,
					JWTManager:              newTestJWTManager(t),
					NetworkPassphrase:       network.TestNetworkPassphrase,
					WebAuthVerifyContractID: testWebAuthVerifyContractID,
					ServerSigningKeypair:    serverKP,
					BaseURL:                 "https://allowed.example.com",
					AllowHTTPRetry:          true,
				}
				req := SEP45ChallengeRequest{
					Account:      testClientContractAddress,
					HomeDomain:   "other.example.com",
					ClientDomain: &clientDomainCopy,
				}
				return ctx, opts, req, nil
			},
		},
		{
			name:        "missing home domain",
			expectError: true,
			errContains: "home_domain",
			build: func(t *testing.T) (context.Context, SEP45ServiceOptions, SEP45ChallengeRequest, func(*testing.T, *SEP45ChallengeResponse)) {
				t.Helper()
				serverKP := keypair.MustRandom()
				ctx := context.Background()
				opts := SEP45ServiceOptions{
					RPCClient:               stellarMocks.NewMockRPCClient(t),
					TOMLClient:              stellartoml.DefaultClient,
					JWTManager:              newTestJWTManager(t),
					NetworkPassphrase:       network.TestNetworkPassphrase,
					WebAuthVerifyContractID: testWebAuthVerifyContractID,
					ServerSigningKeypair:    serverKP,
					BaseURL:                 "https://home.example.com",
					AllowHTTPRetry:          true,
				}
				req := SEP45ChallengeRequest{Account: testClientContractAddress}
				return ctx, opts, req, nil
			},
		},
		{
			name:        "errors when client domain signing key missing",
			expectError: true,
			errContains: "SIGNING_KEY",
			build: func(t *testing.T) (context.Context, SEP45ServiceOptions, SEP45ChallengeRequest, func(*testing.T, *SEP45ChallengeResponse)) {
				t.Helper()

				ctx := context.Background()
				serverKP := keypair.MustRandom()
				rpcMock := stellarMocks.NewMockRPCClient(t)
				tomlMock := &stellartoml.MockClient{}

				clientDomain := "wallet.example.com"
				tomlMock.
					On("GetStellarToml", clientDomain).
					Return(&stellartoml.Response{SigningKey: ""}, nil).
					Once()

				clientDomainCopy := clientDomain

				opts := SEP45ServiceOptions{
					RPCClient:               rpcMock,
					TOMLClient:              tomlMock,
					JWTManager:              newTestJWTManager(t),
					NetworkPassphrase:       network.TestNetworkPassphrase,
					WebAuthVerifyContractID: testWebAuthVerifyContractID,
					ServerSigningKeypair:    serverKP,
					BaseURL:                 "https://home.example.com",
					AllowHTTPRetry:          true,
				}
				req := SEP45ChallengeRequest{
					Account:      testClientContractAddress,
					HomeDomain:   "home.example.com",
					ClientDomain: &clientDomainCopy,
				}
				return ctx, opts, req, nil
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, opts, req, assertFn := tc.build(t)
			svc, err := NewSEP45Service(opts)
			require.NoError(t, err)

			resp, err := svc.CreateChallenge(ctx, req)
			if tc.expectError {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrSEP45Validation) || errors.Is(err, ErrSEP45Internal))
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
				require.Nil(t, resp)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp)
			if assertFn != nil {
				assertFn(t, resp)
			}
		})
	}
}

func extractInvokeArgs(t *testing.T, txB64 string) map[string]string {
	t.Helper()

	var env xdr.TransactionEnvelope
	require.NoError(t, xdr.SafeUnmarshalBase64(txB64, &env))
	require.Equal(t, xdr.EnvelopeTypeEnvelopeTypeTx, env.Type)

	// Extract the first op
	ops := env.V1.Tx.Operations
	require.NotEmpty(t, ops)

	// Extract the invoke contract args
	invoke := ops[0].Body.MustInvokeHostFunctionOp()
	args := invoke.HostFunction.MustInvokeContract().Args
	require.NotEmpty(t, args)

	// Put it in a map
	argMap := args[0].MustMap()
	result := make(map[string]string, len(*argMap))
	for _, entry := range *argMap {
		sym := entry.Key.MustSym()
		if str, ok := entry.Val.GetStr(); ok {
			result[string(sym)] = string(str)
		}
	}
	return result
}

func Test_SEP45Service_ValidateChallenge(t *testing.T) {
	ctx := context.Background()
	serverKP := keypair.MustRandom()
	clientDomainKP := keypair.MustRandom()
	rpcMock := stellarMocks.NewMockRPCClient(t)

	argEntries := xdr.ScMap{
		utils.NewSymbolStringEntry("account", testClientContractAddress),
		utils.NewSymbolStringEntry("client_domain", "wallet.example.com"),
		utils.NewSymbolStringEntry("client_domain_account", clientDomainKP.Address()),
		utils.NewSymbolStringEntry("home_domain", "example.com"),
		utils.NewSymbolStringEntry("nonce", "nonce"),
		utils.NewSymbolStringEntry("web_auth_domain", "example.com"),
		utils.NewSymbolStringEntry("web_auth_domain_account", serverKP.Address()),
	}
	entries := buildAuthorizationEntries(t, argEntries, true, serverKP, clientDomainKP)
	entries[0] = signAuthorizationEntry(t, entries[0], serverKP)
	encodedEntries := encodeAuthorizationEntries(t, entries)

	rpcMock.
		On("SimulateTransaction", mock.Anything, mock.Anything).
		Return(&stellar.SimulationResult{Response: protocol.SimulateTransactionResponse{}}, (*stellar.SimulationError)(nil)).
		Once()

	jwtManager := newTestJWTManager(t)

	svcOpts := SEP45ServiceOptions{
		RPCClient:               rpcMock,
		TOMLClient:              stellartoml.DefaultClient,
		JWTManager:              jwtManager,
		NetworkPassphrase:       network.TestNetworkPassphrase,
		WebAuthVerifyContractID: testWebAuthVerifyContractID,
		ServerSigningKeypair:    serverKP,
		BaseURL:                 "https://example.com",
	}
	svc, err := NewSEP45Service(svcOpts)
	require.NoError(t, err)

	resp, err := svc.ValidateChallenge(ctx, SEP45ValidationRequest{AuthorizationEntries: encodedEntries})
	require.NoError(t, err)
	require.NotNil(t, resp)

	claims, parseErr := jwtManager.ParseSEP45TokenClaims(resp.Token)
	require.NoError(t, parseErr)
	require.Equal(t, "https://example.com/sep45/auth", claims.Issuer)
	require.Equal(t, testClientContractAddress, claims.Subject)
	require.Equal(t, "wallet.example.com", claims.ClientDomain)
	require.Equal(t, "example.com", claims.HomeDomain)
	require.NotNil(t, claims.IssuedAt)
	require.NotNil(t, claims.ExpiresAt)
	require.True(t, claims.ExpiresAt.After(claims.IssuedAt.Time))

	rpcMock.AssertExpectations(t)
}

func Test_SEP45Service_ValidateChallengeErrors(t *testing.T) {
	ctx := context.Background()

	newService := func(t *testing.T, rpcClient stellar.RPCClient, serverKP *keypair.Full) SEP45Service {
		t.Helper()
		svc, err := NewSEP45Service(SEP45ServiceOptions{
			RPCClient:               rpcClient,
			TOMLClient:              stellartoml.DefaultClient,
			JWTManager:              newTestJWTManager(t),
			NetworkPassphrase:       network.TestNetworkPassphrase,
			WebAuthVerifyContractID: testWebAuthVerifyContractID,
			ServerSigningKeypair:    serverKP,
			BaseURL:                 "https://example.com",
		})
		require.NoError(t, err)
		return svc
	}

	t.Run("missing authorization entries", func(t *testing.T) {
		serverKP := keypair.MustRandom()
		rpcMock := stellarMocks.NewMockRPCClient(t)
		svc := newService(t, rpcMock, serverKP)

		resp, err := svc.ValidateChallenge(ctx, SEP45ValidationRequest{})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrSEP45Validation))
		assert.Contains(t, err.Error(), "authorization_entries is required")
		require.Nil(t, resp)
	})

	t.Run("invalid authorization entries encoding", func(t *testing.T) {
		serverKP := keypair.MustRandom()
		rpcMock := stellarMocks.NewMockRPCClient(t)
		svc := newService(t, rpcMock, serverKP)

		resp, err := svc.ValidateChallenge(ctx, SEP45ValidationRequest{AuthorizationEntries: "not-base64"})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrSEP45Validation))
		assert.Contains(t, err.Error(), "decoding authorization entries")
		require.Nil(t, resp)
	})

	t.Run("missing server authorization entry", func(t *testing.T) {
		serverKP := keypair.MustRandom()
		clientDomainKP := keypair.MustRandom()
		rpcMock := stellarMocks.NewMockRPCClient(t)

		argEntries := xdr.ScMap{
			utils.NewSymbolStringEntry("account", testClientContractAddress),
			utils.NewSymbolStringEntry("client_domain", "wallet.example.com"),
			utils.NewSymbolStringEntry("client_domain_account", clientDomainKP.Address()),
			utils.NewSymbolStringEntry("home_domain", "example.com"),
			utils.NewSymbolStringEntry("nonce", "nonce"),
			utils.NewSymbolStringEntry("web_auth_domain", "example.com"),
			utils.NewSymbolStringEntry("web_auth_domain_account", serverKP.Address()),
		}
		entries := buildAuthorizationEntries(t, argEntries, true, serverKP, clientDomainKP)
		entries = entries[1:]
		encoded := encodeAuthorizationEntries(t, entries)

		svc := newService(t, rpcMock, serverKP)
		resp, err := svc.ValidateChallenge(ctx, SEP45ValidationRequest{AuthorizationEntries: encoded})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrSEP45Validation))
		assert.Contains(t, err.Error(), "missing signed server authorization entry")
		require.Nil(t, resp)
	})

	t.Run("missing client authorization entry", func(t *testing.T) {
		serverKP := keypair.MustRandom()
		clientDomainKP := keypair.MustRandom()
		rpcMock := stellarMocks.NewMockRPCClient(t)

		argEntries := xdr.ScMap{
			utils.NewSymbolStringEntry("account", testClientContractAddress),
			utils.NewSymbolStringEntry("client_domain", "wallet.example.com"),
			utils.NewSymbolStringEntry("client_domain_account", clientDomainKP.Address()),
			utils.NewSymbolStringEntry("home_domain", "example.com"),
			utils.NewSymbolStringEntry("nonce", "nonce"),
			utils.NewSymbolStringEntry("web_auth_domain", "example.com"),
			utils.NewSymbolStringEntry("web_auth_domain_account", serverKP.Address()),
		}
		entries := buildAuthorizationEntries(t, argEntries, true, serverKP, clientDomainKP)
		entries[0] = signAuthorizationEntry(t, entries[0], serverKP)
		entries = xdr.SorobanAuthorizationEntries{entries[0], entries[2]}
		encoded := encodeAuthorizationEntries(t, entries)

		svc := newService(t, rpcMock, serverKP)
		resp, err := svc.ValidateChallenge(ctx, SEP45ValidationRequest{AuthorizationEntries: encoded})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrSEP45Validation))
		assert.Contains(t, err.Error(), "missing client account authorization entry")
		require.Nil(t, resp)
		rpcMock.AssertExpectations(t)
	})

	t.Run("missing client domain authorization entry", func(t *testing.T) {
		serverKP := keypair.MustRandom()
		clientDomainKP := keypair.MustRandom()
		rpcMock := stellarMocks.NewMockRPCClient(t)

		argEntries := xdr.ScMap{
			utils.NewSymbolStringEntry("account", testClientContractAddress),
			utils.NewSymbolStringEntry("client_domain", "wallet.example.com"),
			utils.NewSymbolStringEntry("client_domain_account", clientDomainKP.Address()),
			utils.NewSymbolStringEntry("home_domain", "example.com"),
			utils.NewSymbolStringEntry("nonce", "nonce"),
			utils.NewSymbolStringEntry("web_auth_domain", "example.com"),
			utils.NewSymbolStringEntry("web_auth_domain_account", serverKP.Address()),
		}
		entries := buildAuthorizationEntries(t, argEntries, false, serverKP, clientDomainKP)
		entries[0] = signAuthorizationEntry(t, entries[0], serverKP)
		encoded := encodeAuthorizationEntries(t, entries)

		svc := newService(t, rpcMock, serverKP)
		resp, err := svc.ValidateChallenge(ctx, SEP45ValidationRequest{AuthorizationEntries: encoded})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrSEP45Validation))
		assert.Contains(t, err.Error(), "missing client domain authorization entry")
		require.Nil(t, resp)
		rpcMock.AssertExpectations(t)
	})

	t.Run("authorization entry wrong contract id", func(t *testing.T) {
		serverKP := keypair.MustRandom()
		clientDomainKP := keypair.MustRandom()
		rpcMock := stellarMocks.NewMockRPCClient(t)

		argEntries := xdr.ScMap{
			utils.NewSymbolStringEntry("account", testClientContractAddress),
			utils.NewSymbolStringEntry("client_domain", "wallet.example.com"),
			utils.NewSymbolStringEntry("client_domain_account", clientDomainKP.Address()),
			utils.NewSymbolStringEntry("home_domain", "example.com"),
			utils.NewSymbolStringEntry("nonce", "nonce"),
			utils.NewSymbolStringEntry("web_auth_domain", "example.com"),
			utils.NewSymbolStringEntry("web_auth_domain_account", serverKP.Address()),
		}
		entries := buildAuthorizationEntries(t, argEntries, true, serverKP, clientDomainKP)
		entries[0] = signAuthorizationEntry(t, entries[0], serverKP)
		invalidContract := testClientContractID
		entries[0].RootInvocation.Function.ContractFn.ContractAddress.ContractId = &invalidContract
		encoded := encodeAuthorizationEntries(t, entries)

		svc := newService(t, rpcMock, serverKP)
		resp, err := svc.ValidateChallenge(ctx, SEP45ValidationRequest{AuthorizationEntries: encoded})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrSEP45Validation))
		assert.Contains(t, err.Error(), "authorization entry targets unexpected contract")
		require.Nil(t, resp)
	})

	t.Run("authorization entry wrong function name", func(t *testing.T) {
		serverKP := keypair.MustRandom()
		clientDomainKP := keypair.MustRandom()
		rpcMock := stellarMocks.NewMockRPCClient(t)

		argEntries := xdr.ScMap{
			utils.NewSymbolStringEntry("account", testClientContractAddress),
			utils.NewSymbolStringEntry("client_domain", "wallet.example.com"),
			utils.NewSymbolStringEntry("client_domain_account", clientDomainKP.Address()),
			utils.NewSymbolStringEntry("home_domain", "example.com"),
			utils.NewSymbolStringEntry("nonce", "nonce"),
			utils.NewSymbolStringEntry("web_auth_domain", "example.com"),
			utils.NewSymbolStringEntry("web_auth_domain_account", serverKP.Address()),
		}
		entries := buildAuthorizationEntries(t, argEntries, true, serverKP, clientDomainKP)
		entries[0] = signAuthorizationEntry(t, entries[0], serverKP)
		entries[0].RootInvocation.Function.ContractFn.FunctionName = "other_fn"
		encoded := encodeAuthorizationEntries(t, entries)

		svc := newService(t, rpcMock, serverKP)
		resp, err := svc.ValidateChallenge(ctx, SEP45ValidationRequest{AuthorizationEntries: encoded})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrSEP45Validation))
		assert.Contains(t, err.Error(), "authorization entry must call web_auth_verify")
		require.Nil(t, resp)
	})

	t.Run("authorization entry contains sub-invocations", func(t *testing.T) {
		serverKP := keypair.MustRandom()
		clientDomainKP := keypair.MustRandom()
		rpcMock := stellarMocks.NewMockRPCClient(t)

		argEntries := xdr.ScMap{
			utils.NewSymbolStringEntry("account", testClientContractAddress),
			utils.NewSymbolStringEntry("client_domain", "wallet.example.com"),
			utils.NewSymbolStringEntry("client_domain_account", clientDomainKP.Address()),
			utils.NewSymbolStringEntry("home_domain", "example.com"),
			utils.NewSymbolStringEntry("nonce", "nonce"),
			utils.NewSymbolStringEntry("web_auth_domain", "example.com"),
			utils.NewSymbolStringEntry("web_auth_domain_account", serverKP.Address()),
		}
		entries := buildAuthorizationEntries(t, argEntries, true, serverKP, clientDomainKP)
		entries[0] = signAuthorizationEntry(t, entries[0], serverKP)
		subInvocation := xdr.SorobanAuthorizedInvocation{
			Function: xdr.SorobanAuthorizedFunction{
				Type: xdr.SorobanAuthorizedFunctionTypeSorobanAuthorizedFunctionTypeContractFn,
				ContractFn: &xdr.InvokeContractArgs{
					ContractAddress: xdr.ScAddress{
						Type:       xdr.ScAddressTypeScAddressTypeContract,
						ContractId: &testWebAuthVerifyContract,
					},
					FunctionName: "web_auth_verify",
					Args:         entries[0].RootInvocation.Function.ContractFn.Args,
				},
			},
		}
		entries[0].RootInvocation.SubInvocations = []xdr.SorobanAuthorizedInvocation{subInvocation}
		encoded := encodeAuthorizationEntries(t, entries)

		svc := newService(t, rpcMock, serverKP)
		resp, err := svc.ValidateChallenge(ctx, SEP45ValidationRequest{AuthorizationEntries: encoded})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrSEP45Validation))
		assert.Contains(t, err.Error(), "authorization entries cannot contain sub-invocations")
		require.Nil(t, resp)
	})

	t.Run("authorization entry args mismatch", func(t *testing.T) {
		serverKP := keypair.MustRandom()
		clientDomainKP := keypair.MustRandom()
		rpcMock := stellarMocks.NewMockRPCClient(t)

		argEntries := xdr.ScMap{
			utils.NewSymbolStringEntry("account", testClientContractAddress),
			utils.NewSymbolStringEntry("client_domain", "wallet.example.com"),
			utils.NewSymbolStringEntry("client_domain_account", clientDomainKP.Address()),
			utils.NewSymbolStringEntry("home_domain", "example.com"),
			utils.NewSymbolStringEntry("nonce", "nonce"),
			utils.NewSymbolStringEntry("web_auth_domain", "example.com"),
			utils.NewSymbolStringEntry("web_auth_domain_account", serverKP.Address()),
		}
		entries := buildAuthorizationEntries(t, argEntries, true, serverKP, clientDomainKP)
		entries[0] = signAuthorizationEntry(t, entries[0], serverKP)

		modifiedArgs := xdr.ScMap{
			utils.NewSymbolStringEntry("account", testClientContractAddress),
			utils.NewSymbolStringEntry("client_domain", "wallet.example.com"),
			utils.NewSymbolStringEntry("client_domain_account", clientDomainKP.Address()),
			utils.NewSymbolStringEntry("home_domain", "example.com"),
			utils.NewSymbolStringEntry("nonce", "other-nonce"),
			utils.NewSymbolStringEntry("web_auth_domain", "example.com"),
			utils.NewSymbolStringEntry("web_auth_domain_account", serverKP.Address()),
		}
		mapVal, err := xdr.NewScVal(xdr.ScValTypeScvMap, &modifiedArgs)
		require.NoError(t, err)
		entries[1].RootInvocation.Function.ContractFn.Args = xdr.ScVec{mapVal}
		encoded := encodeAuthorizationEntries(t, entries)

		svc := newService(t, rpcMock, serverKP)
		resp, err := svc.ValidateChallenge(ctx, SEP45ValidationRequest{AuthorizationEntries: encoded})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrSEP45Validation))
		assert.Contains(t, err.Error(), "authorization entry arguments mismatch")
		require.Nil(t, resp)
	})

	t.Run("nonce argument required", func(t *testing.T) {
		serverKP := keypair.MustRandom()
		clientDomainKP := keypair.MustRandom()
		rpcMock := stellarMocks.NewMockRPCClient(t)

		argEntries := xdr.ScMap{
			utils.NewSymbolStringEntry("account", testClientContractAddress),
			utils.NewSymbolStringEntry("client_domain", "wallet.example.com"),
			utils.NewSymbolStringEntry("client_domain_account", clientDomainKP.Address()),
			utils.NewSymbolStringEntry("home_domain", "example.com"),
			utils.NewSymbolStringEntry("web_auth_domain", "example.com"),
			utils.NewSymbolStringEntry("web_auth_domain_account", serverKP.Address()),
		}
		entries := buildAuthorizationEntries(t, argEntries, true, serverKP, clientDomainKP)
		entries[0] = signAuthorizationEntry(t, entries[0], serverKP)
		encoded := encodeAuthorizationEntries(t, entries)

		svc := newService(t, rpcMock, serverKP)
		resp, err := svc.ValidateChallenge(ctx, SEP45ValidationRequest{AuthorizationEntries: encoded})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nonce is required")
		require.Nil(t, resp)
	})

	t.Run("web auth domain account mismatch", func(t *testing.T) {
		serverKP := keypair.MustRandom()
		clientDomainKP := keypair.MustRandom()
		rpcMock := stellarMocks.NewMockRPCClient(t)

		argEntries := xdr.ScMap{
			utils.NewSymbolStringEntry("account", testClientContractAddress),
			utils.NewSymbolStringEntry("client_domain", "wallet.example.com"),
			utils.NewSymbolStringEntry("client_domain_account", clientDomainKP.Address()),
			utils.NewSymbolStringEntry("home_domain", "example.com"),
			utils.NewSymbolStringEntry("nonce", "nonce"),
			utils.NewSymbolStringEntry("web_auth_domain", "example.com"),
			utils.NewSymbolStringEntry("web_auth_domain_account", keypair.MustRandom().Address()),
		}
		entries := buildAuthorizationEntries(t, argEntries, true, serverKP, clientDomainKP)
		entries[0] = signAuthorizationEntry(t, entries[0], serverKP)
		encoded := encodeAuthorizationEntries(t, entries)

		svc := newService(t, rpcMock, serverKP)
		resp, err := svc.ValidateChallenge(ctx, SEP45ValidationRequest{AuthorizationEntries: encoded})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "web_auth_domain_account must match server signing key")
		require.Nil(t, resp)
	})

	t.Run("client domain requires account when provided", func(t *testing.T) {
		serverKP := keypair.MustRandom()
		clientDomainKP := keypair.MustRandom()
		rpcMock := stellarMocks.NewMockRPCClient(t)

		argEntries := xdr.ScMap{
			utils.NewSymbolStringEntry("account", testClientContractAddress),
			utils.NewSymbolStringEntry("client_domain", "wallet.example.com"),
			utils.NewSymbolStringEntry("home_domain", "example.com"),
			utils.NewSymbolStringEntry("nonce", "nonce"),
			utils.NewSymbolStringEntry("web_auth_domain", "example.com"),
			utils.NewSymbolStringEntry("web_auth_domain_account", serverKP.Address()),
		}
		entries := buildAuthorizationEntries(t, argEntries, true, serverKP, clientDomainKP)
		entries[0] = signAuthorizationEntry(t, entries[0], serverKP)
		encoded := encodeAuthorizationEntries(t, entries)

		svc := newService(t, rpcMock, serverKP)
		resp, err := svc.ValidateChallenge(ctx, SEP45ValidationRequest{AuthorizationEntries: encoded})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "client_domain_account is required when client_domain is provided")
		require.Nil(t, resp)
	})

	t.Run("client domain account requires client domain", func(t *testing.T) {
		serverKP := keypair.MustRandom()
		rpcMock := stellarMocks.NewMockRPCClient(t)

		argEntries := xdr.ScMap{
			utils.NewSymbolStringEntry("account", testClientContractAddress),
			utils.NewSymbolStringEntry("client_domain_account", keypair.MustRandom().Address()),
			utils.NewSymbolStringEntry("home_domain", "example.com"),
			utils.NewSymbolStringEntry("nonce", "nonce"),
			utils.NewSymbolStringEntry("web_auth_domain", "example.com"),
			utils.NewSymbolStringEntry("web_auth_domain_account", serverKP.Address()),
		}
		entries := buildAuthorizationEntries(t, argEntries, false, serverKP, nil)
		entries[0] = signAuthorizationEntry(t, entries[0], serverKP)
		encoded := encodeAuthorizationEntries(t, entries)

		svc := newService(t, rpcMock, serverKP)
		resp, err := svc.ValidateChallenge(ctx, SEP45ValidationRequest{AuthorizationEntries: encoded})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "client_domain is required when client_domain_account is provided")
		require.Nil(t, resp)
	})

	t.Run("web auth domain mismatch", func(t *testing.T) {
		serverKP := keypair.MustRandom()
		clientDomainKP := keypair.MustRandom()
		rpcMock := stellarMocks.NewMockRPCClient(t)

		argEntries := xdr.ScMap{
			utils.NewSymbolStringEntry("account", testClientContractAddress),
			utils.NewSymbolStringEntry("client_domain", "wallet.example.com"),
			utils.NewSymbolStringEntry("client_domain_account", clientDomainKP.Address()),
			utils.NewSymbolStringEntry("home_domain", "example.com"),
			utils.NewSymbolStringEntry("nonce", "nonce"),
			utils.NewSymbolStringEntry("web_auth_domain", "other.example.com"),
			utils.NewSymbolStringEntry("web_auth_domain_account", serverKP.Address()),
		}
		entries := buildAuthorizationEntries(t, argEntries, true, serverKP, clientDomainKP)
		entries[0] = signAuthorizationEntry(t, entries[0], serverKP)
		encoded := encodeAuthorizationEntries(t, entries)

		svc := newService(t, rpcMock, serverKP)
		resp, err := svc.ValidateChallenge(ctx, SEP45ValidationRequest{AuthorizationEntries: encoded})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "web_auth_domain")
		require.Nil(t, resp)
	})

	t.Run("simulation failure treated as internal for rpc errors", func(t *testing.T) {
		serverKP := keypair.MustRandom()
		clientDomainKP := keypair.MustRandom()
		rpcMock := stellarMocks.NewMockRPCClient(t)

		argEntries := xdr.ScMap{
			utils.NewSymbolStringEntry("account", testClientContractAddress),
			utils.NewSymbolStringEntry("client_domain", "wallet.example.com"),
			utils.NewSymbolStringEntry("client_domain_account", clientDomainKP.Address()),
			utils.NewSymbolStringEntry("home_domain", "example.com"),
			utils.NewSymbolStringEntry("nonce", "nonce"),
			utils.NewSymbolStringEntry("web_auth_domain", "example.com"),
			utils.NewSymbolStringEntry("web_auth_domain_account", serverKP.Address()),
		}
		entries := buildAuthorizationEntries(t, argEntries, true, serverKP, clientDomainKP)
		entries[0] = signAuthorizationEntry(t, entries[0], serverKP)
		encoded := encodeAuthorizationEntries(t, entries)

		rpcMock.
			On("SimulateTransaction", mock.Anything, mock.Anything).
			Return((*stellar.SimulationResult)(nil), stellar.NewSimulationError(errors.New("boom"), nil)).
			Once()

		svc := newService(t, rpcMock, serverKP)
		resp, err := svc.ValidateChallenge(ctx, SEP45ValidationRequest{AuthorizationEntries: encoded})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "simulating transaction")
		assert.True(t, errors.Is(err, ErrSEP45Internal))
		require.Nil(t, resp)
		rpcMock.AssertExpectations(t)
	})

	t.Run("simulation failure treated as validation for contract errors", func(t *testing.T) {
		serverKP := keypair.MustRandom()
		clientDomainKP := keypair.MustRandom()
		rpcMock := stellarMocks.NewMockRPCClient(t)

		argEntries := xdr.ScMap{
			utils.NewSymbolStringEntry("account", testClientContractAddress),
			utils.NewSymbolStringEntry("client_domain", "wallet.example.com"),
			utils.NewSymbolStringEntry("client_domain_account", clientDomainKP.Address()),
			utils.NewSymbolStringEntry("home_domain", "example.com"),
			utils.NewSymbolStringEntry("nonce", "nonce"),
			utils.NewSymbolStringEntry("web_auth_domain", "example.com"),
			utils.NewSymbolStringEntry("web_auth_domain_account", serverKP.Address()),
		}
		entries := buildAuthorizationEntries(t, argEntries, true, serverKP, clientDomainKP)
		entries[0] = signAuthorizationEntry(t, entries[0], serverKP)
		encoded := encodeAuthorizationEntries(t, entries)

		rpcMock.
			On("SimulateTransaction", mock.Anything, mock.Anything).
			Return((*stellar.SimulationResult)(nil), stellar.NewSimulationError(
				errors.New("contract execution failed"),
				&protocol.SimulateTransactionResponse{Error: "contract execution failed: trap"},
			)).
			Once()

		svc := newService(t, rpcMock, serverKP)
		resp, err := svc.ValidateChallenge(ctx, SEP45ValidationRequest{AuthorizationEntries: encoded})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "simulating transaction")
		assert.True(t, errors.Is(err, ErrSEP45Validation))
		require.Nil(t, resp)
		rpcMock.AssertExpectations(t)
	})
}

func makeAuthorizationEntry(t *testing.T, contractID xdr.ContractId, address xdr.ScAddress, argEntries xdr.ScMap) xdr.SorobanAuthorizationEntry {
	t.Helper()
	mapVal, err := xdr.NewScVal(xdr.ScValTypeScvMap, &argEntries)
	require.NoError(t, err)
	emptyVec := xdr.ScVec{}
	emptySignature, err := xdr.NewScVal(xdr.ScValTypeScvVec, &emptyVec)
	require.NoError(t, err)
	return xdr.SorobanAuthorizationEntry{
		Credentials: xdr.SorobanCredentials{
			Type: xdr.SorobanCredentialsTypeSorobanCredentialsAddress,
			Address: &xdr.SorobanAddressCredentials{
				Address:                   address,
				Nonce:                     0,
				SignatureExpirationLedger: 0,
				Signature:                 emptySignature,
			},
		},
		RootInvocation: xdr.SorobanAuthorizedInvocation{
			Function: xdr.SorobanAuthorizedFunction{
				Type: xdr.SorobanAuthorizedFunctionTypeSorobanAuthorizedFunctionTypeContractFn,
				ContractFn: &xdr.InvokeContractArgs{
					ContractAddress: xdr.ScAddress{
						Type:       xdr.ScAddressTypeScAddressTypeContract,
						ContractId: &contractID,
					},
					FunctionName: "web_auth_verify",
					Args:         xdr.ScVec{mapVal},
				},
			},
		},
	}
}

func buildAuthorizationEntries(t *testing.T, argEntries xdr.ScMap, includeClientDomain bool, serverKP, clientDomainKP *keypair.Full) xdr.SorobanAuthorizationEntries {
	t.Helper()
	serverAccountID := xdr.MustAddress(serverKP.Address())
	entries := xdr.SorobanAuthorizationEntries{
		makeAuthorizationEntry(t, testWebAuthVerifyContract, xdr.ScAddress{Type: xdr.ScAddressTypeScAddressTypeAccount, AccountId: &serverAccountID}, argEntries),
		makeAuthorizationEntry(t, testWebAuthVerifyContract, xdr.ScAddress{Type: xdr.ScAddressTypeScAddressTypeContract, ContractId: &testClientContractID}, argEntries),
	}
	if includeClientDomain {
		require.NotNil(t, clientDomainKP)
		clientDomainAccountID := xdr.MustAddress(clientDomainKP.Address())
		entries = append(entries, makeAuthorizationEntry(t, testWebAuthVerifyContract, xdr.ScAddress{Type: xdr.ScAddressTypeScAddressTypeAccount, AccountId: &clientDomainAccountID}, argEntries))
	}
	return entries
}

func signAuthorizationEntry(t *testing.T, entry xdr.SorobanAuthorizationEntry, signingKP *keypair.Full) xdr.SorobanAuthorizationEntry {
	t.Helper()
	signed, err := utils.SignAuthEntry(entry, 1000, signingKP, network.TestNetworkPassphrase)
	require.NoError(t, err)
	return signed
}

func encodeAuthorizationEntries(t *testing.T, entries xdr.SorobanAuthorizationEntries) string {
	t.Helper()
	raw, err := entries.MarshalBinary()
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(raw)
}

func decodeContractID(contract string) xdr.ContractId {
	raw, err := strkey.Decode(strkey.VersionByteContract, contract)
	if err != nil {
		panic(err)
	}
	var id xdr.ContractId
	copy(id[:], raw)
	return id
}
