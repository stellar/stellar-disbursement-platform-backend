package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_defaultMFAManager_MFADeviceRemembered(t *testing.T) {
	ctx := context.Background()

	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, NewDefaultPasswordEncrypter(), false)

	m := newDefaultMFAManager(withMFADatabaseConnectionPool(dbConnectionPool))

	t.Run("Test error when deviceID or userID is empty", func(t *testing.T) {
		_, err := m.MFADeviceRemembered(ctx, "", "")
		require.ErrorContains(t, err, "device ID and user ID are required")
		_, err = m.MFADeviceRemembered(ctx, "deviceID", "")
		require.ErrorContains(t, err, "device ID and user ID are required")
		_, err = m.MFADeviceRemembered(ctx, "", "userID")
		require.ErrorContains(t, err, "device ID and user ID are required")
	})

	t.Run("Test error when user not found", func(t *testing.T) {
		isRemembered, err := m.MFADeviceRemembered(ctx, "deviceID", "nonExistentUser")
		require.NoError(t, err)
		require.False(t, isRemembered)
	})

	t.Run("Test Device Exists: ❌ | Device Valid: – |", func(t *testing.T) {
		isRemembered, err := m.MFADeviceRemembered(ctx, "deviceID", randUser.ID)
		require.NoError(t, err)
		require.False(t, isRemembered)
	})

	t.Run("Test Device Exists: ✅ | Device Valid: ❌ |", func(t *testing.T) {
		defer cleanup(t, ctx, dbConnectionPool)

		// Generate code for device and expire device
		_, err := m.GenerateMFACode(ctx, "deviceID", randUser.ID)
		require.NoError(t, err)
		err = m.ForgetDevice(ctx, "deviceID", randUser.ID)
		require.NoError(t, err)

		isValid, err := m.MFADeviceRemembered(ctx, "deviceID", randUser.ID)
		require.NoError(t, err)
		require.False(t, isValid)
	})

	t.Run("Test Device Exists: ✅ | Device Valid: ✅ |", func(t *testing.T) {
		defer cleanup(t, ctx, dbConnectionPool)

		// Generate code for device and remember device
		code, err := m.GenerateMFACode(ctx, "deviceID", randUser.ID)
		require.NoError(t, err)
		err = m.RememberDevice(ctx, "deviceID", code)
		require.NoError(t, err)

		// Validate device
		isRemembered, err := m.MFADeviceRemembered(ctx, "deviceID", randUser.ID)
		require.NoError(t, err)
		require.True(t, isRemembered)
	})
}

