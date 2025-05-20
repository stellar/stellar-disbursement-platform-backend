package transactionsubmission

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	sdpMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	monitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	storeMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_NewPaymentTransactionHandler(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	distributionKP := keypair.MustRandom()
	processingTestPassphrase := keypair.MustRandom().Seed()
	distAccEncryptionPassphrase := keypair.MustRandom().Seed()

	wantSigService, err := signing.NewSignatureService(signing.SignatureServiceOptions{
		NetworkPassphrase:         network.TestNetworkPassphrase,
		DBConnectionPool:          dbConnectionPool,
		DistributionPrivateKey:    distributionKP.Seed(),
		ChAccEncryptionPassphrase: processingTestPassphrase,
		LedgerNumberTracker:       preconditionsMocks.NewMockLedgerNumberTracker(t),

		DistAccEncryptionPassphrase: distAccEncryptionPassphrase,
		DistributionAccountResolver: sigMocks.NewMockDistributionAccountResolver(t),
	})
	require.NoError(t, err)

	hClient := &horizonclient.Client{
		HorizonURL: "https://horizon-testnet.stellar.org",
		HTTP:       httpclient.DefaultClient(),
	}
	ledgerNumberTracker, err := preconditions.NewLedgerNumberTracker(hClient)
	require.NoError(t, err)
	wantSubmitterEngine := engine.SubmitterEngine{
		HorizonClient:       hClient,
		LedgerNumberTracker: ledgerNumberTracker,
		SignatureService:    wantSigService,
		MaxBaseFee:          100,
	}

	tssMonitorSvc := tssMonitor.TSSMonitorService{
		GitCommitHash: "gitCommitHash0x",
		Version:       "version123",
	}

	testCases := []struct {
		name          string
		engine        *engine.SubmitterEngine
		tssMonitorSvc tssMonitor.TSSMonitorService
		wantError     string
	}{
		{
			name:          "validate engine",
			tssMonitorSvc: tssMonitorSvc,
			wantError:     "engine cannot be nil",
		},
		{
			name:      "validate tssMonitorSvc",
			engine:    &wantSubmitterEngine,
			wantError: "monitor service cannot be nil",
		},
		{
			name:          "🎉 successfully returns a new payment handler",
			engine:        &wantSubmitterEngine,
			tssMonitorSvc: tssMonitorSvc,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			paymentHandler, err := NewPaymentTransactionHandler(tc.engine, tc.tssMonitorSvc)
			if tc.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantError)
				assert.Nil(t, paymentHandler)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, paymentHandler)
				assert.Equal(t, paymentHandler.engine, tc.engine)
				assert.Equal(t, paymentHandler.monitorSvc, tc.tssMonitorSvc)
			}
		})
	}
}

