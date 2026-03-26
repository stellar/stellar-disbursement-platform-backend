package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/stellar/go-stellar-sdk/protocols/horizon/base"
	"github.com/stellar/go-stellar-sdk/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

type Asset struct {
	ID        string     `json:"id" csv:"-" db:"id"`
	Code      string     `json:"code" db:"code"`
	Issuer    string     `json:"issuer" db:"issuer"`
	CreatedAt *time.Time `json:"created_at,omitempty" csv:"-" db:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty" csv:"-" db:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at" csv:"-" db:"deleted_at"`
}

func AssetColumnNames(tableReference, resultAlias string, includeDates bool) string {
	cols := []string{"id", "code"}
	if includeDates {
		cols = append(cols, "created_at", "updated_at", "deleted_at")
	}

	columns := SQLColumnConfig{
		TableReference:        tableReference,
		ResultAlias:           resultAlias,
		RawColumns:            cols,
		CoalesceStringColumns: []string{"issuer"},
	}.Build()

	return strings.Join(columns, ",\n")
}

// IsNative returns true if the asset is the native asset (XLM).
func (a Asset) IsNative() bool {
	return strings.TrimSpace(a.Issuer) == "" &&
		(a.Code == "XLM" || a.Code == "NATIVE")
}

// Equals returns true if the asset is the same as the other asset.
func (a Asset) Equals(other Asset) bool {
	if a.IsNative() && other.IsNative() {
		return true
	}
	return a.Code == other.Code && strings.EqualFold(a.Issuer, other.Issuer)
}

func (a Asset) EqualsHorizonAsset(horizonAsset base.Asset) bool {
	if a.IsNative() && horizonAsset.Type == "native" {
		return true
	}

	return a.Code == horizonAsset.Code && strings.EqualFold(a.Issuer, horizonAsset.Issuer)
}

func (a Asset) ToBasicAsset() txnbuild.Asset {
	if a.IsNative() {
		return txnbuild.NativeAsset{}
	}
	return txnbuild.CreditAsset{Code: a.Code, Issuer: a.Issuer}
}

type AssetModel struct {
	dbConnectionPool db.DBConnectionPool
}

func (a *AssetModel) Get(ctx context.Context, id string) (*Asset, error) {
	var asset Asset
	query := `
		SELECT
		    *
		FROM
		    assets a
		WHERE
		    a.id = $1
		`

	err := a.dbConnectionPool.GetContext(ctx, &asset, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("error querying asset ID %s: %w", id, err)
	}
	return &asset, nil
}

// GetByCodeAndIssuer returns asset filtering by code and issuer.
func (a *AssetModel) GetByCodeAndIssuer(ctx context.Context, code, issuer string) (*Asset, error) {
	var asset Asset
	query := `
		SELECT
		    *
		FROM
		    assets a
		WHERE a.code = $1
		AND a.issuer = $2
		`

	err := a.dbConnectionPool.GetContext(ctx, &asset, query, code, issuer)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("error querying asset with code %s and issuer %s: %w", code, issuer, err)
	}
	return &asset, nil
}

