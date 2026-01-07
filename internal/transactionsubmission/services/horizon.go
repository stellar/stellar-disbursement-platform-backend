package services

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/protocols/horizon"
	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stellar/go-stellar-sdk/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

var ErrInvalidNumOfChannelAccountsToCreate = errors.New("invalid number of channel accounts to create")

// MaximumCreateAccountOperationsPerStellarTx is the max number of sponsored accounts we can create in one transaction
// due to the signature limit.
const MaximumCreateAccountOperationsPerStellarTx = 19

// MaxNumberOfChannelAccounts is the limit for the number of accounts tx submission service should manage.
const MaxNumberOfChannelAccounts = 1000

// MinNumberOfChannelAccounts is the minimum number of accounts tx submission service should manage.
const MinNumberOfChannelAccounts = 1

// CreateAndFundAccountRetryAttempts is the maximum number of attempts to create and fund an account on the Stellar network.
const CreateAndFundAccountRetryAttempts = 5

// DefaultRevokeSponsorshipReserveAmount is the amount of the native asset that the sponsoring account will send
// to the sponsored account to cover the reserve that is needed to for revoking account sponsorship.
// The amount will be sent back to the sponsoring account once the sponsored account is deleted onchain.
const DefaultRevokeSponsorshipReserveAmount = "1.5"

// CreateChannelAccountsOnChain will create up to 19 accounts per Transaction due to the 20 signatures per tx limit This
// is also a good opportunity to periodically write the generated accounts to persistent storage if generating large
// amounts of channel accounts.
func CreateChannelAccountsOnChain(ctx context.Context, submiterEngine engine.SubmitterEngine, numOfChanAccToCreate int) (newAccountAddresses []string, err error) {
	hostAccount := submiterEngine.HostDistributionAccount()
	defer func() {
		// If we failed to create the accounts, we should delete the accounts that were added to the signature service.
		if err != nil {
			cloneOfNewAccountAddresses := slices.Clone(newAccountAddresses)
			for _, accountAddress := range cloneOfNewAccountAddresses {
				if accountAddress == hostAccount.Address {
					continue
				}
				chAccToDelete := schema.NewDefaultChannelAccount(accountAddress)
				deleteErr := submiterEngine.SignerRouter.Delete(ctx, chAccToDelete)
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
		AccountID: hostAccount.Address,
	})
	if err != nil {
		err = utils.NewHorizonErrorWrapper(err)
		return nil, fmt.Errorf("failed to retrieve host account: %w", err)
	}

	var sponsoredCreateAccountOps []txnbuild.Operation

	ledgerBounds, err := submiterEngine.LedgerNumberTracker.GetLedgerBounds()
	if err != nil {
		return nil, fmt.Errorf("failed to get ledger bounds: %w", err)
	}

	stellarAccounts, err := submiterEngine.BatchInsert(ctx, schema.ChannelAccountStellarDB, numOfChanAccToCreate)
	if err != nil {
		return nil, fmt.Errorf("failed to insert channel accounts into signature service: %w", err)
	}

	// Prepare Stellar operations to create the sponsored channel accounts
	for _, stellarAccount := range stellarAccounts {
		publicKey := stellarAccount.Address
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
	tx, err = submiterEngine.SignerRouter.SignStellarTransaction(ctx, tx, append(stellarAccounts, hostAccount)...)
	if err != nil {
		return newAccountAddresses, fmt.Errorf("signing account creation transaction with accounts %v: %w", newAccountAddresses, err)
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
	hostAccount := submiterEngine.HostDistributionAccount()
	rootAccount, err := submiterEngine.HorizonClient.AccountDetail(horizonclient.AccountRequest{
		AccountID: hostAccount.Address,
	})
	if err != nil {
		return fmt.Errorf("retrieving host account from distribution seed: %w", err)
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
				Destination:   hostAccount.Address,
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
	chAccToDelete := schema.NewDefaultChannelAccount(chAccAddress)
	tx, err = submiterEngine.SignerRouter.SignStellarTransaction(ctx, tx, chAccToDelete, hostAccount)
	if err != nil {
		return fmt.Errorf("signing remove account transaction for account %s: %w", chAccAddress, err)
	}

	_, err = submiterEngine.HorizonClient.SubmitTransactionWithOptions(tx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true})
	if err != nil {
		hError := utils.NewHorizonErrorWrapper(err)
		return fmt.Errorf("submitting remove account transaction to the network for account %s: %w", chAccAddress, hError)
	}

	err = submiterEngine.Delete(ctx, chAccToDelete)
	if err != nil {
		return fmt.Errorf("deleting channel account %s from the store: %w", chAccAddress, err)
	}

	return nil
}

// AddTrustlines ensures the provided account trusts all supplied assets and returns how many trustlines were added.
func AddTrustlines(
	ctx context.Context,
	submitterEngine engine.SubmitterEngine,
	account schema.TransactionAccount,
	assets []data.Asset,
) (int, error) {
	if account.Address == "" {
		return 0, fmt.Errorf("transaction account address cannot be empty")
	}

	assetCandidates := make(map[string]data.Asset)
	for _, asset := range assets {
		if asset.IsNative() {
			continue
		}
		assetCandidates[getAssetID(asset.Code, asset.Issuer)] = asset
	}

	if len(assetCandidates) == 0 {
		return 0, nil
	}

	accountDetails, err := submitterEngine.HorizonClient.AccountDetail(horizonclient.AccountRequest{AccountID: account.Address})
	if err != nil {
		return 0, fmt.Errorf("getting account details for %s: %w", account.Address, err)
	}

	existingTrustlines := make(map[string]struct{})
	for _, balance := range accountDetails.Balances {
		if balance.Asset.Type == "native" {
			continue
		}
		existingTrustlines[getAssetID(balance.Asset.Code, balance.Asset.Issuer)] = struct{}{}
	}

	// sort keys to keep deterministic operation ordering for easier debugging/testing
	assetKeys := make([]string, 0, len(assetCandidates))
	for key := range assetCandidates {
		assetKeys = append(assetKeys, key)
	}
	sort.Strings(assetKeys)

	changeTrustOps := make([]txnbuild.Operation, 0, len(assetCandidates))
	for _, key := range assetKeys {
		asset := assetCandidates[key]
		if _, ok := existingTrustlines[key]; ok {
			log.Ctx(ctx).Debugf("account %s already has trustline for %s; skipping", account.Address, key)
			continue
		}

		changeTrustOps = append(changeTrustOps, &txnbuild.ChangeTrust{
			Line:          txnbuild.ChangeTrustAssetWrapper{Asset: asset.ToBasicAsset()},
			Limit:         "",
			SourceAccount: account.Address,
		})
	}

	if len(changeTrustOps) == 0 {
		log.Ctx(ctx).Infof("account %s already has trustlines for all provided assets", account.Address)
		return 0, nil
	}

	transaction, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount: &txnbuild.SimpleAccount{
			AccountID: account.Address,
			Sequence:  accountDetails.Sequence,
		},
		IncrementSequenceNum: true,
		Operations:           changeTrustOps,
		BaseFee:              int64(submitterEngine.MaxBaseFee),
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(20)},
	})
	if err != nil {
		return 0, fmt.Errorf("creating change trust transaction: %w", err)
	}

	signedTx, err := submitterEngine.SignStellarTransaction(ctx, transaction, account)
	if err != nil {
		return 0, fmt.Errorf("signing change trust transaction: %w", err)
	}

	log.Ctx(ctx).Infof("adding %d trustlines to account %s", len(changeTrustOps), account.Address)
	_, err = submitterEngine.HorizonClient.SubmitTransactionWithOptions(signedTx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true})
	if err != nil {
		return 0, fmt.Errorf("submitting change trust transaction to network: %w", utils.NewHorizonErrorWrapper(err))
	}

	return len(changeTrustOps), nil
}

