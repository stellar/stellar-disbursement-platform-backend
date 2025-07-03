package transactionsubmission

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/go/xdr"
	"github.com/stellar/stellar-rpc/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	sdpMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	sdpMonitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

func Test_NewWalletCreationTransactionHandler(t *testing.T) {
	rpcClient := &mocks.MockRPCClient{}
	tssMonitorSvc := tssMonitor.TSSMonitorService{
		GitCommitHash: "gitCommitHash0x",
		Version:       "version123",
		Client:        &sdpMonitorMocks.MockMonitorClient{},
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
			wantError:     "rpc client cannot be nil",
		},
		{
			name:      "validate tssMonitorSvc",
			engine:    &engine.SubmitterEngine{},
			rpcClient: rpcClient,
			wantError: "monitor service cannot be nil",
		},
		{
			name:          "🎉 successfully returns a new wallet creation handler",
			engine:        &engine.SubmitterEngine{},
			rpcClient:     rpcClient,
			tssMonitorSvc: tssMonitorSvc,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			walletCreationHandler, err := NewWalletCreationTransactionHandler(tc.engine, tc.rpcClient, tc.tssMonitorSvc)
			if tc.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantError)
				assert.Nil(t, walletCreationHandler)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, walletCreationHandler)
				assert.Equal(t, walletCreationHandler.engine, tc.engine)
				assert.Equal(t, walletCreationHandler.rpcClient, tc.rpcClient)
				assert.Equal(t, walletCreationHandler.monitorSvc, tc.tssMonitorSvc)
			}
		})
	}
}

