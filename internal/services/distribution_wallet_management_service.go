package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	tssSvc "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/services"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

var (
	// ErrDistributionWalletCapExceeded is returned when a tenant attempts to create more than
	// the v1 per-tenant wallet cap.
	ErrDistributionWalletCapExceeded = fmt.Errorf("tenant has reached the maximum of %d distribution wallets", data.MaxDistributionWalletsPerTenant)
	// ErrUnsupportedDistributionWalletType is returned for account types that cannot be
	// provisioned per-wallet in v1.
	ErrUnsupportedDistributionWalletType = errors.New("only DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT wallets can be created in v1")
	// ErrTenantNotEligibleForMultiWallet is returned when a tenant whose distribution
	// account is not STELLAR.DB_VAULT attempts to provision additional wallets. v1
	// restricts multi-wallet to DB_VAULT tenants to avoid creating mixed-custody tenants.
	ErrTenantNotEligibleForMultiWallet = errors.New("multi-wallet is only available to tenants on DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT in v1")
	// ErrCannotArchiveDefaultWallet is returned when archiving the default wallet without
	// promoting another active wallet first.
	ErrCannotArchiveDefaultWallet = errors.New("the default wallet cannot be archived: promote another active wallet to default first")
	// ErrCannotArchiveLastActiveWallet is returned when archival would leave the tenant with
	// zero active wallets.
	ErrCannotArchiveLastActiveWallet = errors.New("cannot archive the last active distribution wallet")
	// ErrCannotPromoteWallet is returned when promoting a wallet that is missing or archived.
	ErrCannotPromoteWallet = errors.New("only an existing, active wallet can be promoted to default")
)

// DistributionWalletManagementServiceInterface manages the lifecycle of a tenant's
// distribution wallets (the sending accounts), Owner-only at the API layer.
type DistributionWalletManagementServiceInterface interface {
	CreateWallet(ctx context.Context, insert data.DistributionWalletInsert) (*data.DistributionWallet, error)
	GetWallet(ctx context.Context, id string) (*data.DistributionWallet, error)
	ListWallets(ctx context.Context, includeArchived bool) ([]data.DistributionWallet, error)
	ArchiveWallet(ctx context.Context, id string) (*data.DistributionWallet, error)
	PromoteToDefault(ctx context.Context, id string) (*data.DistributionWallet, error)
	ListMemberships(ctx context.Context, walletID string) ([]data.WalletMembership, error)
	ListMembershipAudit(ctx context.Context, walletID string) ([]data.WalletMembershipAuditEntry, error)
	GrantMembership(ctx context.Context, walletID, userID string, role data.UserRole, grantedBy string) (*data.WalletMembership, error)
	RevokeMembership(ctx context.Context, walletID, membershipID, revokedBy string) error
}

type DistributionWalletManagementService struct {
	Models                     *data.Models
	SubmitterEngine            engine.SubmitterEngine
	WalletKeyService           tssSvc.DistributionWalletKeyServiceInterface
	NativeAssetBootstrapAmount int
}

var _ DistributionWalletManagementServiceInterface = (*DistributionWalletManagementService)(nil)

func NewDistributionWalletManagementService(
	models *data.Models,
	submitterEngine engine.SubmitterEngine,
	walletKeyService tssSvc.DistributionWalletKeyServiceInterface,
	nativeAssetBootstrapAmount int,
) (*DistributionWalletManagementService, error) {
	if models == nil {
		return nil, fmt.Errorf("models cannot be nil")
	}
	if walletKeyService == nil {
		return nil, fmt.Errorf("walletKeyService cannot be nil")
	}

	return &DistributionWalletManagementService{
		Models:                     models,
		SubmitterEngine:            submitterEngine,
		WalletKeyService:           walletKeyService,
		NativeAssetBootstrapAmount: nativeAssetBootstrapAmount,
	}, nil
}

