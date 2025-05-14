package data

import (
	"fmt"
	"strings"
)

// SQLColumnConfig contains configuration for generating SQL column names.
type SQLColumnConfig struct {
	// TableReference is the table name or alias in the FROM clause (e.g., "rw" in "FROM receivers_wallet rw")
	TableReference string
	// ResultAlias is the prefix for the result column name (e.g., "wallet" in SELECT rw.id AS "wallet.id")
	ResultAlias string
	// RawColumns is the list of column names to process
	RawColumns []string
	// CoalesceStringColumns indicates which columns should be wrapped in COALESCE(col, '')
	CoalesceStringColumns []string
	// CoalesceFloat64Columns indicates which columns should be wrapped in COALESCE(col, 0.0)
	CoalesceFloat64Columns []string
}

type ColumnType int

const (
	String = iota
	Float64
)

// Build creates a slice of SQL column expressions based on the provided configuration.
// It handles table aliases, column prefixes, and COALESCE wrapping as specified in the config.
func (c SQLColumnConfig) Build() []string {
	if c.TableReference != "" {
		c.TableReference += "."
	}
	if c.ResultAlias != "" {
		c.ResultAlias += "."
	}

	// Process all columns with their respective COALESCE settings
	var completeColumnNames []string
	processColumn := func(column string, shouldCoalesce bool, columnType ColumnType) {
		columnNameAndAlias := strings.SplitN(column, " AS ", 2)
		columnNameAndParser := strings.SplitN(column, "::", 2)

		// Apply COALESCE if needed
		expr := fmt.Sprintf("%s%s", c.TableReference, columnNameAndAlias[0])
		if shouldCoalesce {
			switch columnType {
			case String:
				expr = fmt.Sprintf("COALESCE(%s, '')", expr)
			case Float64:
				expr = fmt.Sprintf("COALESCE(%s, 0.0)", expr)
			}
		}

		// Apply alias if needed
		needsAlias := c.ResultAlias != "" || shouldCoalesce || len(columnNameAndAlias) > 1 || len(columnNameAndParser) > 1
		if needsAlias {
			aliasName := column
			if len(columnNameAndAlias) > 1 {
				aliasName = strings.Trim(columnNameAndAlias[1], `"`)
			} else if len(columnNameAndParser) > 1 {
				aliasName = columnNameAndParser[0]
			}
			expr = fmt.Sprintf(`%s AS "%s%s"`, expr, c.ResultAlias, aliasName)
		}

		completeColumnNames = append(completeColumnNames, expr)
	}

	// Process raw columns
	for _, col := range c.RawColumns {
		processColumn(col, false, String)
	}

	// Process coalesce columns
	for _, col := range c.CoalesceStringColumns {
		processColumn(col, true, String)
	}

	for _, col := range c.CoalesceFloat64Columns {
		processColumn(col, true, Float64)
	}

	return completeColumnNames
}