func Test_WalletCreationHandler_BuildInnerTransaction(t *testing.T) {
	ctx := context.Background()
	distributionAccount := "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"
	channelAccount := "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX"
	publicKeyHex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef01"
	wasmHashHex := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	saltHex := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

	t.Run("input validation", func(t *testing.T) {
		engine := &engine.SubmitterEngine{
			MaxBaseFee: 100,
		}
		rpcClient := &mocks.MockRPCClient{}
		monitorSvc := tssMonitor.TSSMonitorService{
			Client: &sdpMonitorMocks.MockMonitorClient{},
		}
		walletCreationHandler, err := NewWalletCreationTransactionHandler(engine, rpcClient, monitorSvc)
		require.NoError(t, err)

		testCases := []struct {
			name          string
			txJob         *TxJob
			expectedError string
		}{
			{
				name: "returns an error if public key is empty",
				txJob: &TxJob{
					Transaction: store.Transaction{
						WalletCreation: store.WalletCreation{
							PublicKey: "",
							WasmHash:  wasmHashHex,
							Salt:      saltHex,
						},
					},
					ChannelAccount: store.ChannelAccount{
						PublicKey: channelAccount,
					},
					LockedUntilLedgerNumber: 12345,
				},
				expectedError: "public key cannot be empty",
			},
			{
				name: "returns an error if wasm hash is empty",
				txJob: &TxJob{
					Transaction: store.Transaction{
						WalletCreation: store.WalletCreation{
							PublicKey: publicKeyHex,
							WasmHash:  "",
							Salt:      saltHex,
						},
					},
					ChannelAccount: store.ChannelAccount{
						PublicKey: channelAccount,
					},
					LockedUntilLedgerNumber: 12345,
				},
				expectedError: "wasm hash cannot be empty",
			},
			{
				name: "returns an error if public key is invalid hex",
				txJob: &TxJob{
					Transaction: store.Transaction{
						WalletCreation: store.WalletCreation{
							PublicKey: "invalid-hex",
							WasmHash:  wasmHashHex,
							Salt:      saltHex,
						},
					},
					ChannelAccount: store.ChannelAccount{
						PublicKey: channelAccount,
					},
					LockedUntilLedgerNumber: 12345,
				},
				expectedError: "decoding public key",
			},
			{
				name: "returns an error if wasm hash is invalid hex",
				txJob: &TxJob{
					Transaction: store.Transaction{
						WalletCreation: store.WalletCreation{
							PublicKey: publicKeyHex,
							WasmHash:  "invalid-hex",
							Salt:      saltHex,
						},
					},
					ChannelAccount: store.ChannelAccount{
						PublicKey: channelAccount,
					},
					LockedUntilLedgerNumber: 12345,
				},
				expectedError: "decoding wasm hash",
			},
			{
				name: "returns an error if wasm hash is not 32 bytes",
				txJob: &TxJob{
					Transaction: store.Transaction{
						WalletCreation: store.WalletCreation{
							PublicKey: publicKeyHex,
							WasmHash:  "abcdef",
							Salt:      saltHex,
						},
					},
					ChannelAccount: store.ChannelAccount{
						PublicKey: channelAccount,
					},
					LockedUntilLedgerNumber: 12345,
				},
				expectedError: "wasm hash must be 32 bytes",
			},
			{
				name: "returns an error if public key is not 65 bytes",
				txJob: &TxJob{
					Transaction: store.Transaction{
						WalletCreation: store.WalletCreation{
							PublicKey: "0123456789abcdef",
							WasmHash:  wasmHashHex,
							Salt:      saltHex,
						},
					},
					ChannelAccount: store.ChannelAccount{
						PublicKey: channelAccount,
					},
					LockedUntilLedgerNumber: 12345,
				},
				expectedError: "public key must be 65 bytes",
			},
			{
				name: "returns an error if salt is empty",
				txJob: &TxJob{
					Transaction: store.Transaction{
						WalletCreation: store.WalletCreation{
							PublicKey: publicKeyHex,
							WasmHash:  wasmHashHex,
							Salt:      "",
						},
					},
					ChannelAccount: store.ChannelAccount{
						PublicKey: channelAccount,
					},
					LockedUntilLedgerNumber: 12345,
				},
				expectedError: "salt cannot be empty",
			},
			{
				name: "returns an error if salt is invalid hex",
				txJob: &TxJob{
					Transaction: store.Transaction{
						WalletCreation: store.WalletCreation{
							PublicKey: publicKeyHex,
							WasmHash:  wasmHashHex,
							Salt:      "invalid-hex",
						},
					},
					ChannelAccount: store.ChannelAccount{
						PublicKey: channelAccount,
					},
					LockedUntilLedgerNumber: 12345,
				},
				expectedError: "parsing contract salt",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				tx, err := walletCreationHandler.BuildInnerTransaction(ctx, tc.txJob, 100, distributionAccount)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
				assert.Nil(t, tx)
			})
		}
	})

	t.Run("🎉 successfully build a transaction", func(t *testing.T) {
		mHorizonClient := &horizonclient.MockClient{}
		mHorizonClient.On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).Return(
			horizon.Account{
				AccountID: channelAccount,
				Sequence:  int64(123456789),
			}, nil,
		)

		engine := &engine.SubmitterEngine{
			HorizonClient: mHorizonClient,
			MaxBaseFee:    100,
		}

		authXDR := []string{"AAAAAQAAAAHw6CVqzY+dCq3myVJBo1kb3nEGE7oO6obmJeUNvYQ0ukNc84Ms0ZvgAAAAAAAAAAEAAAAAAAAAAeA7wfSg10yaQYZDRmQeyqsepsS/Mb0rbMQxRgDoSVdWAAAAD3dlYl9hdXRoX3ZlcmlmeQAAAAAGAAAADgAAADhDRFlPUUpMS1pXSFoyQ1ZONDNFVkVRTkRMRU41NDRJR0NPNUE1MlVHNFlTNktETjVRUTJMVVdLWQAAAA4AAAADMTIzAAAAAA4AAAAcaHR0cDovL2xvY2FsaG9zdDo4MDgwL2MvYXV0aAAAAA4AAAAObG9jYWxob3N0OjgwODAAAAAAAA4AAAALZXhhbXBsZS5jb20AAAAAAQAAAAA="}
		simulationResponse := protocol.SimulateTransactionResponse{
			Error: "",
			Results: []protocol.SimulateHostFunctionResult{
				{
					AuthXDR: &authXDR,
				},
			},
			TransactionDataXDR: "AAAAAAAAAAUAAAAAAAAAAI6zjC5RtJsxMAzXJfbm813ySujUwQVm4r2uHtkav62tAAAABgAAAAFDEqQxRKsWsubOpgtPPKXSsdhcWDpfu/jRwXKpUugxhQAAABQAAAABAAAABgAAAAGBhvDmuHDARIUDYKVFokPXfBrz+6tx3N4D7hMpL1AiBwAAABQAAAABAAAAB1uPeA45/uPdYS9GdAZXx37bjezG+3vn4JqEwlyjIRGmAAAAB9nC+GOHmV4+xAZQ4T0I434wH3LKi+db6CM9hlRZhRZgAAAAAgAAAAYAAAAAAAAAAI6zjC5RtJsxMAzXJfbm813ySujUwQVm4r2uHtkav62tAAAAFXCHgz/4M7a3AAAAAAAAAAYAAAABQxKkMUSrFrLmzqYLTzyl0rHYXFg6X7v40cFyqVLoMYUAAAAVNIwhp30FbW4AAAAAAB8NxwAAEgAAAACUAAAAAAAY7m4=",
			MinResourceFee:     50,
		}

		rpcClient := &mocks.MockRPCClient{}
		rpcClient.On("SimulateTransaction", mock.Anything, mock.Anything).Return(simulationResponse, nil)

		monitorSvc := tssMonitor.TSSMonitorService{
			Client: &sdpMonitorMocks.MockMonitorClient{},
		}
		walletCreationHandler, err := NewWalletCreationTransactionHandler(engine, rpcClient, monitorSvc)
		require.NoError(t, err)

		recoveryAddress := "GBYJZW5XFAI6XV73H5SAIUYK6XZI4CGGVBUBO3ANA2SV7KKDAXTV6AEB"
		txJob := &TxJob{
			Transaction: store.Transaction{
				WalletCreation: store.WalletCreation{
					PublicKey:       publicKeyHex,
					WasmHash:        wasmHashHex,
					Salt:            saltHex,
					RecoveryAddress: sql.NullString{String: recoveryAddress, Valid: true},
				},
			},
			ChannelAccount: store.ChannelAccount{
				PublicKey: channelAccount,
			},
			LockedUntilLedgerNumber: 12345,
		}

		tx, err := walletCreationHandler.BuildInnerTransaction(ctx, txJob, 100, distributionAccount)
		require.NoError(t, err)
		require.NotNil(t, tx)

		// Verify transaction structure
		assert.Equal(t, channelAccount, tx.SourceAccount().AccountID)
		assert.Len(t, tx.Operations(), 1)

		// Verify it's an InvokeHostFunction operation
		operation, ok := tx.Operations()[0].(*txnbuild.InvokeHostFunction)
		require.True(t, ok)
		assert.Equal(t, distributionAccount, operation.SourceAccount)

		// Verify that auth entries were processed (should have 1 auth entry)
		assert.Len(t, operation.Auth, 1)

		// Verify that transaction data was applied
		assert.Equal(t, 1, int(operation.Ext.V))
		assert.NotNil(t, operation.Ext.SorobanData)

		// Verify the contract invocation has correct arguments
		require.Equal(t, operation.HostFunction.Type, xdr.HostFunctionTypeHostFunctionTypeCreateContractV2)
		require.NotNil(t, operation.HostFunction.CreateContractV2)

		// Verify constructor arguments are in the correct order: [argAdmin, argPublicKey, argRecovery]
		constructorArgs := operation.HostFunction.CreateContractV2.ConstructorArgs
		require.Len(t, constructorArgs, 3)

		// First argument should be argAdmin (distribution account address)
		argAdmin := constructorArgs[0]
		assert.Equal(t, xdr.ScValTypeScvAddress, argAdmin.Type)
		require.NotNil(t, argAdmin.Address)
		assert.Equal(t, xdr.ScAddressTypeScAddressTypeAccount, argAdmin.Address.Type)
		require.NotNil(t, argAdmin.Address.AccountId)
		distributionAccountId := xdr.MustAddress(distributionAccount)
		assert.Equal(t, distributionAccountId, *argAdmin.Address.AccountId)

		// Second argument should be argPublicKey (public key bytes)
		argPublicKey := constructorArgs[1]
		assert.Equal(t, xdr.ScValTypeScvBytes, argPublicKey.Type)
		require.NotNil(t, argPublicKey.Bytes)

		// Verify the public key bytes match the expected decoded hex
		expectedPublicKeyBytes, err := hex.DecodeString(publicKeyHex)
		require.NoError(t, err)
		assert.Equal(t, expectedPublicKeyBytes, []byte(*argPublicKey.Bytes))

		// Third argument should be argRecovery (recovery account address)
		argRecovery := constructorArgs[2]
		assert.Equal(t, xdr.ScValTypeScvAddress, argRecovery.Type)
		require.NotNil(t, argRecovery.Address)
		assert.Equal(t, xdr.ScAddressTypeScAddressTypeAccount, argRecovery.Address.Type)
		require.NotNil(t, argRecovery.Address.AccountId)
		recoveryAccountId := xdr.MustAddress("GBYJZW5XFAI6XV73H5SAIUYK6XZI4CGGVBUBO3ANA2SV7KKDAXTV6AEB")
		assert.Equal(t, recoveryAccountId, *argRecovery.Address.AccountId)

		// Verify the WASM hash is correctly set
		require.NotNil(t, operation.HostFunction.CreateContractV2.Executable.WasmHash)
		expectedWasmHashBytes, err := hex.DecodeString(wasmHashHex)
		require.NoError(t, err)
		assert.Equal(t, expectedWasmHashBytes, (*operation.HostFunction.CreateContractV2.Executable.WasmHash)[:])

		// Verify the contract ID preimage is correctly configured
		contractIdPreimage := operation.HostFunction.CreateContractV2.ContractIdPreimage
		assert.Equal(t, xdr.ContractIdPreimageTypeContractIdPreimageFromAddress, contractIdPreimage.Type)
		require.NotNil(t, contractIdPreimage.FromAddress)

		// Verify the address in the preimage matches the distribution account
		assert.Equal(t, xdr.ScAddressTypeScAddressTypeAccount, contractIdPreimage.FromAddress.Address.Type)
		require.NotNil(t, contractIdPreimage.FromAddress.Address.AccountId)
		assert.Equal(t, distributionAccountId, *contractIdPreimage.FromAddress.Address.AccountId)

		// Verify mocks were called as expected
		mHorizonClient.AssertExpectations(t)
		rpcClient.AssertExpectations(t)
	})

	t.Run("returns error when recovery address is not set", func(t *testing.T) {
		engine := &engine.SubmitterEngine{
			MaxBaseFee: 100,
		}

		rpcClient := &mocks.MockRPCClient{}
		monitorSvc := tssMonitor.TSSMonitorService{
			Client: &sdpMonitorMocks.MockMonitorClient{},
		}
		walletCreationHandler, err := NewWalletCreationTransactionHandler(engine, rpcClient, monitorSvc)
		require.NoError(t, err)

		txJob := &TxJob{
			Transaction: store.Transaction{
				WalletCreation: store.WalletCreation{
					PublicKey:       publicKeyHex,
					WasmHash:        wasmHashHex,
					Salt:            saltHex,
					RecoveryAddress: sql.NullString{Valid: false}, // Not set
				},
			},
			ChannelAccount: store.ChannelAccount{
				PublicKey: channelAccount,
			},
			LockedUntilLedgerNumber: 12345,
		}

		tx, err := walletCreationHandler.BuildInnerTransaction(ctx, txJob, 1000, distributionAccount)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "recovery address not set - this indicates a configuration error")
		assert.Nil(t, tx)
	})

	t.Run("simulation error handling", func(t *testing.T) {
		engine := &engine.SubmitterEngine{
			MaxBaseFee: 100,
		}

		simulationResponse := protocol.SimulateTransactionResponse{
			Error: "contract execution failed",
		}

		rpcClient := &mocks.MockRPCClient{}
		rpcClient.On("SimulateTransaction", mock.Anything, mock.Anything).Return(simulationResponse, nil)

		monitorSvc := tssMonitor.TSSMonitorService{
			Client: &sdpMonitorMocks.MockMonitorClient{},
		}
		walletCreationHandler, err := NewWalletCreationTransactionHandler(engine, rpcClient, monitorSvc)
		require.NoError(t, err)

		recoveryAddress := "GBYJZW5XFAI6XV73H5SAIUYK6XZI4CGGVBUBO3ANA2SV7KKDAXTV6AEB"
		txJob := &TxJob{
			Transaction: store.Transaction{
				WalletCreation: store.WalletCreation{
					PublicKey:       publicKeyHex,
					WasmHash:        wasmHashHex,
					Salt:            saltHex,
					RecoveryAddress: sql.NullString{String: recoveryAddress, Valid: true},
				},
			},
			ChannelAccount: store.ChannelAccount{
				PublicKey: channelAccount,
			},
			LockedUntilLedgerNumber: 12345,
		}

		tx, err := walletCreationHandler.BuildInnerTransaction(ctx, txJob, 100, distributionAccount)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "simulation error: contract execution failed")
		assert.Nil(t, tx)

		rpcClient.AssertExpectations(t)
	})

	t.Run("rpc client error handling", func(t *testing.T) {
		engine := &engine.SubmitterEngine{
			MaxBaseFee: 100,
		}

		rpcClient := &mocks.MockRPCClient{}
		rpcClient.On("SimulateTransaction", mock.Anything, mock.Anything).Return(protocol.SimulateTransactionResponse{}, fmt.Errorf("rpc error"))

		monitorSvc := tssMonitor.TSSMonitorService{
			Client: &sdpMonitorMocks.MockMonitorClient{},
		}
		walletCreationHandler, err := NewWalletCreationTransactionHandler(engine, rpcClient, monitorSvc)
		require.NoError(t, err)

		recoveryAddress := "GBYJZW5XFAI6XV73H5SAIUYK6XZI4CGGVBUBO3ANA2SV7KKDAXTV6AEB"
		txJob := &TxJob{
			Transaction: store.Transaction{
				WalletCreation: store.WalletCreation{
					PublicKey:       publicKeyHex,
					WasmHash:        wasmHashHex,
					Salt:            saltHex,
					RecoveryAddress: sql.NullString{String: recoveryAddress, Valid: true},
				},
			},
			ChannelAccount: store.ChannelAccount{
				PublicKey: channelAccount,
			},
			LockedUntilLedgerNumber: 12345,
		}

		tx, err := walletCreationHandler.BuildInnerTransaction(ctx, txJob, 100, distributionAccount)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "simulating transaction: rpc error")
		assert.Nil(t, tx)

		rpcClient.AssertExpectations(t)
	})

	t.Run("horizon client error handling", func(t *testing.T) {
		mHorizonClient := &horizonclient.MockClient{}
		mHorizonClient.On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).Return(
			horizon.Account{}, fmt.Errorf("horizon error"),
		)

		engine := &engine.SubmitterEngine{
			HorizonClient: mHorizonClient,
			MaxBaseFee:    100,
		}

		authXDR := []string{"AAAAAQAAAAHw6CVqzY+dCq3myVJBo1kb3nEGE7oO6obmJeUNvYQ0ukNc84Ms0ZvgAAAAAAAAAAEAAAAAAAAAAeA7wfSg10yaQYZDRmQeyqsepsS/Mb0rbMQxRgDoSVdWAAAAD3dlYl9hdXRoX3ZlcmlmeQAAAAAGAAAADgAAADhDRFlPUUpMS1pXSFoyQ1ZONDNFVkVRTkRMRU41NDRJR0NPNUE1MlVHNFlTNktETjVRUTJMVVdLWQAAAA4AAAADMTIzAAAAAA4AAAAcaHR0cDovL2xvY2FsaG9zdDo4MDgwL2MvYXV0aAAAAA4AAAAObG9jYWxob3N0OjgwODAAAAAAAA4AAAALZXhhbXBsZS5jb20AAAAAAQAAAAA="}
		simulationResponse := protocol.SimulateTransactionResponse{
			Error: "",
			Results: []protocol.SimulateHostFunctionResult{
				{
					AuthXDR: &authXDR,
				},
			},
			TransactionDataXDR: "AAAAAAAAAAUAAAAAAAAAAI6zjC5RtJsxMAzXJfbm813ySujUwQVm4r2uHtkav62tAAAABgAAAAFDEqQxRKsWsubOpgtPPKXSsdhcWDpfu/jRwXKpUugxhQAAABQAAAABAAAABgAAAAGBhvDmuHDARIUDYKVFokPXfBrz+6tx3N4D7hMpL1AiBwAAABQAAAABAAAAB1uPeA45/uPdYS9GdAZXx37bjezG+3vn4JqEwlyjIRGmAAAAB9nC+GOHmV4+xAZQ4T0I434wH3LKi+db6CM9hlRZhRZgAAAAAgAAAAYAAAAAAAAAAI6zjC5RtJsxMAzXJfbm813ySujUwQVm4r2uHtkav62tAAAAFXCHgz/4M7a3AAAAAAAAAAYAAAABQxKkMUSrFrLmzqYLTzyl0rHYXFg6X7v40cFyqVLoMYUAAAAVNIwhp30FbW4AAAAAAB8NxwAAEgAAAACUAAAAAAAY7m4=",
			MinResourceFee:     50,
		}

		rpcClient := &mocks.MockRPCClient{}
		rpcClient.On("SimulateTransaction", mock.Anything, mock.Anything).Return(simulationResponse, nil)

		monitorSvc := tssMonitor.TSSMonitorService{
			Client: &sdpMonitorMocks.MockMonitorClient{},
		}
		walletCreationHandler, err := NewWalletCreationTransactionHandler(engine, rpcClient, monitorSvc)
		require.NoError(t, err)

		recoveryAddress := "GBYJZW5XFAI6XV73H5SAIUYK6XZI4CGGVBUBO3ANA2SV7KKDAXTV6AEB"
		txJob := &TxJob{
			Transaction: store.Transaction{
				WalletCreation: store.WalletCreation{
					PublicKey:       publicKeyHex,
					WasmHash:        wasmHashHex,
					Salt:            saltHex,
					RecoveryAddress: sql.NullString{String: recoveryAddress, Valid: true},
				},
			},
			ChannelAccount: store.ChannelAccount{
				PublicKey: channelAccount,
			},
			LockedUntilLedgerNumber: 12345,
		}

		tx, err := walletCreationHandler.BuildInnerTransaction(ctx, txJob, 100, distributionAccount)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "horizon error")
		assert.Nil(t, tx)

		var horizonErr *utils.HorizonErrorWrapper
		assert.ErrorAs(t, err, &horizonErr)

		mHorizonClient.AssertExpectations(t)
		rpcClient.AssertExpectations(t)
	})
}

