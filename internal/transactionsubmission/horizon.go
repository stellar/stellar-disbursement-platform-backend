package transactionsubmission

import (
	"context"
	"errors"
	"fmt"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/txnbuild"
	"golang.org/x/exp/slices"

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

// CreateChannelAccountsOnChain will create up to 19 accounts per Transaction due to the 20 signatures per tx limit This
// is also a good opportunity to periodically write the generated accounts to persistent storage if generating large
// amounts of channel accounts.
func CreateChannelAccountsOnChain(ctx context.Context, horizonClient horizonclient.ClientInterface, numOfChanAccToCreate int, maxBaseFee int, sigService engine.SignatureService, currLedgerNumber int) (newAccountAddresses []string, err error) {
	defer func() {
		// If we failed to create the accounts, we should delete the accounts that were added to the signature service.
		if err != nil && sigService != nil {
			cloneOfNewAccountAddresses := slices.Clone(newAccountAddresses)
			for _, accountAddress := range cloneOfNewAccountAddresses {
				if accountAddress == sigService.DistributionAccount() {
					continue
				}
				deleteErr := sigService.Delete(ctx, accountAddress, currLedgerNumber+engine.IncrementForMaxLedgerBounds)
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

	rootAccount, err := horizonClient.AccountDetail(horizonclient.AccountRequest{
		AccountID: sigService.DistributionAccount(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve root account: %w", err)
	}

	var sponsoredCreateAccountOps []txnbuild.Operation

	kpsToCreate := []*keypair.Full{}

	// Prepare Stellar operations to create the sponsored channel accounts
	for i := 0; i < numOfChanAccToCreate; i++ {
		// generate random keypair for this channel account
		var channelAccountKP *keypair.Full
		channelAccountKP, err = keypair.Random()
		if err != nil {
			return nil, fmt.Errorf("failed to generate keypair: %w", err)
		}
		log.Ctx(ctx).Infof("creating sponsored stellar account with address: %s", channelAccountKP.Address())

		sponsoredCreateAccountOps = append(
			sponsoredCreateAccountOps,

			// add sponsor operations for this account
			&txnbuild.BeginSponsoringFutureReserves{
				SponsoredID: channelAccountKP.Address(),
			},
			&txnbuild.CreateAccount{
				Destination: channelAccountKP.Address(),
				Amount:      "0",
			},
			&txnbuild.EndSponsoringFutureReserves{
				SourceAccount: channelAccountKP.Address(),
			},
		)

		// append this channel account to the list of signers
		kpsToCreate = append(kpsToCreate, channelAccountKP)
		newAccountAddresses = append(newAccountAddresses, channelAccountKP.Address())
	}

	err = sigService.BatchInsert(ctx, kpsToCreate, true, currLedgerNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to insert channel accounts into signature service: %w", err)
	}

	// create a new transaction with the account creation/sponsorship operations
	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount:        &rootAccount,
		IncrementSequenceNum: true,
		Operations:           sponsoredCreateAccountOps,
		BaseFee:              int64(maxBaseFee),
		Preconditions: txnbuild.Preconditions{
			TimeBounds: txnbuild.NewTimeout(15),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating transaction for channel account creation: %w", err)
	}

	// sign the transaction
	signers := append([]string{sigService.DistributionAccount()}, newAccountAddresses...)
	tx, err = sigService.SignStellarTransaction(ctx, tx, signers...)
	if err != nil {
		return newAccountAddresses, fmt.Errorf("signing transaction: %w", err)
	}

	_, err = horizonClient.SubmitTransactionWithOptions(tx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true})
	if err != nil {
		hError := utils.NewHorizonErrorWrapper(err)
		return newAccountAddresses, fmt.Errorf("creating sponsored channel accounts: %w", hError)
	}
	log.Ctx(ctx).Infof("🎉 Successfully created %d sponsored channel accounts", len(newAccountAddresses))

	return newAccountAddresses, nil
}

// DeleteChannelAccountOnChain creates, signs, and broadcasts a transaction to delete a channel account onchain.
func DeleteChannelAccountOnChain(
	ctx context.Context,
	horizonClient horizonclient.ClientInterface,
	chAccAddress string,
	maxBaseFee int64,
	sigService engine.SignatureService,
	lockedUntilLedgerNumber int,
) error {
	distributionAccount := sigService.DistributionAccount()
	rootAccount, err := horizonClient.AccountDetail(horizonclient.AccountRequest{
		AccountID: distributionAccount,
	})
	if err != nil {
		return fmt.Errorf("retrieving root account from distribution seed: %w", err)
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
		BaseFee: maxBaseFee,
		Preconditions: txnbuild.Preconditions{
			TimeBounds: txnbuild.NewTimeout(15),
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
	// merging into the distribution account
	tx, err = sigService.SignStellarTransaction(ctx, tx, sigService.DistributionAccount(), chAccAddress)
	if err != nil {
		return fmt.Errorf("signing remove account transaction for account %s: %w", chAccAddress, err)
	}

	_, err = horizonClient.SubmitTransactionWithOptions(tx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true})
	if err != nil {
		hError := utils.NewHorizonErrorWrapper(err)
		return fmt.Errorf("submitting remove account transaction to the network for account %s: %w", chAccAddress, hError)
	}

	err = sigService.Delete(ctx, chAccAddress, lockedUntilLedgerNumber)
	if err != nil {
		return fmt.Errorf("deleting channel account %s from the store: %w", chAccAddress, err)
	}

	return nil
}
