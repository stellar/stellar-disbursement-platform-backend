package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
)

type Country struct {
	Code      string     `json:"code" db:"code"`
	Name      string     `json:"name" db:"name"`
	CreatedAt time.Time  `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at,omitempty" db:"updated_at"`
	DeletedAt *time.Time `json:"-" db:"deleted_at"`
}

type CountryModel struct {
	dbConnectionPool db.DBConnectionPool
}

func (m *CountryModel) Get(ctx context.Context, code string) (*Country, error) {
	var country Country
	query := `
		SELECT 
		    c.code, 
		    c.name,
		    c.created_at,
		    c.updated_at
		FROM 
		    countries c
		WHERE 
		    c.code = $1
		    `

	err := m.dbConnectionPool.GetContext(ctx, &country, query, code)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("error querying country code %s: %w", code, err)
	}
	return &country, nil
}

// GetAll returns all countries in the database
func (m *CountryModel) GetAll(ctx context.Context) ([]Country, error) {
	countries := []Country{}
	query := `
		SELECT 
		    c.code, 
		    c.name,
		    c.created_at,
		    c.updated_at
		FROM 
		    countries c
		ORDER BY
			c.name ASC
		`

	err := m.dbConnectionPool.SelectContext(ctx, &countries, query)
	if err != nil {
		return nil, fmt.Errorf("error querying countries: %w", err)
	}
	return countries, nil
}