func Test_WalletCreationTransactionHandler_BuildSuccessEvent(t *testing.T) {
	walletCreationHandler := &WalletCreationTransactionHandler{}

	ctx := context.Background()
	txJob := &TxJob{
		Transaction: store.Transaction{
			ID:                     "tx-id",
			ExternalID:             "wallet-creation-id",
			TenantID:               "tenant-id",
			StellarTransactionHash: sql.NullString{},
		},
	}

	msg, err := walletCreationHandler.BuildSuccessEvent(ctx, txJob)
	require.NoError(t, err)

	gotWalletCreationCompletedAt := msg.Data.(schemas.EventWalletCreationCompletedData).WalletCreationCompletedAt
	assert.WithinDuration(t, time.Now(), gotWalletCreationCompletedAt, time.Millisecond*100)
	wantMsg := &events.Message{
		Topic:    events.WalletCreationCompletedTopic,
		Key:      txJob.Transaction.ExternalID,
		TenantID: txJob.Transaction.TenantID,
		Type:     events.WalletCreationCompletedSuccessType,
		Data: schemas.EventWalletCreationCompletedData{
			TransactionID:             txJob.Transaction.ID,
			WalletCreationID:          txJob.Transaction.ExternalID,
			WalletCreationStatus:      string(data.SuccessWalletStatus),
			WalletCreationCompletedAt: gotWalletCreationCompletedAt,
			StellarTransactionID:      txJob.Transaction.StellarTransactionHash.String,
		},
	}
	assert.Equal(t, wantMsg, msg)
}

