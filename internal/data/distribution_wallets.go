package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

// DistributionWalletStatus is the lifecycle status of a distribution wallet. Archived wallets
// accept no new disbursements but are never hard-deleted once they have signed transactions.
type DistributionWalletStatus string

const (
	ActiveDistributionWalletStatus   DistributionWalletStatus = "ACTIVE"
	ArchivedDistributionWalletStatus DistributionWalletStatus = "ARCHIVED"
	// PendingDistributionWalletStatus marks a wallet reserved by CreateWallet whose on-chain
	// keypair has not yet been generated and funded — it is not usable and must not be treated
	// as available (see wallet_scope.go resolveSourceWalletForWrite).
	PendingDistributionWalletStatus DistributionWalletStatus = "PENDING"
)

// DefaultDistributionWalletName is the reserved name of the implicit wallet that mirrors the
// tenant's original single distribution account.
const DefaultDistributionWalletName = "default"

// MaxDistributionWalletsPerTenant is the v1 cap on wallets per tenant.
const MaxDistributionWalletsPerTenant = 20

// DistributionWallet is the tenant's SENDING account entity ("distribution wallet"). It is
// unrelated to Wallet, which models recipient wallet providers.
type DistributionWallet struct {
	ID            string                   `json:"id" db:"id"`
	Name          string                   `json:"name" db:"name"`
	Description   *string                  `json:"description,omitempty" db:"description"`
	Address       *string                  `json:"distribution_account_address,omitempty" db:"distribution_account_address"`
	AccountType   schema.AccountType       `json:"distribution_account_type" db:"distribution_account_type"`
	AccountStatus schema.AccountStatus     `json:"distribution_account_status" db:"distribution_account_status"`
	Status        DistributionWalletStatus `json:"status" db:"status"`
	IsDefault     bool                     `json:"is_default" db:"is_default"`
	CreatedAt     time.Time                `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time                `json:"updated_at" db:"updated_at"`
	ArchivedAt    *time.Time               `json:"archived_at,omitempty" db:"archived_at"`
}

// DistributionWalletInsert is the payload to create a new distribution wallet. The account
// address is attached after the wallet's keypair is provisioned.
type DistributionWalletInsert struct {
	Name        string
	Description *string
	AccountType schema.AccountType
}

func (i DistributionWalletInsert) Validate() error {
	if i.Name == "" {
		return fmt.Errorf("name is required: %w", ErrMissingInput)
	}
	if len(i.Name) > 64 {
		return fmt.Errorf("name cannot be longer than 64 characters")
	}
	if !i.AccountType.IsDistribution() {
		return fmt.Errorf("account type %q is not a distribution account type", i.AccountType)
	}
	return nil
}

type DistributionWalletModel struct {
	dbConnectionPool db.DBConnectionPool
}

const distributionWalletColumns = `
	id, name, description, distribution_account_address, distribution_account_type,
	distribution_account_status, status, is_default, created_at, updated_at, archived_at
`

// Insert creates a new (non-default) distribution wallet. The 20-wallet cap and owner-only
// authorization are enforced by the service/API layers.
func (m *DistributionWalletModel) Insert(ctx context.Context, sqlExec db.SQLExecuter, insert DistributionWalletInsert) (*DistributionWallet, error) {
	if err := insert.Validate(); err != nil {
		return nil, fmt.Errorf("validating distribution wallet insert: %w", err)
	}

	query := fmt.Sprintf(`
		INSERT INTO distribution_wallets (name, description, distribution_account_type, status)
		VALUES ($1, $2, $3, $4)
		RETURNING %s
	`, distributionWalletColumns)

	var wallet DistributionWallet
	err := sqlExec.GetContext(ctx, &wallet, query, insert.Name, insert.Description, insert.AccountType, PendingDistributionWalletStatus)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return nil, fmt.Errorf("a distribution wallet with name %q already exists: %w", insert.Name, ErrRecordAlreadyExists)
		}
		return nil, fmt.Errorf("inserting distribution wallet: %w", err)
	}

	return &wallet, nil
}

// Get returns the distribution wallet with the given id.
func (m *DistributionWalletModel) Get(ctx context.Context, sqlExec db.SQLExecuter, id string) (*DistributionWallet, error) {
	query := fmt.Sprintf(`SELECT %s FROM distribution_wallets WHERE id = $1`, distributionWalletColumns)

	var wallet DistributionWallet
	err := sqlExec.GetContext(ctx, &wallet, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("getting distribution wallet %q: %w", id, ErrRecordNotFound)
		}
		return nil, fmt.Errorf("getting distribution wallet %q: %w", id, err)
	}

	return &wallet, nil
}

// GetByName returns the distribution wallet with the given name.
func (m *DistributionWalletModel) GetByName(ctx context.Context, sqlExec db.SQLExecuter, name string) (*DistributionWallet, error) {
	query := fmt.Sprintf(`SELECT %s FROM distribution_wallets WHERE name = $1`, distributionWalletColumns)

	var wallet DistributionWallet
	err := sqlExec.GetContext(ctx, &wallet, query, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("getting distribution wallet named %q: %w", name, ErrRecordNotFound)
		}
		return nil, fmt.Errorf("getting distribution wallet named %q: %w", name, err)
	}

	return &wallet, nil
}

// GetAll returns the tenant's distribution wallets, default first then by creation time.
// Non-active wallets (ARCHIVED, or PENDING mid-provisioning) are excluded unless
// includeArchived is true: operational surfaces (pickers, dropdowns, filters) show only usable
// wallets; audit/admin views include everything regardless of status.
func (m *DistributionWalletModel) GetAll(ctx context.Context, sqlExec db.SQLExecuter, includeArchived bool) ([]DistributionWallet, error) {
	query := fmt.Sprintf(`SELECT %s FROM distribution_wallets`, distributionWalletColumns)
	if !includeArchived {
		query += ` WHERE status = 'ACTIVE'`
	}
	query += ` ORDER BY is_default DESC, created_at ASC`

	wallets := []DistributionWallet{}
	if err := sqlExec.SelectContext(ctx, &wallets, query); err != nil {
		return nil, fmt.Errorf("getting distribution wallets: %w", err)
	}

	return wallets, nil
}

// GetDefault returns the tenant's default distribution wallet.
func (m *DistributionWalletModel) GetDefault(ctx context.Context, sqlExec db.SQLExecuter) (*DistributionWallet, error) {
	query := fmt.Sprintf(`SELECT %s FROM distribution_wallets WHERE is_default`, distributionWalletColumns)

	var wallet DistributionWallet
	err := sqlExec.GetContext(ctx, &wallet, query)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("getting default distribution wallet: %w", ErrRecordNotFound)
		}
		return nil, fmt.Errorf("getting default distribution wallet: %w", err)
	}

	return &wallet, nil
}

// Count returns the total number of distribution wallets (archived included — archived wallets
// still occupy a slot under the v1 cap, since they are never hard-deleted).
func (m *DistributionWalletModel) Count(ctx context.Context, sqlExec db.SQLExecuter) (int, error) {
	var count int
	if err := sqlExec.GetContext(ctx, &count, `SELECT COUNT(*) FROM distribution_wallets`); err != nil {
		return 0, fmt.Errorf("counting distribution wallets: %w", err)
	}
	return count, nil
}

// UpdateAddress attaches the provisioned account address to a wallet that does not have one
// yet. The address is immutable once set, mirroring the immutability of the account type.
func (m *DistributionWalletModel) UpdateAddress(ctx context.Context, sqlExec db.SQLExecuter, id, address string) (*DistributionWallet, error) {
	if address == "" {
		return nil, fmt.Errorf("address is required: %w", ErrMissingInput)
	}

	query := fmt.Sprintf(`
		UPDATE distribution_wallets
		SET distribution_account_address = $2
		WHERE id = $1 AND distribution_account_address IS NULL
		RETURNING %s
	`, distributionWalletColumns)

	var wallet DistributionWallet
	err := sqlExec.GetContext(ctx, &wallet, query, id, address)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("updating address of distribution wallet %q (missing, or address already set): %w", id, ErrRecordNotFound)
		}
		return nil, fmt.Errorf("updating address of distribution wallet %q: %w", id, err)
	}

	return &wallet, nil
}

// Activate promotes a PENDING wallet to ACTIVE once its on-chain account has been generated and
// funded. Only a PENDING wallet can be activated — this is not a general status-flip method.
func (m *DistributionWalletModel) Activate(ctx context.Context, sqlExec db.SQLExecuter, id string) (*DistributionWallet, error) {
	query := fmt.Sprintf(`
		UPDATE distribution_wallets
		SET status = 'ACTIVE'
		WHERE id = $1 AND status = 'PENDING'
		RETURNING %s
	`, distributionWalletColumns)

	var wallet DistributionWallet
	err := sqlExec.GetContext(ctx, &wallet, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("activating distribution wallet %q (missing or not pending): %w", id, ErrRecordNotFound)
		}
		return nil, fmt.Errorf("activating distribution wallet %q: %w", id, err)
	}

	return &wallet, nil
}

// Archive marks a wallet ARCHIVED (no new disbursements; history intact). The DB layer
// independently rejects archiving the default wallet (CHECK) and archiving the last active
// wallet (trigger); callers should pre-check both for friendly errors.
func (m *DistributionWalletModel) Archive(ctx context.Context, sqlExec db.SQLExecuter, id string) (*DistributionWallet, error) {
	query := fmt.Sprintf(`
		UPDATE distribution_wallets
		SET status = 'ARCHIVED', archived_at = NOW()
		WHERE id = $1 AND status = 'ACTIVE'
		RETURNING %s
	`, distributionWalletColumns)

	var wallet DistributionWallet
	err := sqlExec.GetContext(ctx, &wallet, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("archiving distribution wallet %q (missing or already archived): %w", id, ErrRecordNotFound)
		}
		return nil, fmt.Errorf("archiving distribution wallet %q: %w", id, err)
	}

	return &wallet, nil
}

// PromoteToDefault atomically makes the given ACTIVE wallet the tenant's default: it demotes
// the current default and promotes the new one. It MUST run inside a transaction together
// with any default-bound association reassignment — all-or-nothing per the spec.
func (m *DistributionWalletModel) PromoteToDefault(ctx context.Context, sqlExec db.SQLExecuter, id string) (*DistributionWallet, error) {
	if _, err := sqlExec.ExecContext(ctx, `UPDATE distribution_wallets SET is_default = FALSE WHERE is_default`); err != nil {
		return nil, fmt.Errorf("demoting current default distribution wallet: %w", err)
	}

	query := fmt.Sprintf(`
		UPDATE distribution_wallets
		SET is_default = TRUE
		WHERE id = $1 AND status = 'ACTIVE'
		RETURNING %s
	`, distributionWalletColumns)

	var wallet DistributionWallet
	err := sqlExec.GetContext(ctx, &wallet, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("promoting distribution wallet %q (missing or not active): %w", id, ErrRecordNotFound)
		}
		return nil, fmt.Errorf("promoting distribution wallet %q: %w", id, err)
	}

	return &wallet, nil
}

// Delete hard-deletes a distribution wallet row. It exists ONLY for cleanup of freshly created
// wallets whose provisioning failed (no disbursements can reference them yet). Wallets that
// have submitted transactions are protected by the ON DELETE RESTRICT FK and must be archived
// instead, per the spec's archive-don't-delete rule.
func (m *DistributionWalletModel) Delete(ctx context.Context, sqlExec db.SQLExecuter, id string) error {
	res, err := sqlExec.ExecContext(ctx, `DELETE FROM distribution_wallets WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting distribution wallet %q: %w", id, err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reading rows affected when deleting distribution wallet %q: %w", id, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("deleting distribution wallet %q: %w", id, ErrRecordNotFound)
	}
	return nil
}

