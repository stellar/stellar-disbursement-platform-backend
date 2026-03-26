package transactionsubmission

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"testing"
	"time"

	protocol "github.com/stellar/go-stellar-sdk/protocols/rpc"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	sdpMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

func Test_NewSponsoredTransactionHandler(t *testing.T) {
	rpcClient := &mocks.MockRPCClient{}
	tssMonitorSvc := tssMonitor.TSSMonitorService{
		GitCommitHash: "gitCommitHash0x",
		Version:       "version123",
		Client:        &sdpMonitor.MockMonitorClient{},
	}

	testCases := []struct {
		name          string
		engine        *engine.SubmitterEngine
		rpcClient     stellar.RPCClient
		tssMonitorSvc tssMonitor.TSSMonitorService
		wantError     string
	}{
		{
			name:          "validate engine",
			rpcClient:     rpcClient,
			tssMonitorSvc: tssMonitorSvc,
			wantError:     "engine cannot be nil",
		},
		{
			name:          "validate rpcClient",
			engine:        &engine.SubmitterEngine{},
			tssMonitorSvc: tssMonitorSvc,
			wantError:     "rpcClient cannot be nil",
		},
		{
			name:      "validate tssMonitorSvc",
			engine:    &engine.SubmitterEngine{},
			rpcClient: rpcClient,
			wantError: "monitorSvc cannot be nil",
		},
		{
			name:          "🎉 successfully returns a new sponsored transaction handler",
			engine:        &engine.SubmitterEngine{},
			rpcClient:     rpcClient,
			tssMonitorSvc: tssMonitorSvc,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sponsoredHandler, err := NewSponsoredTransactionHandler(tc.engine, tc.rpcClient, tc.tssMonitorSvc)
			if tc.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantError)
				assert.Nil(t, sponsoredHandler)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, sponsoredHandler)
				assert.Equal(t, sponsoredHandler.engine, tc.engine)
				assert.Equal(t, sponsoredHandler.rpcClient, tc.rpcClient)
				assert.Equal(t, sponsoredHandler.monitorSvc, tc.tssMonitorSvc)
			}
		})
	}
}