func Test_WalletCreationTransactionHandler_BuildFailureEvent(t *testing.T) {
	walletCreationHandler := &WalletCreationTransactionHandler{}

	ctx := context.Background()
	txJob := &TxJob{
		Transaction: store.Transaction{
			ID:                     "tx-123",
			ExternalID:             "wallet_token_abc",
			TenantID:               "tenant-1",
			StellarTransactionHash: sql.NullString{},
		},
	}
	hErr := &utils.HorizonErrorWrapper{
		Err: fmt.Errorf("test error"),
	}

	msg, err := walletCreationHandler.BuildFailureEvent(ctx, txJob, hErr)
	require.NoError(t, err)

	gotWalletCreationCompletedAt := msg.Data.(schemas.EventWalletCreationCompletedData).WalletCreationCompletedAt
	assert.WithinDuration(t, time.Now(), gotWalletCreationCompletedAt, time.Millisecond*100)
	wantMsg := &events.Message{
		Topic:    events.WalletCreationCompletedTopic,
		Key:      txJob.Transaction.ExternalID,
		TenantID: txJob.Transaction.TenantID,
		Type:     events.WalletCreationCompletedErrorType,
		Data: schemas.EventWalletCreationCompletedData{
			TransactionID:               txJob.Transaction.ID,
			WalletCreationID:            txJob.Transaction.ExternalID,
			WalletCreationStatus:        string(data.FailedWalletStatus),
			WalletCreationStatusMessage: hErr.Error(),
			WalletCreationCompletedAt:   gotWalletCreationCompletedAt,
			StellarTransactionID:        txJob.Transaction.StellarTransactionHash.String,
		},
	}
	assert.Equal(t, wantMsg, msg)
}

