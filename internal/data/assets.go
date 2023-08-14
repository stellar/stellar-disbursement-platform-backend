package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
)

type Asset struct {
	ID        string     `json:"id" db:"id"`
	Code      string     `json:"code" db:"code"`
	Issuer    string     `json:"issuer" db:"issuer"`
	CreatedAt *time.Time `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty" db:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at" db:"deleted_at"`
}

type AssetModel struct {
	dbConnectionPool db.DBConnectionPool
}

func (a *AssetModel) Get(ctx context.Context, id string) (*Asset, error) {
	var asset Asset
	query := `
		SELECT 
		    a.id, 
		    a.code, 
		    a.issuer,
		    a.created_at,
		    a.updated_at,
		    a.deleted_at
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
		    a.id, 
		    a.code, 
		    a.issuer,
		    a.created_at,
		    a.updated_at,
		    a.deleted_at
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

// GetAll returns all assets in the database.
func (a *AssetModel) GetAll(ctx context.Context) ([]Asset, error) {
	// TODO: We will want to filter out "deleted" assets at some point
	assets := []Asset{}
	query := `
		SELECT 
			a.id, 
			a.code, 
			a.issuer,
			a.created_at,
			a.updated_at,
			a.deleted_at
		FROM 
			assets a
		ORDER BY 
			a.code ASC
	`

	err := a.dbConnectionPool.SelectContext(ctx, &assets, query)
	if err != nil {
		return nil, fmt.Errorf("error querying assets: %w", err)
	}
	return assets, nil
}

func (a *AssetModel) Insert(ctx context.Context, sqlExec db.SQLExecuter, code string, issuer string) (*Asset, error) {
	const query = `
		INSERT INTO assets
			(code, issuer)
		VALUES
			($1, $2)
		ON CONFLICT (code, issuer) DO
		UPDATE SET
			deleted_at = NULL
		WHERE
			assets.deleted_at IS NOT NULL
		RETURNING *
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
	WalletID       string         `db:"wallet_id"`
	ReceiverWallet ReceiverWallet `db:"receiver_wallet"`
	Asset          Asset          `db:"asset"`
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
				d.wallet_id,
				p.asset_id
			FROM
				payments p
				INNER JOIN disbursements d ON d.id = p.disbursement_id
				INNER JOIN assets a ON a.id = p.asset_id
			WHERE
				p.status = $1
			GROUP BY
				p.id, p.asset_id, d.wallet_id
			ORDER BY
				p.updated_at DESC
		)
		SELECT DISTINCT
			lpw.wallet_id,
			rw.id AS "receiver_wallet.id",
			r.id AS "receiver_wallet.receiver.id",
			r.phone_number AS "receiver_wallet.receiver.phone_number",
			r.email AS "receiver_wallet.receiver.email",
			a.id AS "asset.id",
			a.code AS "asset.code",
			a.issuer AS "asset.issuer",
			a.created_at AS "asset.created_at",
			a.updated_at AS "asset.updated_at"
		FROM
			assets a
			INNER JOIN latest_payments_by_wallet lpw ON lpw.asset_id = a.id
			INNER JOIN payments p ON p.id = lpw.payment_id
			INNER JOIN receiver_wallets rw ON rw.id = p.receiver_wallet_id
			INNER JOIN receivers r ON r.id = rw.receiver_id
		WHERE
			rw.id = ANY($2)
	`

	err := a.dbConnectionPool.SelectContext(ctx, &receiverWalletsAssets, query, ReadyPaymentStatus, pq.Array(receiverWalletIDs))
	if err != nil {
		return nil, fmt.Errorf("error querying most recent asset per receiver wallet: %w", err)
	}

	return receiverWalletsAssets, nil
}