func Test_SponsoredTransactionHandler_BuildInnerTransaction(t *testing.T) {
	ctx := context.Background()
	distributionAccount := "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"
	channelAccount := "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX"
	sponsoredAccount := "CDTY3P6OVY3SMZXR3DZA667NAXFECA6A3AOZXEU33DD2ACBY43CIKDPT"

	contractIDBytes := strkey.MustDecode(strkey.VersionByteContract, "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC")
	var contractID xdr.Hash
	copy(contractID[:], contractIDBytes)
	contractAddress := xdr.ScAddress{
		Type:       xdr.ScAddressTypeScAddressTypeContract,
		ContractId: (*xdr.ContractId)(&contractID),
	}
	functionSymbol := xdr.ScSymbol("transfer")

	validOp := xdr.InvokeHostFunctionOp{
		HostFunction: xdr.HostFunction{
			Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
			InvokeContract: &xdr.InvokeContractArgs{
				ContractAddress: contractAddress,
				FunctionName:    functionSymbol,
				Args:            []xdr.ScVal{},
			},
		},
		Auth: []xdr.SorobanAuthorizationEntry{},
	}

	validOpXDRBytes, err := validOp.MarshalBinary()
	require.NoError(t, err)
	validOpXDR := base64.StdEncoding.EncodeToString(validOpXDRBytes)

	t.Run("input validation", func(t *testing.T) {
		engine := &engine.SubmitterEngine{
			MaxBaseFee: 100,
		}
		rpcClient := &mocks.MockRPCClient{}
		monitorSvc := tssMonitor.TSSMonitorService{
			Client: &sdpMonitor.MockMonitorClient{},
		}
		sponsoredHandler, err := NewSponsoredTransactionHandler(engine, rpcClient, monitorSvc)
		require.NoError(t, err)

		testCases := []struct {
			name          string
			txJob         *TxJob
			expectedError string
		}{
			{
				name: "returns an error if sponsored account is empty",
				txJob: &TxJob{
					Transaction: store.Transaction{
						Sponsored: store.Sponsored{
							SponsoredAccount:      "",
							SponsoredOperationXDR: validOpXDR,
						},
					},
					ChannelAccount: store.ChannelAccount{
						PublicKey: channelAccount,
					},
					LockedUntilLedgerNumber: 12345,
				},
				expectedError: "sponsored account cannot be empty",
			},
			{
				name: "returns an error if sponsored operation XDR is empty",
				txJob: &TxJob{
					Transaction: store.Transaction{
						Sponsored: store.Sponsored{
							SponsoredAccount:      sponsoredAccount,
							SponsoredOperationXDR: "",
						},
					},
					ChannelAccount: store.ChannelAccount{
						PublicKey: channelAccount,
					},
					LockedUntilLedgerNumber: 12345,
				},
				expectedError: "sponsored operation XDR cannot be empty",
			},
			{
				name: "returns an error if sponsored account is not a valid contract address",
				txJob: &TxJob{
					Transaction: store.Transaction{
						Sponsored: store.Sponsored{
							SponsoredAccount:      "INVALID_ADDRESS",
							SponsoredOperationXDR: validOpXDR,
						},
					},
					ChannelAccount: store.ChannelAccount{
						PublicKey: channelAccount,
					},
					LockedUntilLedgerNumber: 12345,
				},
				expectedError: "sponsored account is not a valid contract address",
			},
			{
				name: "returns an error if operation XDR is invalid base64",
				txJob: &TxJob{
					Transaction: store.Transaction{
						Sponsored: store.Sponsored{
							SponsoredAccount:      sponsoredAccount,
							SponsoredOperationXDR: "invalid-base64",
						},
					},
					ChannelAccount: store.ChannelAccount{
						PublicKey: channelAccount,
					},
					LockedUntilLedgerNumber: 12345,
				},
				expectedError: "sponsored operation XDR is not valid base64",
			},
			{
				name: "returns an error if operation XDR is not valid XDR",
				txJob: &TxJob{
					Transaction: store.Transaction{
						Sponsored: store.Sponsored{
							SponsoredAccount:      sponsoredAccount,
							SponsoredOperationXDR: "dGVzdA==",
						},
					},
					ChannelAccount: store.ChannelAccount{
						PublicKey: channelAccount,
					},
					LockedUntilLedgerNumber: 12345,
				},
				expectedError: "sponsored operation XDR is not valid",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				tx, err := sponsoredHandler.BuildInnerTransaction(ctx, tc.txJob, 100, distributionAccount)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
				assert.Nil(t, tx)
			})
		}
	})

	t.Run("host function validation", func(t *testing.T) {
		engine := &engine.SubmitterEngine{MaxBaseFee: 100}
		rpcClient := &mocks.MockRPCClient{}
		monitorSvc := tssMonitor.TSSMonitorService{
			Client: &sdpMonitor.MockMonitorClient{},
		}
		sponsoredHandler, err := NewSponsoredTransactionHandler(engine, rpcClient, monitorSvc)
		require.NoError(t, err)

		accountID := xdr.MustAddress(distributionAccount)

		invalidOp := xdr.InvokeHostFunctionOp{
			HostFunction: xdr.HostFunction{
				Type: xdr.HostFunctionTypeHostFunctionTypeCreateContractV2,
				CreateContractV2: &xdr.CreateContractArgsV2{
					ContractIdPreimage: xdr.ContractIdPreimage{
						Type: xdr.ContractIdPreimageTypeContractIdPreimageFromAddress,
						FromAddress: &xdr.ContractIdPreimageFromAddress{
							Address: xdr.ScAddress{
								Type:      xdr.ScAddressTypeScAddressTypeAccount,
								AccountId: &accountID,
							},
						},
					},
					Executable: xdr.ContractExecutable{
						Type: xdr.ContractExecutableTypeContractExecutableStellarAsset,
					},
				},
			},
			Auth: []xdr.SorobanAuthorizationEntry{},
		}

		invalidOpXDRBytes, err := invalidOp.MarshalBinary()
		require.NoError(t, err)
		invalidOpXDR := base64.StdEncoding.EncodeToString(invalidOpXDRBytes)

		txJob := &TxJob{
			Transaction: store.Transaction{
				Sponsored: store.Sponsored{
					SponsoredAccount:      sponsoredAccount,
					SponsoredOperationXDR: invalidOpXDR,
				},
			},
			ChannelAccount: store.ChannelAccount{
				PublicKey: channelAccount,
			},
			LockedUntilLedgerNumber: 12345,
		}

		tx, err := sponsoredHandler.BuildInnerTransaction(ctx, txJob, 100, distributionAccount)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported host function type")
		assert.Nil(t, tx)
	})

	t.Run("auth validation", func(t *testing.T) {
		engine := &engine.SubmitterEngine{MaxBaseFee: 100}
		rpcClient := &mocks.MockRPCClient{}
		monitorSvc := tssMonitor.TSSMonitorService{
			Client: &sdpMonitor.MockMonitorClient{},
		}
		sponsoredHandler, err := NewSponsoredTransactionHandler(engine, rpcClient, monitorSvc)
		require.NoError(t, err)

		t.Run("rejects operation requiring auth from channel account", func(t *testing.T) {
			channelAccountID := xdr.MustAddress(channelAccount)
			authEntry := xdr.SorobanAuthorizationEntry{
				Credentials: xdr.SorobanCredentials{
					Type: xdr.SorobanCredentialsTypeSorobanCredentialsAddress,
					Address: &xdr.SorobanAddressCredentials{
						Address: xdr.ScAddress{
							Type:      xdr.ScAddressTypeScAddressTypeAccount,
							AccountId: &channelAccountID,
						},
						Nonce:                     1,
						SignatureExpirationLedger: 100,
						Signature:                 xdr.ScVal{Type: xdr.ScValTypeScvVoid},
					},
				},
				RootInvocation: xdr.SorobanAuthorizedInvocation{
					Function: xdr.SorobanAuthorizedFunction{
						Type: xdr.SorobanAuthorizedFunctionTypeSorobanAuthorizedFunctionTypeContractFn,
						ContractFn: &xdr.InvokeContractArgs{
							ContractAddress: contractAddress,
							FunctionName:    functionSymbol,
							Args:            []xdr.ScVal{},
						},
					},
					SubInvocations: []xdr.SorobanAuthorizedInvocation{},
				},
			}

			opWithChannelAuth := xdr.InvokeHostFunctionOp{
				HostFunction: xdr.HostFunction{
					Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
					InvokeContract: &xdr.InvokeContractArgs{
						ContractAddress: contractAddress,
						FunctionName:    functionSymbol,
						Args:            []xdr.ScVal{},
					},
				},
				Auth: []xdr.SorobanAuthorizationEntry{authEntry},
			}

			opXDRBytes, err := opWithChannelAuth.MarshalBinary()
			require.NoError(t, err)
			opXDR := base64.StdEncoding.EncodeToString(opXDRBytes)

			txJob := &TxJob{
				Transaction: store.Transaction{
					Sponsored: store.Sponsored{
						SponsoredAccount:      sponsoredAccount,
						SponsoredOperationXDR: opXDR,
					},
				},
				ChannelAccount: store.ChannelAccount{
					PublicKey: channelAccount,
				},
				LockedUntilLedgerNumber: 12345,
			}

			tx, err := sponsoredHandler.BuildInnerTransaction(ctx, txJob, 100, distributionAccount)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "sponsored operation cannot require authorization from channel account")
			assert.Nil(t, tx)
		})

		t.Run("rejects operation requiring auth from distribution account", func(t *testing.T) {
			distributionAccountID := xdr.MustAddress(distributionAccount)
			authEntry := xdr.SorobanAuthorizationEntry{
				Credentials: xdr.SorobanCredentials{
					Type: xdr.SorobanCredentialsTypeSorobanCredentialsAddress,
					Address: &xdr.SorobanAddressCredentials{
						Address: xdr.ScAddress{
							Type:      xdr.ScAddressTypeScAddressTypeAccount,
							AccountId: &distributionAccountID,
						},
						Nonce:                     1,
						SignatureExpirationLedger: 100,
						Signature:                 xdr.ScVal{Type: xdr.ScValTypeScvVoid},
					},
				},
				RootInvocation: xdr.SorobanAuthorizedInvocation{
					Function: xdr.SorobanAuthorizedFunction{
						Type: xdr.SorobanAuthorizedFunctionTypeSorobanAuthorizedFunctionTypeContractFn,
						ContractFn: &xdr.InvokeContractArgs{
							ContractAddress: contractAddress,
							FunctionName:    functionSymbol,
							Args:            []xdr.ScVal{},
						},
					},
					SubInvocations: []xdr.SorobanAuthorizedInvocation{},
				},
			}

			opWithDistributionAuth := xdr.InvokeHostFunctionOp{
				HostFunction: xdr.HostFunction{
					Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
					InvokeContract: &xdr.InvokeContractArgs{
						ContractAddress: contractAddress,
						FunctionName:    functionSymbol,
						Args:            []xdr.ScVal{},
					},
				},
				Auth: []xdr.SorobanAuthorizationEntry{authEntry},
			}

			opXDRBytes, err := opWithDistributionAuth.MarshalBinary()
			require.NoError(t, err)
			opXDR := base64.StdEncoding.EncodeToString(opXDRBytes)

			txJob := &TxJob{
				Transaction: store.Transaction{
					Sponsored: store.Sponsored{
						SponsoredAccount:      sponsoredAccount,
						SponsoredOperationXDR: opXDR,
					},
				},
				ChannelAccount: store.ChannelAccount{
					PublicKey: channelAccount,
				},
				LockedUntilLedgerNumber: 12345,
			}

			tx, err := sponsoredHandler.BuildInnerTransaction(ctx, txJob, 100, distributionAccount)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "sponsored operation cannot require authorization from distribution account")
			assert.Nil(t, tx)
		})

		t.Run("rejects operation requiring auth from channel muxed account", func(t *testing.T) {
			channelAccountID := xdr.MustAddress(channelAccount)
			authEntry := xdr.SorobanAuthorizationEntry{
				Credentials: xdr.SorobanCredentials{
					Type: xdr.SorobanCredentialsTypeSorobanCredentialsAddress,
					Address: &xdr.SorobanAddressCredentials{
						Address: xdr.ScAddress{
							Type: xdr.ScAddressTypeScAddressTypeMuxedAccount,
							MuxedAccount: &xdr.MuxedEd25519Account{
								Id:      1,
								Ed25519: channelAccountID.MustEd25519(),
							},
						},
						Nonce:                     1,
						SignatureExpirationLedger: 100,
						Signature:                 xdr.ScVal{Type: xdr.ScValTypeScvVoid},
					},
				},
				RootInvocation: xdr.SorobanAuthorizedInvocation{
					Function: xdr.SorobanAuthorizedFunction{
						Type: xdr.SorobanAuthorizedFunctionTypeSorobanAuthorizedFunctionTypeContractFn,
						ContractFn: &xdr.InvokeContractArgs{
							ContractAddress: contractAddress,
							FunctionName:    functionSymbol,
							Args:            []xdr.ScVal{},
						},
					},
					SubInvocations: []xdr.SorobanAuthorizedInvocation{},
				},
			}

			opWithChannelMuxedAuth := xdr.InvokeHostFunctionOp{
				HostFunction: xdr.HostFunction{
					Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
					InvokeContract: &xdr.InvokeContractArgs{
						ContractAddress: contractAddress,
						FunctionName:    functionSymbol,
						Args:            []xdr.ScVal{},
					},
				},
				Auth: []xdr.SorobanAuthorizationEntry{authEntry},
			}

			opXDRBytes, err := opWithChannelMuxedAuth.MarshalBinary()
			require.NoError(t, err)
			opXDR := base64.StdEncoding.EncodeToString(opXDRBytes)

			txJob := &TxJob{
				Transaction: store.Transaction{
					Sponsored: store.Sponsored{
						SponsoredAccount:      sponsoredAccount,
						SponsoredOperationXDR: opXDR,
					},
				},
				ChannelAccount: store.ChannelAccount{
					PublicKey: channelAccount,
				},
				LockedUntilLedgerNumber: 12345,
			}

			tx, err := sponsoredHandler.BuildInnerTransaction(ctx, txJob, 100, distributionAccount)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "sponsored operation cannot require authorization from channel account")
			assert.Nil(t, tx)
		})

		t.Run("rejects operation requiring auth from distribution muxed account", func(t *testing.T) {
			distributionAccountID := xdr.MustAddress(distributionAccount)
			authEntry := xdr.SorobanAuthorizationEntry{
				Credentials: xdr.SorobanCredentials{
					Type: xdr.SorobanCredentialsTypeSorobanCredentialsAddress,
					Address: &xdr.SorobanAddressCredentials{
						Address: xdr.ScAddress{
							Type: xdr.ScAddressTypeScAddressTypeMuxedAccount,
							MuxedAccount: &xdr.MuxedEd25519Account{
								Id:      2,
								Ed25519: distributionAccountID.MustEd25519(),
							},
						},
						Nonce:                     1,
						SignatureExpirationLedger: 100,
						Signature:                 xdr.ScVal{Type: xdr.ScValTypeScvVoid},
					},
				},
				RootInvocation: xdr.SorobanAuthorizedInvocation{
					Function: xdr.SorobanAuthorizedFunction{
						Type: xdr.SorobanAuthorizedFunctionTypeSorobanAuthorizedFunctionTypeContractFn,
						ContractFn: &xdr.InvokeContractArgs{
							ContractAddress: contractAddress,
							FunctionName:    functionSymbol,
							Args:            []xdr.ScVal{},
						},
					},
					SubInvocations: []xdr.SorobanAuthorizedInvocation{},
				},
			}

			opWithDistributionMuxedAuth := xdr.InvokeHostFunctionOp{
				HostFunction: xdr.HostFunction{
					Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
					InvokeContract: &xdr.InvokeContractArgs{
						ContractAddress: contractAddress,
						FunctionName:    functionSymbol,
						Args:            []xdr.ScVal{},
					},
				},
				Auth: []xdr.SorobanAuthorizationEntry{authEntry},
			}

			opXDRBytes, err := opWithDistributionMuxedAuth.MarshalBinary()
			require.NoError(t, err)
			opXDR := base64.StdEncoding.EncodeToString(opXDRBytes)

			txJob := &TxJob{
				Transaction: store.Transaction{
					Sponsored: store.Sponsored{
						SponsoredAccount:      sponsoredAccount,
						SponsoredOperationXDR: opXDR,
					},
				},
				ChannelAccount: store.ChannelAccount{
					PublicKey: channelAccount,
				},
				LockedUntilLedgerNumber: 12345,
			}

			tx, err := sponsoredHandler.BuildInnerTransaction(ctx, txJob, 100, distributionAccount)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "sponsored operation cannot require authorization from distribution account")
			assert.Nil(t, tx)
		})

		t.Run("accepts operation requiring auth from other accounts", func(t *testing.T) {
			otherAccount := "GCLWGQPMKXQSPF776IU33AH4PZNOOWNAWGGKVTBQMIC5IMKUNP3E6NVU"
			otherAccountID := xdr.MustAddress(otherAccount)
			authEntry := xdr.SorobanAuthorizationEntry{
				Credentials: xdr.SorobanCredentials{
					Type: xdr.SorobanCredentialsTypeSorobanCredentialsAddress,
					Address: &xdr.SorobanAddressCredentials{
						Address: xdr.ScAddress{
							Type:      xdr.ScAddressTypeScAddressTypeAccount,
							AccountId: &otherAccountID,
						},
						Nonce:                     1,
						SignatureExpirationLedger: 100,
						Signature:                 xdr.ScVal{Type: xdr.ScValTypeScvVoid},
					},
				},
				RootInvocation: xdr.SorobanAuthorizedInvocation{
					Function: xdr.SorobanAuthorizedFunction{
						Type: xdr.SorobanAuthorizedFunctionTypeSorobanAuthorizedFunctionTypeContractFn,
						ContractFn: &xdr.InvokeContractArgs{
							ContractAddress: contractAddress,
							FunctionName:    functionSymbol,
							Args:            []xdr.ScVal{},
						},
					},
					SubInvocations: []xdr.SorobanAuthorizedInvocation{},
				},
			}

			opWithOtherAuth := xdr.InvokeHostFunctionOp{
				HostFunction: xdr.HostFunction{
					Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
					InvokeContract: &xdr.InvokeContractArgs{
						ContractAddress: contractAddress,
						FunctionName:    functionSymbol,
						Args:            []xdr.ScVal{},
					},
				},
				Auth: []xdr.SorobanAuthorizationEntry{authEntry},
			}

			opXDRBytes, err := opWithOtherAuth.MarshalBinary()
			require.NoError(t, err)
			opXDR := base64.StdEncoding.EncodeToString(opXDRBytes)

			simulationResponse := protocol.SimulateTransactionResponse{
				Error:              "",
				TransactionDataXDR: "AAAAAAAAAAUAAAAAAAAAAI6zjC5RtJsxMAzXJfbm813ySujUwQVm4r2uHtkav62tAAAABgAAAAFDEqQxRKsWsubOpgtPPKXSsdhcWDpfu/jRwXKpUugxhQAAABQAAAABAAAABgAAAAGBhvDmuHDARIUDYKVFokPXfBrz+6tx3N4D7hMpL1AiBwAAABQAAAABAAAAB1uPeA45/uPdYS9GdAZXx37bjezG+3vn4JqEwlyjIRGmAAAAB9nC+GOHmV4+xAZQ4T0I434wH3LKi+db6CM9hlRZhRZgAAAAAgAAAAYAAAAAAAAAAI6zjC5RtJsxMAzXJfbm813ySujUwQVm4r2uHtkav62tAAAAFXCHgz/4M7a3AAAAAAAAAAYAAAABQxKkMUSrFrLmzqYLTzyl0rHYXFg6X7v40cFyqVLoMYUAAAAVNIwhp30FbW4AAAAAAB8NxwAAEgAAAACUAAAAAAAY7m4=",
				MinResourceFee:     50,
			}

			rpcClient := &mocks.MockRPCClient{}
			rpcClient.On("SimulateTransaction", mock.Anything, mock.Anything).Return(&stellar.SimulationResult{Response: simulationResponse}, (*stellar.SimulationError)(nil))

			monitorSvc := tssMonitor.TSSMonitorService{
				Client: &sdpMonitor.MockMonitorClient{},
			}
			handler, err := NewSponsoredTransactionHandler(engine, rpcClient, monitorSvc)
			require.NoError(t, err)

			txJob := &TxJob{
				Transaction: store.Transaction{
					Sponsored: store.Sponsored{
						SponsoredAccount:      sponsoredAccount,
						SponsoredOperationXDR: opXDR,
					},
				},
				ChannelAccount: store.ChannelAccount{
					PublicKey: channelAccount,
				},
				LockedUntilLedgerNumber: 12345,
			}

			tx, err := handler.BuildInnerTransaction(ctx, txJob, 100, distributionAccount)
			require.NoError(t, err)
			assert.NotNil(t, tx)

			rpcClient.AssertExpectations(t)
		})

		t.Run("accepts operation with contract auth", func(t *testing.T) {
			authContractIDBytes := strkey.MustDecode(strkey.VersionByteContract, "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC")
			var authContractID xdr.Hash
			copy(authContractID[:], authContractIDBytes)

			authEntry := xdr.SorobanAuthorizationEntry{
				Credentials: xdr.SorobanCredentials{
					Type: xdr.SorobanCredentialsTypeSorobanCredentialsAddress,
					Address: &xdr.SorobanAddressCredentials{
						Address: xdr.ScAddress{
							Type:       xdr.ScAddressTypeScAddressTypeContract,
							ContractId: (*xdr.ContractId)(&authContractID),
						},
						Nonce:                     1,
						SignatureExpirationLedger: 100,
						Signature:                 xdr.ScVal{Type: xdr.ScValTypeScvVoid},
					},
				},
				RootInvocation: xdr.SorobanAuthorizedInvocation{
					Function: xdr.SorobanAuthorizedFunction{
						Type: xdr.SorobanAuthorizedFunctionTypeSorobanAuthorizedFunctionTypeContractFn,
						ContractFn: &xdr.InvokeContractArgs{
							ContractAddress: contractAddress,
							FunctionName:    functionSymbol,
							Args:            []xdr.ScVal{},
						},
					},
					SubInvocations: []xdr.SorobanAuthorizedInvocation{},
				},
			}

			opWithContractAuth := xdr.InvokeHostFunctionOp{
				HostFunction: xdr.HostFunction{
					Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
					InvokeContract: &xdr.InvokeContractArgs{
						ContractAddress: contractAddress,
						FunctionName:    functionSymbol,
						Args:            []xdr.ScVal{},
					},
				},
				Auth: []xdr.SorobanAuthorizationEntry{authEntry},
			}

			opXDRBytes, err := opWithContractAuth.MarshalBinary()
			require.NoError(t, err)
			opXDR := base64.StdEncoding.EncodeToString(opXDRBytes)

			simulationResponse := protocol.SimulateTransactionResponse{
				Error:              "",
				TransactionDataXDR: "AAAAAAAAAAUAAAAAAAAAAI6zjC5RtJsxMAzXJfbm813ySujUwQVm4r2uHtkav62tAAAABgAAAAFDEqQxRKsWsubOpgtPPKXSsdhcWDpfu/jRwXKpUugxhQAAABQAAAABAAAABgAAAAGBhvDmuHDARIUDYKVFokPXfBrz+6tx3N4D7hMpL1AiBwAAABQAAAABAAAAB1uPeA45/uPdYS9GdAZXx37bjezG+3vn4JqEwlyjIRGmAAAAB9nC+GOHmV4+xAZQ4T0I434wH3LKi+db6CM9hlRZhRZgAAAAAgAAAAYAAAAAAAAAAI6zjC5RtJsxMAzXJfbm813ySujUwQVm4r2uHtkav62tAAAAFXCHgz/4M7a3AAAAAAAAAAYAAAABQxKkMUSrFrLmzqYLTzyl0rHYXFg6X7v40cFyqVLoMYUAAAAVNIwhp30FbW4AAAAAAB8NxwAAEgAAAACUAAAAAAAY7m4=",
				MinResourceFee:     50,
			}

			rpcClient := &mocks.MockRPCClient{}
			rpcClient.On("SimulateTransaction", mock.Anything, mock.Anything).Return(&stellar.SimulationResult{Response: simulationResponse}, (*stellar.SimulationError)(nil))

			monitorSvc := tssMonitor.TSSMonitorService{
				Client: &sdpMonitor.MockMonitorClient{},
			}
			handler, err := NewSponsoredTransactionHandler(engine, rpcClient, monitorSvc)
			require.NoError(t, err)

			txJob := &TxJob{
				Transaction: store.Transaction{
					Sponsored: store.Sponsored{
						SponsoredAccount:      sponsoredAccount,
						SponsoredOperationXDR: opXDR,
					},
				},
				ChannelAccount: store.ChannelAccount{
					PublicKey: channelAccount,
				},
				LockedUntilLedgerNumber: 12345,
			}

			tx, err := handler.BuildInnerTransaction(ctx, txJob, 100, distributionAccount)
			require.NoError(t, err)
			assert.NotNil(t, tx)

			rpcClient.AssertExpectations(t)
		})

		t.Run("rejects operation with invalid auth credentials type", func(t *testing.T) {
			authEntry := xdr.SorobanAuthorizationEntry{
				Credentials: xdr.SorobanCredentials{
					Type: xdr.SorobanCredentialsTypeSorobanCredentialsSourceAccount,
				},
				RootInvocation: xdr.SorobanAuthorizedInvocation{
					Function: xdr.SorobanAuthorizedFunction{
						Type: xdr.SorobanAuthorizedFunctionTypeSorobanAuthorizedFunctionTypeContractFn,
						ContractFn: &xdr.InvokeContractArgs{
							ContractAddress: contractAddress,
							FunctionName:    functionSymbol,
							Args:            []xdr.ScVal{},
						},
					},
					SubInvocations: []xdr.SorobanAuthorizedInvocation{},
				},
			}

			opWithInvalidAuth := xdr.InvokeHostFunctionOp{
				HostFunction: xdr.HostFunction{
					Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
					InvokeContract: &xdr.InvokeContractArgs{
						ContractAddress: contractAddress,
						FunctionName:    functionSymbol,
						Args:            []xdr.ScVal{},
					},
				},
				Auth: []xdr.SorobanAuthorizationEntry{authEntry},
			}

			opXDRBytes, err := opWithInvalidAuth.MarshalBinary()
			require.NoError(t, err)
			opXDR := base64.StdEncoding.EncodeToString(opXDRBytes)

			txJob := &TxJob{
				Transaction: store.Transaction{
					Sponsored: store.Sponsored{
						SponsoredAccount:      sponsoredAccount,
						SponsoredOperationXDR: opXDR,
					},
				},
				ChannelAccount: store.ChannelAccount{
					PublicKey: channelAccount,
				},
				LockedUntilLedgerNumber: 12345,
			}

			tx, err := sponsoredHandler.BuildInnerTransaction(ctx, txJob, 100, distributionAccount)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid auth credentials type")
			assert.Nil(t, tx)
		})
	})

	t.Run("🎉 successfully build a transaction", func(t *testing.T) {
		engine := &engine.SubmitterEngine{
			MaxBaseFee: 100,
		}

		simulationResponse := protocol.SimulateTransactionResponse{
			Error:              "",
			TransactionDataXDR: "AAAAAAAAAAUAAAAAAAAAAI6zjC5RtJsxMAzXJfbm813ySujUwQVm4r2uHtkav62tAAAABgAAAAFDEqQxRKsWsubOpgtPPKXSsdhcWDpfu/jRwXKpUugxhQAAABQAAAABAAAABgAAAAGBhvDmuHDARIUDYKVFokPXfBrz+6tx3N4D7hMpL1AiBwAAABQAAAABAAAAB1uPeA45/uPdYS9GdAZXx37bjezG+3vn4JqEwlyjIRGmAAAAB9nC+GOHmV4+xAZQ4T0I434wH3LKi+db6CM9hlRZhRZgAAAAAgAAAAYAAAAAAAAAAI6zjC5RtJsxMAzXJfbm813ySujUwQVm4r2uHtkav62tAAAAFXCHgz/4M7a3AAAAAAAAAAYAAAABQxKkMUSrFrLmzqYLTzyl0rHYXFg6X7v40cFyqVLoMYUAAAAVNIwhp30FbW4AAAAAAB8NxwAAEgAAAACUAAAAAAAY7m4=",
			MinResourceFee:     50,
		}

		rpcClient := &mocks.MockRPCClient{}
		rpcClient.On("SimulateTransaction", mock.Anything, mock.Anything).Return(&stellar.SimulationResult{Response: simulationResponse}, (*stellar.SimulationError)(nil))

		monitorSvc := tssMonitor.TSSMonitorService{
			Client: &sdpMonitor.MockMonitorClient{},
		}
		sponsoredHandler, err := NewSponsoredTransactionHandler(engine, rpcClient, monitorSvc)
		require.NoError(t, err)

		txJob := &TxJob{
			Transaction: store.Transaction{
				Sponsored: store.Sponsored{
					SponsoredAccount:      sponsoredAccount,
					SponsoredOperationXDR: validOpXDR,
				},
			},
			ChannelAccount: store.ChannelAccount{
				PublicKey: channelAccount,
			},
			LockedUntilLedgerNumber: 12345,
		}

		tx, err := sponsoredHandler.BuildInnerTransaction(ctx, txJob, 100, distributionAccount)
		require.NoError(t, err)

		var transactionData xdr.SorobanTransactionData
		require.NoError(t, xdr.SafeUnmarshalBase64(simulationResponse.TransactionDataXDR, &transactionData))

		var decodedOp xdr.InvokeHostFunctionOp
		require.NoError(t, xdr.SafeUnmarshalBase64(validOpXDR, &decodedOp))

		sponsoredOperation := &txnbuild.InvokeHostFunction{
			SourceAccount: distributionAccount,
			HostFunction:  decodedOp.HostFunction,
			Auth:          decodedOp.Auth,
			Ext: xdr.TransactionExt{
				V:           1,
				SorobanData: &transactionData,
			},
		}
		wantTx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
			SourceAccount: &txnbuild.SimpleAccount{
				AccountID: channelAccount,
				Sequence:  101,
			},
			IncrementSequenceNum: false,
			BaseFee:              100,
			Operations:           []txnbuild.Operation{sponsoredOperation},
			Preconditions: txnbuild.Preconditions{
				TimeBounds:   tx.Timebounds(),
				LedgerBounds: &txnbuild.LedgerBounds{MaxLedger: uint32(txJob.LockedUntilLedgerNumber)},
			},
		})
		require.NoError(t, err)

		require.Equal(t, wantTx, tx)
		require.InDelta(t, time.Now().Add(300*time.Second).UTC().Unix(), tx.Timebounds().MaxTime, 5)
		rpcClient.AssertExpectations(t)
	})

	t.Run("simulation error handling", func(t *testing.T) {
		engine := &engine.SubmitterEngine{
			MaxBaseFee: 100,
		}

		simulationError := &stellar.SimulationError{
			Type:     stellar.SimulationErrorTypeContractExecution,
			Err:      errors.New("contract execution failed"),
			Response: &protocol.SimulateTransactionResponse{Error: "contract execution failed"},
		}

		rpcClient := &mocks.MockRPCClient{}
		rpcClient.On("SimulateTransaction", mock.Anything, mock.Anything).Return((*stellar.SimulationResult)(nil), simulationError)

		monitorSvc := tssMonitor.TSSMonitorService{
			Client: &sdpMonitor.MockMonitorClient{},
		}
		sponsoredHandler, err := NewSponsoredTransactionHandler(engine, rpcClient, monitorSvc)
		require.NoError(t, err)

		txJob := &TxJob{
			Transaction: store.Transaction{
				Sponsored: store.Sponsored{
					SponsoredAccount:      sponsoredAccount,
					SponsoredOperationXDR: validOpXDR,
				},
			},
			ChannelAccount: store.ChannelAccount{
				PublicKey: channelAccount,
			},
			LockedUntilLedgerNumber: 12345,
		}

		tx, err := sponsoredHandler.BuildInnerTransaction(ctx, txJob, 100, distributionAccount)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "contract execution failed")
		assert.Nil(t, tx)

		rpcClient.AssertExpectations(t)
	})

	t.Run("rpc client error handling", func(t *testing.T) {
		engine := &engine.SubmitterEngine{
			MaxBaseFee: 100,
		}

		networkError := &stellar.SimulationError{
			Type:     stellar.SimulationErrorTypeNetwork,
			Err:      fmt.Errorf("rpc error"),
			Response: nil,
		}

		rpcClient := &mocks.MockRPCClient{}
		rpcClient.On("SimulateTransaction", mock.Anything, mock.Anything).Return((*stellar.SimulationResult)(nil), networkError)

		monitorSvc := tssMonitor.TSSMonitorService{
			Client: &sdpMonitor.MockMonitorClient{},
		}
		sponsoredHandler, err := NewSponsoredTransactionHandler(engine, rpcClient, monitorSvc)
		require.NoError(t, err)

		txJob := &TxJob{
			Transaction: store.Transaction{
				Sponsored: store.Sponsored{
					SponsoredAccount:      sponsoredAccount,
					SponsoredOperationXDR: validOpXDR,
				},
			},
			ChannelAccount: store.ChannelAccount{
				PublicKey: channelAccount,
			},
			LockedUntilLedgerNumber: 12345,
		}

		tx, err := sponsoredHandler.BuildInnerTransaction(ctx, txJob, 100, distributionAccount)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "rpc error")
		assert.Nil(t, tx)

		var rpcErr *utils.RPCErrorWrapper
		if assert.ErrorAs(t, err, &rpcErr) {
			assert.True(t, rpcErr.IsRPCError())
		}

		rpcClient.AssertExpectations(t)
	})
}