func Test_WalletCreationTransactionHandler_MonitorTransactionProcessingStarted(t *testing.T) {
	ctx := context.Background()
	txJob := TxJob{
		Transaction: store.Transaction{},
	}
	jobUUID := "job-uuid"

	mMonitorClient := sdpMonitorMocks.NewMockMonitorClient(t)
	mMonitorClient.
		On("MonitorCounters", sdpMonitor.WalletCreationProcessingStartedTag, mock.Anything).
		Return(nil).
		Once()
	tssMonitorService := tssMonitor.TSSMonitorService{
		Version:       "0.01",
		GitCommitHash: "0xABC",
		Client:        mMonitorClient,
	}
	walletCreationHandler := &WalletCreationTransactionHandler{
		monitorSvc: tssMonitorService,
	}

	walletCreationHandler.MonitorTransactionProcessingStarted(ctx, &txJob, jobUUID)
}

func Test_WalletCreationTransactionHandler_MonitorTransactionProcessingSuccess(t *testing.T) {
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

	mMonitorClient := sdpMonitorMocks.NewMockMonitorClient(t)
	mMonitorClient.
		On("MonitorCounters", sdpMonitor.WalletCreationTransactionSuccessfulTag, mock.Anything).
		Return(nil).
		Once()
	tssMonitorService := tssMonitor.TSSMonitorService{
		Version:       "0.01",
		GitCommitHash: "0xABC",
		Client:        mMonitorClient,
	}
	walletCreationHandler := &WalletCreationTransactionHandler{
		monitorSvc: tssMonitorService,
	}

	walletCreationHandler.MonitorTransactionProcessingSuccess(ctx, &txJob, jobUUID)
}