func Test_PaymentHandler_BuildInnerTransaction(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	const currentLedger = 1
	const lockedToLedger = 2
	const accountSequence = 123

	distributionKP := keypair.MustRandom()
	distAccount := schema.NewStellarEnvTransactionAccount(distributionKP.Address())
	mDistAccResolver := sigMocks.NewMockDistributionAccountResolver(t)
	mDistAccResolver.
		On("DistributionAccount", ctx, mock.AnythingOfType("string")).
		Return(distAccount, nil).
		Maybe()
	distAccEncryptionPassphrase := keypair.MustRandom().Seed()

	sigService, err := signing.NewSignatureService(signing.SignatureServiceOptions{
		NetworkPassphrase:           network.TestNetworkPassphrase,
		DBConnectionPool:            dbConnectionPool,
		DistributionPrivateKey:      distributionKP.Seed(),
		ChAccEncryptionPassphrase:   chAccEncryptionPassphrase,
		LedgerNumberTracker:         preconditionsMocks.NewMockLedgerNumberTracker(t),
		DistributionAccountResolver: mDistAccResolver,
		DistAccEncryptionPassphrase: distAccEncryptionPassphrase,
	})
	require.NoError(t, err)

	testCases := []struct {
		name               string
		assetCode          string
		assetIssuer        string
		wantErrorContains  string
		destinationAddress string
		memoType           schema.MemoType
		memoValue          string
		wantMemo           txnbuild.Memo
	}{
		{
			name:              "returns an error if the asset code is empty",
			wantErrorContains: "asset code cannot be empty",
		},
		{
			name:              "returns an error if the asset code is not XLM and the issuer is not valid",
			assetCode:         "USDC",
			assetIssuer:       "FOOBAR",
			wantErrorContains: "invalid asset issuer: FOOBAR",
		},
		{
			name:               "returns an error if memo is present for C destination",
			assetCode:          "USDC",
			assetIssuer:        "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			destinationAddress: "CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53",
			memoType:           schema.MemoTypeText,
			memoValue:          "HelloWorld!",
			wantErrorContains:  "memo is not supported for contract destination",
		},
		{
			name:               "🎉 successfully build a payment transaction for G destination",
			assetCode:          "USDC",
			assetIssuer:        "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			destinationAddress: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
		},
		{
			name:               "🎉 successfully build a payment transaction with native asset for G destination",
			assetCode:          "XLM",
			assetIssuer:        "",
			destinationAddress: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
		},
		{
			name:               "🎉 successfully build a payment transaction with memo for G destination",
			assetCode:          "USDC",
			assetIssuer:        "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			destinationAddress: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
			memoType:           schema.MemoTypeText,
			memoValue:          "HelloWorld!",
			wantMemo:           txnbuild.MemoText("HelloWorld!"),
		},
		{
			name:               "🎉 successfully build a SAC transfer transaction for C destination",
			assetCode:          "USDC",
			assetIssuer:        "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			destinationAddress: "CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53",
			wantMemo:           nil,
		},
		{
			name:               "🎉 successfully build a SAC transfer transaction with native asset for C destination",
			assetCode:          "XLM",
			assetIssuer:        "",
			destinationAddress: "CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53",
			wantMemo:           nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
			defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)
			defer tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

			tnt := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "test-tenant", distributionKP.Address())
			txJob := createTxJobFixture(t, ctx, dbConnectionPool, true, currentLedger, lockedToLedger, tnt.ID)
			txJob.Transaction.AssetCode = tc.assetCode
			txJob.Transaction.AssetIssuer = tc.assetIssuer
			txJob.Transaction.Destination = tc.destinationAddress
			txJob.Transaction.Memo = tc.memoValue
			txJob.Transaction.MemoType = tc.memoType

			// mock horizon
			mockHorizon := &horizonclient.MockClient{}
			mockStore := &storeMocks.MockChannelAccountStore{}
			mockStore.On("Get", ctx, mock.Anything, txJob.ChannelAccount.PublicKey, 0).Return(txJob.ChannelAccount, nil)

			// Create a transaction worker:
			mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
			submitterEngine := &engine.SubmitterEngine{
				HorizonClient:       mockHorizon,
				LedgerNumberTracker: mLedgerNumberTracker,
				SignatureService:    sigService,
				MaxBaseFee:          100,
			}
			paymentHandler := &PaymentTransactionHandler{
				engine: submitterEngine,
			}

			gotInnerTx, err := paymentHandler.BuildInnerTransaction(ctx, &txJob, accountSequence, distributionKP.Address())
			if tc.wantErrorContains != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantErrorContains)
				assert.Nil(t, gotInnerTx)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, gotInnerTx)

				// Check that the transaction was built correctly:
				var wantAsset txnbuild.Asset = txnbuild.NativeAsset{}
				if strings.ToUpper(txJob.Transaction.AssetCode) != "XLM" {
					wantAsset = txnbuild.CreditAsset{
						Code:   txJob.Transaction.AssetCode,
						Issuer: txJob.Transaction.AssetIssuer,
					}
				}

				var operation txnbuild.Operation
				amount := strconv.FormatFloat(txJob.Transaction.Amount, 'f', 6, 32)
				if strkey.IsValidEd25519PublicKey(tc.destinationAddress) {
					operation = &txnbuild.Payment{
						SourceAccount: distributionKP.Address(),
						Amount:        amount,
						Destination:   txJob.Transaction.Destination,
						Asset:         wantAsset,
					}
				} else if strkey.IsValidContractAddress(tc.destinationAddress) {
					params := txnbuild.PaymentToContractParams{
						NetworkPassphrase: network.TestNetworkPassphrase,
						Destination:       txJob.Transaction.Destination,
						Amount:            amount,
						Asset:             wantAsset,
						SourceAccount:     distributionKP.Address(),
					}
					op, _ := txnbuild.NewPaymentToContract(params)
					operation = &op
				}

				wantInnerTx, err := txnbuild.NewTransaction(
					txnbuild.TransactionParams{
						SourceAccount: &txnbuild.SimpleAccount{
							AccountID: txJob.ChannelAccount.PublicKey,
							Sequence:  accountSequence,
						},
						Memo:       tc.wantMemo,
						Operations: []txnbuild.Operation{operation},
						BaseFee:    int64(submitterEngine.MaxBaseFee),
						Preconditions: txnbuild.Preconditions{
							TimeBounds:   txnbuild.NewTimeout(300),
							LedgerBounds: &txnbuild.LedgerBounds{MaxLedger: uint32(txJob.LockedUntilLedgerNumber)},
						},
						IncrementSequenceNum: true,
					},
				)
				require.NoError(t, err)
				assert.Equal(t, wantInnerTx, gotInnerTx)
			}
		})
	}
}

