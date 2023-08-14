package auth

import (
	"context"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_DefaultRoleManager_getUserRolesInfo(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	pe := NewDefaultPasswordEncrypter()
	rm := newDefaultRoleManager(withRoleManagerDBConnectionPool(dbConnectionPool))

	t.Run("returns correctly when user is a super user", func(t *testing.T) {
		rau := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, true)

		u := &User{
			ID:    rau.ID,
			Email: rau.Email,
			Roles: []string{"role1"},
		}

		ur, err := rm.getUserRolesInfo(ctx, u)
		require.NoError(t, err)

		assert.True(t, ur.IsOwner)
	})

	t.Run("returns correctly when user isn't a super user", func(t *testing.T) {
		roles := []string{"role1"}

		rau := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, false, roles...)

		u := &User{
			ID:    rau.ID,
			Email: rau.Email,
			Roles: []string{"role1"},
		}

		ur, err := rm.getUserRolesInfo(ctx, u)
		require.NoError(t, err)

		assert.False(t, ur.IsOwner)
		assert.Equal(t, roles, []string(ur.Roles))
	})

	t.Run("returns correctly when user has no roles and is not super user", func(t *testing.T) {
		rau := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, false)

		u := &User{
			ID:    rau.ID,
			Email: rau.Email,
			Roles: []string{"role1"},
		}

		ur, err := rm.getUserRolesInfo(ctx, u)
		require.NoError(t, err)

		assert.False(t, ur.IsOwner)
		assert.Empty(t, ur.Roles)
	})
}

func Test_DefaultRoleManager_GetUserRoles(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	pe := NewDefaultPasswordEncrypter()
	rm := newDefaultRoleManager(
		withRoleManagerDBConnectionPool(dbConnectionPool),
	)

	t.Run("returns all the roles correctly", func(t *testing.T) {
		expectedRoles := []string{"role1", "role2", "role3"}

		rau := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, false, expectedRoles...)

		u := &User{
			ID:    rau.ID,
			Email: rau.Email,
		}

		gotRoles, err := rm.GetUserRoles(ctx, u)
		require.NoError(t, err)

		assert.Equal(t, expectedRoles, gotRoles)
	})

	t.Run("returns owner role correctly", func(t *testing.T) {
		roles := []string{"role1", "role2", "role3"}

		rau := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, true, roles...)

		u := &User{
			ID:    rau.ID,
			Email: rau.Email,
		}

		gotRoles, err := rm.GetUserRoles(ctx, u)
		require.NoError(t, err)

		assert.Equal(t, []string{defaultOwnerRoleName}, gotRoles)
	})
}

func Test_DefaultRoleManager_HasAllRoles(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	pe := NewDefaultPasswordEncrypter()
	rm := newDefaultRoleManager(
		withRoleManagerDBConnectionPool(dbConnectionPool),
	)

	t.Run("return false when user isOwner but doesn't have the roles", func(t *testing.T) {
		rau := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, true, "role1")

		u := &User{
			ID:    rau.ID,
			Email: rau.Email,
		}

		hasRoles, err := rm.HasAllRoles(ctx, u, []string{"role1", "role2", "role3"})
		require.NoError(t, err)

		assert.False(t, hasRoles)
	})

	t.Run("validates the user roles correctly", func(t *testing.T) {
		rau := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, false, "role1", "role2")

		u := &User{
			ID:    rau.ID,
			Email: rau.Email,
		}

		hasRoles, err := rm.HasAllRoles(ctx, u, []string{"role1", "role2", "role3"})
		require.NoError(t, err)
		assert.False(t, hasRoles)

		hasRoles, err = rm.HasAllRoles(ctx, u, []string{"role3"})
		require.NoError(t, err)
		assert.False(t, hasRoles)

		hasRoles, err = rm.HasAllRoles(ctx, u, []string{"role1"})
		require.NoError(t, err)
		assert.True(t, hasRoles)

		hasRoles, err = rm.HasAllRoles(ctx, u, []string{"role2"})
		require.NoError(t, err)
		assert.True(t, hasRoles)

		hasRoles, err = rm.HasAllRoles(ctx, u, []string{"role1", "role2"})
		require.NoError(t, err)
		assert.True(t, hasRoles)

		hasRoles, err = rm.HasAllRoles(ctx, u, []string{"role1", "role3"})
		require.NoError(t, err)
		assert.False(t, hasRoles)
	})
}

