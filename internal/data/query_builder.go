package data

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

// QueryBuilder is a helper struct for building SQL queries
type QueryBuilder struct {
	baseQuery           string
	whereClause         string
	whereParams         []interface{}
	sortClause          string
	groupByClause       string
	paginationClause    string
	paginationParams    []interface{}
	forUpdateSkipLocked bool
}

// NewQueryBuilder creates a new QueryBuilder
func NewQueryBuilder(query string) *QueryBuilder {
	return &QueryBuilder{
		baseQuery: query,
	}
}

// AddCondition adds a AND condition to the query
// The condition should be a string with a placeholder for the value e.g. "name = ?", "id > ?"
func (qb *QueryBuilder) AddCondition(condition string, value ...interface{}) *QueryBuilder {
	if len(value) >= 0 {
		qb.whereClause = fmt.Sprintf("%s %s", qb.whereClause, "AND "+condition)
		if len(value) > 0 {
			qb.whereParams = append(qb.whereParams, value...)
		}
	}
	return qb
}

func (qb *QueryBuilder) AddGroupBy(fields string) *QueryBuilder {
	qb.groupByClause = fmt.Sprintf("GROUP BY %s", fields)
	return qb
}

// TODO [SDP-1190]: combine AddCondition and AddOrCondition into one function with a parameter for the condition type
// AddOrCondition adds an OR condition to the query
func (qb *QueryBuilder) AddOrCondition(condition string, value ...interface{}) *QueryBuilder {
	if len(value) >= 0 {
		qb.whereClause = fmt.Sprintf("%s %s", qb.whereClause, "OR "+condition)
		if len(value) > 0 {
			qb.whereParams = append(qb.whereParams, value...)
		}
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
	if qb.groupByClause != "" {
		query = fmt.Sprintf("%s %s", query, qb.groupByClause)
	}
	if qb.sortClause != "" {
		query = fmt.Sprintf("%s %s", query, qb.sortClause)
	}
	if qb.paginationClause != "" {
		query = fmt.Sprintf("%s %s", query, qb.paginationClause)
		params = append(params, qb.paginationParams...)
	}
	if qb.forUpdateSkipLocked {
		query = fmt.Sprintf("%s FOR UPDATE SKIP LOCKED", query)
	}
	return query, params
}

func (qb *QueryBuilder) BuildAndRebind(sqlExec db.SQLExecuter) (string, []interface{}) {
	query, params := qb.Build()
	query = sqlExec.Rebind(query)
	return query, params
}

// BuildSetClause builds a SET clause for an UPDATE query based on the provided struct and its "db" tags. For instance,
// given the following struct:
//
//	type User struct {
//	    ID   int64  `db:"id"`
//	    Name string `db:"name"`
//	}
//
// The function will return the following string and slice when called with an instance of `User{ID: 1, Name: "John"}`:
// "id = ?, name = ?", []interface{}{1, "John"}
func BuildSetClause(u interface{}) (string, []interface{}) {
	v := reflect.ValueOf(u)
	t := reflect.TypeOf(u)

	// Check if the provided argument is a struct
	if t.Kind() != reflect.Struct {
		return "", nil
	}

	var setClauses []string
	var params []interface{}

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)
		dbTag := fieldType.Tag.Get("db")
		dbTag = strings.Split(dbTag, ",")[0]
		if dbTag == "" {
			continue
		}

		// Check if the field is not zero-value
		if !field.IsZero() {
			setClauses = append(setClauses, fmt.Sprintf("%s = ?", dbTag))
			params = append(params, field.Interface())
		}
	}

	return strings.Join(setClauses, ", "), params
}
