package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_QueryBuilder(t *testing.T) {
	t.Run("Test AddCondition", func(t *testing.T) {
		qb := NewQueryBuilder("SELECT * FROM disbursements")

		qb.AddCondition("name = ?", "Disbursement 1")
		actual, params := qb.Build()

		expectedQuery := "SELECT * FROM disbursements WHERE 1=1 AND name = ?"

		assert.Equal(t, expectedQuery, actual)
		assert.Equal(t, []interface{}{"Disbursement 1"}, params)
	})

	t.Run("Test AddCondition multiple params", func(t *testing.T) {
		qb := NewQueryBuilder("SELECT * FROM receivers")

		qb.AddCondition("(id ILIKE ? OR email ILIKE ? OR phone_number ILIKE ?)", "id", "mock@email.com", "+9999999")
		actual, params := qb.Build()

		expectedQuery := "SELECT * FROM receivers WHERE 1=1 AND (id ILIKE ? OR email ILIKE ? OR phone_number ILIKE ?)"

		assert.Equal(t, expectedQuery, actual)
		assert.Equal(t, []interface{}{"id", "mock@email.com", "+9999999"}, params)
	})

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
}
