package transactionsubmission

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stellar/go/txnbuild"
	"github.com/stellar/stellar-rpc/client"
	"github.com/stellar/stellar-rpc/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	sdpMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	sdpMonitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

func createMockRPCClient() *client.Client {
	return client.NewClient("http://localhost:8000", &http.Client{})
}

func Test_NewWalletCreationTransactionHandler(t *testing.T) {
	rpcClient := createMockRPCClient()
	tssMonitorSvc := tssMonitor.TSSMonitorService{
		GitCommitHash: "gitCommitHash0x",
		Version:       "version123",
		Client:        &sdpMonitorMocks.MockMonitorClient{},
	}

	testCases := []struct {
		name          string
		engine        *engine.SubmitterEngine
		rpcClient     *client.Client
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
		{
			name:          "🎉 successfully returns a new wallet creation handler with nil RPC client",
			engine:        &engine.SubmitterEngine{},
			rpcClient:     nil,
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

	t.Run("validation errors (no RPC client)", func(t *testing.T) {
		engine := &engine.SubmitterEngine{
			MaxBaseFee: 100,
		}
		monitorSvc := tssMonitor.TSSMonitorService{
			Client: &sdpMonitorMocks.MockMonitorClient{},
		}
		handler, err := NewWalletCreationTransactionHandler(engine, nil, monitorSvc)
		require.NoError(t, err)

		testCases := []struct {
			name          string
			txJob         *TxJob
			expectedError string
		}{
			{
				name: "fails when RPC client is nil",
				txJob: &TxJob{
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
				},
				expectedError: "RPC client is required for wallet creation transactions",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				tx, err := handler.BuildInnerTransaction(ctx, tc.txJob, 100, distributionAccount)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
				assert.Nil(t, tx)
			})
		}
	})

	t.Run("input validation (with RPC client)", func(t *testing.T) {
		engine := &engine.SubmitterEngine{
			MaxBaseFee: 100,
		}
		rpcClient := createMockRPCClient()
		monitorSvc := tssMonitor.TSSMonitorService{
			Client: &sdpMonitorMocks.MockMonitorClient{},
		}
		handler, err := NewWalletCreationTransactionHandler(engine, rpcClient, monitorSvc)
		require.NoError(t, err)

		testCases := []struct {
			name          string
			txJob         *TxJob
			expectedError string
		}{
			{
				name: "fails when public key is empty",
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
				name: "fails when wasm hash is empty",
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
				name: "fails when public key is invalid hex",
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
				name: "fails when wasm hash is invalid hex",
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
				name: "fails when wasm hash is not 32 bytes",
				txJob: &TxJob{
					Transaction: store.Transaction{
						WalletCreation: store.WalletCreation{
							PublicKey: publicKeyHex,
							WasmHash:  "abcdef", // Too short - only 3 bytes
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
				name: "fails when public key is not 65 bytes",
				txJob: &TxJob{
					Transaction: store.Transaction{
						WalletCreation: store.WalletCreation{
							PublicKey: "0123456789abcdef", // Too short - only 8 bytes
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
				tx, err := handler.BuildInnerTransaction(ctx, tc.txJob, 100, distributionAccount)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
				assert.Nil(t, tx)
			})
		}
	})
}

func Test_WalletCreationTransactionHandler_BuildSuccessEvent(t *testing.T) {
	ctx := context.Background()
	engine := &engine.SubmitterEngine{}
	rpcClient := createMockRPCClient()
	monitorSvc := tssMonitor.TSSMonitorService{
		Client: &sdpMonitorMocks.MockMonitorClient{},
	}
	handler, err := NewWalletCreationTransactionHandler(engine, rpcClient, monitorSvc)
	require.NoError(t, err)

	txJob := &TxJob{
		Transaction: store.Transaction{
			ID:         "tx-123",
			ExternalID: "wallet_token_abc",
			TenantID:   "tenant-1",
			StellarTransactionHash: sql.NullString{
				String: "stellar-hash-123",
				Valid:  true,
			},
		},
	}

	message, err := handler.BuildSuccessEvent(ctx, txJob)
	require.NoError(t, err)
	require.NotNil(t, message)

	assert.Equal(t, events.WalletCreationCompletedTopic, message.Topic)
	assert.Equal(t, "wallet_token_abc", message.Key)
	assert.Equal(t, "tenant-1", message.TenantID)
	assert.Equal(t, events.WalletCreationCompletedSuccessType, message.Type)

	data, ok := message.Data.(schemas.EventWalletCreationCompletedData)
	require.True(t, ok)
	assert.Equal(t, "tx-123", data.TransactionID)
	assert.Equal(t, "wallet_token_abc", data.WalletCreationID)
	assert.Equal(t, "SUCCESS", data.WalletCreationStatus)
	assert.Empty(t, data.WalletCreationMessage)
	assert.Equal(t, "stellar-hash-123", data.StellarTransactionID)
	assert.WithinDuration(t, time.Now(), data.WalletCreationCompletedAt, time.Second)
}

func Test_WalletCreationTransactionHandler_BuildFailureEvent(t *testing.T) {
	ctx := context.Background()
	engine := &engine.SubmitterEngine{}
	rpcClient := createMockRPCClient()
	monitorSvc := tssMonitor.TSSMonitorService{
		Client: &sdpMonitorMocks.MockMonitorClient{},
	}
	handler, err := NewWalletCreationTransactionHandler(engine, rpcClient, monitorSvc)
	require.NoError(t, err)

	txJob := &TxJob{
		Transaction: store.Transaction{
			ID:         "tx-123",
			ExternalID: "wallet_token_abc",
			TenantID:   "tenant-1",
			StellarTransactionHash: sql.NullString{
				String: "stellar-hash-123",
				Valid:  true,
			},
		},
	}

	horizonErr := &utils.HorizonErrorWrapper{
		Err: fmt.Errorf("test error"),
	}

	message, err := handler.BuildFailureEvent(ctx, txJob, horizonErr)
	require.NoError(t, err)
	require.NotNil(t, message)

	assert.Equal(t, events.WalletCreationCompletedTopic, message.Topic)
	assert.Equal(t, "wallet_token_abc", message.Key)
	assert.Equal(t, "tenant-1", message.TenantID)
	assert.Equal(t, events.WalletCreationCompletedErrorType, message.Type)

	data, ok := message.Data.(schemas.EventWalletCreationCompletedData)
	require.True(t, ok)
	assert.Equal(t, "tx-123", data.TransactionID)
	assert.Equal(t, "wallet_token_abc", data.WalletCreationID)
	assert.Equal(t, "FAILED", data.WalletCreationStatus)
	assert.Contains(t, data.WalletCreationMessage, "test error")
	assert.Equal(t, "stellar-hash-123", data.StellarTransactionID)
	assert.WithinDuration(t, time.Now(), data.WalletCreationCompletedAt, time.Second)
}

func Test_WalletCreationTransactionHandler_MonitorTransactionProcessingStarted(t *testing.T) {
	ctx := context.Background()
	txJob := TxJob{
		Transaction: store.Transaction{},
		ChannelAccount: store.ChannelAccount{
			PublicKey: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
		},
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
	handler := &WalletCreationTransactionHandler{
		monitorSvc: tssMonitorService,
	}

	handler.MonitorTransactionProcessingStarted(ctx, &txJob, jobUUID)
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
	handler := &WalletCreationTransactionHandler{
		monitorSvc: tssMonitorService,
	}

	handler.MonitorTransactionProcessingSuccess(ctx, &txJob, jobUUID)
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
	handler := &WalletCreationTransactionHandler{
		monitorSvc: tssMonitorService,
	}

	handler.MonitorTransactionProcessingFailed(ctx, &txJob, jobUUID, isRetryable, errStack)
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
	handler := &WalletCreationTransactionHandler{
		monitorSvc: tssMonitorService,
	}

	handler.MonitorTransactionReconciliationSuccess(ctx, &txJob, jobUUID, ReconcileSuccess)
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
	handler := &WalletCreationTransactionHandler{
		monitorSvc: tssMonitorService,
	}

	handler.MonitorTransactionReconciliationFailure(ctx, &txJob, jobUUID, isHorizonErr, errStack)
}

func Test_WalletCreationTransactionHandler_AddContextLoggerFields(t *testing.T) {
	engine := &engine.SubmitterEngine{}
	rpcClient := createMockRPCClient()
	monitorSvc := tssMonitor.TSSMonitorService{
		Client: &sdpMonitorMocks.MockMonitorClient{},
	}
	handler, err := NewWalletCreationTransactionHandler(engine, rpcClient, monitorSvc)
	require.NoError(t, err)

	publicKeyHex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef01"
	wasmHashHex := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

	transaction := &store.Transaction{
		WalletCreation: store.WalletCreation{
			PublicKey: publicKeyHex,
			WasmHash:  wasmHashHex,
		},
	}

	fields := handler.AddContextLoggerFields(transaction)

	assert.Equal(t, publicKeyHex, fields["public_key"])
	assert.Equal(t, wasmHashHex, fields["wasm_hash"])
	assert.Len(t, fields, 2)
}

func Test_WalletCreationTransactionHandler_HelperMethods(t *testing.T) {
	engine := &engine.SubmitterEngine{MaxBaseFee: 100}
	rpcClient := createMockRPCClient()
	monitorSvc := tssMonitor.TSSMonitorService{
		Client: &sdpMonitorMocks.MockMonitorClient{},
	}
	handler, err := NewWalletCreationTransactionHandler(engine, rpcClient, monitorSvc)
	require.NoError(t, err)

	t.Run("calculateAdjustedBaseFee", func(t *testing.T) {
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
			// 100 - 50 = 50, math.Max(50, MinBaseFee) = math.Max(50, 100) = 100
			// MinBaseFee is 100, so math.Max(50, 100) = 100
			assert.Equal(t, int64(100), fee)
		})

		t.Run("with resource fee exceeding max", func(t *testing.T) {
			resp := protocol.SimulateTransactionResponse{
				MinResourceFee: 200,
			}
			fee := handler.calculateAdjustedBaseFee(resp)
			// 100 - 200 = -100, math.Max(-100, MinBaseFee) = MinBaseFee
			assert.Equal(t, int64(txnbuild.MinBaseFee), fee)
		})
	})
}

func Test_WalletCreationTransactionHandler_MonitoringBehavior(t *testing.T) {
	ctx := context.Background()
	txJob := &TxJob{
		Transaction: store.Transaction{
			AttemptsCount: 2, // Test reprocessing path
		},
		ChannelAccount: store.ChannelAccount{
			PublicKey: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
		},
	}
	jobUUID := "job-uuid"

	t.Run("MonitorTransactionProcessingSuccess with reprocessing", func(t *testing.T) {
		mMonitorClient := sdpMonitorMocks.NewMockMonitorClient(t)
		mMonitorClient.
			On("MonitorCounters", sdpMonitor.WalletCreationTransactionSuccessfulTag, mock.Anything).
			Return(nil).
			Once()

		tssMonitorService := tssMonitor.TSSMonitorService{
			Client: mMonitorClient,
		}
		handler := &WalletCreationTransactionHandler{
			monitorSvc: tssMonitorService,
		}

		handler.MonitorTransactionProcessingSuccess(ctx, txJob, jobUUID)
		mMonitorClient.AssertExpectations(t)
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
		handler := &WalletCreationTransactionHandler{
			monitorSvc: tssMonitorService,
		}

		handler.MonitorTransactionProcessingFailed(ctx, txJob, jobUUID, true, "retryable error")
		mMonitorClient.AssertExpectations(t)
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
		handler := &WalletCreationTransactionHandler{
			monitorSvc: tssMonitorService,
		}

		handler.MonitorTransactionReconciliationSuccess(ctx, txJob, jobUUID, ReconcileReprocessing)
		mMonitorClient.AssertExpectations(t)
	})
}
