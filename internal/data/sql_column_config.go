package data

import (
	"fmt"
	"strings"
)

// SQLColumnConfig contains configuration for generating SQL column names.
type SQLColumnConfig struct {
	// TableReference is the table name or alias in the FROM clause (e.g., "rw" in "FROM receivers_wallet rw")
	TableReference string
	// CoalesceToEmptyString indicates whether to wrap column in COALESCE(col, '')
	CoalesceToEmptyString bool
	// ResultAlias is the prefix for the result column name (e.g., "wallet" in SELECT rw.id AS "wallet.id")
	ResultAlias string
	// Columns is the list of column names to process
	Columns []string
}

// GenerateColumnNames creates a slice of SQL column expressions based on the provided configuration.
// It handles table aliases, column prefixes, and COALESCE wrapping as specified in the config.
func GenerateColumnNames(config SQLColumnConfig) []string {
	if config.TableReference != "" {
		config.TableReference += "."
	}
	if config.ResultAlias != "" {
		config.ResultAlias += "."
	}

	var completeColumnNames []string
	for _, column := range config.Columns {
		// Apply COALESCE if needed
		scanName := fmt.Sprintf("%s%s", config.TableReference, column)
		if config.CoalesceToEmptyString {
			scanName = fmt.Sprintf("COALESCE(%s, '')", scanName)
		}

		// Apply alias if needed
		var columnAlias string
		if config.ResultAlias != "" || config.CoalesceToEmptyString {
			column = strings.SplitN(column, "::", 2)[0]
			columnAlias = fmt.Sprintf(` AS "%s%s"`, config.ResultAlias, column)
		}

		completeColumnNames = append(completeColumnNames, fmt.Sprintf("%s%s", scanName, columnAlias))
	}

	return completeColumnNames
}