func Test_PaymentHandler_BuildSuccessEvent(t *testing.T) {
	paymentHandler := &PaymentTransactionHandler{}

	ctx := context.Background()
	txJob := TxJob{
		Transaction: store.Transaction{
			ID:                     "tx-id",
			ExternalID:             "payment-id",
			TenantID:               "tenant-id",
			StellarTransactionHash: sql.NullString{},
		},
	}
	msg, err := paymentHandler.BuildSuccessEvent(ctx, &txJob)
	require.NoError(t, err)

	gotPaymentCompletedAt := msg.Data.(schemas.EventPaymentCompletedData).PaymentCompletedAt
	assert.WithinDuration(t, time.Now(), gotPaymentCompletedAt, time.Millisecond*100)
	wantMsg := &events.Message{
		Topic:    events.PaymentCompletedTopic,
		Key:      txJob.Transaction.ExternalID,
		TenantID: txJob.Transaction.TenantID,
		Type:     events.PaymentCompletedSuccessType,
		Data: schemas.EventPaymentCompletedData{
			TransactionID:        txJob.Transaction.ID,
			PaymentID:            txJob.Transaction.ExternalID,
			PaymentStatus:        string(data.SuccessPaymentStatus),
			PaymentCompletedAt:   gotPaymentCompletedAt,
			StellarTransactionID: txJob.Transaction.StellarTransactionHash.String,
		},
	}
	assert.Equal(t, wantMsg, msg)
}

func Test_PaymentHandler_BuildFailureEvent(t *testing.T) {
	paymentHandler := &PaymentTransactionHandler{}

	ctx := context.Background()
	txJob := TxJob{
		Transaction: store.Transaction{
			ID:                     "tx-id",
			ExternalID:             "payment-id",
			TenantID:               "tenant-id",
			StellarTransactionHash: sql.NullString{},
		},
	}
	hErr := &utils.HorizonErrorWrapper{}
	msg, err := paymentHandler.BuildFailureEvent(ctx, &txJob, hErr)
	require.NoError(t, err)

	gotPaymentCompletedAt := msg.Data.(schemas.EventPaymentCompletedData).PaymentCompletedAt
	assert.WithinDuration(t, time.Now(), gotPaymentCompletedAt, time.Millisecond*100)
	wantMsg := &events.Message{
		Topic:    events.PaymentCompletedTopic,
		Key:      txJob.Transaction.ExternalID,
		TenantID: txJob.Transaction.TenantID,
		Type:     events.PaymentCompletedErrorType,
		Data: schemas.EventPaymentCompletedData{
			TransactionID:        txJob.Transaction.ID,
			PaymentID:            txJob.Transaction.ExternalID,
			PaymentStatus:        string(data.FailedPaymentStatus),
			PaymentStatusMessage: hErr.Error(),
			PaymentCompletedAt:   gotPaymentCompletedAt,
			StellarTransactionID: txJob.Transaction.StellarTransactionHash.String,
		},
	}
	assert.Equal(t, wantMsg, msg)
}

func Test_PaymentHandler_MonitorTransactionProcessingStarted(t *testing.T) {
	ctx := context.Background()
	txJob := TxJob{
		Transaction: store.Transaction{},
	}
	jobUUID := "job-uuid"

	mMonitorClient := monitorMocks.NewMockMonitorClient(t)
	mMonitorClient.
		On("MonitorCounters", sdpMonitor.PaymentProcessingStartedTag, mock.Anything).
		Return(nil).
		Once()
	tssMonitorService := tssMonitor.TSSMonitorService{
		Version:       "0.01",
		GitCommitHash: "0xABC",
		Client:        mMonitorClient,
	}
	paymentHandler := &PaymentTransactionHandler{
		monitorSvc: tssMonitorService,
	}

	paymentHandler.MonitorTransactionProcessingStarted(ctx, &txJob, jobUUID)
}

