package data

import "fmt"

// SQLColumnConfig contains configuration for generating SQL column names.
type SQLColumnConfig struct {
	// TableAlias is the prefix for the table (e.g., "rw" in "rw.column_name")
	TableAlias string
	// CoalesceToEmptyString indicates whether to wrap column in COALESCE(col, '')
	CoalesceToEmptyString bool
	// AliasPrefix is the prefix for the AS clause (e.g., "wallet" in AS "wallet.column_name")
	AliasPrefix string
	// Columns is the list of column names to process
	Columns []string
}

// GenerateColumnNames creates a slice of SQL column expressions based on the provided configuration.
// It handles table aliases, column prefixes, and COALESCE wrapping as specified in the config.
func GenerateColumnNames(config SQLColumnConfig) []string {
	if config.TableAlias != "" {
		config.TableAlias += "."
	}
	if config.AliasPrefix != "" {
		config.AliasPrefix += "."
	}

	var completeColumnNames []string
	for _, column := range config.Columns {
		// Apply COALESCE if needed
		scanName := fmt.Sprintf("%s%s", config.TableAlias, column)
		if config.CoalesceToEmptyString {
			scanName = fmt.Sprintf("COALESCE(%s, '')", scanName)
		}

		// Apply alias if needed
		var columnAlias string
		if config.AliasPrefix != "" || config.CoalesceToEmptyString {
			columnAlias = fmt.Sprintf(` AS "%s%s"`, config.AliasPrefix, column)
		}

		completeColumnNames = append(completeColumnNames, fmt.Sprintf("%s%s", scanName, columnAlias))
	}

	return completeColumnNames
}
