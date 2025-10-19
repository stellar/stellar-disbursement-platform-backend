package transactionsubmission

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	sdpUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

const serviceName = "Transaction Submission Service"

type SubmitterOptions struct {
	NumChannelAccounts   int
	QueuePollingInterval int
	MonitorService       tssMonitor.TSSMonitorService
	CrashTrackerClient   crashtracker.CrashTrackerClient

	SubmitterEngine  engine.SubmitterEngine
	DBConnectionPool db.DBConnectionPool
}

func (so *SubmitterOptions) validate() error {
	if so.DBConnectionPool == nil {
		return fmt.Errorf("database connection pool cannot be nil")
	}

	if err := so.SubmitterEngine.Validate(); err != nil {
		return fmt.Errorf("validating submitter engine: %w", err)
	}

	if so.NumChannelAccounts < services.MinNumberOfChannelAccounts || so.NumChannelAccounts > services.MaxNumberOfChannelAccounts {
		return fmt.Errorf("num channel accounts must stay in the range from %d to %d", services.MinNumberOfChannelAccounts, services.MaxNumberOfChannelAccounts)
	}

	if so.QueuePollingInterval < 6 {
		return fmt.Errorf("queue polling interval must be greater than 6 seconds")
	}

	if sdpUtils.IsEmpty(so.MonitorService) {
		return fmt.Errorf("monitor service cannot be nil")
	}

	return nil
}

type Manager struct {
	// Data model:
	dbConnectionPool db.DBConnectionPool
	txModel          *store.TransactionModel
	chAccModel       *store.ChannelAccountModel
	chTxBundleModel  *store.ChannelTransactionBundleModel
	// job-related:
	pollingInterval     time.Duration
	txProcessingLimiter engine.TransactionProcessingLimiter
	// transaction submission:
	engine *engine.SubmitterEngine
	// crash & metrics monitoring:
	monitorService     tssMonitor.TSSMonitorService
	crashTrackerClient crashtracker.CrashTrackerClient
}

func NewManager(ctx context.Context, opts SubmitterOptions) (m *Manager, err error) {
	// initialize crash tracker client
	crashTrackerClient := opts.CrashTrackerClient
	if opts.CrashTrackerClient == nil {
		log.Ctx(ctx).Warn("crash tracker client not set, using DRY_RUN client")
		crashTrackerClient, err = crashtracker.NewDryRunClient()
		if err != nil {
			return nil, fmt.Errorf("unable to initialize DRY_RUN crash tracker client: %w", err)
		}
	}
	defer crashTrackerClient.FlushEvents(2 * time.Second)
	defer crashTrackerClient.Recover()

	// validate options
	err = opts.validate()
	if err != nil {
		return nil, fmt.Errorf("validating options: %w", err)
	}

	txModel := store.NewTransactionModel(opts.DBConnectionPool)
	chAccModel := &store.ChannelAccountModel{DBConnectionPool: opts.DBConnectionPool}
	chTxBundleModel, err := store.NewChannelTransactionBundleModel(opts.DBConnectionPool)
	if err != nil {
		return nil, fmt.Errorf("initializing channel transaction bundle model: %w", err)
	}

	// validate if we have any channel accounts in the DB.
	chAccCount, err := chAccModel.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("counting channel accounts: %w", err)
	}
	if chAccCount == 0 {
		return nil, fmt.Errorf("no channel accounts found in the database, use the 'channel-accounts ensure' command to configure the number of accounts you want to use")
	}
	log.Ctx(ctx).Infof("Found '%d' channel accounts in the database...", chAccCount)

	if opts.NumChannelAccounts > chAccCount {
		log.Ctx(ctx).Warnf("The number of channel accounts in the database is smaller than expected, (%d < %d)", chAccCount, opts.NumChannelAccounts)
	}

	txProcessingLimiter := engine.NewTransactionProcessingLimiter(opts.NumChannelAccounts)

	return &Manager{
		dbConnectionPool: opts.DBConnectionPool,
		chAccModel:       chAccModel,
		txModel:          txModel,
		chTxBundleModel:  chTxBundleModel,

		pollingInterval:     time.Second * time.Duration(opts.QueuePollingInterval),
		txProcessingLimiter: txProcessingLimiter,

		engine: &opts.SubmitterEngine,

		crashTrackerClient: crashTrackerClient,
		monitorService:     opts.MonitorService,
	}, nil
}

func (m *Manager) ProcessTransactions(ctx context.Context) {
	defer m.crashTrackerClient.FlushEvents(2 * time.Second)
	defer m.crashTrackerClient.Recover()
	log.Ctx(ctx).Infof("Starting %s...", serviceName)

	// initialize signal channel, to react to OS signals
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	ticker := time.NewTicker(m.pollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Ctx(ctx).Infof("Stopping %s due to context cancellation...", serviceName)
			return

		case sig := <-signalChan:
			log.Ctx(ctx).Infof("Stopping %s due to OS signal '%+v'", serviceName, sig)
			return

		case <-ticker.C:
			log.Ctx(ctx).Debug("Loading transactions from database...")
			jobs, err := m.loadReadyForProcessingBundles(ctx)
			if err != nil {
				err = fmt.Errorf("attempting to load transactions from database: %w", err)
				if errors.Is(err, store.ErrInsuficientChannelAccounts) {
					// TODO: should we handle 'errors.Is(err, ErrInsuficientChannelAccounts)' differently?
					log.Ctx(ctx).Warn(err)
				} else {
					m.crashTrackerClient.LogAndReportErrors(ctx, err, "")
				}
				continue
			}

			log.Ctx(ctx).Debugf("Loaded '%d' transactions from database", len(jobs))

			for _, job := range jobs {
				worker, err := NewTransactionWorker(
					m.dbConnectionPool,
					m.txModel,
					m.chAccModel,
					m.engine,
					m.crashTrackerClient,
					m.txProcessingLimiter,
					m.monitorService,
				)
				if err != nil {
					m.crashTrackerClient.LogAndReportErrors(ctx, err, "")
					continue
				}

				txJob := TxJob(*job)
				go worker.Run(ctx, &txJob)
			}
		}
	}
}

// loadReadyForProcessingBundles loads a list of {channelAccount, Transaction, LedgerBoundsMax} bundles from the
// database which are ready to be processed. The bundles are locked for processing ar rge database, so that other
// instances of the process don't pick them up.
func (m *Manager) loadReadyForProcessingBundles(ctx context.Context) ([]*store.ChannelTransactionBundle, error) {
	currentLedgerNumber, err := m.engine.LedgerNumberTracker.GetLedgerNumber()
	if err != nil {
		return nil, fmt.Errorf("getting current ledger number: %w", err)
	}
	lockToLedgerNumber := currentLedgerNumber + preconditions.IncrementForMaxLedgerBounds

	chTxBundles, err := m.chTxBundleModel.LoadAndLockTuples(ctx, currentLedgerNumber, lockToLedgerNumber, m.txProcessingLimiter.LimitValue())
	if err != nil {
		return nil, fmt.Errorf("loading channel transaction bundles: %w", err)
	}

	return chTxBundles, nil
}