func Test_defaultMFAManager_GenerateMFACode(t *testing.T) {
	ctx := context.Background()

	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, NewDefaultPasswordEncrypter(), false)

	m := newDefaultMFAManager(withMFADatabaseConnectionPool(dbConnectionPool))

	t.Run("Test error when deviceID or userID is empty", func(t *testing.T) {
		_, err := m.GenerateMFACode(ctx, "", "")
		require.ErrorContains(t, err, "device ID and user ID are required")
		_, err = m.GenerateMFACode(ctx, "deviceID", "")
		require.ErrorContains(t, err, "device ID and user ID are required")
		_, err = m.GenerateMFACode(ctx, "", "userID")
		require.ErrorContains(t, err, "device ID and user ID are required")
	})

	t.Run("Test error when user not found", func(t *testing.T) {
		defer cleanup(t, ctx, dbConnectionPool)

		_, err := m.GenerateMFACode(ctx, "deviceID", "nonExistentUser")
		require.ErrorContains(t, err, "error updating MFA code for user ID nonExistentUser and device ID deviceID")
	})

	t.Run("Test Device Exists: ❌ | Code Exists: -  | Code Valid: -", func(t *testing.T) {
		defer cleanup(t, ctx, dbConnectionPool)

		code, err := m.GenerateMFACode(ctx, "deviceID", randUser.ID)
		require.NoError(t, err)
		require.NotNil(t, code)
		require.Equal(t, 6, len(code))

		mc, err := m.getByDeviceAndCode(ctx, "deviceID", code)
		require.NoError(t, err)
		require.NotNil(t, mc)
		require.Equal(t, code, mc.Code)
		require.Equal(t, "deviceID", mc.DeviceID)
		require.Equal(t, randUser.ID, mc.UserID)
		require.Nil(t, mc.DeviceExpiresAt)
		require.True(t, mc.CodeExpiresAt.After(time.Now().Add(mfaCodeExpiryMinutes).Add(-time.Minute)))
	})

	t.Run("Test Device Exists: ✅ | Code Exists: ❌ | Code Valid: -", func(t *testing.T) {
		defer cleanup(t, ctx, dbConnectionPool)

		// Insert entry for `deviceID` and `randUser.ID`
		_, err := dbConnectionPool.ExecContext(ctx, `
			INSERT INTO auth_user_mfa_codes (device_id, auth_user_id, device_expires_at)
			VALUES ($1, $2, NOW() + INTERVAL '1 hour')`, "deviceID", randUser.ID)
		require.NoError(t, err)

		// Generate new code for `deviceID` and `randUser.ID`
		code, err := m.GenerateMFACode(ctx, "deviceID", randUser.ID)
		require.NoError(t, err)
		require.NotNil(t, code)
		require.Equal(t, 6, len(code))

		mc, err := m.getByDeviceAndCode(ctx, "deviceID", code)
		require.NoError(t, err)
		require.NotNil(t, mc)
		require.Equal(t, code, mc.Code)
		require.Equal(t, "deviceID", mc.DeviceID)
		require.Equal(t, randUser.ID, mc.UserID)
		require.True(t, mc.CodeExpiresAt.After(time.Now().Add(mfaCodeExpiryMinutes).Add(-time.Minute)))
	})

	t.Run("Test Device Exists: ✅ | Code Exists: ✅ | Code Valid: ❌", func(t *testing.T) {
		defer cleanup(t, ctx, dbConnectionPool)

		// Generate code and expire it
		expiredCode, err := m.GenerateMFACode(ctx, "deviceID", randUser.ID)
		require.NoError(t, err)
		_, err = dbConnectionPool.ExecContext(ctx, `
			UPDATE auth_user_mfa_codes SET code_expires_at = NOW() - INTERVAL '1 hour'
			WHERE device_id = $1 AND auth_user_id = $2`, "deviceID", randUser.ID)
		require.NoError(t, err)

		// Generate new code for `deviceID` and `randUser.ID`
		code, err := m.GenerateMFACode(ctx, "deviceID", randUser.ID)
		require.NoError(t, err)
		require.NotNil(t, code)
		require.Equal(t, 6, len(code))
		require.NotEqual(t, expiredCode, code)

		mc, err := m.getByDeviceAndCode(ctx, "deviceID", code)
		require.NoError(t, err)
		require.NotNil(t, mc)
		require.Equal(t, code, mc.Code)
		require.Equal(t, "deviceID", mc.DeviceID)
		require.Equal(t, randUser.ID, mc.UserID)
		require.Nil(t, mc.DeviceExpiresAt)
		require.True(t, mc.CodeExpiresAt.After(time.Now().Add(mfaCodeExpiryMinutes).Add(-time.Minute)))
	})

	t.Run("Test code expired and re-generated when valid one exists", func(t *testing.T) {
		defer cleanup(t, ctx, dbConnectionPool)

		// Generate code
		code, err := m.GenerateMFACode(ctx, "deviceID", randUser.ID)
		require.NoError(t, err)
		require.NotNil(t, code)
		require.Equal(t, 6, len(code))

		// Try generating another one
		newCode, err := m.GenerateMFACode(ctx, "deviceID", randUser.ID)
		require.NoError(t, err)
		require.NotEqual(t, newCode, code)
	})
}

func Test_defaultMFAManager_ValidateMFACode(t *testing.T) {
	ctx := context.Background()

	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, NewDefaultPasswordEncrypter(), false)

	m := newDefaultMFAManager(withMFADatabaseConnectionPool(dbConnectionPool))

	t.Run("Test error when deviceID or code is empty", func(t *testing.T) {
		_, err := m.ValidateMFACode(ctx, "", "")
		require.ErrorContains(t, err, "device ID and code are required")
		_, err = m.ValidateMFACode(ctx, "deviceID", "")
		require.ErrorContains(t, err, "device ID and code are required")
		_, err = m.ValidateMFACode(ctx, "", "code")
		require.ErrorContains(t, err, "device ID and code are required")
	})

	t.Run("Test MFA code validation", func(t *testing.T) {
		testDeviceID := "testDeviceID"
		testCode := "111333"
		_, err := dbConnectionPool.ExecContext(ctx, `
            INSERT INTO auth_user_mfa_codes (device_id, code, auth_user_id, device_expires_at, code_expires_at)
            VALUES ($1, $2, $3, NOW() + INTERVAL '1 hour', NOW() + INTERVAL '1 hour')`, testDeviceID, testCode, randUser.ID)
		require.NoError(t, err)

		// Test MFA code validation
		userID, err := m.ValidateMFACode(ctx, testDeviceID, testCode)
		assert.NoError(t, err)
		assert.Equal(t, randUser.ID, userID)
	})

	t.Run("Test invalid MFA code", func(t *testing.T) {
		testDeviceID := "anotherDeviceID"
		testCode := "222333"
		_, err := dbConnectionPool.ExecContext(ctx, `
            INSERT INTO auth_user_mfa_codes (device_id, code, auth_user_id, device_expires_at, code_expires_at)
            VALUES ($1, $2, $3, NOW() + INTERVAL '1 hour', NOW() - INTERVAL '1 hour')`, testDeviceID, testCode, randUser.ID)
		require.NoError(t, err)

		_, err = m.ValidateMFACode(ctx, testDeviceID, testCode)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrMFACodeInvalid))
	})
}