func Test_SponsoredTransactionHandler_MonitorTransactionProcessingStarted(t *testing.T) {
	ctx := context.Background()
	txJob := TxJob{
		Transaction: store.Transaction{},
	}
	jobUUID := "job-uuid"

	mMonitorClient := sdpMonitor.NewMockMonitorClient(t)
	mMonitorClient.
		On("MonitorCounters", sdpMonitor.SponsoredTransactionProcessingStartedTag, mock.Anything).
		Return(nil).
		Once()
	tssMonitorService := tssMonitor.TSSMonitorService{
		Version:       "0.01",
		GitCommitHash: "0xABC",
		Client:        mMonitorClient,
	}
	sponsoredHandler := &SponsoredTransactionHandler{
		monitorSvc: tssMonitorService,
	}

	sponsoredHandler.MonitorTransactionProcessingStarted(ctx, &txJob, jobUUID)
}

func Test_SponsoredTransactionHandler_MonitorTransactionProcessingSuccess(t *testing.T) {
	ctx := context.Background()
	txJob := TxJob{
		Transaction: store.Transaction{
			CompletedAt:            func() *time.Time { t := time.Now(); return &t }(),
			AttemptsCount:          1,
			XDRSent:                sql.NullString{String: "xdr-sent", Valid: true},
			XDRReceived:            sql.NullString{String: "xdr-received", Valid: true},
			StellarTransactionHash: sql.NullString{String: "tx-hash", Valid: true},
		},
		ChannelAccount: store.ChannelAccount{
			PublicKey: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
		},
	}
	jobUUID := "job-uuid"

	mMonitorClient := sdpMonitor.NewMockMonitorClient(t)
	mMonitorClient.
		On("MonitorCounters", sdpMonitor.SponsoredTransactionTransactionSuccessfulTag, mock.Anything).
		Return(nil).
		Once()
	tssMonitorService := tssMonitor.TSSMonitorService{
		Version:       "0.01",
		GitCommitHash: "0xABC",
		Client:        mMonitorClient,
	}
	sponsoredHandler := &SponsoredTransactionHandler{
		monitorSvc: tssMonitorService,
	}

	sponsoredHandler.MonitorTransactionProcessingSuccess(ctx, &txJob, jobUUID)
}

