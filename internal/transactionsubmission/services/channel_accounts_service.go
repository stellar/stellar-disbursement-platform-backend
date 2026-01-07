package services

import (
	"context"
	"fmt"
	"slices"

	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

const advisoryLockID = int(2172398390434160)

func acquireAdvisoryLockForCommand(ctx context.Context, dbConnectionPool db.DBConnectionPool) error {
	locked, err := utils.AcquireAdvisoryLock(ctx, dbConnectionPool, advisoryLockID)
	if err != nil {
		return fmt.Errorf("problem retrieving db advisory lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("advisory lock is unavailable")
	}

	return nil
}

// ViewChannelAccounts prints all the database channel accounts.
func ViewChannelAccounts(ctx context.Context, dbConnectionPool db.DBConnectionPool) error {
	if dbConnectionPool == nil {
		return fmt.Errorf("db connection pool cannot be nil")
	}
	chAccModel := store.NewChannelAccountModel(dbConnectionPool)
	accounts, err := chAccModel.GetAll(ctx, dbConnectionPool, 0, 0)
	if err != nil {
		return fmt.Errorf("loading channel accounts from database in ViewChannelAccounts: %w", err)
	}

	log.Ctx(ctx).Infof("Discovered %d channel accounts in database...", len(accounts))
	for _, acc := range accounts {
		log.Ctx(ctx).Infof("Found account %s", acc.PublicKey)
	}

	return nil
}

type ChannelAccountsService struct {
	engine.SubmitterEngine

	TSSDBConnectionPool db.DBConnectionPool
	chAccStore          store.ChannelAccountStore
}

// validate initializes the ChannelAccountsManagementService, preparing it to access the DB and the Stellar network.
func (s *ChannelAccountsService) validate() error {
	if s.TSSDBConnectionPool == nil {
		return fmt.Errorf("tss db connection pool cannot be nil")
	}

	if s.SubmitterEngine == (engine.SubmitterEngine{}) {
		return fmt.Errorf("submitter engine cannot be empty")
	}
	if err := s.SubmitterEngine.Validate(); err != nil {
		return fmt.Errorf("validating submitter engine: %w", err)
	}

	if !slices.Contains(s.SubmitterEngine.SignerRouter.SupportedAccountTypes(), schema.HostStellarEnv) {
		return fmt.Errorf("signature engine's host signer cannot be nil")
	}

	err := acquireAdvisoryLockForCommand(context.Background(), s.TSSDBConnectionPool)
	if err != nil {
		return fmt.Errorf("failed getting db advisory lock: %w", err)
	}

	return nil
}

func (s *ChannelAccountsService) GetChannelAccountStore() store.ChannelAccountStore {
	if s.chAccStore == nil {
		s.chAccStore = store.NewChannelAccountModel(s.TSSDBConnectionPool)
	}
	return s.chAccStore
}

// CreateChannelAccounts creates the specified number of channel accounts on the network and stores them in the database.
func (s *ChannelAccountsService) CreateChannelAccounts(ctx context.Context, amount int) error {
	if err := s.validate(); err != nil {
		return fmt.Errorf("initializing channel account service: %w", err)
	}

	for amount > 0 {
		batchSize := amount
		if amount > MaximumCreateAccountOperationsPerStellarTx {
			// only create a MaxBatchSize (19) of accounts per transaction, this is due to the signature limit of a transaction
			batchSize = MaximumCreateAccountOperationsPerStellarTx
		}
		log.Ctx(ctx).Debugf("batch size: %d", batchSize)

		accounts, err := CreateChannelAccountsOnChain(ctx, s.SubmitterEngine, batchSize)
		if err != nil {
			return fmt.Errorf("creating channel accounts onchain: %w", err)
		}

		// Unlock ready-to-use channel accounts
		for _, account := range accounts {
			_, err = s.GetChannelAccountStore().Unlock(ctx, s.TSSDBConnectionPool, account)
			if err != nil {
				return fmt.Errorf("unlocking account %s", account)
			}
			log.Ctx(ctx).Infof("✅ Channel account with public key '%s' is ready to be used", account)
		}
		amount -= len(accounts)
	}

	return nil
}

type DeleteChannelAccountsOptions struct {
	ChannelAccountID  string
	DeleteAllAccounts bool
}

// DeleteChannelAccount removes the specofied channel accounts rom the database and the network.
func (s *ChannelAccountsService) DeleteChannelAccount(ctx context.Context, opts DeleteChannelAccountsOptions) error {
	if err := s.validate(); err != nil {
		return fmt.Errorf("initializing channel account service: %w", err)
	}

	if opts.ChannelAccountID != "" { // delete specified accounts
		currLedgerNum, err := s.LedgerNumberTracker.GetLedgerNumber()
		if err != nil {
			return fmt.Errorf("retrieving current ledger number in DeleteChannelAccount: %w", err)
		}

		lockedUntilLedgerNumber := currLedgerNum + preconditions.IncrementForMaxLedgerBounds
		channelAccount, err := s.GetChannelAccountStore().GetAndLock(ctx, opts.ChannelAccountID, currLedgerNum, lockedUntilLedgerNumber)
		if err != nil {
			return fmt.Errorf(
				"retrieving account %s from database in DeleteChannelAccount: %w", opts.ChannelAccountID, err)
		}

		err = s.deleteChannelAccount(ctx, channelAccount.PublicKey)
		if err != nil {
			return fmt.Errorf("deleting account %s in DeleteChannelAccount: %w", channelAccount.PublicKey, err)
		}
	} else if opts.DeleteAllAccounts { // delete all managed accounts
		accountsCount, err := s.GetChannelAccountStore().Count(ctx)
		log.Ctx(ctx).Infof("Found %d accounts to delete...", accountsCount)

		if err != nil {
			return fmt.Errorf("cannot get count for accounts in DeleteChannelAccount: %w", err)
		}
		err = s.deleteChannelAccounts(ctx, accountsCount)
		if err != nil {
			return fmt.Errorf("cannot delete all accounts in DeleteChannelAccount: %w", err)
		}
	} else {
		log.Ctx(ctx).Warn("Specify an account to delete or enable deletion of all accounts")
	}

	return nil
}

func (s *ChannelAccountsService) deleteChannelAccounts(ctx context.Context, numAccountsToDelete int) error {
	if err := s.validate(); err != nil {
		return fmt.Errorf("initializing channel account service: %w", err)
	}

	for i := 0; i < numAccountsToDelete; i++ {
		currLedgerNum, err := s.LedgerNumberTracker.GetLedgerNumber()
		if err != nil {
			return fmt.Errorf("retrieving current ledger number in DeleteChannelAccount: %w", err)
		}

		lockedUntilLedgerNumber := currLedgerNum + preconditions.IncrementForMaxLedgerBounds
		accounts, err := s.GetChannelAccountStore().GetAndLockAll(ctx, currLedgerNum, lockedUntilLedgerNumber, 1)
		if err != nil {
			return fmt.Errorf("cannot retrieve free channel account: %w", err)
		}

		if len(accounts) == 0 {
			log.Ctx(ctx).Warn("Could not find any accounts to delete. Exiting...")
			return nil
		}

		accountToDelete := accounts[0]
		err = s.deleteChannelAccount(ctx, accountToDelete.PublicKey)
		if err != nil {
			return fmt.Errorf("cannot delete account %s: %w", accountToDelete.PublicKey, err)
		}
	}

	return nil
}

func (s *ChannelAccountsService) deleteChannelAccount(ctx context.Context, publicKey string) error {
	if _, err := s.HorizonClient.AccountDetail(horizonclient.AccountRequest{AccountID: publicKey}); err != nil {
		if !horizonclient.IsNotFoundError(err) {
			return fmt.Errorf("failed to reach account %s on the network: %w", publicKey, err)
		}

		log.Ctx(ctx).Warnf("Account %s does not exist on the network", publicKey)
		chAccToDelete := schema.NewDefaultChannelAccount(publicKey)
		err = s.SignatureService.SignerRouter.Delete(ctx, chAccToDelete)
		if err != nil {
			return fmt.Errorf("deleting %s from signature service: %w", publicKey, err)
		}
	} else {
		log.Ctx(ctx).Infof("⏳ Deleting Stellar account with address: %s", publicKey)
		err = DeleteChannelAccountOnChain(ctx, s.SubmitterEngine, publicKey)
		if err != nil {
			return fmt.Errorf("deleting account %s onchain: %w", publicKey, err)
		}
	}

	log.Ctx(ctx).Infof("🎉 Successfully deleted channel account %s", publicKey)

	return nil
}

// EnsureChannelAccountsCount ensures that the number of channel accounts in the database is equal to the number
// specified in the parameter.
func (s *ChannelAccountsService) EnsureChannelAccountsCount(ctx context.Context, numAccountsToEnsure int) error {
	if err := s.validate(); err != nil {
		return fmt.Errorf("initializing channel account service: %w", err)
	}

	log.Ctx(ctx).Infof("⚙️ Desired Accounts Count: %d", numAccountsToEnsure)

	if numAccountsToEnsure > MaxNumberOfChannelAccounts {
		return fmt.Errorf("count entered %d is greater than the channel accounts count limit %d in EnsureChannelAccountsCount", numAccountsToEnsure, MaxNumberOfChannelAccounts)
	}

	accountsCount, err := s.GetChannelAccountStore().Count(ctx)
	if err != nil {
		return fmt.Errorf("retrieving channel accounts count in EnsureChannelAccountsCount: %w", err)
	}

	if accountsCount == numAccountsToEnsure {
		log.Ctx(ctx).Infof("✅ There are exactly %d managed channel accounts currently. Exiting...", numAccountsToEnsure)
		return nil
	} else if accountsCount > numAccountsToEnsure { // delete some accounts
		numAccountsToDelete := accountsCount - numAccountsToEnsure
		log.Ctx(ctx).Infof("⏳ Deleting %d accounts...", numAccountsToDelete)

		err = s.deleteChannelAccounts(ctx, numAccountsToDelete)
		if err != nil {
			return fmt.Errorf("deleting %d accounts in EnsureChannelAccountsCount: %w", numAccountsToDelete, err)
		}
	} else { // add some accounts
		numAccountsToCreate := numAccountsToEnsure - accountsCount
		log.Ctx(ctx).Infof("⏳ Creating %d accounts...", numAccountsToCreate)

		createAccErr := s.CreateChannelAccounts(ctx, numAccountsToCreate)
		if createAccErr != nil {
			return fmt.Errorf("creating channel accounts in batch in EnsureChannelAccountsCount: %w", createAccErr)
		}
	}

	return nil
}

// VerifyChannelAccounts verifies that all the database channel accounts exist on the network. If the
// deleteInvalidAccounts flag is set to true, it will delete these invalid accounts from the database.
func (s *ChannelAccountsService) VerifyChannelAccounts(ctx context.Context, deleteInvalidAccounts bool) error {
	if err := s.validate(); err != nil {
		return fmt.Errorf("initializing channel account service: %w", err)
	}

	log.Ctx(ctx).Infof("DeleteInvalidAccounts?: %t", deleteInvalidAccounts)
	accounts, err := s.GetChannelAccountStore().GetAll(ctx, s.TSSDBConnectionPool, 0, 0)
	if err != nil {
		return fmt.Errorf("loading channel accounts from database in VerifyChannelAccounts: %w", err)
	}

	log.Ctx(ctx).Infof("Discovered %d channel accounts in database", len(accounts))

	invalidAccountsCount := 0
	for _, account := range accounts {
		_, err := s.HorizonClient.AccountDetail(horizonclient.AccountRequest{AccountID: account.PublicKey})
		if err == nil {
			continue
		}

		if !horizonclient.IsNotFoundError(err) {
			// return any error other than 404's
			return fmt.Errorf("retrieving account details through horizon for account %s in VerifyChannelAccounts: %w", account.PublicKey, horizonclient.GetError(err))
		}

		warnMessage := fmt.Sprintf("Account %s does not exist on the network", account.PublicKey)
		if !deleteInvalidAccounts {
			warnMessage += ". Use the '--delete-invalid-accounts' flag to erase it from the database"
		}
		log.Ctx(ctx).Warn(warnMessage)

		if deleteInvalidAccounts {
			if err = s.GetChannelAccountStore().Delete(ctx, s.TSSDBConnectionPool, account.PublicKey); err != nil {
				return fmt.Errorf("deleting %s from database in VerifyChannelAccounts: %w", account.PublicKey, err)
			}
			log.Ctx(ctx).Infof("✅ Successfully deleted channel account %q", account.PublicKey)
		}

		invalidAccountsCount++
	}

	if invalidAccountsCount == 0 {
		log.Ctx(ctx).Info("No invalid channel accounts discovered")
	}

	return nil
}
