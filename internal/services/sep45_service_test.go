package services

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stellar/go/clients/stellartoml"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/xdr"
	"github.com/stellar/stellar-rpc/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar"
	stellarMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/stellar/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

const (
	testWebAuthVerifyContractID = "CD3LA6RKF5D2FN2R2L57MWXLBRSEWWENE74YBEFZSSGNJRJGICFGQXMX"
	testClientContractAddress   = "CCYU2FUIMK23K34U3SWCN2O2JVI6JBGUGQUILYK7GRPCIDABVVTCS7R4"
)

var (
	testWebAuthVerifyContract = decodeContractID(testWebAuthVerifyContractID)
	testClientContractID      = decodeContractID(testClientContractAddress)
)

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
				authEntries := marshalAuthorizationEntries(t, []xdr.SorobanAuthorizationEntry{
					makeAuthorizationEntry(t, testWebAuthVerifyContract, xdr.ScAddress{Type: xdr.ScAddressTypeScAddressTypeAccount, AccountId: &serverAccountID}, argEntries),
					makeAuthorizationEntry(t, testWebAuthVerifyContract, xdr.ScAddress{Type: xdr.ScAddressTypeScAddressTypeContract, ContractId: &clientContractID}, argEntries),
					makeAuthorizationEntry(t, testWebAuthVerifyContract, xdr.ScAddress{Type: xdr.ScAddressTypeScAddressTypeAccount, AccountId: &clientDomainAccountID}, argEntries),
				})

				var capturedTx string
				rpcMock.
					On("SimulateTransaction", mock.Anything, mock.MatchedBy(func(req protocol.SimulateTransactionRequest) bool {
						capturedTx = req.Transaction
						return true
					})).
					Return(&stellar.SimulationResult{
						Response: protocol.SimulateTransactionResponse{
							Results: []protocol.SimulateHostFunctionResult{{AuthXDR: &authEntries}},
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
						RPCClient:                 rpcMock,
						TOMLClient:                tomlMock,
						NetworkPassphrase:         network.TestNetworkPassphrase,
						WebAuthVerifyContractID:   testWebAuthVerifyContractID,
						ServerSigningKeypair:      serverKP,
						BaseURL:                   "https://" + baseHost,
						AllowHTTPRetry:            true,
						ClientAttributionRequired: true,
					}, SEP45ChallengeRequest{
						Account:      clientContractAddress,
						HomeDomain:   homeDomain,
						ClientDomain: &clientDomainCopy,
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
			name:        "requires client domain when attribution enforced",
			expectError: true,
			errContains: "client_domain",
			build: func(t *testing.T) (context.Context, SEP45ServiceOptions, SEP45ChallengeRequest, func(*testing.T, *SEP45ChallengeResponse)) {
				t.Helper()
				serverKP := keypair.MustRandom()
				ctx := context.Background()
				opts := SEP45ServiceOptions{
					RPCClient:                 stellarMocks.NewMockRPCClient(t),
					TOMLClient:                stellartoml.DefaultClient,
					NetworkPassphrase:         network.TestNetworkPassphrase,
					WebAuthVerifyContractID:   testWebAuthVerifyContractID,
					ServerSigningKeypair:      serverKP,
					BaseURL:                   "https://home.example.com",
					AllowHTTPRetry:            true,
					ClientAttributionRequired: true,
				}
				req := SEP45ChallengeRequest{Account: testClientContractAddress, HomeDomain: "home.example.com"}
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
					RPCClient:                 rpcMock,
					TOMLClient:                tomlMock,
					NetworkPassphrase:         network.TestNetworkPassphrase,
					WebAuthVerifyContractID:   testWebAuthVerifyContractID,
					ServerSigningKeypair:      serverKP,
					BaseURL:                   "https://home.example.com",
					AllowHTTPRetry:            true,
					ClientAttributionRequired: true,
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

func marshalAuthorizationEntries(t *testing.T, entries []xdr.SorobanAuthorizationEntry) []string {
	t.Helper()
	encoded := make([]string, 0, len(entries))
	for _, entry := range entries {
		bytes, err := entry.MarshalBinary()
		require.NoError(t, err)
		encoded = append(encoded, base64.StdEncoding.EncodeToString(bytes))
	}
	return encoded
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

func decodeContractID(contract string) xdr.ContractId {
	raw, err := strkey.Decode(strkey.VersionByteContract, contract)
	if err != nil {
		panic(err)
	}
	var id xdr.ContractId
	copy(id[:], raw)
	return id
}
