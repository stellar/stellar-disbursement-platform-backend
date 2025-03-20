package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

var (
	ErrWalletNameAlreadyExists           = errors.New("a wallet with this name already exists")
	ErrWalletHomepageAlreadyExists       = errors.New("a wallet with this homepage already exists")
	ErrWalletDeepLinkSchemaAlreadyExists = errors.New("a wallet with this deep link schema already exists")
	ErrInvalidAssetID                    = errors.New("invalid asset ID")
)

type Wallet struct {
	ID                string       `json:"id" csv:"-" db:"id"`
	Name              string       `json:"name" db:"name"`
	Homepage          string       `json:"homepage,omitempty" csv:"-" db:"homepage"`
	SEP10ClientDomain string       `json:"sep_10_client_domain,omitempty" csv:"-" db:"sep_10_client_domain"`
	DeepLinkSchema    string       `json:"deep_link_schema,omitempty" csv:"-" db:"deep_link_schema"`
	Enabled           bool         `json:"enabled" csv:"-" db:"enabled"`
	UserManaged       bool         `json:"user_managed,omitempty" csv:"-" db:"user_managed"`
	Assets            WalletAssets `json:"assets,omitempty" csv:"-" db:"assets"`
	CreatedAt         *time.Time   `json:"created_at,omitempty" csv:"-" db:"created_at"`
	UpdatedAt         *time.Time   `json:"updated_at,omitempty" csv:"-" db:"updated_at"`
	DeletedAt         *time.Time   `json:"-" csv:"-" db:"deleted_at"`
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

// WalletColumnNames returns a comma-separated string of wallet column names for SQL queries. It includes optional date
// fields based on the provided parameter.
func WalletColumnNames(tableReference, resultAlias string, includeDates bool) string {
	colNames := []string{
		"id",
		"name",
		"sep_10_client_domain",
		"homepage",
		"enabled",
		"deep_link_schema",
		"user_managed",
	}
	if includeDates {
		colNames = append(colNames, "created_at", "updated_at", "deleted_at")
	}

	columns := SQLColumnConfig{
		TableReference: tableReference,
		ResultAlias:    resultAlias,
		RawColumns:     colNames,
	}.Build()

	return strings.Join(columns, ",\n")
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
	`

func (wm *WalletModel) Get(ctx context.Context, id string) (*Wallet, error) {
	var wallet Wallet
	query := fmt.Sprintf("%s %s", getQuery, `WHERE w.id = $1 GROUP BY w.id`)

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
	query := fmt.Sprintf("%s %s", getQuery, `WHERE w.name = $1 GROUP BY w.id`)

	err := wm.dbConnectionPool.GetContext(ctx, &wallet, query, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("error querying wallet with name %s: %w", name, err)
	}
	return &wallet, nil
}

const (
	FilterEnabledWallets FilterKey = "enabled"
	FilterUserManaged    FilterKey = "user_managed"
)

// FindWallets returns wallets filtering by enabled status.
func (wm *WalletModel) FindWallets(ctx context.Context, filters ...Filter) ([]Wallet, error) {
	qb := NewQueryBuilder(getQuery)
	for _, filter := range filters {
		qb.AddCondition(filter.Key.Equals(), filter.Value)
	}
	qb.AddGroupBy("w.id")
	qb.AddSorting(SortFieldName, SortOrderASC, "w")
	query, args := qb.BuildAndRebind(wm.dbConnectionPool)

	wallets := []Wallet{}
	err := wm.dbConnectionPool.SelectContext(ctx, &wallets, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying wallets: %w", err)
	}
	return wallets, nil
}

// GetAll returns all wallets in the database
func (wm *WalletModel) GetAll(ctx context.Context) ([]Wallet, error) {
	return wm.FindWallets(ctx)
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
					"wallets_assets_asset_id_fkey": ErrInvalidAssetID,
					"wallets_name_key":             ErrWalletNameAlreadyExists,
					"wallets_homepage_key":         ErrWalletHomepageAlreadyExists,
					"wallets_deep_link_schema_key": ErrWalletDeepLinkSchemaAlreadyExists,
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
			*
	)
	SELECT * FROM create_wallet cw
	UNION ALL
	SELECT
		*
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

func (w *WalletModel) SoftDelete(ctx context.Context, walletID string) (*Wallet, error) {
	const query = `
		UPDATE
			wallets
		SET
			deleted_at = NOW()
		WHERE
			id = $1
			AND deleted_at IS NULL
		RETURNING *
	`

	var wallet Wallet
	err := w.dbConnectionPool.GetContext(ctx, &wallet, query, walletID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("soft deleting wallet ID %s: %w", walletID, err)
	}

	return &wallet, nil
}

func (wm *WalletModel) Update(ctx context.Context, walletID string, enabled bool) (*Wallet, error) {
	const query = `
		UPDATE
			wallets
		SET
			enabled = $1
		WHERE
			id = $2
		RETURNING *
	`
	var wallet Wallet
	err := wm.dbConnectionPool.GetContext(ctx, &wallet, query, enabled, walletID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("updating wallet enabled status: %w", err)
	}

	return &wallet, nil
}
