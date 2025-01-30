package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_GenerateColumnNames(t *testing.T) {
	testCases := []struct {
		name     string
		config   SQLColumnConfig
		expected []string
	}{
		{
			name: "basic columns without alias nor special characters",
			config: SQLColumnConfig{
				Columns: []string{"id", "name", "status"},
			},
			expected: []string{"id", "name", "status"},
		},
		{
			name: "columns with table reference",
			config: SQLColumnConfig{
				TableReference: "t",
				Columns:        []string{"id", "name", "status"},
			},
			expected: []string{"t.id", "t.name", "t.status"},
		},
		{
			name: "columns with result alias",
			config: SQLColumnConfig{
				ResultAlias: "user",
				Columns:     []string{"id", "name", "status"},
			},
			expected: []string{
				`id AS "user.id"`,
				`name AS "user.name"`,
				`status AS "user.status"`,
			},
		},
		{
			name: "columns with table reference and result alias",
			config: SQLColumnConfig{
				TableReference: "t",
				ResultAlias:    "user",
				Columns:        []string{"id", "name", "status"},
			},
			expected: []string{
				`t.id AS "user.id"`,
				`t.name AS "user.name"`,
				`t.status AS "user.status"`,
			},
		},
		{
			name: "columns with coalesce",
			config: SQLColumnConfig{
				CoalesceToEmptyString: true,
				Columns:               []string{"id", "name"},
			},
			expected: []string{
				`COALESCE(id, '') AS "id"`,
				`COALESCE(name, '') AS "name"`,
			},
		},
		{
			name: "columns with type cast",
			config: SQLColumnConfig{
				Columns: []string{"verification_field::text"},
			},
			expected: []string{`verification_field::text AS "verification_field"`},
		},
		{
			name: "columns with COALESCE and type cast",
			config: SQLColumnConfig{
				CoalesceToEmptyString: true,
				Columns:               []string{"verification_field::text"},
			},
			expected: []string{`COALESCE(verification_field::text, '') AS "verification_field"`},
		},
		{
			name: "columns with explicit alias",
			config: SQLColumnConfig{
				Columns: []string{`receiver_id AS "receiver.id"`},
			},
			expected: []string{`receiver_id AS "receiver.id"`},
		},
		{
			name: "columns with explicit alias and type cast",
			config: SQLColumnConfig{
				Columns: []string{`receiver_id::text AS "receiver.id"`},
			},
			expected: []string{`receiver_id::text AS "receiver.id"`},
		},
		{
			name: "all features",
			config: SQLColumnConfig{
				TableReference:        "rw",
				CoalesceToEmptyString: true,
				ResultAlias:           "receiver_wallet",
				Columns:               []string{`receiver_id::text AS "receiver.id"`},
			},
			expected: []string{`COALESCE(rw.receiver_id::text, '') AS "receiver_wallet.receiver.id"`},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := GenerateColumnNames(tc.config)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