// acquireTenantWalletLock serializes wallet-lifecycle mutations (create/archive/promote) per
// tenant via a transaction-scoped advisory lock keyed on the tenant schema, making the
// read-modify-write invariants (wallet cap, single-default, zero-active) race-safe against
// concurrent owner requests. Released at COMMIT; the DB triggers remain the ultimate backstop.
func acquireTenantWalletLock(ctx context.Context, dbTx db.DBTransaction) error {
	if _, err := dbTx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext(current_schema())::bigint)"); err != nil {
		return fmt.Errorf("acquiring per-tenant wallet-lifecycle lock: %w", err)
	}
	return nil
}

// CreateWallet provisions a new, independently funded distribution wallet for the tenant:
// it reserves the wallet row, generates a fresh keypair stored with per-wallet envelope
// encryption, funds the account from the host distribution account, and adds trustlines for
// the tenant's enabled assets. On provisioning failure the never-used row and key are cleaned
// up — archive-don't-delete only protects wallets that have signed transactions.
func (s *DistributionWalletManagementService) CreateWallet(ctx context.Context, insert data.DistributionWalletInsert) (*data.DistributionWallet, error) {
	if insert.AccountType == "" {
		insert.AccountType = schema.DistributionAccountStellarDBVault
	}
	if insert.AccountType != schema.DistributionAccountStellarDBVault {
		return nil, fmt.Errorf("creating wallet with account type %q: %w", insert.AccountType, ErrUnsupportedDistributionWalletType)
	}

	dbPool := s.Models.DBConnectionPool

	// Reserve the wallet row atomically. A per-tenant (per-schema) advisory lock makes the
	// eligibility + cap checks and the insert one critical section: without it, two concurrent
	// owner creates could each pass a stale Count check and exceed the cap (TOCTOU). The
	// lock is held only for these fast DB ops and released at COMMIT — the on-chain keygen,
	// funding, and trustline work below deliberately runs OUTSIDE this transaction.
	wallet, err := db.RunInTransactionWithResult(ctx, dbPool, nil, func(dbTx db.DBTransaction) (*data.DistributionWallet, error) {
		if lockErr := acquireTenantWalletLock(ctx, dbTx); lockErr != nil {
			return nil, lockErr
		}

		// v1 multi-wallet eligibility: only tenants whose distribution account is
		// STELLAR.DB_VAULT may provision additional wallets. A non-DB_VAULT tenant
		// (STELLAR.ENV or CIRCLE) would become mixed-custody, and promoting the new
		// DB_VAULT wallet to default would silently flip the tenant's account type and
		// break balance aggregation (Circle balances come from the Circle API, Stellar
		// from Horizon). Relaxing this to STELLAR.ENV is a safe later step; CIRCLE needs
		// a designed mixed-custody story first.
		defaultWallet, defErr := s.Models.DistributionWallets.GetDefault(ctx, dbTx)
		if defErr != nil {
			return nil, fmt.Errorf("resolving tenant multi-wallet eligibility: %w", defErr)
		}
		if defaultWallet.AccountType != schema.DistributionAccountStellarDBVault {
			return nil, fmt.Errorf("tenant distribution account type is %q: %w", defaultWallet.AccountType, ErrTenantNotEligibleForMultiWallet)
		}

		count, cntErr := s.Models.DistributionWallets.Count(ctx, dbTx)
		if cntErr != nil {
			return nil, fmt.Errorf("counting distribution wallets: %w", cntErr)
		}
		if count >= data.MaxDistributionWalletsPerTenant {
			return nil, ErrDistributionWalletCapExceeded
		}

		return s.Models.DistributionWallets.Insert(ctx, dbTx, insert)
	})
	if err != nil {
		return nil, fmt.Errorf("reserving distribution wallet row: %w", err)
	}

	kp, err := keypair.Random()
	if err != nil {
		s.cleanupWallet(ctx, wallet.ID, "")
		return nil, fmt.Errorf("generating keypair for distribution wallet %q: %w", wallet.ID, err)
	}

	if err = s.WalletKeyService.StoreKey(ctx, kp.Address(), kp.Seed()); err != nil {
		s.cleanupWallet(ctx, wallet.ID, "")
		return nil, fmt.Errorf("storing key for distribution wallet %q: %w", wallet.ID, err)
	}

	updatedWallet, err := s.Models.DistributionWallets.UpdateAddress(ctx, dbPool, wallet.ID, kp.Address())
	if err != nil {
		s.cleanupWallet(ctx, wallet.ID, kp.Address())
		return nil, fmt.Errorf("attaching address to distribution wallet %q: %w", wallet.ID, err)
	}
	wallet = updatedWallet

	hostAccount := s.SubmitterEngine.HostDistributionAccount()
	if err = tssSvc.CreateAndFundAccount(ctx, s.SubmitterEngine, s.NativeAssetBootstrapAmount, hostAccount.Address, kp.Address()); err != nil {
		s.cleanupWallet(ctx, wallet.ID, kp.Address())
		return nil, fmt.Errorf("funding distribution wallet %q: %w", wallet.ID, err)
	}

	// Trustlines are best-effort: the wallet is already live and funded on-chain, so a
	// trustline failure must not orphan it. Missing trustlines only block non-native assets
	// and can be re-attempted by operations.
	if err = s.addTrustlines(ctx, *wallet); err != nil {
		log.Ctx(ctx).Errorf("adding trustlines for new distribution wallet %q (%s): %v — wallet is funded and ACTIVE; trustlines must be added manually", wallet.ID, kp.Address(), err)
	}

	// Outbox: wallet.created. Creation is not transactional (it spans on-chain calls),
	// so a write failure is logged rather than failing the already-provisioned wallet.
	if err = events.Write(ctx, dbPool, events.WalletCreated, wallet.ID, map[string]any{
		"wallet_id": wallet.ID, "name": wallet.Name, "status": string(wallet.Status), "is_default": wallet.IsDefault,
	}); err != nil {
		log.Ctx(ctx).Errorf("writing wallet.created event for %q: %v", wallet.ID, err)
	}

	return wallet, nil
}

