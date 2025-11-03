package transactionsubmission

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stellar/go/txnbuild"
	"github.com/stellar/go/xdr"
	"github.com/stellar/stellar-rpc/protocol"
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
	sdpUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func Test_NewWalletCreationTransactionHandler(t *testing.T) {
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

	t.Run("input validation", func(t *testing.T) {
		engine := &engine.SubmitterEngine{
			MaxBaseFee: 100,
		}
		rpcClient := &mocks.MockRPCClient{}
		monitorSvc := tssMonitor.TSSMonitorService{
			Client: &sdpMonitor.MockMonitorClient{},
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
						},
					},
					ChannelAccount: store.ChannelAccount{
						PublicKey: channelAccount,
					},
					LockedUntilLedgerNumber: 12345,
				},
				expectedError: "public key must be 65 bytes",
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
		engine := &engine.SubmitterEngine{
			MaxBaseFee: 100,
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
		rpcClient.On("SimulateTransaction", mock.Anything, mock.Anything).Return(&stellar.SimulationResult{Response: simulationResponse}, (*stellar.SimulationError)(nil))

		monitorSvc := tssMonitor.TSSMonitorService{
			Client: &sdpMonitor.MockMonitorClient{},
		}
		walletCreationHandler, err := NewWalletCreationTransactionHandler(engine, rpcClient, monitorSvc)
		require.NoError(t, err)

		txJob := &TxJob{
			Transaction: store.Transaction{
				WalletCreation: store.WalletCreation{
					PublicKey: publicKeyHex,
					WasmHash:  wasmHashHex,
				},
			},
			ChannelAccount: store.ChannelAccount{
				PublicKey: channelAccount,
			},
			LockedUntilLedgerNumber: 12345,
		}

		// Call method under test
		tx, err := walletCreationHandler.BuildInnerTransaction(ctx, txJob, 100, distributionAccount)
		require.NoError(t, err)

		// Build the expected transaction, for assertions
		var transactionData xdr.SorobanTransactionData
		require.NoError(t, xdr.SafeUnmarshalBase64(simulationResponse.TransactionDataXDR, &transactionData))
		createContractOp := txnbuild.InvokeHostFunction{
			SourceAccount: distributionAccount,
			HostFunction: xdr.HostFunction{
				Type: xdr.HostFunctionTypeHostFunctionTypeCreateContractV2,
				CreateContractV2: &xdr.CreateContractArgsV2{
					ContractIdPreimage: xdr.ContractIdPreimage{
						Type: xdr.ContractIdPreimageTypeContractIdPreimageFromAddress,
						FromAddress: &xdr.ContractIdPreimageFromAddress{
							Address: xdr.ScAddress{
								Type:      xdr.ScAddressTypeScAddressTypeAccount,
								AccountId: sdpUtils.Ptr(xdr.MustAddress(distributionAccount)),
							},
							Salt: xdr.Uint256{
								99, 129, 232, 41, 167, 236, 207, 121, 224, 247, 188, 136, 165, 129, 4, 28, 105, 128, 46, 193, 138, 11, 31, 179, 182, 238, 155, 201, 113, 108, 56, 41,
							},
						},
					},
					Executable: xdr.ContractExecutable{
						Type: xdr.ContractExecutableTypeContractExecutableWasm,
						WasmHash: &xdr.Hash{
							171, 205, 239, 1, 35, 69, 103, 137, 171, 205, 239, 1, 35, 69, 103, 137, 171, 205, 239, 1, 35, 69, 103, 137, 171, 205, 239, 1, 35, 69, 103, 137,
						},
					},
					ConstructorArgs: []xdr.ScVal{
						{
							Type: xdr.ScValTypeScvAddress,
							Address: &xdr.ScAddress{
								Type:      xdr.ScAddressTypeScAddressTypeAccount,
								AccountId: sdpUtils.Ptr(xdr.MustAddress(distributionAccount)),
							},
						},
						{
							Type: xdr.ScValTypeScvBytes,
							Bytes: &xdr.ScBytes{
								1, 35, 69, 103, 137, 171, 205, 239, 1, 35, 69, 103, 137, 171, 205, 239, 1, 35, 69, 103, 137, 171, 205, 239, 1, 35, 69, 103, 137, 171, 205, 239, 1, 35, 69, 103, 137, 171, 205, 239, 1, 35, 69, 103, 137, 171, 205, 239, 1, 35, 69, 103, 137, 171, 205, 239, 1, 35, 69, 103, 137, 171, 205, 239, 1,
							},
						},
					},
				},
			},
			Ext: xdr.TransactionExt{
				V:           1,
				SorobanData: &transactionData,
			},
		}
		createContractOp.Auth, err = walletCreationHandler.extractAuthEntries(simulationResponse)
		require.NoError(t, err)
		wantTx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
			SourceAccount: &txnbuild.SimpleAccount{
				AccountID: channelAccount,
				Sequence:  101,
			},
			IncrementSequenceNum: false,
			BaseFee:              100,
			Operations:           []txnbuild.Operation{&createContractOp},
			Preconditions: txnbuild.Preconditions{
				TimeBounds:   tx.Timebounds(),
				LedgerBounds: &txnbuild.LedgerBounds{MaxLedger: uint32(txJob.LockedUntilLedgerNumber)},
			},
		})
		require.NoError(t, err)

		// Assertions
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
		walletCreationHandler, err := NewWalletCreationTransactionHandler(engine, rpcClient, monitorSvc)
		require.NoError(t, err)

		txJob := &TxJob{
			Transaction: store.Transaction{
				WalletCreation: store.WalletCreation{
					PublicKey: publicKeyHex,
					WasmHash:  wasmHashHex,
				},
			},
			ChannelAccount: store.ChannelAccount{
				PublicKey: channelAccount,
			},
			LockedUntilLedgerNumber: 12345,
		}

		tx, err := walletCreationHandler.BuildInnerTransaction(ctx, txJob, 100, distributionAccount)
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
		walletCreationHandler, err := NewWalletCreationTransactionHandler(engine, rpcClient, monitorSvc)
		require.NoError(t, err)

		txJob := &TxJob{
			Transaction: store.Transaction{
				WalletCreation: store.WalletCreation{
					PublicKey: publicKeyHex,
					WasmHash:  wasmHashHex,
				},
			},
			ChannelAccount: store.ChannelAccount{
				PublicKey: channelAccount,
			},
			LockedUntilLedgerNumber: 12345,
		}

		tx, err := walletCreationHandler.BuildInnerTransaction(ctx, txJob, 100, distributionAccount)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "rpc error")
		assert.Nil(t, tx)

		var rpcErr *utils.RPCErrorWrapper
		assert.ErrorAs(t, err, &rpcErr)
		assert.True(t, rpcErr.IsRPCError())

		rpcClient.AssertExpectations(t)
	})
}