// EnsureDefaultWallet makes the tenant's default distribution wallet reflect the given account
// material, updating the existing default row or creating it when absent (e.g. tenant schemas
// migrated before the tenant's account was provisioned, or test layouts without the backfill).
func (m *DistributionWalletModel) EnsureDefaultWallet(ctx context.Context, sqlExec db.SQLExecuter, address *string, accountType schema.AccountType, accountStatus schema.AccountStatus) (*DistributionWallet, error) {
	if !accountType.IsDistribution() {
		return nil, fmt.Errorf("account type %q is not a distribution account type", accountType)
	}

	updateQuery := fmt.Sprintf(`
		UPDATE distribution_wallets
		SET distribution_account_address = $1, distribution_account_type = $2, distribution_account_status = $3
		WHERE is_default
		RETURNING %s
	`, distributionWalletColumns)

	var wallet DistributionWallet
	err := sqlExec.GetContext(ctx, &wallet, updateQuery, address, accountType, accountStatus)
	if err == nil {
		return &wallet, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("updating default distribution wallet: %w", err)
	}

	insertQuery := fmt.Sprintf(`
		INSERT INTO distribution_wallets
			(name, description, distribution_account_address, distribution_account_type, distribution_account_status, is_default)
		VALUES
			($1, 'Auto-created wallet for the tenant''s original distribution account.', $2, $3, $4, TRUE)
		RETURNING %s
	`, distributionWalletColumns)

	err = sqlExec.GetContext(ctx, &wallet, insertQuery, DefaultDistributionWalletName, address, accountType, accountStatus)
	if err != nil {
		return nil, fmt.Errorf("creating default distribution wallet: %w", err)
	}

	return &wallet, nil
}