func Test_SponsoredTransactionHandler_MonitorTransactionProcessingFailed(t *testing.T) {
	ctx := context.Background()
	txJob := TxJob{
		Transaction: store.Transaction{
			CompletedAt:            func() *time.Time { t := time.Now(); return &t }(),
			AttemptsCount:          1,
			XDRSent:                sql.NullString{String: "xdr-sent", Valid: true},
			XDRReceived:            sql.NullString{String: "xdr-received", Valid: true},
			StellarTransactionHash: sql.NullString{String: "tx-hash", Valid: true},
		},
		ChannelAccount: store.ChannelAccount{
			PublicKey: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
		},
	}
	jobUUID := "job-uuid"
	isRetryable := true
	errStack := "error stack"

	mMonitorClient := sdpMonitor.NewMockMonitorClient(t)
	mMonitorClient.
		On("MonitorCounters", sdpMonitor.SponsoredTransactionErrorTag, mock.Anything).
		Return(nil).
		Once()
	tssMonitorService := tssMonitor.TSSMonitorService{
		Version:       "0.01",
		GitCommitHash: "0xABC",
		Client:        mMonitorClient,
	}
	sponsoredHandler := &SponsoredTransactionHandler{
		monitorSvc: tssMonitorService,
	}

	sponsoredHandler.MonitorTransactionProcessingFailed(ctx, &txJob, jobUUID, isRetryable, errStack)
}

