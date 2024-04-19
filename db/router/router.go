package router

import (
	"fmt"
	"net/url"
)

const (
	AdminSchemaName     string = "admin"
	SDPSchemaNamePrefix string = "sdp_"
	TSSSchemaName       string = "tss"
)

// GetDNSForAdmin returns the database DSN for the Admin schema. It is the same as the root database DSN, clearing the
// `search_path` if it exists.
func GetDNSForAdmin(dataSourceName string) (string, error) {
	u, err := url.Parse(dataSourceName)
	if err != nil {
		return "", fmt.Errorf("parsing database DSN: %w", err)
	}

	q := u.Query()
	q.Set("search_path", AdminSchemaName)
	u.RawQuery = q.Encode()

	return u.String(), nil
}

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