func Test_defaultMFAManager_RememberDevice(t *testing.T) {
	ctx := context.Background()

	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, NewDefaultPasswordEncrypter(), false)

	m := newDefaultMFAManager(withMFADatabaseConnectionPool(dbConnectionPool))

	t.Run("Test error when deviceID or code is empty", func(t *testing.T) {
		err := m.RememberDevice(ctx, "", "")
		require.ErrorContains(t, err, "device ID and code are required")
		err = m.RememberDevice(ctx, "deviceID", "")
		require.ErrorContains(t, err, "device ID and code are required")
		err = m.RememberDevice(ctx, "", "code")
		require.ErrorContains(t, err, "device ID and code are required")
	})

	t.Run("Test updating device expiry", func(t *testing.T) {
		testDeviceID := "testDeviceID"
		testCode := "111333"
		_, err := dbConnectionPool.ExecContext(ctx, `
            INSERT INTO auth_user_mfa_codes (device_id, code, auth_user_id, device_expires_at, code_expires_at)
            VALUES ($1, $2, $3, NOW() - INTERVAL '1 hour', NOW() + INTERVAL '1 hour')`, testDeviceID, testCode, randUser.ID)
		require.NoError(t, err)

		err = m.RememberDevice(ctx, testDeviceID, testCode)
		require.NoError(t, err)

		mc, err := m.getByDeviceAndUser(ctx, testDeviceID, randUser.ID)
		require.NoError(t, err)
		require.True(t, mc.DeviceExpiresAt.After(time.Now()))
	})
}

func Test_defaultMFAManager_ForgetDevice(t *testing.T) {
	ctx := context.Background()

	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, NewDefaultPasswordEncrypter(), false)

	m := newDefaultMFAManager(withMFADatabaseConnectionPool(dbConnectionPool))

	t.Run("Test error when deviceID or code is empty", func(t *testing.T) {
		err := m.ForgetDevice(ctx, "", "")
		require.EqualError(t, err, "device ID and user ID are required")
		err = m.ForgetDevice(ctx, "deviceID", "")
		require.EqualError(t, err, "device ID and user ID are required")
		err = m.ForgetDevice(ctx, "", "code")
		require.EqualError(t, err, "device ID and user ID are required")
	})

	t.Run("Test forget device", func(t *testing.T) {
		defer cleanup(t, ctx, dbConnectionPool)

		testDeviceID := "testDeviceID"

		// Generate code and remember device
		code, err := m.GenerateMFACode(ctx, testDeviceID, randUser.ID)
		require.NoError(t, err)
		require.Equal(t, 6, len(code))

		err = m.RememberDevice(ctx, testDeviceID, code)
		require.NoError(t, err)

		// Fetch entry and check that device is remembered
		mc, err := m.getByDeviceAndUser(ctx, testDeviceID, randUser.ID)
		require.NoError(t, err)
		require.NotNil(t, mc)
		require.True(t, mc.DeviceExpiresAt.After(time.Now()))

		// Forget device
		err = m.ForgetDevice(ctx, testDeviceID, randUser.ID)
		require.NoError(t, err)

		// Fetch entry and check that device is forgotten
		mc, err = m.getByDeviceAndUser(ctx, testDeviceID, randUser.ID)
		require.NoError(t, err)
		require.NotNil(t, mc)
		require.Nil(t, mc.DeviceExpiresAt)
	})
}