func Test_SponsoredTransactionHandler_MonitorTransactionReconciliationSuccess(t *testing.T) {
	ctx := context.Background()
	txJob := TxJob{
		Transaction: store.Transaction{
			CompletedAt:            func() *time.Time { t := time.Now(); return &t }(),
			XDRSent:                sql.NullString{String: "xdr-sent", Valid: true},
			XDRReceived:            sql.NullString{String: "xdr-received", Valid: true},
			StellarTransactionHash: sql.NullString{String: "tx-hash", Valid: true},
		},
		ChannelAccount: store.ChannelAccount{
			PublicKey: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
		},
	}
	jobUUID := "job-uuid"

	mMonitorClient := sdpMonitor.NewMockMonitorClient(t)
	mMonitorClient.
		On("MonitorCounters", sdpMonitor.SponsoredTransactionReconciliationSuccessfulTag, mock.Anything).
		Return(nil).
		Once()
	tssMonitorService := tssMonitor.TSSMonitorService{
		Version:       "0.01",
		GitCommitHash: "0xABC",
		Client:        mMonitorClient,
	}
	sponsoredHandler := &SponsoredTransactionHandler{
		monitorSvc: tssMonitorService,
	}

	sponsoredHandler.MonitorTransactionReconciliationSuccess(ctx, &txJob, jobUUID, ReconcileSuccess)
}

