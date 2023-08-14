package data

import (
	"fmt"
)

// QueryBuilder is a helper struct for building SQL queries
type QueryBuilder struct {
	baseQuery        string
	whereClause      string
	whereParams      []interface{}
	sortClause       string
	paginationClause string
	paginationParams []interface{}
}

// NewQueryBuilder creates a new QueryBuilder
func NewQueryBuilder(query string) *QueryBuilder {
	return &QueryBuilder{
		baseQuery: query,
	}
}

// AddCondition adds a condition to the query
// If the value is nil or empty, the condition is not added
// The condition should be a string with a placeholder for the value e.g. "name = ?", "id > ?"
func (qb *QueryBuilder) AddCondition(condition string, value ...interface{}) *QueryBuilder {
	if len(value) > 0 {
		qb.whereClause = fmt.Sprintf("%s %s", qb.whereClause, "AND "+condition)
		qb.whereParams = append(qb.whereParams, value...)
	}
	return qb
}

// AddSorting adds a sorting clause to the query
// prefix is the prefix to use for the sort field e.g. "d" for "d.created_at"
func (qb *QueryBuilder) AddSorting(sortField SortField, sortOrder SortOrder, prefix string) *QueryBuilder {
	if sortField != "" {
		qb.sortClause = fmt.Sprintf("ORDER BY %s.%s %s", prefix, sortField, sortOrder)
	}
	return qb
}

// AddPagination adds a pagination clause to the query
func (qb *QueryBuilder) AddPagination(page int, pageLimit int) *QueryBuilder {
	if page > 0 && pageLimit > 0 {
		offset := (page - 1) * pageLimit
		qb.paginationClause = "LIMIT ? OFFSET ?"
		qb.paginationParams = append(qb.paginationParams, pageLimit, offset)
	}
	return qb
}

// Build assembles all statements in the correct order and returns the query and the parameters
func (qb *QueryBuilder) Build() (string, []interface{}) {
	query := qb.baseQuery
	params := []interface{}{}
	if qb.whereClause != "" {
		query = fmt.Sprintf("%s WHERE 1=1%s", query, qb.whereClause)
		params = append(params, qb.whereParams...)
	}
	if qb.sortClause != "" {
		query = fmt.Sprintf("%s %s", query, qb.sortClause)
	}
	if qb.paginationClause != "" {
		query = fmt.Sprintf("%s %s", query, qb.paginationClause)
		params = append(params, qb.paginationParams...)
	}
	return query, params
}
