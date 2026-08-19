package httphandler

import (
	"context"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

// resolveSourceDistributionAccount turns the source wallet a fund-moving write was routed to
// into the schema.TransactionAccount its balance/trustline checks must run against.
//
// For a Stellar wallet the account comes from the wallet row itself: the check has to run
// against THIS request's own source wallet, not the tenant's legacy single distribution account
// (DistributionAccountFromContext) — on a multi-wallet tenant those are frequently different
// accounts, and checking the wrong one either wrongly blocks a valid request or (worse)
// validates against an account with no relation to the funds actually being spent.
//
// Circle wallets have to come from the tenant instead, because distribution_wallets can only
// describe a Stellar account. It has no column for a Circle wallet ID, and the row is created
// blank and PENDING_USER_ACTIVATION for a Circle tenant — provisionDistributionAccount returns
// without an address, and syncDefaultDistributionWallet only copies one across when the type
// IsStellar. Completing Circle setup does not repair it either: the config handler upserts
// circle_client_config and flips the TENANT to ACTIVE, never touching this row. Reading the
// account off the row would therefore reject every Circle request on the empty-address check
// below, and even past that would hand the service an account with no CircleWalletID — which is
// how Circle payments locate the money (TransactionAccount.ID() is "circle:<walletID>").
//
// Nothing is lost by resolving those from the tenant, which is where a Circle account's wallet
// ID and live status actually live: multi-wallet is Stellar-only, since CreateWallet mints a
// Stellar keypair, so a Circle tenant has exactly one distribution account to resolve.
//
// noFundedAccountMsg is the 400 rendered for a Stellar wallet whose account is not provisioned
// yet, so each call site can name its own resource.
func resolveSourceDistributionAccount(
	ctx context.Context,
	resolver signing.DistributionAccountResolver,
	sourceWallet *data.DistributionWallet,
	noFundedAccountMsg string,
) (schema.TransactionAccount, *httperror.HTTPError) {
	if sourceWallet.AccountType.IsStellar() {
		if sourceWallet.Address == nil || *sourceWallet.Address == "" {
			return schema.TransactionAccount{}, httperror.BadRequest(noFundedAccountMsg, nil, nil)
		}

		return schema.TransactionAccount{
			Address: *sourceWallet.Address,
			Type:    sourceWallet.AccountType,
			Status:  sourceWallet.AccountStatus,
		}, nil
	}

	distributionAccount, err := resolver.DistributionAccountFromContext(ctx)
	if err != nil {
		return schema.TransactionAccount{}, httperror.InternalError(ctx, "Cannot get distribution account", err, nil)
	}

	return distributionAccount, nil
}

// resolveDistributionAccountForBalanceRead is the read-mode counterpart: it resolves the account
// whose balances a read endpoint should report for one distribution wallet.
//
// The resolution rule is the same one documented above — Stellar from the wallet row, everything
// else (Circle) from the tenant, because a Circle tenant's row is blank and
// PENDING_USER_ACTIVATION by design. Reading the account off the row instead would report an
// empty balance set for every Circle tenant, disagreeing with the legacy GET /balances, which
// resolves the same tenant's account from the context.
//
// The one difference is what an unprovisioned Stellar wallet means. A balance read moves no
// funds, so it is not the error a fund-moving write has to raise: ok is false and the caller
// reports an empty balance set, which is exactly what a wallet with no on-chain account holds.
func resolveDistributionAccountForBalanceRead(
	ctx context.Context,
	resolver signing.DistributionAccountResolver,
	wallet *data.DistributionWallet,
) (schema.TransactionAccount, bool, *httperror.HTTPError) {
	if wallet.AccountType.IsStellar() && (wallet.Address == nil || *wallet.Address == "") {
		return schema.TransactionAccount{}, false, nil
	}

	// The guard above already absorbed the only case that renders noFundedAccountMsg, so the
	// message below is unreachable; it is written out rather than left empty so a future change
	// to the Stellar branch cannot surface a blank 400.
	account, httpErr := resolveSourceDistributionAccount(ctx, resolver, wallet,
		"the distribution wallet has no funded distribution account yet")
	if httpErr != nil {
		return schema.TransactionAccount{}, false, httpErr
	}

	return account, true, nil
}