// GetWallet returns one distribution wallet by id.
func (s *DistributionWalletManagementService) GetWallet(ctx context.Context, id string) (*data.DistributionWallet, error) {
	wallet, err := s.Models.DistributionWallets.Get(ctx, s.Models.DBConnectionPool, id)
	if err != nil {
		return nil, fmt.Errorf("getting distribution wallet %q: %w", id, err)
	}
	return wallet, nil
}

// ListWallets returns the tenant's distribution wallets, optionally including archived ones
// (operational surfaces hide archived wallets; audit/admin views include them).
func (s *DistributionWalletManagementService) ListWallets(ctx context.Context, includeArchived bool) ([]data.DistributionWallet, error) {
	wallets, err := s.Models.DistributionWallets.GetAll(ctx, s.Models.DBConnectionPool, includeArchived)
	if err != nil {
		return nil, fmt.Errorf("listing distribution wallets: %w", err)
	}
	return wallets, nil
}

// ArchiveWallet marks a wallet ARCHIVED: it accepts no new disbursements but its history and
// key material are preserved permanently for audit, dispute, and compliance lookups. The
// default wallet cannot be archived (promote another wallet first), and archival never leaves
// the tenant with zero active wallets. Both rules are independently enforced by the DB.
func (s *DistributionWalletManagementService) ArchiveWallet(ctx context.Context, id string) (*data.DistributionWallet, error) {
	archived, err := db.RunInTransactionWithResult(ctx, s.Models.DBConnectionPool, nil, func(dbTx db.DBTransaction) (*data.DistributionWallet, error) {
		// Serialize lifecycle mutations so the "not last active" check can't race a concurrent
		// archive into a zero-active state; all reads + the archive run under one snapshot.
		if lockErr := acquireTenantWalletLock(ctx, dbTx); lockErr != nil {
			return nil, lockErr
		}

		wallet, gErr := s.Models.DistributionWallets.Get(ctx, dbTx, id)
		if gErr != nil {
			return nil, fmt.Errorf("loading distribution wallet %q: %w", id, gErr)
		}
		if wallet.IsDefault {
			return nil, ErrCannotArchiveDefaultWallet
		}

		activeWallets, lErr := s.Models.DistributionWallets.GetAll(ctx, dbTx, false)
		if lErr != nil {
			return nil, fmt.Errorf("listing active distribution wallets: %w", lErr)
		}
		remaining := 0
		for _, w := range activeWallets {
			if w.ID != id {
				remaining++
			}
		}
		if remaining == 0 {
			return nil, ErrCannotArchiveLastActiveWallet
		}

		archivedWallet, aErr := s.Models.DistributionWallets.Archive(ctx, dbTx, id)
		if aErr != nil {
			return nil, fmt.Errorf("archiving distribution wallet %q: %w", id, aErr)
		}
		// Outbox: wallet.archived, same transaction.
		if eErr := events.Write(ctx, dbTx, events.WalletArchived, archivedWallet.ID, map[string]any{
			"wallet_id": archivedWallet.ID, "name": archivedWallet.Name, "status": string(archivedWallet.Status),
		}); eErr != nil {
			return nil, fmt.Errorf("writing wallet.archived event: %w", eErr)
		}
		return archivedWallet, nil
	})
	if err != nil {
		return nil, err
	}

	return archived, nil
}

