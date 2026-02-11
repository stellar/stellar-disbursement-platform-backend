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
	ErrWalletInUse                       = errors.New("wallet has pending registrations and cannot be deleted")
)

type Wallet struct {
	ID                string       `json:"id" csv:"-" db:"id"`
	Name              string       `json:"name" db:"name"`
	Homepage          string       `json:"homepage,omitempty" csv:"-" db:"homepage"`
	SEP10ClientDomain string       `json:"sep_10_client_domain,omitempty" csv:"-" db:"sep_10_client_domain"`
	DeepLinkSchema    string       `json:"deep_link_schema,omitempty" csv:"-" db:"deep_link_schema"`
	Enabled           bool         `json:"enabled" csv:"-" db:"enabled"`
	UserManaged       bool         `json:"user_managed,omitempty" csv:"-" db:"user_managed"`
	Embedded          bool         `json:"embedded,omitempty" csv:"-" db:"embedded"`
	Assets            WalletAssets `json:"assets,omitempty" csv:"-" db:"assets"`
	CreatedAt         *time.Time   `json:"created_at,omitempty" csv:"-" db:"created_at"`
	UpdatedAt         *time.Time   `json:"updated_at,omitempty" csv:"-" db:"updated_at"`
	DeletedAt         *time.Time   `json:"-" csv:"-" db:"deleted_at"`
}

func (w *Wallet) IsSelfHosted() bool {
	return w.DeepLinkSchema == "SELF"
}

type WalletInsert struct {
	Name              string   `db:"name"`
	Homepage          string   `db:"homepage"`
	SEP10ClientDomain string   `db:"sep_10_client_domain"`
	DeepLinkSchema    string   `db:"deep_link_schema"`
	Enabled           bool     `db:"enabled"`
	AssetsIDs         []string `db:"assets_ids"`
}

type WalletUpdate struct {
	Name              *string   `db:"name"`
	Homepage          *string   `db:"homepage"`
	SEP10ClientDomain *string   `db:"sep_10_client_domain"`
	DeepLinkSchema    *string   `db:"deep_link_schema"`
	Enabled           *bool     `db:"enabled"`
	AssetsIDs         *[]string `db:"assets_ids"`
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
		"embedded",
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
	FilterEnabledWallets  FilterKey = "enabled"
	FilterUserManaged     FilterKey = "user_managed"
	FilterSupportedAssets FilterKey = "supported_assets"
	FilterIncludeDeleted  FilterKey = "include_deleted"
	FilterEmbedded        FilterKey = "embedded"
)

// FindWallets returns wallets filtering by enabled status.
func (wm *WalletModel) FindWallets(ctx context.Context, filters ...Filter) ([]Wallet, error) {
	query, args := newWalletQuery(getQuery, wm.dbConnectionPool, filters...)

	wallets := []Wallet{}
	err := wm.dbConnectionPool.SelectContext(ctx, &wallets, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying wallets: %w", err)
	}
	return wallets, nil
}