func Test_WalletCreationTransactionHandler_MonitorTransactionProcessingFailed(t *testing.T) {
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

	mMonitorClient := sdpMonitorMocks.NewMockMonitorClient(t)
	mMonitorClient.
		On("MonitorCounters", sdpMonitor.WalletCreationErrorTag, mock.Anything).
		Return(nil).
		Once()
	tssMonitorService := tssMonitor.TSSMonitorService{
		Version:       "0.01",
		GitCommitHash: "0xABC",
		Client:        mMonitorClient,
	}
	walletCreationHandler := &WalletCreationTransactionHandler{
		monitorSvc: tssMonitorService,
	}

	walletCreationHandler.MonitorTransactionProcessingFailed(ctx, &txJob, jobUUID, isRetryable, errStack)
}

func Test_WalletCreationTransactionHandler_MonitorTransactionReconciliationSuccess(t *testing.T) {
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

	mMonitorClient := sdpMonitorMocks.NewMockMonitorClient(t)
	mMonitorClient.
		On("MonitorCounters", sdpMonitor.WalletCreationReconciliationSuccessfulTag, mock.Anything).
		Return(nil).
		Once()
	tssMonitorService := tssMonitor.TSSMonitorService{
		Version:       "0.01",
		GitCommitHash: "0xABC",
		Client:        mMonitorClient,
	}
	walletCreationHandler := &WalletCreationTransactionHandler{
		monitorSvc: tssMonitorService,
	}

	walletCreationHandler.MonitorTransactionReconciliationSuccess(ctx, &txJob, jobUUID, ReconcileSuccess)
}

