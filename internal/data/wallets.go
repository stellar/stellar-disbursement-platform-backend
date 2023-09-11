package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
)

var (
	ErrWalletNameAlreadyExists           = errors.New("a wallet with this name already exists")
	ErrWalletHomepageAlreadyExists       = errors.New("a wallet with this homepage already exists")
	ErrWalletDeepLinkSchemaAlreadyExists = errors.New("a wallet with this deep link schema already exists")
	ErrInvalidAssetID                    = errors.New("invalid asset ID")
)

type Wallet struct {
	ID                string       `json:"id" db:"id"`
	Name              string       `json:"name" db:"name"`
	Homepage          string       `json:"homepage,omitempty" db:"homepage"`
	SEP10ClientDomain string       `json:"sep_10_client_domain,omitempty" db:"sep_10_client_domain"`
	DeepLinkSchema    string       `json:"deep_link_schema,omitempty" db:"deep_link_schema"`
	Assets            WalletAssets `json:"assets,omitempty" db:"assets"`
	CreatedAt         *time.Time   `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt         *time.Time   `json:"updated_at,omitempty" db:"updated_at"`
	DeletedAt         *time.Time   `json:"-" db:"deleted_at"`
}

type WalletInsert struct {
	Name              string   `db:"name"`
	Homepage          string   `db:"homepage"`
	SEP10ClientDomain string   `db:"sep_10_client_domain"`
	DeepLinkSchema    string   `db:"deep_link_schema"`
	AssetsIDs         []string `db:"assets_ids"`
}

type WalletAssets []Asset

var _ sql.Scanner = (*WalletAssets)(nil)

func (wa *WalletAssets) Scan(src interface{}) error {
	if src == nil {
		*wa = make(WalletAssets, 0)
		return nil
	}

	data, ok := src.([]byte)
	if !ok {
		return fmt.Errorf("could not parse assets")
	}
	return json.Unmarshal(data, wa)
}

type WalletModel struct {
	dbConnectionPool db.DBConnectionPool
}

const getQuery = `
		SELECT 
		    w.*, 
			jsonb_agg(
				DISTINCT to_jsonb(a)
			) FILTER (WHERE a.id IS NOT NULL) AS assets
		FROM 
		    wallets w
			LEFT JOIN wallets_assets wa ON w.id = wa.wallet_id
			LEFT JOIN assets a ON a.id = wa.asset_id
		%s
	`

func (wm *WalletModel) Get(ctx context.Context, id string) (*Wallet, error) {
	var wallet Wallet
	query := fmt.Sprintf(getQuery, `WHERE w.id = $1 GROUP BY w.id`)

	err := wm.dbConnectionPool.GetContext(ctx, &wallet, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("error querying wallet ID %s: %w", id, err)
	}
	return &wallet, nil
}

// GetByWalletName returns wallet filtering by wallet name.
func (wm *WalletModel) GetByWalletName(ctx context.Context, name string) (*Wallet, error) {
	var wallet Wallet
	query := fmt.Sprintf(getQuery, `WHERE w.name = $1 GROUP BY w.id`)

	err := wm.dbConnectionPool.GetContext(ctx, &wallet, query, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("error querying wallet with name %s: %w", name, err)
	}
	return &wallet, nil
}

// GetAll returns all wallets in the database
func (wm *WalletModel) GetAll(ctx context.Context) ([]Wallet, error) {
	wallets := []Wallet{}
	query := fmt.Sprintf(getQuery, `GROUP BY w.id ORDER BY w.name`)

	err := wm.dbConnectionPool.SelectContext(ctx, &wallets, query)
	if err != nil {
		return nil, fmt.Errorf("error querying wallets: %w", err)
	}
	return wallets, nil
}

func (wm *WalletModel) Insert(ctx context.Context, newWallet WalletInsert) (*Wallet, error) {
	wallet, err := db.RunInTransactionWithResult(ctx, wm.dbConnectionPool, nil, func(dbTx db.DBTransaction) (*Wallet, error) {
		const query = `
			WITH new_wallet AS (
				INSERT INTO wallets
					(name, homepage, deep_link_schema, sep_10_client_domain)
				VALUES
					($1, $2, $3, $4)
				RETURNING
					*
			), assets_cte AS (
				SELECT UNNEST($5::text[]) id
			), new_wallet_assets AS (
				INSERT INTO wallets_assets
					(wallet_id, asset_id)
				SELECT
					w.id, a.id
				FROM
					new_wallet w, assets_cte a
				ON CONFLICT DO NOTHING
			)
			SELECT * FROM new_wallet
		`

		var w Wallet
		err := dbTx.GetContext(
			ctx, &w, query,
			newWallet.Name, newWallet.Homepage, newWallet.DeepLinkSchema, newWallet.SEP10ClientDomain,
			pq.Array(newWallet.AssetsIDs),
		)
		if err != nil {
			if pqError, ok := err.(*pq.Error); ok {
				constraintErrMap := map[string]error{
					"wallets_name_key":             ErrWalletNameAlreadyExists,
					"wallets_homepage_key":         ErrWalletHomepageAlreadyExists,
					"wallets_deep_link_schema_key": ErrWalletDeepLinkSchemaAlreadyExists,
					"wallets_assets_asset_id_fkey": ErrInvalidAssetID,
				}

				errConstraint, ok := constraintErrMap[pqError.Constraint]
				if ok {
					return nil, errConstraint
				}
			}

			return nil, fmt.Errorf("inserting wallet: %w", err)
		}

		return &w, nil
	})
	if err != nil {
		return nil, err
	}

	return wallet, nil
}

func (wm *WalletModel) GetOrCreate(ctx context.Context, name, homepage, deepLink, sep10Domain string) (*Wallet, error) {
	const query = `
	WITH create_wallet AS(
		INSERT INTO wallets
			(name, homepage, deep_link_schema, sep_10_client_domain)
		VALUES
			($1, $2, $3, $4)
		ON CONFLICT (name, homepage, deep_link_schema) DO NOTHING
		RETURNING
			id, 
			name, 
			homepage,
			sep_10_client_domain,
			deep_link_schema,
			created_at,
			updated_at
	)
	SELECT * FROM create_wallet cw
	UNION ALL
	SELECT
		id, 
		name, 
		homepage,
		sep_10_client_domain,
		deep_link_schema,
		created_at,
		updated_at
	FROM wallets w
	WHERE w.name = $1
	`

	var wallet Wallet
	err := wm.dbConnectionPool.GetContext(ctx, &wallet, query, name, homepage, deepLink, sep10Domain)
	if err != nil {
		return nil, fmt.Errorf("error getting or creating wallet: %w", err)
	}

	return &wallet, nil
}

func (wm *WalletModel) GetAssets(ctx context.Context, walletID string) ([]Asset, error) {
	const query = `
		SELECT
			a.*
		FROM
			wallets_assets wa
			INNER JOIN assets a ON a.id = wa.asset_id
		WHERE
			wa.wallet_id = $1
		ORDER BY
			code
	`

	assets := make([]Asset, 0)
	err := wm.dbConnectionPool.SelectContext(ctx, &assets, query, walletID)
	if err != nil {
		return nil, fmt.Errorf("querying wallet assets: %w", err)
	}

	return assets, nil
}