func newWalletQuery(baseQuery string, sqlExec db.SQLExecuter, filters ...Filter) (string, []any) {
	qb := NewQueryBuilder(baseQuery)
	includeDeleted := false

	for _, filter := range filters {
		switch filter.Key {
		case FilterEnabledWallets:
			qb.AddCondition("w.enabled = ?", filter.Value)
		case FilterUserManaged:
			qb.AddCondition("w.user_managed = ?", filter.Value)
		case FilterEmbedded:
			qb.AddCondition("w.embedded = ?", filter.Value)
		case FilterSupportedAssets:
			if assets, ok := filter.Value.([]string); ok && len(assets) > 0 {
				// Filter wallets that support all specified assets
				assetCondition := `w.id IN (
					SELECT wa.wallet_id
					FROM wallets_assets wa
					JOIN assets a ON wa.asset_id = a.id
					WHERE a.code = ANY(?) OR a.id = ANY(?)
					GROUP BY wa.wallet_id
					HAVING COUNT(DISTINCT a.id) = ?
				)`
				qb.AddCondition(assetCondition, pq.Array(assets), pq.Array(assets), len(assets))
			}
		case FilterIncludeDeleted:
			if include, ok := filter.Value.(bool); ok {
				includeDeleted = include
			}
		default:
			qb.AddCondition(filter.Key.Equals(), filter.Value)
		}
	}

	// By default, exclude deleted wallets unless explicitly requested
	if !includeDeleted {
		qb.AddCondition("w.deleted_at IS NULL")
	}

	qb.AddGroupBy("w.id")
	qb.AddSorting(SortFieldName, SortOrderASC, "w")
	return qb.BuildAndRebind(sqlExec)
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
					(name, homepage, deep_link_schema, sep_10_client_domain, enabled)
				VALUES
					($1, $2, $3, $4, $5)
				RETURNING
					*
			), assets_cte AS (
				SELECT UNNEST($6::text[]) id
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
		if err := dbTx.GetContext(
			ctx, &w, query,
			newWallet.Name, newWallet.Homepage, newWallet.DeepLinkSchema, newWallet.SEP10ClientDomain, newWallet.Enabled,
			pq.Array(newWallet.AssetsIDs),
		); err != nil {
			var pqError *pq.Error
			if errors.As(err, &pqError) {
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
		return nil, fmt.Errorf("inserting wallet: %w", err)
	}

	return wallet, nil
}

func (wm *WalletModel) GetOrCreate(ctx context.Context, name, homepage, deepLink, sep10Domain string, embedded bool) (*Wallet, error) {
	const query = `
	WITH create_wallet AS(
		INSERT INTO wallets
			(name, homepage, deep_link_schema, sep_10_client_domain, embedded)
		VALUES
			($1, $2, $3, $4, $5)
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
	err := wm.dbConnectionPool.GetContext(ctx, &wallet, query, name, homepage, deepLink, sep10Domain, embedded)
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

// HasPendingReceiverWallets checks if a wallet has any receiver_wallets in DRAFT or READY status.
func (wm *WalletModel) HasPendingReceiverWallets(ctx context.Context, walletID string) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM receiver_wallets
			WHERE wallet_id = $1
			AND status IN ('DRAFT', 'READY')
		)
	`

	var exists bool
	err := wm.dbConnectionPool.GetContext(ctx, &exists, query, walletID)
	if err != nil {
		return false, fmt.Errorf("checking pending receiver wallets for wallet %s: %w", walletID, err)
	}

	return exists, nil
}

// SoftDelete marks a wallet as deleted if it has no pending receiver_wallets in DRAFT or READY status.
func (wm *WalletModel) SoftDelete(ctx context.Context, walletID string) (*Wallet, error) {
	const query = `
		WITH pending_check AS (
			SELECT EXISTS (
				SELECT 1
				FROM receiver_wallets
				WHERE wallet_id = $1
				AND status IN ('DRAFT', 'READY')
			) AS has_pending
		)
		UPDATE wallets
		SET deleted_at = NOW()
		WHERE id = $1
			AND deleted_at IS NULL
			AND NOT (SELECT has_pending FROM pending_check)
		RETURNING *
	`

	var wallet Wallet
	err := wm.dbConnectionPool.GetContext(ctx, &wallet, query, walletID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Check if wallet exists but has pending receiver_wallets, or if it simply doesn't exist
			hasPending, checkErr := wm.HasPendingReceiverWallets(ctx, walletID)
			if checkErr != nil {
				return nil, fmt.Errorf("checking wallet usage after failed delete: %w", checkErr)
			}
			if hasPending {
				return nil, ErrWalletInUse
			}
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("soft deleting wallet ID %s: %w", walletID, err)
	}

	return &wallet, nil
}

func (wm *WalletModel) Update(ctx context.Context, walletID string, update WalletUpdate) (*Wallet, error) {
	wallet, err := db.RunInTransactionWithResult(ctx, wm.dbConnectionPool, nil, func(dbTx db.DBTransaction) (*Wallet, error) {
		var setClauses []string
		var args []any

		if update.Name != nil {
			setClauses = append(setClauses, "name = ?")
			args = append(args, *update.Name)
		}
		if update.Homepage != nil {
			setClauses = append(setClauses, "homepage = ?")
			args = append(args, *update.Homepage)
		}
		if update.SEP10ClientDomain != nil {
			setClauses = append(setClauses, "sep_10_client_domain = ?")
			args = append(args, *update.SEP10ClientDomain)
		}
		if update.DeepLinkSchema != nil {
			setClauses = append(setClauses, "deep_link_schema = ?")
			args = append(args, *update.DeepLinkSchema)
		}
		if update.Enabled != nil {
			setClauses = append(setClauses, "enabled = ?")
			args = append(args, *update.Enabled)
		}

		if len(setClauses) == 0 && update.AssetsIDs == nil {
			return nil, fmt.Errorf("no fields provided for update")
		}

		var w Wallet
		if len(setClauses) > 0 {
			setClauses = append(setClauses, "updated_at = NOW()")
			query := dbTx.Rebind(fmt.Sprintf(`
				UPDATE wallets 
				SET %s
				WHERE id = ? AND deleted_at IS NULL
				RETURNING *
			`, strings.Join(setClauses, ", ")))

			args = append(args, walletID)

			if err := dbTx.GetContext(ctx, &w, query, args...); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return nil, ErrRecordNotFound
				}
				var pqError *pq.Error
				if errors.As(err, &pqError) {
					constraintErrMap := map[string]error{
						"wallets_name_key":             ErrWalletNameAlreadyExists,
						"wallets_homepage_key":         ErrWalletHomepageAlreadyExists,
						"wallets_deep_link_schema_key": ErrWalletDeepLinkSchemaAlreadyExists,
					}

					if mappedErr, ok := constraintErrMap[pqError.Constraint]; ok {
						return nil, mappedErr
					}
				}
				return nil, fmt.Errorf("updating wallet: %w", err)
			}
		} else {
			const q = "SELECT * FROM wallets WHERE id = $1 AND deleted_at IS NULL"
			if err := dbTx.GetContext(ctx, &w, q, walletID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return nil, ErrRecordNotFound
				}
				return nil, fmt.Errorf("getting wallet: %w", err)
			}
		}

		if update.AssetsIDs != nil {
			if _, err := dbTx.ExecContext(ctx, "DELETE FROM wallets_assets WHERE wallet_id = $1", walletID); err != nil {
				return nil, fmt.Errorf("deleting existing wallet assets: %w", err)
			}

			if len(*update.AssetsIDs) > 0 {
				const q = `
					INSERT INTO wallets_assets (wallet_id, asset_id)
					SELECT $1, UNNEST($2::text[])
					ON CONFLICT DO NOTHING
				`
				if _, err := dbTx.ExecContext(ctx, q, walletID, pq.Array(*update.AssetsIDs)); err != nil {
					var pqError *pq.Error
					if errors.As(err, &pqError) && pqError.Constraint == "wallets_assets_asset_id_fkey" {
						return nil, ErrInvalidAssetID
					}
					return nil, fmt.Errorf("inserting new wallet assets: %w", err)
				}
			}
		}

		return &w, nil
	})
	if err != nil {
		return nil, fmt.Errorf("updating wallet: %w", err)
	}

	return wm.Get(ctx, wallet.ID)
}
