package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
)

type Wallet struct {
	ID                string     `json:"id" db:"id"`
	Name              string     `json:"name" db:"name"`
	Homepage          string     `json:"homepage,omitempty" db:"homepage"`
	SEP10ClientDomain string     `json:"sep_10_client_domain,omitempty" db:"sep_10_client_domain"`
	DeepLinkSchema    string     `json:"deep_link_schema,omitempty" db:"deep_link_schema"`
	CreatedAt         *time.Time `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt         *time.Time `json:"updated_at,omitempty" db:"updated_at"`
	DeletedAt         *time.Time `json:"-" db:"deleted_at"`
}

type WalletModel struct {
	dbConnectionPool db.DBConnectionPool
}

func (w *WalletModel) Get(ctx context.Context, id string) (*Wallet, error) {
	var wallet Wallet
	query := `
		SELECT 
		    w.id, 
		    w.name, 
		    w.homepage,
		    w.sep_10_client_domain,
		    w.deep_link_schema,
		    w.created_at,
		    w.updated_at
		FROM 
		    wallets w
		WHERE 
		    w.id = $1
	`

	err := w.dbConnectionPool.GetContext(ctx, &wallet, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("error querying wallet ID %s: %w", id, err)
	}
	return &wallet, nil
}

// GetByWalletName returns wallet filtering by wallet name.
func (w *WalletModel) GetByWalletName(ctx context.Context, name string) (*Wallet, error) {
	var wallet Wallet
	query := `
		SELECT 
		    w.id, 
		    w.name, 
		    w.homepage,
		    w.sep_10_client_domain,
		    w.deep_link_schema,
		    w.created_at,
		    w.updated_at
		FROM 
		    wallets w
		WHERE 
		    w.name = $1
	`

	err := w.dbConnectionPool.GetContext(ctx, &wallet, query, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("error querying wallet with name %s: %w", name, err)
	}
	return &wallet, nil
}

// GetAll returns all wallets in the database
func (w *WalletModel) GetAll(ctx context.Context) ([]Wallet, error) {
	wallets := []Wallet{}
	query := `
		SELECT 
		    w.id, 
		    w.name, 
		    w.homepage,
			w.sep_10_client_domain,
		    w.deep_link_schema,
		    w.created_at,
		    w.updated_at
		FROM 
		    wallets w
		ORDER BY
			name
	`

	err := w.dbConnectionPool.SelectContext(ctx, &wallets, query)
	if err != nil {
		return nil, fmt.Errorf("error querying wallets: %w", err)
	}
	return wallets, nil
}

func (w *WalletModel) Insert(ctx context.Context, name string, homepage string, deepLink string, sep10Domain string) (*Wallet, error) {
	const query = `
	INSERT INTO wallets
		(name, homepage, deep_link_schema, sep_10_client_domain)
	VALUES
		($1, $2, $3, $4)
 	RETURNING
		id, 
		name, 
		homepage,
		sep_10_client_domain,
		deep_link_schema,
		created_at,
		updated_at
	`

	var wallet Wallet
	err := w.dbConnectionPool.GetContext(ctx, &wallet, query, name, homepage, deepLink, sep10Domain)
	if err != nil {
		return nil, fmt.Errorf("error inserting wallet: %w", err)
	}

	return &wallet, nil
}

func (w *WalletModel) GetOrCreate(ctx context.Context, name, homepage, deepLink, sep10Domain string) (*Wallet, error) {
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
	err := w.dbConnectionPool.GetContext(ctx, &wallet, query, name, homepage, deepLink, sep10Domain)
	if err != nil {
		return nil, fmt.Errorf("error getting or creating wallet: %w", err)
	}

	return &wallet, nil
}