// PromoteToDefault atomically promotes an active wallet to be the tenant's default: in a
// single transaction it demotes the old default, promotes the new one, and reassigns the
// default-bound association — the tenant's distribution-account fields in the admin schema,
// which legacy single-wallet resolution reads. All-or-nothing: any failure rolls back both
// the default pointer and the association reassignment.
func (s *DistributionWalletManagementService) PromoteToDefault(ctx context.Context, id string) (*data.DistributionWallet, error) {
	promoted, err := db.RunInTransactionWithResult(ctx, s.Models.DBConnectionPool, nil, func(dbTx db.DBTransaction) (*data.DistributionWallet, error) {
		// Serialize lifecycle mutations so concurrent promotes can't race the single-default
		// invariant (the DB constraint is the backstop).
		if lockErr := acquireTenantWalletLock(ctx, dbTx); lockErr != nil {
			return nil, lockErr
		}

		wallet, txErr := s.Models.DistributionWallets.PromoteToDefault(ctx, dbTx, id)
		if txErr != nil {
			if errors.Is(txErr, data.ErrRecordNotFound) {
				return nil, fmt.Errorf("%w: %w", ErrCannotPromoteWallet, txErr)
			}
			return nil, fmt.Errorf("promoting distribution wallet %q: %w", id, txErr)
		}

		if syncErr := s.syncTenantDefaultAccount(ctx, dbTx, wallet); syncErr != nil {
			return nil, fmt.Errorf("reassigning default-bound associations for wallet %q: %w", id, syncErr)
		}

		// Outbox: wallet.promoted_to_default, same transaction as the promotion.
		if eventErr := events.Write(ctx, dbTx, events.WalletPromotedToDefault, wallet.ID, map[string]any{
			"wallet_id": wallet.ID, "name": wallet.Name,
		}); eventErr != nil {
			return nil, fmt.Errorf("writing wallet.promoted_to_default event: %w", eventErr)
		}

		return wallet, nil
	})
	if err != nil {
		return nil, err
	}

	return promoted, nil
}

// syncTenantDefaultAccount points the tenant row's distribution-account fields (admin schema)
// at the new default wallet's account, inside the caller's transaction. Pre-multi-wallet code
// paths resolve "the tenant's account" through those fields, so they must always mirror the
// default wallet. No-op in layouts without the admin schema (flat test databases) or without
// a tenant in context.
func (s *DistributionWalletManagementService) syncTenantDefaultAccount(ctx context.Context, dbTx db.DBTransaction, wallet *data.DistributionWallet) error {
	var adminTenantsExists bool
	if err := dbTx.GetContext(ctx, &adminTenantsExists, `SELECT to_regclass('admin.tenants') IS NOT NULL`); err != nil {
		return fmt.Errorf("checking for admin.tenants: %w", err)
	}
	if !adminTenantsExists {
		return nil
	}

	tnt, err := sdpcontext.GetTenantFromContext(ctx)
	if err != nil || tnt == nil {
		//nolint:nilerr // a missing tenant in context (single-schema/test layouts) is the documented no-op condition.
		return nil
	}

	res, err := dbTx.ExecContext(ctx, `
		UPDATE admin.tenants
		SET distribution_account_address = $2,
		    distribution_account_type = $3::admin.distribution_account_type,
		    distribution_account_status = $4::admin.distribution_account_status
		WHERE id = $1`,
		tnt.ID, wallet.Address, wallet.AccountType, wallet.AccountStatus)
	if err != nil {
		return fmt.Errorf("updating tenant %q distribution account: %w", tnt.ID, err)
	}
	if _, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("reading rows affected: %w", err)
	}

	return nil
}