func Test_SponsoredTransactionHandler_MonitorTransactionReconciliationFailure(t *testing.T) {
	ctx := context.Background()
	txJob := TxJob{
		Transaction: store.Transaction{
			CompletedAt:            func() *time.Time { t := time.Now(); return &t }(),
			XDRSent:                sql.NullString{String: "xdr-sent", Valid: true},
			XDRReceived:            sql.NullString{String: "xdr-received", Valid: true},
			StellarTransactionHash: sql.NullString{String: "tx-hash", Valid: true},
		},
		ChannelAccount: store.ChannelAccount{
			PublicKey: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
		},
	}
	jobUUID := "job-uuid"
	isHorizonErr := true
	errStack := "error stack"

	mMonitorClient := sdpMonitor.NewMockMonitorClient(t)
	mMonitorClient.
		On("MonitorCounters", sdpMonitor.SponsoredTransactionReconciliationFailureTag, mock.Anything).
		Return(nil).
		Once()
	tssMonitorService := tssMonitor.TSSMonitorService{
		Version:       "0.01",
		GitCommitHash: "0xABC",
		Client:        mMonitorClient,
	}
	sponsoredHandler := &SponsoredTransactionHandler{
		monitorSvc: tssMonitorService,
	}

	sponsoredHandler.MonitorTransactionReconciliationFailure(ctx, &txJob, jobUUID, isHorizonErr, errStack)
}