func Test_PaymentHandler_MonitorTransactionProcessingSuccess(t *testing.T) {
	ctx := context.Background()
	txJob := TxJob{
		Transaction: store.Transaction{
			CompletedAt:            func() *time.Time { t := time.Now(); return &t }(),
			AttemptsCount:          1,
			XDRSent:                sql.NullString{String: "xdr-sent", Valid: true},
			XDRReceived:            sql.NullString{String: "xdr-received", Valid: true},
			StellarTransactionHash: sql.NullString{String: "tx-hash", Valid: true},
		},
	}
	jobUUID := "job-uuid"

	mMonitorClient := monitorMocks.NewMockMonitorClient(t)
	mMonitorClient.
		On("MonitorCounters", sdpMonitor.PaymentTransactionSuccessfulTag, mock.Anything).
		Return(nil).
		Once()
	tssMonitorService := tssMonitor.TSSMonitorService{
		Version:       "0.01",
		GitCommitHash: "0xABC",
		Client:        mMonitorClient,
	}
	paymentHandler := &PaymentTransactionHandler{
		monitorSvc: tssMonitorService,
	}

	paymentHandler.MonitorTransactionProcessingSuccess(ctx, &txJob, jobUUID)
}

func Test_PaymentHandler_MonitorTransactionProcessingFailed(t *testing.T) {
	ctx := context.Background()
	txJob := TxJob{
		Transaction: store.Transaction{
			CompletedAt:            func() *time.Time { t := time.Now(); return &t }(),
			AttemptsCount:          1,
			XDRSent:                sql.NullString{String: "xdr-sent", Valid: true},
			XDRReceived:            sql.NullString{String: "xdr-received", Valid: true},
			StellarTransactionHash: sql.NullString{String: "tx-hash", Valid: true},
		},
	}
	jobUUID := "job-uuid"
	isRetryable := true
	errStack := "error stack"

	mMonitorClient := monitorMocks.NewMockMonitorClient(t)
	mMonitorClient.
		On("MonitorCounters", sdpMonitor.PaymentErrorTag, mock.Anything).
		Return(nil).
		Once()
	tssMonitorService := tssMonitor.TSSMonitorService{
		Version:       "0.01",
		GitCommitHash: "0xABC",
		Client:        mMonitorClient,
	}
	paymentHandler := &PaymentTransactionHandler{
		monitorSvc: tssMonitorService,
	}

	paymentHandler.MonitorTransactionProcessingFailed(ctx, &txJob, jobUUID, isRetryable, errStack)
}

func Test_PaymentHandler_MonitorTransactionReconciliationSuccess(t *testing.T) {
	ctx := context.Background()
	txJob := TxJob{
		Transaction: store.Transaction{
			CompletedAt:            func() *time.Time { t := time.Now(); return &t }(),
			XDRSent:                sql.NullString{String: "xdr-sent", Valid: true},
			XDRReceived:            sql.NullString{String: "xdr-received", Valid: true},
			StellarTransactionHash: sql.NullString{String: "tx-hash", Valid: true},
		},
	}
	jobUUID := "job-uuid"

	mMonitorClient := monitorMocks.NewMockMonitorClient(t)
	mMonitorClient.
		On("MonitorCounters", sdpMonitor.PaymentReconciliationSuccessfulTag, mock.Anything).
		Return(nil).
		Once()
	tssMonitorService := tssMonitor.TSSMonitorService{
		Version:       "0.01",
		GitCommitHash: "0xABC",
		Client:        mMonitorClient,
	}
	paymentHandler := &PaymentTransactionHandler{
		monitorSvc: tssMonitorService,
	}

	paymentHandler.MonitorTransactionReconciliationSuccess(ctx, &txJob, jobUUID, ReconcileSuccess)
}

func Test_PaymentHandler_MonitorTransactionReconciliationFailure(t *testing.T) {
	ctx := context.Background()
	txJob := TxJob{
		Transaction: store.Transaction{
			CompletedAt:            func() *time.Time { t := time.Now(); return &t }(),
			XDRSent:                sql.NullString{String: "xdr-sent", Valid: true},
			XDRReceived:            sql.NullString{String: "xdr-received", Valid: true},
			StellarTransactionHash: sql.NullString{String: "tx-hash", Valid: true},
		},
	}
	jobUUID := "job-uuid"
	isHorizonErr := true
	errStack := "error stack"

	mMonitorClient := monitorMocks.NewMockMonitorClient(t)
	mMonitorClient.
		On("MonitorCounters", sdpMonitor.PaymentReconciliationFailureTag, mock.Anything).
		Return(nil).
		Once()
	tssMonitorService := tssMonitor.TSSMonitorService{
		Version:       "0.01",
		GitCommitHash: "0xABC",
		Client:        mMonitorClient,
	}
	paymentHandler := &PaymentTransactionHandler{
		monitorSvc: tssMonitorService,
	}

	paymentHandler.MonitorTransactionReconciliationFailure(ctx, &txJob, jobUUID, isHorizonErr, errStack)
}