func Test_defaultMFAManager_getByDeviceAndCode(t *testing.T) {
	ctx := context.Background()

	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, NewDefaultPasswordEncrypter(), false)

	m := newDefaultMFAManager(withMFADatabaseConnectionPool(dbConnectionPool))

	t.Run("Test error when deviceID or code is empty", func(t *testing.T) {
		_, err := m.getByDeviceAndCode(ctx, "", "")
		require.EqualError(t, err, "device ID and code are required")
		_, err = m.getByDeviceAndCode(ctx, "deviceID", "")
		require.EqualError(t, err, "device ID and code are required")
		_, err = m.getByDeviceAndCode(ctx, "", "code")
		require.EqualError(t, err, "device ID and code are required")
	})

	t.Run("Test fetching MFA code by device and code", func(t *testing.T) {
		testDeviceID := "testDeviceID"
		testCode := "111333"
		_, err := dbConnectionPool.ExecContext(ctx, `
            INSERT INTO auth_user_mfa_codes (device_id, code, auth_user_id, code_expires_at)
            VALUES ($1, $2, $3, NOW() + INTERVAL '1 hour')`, testDeviceID, testCode, randUser.ID)
		require.NoError(t, err)

		mc, err := m.getByDeviceAndCode(ctx, testDeviceID, testCode)
		require.NoError(t, err)
		require.NotNil(t, mc)
		require.Equal(t, testCode, mc.Code)
		require.Equal(t, testDeviceID, mc.DeviceID)
		require.Equal(t, randUser.ID, mc.UserID)
		require.Nil(t, mc.DeviceExpiresAt)
		require.True(t, mc.CodeExpiresAt.After(time.Now().Add(mfaCodeExpiryMinutes).Add(-time.Minute)))
	})

	t.Run("Test fetching non-existent MFA code", func(t *testing.T) {
		testDeviceID := "testDeviceID"
		testCode := "nonExistentCode"

		// Test fetching MFA code
		_, err := m.getByDeviceAndCode(ctx, testDeviceID, testCode)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrMFANoCodeForUserDevice))
	})
}

func Test_defaultMFAManager_generateAndUpdateMFACode(t *testing.T) {
	ctx := context.Background()

	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, NewDefaultPasswordEncrypter(), false)

	m := newDefaultMFAManager(withMFADatabaseConnectionPool(dbConnectionPool))

	t.Run("Test generate and upsert new MFA code", func(t *testing.T) {
		testDeviceID := "testDeviceID"

		generatedCode, err := m.generateAndUpdateMFACode(ctx, testDeviceID, randUser.ID)
		require.NoError(t, err)
		require.NotEmpty(t, generatedCode)

		mc, err := m.getByDeviceAndUser(ctx, testDeviceID, randUser.ID)
		require.NoError(t, err)
		require.Equal(t, generatedCode, mc.Code)
		require.Equal(t, testDeviceID, mc.DeviceID)
		require.Equal(t, randUser.ID, mc.UserID)
		require.Nil(t, mc.DeviceExpiresAt)
		require.True(t, mc.CodeExpiresAt.After(time.Now().Add(mfaCodeExpiryMinutes).Add(-time.Minute)))
	})
}

func Test_defaultMFAManager_upsertMFACode(t *testing.T) {
	ctx := context.Background()

	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, NewDefaultPasswordEncrypter(), false)

	m := newDefaultMFAManager(withMFADatabaseConnectionPool(dbConnectionPool))

	t.Run("Test upsert new MFA code", func(t *testing.T) {
		testDeviceID := "testDeviceID"
		testCode := "111333"

		// Test inserting new MFA code
		err := m.upsertMFACode(ctx, testDeviceID, randUser.ID, testCode)
		assert.NoError(t, err)

		// Check that the record was inserted correctly
		mc, err := m.getByDeviceAndUser(ctx, testDeviceID, randUser.ID)
		require.NoError(t, err)
		assert.Equal(t, testCode, mc.Code)

		// Cleanup: Delete the test record
		_, err = dbConnectionPool.ExecContext(ctx, `
            DELETE FROM auth_user_mfa_codes WHERE device_id = $1 AND auth_user_id = $2`, testDeviceID, randUser.ID)
		require.NoError(t, err)
	})

	t.Run("Test update existing MFA code", func(t *testing.T) {
		testDeviceID := "testDeviceID"
		testCode := "111333"
		_, err := dbConnectionPool.ExecContext(ctx, `
            INSERT INTO auth_user_mfa_codes (device_id, code, auth_user_id, code_expires_at)
            VALUES ($1, $2, $3, NOW() + INTERVAL '1 hour')`, testDeviceID, testCode, randUser.ID)
		require.NoError(t, err)

		// Test updating existing MFA code
		newCode := "222444"
		err = m.upsertMFACode(ctx, testDeviceID, randUser.ID, newCode)
		assert.NoError(t, err)

		// Check that the record was updated correctly
		mc, err := m.getByDeviceAndUser(ctx, testDeviceID, randUser.ID)
		require.NoError(t, err)
		assert.Equal(t, newCode, mc.Code)

		// Cleanup: Delete the test record
		_, err = dbConnectionPool.ExecContext(ctx, `
            DELETE FROM auth_user_mfa_codes WHERE device_id = $1 AND auth_user_id = $2`, testDeviceID, randUser.ID)
		require.NoError(t, err)
	})
}