func Test_SponsoredTransactionHandler_AddContextLoggerFields(t *testing.T) {
	engine := &engine.SubmitterEngine{}
	rpcClient := &mocks.MockRPCClient{}
	monitorSvc := tssMonitor.TSSMonitorService{
		Client: &sdpMonitor.MockMonitorClient{},
	}
	sponsoredHandler, err := NewSponsoredTransactionHandler(engine, rpcClient, monitorSvc)
	require.NoError(t, err)

	sponsoredAccount := "CDTY3P6OVY3SMZXR3DZA667NAXFECA6A3AOZXEU33DD2ACBY43CIKDPT"

	transaction := &store.Transaction{
		Sponsored: store.Sponsored{
			SponsoredAccount: sponsoredAccount,
		},
	}

	fields := sponsoredHandler.AddContextLoggerFields(transaction)

	assert.Equal(t, sponsoredAccount, fields["sponsored_account"])
	assert.Len(t, fields, 1)
}

func Test_SponsoredTransactionHandler_MonitoringBehavior(t *testing.T) {
	ctx := context.Background()
	txJob := &TxJob{
		Transaction: store.Transaction{
			AttemptsCount: 2,
		},
		ChannelAccount: store.ChannelAccount{
			PublicKey: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
		},
	}
	jobUUID := "job-uuid"

	t.Run("MonitorTransactionProcessingSuccess", func(t *testing.T) {
		mMonitorClient := sdpMonitor.NewMockMonitorClient(t)
		mMonitorClient.
			On("MonitorCounters", sdpMonitor.SponsoredTransactionTransactionSuccessfulTag, mock.Anything).
			Return(nil).
			Once()

		tssMonitorService := tssMonitor.TSSMonitorService{
			Client: mMonitorClient,
		}
		sponsoredHandler := &SponsoredTransactionHandler{
			monitorSvc: tssMonitorService,
		}

		sponsoredHandler.MonitorTransactionProcessingSuccess(ctx, txJob, jobUUID)
	})

	t.Run("MonitorTransactionProcessingFailed with retryable error", func(t *testing.T) {
		mMonitorClient := sdpMonitor.NewMockMonitorClient(t)
		mMonitorClient.
			On("MonitorCounters", sdpMonitor.SponsoredTransactionErrorTag, mock.Anything).
			Return(nil).
			Once()

		tssMonitorService := tssMonitor.TSSMonitorService{
			Client: mMonitorClient,
		}
		sponsoredHandler := &SponsoredTransactionHandler{
			monitorSvc: tssMonitorService,
		}

		sponsoredHandler.MonitorTransactionProcessingFailed(ctx, txJob, jobUUID, true, "retryable error")
	})

	t.Run("MonitorTransactionReconciliationSuccess with reprocessing type", func(t *testing.T) {
		mMonitorClient := sdpMonitor.NewMockMonitorClient(t)
		mMonitorClient.
			On("MonitorCounters", sdpMonitor.SponsoredTransactionReconciliationSuccessfulTag, mock.Anything).
			Return(nil).
			Once()

		tssMonitorService := tssMonitor.TSSMonitorService{
			Client: mMonitorClient,
		}
		sponsoredHandler := &SponsoredTransactionHandler{
			monitorSvc: tssMonitorService,
		}

		sponsoredHandler.MonitorTransactionReconciliationSuccess(ctx, txJob, jobUUID, ReconcileReprocessing)
	})
}

