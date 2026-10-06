package auth

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
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

	t.Run("returns ErrUserNotFound when the user is deactivated", func(t *testing.T) {
		for _, isOwner := range []bool{true, false} {
			rau := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, isOwner, "role1")
			u := &User{ID: rau.ID, Email: rau.Email}

			_, err := dbConnectionPool.ExecContext(ctx, "UPDATE auth_users SET is_active = false WHERE id = $1", rau.ID)
			require.NoError(t, err)

			ur, err := rm.getUserRolesInfo(ctx, u)
			assert.ErrorIs(t, err, ErrUserNotFound)
			assert.Nil(t, ur)

			_, err = dbConnectionPool.ExecContext(ctx, "UPDATE auth_users SET is_active = true WHERE id = $1", rau.ID)
			require.NoError(t, err)

			ur, err = rm.getUserRolesInfo(ctx, u)
			require.NoError(t, err)
			assert.Equal(t, isOwner, ur.IsOwner)
		}
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

	t.Run("returns ErrUserNotFound when the user is deactivated", func(t *testing.T) {
		rau := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, false, "role1")
		u := &User{ID: rau.ID, Email: rau.Email}

		hasRoles, err := rm.HasAnyRoles(ctx, u, []string{"role1"})
		require.NoError(t, err)
		assert.True(t, hasRoles)

		_, err = dbConnectionPool.ExecContext(ctx, "UPDATE auth_users SET is_active = false WHERE id = $1", rau.ID)
		require.NoError(t, err)

		hasRoles, err = rm.HasAnyRoles(ctx, u, []string{"role1"})
		assert.ErrorIs(t, err, ErrUserNotFound)
		assert.False(t, hasRoles)
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
	assert.ErrorIs(t, err, ErrNoRowsAffected)
}

func assertUserOwnerState(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, userID string, wantIsOwner bool, wantRoles []string) {
	t.Helper()

	var got userRolesInfo
	err := dbConnectionPool.GetContext(ctx, &got, "SELECT roles, is_owner FROM auth_users WHERE id = $1", userID)
	require.NoError(t, err)
	assert.Equal(t, wantIsOwner, got.IsOwner)
	assert.Equal(t, wantRoles, []string(got.Roles))
}

func Test_DefaultRoleManager_UpdateRoles_ownerFlagAndLastOwner(t *testing.T) {
	ctx := context.Background()
	pe := NewDefaultPasswordEncrypter()

	setup := func(t *testing.T) (db.DBConnectionPool, *defaultRoleManager) {
		dbt := dbtest.Open(t)
		t.Cleanup(dbt.Close)
		dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
		require.NoError(t, err)
		t.Cleanup(func() { dbConnectionPool.Close() })

		return dbConnectionPool, newDefaultRoleManager(withRoleManagerDBConnectionPool(dbConnectionPool))
	}

	deactivate := func(t *testing.T, dbConnectionPool db.DBConnectionPool, userID string) {
		_, err := dbConnectionPool.ExecContext(ctx, "UPDATE auth_users SET is_active = false WHERE id = $1", userID)
		require.NoError(t, err)
	}

	t.Run("demoting a flagged owner clears the flag while another owner remains", func(t *testing.T) {
		dbConnectionPool, rm := setup(t)
		CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, true, "owner")
		target := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, true, "financial_controller")

		err := rm.UpdateRoles(ctx, &User{ID: target.ID}, []string{"business"})
		require.NoError(t, err)

		assertUserOwnerState(t, ctx, dbConnectionPool, target.ID, false, []string{"business"})
		roles, err := rm.GetUserRoles(ctx, &User{ID: target.ID})
		require.NoError(t, err)
		assert.Equal(t, []string{"business"}, roles)
	})

	t.Run("promoting to owner sets the flag", func(t *testing.T) {
		dbConnectionPool, rm := setup(t)
		target := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, false, "business")

		err := rm.UpdateRoles(ctx, &User{ID: target.ID}, []string{"owner"})
		require.NoError(t, err)

		assertUserOwnerState(t, ctx, dbConnectionPool, target.ID, true, []string{"owner"})
	})

	t.Run("a nil or empty role list clears the flag", func(t *testing.T) {
		dbConnectionPool, rm := setup(t)
		CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, true, "owner")
		nilTarget := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, true, "owner")
		emptyTarget := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, true, "owner")

		require.NoError(t, rm.UpdateRoles(ctx, &User{ID: nilTarget.ID}, nil))
		require.NoError(t, rm.UpdateRoles(ctx, &User{ID: emptyTarget.ID}, []string{}))

		assertUserOwnerState(t, ctx, dbConnectionPool, nilTarget.ID, false, nil)
		assertUserOwnerState(t, ctx, dbConnectionPool, emptyTarget.ID, false, []string{})
	})

	t.Run("the last active owner cannot be demoted", func(t *testing.T) {
		testCases := []struct {
			name    string
			isOwner bool
			roles   []string
		}{
			{name: "flag only", isOwner: true, roles: []string{"financial_controller"}},
			{name: "role only", isOwner: false, roles: []string{"owner"}},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				dbConnectionPool, rm := setup(t)
				target := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, tc.isOwner, tc.roles...)

				err := rm.UpdateRoles(ctx, &User{ID: target.ID}, []string{"business"})
				require.ErrorIs(t, err, ErrLastOwner)

				assertUserOwnerState(t, ctx, dbConnectionPool, target.ID, tc.isOwner, tc.roles)
			})
		}
	})

	t.Run("an inactive owner does not count as the remaining owner", func(t *testing.T) {
		dbConnectionPool, rm := setup(t)
		inactiveOwner := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, true, "owner")
		deactivate(t, dbConnectionPool, inactiveOwner.ID)
		target := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, true, "owner")

		err := rm.UpdateRoles(ctx, &User{ID: target.ID}, []string{"business"})
		require.ErrorIs(t, err, ErrLastOwner)
	})

	t.Run("an inactive owner can be demoted", func(t *testing.T) {
		dbConnectionPool, rm := setup(t)
		CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, true, "owner")
		target := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, true, "owner")
		deactivate(t, dbConnectionPool, target.ID)

		err := rm.UpdateRoles(ctx, &User{ID: target.ID}, []string{"business"})
		require.NoError(t, err)

		assertUserOwnerState(t, ctx, dbConnectionPool, target.ID, false, []string{"business"})
	})

	t.Run("two owners demoting each other at once leave one owner", func(t *testing.T) {
		dbConnectionPool, rm := setup(t)
		ownerA := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, true, "owner")
		ownerB := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, true, "owner")

		errs := make(chan error, 2)
		var wg sync.WaitGroup
		for _, id := range []string{ownerA.ID, ownerB.ID} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs <- rm.UpdateRoles(ctx, &User{ID: id}, []string{"business"})
			}()
		}
		wg.Wait()
		close(errs)

		var succeeded, refused int
		for err := range errs {
			if err == nil {
				succeeded++
			} else {
				require.ErrorIs(t, err, ErrLastOwner)
				refused++
			}
		}
		assert.Equal(t, 1, succeeded)
		assert.Equal(t, 1, refused)
	})

	t.Run("demotion and deactivation at once leave one owner", func(t *testing.T) {
		dbConnectionPool, rm := setup(t)
		authenticator := newDefaultAuthenticator(withAuthenticatorDatabaseConnectionPool(dbConnectionPool))
		ownerA := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, true, "owner")
		ownerB := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, pe, true, "owner")

		errs := make(chan error, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			errs <- rm.UpdateRoles(ctx, &User{ID: ownerA.ID}, []string{"business"})
		}()
		go func() {
			defer wg.Done()
			errs <- authenticator.DeactivateUser(ctx, ownerB.ID)
		}()
		wg.Wait()
		close(errs)

		var refused int
		for err := range errs {
			if err != nil {
				require.ErrorIs(t, err, ErrLastOwner)
				refused++
			}
		}
		assert.Equal(t, 1, refused)
	})
}

func Test_withOwnerRoleName(t *testing.T) {
	expectedRoleName := "my-owner-role-name"
	rm := newDefaultRoleManager(withOwnerRoleName(expectedRoleName))
	assert.NotEqual(t, defaultOwnerRoleName, rm.ownerRoleName)
	assert.Equal(t, expectedRoleName, rm.ownerRoleName)
}
