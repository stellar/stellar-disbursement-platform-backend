package services

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

var ErrInvalidNumOfChannelAccountsToCreate = errors.New("invalid number of channel accounts to create")

// MaximumCreateAccountOperationsPerStellarTx is the max number of sponsored accounts we can create in one transaction
// due to the signature limit.
const MaximumCreateAccountOperationsPerStellarTx = 19

// MaxNumberOfChannelAccounts is the limit for the number of accounts tx submission service should manage.
const MaxNumberOfChannelAccounts = 1000

// MinNumberOfChannelAccounts is the minimum number of accounts tx submission service should manage.
const MinNumberOfChannelAccounts = 1

// DefaultRevokeSponsorshipReserveAmount is the amount of the native asset that the sponsoring account will send
// to the sponsored account to cover the reserve that is needed to for revoking account sponsorship.
// The amount will be send back to the sponsoring account once the sponsored account is deleted onchain.
const DefaultRevokeSponsorshipReserveAmount = "1.5"

// MinTenantDistributionAccountAmount is the minimum amount of the native asset that the host distribution account is allowed to
// send to the tenant distribution account at a time. It is also used as the default amount to bootstrap a tenant distribution account,
// when non is specified.
const MinTenantDistributionAccountAmount = 5

// MaxTenantDistributionAccountAmount is the maximum amount of the native asset that the host distribution account is allowed to
// send to the tenant distribution account at a time.
const MaxTenantDistributionAccountAmount = 50

// CreateChannelAccountsOnChain will create up to 19 accounts per Transaction due to the 20 signatures per tx limit This
// is also a good opportunity to periodically write the generated accounts to persistent storage if generating large
// amounts of channel accounts.
func CreateChannelAccountsOnChain(ctx context.Context, submiterEngine engine.SubmitterEngine, numOfChanAccToCreate int) (newAccountAddresses []string, err error) {
	defer func() {
		// If we failed to create the accounts, we should delete the accounts that were added to the signature service.
		if err != nil {
			cloneOfNewAccountAddresses := slices.Clone(newAccountAddresses)
			for _, accountAddress := range cloneOfNewAccountAddresses {
				if accountAddress == submiterEngine.HostDistributionAccount() {
					continue
				}
				deleteErr := submiterEngine.ChAccountSigner.Delete(ctx, accountAddress)
				if deleteErr != nil {
					log.Ctx(ctx).Errorf("failed to delete channel account %s: %v", accountAddress, deleteErr)
				}
			}
			newAccountAddresses = nil
		}
	}()

	if numOfChanAccToCreate > MaximumCreateAccountOperationsPerStellarTx {
		return nil, fmt.Errorf("cannot create more than %d channel accounts", MaximumCreateAccountOperationsPerStellarTx)
	}

	if numOfChanAccToCreate <= 0 {
		return nil, ErrInvalidNumOfChannelAccountsToCreate
	}

	rootAccount, err := submiterEngine.HorizonClient.AccountDetail(horizonclient.AccountRequest{
		AccountID: submiterEngine.HostDistributionAccount(),
	})
	if err != nil {
		err = utils.NewHorizonErrorWrapper(err)
		return nil, fmt.Errorf("failed to retrieve root account: %w", err)
	}

	var sponsoredCreateAccountOps []txnbuild.Operation

	ledgerBounds, err := submiterEngine.LedgerNumberTracker.GetLedgerBounds()
	if err != nil {
		return nil, fmt.Errorf("failed to get ledger bounds: %w", err)
	}

	publicKeys, err := submiterEngine.ChAccountSigner.BatchInsert(ctx, numOfChanAccToCreate)
	if err != nil {
		return nil, fmt.Errorf("failed to insert channel accounts into signature service: %w", err)
	}

	// Prepare Stellar operations to create the sponsored channel accounts
	for _, publicKey := range publicKeys {
		// generate random keypair for this channel account
		log.Ctx(ctx).Infof("⏳ Creating sponsored Stellar account with address: %s", publicKey)

		sponsoredCreateAccountOps = append(
			sponsoredCreateAccountOps,

			// add sponsor operations for this account
			&txnbuild.BeginSponsoringFutureReserves{
				SponsoredID: publicKey,
			},
			&txnbuild.CreateAccount{
				Destination: publicKey,
				Amount:      "0",
			},
			&txnbuild.EndSponsoringFutureReserves{
				SourceAccount: publicKey,
			},
		)

		// append this channel account to the list of signers
		newAccountAddresses = append(newAccountAddresses, publicKey)
	}

	// create a new transaction with the account creation/sponsorship operations
	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount:        &rootAccount,
		IncrementSequenceNum: true,
		Operations:           sponsoredCreateAccountOps,
		BaseFee:              int64(submiterEngine.MaxBaseFee),
		Preconditions: txnbuild.Preconditions{
			TimeBounds:   txnbuild.NewTimeout(15),
			LedgerBounds: ledgerBounds,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating transaction for channel account creation: %w", err)
	}

	// sign the transaction
	// Channel account signing:
	tx, err = submiterEngine.ChAccountSigner.SignStellarTransaction(ctx, tx, newAccountAddresses...)
	if err != nil {
		return newAccountAddresses, fmt.Errorf("signing account creation transaction for channel accounts %v: %w", newAccountAddresses, err)
	}
	// Host distribution account signing:
	tx, err = submiterEngine.HostAccountSigner.SignStellarTransaction(ctx, tx, submiterEngine.HostDistributionAccount())
	if err != nil {
		return newAccountAddresses, fmt.Errorf("signing account creation transaction for host distribution account %s: %w", submiterEngine.HostDistributionAccount(), err)
	}

	_, err = submiterEngine.HorizonClient.SubmitTransactionWithOptions(tx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true})
	if err != nil {
		hError := utils.NewHorizonErrorWrapper(err)
		return newAccountAddresses, fmt.Errorf("creating sponsored channel accounts: %w", hError)
	}
	log.Ctx(ctx).Infof("🎉 Successfully created %d sponsored channel accounts", len(newAccountAddresses))

	return newAccountAddresses, nil
}