// ListMemberships returns a wallet's memberships. Archived wallets remain queryable: their
// memberships persist for historical authorization ("who could approve on Wallet A").
func (s *DistributionWalletManagementService) ListMemberships(ctx context.Context, walletID string) ([]data.WalletMembership, error) {
	dbPool := s.Models.DBConnectionPool

	if _, err := s.Models.DistributionWallets.Get(ctx, dbPool, walletID); err != nil {
		return nil, fmt.Errorf("loading distribution wallet %q: %w", walletID, err)
	}

	memberships, err := s.Models.WalletMemberships.ListByWallet(ctx, dbPool, walletID)
	if err != nil {
		return nil, fmt.Errorf("listing memberships for wallet %q: %w", walletID, err)
	}
	return memberships, nil
}

// membershipAuditLimit caps the audit read; pagination is a follow-up design decision.
const membershipAuditLimit = 100

// ListMembershipAudit returns a wallet's membership-change history (grants and revokes) from
// the append-only audit table, newest first. Archived wallets remain queryable.
func (s *DistributionWalletManagementService) ListMembershipAudit(ctx context.Context, walletID string) ([]data.WalletMembershipAuditEntry, error) {
	dbPool := s.Models.DBConnectionPool

	if _, err := s.Models.DistributionWallets.Get(ctx, dbPool, walletID); err != nil {
		return nil, fmt.Errorf("loading distribution wallet %q: %w", walletID, err)
	}

	entries, err := s.Models.WalletMemberships.ListAuditByWallet(ctx, dbPool, walletID, membershipAuditLimit)
	if err != nil {
		return nil, fmt.Errorf("listing membership audit for wallet %q: %w", walletID, err)
	}
	return entries, nil
}

// GrantMembership grants a wallet-scoped role to a user, recording the grantor. Grants on
// archived wallets fail (the API maps it to 409) — inert grants are audit-log noise.
func (s *DistributionWalletManagementService) GrantMembership(ctx context.Context, walletID, userID string, role data.UserRole, grantedBy string) (*data.WalletMembership, error) {
	dbPool := s.Models.DBConnectionPool

	if _, err := s.Models.DistributionWallets.Get(ctx, dbPool, walletID); err != nil {
		return nil, fmt.Errorf("loading distribution wallet %q: %w", walletID, err)
	}

	var grantedByPtr *string
	if grantedBy != "" {
		grantedByPtr = &grantedBy
	}

	membership, err := db.RunInTransactionWithResult(ctx, dbPool, nil, func(dbTx db.DBTransaction) (*data.WalletMembership, error) {
		granted, txErr := s.Models.WalletMemberships.Insert(ctx, dbTx, userID, walletID, role, grantedByPtr)
		if txErr != nil {
			return nil, fmt.Errorf("granting membership on wallet %q: %w", walletID, txErr)
		}
		// Outbox: membership grant with actor attribution, same transaction.
		if txErr = events.Write(ctx, dbTx, events.WalletMembershipGranted, walletID, map[string]any{
			"membership_id": granted.ID, "wallet_id": walletID, "user_id": userID,
			"role": string(role), "actor_user_id": grantedBy,
		}); txErr != nil {
			return nil, fmt.Errorf("writing wallet.membership_granted event: %w", txErr)
		}
		return granted, nil
	})
	if err != nil {
		return nil, err
	}

	return membership, nil
}