// ExistsByCodeOrID checks if an asset exists by either code or ID.
func (a *AssetModel) ExistsByCodeOrID(ctx context.Context, codeOrID string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM assets 
			WHERE (code = $1 OR id = $1) 
			AND deleted_at IS NULL
		)
	`
	var exists bool
	err := a.dbConnectionPool.GetContext(ctx, &exists, query, codeOrID)
	if err != nil {
		return false, fmt.Errorf("checking asset existence for '%s': %w", codeOrID, err)
	}
	return exists, nil
}

// GetByWalletID returns all assets associated with a wallet.
func (a *AssetModel) GetByWalletID(ctx context.Context, walletID string) ([]Asset, error) {
	assets := []Asset{}
	query := `
		SELECT
		    a.*
		FROM
		    assets a
		JOIN
		    wallets_assets wa ON a.id = wa.asset_id
		WHERE
		    deleted_at IS NULL
		    AND wa.wallet_id = $1
		ORDER BY
		    a.code ASC
	`

	err := a.dbConnectionPool.SelectContext(ctx, &assets, query, walletID)
	if err != nil {
		return nil, fmt.Errorf("selecting assets by wallet ID %s: %w", walletID, err)
	}
	return assets, nil
}

// GetAll returns all assets in the database.
func (a *AssetModel) GetAll(ctx context.Context) ([]Asset, error) {
	assets := []Asset{}
	query := `
		SELECT
			*
		FROM
			assets
		WHERE
		    deleted_at IS NULL
		ORDER BY
			code ASC
	`

	err := a.dbConnectionPool.SelectContext(ctx, &assets, query)
	if err != nil {
		return nil, fmt.Errorf("selecting assets: %w", err)
	}
	return assets, nil
}

// Insert is idempotent and returns a new asset if it doesn't exist or the existing one if it does, clearing the
// deleted_at field if it was marked as deleted.
func (a *AssetModel) Insert(ctx context.Context, sqlExec db.SQLExecuter, code string, issuer string) (*Asset, error) {
	const query = `
		WITH upsert_asset AS (
			INSERT INTO assets
				(code, issuer)
			VALUES
				($1, $2)
			ON CONFLICT (code, issuer) DO UPDATE
				SET deleted_at = NULL WHERE assets.deleted_at IS NOT NULL
			RETURNING *
		)
		SELECT * FROM upsert_asset
		UNION ALL  -- // The UNION statement is applied to prevent the updated_at field from being autoupdated when the asset already exists.
		SELECT * FROM assets WHERE code = $1 AND issuer = $2 AND NOT EXISTS (SELECT 1 FROM upsert_asset);
	`

	var asset Asset
	err := sqlExec.GetContext(ctx, &asset, query, code, issuer)
	if err != nil {
		return nil, fmt.Errorf("error inserting asset: %w", err)
	}

	return &asset, nil
}

func (a *AssetModel) GetOrCreate(ctx context.Context, code, issuer string) (*Asset, error) {
	const query = `
	WITH create_asset AS(
		INSERT INTO assets
			(code, issuer)
		VALUES
			($1, $2)
		ON CONFLICT (code, issuer) DO NOTHING
		RETURNING *
	)
	SELECT * FROM create_asset ca
	UNION ALL
	SELECT * FROM assets a
	WHERE a.code = $1
	AND a.issuer = $2
	`

	var asset Asset
	err := a.dbConnectionPool.GetContext(ctx, &asset, query, code, issuer)
	if err != nil {
		return nil, fmt.Errorf("error getting or creating asset: %w", err)
	}

	return &asset, nil
}

func (a *AssetModel) SoftDelete(ctx context.Context, sqlExec db.SQLExecuter, id string) (*Asset, error) {
	query := `
	UPDATE
		assets
	SET
		deleted_at = NOW()
 	WHERE id = $1
	RETURNING *
	`

	var asset Asset
	err := sqlExec.GetContext(ctx, &asset, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("error soft deleting asset ID %s: %w", id, err)
	}
	return &asset, nil
}

type ReceiverWalletAsset struct {
	WalletID                                    string           `db:"wallet_id"`
	ReceiverWallet                              ReceiverWallet   `db:"receiver_wallet"`
	Asset                                       Asset            `db:"asset"`
	DisbursementReceiverRegistrationMsgTemplate *string          `json:"-" db:"receiver_registration_message_template"`
	VerificationField                           VerificationType `db:"verification_field"`
}

// GetAssetsPerReceiverWallet returns the assets associated with a READY payment for each receiver
// wallet provided.
func (a *AssetModel) GetAssetsPerReceiverWallet(ctx context.Context, receiverWallets ...*ReceiverWallet) ([]ReceiverWalletAsset, error) {
	receiverWalletIDs := make([]string, len(receiverWallets))
	for i, rw := range receiverWallets {
		receiverWalletIDs[i] = rw.ID
	}

	var receiverWalletsAssets []ReceiverWalletAsset
	query := `
		WITH latest_payments_by_wallet AS (
			-- Gets the latest payment by wallet with its asset
			SELECT
				p.id AS payment_id,
				rw.wallet_id,
				COALESCE(d.receiver_registration_message_template, '') as receiver_registration_message_template,
				COALESCE(d.verification_field::text, '') as verification_field,
				p.asset_id
			FROM
				payments p
				INNER JOIN receiver_wallets rw ON rw.id = p.receiver_wallet_id
				INNER JOIN assets a ON a.id = p.asset_id
				LEFT JOIN disbursements d ON (p.type = 'DISBURSEMENT' AND d.id = p.disbursement_id)
			WHERE
				p.status = $1
			GROUP BY
				p.id, p.asset_id, rw.wallet_id, d.receiver_registration_message_template, d.verification_field
			ORDER BY
				p.updated_at DESC
		), messages_resent_since_invitation AS (
			-- Gets the number of attempts we resent the invitation message to the receiver by wallet with its asset.
			SELECT
				m.receiver_wallet_id,
				m.wallet_id,
				m.asset_id,
				COUNT(*) AS total_invitation_resent_attempts
			FROM
				messages m
				INNER JOIN receiver_wallets rw ON rw.id = m.receiver_wallet_id AND rw.wallet_id = m.wallet_id
			WHERE
				rw.id = ANY($2)
				AND rw.invitation_sent_at IS NOT NULL
				AND m.created_at > rw.invitation_sent_at
				AND m.status = 'SUCCESS'::message_status
			GROUP BY
				m.receiver_wallet_id,
				m.wallet_id,
				m.asset_id
		)
		SELECT DISTINCT
			lpw.wallet_id,
			lpw.receiver_registration_message_template,
			lpw.verification_field,
			rw.id AS "receiver_wallet.id",
			rw.invitation_sent_at AS "receiver_wallet.invitation_sent_at",
			COALESCE(mrsi.total_invitation_resent_attempts, 0) AS "receiver_wallet.total_invitation_resent_attempts",
			r.id AS "receiver_wallet.receiver.id",
			COALESCE(r.phone_number, '') AS "receiver_wallet.receiver.phone_number",
			COALESCE(r.email, '') AS "receiver_wallet.receiver.email",
			` + AssetColumnNames("a", "asset", true) + `
		FROM
			assets a
			INNER JOIN latest_payments_by_wallet lpw ON lpw.asset_id = a.id
			INNER JOIN payments p ON p.id = lpw.payment_id
			INNER JOIN receiver_wallets rw ON rw.id = p.receiver_wallet_id
			INNER JOIN receivers r ON r.id = rw.receiver_id
			LEFT JOIN messages_resent_since_invitation mrsi ON rw.id = mrsi.receiver_wallet_id AND rw.wallet_id = mrsi.wallet_id AND a.id = mrsi.asset_id
		WHERE
			rw.id = ANY($2)
	`

	err := a.dbConnectionPool.SelectContext(ctx, &receiverWalletsAssets, query, ReadyPaymentStatus, pq.Array(receiverWalletIDs))
	if err != nil {
		return nil, fmt.Errorf("error querying most recent asset per receiver wallet: %w", err)
	}

	return receiverWalletsAssets, nil
}
