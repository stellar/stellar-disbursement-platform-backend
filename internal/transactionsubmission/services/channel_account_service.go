package services

import (
	"context"
	"fmt"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	txSub "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

const advisoryLock = int(2172398390434160)

type ChannelAccountsService struct {
	dbConnectionPool    db.DBConnectionPool
	caStore             store.ChannelAccountStore
	horizonClient       horizonclient.ClientInterface
	ledgerNumberTracker engine.LedgerNumberTracker
}

type ChannelAccountsServiceInterface interface {
	CreateChannelAccountsOnChain(context.Context, ChannelAccountServiceOptions) error
	VerifyChannelAccounts(context.Context, ChannelAccountServiceOptions) error
	DeleteChannelAccount(context.Context, ChannelAccountServiceOptions) error
	EnsureChannelAccountsCount(context.Context, ChannelAccountServiceOptions) error
	ViewChannelAccounts(context.Context) error
}

// make sure *ChannelAccountsService implements ChannelAccountsServiceInterface:
var _ ChannelAccountsServiceInterface = (*ChannelAccountsService)(nil)

type ChannelAccountServiceOptions struct {
	ChannelAccountID       string
	DatabaseDSN            string
	DeleteAllAccounts      bool
	DeleteInvalidAcccounts bool
	HorizonUrl             string
	MaxBaseFee             int
	NetworkPassphrase      string
	NumChannelAccounts     int
	RootSeed               string
}

func NewChannelAccountService(ctx context.Context, opts ChannelAccountServiceOptions) (*ChannelAccountsService, error) {
	dbConnectionPool, err := db.OpenDBConnectionPool(opts.DatabaseDSN)
	if err != nil {
		return nil, fmt.Errorf("opening db connection pool: %w", err)
	}

	caModel := &store.ChannelAccountModel{DBConnectionPool: dbConnectionPool}
	horizonClient := &horizonclient.Client{
		HorizonURL: opts.HorizonUrl,
		HTTP:       httpclient.DefaultClient(),
	}

	ledgerNumberTracker, err := engine.NewLedgerNumberTracker(horizonClient)
	if err != nil {
		return nil, fmt.Errorf("cannot create new ledger number tracker")
	}

	err = acquireAdvisoryLockForCommand(ctx, dbConnectionPool)
	if err != nil {
		return nil, fmt.Errorf("failed getting db advisory lock: %w", err)
	}

	return &ChannelAccountsService{
		dbConnectionPool:    dbConnectionPool,
		caStore:             caModel,
		horizonClient:       horizonClient,
		ledgerNumberTracker: ledgerNumberTracker,
	}, nil
}

// CreateChannelAccountsOnChain creates a specified count of sponsored channel accounts onchain and internally in the database.
func (s *ChannelAccountsService) CreateChannelAccountsOnChain(ctx context.Context, opts ChannelAccountServiceOptions) error {
	log.Ctx(ctx).Infof("NumChannelAccounts: %d, Horizon: %s, Passphrase: %s", opts.NumChannelAccounts, opts.HorizonUrl, opts.NetworkPassphrase)
	// createAccountsInBatch creates count number of channel accounts in batches of MaxBatchSize or less per loop
	err := createAccountsInBatch(ctx, s.dbConnectionPool, opts, s.horizonClient, s.caStore, s.ledgerNumberTracker)
	if err != nil {
		return fmt.Errorf("creating channel accounts in batch in CreateChannelAccountsOnChain: %w", err)
	}

	return nil
}

func createAccountsInBatch(
	ctx context.Context,
	dbConnectionPool db.DBConnectionPool,
	opts ChannelAccountServiceOptions,
	horizonClient horizonclient.ClientInterface,
	chAccModel store.ChannelAccountStore,
	ledgerNumberTracker engine.LedgerNumberTracker,
) error {
	sigService, err := engine.NewDefaultSignatureService(opts.NetworkPassphrase, dbConnectionPool, opts.RootSeed, chAccModel, &utils.DefaultPrivateKeyEncrypter{}, opts.RootSeed)
	if err != nil {
		return fmt.Errorf("creating signature service: %w", err)
	}

	numberOfAccountsToCreate := opts.NumChannelAccounts
	for numberOfAccountsToCreate > 0 {
		batchSize := numberOfAccountsToCreate
		if numberOfAccountsToCreate > txSub.MaximumCreateAccountOperationsPerStellarTx {
			// only create a MaxBatchSize (19) of accounts per transaction, this is due to the signature limit of a transaction
			batchSize = txSub.MaximumCreateAccountOperationsPerStellarTx
		}
		log.Ctx(ctx).Infof("batch size: %d", batchSize)

		currLedgerNumber, err := ledgerNumberTracker.GetLedgerNumber()
		if err != nil {
			return fmt.Errorf("cannot get current ledger number: %w", err)
		}
		accounts, err := txSub.CreateChannelAccountsOnChain(
			ctx,
			horizonClient,
			batchSize,
			opts.MaxBaseFee,
			sigService,
			currLedgerNumber,
		)
		if err != nil {
			return err
		}

		// write the channel accounts to the database
		for _, account := range accounts {
			_, err = chAccModel.Unlock(ctx, dbConnectionPool, account)
			if err != nil {
				return fmt.Errorf("cannot unlock account %s", account)
			}
			log.Ctx(ctx).Infof("Created channel account with public key %s", account)
		}
		numberOfAccountsToCreate -= len(accounts)
	}

	return nil
}

// VerifyChannelAccounts verifies the existance of all channel accounts in the data store onchain.
func (s *ChannelAccountsService) VerifyChannelAccounts(ctx context.Context, opts ChannelAccountServiceOptions) error {
	log.Ctx(ctx).Infof("DeleteInvalidAccounts?: %t", opts.DeleteInvalidAcccounts)
	accounts, err := s.caStore.GetAll(ctx, s.dbConnectionPool, 0, 0)
	if err != nil {
		return fmt.Errorf("loading channel accounts from database in VerifyChannelAccounts: %w", err)
	}

	log.Ctx(ctx).Infof("Discovered %d channel accounts in database", len(accounts))

	invalidAccountsCount := 0
	for _, account := range accounts {
		_, err := s.horizonClient.AccountDetail(horizonclient.AccountRequest{AccountID: account.PublicKey})
		if err != nil {
			if horizonclient.IsNotFoundError(err) {
				log.Ctx(ctx).Warnf("Account %s does not exist on the network", account.PublicKey)
				if opts.DeleteInvalidAcccounts {
					deleteErr := s.caStore.Delete(ctx, s.dbConnectionPool, account.PublicKey)
					if deleteErr != nil {
						return fmt.Errorf(
							"deleting %s from database in VerifyChannelAccounts: %w",
							account.PublicKey,
							deleteErr,
						)
					}
					log.Ctx(ctx).Infof("Successfully deleted channel account %q", account.PublicKey)
				}

				invalidAccountsCount++
			} else {
				// return any error other than 404's
				return fmt.Errorf(
					"retrieving account details through horizon for account %s in VerifyChannelAccounts: %w",
					account.PublicKey,
					horizonclient.GetError(err),
				)
			}
		}
	}

	if invalidAccountsCount == 0 {
		log.Ctx(ctx).Info("No invalid channel accounts discovered")
	}

	return nil
}

func (s *ChannelAccountsService) EnsureChannelAccountsCount(
	ctx context.Context,
	opts ChannelAccountServiceOptions,
) error {
	log.Ctx(ctx).Infof("Desired Accounts Count: %d", opts.NumChannelAccounts)

	numAccountsToEnsure := opts.NumChannelAccounts
	if numAccountsToEnsure > txSub.MaxNumberOfChannelAccounts {
		return fmt.Errorf(
			"count entered %d is greater than the channel accounts count limit %d in EnsureChannelAccountsCount",
			numAccountsToEnsure,
			txSub.MaxNumberOfChannelAccounts,
		)
	}

	accountsCount, err := s.caStore.Count(ctx)
	if err != nil {
		return fmt.Errorf("retrieving channel accounts count in EnsureChannelAccountsCount: %w", err)
	}

	if accountsCount == numAccountsToEnsure {
		log.Ctx(ctx).Infof("There are exactly %d managed channel accounts currently. Exiting...", numAccountsToEnsure)
		return nil
	} else if accountsCount > numAccountsToEnsure { // delete some accounts
		numAccountsToDelete := accountsCount - numAccountsToEnsure
		log.Ctx(ctx).Infof("Deleting %d accounts...", numAccountsToDelete)

		err = s.deleteChannelAccounts(ctx, opts, numAccountsToDelete)
		if err != nil {
			return fmt.Errorf("deleting %d accounts in EnsureChannelAccountsCount: %w", numAccountsToDelete, err)
		}
	} else { // add some accounts
		numAccountsToCreate := numAccountsToEnsure - accountsCount
		opts.NumChannelAccounts = numAccountsToCreate
		log.Ctx(ctx).Infof("Creating %d accounts...", numAccountsToCreate)

		createAccErr := createAccountsInBatch(ctx, s.dbConnectionPool, opts, s.horizonClient, s.caStore, s.ledgerNumberTracker)
		if createAccErr != nil {
			return fmt.Errorf("creating channel accounts in batch in EnsureChannelAccountsCount: %w", createAccErr)
		}
	}

	return nil
}

// DeleteChannelAccount removes a specified channel account from the database and onchain.
func (s *ChannelAccountsService) DeleteChannelAccount(
	ctx context.Context,
	opts ChannelAccountServiceOptions,
) error {
	if opts.ChannelAccountID != "" { // delete specified accounts
		currLedgerNum, err := s.ledgerNumberTracker.GetLedgerNumber()
		if err != nil {
			return fmt.Errorf("retrieving current ledger number in DeleteChannelAccount: %w", err)
		}

		lockedUntilLedgerNumber := currLedgerNum + engine.IncrementForMaxLedgerBounds
		channelAccount, err := s.caStore.GetAndLock(ctx, opts.ChannelAccountID, currLedgerNum, lockedUntilLedgerNumber)
		if err != nil {
			return fmt.Errorf(
				"retrieving account %s from database in DeleteChannelAccount: %w", opts.ChannelAccountID, err)
		}

		err = s.deleteChannelAccount(ctx, opts, channelAccount.PublicKey, lockedUntilLedgerNumber)
		if err != nil {
			return fmt.Errorf("deleting account %s in DeleteChannelAccount: %w", channelAccount.PublicKey, err)
		}
	} else if opts.DeleteAllAccounts { // delete all managed accounts
		accountsCount, err := s.caStore.Count(ctx)
		log.Ctx(ctx).Infof("Found %d accounts to delete...", accountsCount)

		if err != nil {
			return fmt.Errorf("cannot get count for accounts in DeleteChannelAccount: %w", err)
		}
		err = s.deleteChannelAccounts(ctx, opts, accountsCount)
		if err != nil {
			return fmt.Errorf("cannot delete all accounts in DeleteChannelAccount: %w", err)
		}
	} else {
		log.Ctx(ctx).Warn("Specify an account to delete or enable deletion of all accounts")
	}

	return nil
}

func (s *ChannelAccountsService) ViewChannelAccounts(ctx context.Context) error {
	accounts, err := s.caStore.GetAll(ctx, s.dbConnectionPool, 0, 0)
	if err != nil {
		return fmt.Errorf("loading channel accounts from database in ViewChannelAccounts: %w", err)
	}

	log.Ctx(ctx).Infof("Discovered %d channel accounts in database...", len(accounts))

	for _, acc := range accounts {
		log.Ctx(ctx).Infof("Found account %s", acc.PublicKey)
	}

	return nil
}

func (s *ChannelAccountsService) deleteChannelAccounts(ctx context.Context, opts ChannelAccountServiceOptions, numAccountsToDelete int) error {
	for i := 0; i < numAccountsToDelete; i++ {
		currLedgerNum, err := s.ledgerNumberTracker.GetLedgerNumber()
		if err != nil {
			return fmt.Errorf("retrieving current ledger number in DeleteChannelAccount: %w", err)
		}

		lockedUntilLedgerNumber := currLedgerNum + engine.IncrementForMaxLedgerBounds
		accounts, err := s.caStore.GetAndLockAll(ctx, currLedgerNum, lockedUntilLedgerNumber, 1)
		if err != nil {
			return fmt.Errorf("cannot retrieve free channel account: %w", err)
		}

		if len(accounts) == 0 {
			log.Ctx(ctx).Warn("Could not find any accounts to deleting. Exiting...")
			return nil
		}

		accountToDelete := accounts[0]
		err = s.deleteChannelAccount(ctx, opts, accountToDelete.PublicKey, lockedUntilLedgerNumber)
		if err != nil {
			return fmt.Errorf("cannot delete account %s: %w", accountToDelete.PublicKey, err)
		}
	}

	return nil
}

func (s *ChannelAccountsService) deleteChannelAccount(
	ctx context.Context,
	opts ChannelAccountServiceOptions,
	chAccAddress string,
	lockedUntilLedger int,
) error {
	sigService, err := engine.NewDefaultSignatureService(opts.NetworkPassphrase, s.dbConnectionPool, opts.RootSeed, s.caStore, &utils.DefaultPrivateKeyEncrypter{}, opts.RootSeed)
	if err != nil {
		return fmt.Errorf("creating signature service: %w", err)
	}

	_, err = s.horizonClient.AccountDetail(horizonclient.AccountRequest{AccountID: chAccAddress})
	if err != nil {
		if horizonclient.IsNotFoundError(err) {
			log.Ctx(ctx).Warnf("Account %s does not exist on the network", chAccAddress)
			err = sigService.Delete(ctx, chAccAddress, lockedUntilLedger)
			if err != nil {
				return fmt.Errorf("deleting %s from signature service: %w", chAccAddress, err)
			}
		} else {
			return fmt.Errorf("cannot find account %s on the network: %w", chAccAddress, err)
		}
	} else {
		err = txSub.DeleteChannelAccountOnChain(
			ctx,
			s.horizonClient,
			chAccAddress,
			int64(opts.MaxBaseFee),
			sigService,
			lockedUntilLedger,
		)
		if err != nil {
			return fmt.Errorf("deleting account %s onchain: %w", opts.ChannelAccountID, err)
		}
	}

	log.Ctx(ctx).Infof("Successfully deleted channel account %q", chAccAddress)

	return nil
}

func acquireAdvisoryLockForCommand(ctx context.Context, dbConnectionPool db.DBConnectionPool) error {
	locked, err := utils.AcquireAdvisoryLock(ctx, dbConnectionPool, advisoryLock)
	if err != nil {
		return fmt.Errorf("problem retrieving db advisory lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("cannot retrieve unavailable db advisory lock")
	}

	return nil
}