func Test_WalletCreationTransactionHandler_MonitorTransactionReconciliationFailure(t *testing.T) {
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

	mMonitorClient := sdpMonitorMocks.NewMockMonitorClient(t)
	mMonitorClient.
		On("MonitorCounters", sdpMonitor.WalletCreationReconciliationFailureTag, mock.Anything).
		Return(nil).
		Once()
	tssMonitorService := tssMonitor.TSSMonitorService{
		Version:       "0.01",
		GitCommitHash: "0xABC",
		Client:        mMonitorClient,
	}
	walletCreationHandler := &WalletCreationTransactionHandler{
		monitorSvc: tssMonitorService,
	}

	walletCreationHandler.MonitorTransactionReconciliationFailure(ctx, &txJob, jobUUID, isHorizonErr, errStack)
}

func Test_WalletCreationTransactionHandler_AddContextLoggerFields(t *testing.T) {
	engine := &engine.SubmitterEngine{}
	rpcClient := &mocks.MockRPCClient{}
	monitorSvc := tssMonitor.TSSMonitorService{
		Client: &sdpMonitorMocks.MockMonitorClient{},
	}
	walletCreationHandler, err := NewWalletCreationTransactionHandler(engine, rpcClient, monitorSvc)
	require.NoError(t, err)

	publicKeyHex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef01"
	wasmHashHex := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

	transaction := &store.Transaction{
		WalletCreation: store.WalletCreation{
			PublicKey: publicKeyHex,
			WasmHash:  wasmHashHex,
		},
	}

	fields := walletCreationHandler.AddContextLoggerFields(transaction)

	assert.Equal(t, publicKeyHex, fields["public_key"])
	assert.Equal(t, wasmHashHex, fields["wasm_hash"])
	assert.Len(t, fields, 2)
}

func Test_WalletCreationTransactionHandler_CalculateAdjustedBaseFee(t *testing.T) {
	engine := &engine.SubmitterEngine{MaxBaseFee: 100}
	rpcClient := &mocks.MockRPCClient{}
	monitorSvc := tssMonitor.TSSMonitorService{
		Client: &sdpMonitorMocks.MockMonitorClient{},
	}
	handler, err := NewWalletCreationTransactionHandler(engine, rpcClient, monitorSvc)
	require.NoError(t, err)

	t.Run("zero min resource fee", func(t *testing.T) {
		resp := protocol.SimulateTransactionResponse{
			MinResourceFee: 0,
		}
		fee := handler.calculateAdjustedBaseFee(resp)
		assert.Equal(t, int64(100), fee)
	})

	t.Run("with resource fee within max", func(t *testing.T) {
		resp := protocol.SimulateTransactionResponse{
			MinResourceFee: 50,
		}
		fee := handler.calculateAdjustedBaseFee(resp)
		assert.Equal(t, int64(100), fee)
	})

	t.Run("with resource fee exceeding max", func(t *testing.T) {
		resp := protocol.SimulateTransactionResponse{
			MinResourceFee: 200,
		}
		fee := handler.calculateAdjustedBaseFee(resp)
		assert.Equal(t, int64(txnbuild.MinBaseFee), fee)
	})
}