func Test_defaultMFAManager_resetDeviceExpiry(t *testing.T) {
	ctx := context.Background()

	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, NewDefaultPasswordEncrypter(), false)

	m := newDefaultMFAManager(withMFADatabaseConnectionPool(dbConnectionPool))

	t.Run("Test error when deviceID or code is empty", func(t *testing.T) {
		err := m.resetDeviceExpiry(ctx, "", "")
		assert.EqualError(t, err, "device ID and code are required")
		err = m.resetDeviceExpiry(ctx, "deviceID", "")
		assert.EqualError(t, err, "device ID and code are required")
		err = m.resetDeviceExpiry(ctx, "", "code")
		assert.EqualError(t, err, "device ID and code are required")
	})

	t.Run("Test device expiry reset", func(t *testing.T) {
		testDeviceID := "testDeviceID"
		testCode := "111333"
		_, err := dbConnectionPool.ExecContext(ctx, `
            INSERT INTO auth_user_mfa_codes (device_id, code, auth_user_id, code_expires_at)
            VALUES ($1, $2, $3, NOW() + INTERVAL '1 hour')`, testDeviceID, testCode, randUser.ID)
		require.NoError(t, err)

		err = m.resetDeviceExpiry(ctx, testDeviceID, testCode)
		assert.NoError(t, err)

		// Check that the record was updated correctly
		mc, err := m.getByDeviceAndUser(ctx, testDeviceID, randUser.ID)
		require.NoError(t, err)
		require.True(t, mc.DeviceExpiresAt.After(time.Now().Add(mfaDeviceExpiryHours).Add(-time.Minute)))
	})
}

func Test_defaultMFAManager_expireMFACode(t *testing.T) {
	ctx := context.Background()

	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, NewDefaultPasswordEncrypter(), false)

	m := newDefaultMFAManager(withMFADatabaseConnectionPool(dbConnectionPool))

	t.Run("Test error when deviceID or code is empty", func(t *testing.T) {
		err := m.expireMFACode(ctx, "", "")
		assert.EqualError(t, err, "device ID and code are required")
		err = m.expireMFACode(ctx, "deviceID", "")
		assert.EqualError(t, err, "device ID and code are required")
		err = m.expireMFACode(ctx, "", "code")
		assert.EqualError(t, err, "device ID and code are required")
	})

	t.Run("Test entry not found", func(t *testing.T) {
		testDeviceID := "testDeviceID"
		testCode := "111333"
		_, err := dbConnectionPool.ExecContext(ctx, `
            INSERT INTO auth_user_mfa_codes (device_id, code, auth_user_id, code_expires_at)
            VALUES ($1, $2, $3, NOW() + INTERVAL '1 hour')`, testDeviceID, testCode, randUser.ID)
		require.NoError(t, err)

		err = m.expireMFACode(ctx, testDeviceID, testCode)
		assert.NoError(t, err)

		// Check that the record was updated correctly
		mc, err := m.getByDeviceAndUser(ctx, testDeviceID, randUser.ID)
		require.NoError(t, err)
		require.Nil(t, mc.CodeExpiresAt)
		require.Equal(t, "", mc.Code)
	})
}

func Test_defaultMFAManager_generateMFACode(t *testing.T) {
	code, err := generateMFACode()
	assert.NoError(t, err)
	assert.Equal(t, 6, len(code))
	for _, c := range code {
		assert.True(t, c >= '0' && c <= '9')
	}
}

func cleanup(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool) {
	_, err := dbConnectionPool.ExecContext(ctx, "DELETE FROM auth_user_mfa_codes")
	require.NoError(t, err)
}
