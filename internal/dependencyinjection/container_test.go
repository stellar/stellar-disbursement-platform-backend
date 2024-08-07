package dependencyinjection

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

func Test_SetInstance(t *testing.T) {
	defer ClearInstancesTestHelper(t)

	assert.Nil(t, dependenciesStore["testKey"])

	SetInstance("testKey", "testValue")
	assert.Equal(t, "testValue", dependenciesStore["testKey"])

	SetInstance("testKey", "testValue2")
	assert.Equal(t, "testValue2", dependenciesStore["testKey"])
}

func Test_GetInstance(t *testing.T) {
	defer ClearInstancesTestHelper(t)

	dependenciesStore["testKey"] = "testValue"
	instance, ok := GetInstance("testKey")
	assert.True(t, ok)
	assert.Equal(t, "testValue", instance)

	instance, ok = GetInstance("testKey2")
	assert.False(t, ok)
	assert.Nil(t, instance)
}

func Test_CleanupInstanceByKey(t *testing.T) {
	defer ClearInstancesTestHelper(t)
	ctx := context.Background()

	t.Run("attempting to delete a non-existing key should not panic", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		assert.NotPanics(t, func() { CleanupInstanceByKey(ctx, "testKey") })
	})

	t.Run("deleting something that's not a dbConnectionPool", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		SetInstance("testKey", "testValue")
		_, ok := GetInstance("testKey")
		assert.True(t, ok)

		CleanupInstanceByKey(ctx, "testKey")
		_, ok = GetInstance("testKey")
		assert.False(t, ok)
	})

	t.Run("deleting a dbConnectionPool and asserting it gets closed automatically", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		dbt := dbtest.Open(t)
		defer dbt.Close()
		dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
		require.NoError(t, err)
		defer dbConnectionPool.Close()

		SetInstance("dbKey", dbConnectionPool)
		_, ok := GetInstance("dbKey")
		assert.True(t, ok)

		CleanupInstanceByKey(ctx, "dbKey")
		_, ok = GetInstance("dbKey")
		assert.False(t, ok)
		err = dbConnectionPool.Ping(ctx)
		assert.ErrorContains(t, err, "sql: database is closed")
	})
}

func Test_CleanupInstanceByValue(t *testing.T) {
	defer ClearInstancesTestHelper(t)

	ctx := context.Background()

	t.Run("attempting to delete a non-existing value should not panic", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		assert.NotPanics(t, func() { CleanupInstanceByValue(ctx, "testValue") })
	})

	t.Run("deleting something that's not a dbConnectionPool", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		SetInstance("testKey", "testValue")
		_, ok := GetInstance("testKey")
		assert.True(t, ok)

		CleanupInstanceByValue(ctx, "testValue")
		_, ok = GetInstance("testKey")
		assert.False(t, ok)
	})

	t.Run("deleting a dbConnectionPool and asserting it gets closed automatically", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		dbt := dbtest.Open(t)
		defer dbt.Close()
		dbConnectionPool1, err := db.OpenDBConnectionPool(dbt.DSN)
		require.NoError(t, err)
		defer dbConnectionPool1.Close()
		dbConnectionPool2, err := db.OpenDBConnectionPool(dbt.DSN)
		require.NoError(t, err)
		defer dbConnectionPool2.Close()
		SetInstance("dbConnectionPool2", dbConnectionPool2)

		keyNames := []string{"dbConnectionPool1.a", "dbConnectionPool1.b", "dbConnectionPool1.c"}
		for i, keyName := range keyNames {
			SetInstance(keyName, dbConnectionPool1)
			_, ok := GetInstance(keyName)
			require.Truef(t, ok, "instance missing for index %d", i)
		}

		CleanupInstanceByValue(ctx, dbConnectionPool1)
		for i, keyName := range keyNames {
			_, ok := GetInstance(keyName)
			require.Falsef(t, ok, "instance %d should have been deleted", i)
			err = dbConnectionPool1.Ping(ctx)
			assert.ErrorContains(t, err, "sql: database is closed")
		}

		_, ok := GetInstance("dbConnectionPool2")
		require.True(t, ok)
		err = dbConnectionPool2.Ping(ctx)
		require.NoError(t, err)
	})
}