// RevokeMembership revokes one membership on the given wallet. The append-only audit table
// retains the full grant/revoke history, and a WalletMembershipRevoked event is emitted in the
// same transaction.
func (s *DistributionWalletManagementService) RevokeMembership(ctx context.Context, walletID, membershipID, revokedBy string) error {
	dbPool := s.Models.DBConnectionPool

	membership, err := s.Models.WalletMemberships.Get(ctx, dbPool, membershipID)
	if err != nil {
		return fmt.Errorf("loading membership %q: %w", membershipID, err)
	}
	if membership.WalletID != walletID {
		// Per the read-leakage rules, do not disclose that the membership exists elsewhere.
		return fmt.Errorf("loading membership %q: %w", membershipID, data.ErrRecordNotFound)
	}

	err = db.RunInTransaction(ctx, dbPool, nil, func(dbTx db.DBTransaction) error {
		if txErr := s.Models.WalletMemberships.Delete(ctx, dbTx, membershipID); txErr != nil {
			return fmt.Errorf("revoking membership %q: %w", membershipID, txErr)
		}
		// Outbox: membership revocation with actor attribution, same transaction.
		if txErr := events.Write(ctx, dbTx, events.WalletMembershipRevoked, membership.WalletID, map[string]any{
			"membership_id": membershipID, "wallet_id": membership.WalletID, "user_id": membership.UserID,
			"role": string(membership.Role), "actor_user_id": revokedBy,
		}); txErr != nil {
			return fmt.Errorf("writing wallet.membership_revoked event: %w", txErr)
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.Ctx(ctx).Infof("wallet membership revoked: membership=%s wallet=%s user=%s role=%s revoked_by=%s",
		membershipID, membership.WalletID, membership.UserID, membership.Role, revokedBy)

	return nil
}

// cleanupWallet removes a freshly created wallet row (and its stored key, when one exists)
// after a provisioning failure. Best-effort: cleanup failures are logged, not returned, so the
// original provisioning error stays primary.
func (s *DistributionWalletManagementService) cleanupWallet(ctx context.Context, walletID, publicKey string) {
	if publicKey != "" {
		if err := s.WalletKeyService.DeleteKey(ctx, publicKey); err != nil {
			log.Ctx(ctx).Errorf("cleaning up key for failed distribution wallet %q (%s): %v", walletID, publicKey, err)
		}
	}
	if err := s.Models.DistributionWallets.Delete(ctx, s.Models.DBConnectionPool, walletID); err != nil {
		log.Ctx(ctx).Errorf("cleaning up failed distribution wallet %q: %v", walletID, err)
	}
}

// addTrustlines adds trustlines on the new wallet for all non-native assets supported by the
// tenant's enabled recipient wallet providers, mirroring tenant-provisioning behavior.
func (s *DistributionWalletManagementService) addTrustlines(ctx context.Context, wallet data.DistributionWallet) error {
	wallets, err := s.Models.Wallets.FindWallets(ctx, data.NewFilter(data.FilterEnabledWallets, true))
	if err != nil {
		return fmt.Errorf("listing enabled wallets: %w", err)
	}

	supportedAssets := make(map[string]data.Asset)
	for _, w := range wallets {
		for _, asset := range w.Assets {
			if asset.IsNative() {
				continue
			}
			supportedAssets[fmt.Sprintf("%s:%s", asset.Code, asset.Issuer)] = asset
		}
	}
	if len(supportedAssets) == 0 {
		return nil
	}

	assetsToTrust := make([]data.Asset, 0, len(supportedAssets))
	for _, asset := range supportedAssets {
		assetsToTrust = append(assetsToTrust, asset)
	}

	distAccount := schema.TransactionAccount{
		Address: *wallet.Address,
		Type:    wallet.AccountType,
		Status:  wallet.AccountStatus,
	}
	if _, err := tssSvc.AddTrustlines(ctx, s.SubmitterEngine, distAccount, assetsToTrust); err != nil {
		return fmt.Errorf("submitting change trust transaction: %w", err)
	}

	return nil
}
