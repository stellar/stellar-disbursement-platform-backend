package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

// WalletMembership grants one user one role on one distribution wallet. Owner is always
// tenant-wide and never holds membership rows; every other role is wallet-scoped. Role
// semantics are unchanged by membership — it is an additional scoping dimension.
type WalletMembership struct {
	ID        string    `json:"id" db:"id"`
	UserID    string    `json:"user_id" db:"user_id"`
	WalletID  string    `json:"wallet_id" db:"wallet_id"`
	Role      UserRole  `json:"role" db:"role"`
	GrantedBy *string   `json:"granted_by,omitempty" db:"granted_by"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type WalletMembershipModel struct {
	dbConnectionPool db.DBConnectionPool
}

const walletMembershipColumns = `id, user_id, wallet_id, role, granted_by, created_at, updated_at`

// ErrWalletArchivedForMembership is returned when granting membership on an archived wallet;
// the API layer maps it to 409 Conflict.
var ErrWalletArchivedForMembership = errors.New("cannot grant membership on an archived wallet")

// Insert grants user the role on the wallet. Granting on an archived wallet fails (409 at the
// API layer); duplicate grants map to ErrRecordAlreadyExists.
func (m *WalletMembershipModel) Insert(ctx context.Context, sqlExec db.SQLExecuter, userID, walletID string, role UserRole, grantedBy *string) (*WalletMembership, error) {
	if userID == "" || walletID == "" || role == "" {
		return nil, fmt.Errorf("user ID, wallet ID, and role are required: %w", ErrMissingInput)
	}
	if role == OwnerUserRole {
		return nil, fmt.Errorf("the owner role is always tenant-wide and cannot be wallet-scoped: %w", ErrMissingInput)
	}

	query := fmt.Sprintf(`
		INSERT INTO wallet_memberships (user_id, wallet_id, role, granted_by)
		VALUES ($1, $2, $3, $4)
		RETURNING %s
	`, walletMembershipColumns)

	var membership WalletMembership
	err := sqlExec.GetContext(ctx, &membership, query, userID, walletID, role, grantedBy)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) {
			switch {
			case pqErr.Constraint == "wallet_memberships_wallet_must_be_active":
				return nil, fmt.Errorf("granting membership on wallet %q: %w", walletID, ErrWalletArchivedForMembership)
			case pqErr.Code == "23505":
				return nil, fmt.Errorf("user %q already has role %q on wallet %q: %w", userID, role, walletID, ErrRecordAlreadyExists)
			}
		}
		return nil, fmt.Errorf("granting membership (user %q, wallet %q, role %q): %w", userID, walletID, role, err)
	}

	return &membership, nil
}

// Delete revokes one membership. The membership row is removed; its full history remains in
// the append-only audit table.
func (m *WalletMembershipModel) Delete(ctx context.Context, sqlExec db.SQLExecuter, id string) error {
	res, err := sqlExec.ExecContext(ctx, `DELETE FROM wallet_memberships WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("revoking membership %q: %w", id, err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reading rows affected for membership %q: %w", id, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("revoking membership %q: %w", id, ErrRecordNotFound)
	}
	return nil
}

// GetWalletIDsForUser returns the distinct wallet IDs where the user holds any membership —
// the read-filter input for membership-filtered endpoints (taxonomy). Archived wallets
// are included: memberships persist through archival for historical authorization.
func (m *WalletMembershipModel) GetWalletIDsForUser(ctx context.Context, sqlExec db.SQLExecuter, userID string) ([]string, error) {
	walletIDs := []string{}
	err := sqlExec.SelectContext(ctx, &walletIDs,
		`SELECT DISTINCT wallet_id FROM wallet_memberships WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("listing wallet IDs for user %q: %w", userID, err)
	}
	return walletIDs, nil
}

// HasRoleOnWallet reports whether the user holds any of the given roles on the wallet — the
// (role, wallet) action-authorization gate.
func (m *WalletMembershipModel) HasRoleOnWallet(ctx context.Context, sqlExec db.SQLExecuter, userID, walletID string, roles ...UserRole) (bool, error) {
	if len(roles) == 0 {
		return false, fmt.Errorf("at least one role is required: %w", ErrMissingInput)
	}
	roleStrings := make([]string, len(roles))
	for i, r := range roles {
		roleStrings[i] = string(r)
	}

	var exists bool
	err := sqlExec.GetContext(ctx, &exists, `
		SELECT EXISTS (
			SELECT 1 FROM wallet_memberships
			WHERE user_id = $1 AND wallet_id = $2 AND role = ANY($3)
		)`, userID, walletID, pq.Array(roleStrings))
	if err != nil {
		return false, fmt.Errorf("checking role on wallet (user %q, wallet %q): %w", userID, walletID, err)
	}
	return exists, nil
}

// ListByWallet returns the wallet's memberships ("who could approve on Wallet A as of now");
// historical as-of queries read the audit table.
func (m *WalletMembershipModel) ListByWallet(ctx context.Context, sqlExec db.SQLExecuter, walletID string) ([]WalletMembership, error) {
	memberships := []WalletMembership{}
	query := fmt.Sprintf(`SELECT %s FROM wallet_memberships WHERE wallet_id = $1 ORDER BY created_at ASC`, walletMembershipColumns)
	if err := sqlExec.SelectContext(ctx, &memberships, query, walletID); err != nil {
		return nil, fmt.Errorf("listing memberships for wallet %q: %w", walletID, err)
	}
	return memberships, nil
}

// ListByUser returns all of a user's memberships.
func (m *WalletMembershipModel) ListByUser(ctx context.Context, sqlExec db.SQLExecuter, userID string) ([]WalletMembership, error) {
	memberships := []WalletMembership{}
	query := fmt.Sprintf(`SELECT %s FROM wallet_memberships WHERE user_id = $1 ORDER BY created_at ASC`, walletMembershipColumns)
	if err := sqlExec.SelectContext(ctx, &memberships, query, userID); err != nil {
		return nil, fmt.Errorf("listing memberships for user %q: %w", userID, err)
	}
	return memberships, nil
}

// Get returns one membership by id.
func (m *WalletMembershipModel) Get(ctx context.Context, sqlExec db.SQLExecuter, id string) (*WalletMembership, error) {
	query := fmt.Sprintf(`SELECT %s FROM wallet_memberships WHERE id = $1`, walletMembershipColumns)

	var membership WalletMembership
	err := sqlExec.GetContext(ctx, &membership, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("getting membership %q: %w", id, ErrRecordNotFound)
		}
		return nil, fmt.Errorf("getting membership %q: %w", id, err)
	}
	return &membership, nil
}
