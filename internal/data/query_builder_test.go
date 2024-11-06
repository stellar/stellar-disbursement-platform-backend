package data

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_QueryBuilder(t *testing.T) {
	baseQuery := "SELECT * FROM receivers"
	testCases := []struct {
		name          string
		condition     string
		values        []interface{}
		expectedQuery string
	}{
		{
			name:          "single parameter",
			condition:     "id = ?",
			values:        []interface{}{"123"},
			expectedQuery: "SELECT * FROM receivers WHERE 1=1 %s id = ?",
		},
		{
			name:          "multiple parameters",
			condition:     "(id ILIKE ? OR email ILIKE ? OR phone_number ILIKE ?)",
			values:        []interface{}{"id", "mock@email.com", "+9999999"},
			expectedQuery: "SELECT * FROM receivers WHERE 1=1 %s (id ILIKE ? OR email ILIKE ? OR phone_number ILIKE ?)",
		},
		{
			name:          "empty value",
			condition:     "email is NULL",
			values:        []interface{}{},
			expectedQuery: "SELECT * FROM receivers WHERE 1=1 %s email is NULL",
		},
	}

	for _, tc := range testCases {
		for _, condition := range []string{"AND", "OR"} {
			t.Run("Test `"+condition+"`: "+tc.name, func(t *testing.T) {
				qb := NewQueryBuilder(baseQuery)

				if condition == "AND" {
					qb.AddCondition(tc.condition, tc.values...)
				} else {
					qb.AddOrCondition(tc.condition, tc.values...)
				}
				actualQuery, params := qb.Build()

				expectedQuery := fmt.Sprintf(tc.expectedQuery, condition)

				assert.Equal(t, expectedQuery, actualQuery)
				assert.Equal(t, tc.values, params)
			})
		}
	}

	t.Run("Test AddSorting", func(t *testing.T) {
		qb := NewQueryBuilder("SELECT * FROM disbursements d")

		qb.AddSorting("created_at", "DESC", "d")
		actual, _ := qb.Build()

		expectedQuery := "SELECT * FROM disbursements d ORDER BY d.created_at DESC"
		assert.Equal(t, expectedQuery, actual)
	})

	t.Run("Test AddPagination", func(t *testing.T) {
		qb := NewQueryBuilder("SELECT * FROM disbursements d")

		qb.AddPagination(2, 20)
		actual, params := qb.Build()

		expectedQuery := "SELECT * FROM disbursements d LIMIT ? OFFSET ?"
		assert.Equal(t, expectedQuery, actual)
		assert.Equal(t, []interface{}{20, 20}, params)
	})

	t.Run("Test Full query", func(t *testing.T) {
		qb := NewQueryBuilder("SELECT * FROM disbursements d")
		qb.AddCondition("name = ?", "Disbursement 1")
		qb.AddSorting("created_at", "DESC", "d")
		qb.AddPagination(2, 20)
		actual, params := qb.Build()

		expectedQuery := "SELECT * FROM disbursements d WHERE 1=1 AND name = ? ORDER BY d.created_at DESC LIMIT ? OFFSET ?"
		assert.Equal(t, expectedQuery, actual)
		assert.Equal(t, []interface{}{"Disbursement 1", 20, 20}, params)
	})

	t.Run("Test AddGroupBy", func(t *testing.T) {
		qb := NewQueryBuilder("SELECT * FROM disbursements d")

		qb.AddGroupBy("d.id")
		actual, _ := qb.Build()

		expectedQuery := "SELECT * FROM disbursements d GROUP BY d.id"
		assert.Equal(t, expectedQuery, actual)
	})
}

func Test_BuildSetClause(t *testing.T) {
	testCases := []struct {
		name          string
		input         interface{}
		expectedQuery string
		expectedArgs  []interface{}
	}{
		{
			name:  "non-struct generates empty output",
			input: "non-struct",
		},
		{
			name:  "struct without the \"db\" tag generates empty output",
			input: struct{ Name string }{Name: "John"},
		},
		{
			name: "struct without \"db\" tag generates the expected output, only included non-empty fields",
			input: struct {
				Name     string `db:"name"`
				LastName string `db:"last_name"`
			}{Name: "John"},
			expectedQuery: "name = ?",
			expectedArgs:  []interface{}{"John"},
		},
		{
			name: "struct without \"db\" tag generates the expected output, only included non-empty fields",
			input: struct {
				Name     string `db:"name"`
				LastName string `db:"last_name"`
			}{Name: "John"},
			expectedQuery: "name = ?",
			expectedArgs:  []interface{}{"John"},
		},
		{
			name: "struct without \"db,qualifier\" tag generates the expected output, not including the qualifier",
			input: struct {
				Name     string `db:"name,qualifier"`
				LastName string `db:"last_name,omitempty"`
			}{Name: "John", LastName: "Doe"},
			expectedQuery: "name = ?, last_name = ?",
			expectedArgs:  []interface{}{"John", "Doe"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			query, args := BuildSetClause(tc.input)
			assert.Equal(t, tc.expectedQuery, query)
			assert.Equal(t, tc.expectedArgs, args)
		})
	}
}
