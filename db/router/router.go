package router

import (
	"fmt"
	"net/url"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

const (
	TSSSchemaName       string = "tss"
	SDPSchemaNamePrefix string = "sdp_"
)

// GetDSNForTSS returns the database DSN for the TSS schema. It is basically the same as the root database DSN, but
// with the `search_path` query parameter (AKA schema) set to `tss`.
func GetDNSForTSS(dataSourceName string) (string, error) {
	u, err := url.Parse(dataSourceName)
	if err != nil {
		return "", fmt.Errorf("parsing database DSN: %w", err)
	}

	q := u.Query()
	q.Set("search_path", TSSSchemaName)
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// GetDSNForTenant returns the database DSN for the tenant schema. It is basically the same as the root database DSN,
// but with the `search_path` query parameter (AKA schema) set to `sdp_<tenant_name>`.
func GetDSNForTenant(dataSourceName, tenantName string) (string, error) {
	u, err := url.Parse(dataSourceName)
	if err != nil {
		return "", fmt.Errorf("parsing database DSN: %w", err)
	}

	q := u.Query()
	schemaName := fmt.Sprintf("%s%s", SDPSchemaNamePrefix, tenantName)
	q.Set("search_path", schemaName)
	u.RawQuery = q.Encode()

	return u.String(), nil
}

func GetDBForTSSSchema(dbURL string) (db.DBConnectionPool, error) {
	tssDNS, err := GetDNSForTSS(dbURL)
	if err != nil {
		return nil, fmt.Errorf("getting TSS database DNS: %w", err)
	}
	dbConnectionPool, err := db.OpenDBConnectionPool(tssDNS)
	if err != nil {
		return nil, fmt.Errorf("opening TSS DB connection pool: %w", err)
	}

	return dbConnectionPool, nil
}