// DeleteChannelAccountOnChain creates, signs, and broadcasts a transaction to delete a channel account onchain.
func DeleteChannelAccountOnChain(ctx context.Context, submiterEngine engine.SubmitterEngine, chAccAddress string) error {
	distributionAccount := submiterEngine.HostDistributionAccount()
	rootAccount, err := submiterEngine.HorizonClient.AccountDetail(horizonclient.AccountRequest{
		AccountID: distributionAccount,
	})
	if err != nil {
		return fmt.Errorf("retrieving root account from distribution seed: %w", err)
	}

	ledgerBounds, err := submiterEngine.LedgerNumberTracker.GetLedgerBounds()
	if err != nil {
		return fmt.Errorf("failed to get ledger bounds: %w", err)
	}

	// TODO: Currently, this transaction deletes a single sponsored account onchain, we may want to
	// attempt to delete more accounts per tx in the future up to the limit of operations and
	// signatures a single tx will allow
	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount:        &rootAccount,
		IncrementSequenceNum: true,
		Operations: []txnbuild.Operation{
			&txnbuild.Payment{
				SourceAccount: rootAccount.AccountID,
				Destination:   chAccAddress,
				Amount:        DefaultRevokeSponsorshipReserveAmount,
				Asset:         txnbuild.NativeAsset{},
			},
			&txnbuild.RevokeSponsorship{
				SponsorshipType: txnbuild.RevokeSponsorshipTypeAccount,
				Account:         &chAccAddress,
			},
			&txnbuild.AccountMerge{
				Destination:   distributionAccount,
				SourceAccount: chAccAddress,
			},
		},
		BaseFee: int64(submiterEngine.MaxBaseFee),
		Preconditions: txnbuild.Preconditions{
			LedgerBounds: ledgerBounds,
			TimeBounds:   txnbuild.NewTimeout(15),
		},
	})
	if err != nil {
		return fmt.Errorf(
			"constructing remove channel account transaction for account %s: %w",
			chAccAddress,
			err,
		)
	}

	// the root account authorizes the sponsorship revocation, while the channel account authorizes
	// merging into the distribution account.
	// Channel account signing:
	tx, err = submiterEngine.ChAccountSigner.SignStellarTransaction(ctx, tx, chAccAddress)
	if err != nil {
		return fmt.Errorf("signing remove account transaction for account %s: %w", chAccAddress, err)
	}
	// Host distribution account signing:
	tx, err = submiterEngine.HostAccountSigner.SignStellarTransaction(ctx, tx, submiterEngine.HostDistributionAccount())
	if err != nil {
		return fmt.Errorf("signing remove account transaction for host distribution account %s: %w", submiterEngine.HostDistributionAccount(), err)
	}

	_, err = submiterEngine.HorizonClient.SubmitTransactionWithOptions(tx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true})
	if err != nil {
		hError := utils.NewHorizonErrorWrapper(err)
		return fmt.Errorf("submitting remove account transaction to the network for account %s: %w", chAccAddress, hError)
	}

	err = submiterEngine.ChAccountSigner.Delete(ctx, chAccAddress)
	if err != nil {
		return fmt.Errorf("deleting channel account %s from the store: %w", chAccAddress, err)
	}

	return nil
}