func Test_WalletCreationTransactionHandler_MonitorTransactionProcessingStarted(t *testing.T) {
	ctx := context.Background()
	txJob := TxJob{
		Transaction: store.Transaction{},
	}
	jobUUID := "job-uuid"

	mMonitorClient := sdpMonitor.NewMockMonitorClient(t)
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

	mMonitorClient := sdpMonitor.NewMockMonitorClient(t)
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

	mMonitorClient := sdpMonitor.NewMockMonitorClient(t)
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

	mMonitorClient := sdpMonitor.NewMockMonitorClient(t)
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

	mMonitorClient := sdpMonitor.NewMockMonitorClient(t)
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
		Client: &sdpMonitor.MockMonitorClient{},
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
		mMonitorClient := sdpMonitor.NewMockMonitorClient(t)
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
		mMonitorClient := sdpMonitor.NewMockMonitorClient(t)
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
		mMonitorClient := sdpMonitor.NewMockMonitorClient(t)
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
		Client: &sdpMonitor.MockMonitorClient{},
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
		Client: &sdpMonitor.MockMonitorClient{},
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

func Test_WalletCreationTransactionHandler_RequiresRebuildOnRetry(t *testing.T) {
	handler := &WalletCreationTransactionHandler{}

	result := handler.RequiresRebuildOnRetry()
	assert.True(t, result)
}