// CreateAndFundAccount creates and funds a new destination account on the Stellar network with the given amount of native asset from the source account.
func CreateAndFundAccount(ctx context.Context, submitterEngine engine.SubmitterEngine, amountNativeAssetToSend int, sourceAcc, destinationAcc string) error {
	hostAccount := submitterEngine.HostDistributionAccount()
	if sourceAcc == destinationAcc {
		return fmt.Errorf("funding source account and destination account cannot be the same: %s", sourceAcc)
	}

	if err := retry.Do(func() error {
		srcAccDetails, err := getAccountDetails(submitterEngine.HorizonClient, sourceAcc)
		if err != nil {
			return fmt.Errorf("getting details for source account: %w", err)
		}

		tx, err := txnbuild.NewTransaction(
			txnbuild.TransactionParams{
				SourceAccount:        srcAccDetails,
				IncrementSequenceNum: true,
				BaseFee:              int64(submitterEngine.MaxBaseFee),
				Preconditions: txnbuild.Preconditions{
					TimeBounds: txnbuild.NewTimeout(30),
				},
				Operations: []txnbuild.Operation{
					&txnbuild.CreateAccount{
						Destination: destinationAcc,
						Amount:      strconv.Itoa(amountNativeAssetToSend),
					},
				},
			},
		)
		if err != nil {
			return fmt.Errorf(
				"creating raw create account tx for account %s: %w",
				destinationAcc,
				err,
			)
		}
		// Host distribution account signing:
		tx, err = submitterEngine.SignStellarTransaction(ctx, tx, hostAccount)
		if err != nil {
			return fmt.Errorf(
				"signing create account tx for account %s: %w",
				destinationAcc,
				err,
			)
		}

		_, err = submitterEngine.HorizonClient.SubmitTransactionWithOptions(tx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true})
		//nolint:wrapcheck // not wrapping because stellar/go's horizonclient.Error is not compatible with the fmt.Errorf wrapper
		return err
	},
		retry.Context(ctx), // Respect the context's cancellation
		retry.Attempts(CreateAndFundAccountRetryAttempts),
		retry.MaxDelay(1*time.Minute),
		retry.DelayType(retry.BackOffDelay),
		retry.RetryIf(func(err error) bool {
			hError := utils.NewHorizonErrorWrapper(err)
			// issues not related to the tx submission on the network should be retried
			if !hError.IsHorizonError() {
				return true
			} else if !hError.ShouldMarkAsError() {
				log.Ctx(ctx).Warnf("submitting create account tx for account %s to the Stellar network - retriable error: %v", destinationAcc, hError)
				return true
			}
			// if any terminal errors encountered, we should not retry
			return false
		}),
		retry.OnRetry(func(n uint, err error) {
			log.Ctx(ctx).Warnf("🔄 Submitting create account tx for account %s - attempt %d failed with error: %v",
				destinationAcc,
				n+1,
				err)
		}),
	); err != nil {
		return fmt.Errorf("maximum number of retries reached or terminal error encountered: %w", utils.NewHorizonErrorWrapper(err))
	}

	_, err := getAccountDetails(submitterEngine.HorizonClient, destinationAcc)
	if err != nil {
		return fmt.Errorf("getting details for destination account: %w", err)
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

// getAssetID returns asset identifier formatted as CODE:issuer.
func getAssetID(code, issuer string) string {
	return fmt.Sprintf("%s:%s", strings.ToUpper(code), issuer)
}