func Test_DefaultRoleManager_HasAnyRoles(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	pe := NewDefaultPasswordEncrypter()
	rm := newDefaultRoleManager(
		withRoleManagerDBConnectionPool(dbConnectionPool),
	)

	t.Run("return false when user isOwner but doesn't have the roles", func(t *testing.T) {
		rau := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, true, "role4")

		u := &User{
			ID:    rau.ID,
			Email: rau.Email,
		}

		hasRoles, err := rm.HasAnyRoles(ctx, u, []string{"role1", "role2", "role3"})
		require.NoError(t, err)

		assert.False(t, hasRoles)
	})

	t.Run("validates the user roles correctly", func(t *testing.T) {
		rau := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, false, "role1", "role2")

		u := &User{
			ID:    rau.ID,
			Email: rau.Email,
		}

		hasRoles, err := rm.HasAnyRoles(ctx, u, []string{"role1", "role2", "role3"})
		require.NoError(t, err)
		assert.True(t, hasRoles)

		hasRoles, err = rm.HasAnyRoles(ctx, u, []string{"role3"})
		require.NoError(t, err)
		assert.False(t, hasRoles)

		hasRoles, err = rm.HasAnyRoles(ctx, u, []string{"role1"})
		require.NoError(t, err)
		assert.True(t, hasRoles)

		hasRoles, err = rm.HasAnyRoles(ctx, u, []string{"role2"})
		require.NoError(t, err)
		assert.True(t, hasRoles)

		hasRoles, err = rm.HasAnyRoles(ctx, u, []string{"role1", "role2"})
		require.NoError(t, err)
		assert.True(t, hasRoles)

		hasRoles, err = rm.HasAnyRoles(ctx, u, []string{"role1", "role3"})
		require.NoError(t, err)
		assert.True(t, hasRoles)
	})
}

func Test_DefaultRoleManager_IsSuperUser(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	pe := NewDefaultPasswordEncrypter()
	rm := newDefaultRoleManager(
		withRoleManagerDBConnectionPool(dbConnectionPool),
	)

	rau := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, false)
	rauOwner := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, true)

	u := &User{
		ID:    rau.ID,
		Email: rau.Email,
	}

	uo := &User{
		ID:    rauOwner.ID,
		Email: rauOwner.Email,
	}

	isSuperUser, err := rm.IsSuperUser(ctx, u)
	require.NoError(t, err)
	assert.False(t, isSuperUser)

	isSuperUser, err = rm.IsSuperUser(ctx, uo)
	require.NoError(t, err)
	assert.True(t, isSuperUser)
}

func Test_DefaultRoleManager_UpdateRoles(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	pe := NewDefaultPasswordEncrypter()
	rm := newDefaultRoleManager(
		withRoleManagerDBConnectionPool(dbConnectionPool),
	)

	rau := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, false)

	u := &User{
		ID:    rau.ID,
		Email: rau.Email,
	}

	err = rm.UpdateRoles(ctx, u, []string{"role1"})
	require.NoError(t, err)

	roles, err := rm.GetUserRoles(ctx, u)
	require.NoError(t, err)
	assert.Equal(t, []string{"role1"}, roles)

	err = rm.UpdateRoles(ctx, u, []string{"role1", "role2"})
	require.NoError(t, err)

	roles, err = rm.GetUserRoles(ctx, u)
	require.NoError(t, err)
	assert.Equal(t, []string{"role1", "role2"}, roles)

	err = rm.UpdateRoles(ctx, u, []string{"role3"})
	require.NoError(t, err)

	roles, err = rm.GetUserRoles(ctx, u)
	require.NoError(t, err)
	assert.Equal(t, []string{"role3"}, roles)

	err = rm.UpdateRoles(ctx, &User{ID: "user-id"}, []string{"role3"})
	assert.EqualError(t, err, ErrNoRowsAffected.Error())
}

func Test_withOwnerRoleName(t *testing.T) {
	expectedRoleName := "my-owner-role-name"
	rm := newDefaultRoleManager(withOwnerRoleName(expectedRoleName))
	assert.NotEqual(t, defaultOwnerRoleName, rm.ownerRoleName)
	assert.Equal(t, expectedRoleName, rm.ownerRoleName)
}