func CreateAndFundDistributionAccount(ctx context.Context, submitterEngine engine.SubmitterEngine, tenantID string, amountNativeAssetToSend int) error {
	if amountNativeAssetToSend < MinTenantDistributionAccountAmount || amountNativeAssetToSend > MaxTenantDistributionAccountAmount {
		if amountNativeAssetToSend <= 0 {
			return fmt.Errorf("invalid amount of native asset to send: %d", amountNativeAssetToSend)
		}

		return fmt.Errorf("amount of native asset to send must be between %d and %d", MinTenantDistributionAccountAmount, MaxTenantDistributionAccountAmount)
	}

	tenantDistributionAcc, err := submitterEngine.DistributionAccount(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("retrieving distribution account for tenant %s: %w", tenantID, err)
	}

	hostDistributionAcc := submitterEngine.HostDistributionAccount()
	if tenantDistributionAcc == hostDistributionAcc {
		log.Ctx(ctx).Info("Host distribution account and tenant distribution account are the same, no need to initiate funding.")
		return nil
	}

	hostAccount, err := getAccountDetails(submitterEngine.HorizonClient, hostDistributionAcc)
	if err != nil {
		return fmt.Errorf("getting details for host distribution account: %w", err)
	}

	tx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        hostAccount,
			IncrementSequenceNum: true,
			BaseFee:              txnbuild.MinBaseFee,
			Preconditions: txnbuild.Preconditions{
				TimeBounds: txnbuild.NewInfiniteTimeout(), // 30 seconds
			},
			Operations: []txnbuild.Operation{
				&txnbuild.CreateAccount{
					Destination: tenantDistributionAcc,
					Amount:      strconv.Itoa(amountNativeAssetToSend),
				},
			},
		},
	)
	if err != nil {
		return fmt.Errorf(
			"creating raw payment tx from %s to %s: %w",
			hostAccount.AccountID,
			tenantDistributionAcc,
			err,
		)
	}

	// Host distribution account signing:
	tx, err = submitterEngine.HostAccountSigner.SignStellarTransaction(ctx, tx, hostDistributionAcc)
	if err != nil {
		return fmt.Errorf(
			"signing payment tx from %s to %s: %w",
			hostAccount.AccountID,
			tenantDistributionAcc,
			err,
		)
	}

	_, err = submitterEngine.HorizonClient.SubmitTransactionWithOptions(tx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true})
	if err != nil {
		hError := utils.NewHorizonErrorWrapper(err)
		return fmt.Errorf("submitting payment tx from %s to %s to the Stellar network: %w", hostAccount.AccountID,
			tenantDistributionAcc, hError)
	}

	_, err = getAccountDetails(submitterEngine.HorizonClient, tenantDistributionAcc)
	if err != nil {
		return fmt.Errorf("getting details for tenant distribution account: %w", err)
	}

	return nil
}

func getAccountDetails(client horizonclient.ClientInterface, accountID string) (*horizon.Account, error) {
	account, err := client.AccountDetail(horizonclient.AccountRequest{
		AccountID: accountID,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot find account on the network %s: %w", accountID, err)
	}

	return &account, nil
}
