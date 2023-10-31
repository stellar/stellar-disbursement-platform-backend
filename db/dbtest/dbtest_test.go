package dbtest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen(t *testing.T) {
	db := Open(t)
	defer db.Close()

	session := db.Open()
	defer session.Close()

	count := 0

	// Tenant migrations
	err := session.Get(&count, `SELECT COUNT(*) FROM migrations`)
	require.NoError(t, err)
	assert.Greater(t, count, 0)

	// SDP migrations
	err = session.Get(&count, `SELECT COUNT(*) FROM gorp_migrations`)
	require.NoError(t, err)
	assert.Greater(t, count, 0)

	// Auth Migrations
	err = session.Get(&count, `SELECT COUNT(*) FROM auth_migrations`)
	require.NoError(t, err)
	assert.Greater(t, count, 0)
}