func Test_SponsoredTransactionHandler_ApplyTransactionData(t *testing.T) {
	engine := &engine.SubmitterEngine{}
	rpcClient := &mocks.MockRPCClient{}
	monitorSvc := tssMonitor.TSSMonitorService{
		Client: &sdpMonitor.MockMonitorClient{},
	}
	handler, err := NewSponsoredTransactionHandler(engine, rpcClient, monitorSvc)
	require.NoError(t, err)

	t.Run("empty transaction data", func(t *testing.T) {
		operation := &txnbuild.InvokeHostFunction{}
		response := protocol.SimulateTransactionResponse{
			TransactionDataXDR: "",
		}

		err := handler.applyTransactionData(operation, response)
		require.NoError(t, err)

		assert.Equal(t, 0, int(operation.Ext.V))
	})

	t.Run("valid transaction data", func(t *testing.T) {
		operation := &txnbuild.InvokeHostFunction{}
		response := protocol.SimulateTransactionResponse{
			TransactionDataXDR: "AAAAAAAAAAUAAAAAAAAAAI6zjC5RtJsxMAzXJfbm813ySujUwQVm4r2uHtkav62tAAAABgAAAAFDEqQxRKsWsubOpgtPPKXSsdhcWDpfu/jRwXKpUugxhQAAABQAAAABAAAABgAAAAGBhvDmuHDARIUDYKVFokPXfBrz+6tx3N4D7hMpL1AiBwAAABQAAAABAAAAB1uPeA45/uPdYS9GdAZXx37bjezG+3vn4JqEwlyjIRGmAAAAB9nC+GOHmV4+xAZQ4T0I434wH3LKi+db6CM9hlRZhRZgAAAAAgAAAAYAAAAAAAAAAI6zjC5RtJsxMAzXJfbm813ySujUwQVm4r2uHtkav62tAAAAFXCHgz/4M7a3AAAAAAAAAAYAAAABQxKkMUSrFrLmzqYLTzyl0rHYXFg6X7v40cFyqVLoMYUAAAAVNIwhp30FbW4AAAAAAB8NxwAAEgAAAACUAAAAAAAY7m4=",
		}

		err := handler.applyTransactionData(operation, response)
		require.NoError(t, err)

		assert.Equal(t, 1, int(operation.Ext.V))
		assert.NotNil(t, operation.Ext.SorobanData)
	})

	t.Run("invalid transaction data XDR", func(t *testing.T) {
		operation := &txnbuild.InvokeHostFunction{}
		response := protocol.SimulateTransactionResponse{
			TransactionDataXDR: "invalid-base64",
		}

		err := handler.applyTransactionData(operation, response)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshaling transaction data")
	})
}

func Test_SponsoredTransactionHandler_RequiresRebuildOnRetry(t *testing.T) {
	handler := &SponsoredTransactionHandler{}

	result := handler.RequiresRebuildOnRetry()
	assert.False(t, result)
}