func Test_WalletCreationTransactionHandler_MonitoringBehavior(t *testing.T) {
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
		mMonitorClient := sdpMonitorMocks.NewMockMonitorClient(t)
		mMonitorClient.
			On("MonitorCounters", sdpMonitor.WalletCreationTransactionSuccessfulTag, mock.Anything).
			Return(nil).
			Once()

		tssMonitorService := tssMonitor.TSSMonitorService{
			Client: mMonitorClient,
		}
		walletCreationHandler := &WalletCreationTransactionHandler{
			monitorSvc: tssMonitorService,
		}

		walletCreationHandler.MonitorTransactionProcessingSuccess(ctx, txJob, jobUUID)
	})

	t.Run("MonitorTransactionProcessingFailed with retryable error", func(t *testing.T) {
		mMonitorClient := sdpMonitorMocks.NewMockMonitorClient(t)
		mMonitorClient.
			On("MonitorCounters", sdpMonitor.WalletCreationErrorTag, mock.Anything).
			Return(nil).
			Once()

		tssMonitorService := tssMonitor.TSSMonitorService{
			Client: mMonitorClient,
		}
		walletCreationHandler := &WalletCreationTransactionHandler{
			monitorSvc: tssMonitorService,
		}

		walletCreationHandler.MonitorTransactionProcessingFailed(ctx, txJob, jobUUID, true, "retryable error")
	})

	t.Run("MonitorTransactionReconciliationSuccess with reprocessing type", func(t *testing.T) {
		mMonitorClient := sdpMonitorMocks.NewMockMonitorClient(t)
		mMonitorClient.
			On("MonitorCounters", sdpMonitor.WalletCreationReconciliationSuccessfulTag, mock.Anything).
			Return(nil).
			Once()

		tssMonitorService := tssMonitor.TSSMonitorService{
			Client: mMonitorClient,
		}
		walletCreationHandler := &WalletCreationTransactionHandler{
			monitorSvc: tssMonitorService,
		}

		walletCreationHandler.MonitorTransactionReconciliationSuccess(ctx, txJob, jobUUID, ReconcileReprocessing)
	})
}

func Test_WalletCreationTransactionHandler_ExtractAuthEntries(t *testing.T) {
	engine := &engine.SubmitterEngine{}
	rpcClient := &mocks.MockRPCClient{}
	monitorSvc := tssMonitor.TSSMonitorService{
		Client: &sdpMonitorMocks.MockMonitorClient{},
	}
	handler, err := NewWalletCreationTransactionHandler(engine, rpcClient, monitorSvc)
	require.NoError(t, err)

	t.Run("empty results", func(t *testing.T) {
		response := protocol.SimulateTransactionResponse{
			Results: []protocol.SimulateHostFunctionResult{},
		}

		auth, err := handler.extractAuthEntries(response)
		require.NoError(t, err)
		assert.Empty(t, auth)
	})

	t.Run("no auth XDR", func(t *testing.T) {
		response := protocol.SimulateTransactionResponse{
			Results: []protocol.SimulateHostFunctionResult{
				{
					AuthXDR: nil,
				},
			},
		}

		auth, err := handler.extractAuthEntries(response)
		require.NoError(t, err)
		assert.Empty(t, auth)
	})

	t.Run("valid auth entries", func(t *testing.T) {
		authXDR := []string{"AAAAAQAAAAHw6CVqzY+dCq3myVJBo1kb3nEGE7oO6obmJeUNvYQ0ukNc84Ms0ZvgAAAAAAAAAAEAAAAAAAAAAeA7wfSg10yaQYZDRmQeyqsepsS/Mb0rbMQxRgDoSVdWAAAAD3dlYl9hdXRoX3ZlcmlmeQAAAAAGAAAADgAAADhDRFlPUUpMS1pXSFoyQ1ZONDNFVkVRTkRMRU41NDRJR0NPNUE1MlVHNFlTNktETjVRUTJMVVdLWQAAAA4AAAADMTIzAAAAAA4AAAAcaHR0cDovL2xvY2FsaG9zdDo4MDgwL2MvYXV0aAAAAA4AAAAObG9jYWxob3N0OjgwODAAAAAAAA4AAAALZXhhbXBsZS5jb20AAAAAAQAAAAA="}
		response := protocol.SimulateTransactionResponse{
			Results: []protocol.SimulateHostFunctionResult{
				{
					AuthXDR: &authXDR,
				},
			},
		}

		auth, err := handler.extractAuthEntries(response)
		require.NoError(t, err)
		assert.Len(t, auth, 1)
	})

	t.Run("invalid auth XDR", func(t *testing.T) {
		authXDR := []string{"invalid-base64"}
		response := protocol.SimulateTransactionResponse{
			Results: []protocol.SimulateHostFunctionResult{
				{
					AuthXDR: &authXDR,
				},
			},
		}

		auth, err := handler.extractAuthEntries(response)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshalling authorization entry")
		assert.Nil(t, auth)
	})
}

func Test_WalletCreationTransactionHandler_ApplyTransactionData(t *testing.T) {
	engine := &engine.SubmitterEngine{}
	rpcClient := &mocks.MockRPCClient{}
	monitorSvc := tssMonitor.TSSMonitorService{
		Client: &sdpMonitorMocks.MockMonitorClient{},
	}
	handler, err := NewWalletCreationTransactionHandler(engine, rpcClient, monitorSvc)
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
