package transactionsubmission

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/protocols/horizon"
	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stellar/go-stellar-sdk/support/render/problem"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	sdpMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

// A worker that outlives its lock must not overwrite a row that a newer job owns. The entry_archived restore path
// provides the slow step that makes worker A stale.
// Real store, selection query, worker, handler and signing; only Horizon is mocked.
func Test_TransactionWorker_staleWorkerCannotOverwriteNewerAttempt(t *testing.T) {
	t.Run("restore path, newer attempt in flight", func(t *testing.T) {
		l := newStaleWorkerLab(t)
		restoreArrived, releaseRestore := make(chan struct{}), make(chan struct{})
		cArrived, releaseC := make(chan struct{}), make(chan struct{})
		l.onSubmit(false, nil, horizon.Transaction{}, entryArchivedHorizonError())
		l.onSubmit(false, func() { close(cArrived); <-releaseC }, successfulHorizonResponse(t), nil)
		l.onSubmit(true, func() { close(restoreArrived); <-releaseRestore }, successfulHorizonResponse(t), nil)

		// ledger 100: A locks the row until 110, attempt A fails with entry_archived, A waits on its restore.
		doneA := l.runAsync(l.pick(100))
		waitOrFail(t, restoreArrived, "A's restore submission")
		hashA := l.row().StellarTransactionHash.String
		l.horizon.On("TransactionDetail", hashA).Return(horizon.Transaction{Successful: false}, nil).Once()

		// ledger 111: A's lock expired; B reconciles attempt A and resets the row.
		l.run(l.pick(111))

		// ledger 112: C locks the row until 122 and submits attempt C; Horizon holds the response.
		doneC := l.runAsync(l.pick(112))
		waitOrFail(t, cArrived, "attempt C")
		hashC := l.row().StellarTransactionHash.String

		l.releaseStaleWorker(releaseRestore, doneA, 110)
		l.requireHeldBy(hashC, 122)
		l.requireNotSelectable(122)

		close(releaseC)
		waitOrFail(t, doneC, "worker C")
		l.requireSuccess(hashC)
		assert.Equal(t, []string{hashA, hashC}, l.submittedPayments())
		l.requireNotSelectable(123)
		l.horizon.AssertExpectations(t)
		l.crash.AssertNotCalled(t, "LogAndReportErrors", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("restore path, newer attempt held after a Horizon 504", func(t *testing.T) {
		l := newStaleWorkerLab(t)
		restoreArrived, releaseRestore := make(chan struct{}), make(chan struct{})
		l.onSubmit(false, nil, horizon.Transaction{}, entryArchivedHorizonError())
		l.onSubmit(false, nil, horizon.Transaction{}, timeoutHorizonError())
		l.onSubmit(true, func() { close(restoreArrived); <-releaseRestore }, successfulHorizonResponse(t), nil)

		// ledger 100: A locks the row until 110, attempt A fails with entry_archived, A waits on its restore.
		doneA := l.runAsync(l.pick(100))
		waitOrFail(t, restoreArrived, "A's restore submission")
		hashA := l.row().StellarTransactionHash.String
		l.horizon.On("TransactionDetail", hashA).Return(horizon.Transaction{Successful: false}, nil).Once()

		// ledger 111: A's lock expired; B reconciles attempt A and resets the row.
		l.run(l.pick(111))

		// ledger 112: C submits attempt C, gets a 504 and holds its hash and lock until 122.
		l.run(l.pick(112))
		hashC := l.row().StellarTransactionHash.String

		l.releaseStaleWorker(releaseRestore, doneA, 110)
		l.requireHeldBy(hashC, 122)
		l.requireNotSelectable(122)

		// ledger 123: the hold has expired; F reconciles attempt C, which landed.
		landed := successfulHorizonResponse(t)
		landed.Hash = hashC
		l.horizon.On("TransactionDetail", hashC).Return(landed, nil).Once()
		l.run(l.pick(123))
		l.requireSuccess(hashC)
		assert.Equal(t, []string{hashA, hashC}, l.submittedPayments())
		l.horizon.AssertExpectations(t)
		l.crash.AssertNotCalled(t, "LogAndReportErrors", mock.Anything, mock.Anything, mock.Anything)
	})
}

// settableLedgerTracker stands in for the Horizon-backed tracker so the test controls the current ledger.
type settableLedgerTracker struct{ n atomic.Int64 }

func (s *settableLedgerTracker) GetLedgerNumber() (int, error) { return int(s.n.Load()), nil }

func (s *settableLedgerTracker) GetLedgerBounds() (*txnbuild.LedgerBounds, error) {
	return &txnbuild.LedgerBounds{MaxLedger: uint32(s.n.Load() + preconditions.IncrementForMaxLedgerBounds)}, nil
}

type staleWorkerLab struct {
	t          *testing.T
	ctx        context.Context
	pool       db.DBConnectionPool
	txModel    *store.TransactionModel
	chAccModel *store.ChannelAccountModel
	bundles    *store.ChannelTransactionBundleModel
	tracker    *settableLedgerTracker
	horizon    *horizonclient.MockClient
	eng        *engine.SubmitterEngine
	handler    *PaymentTransactionHandler
	monitorSvc tssMonitor.TSSMonitorService
	limiter    engine.TransactionProcessingLimiter
	crash      *crashtracker.MockCrashTrackerClient
	txID       string

	mu       sync.Mutex
	payments []string // outer hashes of the payment envelopes submitted to Horizon, in order
}

// newStaleWorkerLab creates one payment to a contract destination, three channel accounts and real signing.
func newStaleWorkerLab(t *testing.T) *staleWorkerLab {
	t.Helper()
	ctx := context.Background()
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	t.Cleanup(func() { dbt.Close() })
	pool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	bundles, err := store.NewChannelTransactionBundleModel(pool)
	require.NoError(t, err)
	store.CreateChannelAccountFixturesEncrypted(t, ctx, pool, encrypter, chAccEncryptionPassphrase, 3)
	tx := store.CreateTransactionFixture(t, ctx, pool, store.TransactionFixture{
		ExternalID:         uuid.NewString(),
		TransactionType:    store.TransactionTypePayment,
		AssetCode:          "USDC",
		AssetIssuer:        "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
		DestinationAddress: "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC",
		Status:             store.TransactionStatusPending,
		Amount:             decimal.NewFromInt(100),
		TenantID:           uuid.NewString(),
	})

	distributionKP := keypair.MustRandom()
	distAccount := schema.NewStellarEnvTransactionAccount(distributionKP.Address())
	resolver := sigMocks.NewMockDistributionAccountResolver(t)
	resolver.On("DistributionAccountForWallet", mock.Anything, mock.Anything, mock.Anything).Return(distAccount, nil).Maybe()
	resolver.On("DistributionAccount", mock.Anything, mock.Anything).Return(distAccount, nil).Maybe()

	tracker := &settableLedgerTracker{}
	sigService, err := signing.NewSignatureService(signing.SignatureServiceOptions{
		NetworkPassphrase:           network.TestNetworkPassphrase,
		DBConnectionPool:            pool,
		DistributionPrivateKey:      distributionKP.Seed(),
		ChAccEncryptionPassphrase:   chAccEncryptionPassphrase,
		DistAccEncryptionPassphrase: keypair.MustRandom().Seed(),
		LedgerNumberTracker:         tracker,
		DistributionAccountResolver: resolver,
	})
	require.NoError(t, err)

	hz := &horizonclient.MockClient{}
	hz.On("AccountDetail", mock.Anything).Return(horizon.Account{Sequence: 4242}, nil).Maybe()
	eng := &engine.SubmitterEngine{HorizonClient: hz, LedgerNumberTracker: tracker, SignatureService: sigService, MaxBaseFee: 100 * txnbuild.MinBaseFee}

	mClient := sdpMonitor.NewMockMonitorClient(t)
	mClient.On("MonitorCounters", mock.Anything, mock.Anything).Return().Maybe()
	mClient.On("MonitorDuration", mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
	mClient.On("MonitorHistogram", mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
	monitorSvc := tssMonitor.TSSMonitorService{Version: "0.01", GitCommitHash: "0xABC", Client: mClient}

	handler, err := NewPaymentTransactionHandler(eng, monitorSvc)
	require.NoError(t, err)
	crash := crashtracker.NewMockCrashTrackerClient(t)
	crash.On("LogAndReportErrors", mock.Anything, mock.Anything, mock.Anything).Return().Maybe()

	return &staleWorkerLab{
		t: t, ctx: ctx, pool: pool, txModel: store.NewTransactionModel(pool), chAccModel: store.NewChannelAccountModel(pool),
		bundles: bundles, tracker: tracker, horizon: hz, eng: eng, handler: handler, monitorSvc: monitorSvc,
		limiter: engine.NewTransactionProcessingLimiter(1), crash: crash, txID: tx.ID,
	}
}

// pick does what the Manager does at the given ledger: lock the row and a channel account until ledger+10.
func (l *staleWorkerLab) pick(ledger int) TxJob {
	l.t.Helper()
	l.tracker.n.Store(int64(ledger))
	bundles, err := l.bundles.LoadAndLockTuples(l.ctx, ledger, ledger+preconditions.IncrementForMaxLedgerBounds, 1)
	require.NoError(l.t, err)
	require.Len(l.t, bundles, 1, "expected the row to be selectable at ledger %d", ledger)
	require.Equal(l.t, l.txID, bundles[0].Transaction.ID)
	return TxJob(*bundles[0])
}

func (l *staleWorkerLab) requireNotSelectable(ledger int) {
	l.t.Helper()
	bundles, err := l.bundles.LoadAndLockTuples(l.ctx, ledger, ledger+preconditions.IncrementForMaxLedgerBounds, 1)
	require.NoError(l.t, err)
	require.Empty(l.t, bundles, "expected the row not to be selectable at ledger %d", ledger)
}

func (l *staleWorkerLab) worker() TransactionWorker {
	w, err := NewTransactionWorker(l.pool, l.txModel, l.chAccModel, l.eng, l.crash, l.limiter, l.monitorSvc, l.handler)
	require.NoError(l.t, err)
	return w
}

func (l *staleWorkerLab) run(job TxJob) {
	w := l.worker()
	w.Run(l.ctx, &job)
}

func (l *staleWorkerLab) runAsync(job TxJob) <-chan struct{} {
	w := l.worker()
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Run(l.ctx, &job)
	}()
	return done
}

// releaseStaleWorker lets the stale worker finish and asserts it stopped on its lost lock.
func (l *staleWorkerLab) releaseStaleWorker(release chan struct{}, done <-chan struct{}, lockedUntil int) {
	l.t.Helper()
	getEntries := log.DefaultLogger.StartTest(log.WarnLevel)
	close(release)
	waitOrFail(l.t, done, "stale worker A")
	var lockLost bool
	for _, e := range getEntries() {
		if e.Message == "lock lost to a newer job; skipping remaining writes" && e.Data["locked_until_ledger_number"] == lockedUntil {
			lockLost = true
		}
	}
	require.True(l.t, lockLost, "expected a lock-lost warning for the job locked until ledger %d", lockedUntil)
}

func (l *staleWorkerLab) row() *store.Transaction {
	l.t.Helper()
	tx, err := l.txModel.Get(l.ctx, l.txID)
	require.NoError(l.t, err)
	return tx
}

func (l *staleWorkerLab) requireHeldBy(hash string, lockedUntil int) {
	l.t.Helper()
	tx := l.row()
	require.Equal(l.t, store.TransactionStatusProcessing, tx.Status)
	require.Equal(l.t, hash, tx.StellarTransactionHash.String)
	require.EqualValues(l.t, lockedUntil, tx.LockedUntilLedgerNumber.Int32)
}

func (l *staleWorkerLab) requireSuccess(hash string) {
	l.t.Helper()
	tx := l.row()
	require.Equal(l.t, store.TransactionStatusSuccess, tx.Status)
	require.Equal(l.t, hash, tx.StellarTransactionHash.String)
}

// onSubmit queues Horizon's next answer to a payment or, with restore, a RestoreFootprint submission.
func (l *staleWorkerLab) onSubmit(restore bool, run func(), resp horizon.Transaction, err error) {
	isRestore := mock.MatchedBy(func(fb *txnbuild.FeeBumpTransaction) bool {
		_, ok := fb.InnerTransaction().Operations()[0].(*txnbuild.RestoreFootprint)
		return ok == restore
	})
	l.horizon.On("SubmitFeeBumpTransactionWithOptions", isRestore, mock.Anything).
		Run(func(args mock.Arguments) {
			if !restore {
				hash, hashErr := args.Get(0).(*txnbuild.FeeBumpTransaction).HashHex(network.TestNetworkPassphrase)
				assert.NoError(l.t, hashErr)
				l.mu.Lock()
				l.payments = append(l.payments, hash)
				l.mu.Unlock()
			}
			if run != nil {
				run()
			}
		}).
		Return(resp, err).
		Once()
}

func (l *staleWorkerLab) submittedPayments() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.payments...)
}

func waitOrFail(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(30 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// entryArchivedHorizonError is Horizon's 400 for a transaction included and failed with entry_archived.
func entryArchivedHorizonError() error {
	return &horizonclient.Error{Problem: problem.P{
		Type: "https://stellar.org/horizon-errors/transaction_failed", Title: "Transaction Failed", Status: http.StatusBadRequest,
		Extras: map[string]interface{}{"result_codes": map[string]interface{}{"transaction": "tx_failed", "operations": []string{"entry_archived"}}},
	}}
}

// timeoutHorizonError is Horizon's 504: the envelope may still be included.
func timeoutHorizonError() error {
	return &horizonclient.Error{Problem: problem.P{Type: "https://stellar.org/horizon-errors/timeout", Title: "Timeout", Status: http.StatusGatewayTimeout}}
}

func successfulHorizonResponse(t *testing.T) horizon.Transaction {
	t.Helper()
	res := xdr.TransactionResult{FeeCharged: 200, Result: xdr.TransactionResultResult{Code: xdr.TransactionResultCodeTxSuccess, Results: &[]xdr.OperationResult{}}}
	resultXDR, err := xdr.MarshalBase64(res)
	require.NoError(t, err)
	return horizon.Transaction{Successful: true, ResultXdr: resultXDR}
}
