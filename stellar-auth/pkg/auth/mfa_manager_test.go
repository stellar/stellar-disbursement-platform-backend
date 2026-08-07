package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
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
		_, err := m.GenerateMFACode(ctx, "deviceID", randUser.ID)
		require.NoError(t, err)
		err = m.RememberDevice(ctx, "deviceID", randUser.ID)
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

	t.Run("Test device is locked out after reaching the max attempts", func(t *testing.T) {
		testDeviceID := "lockout-device"
		testCode := "424242"
		_, err := dbConnectionPool.ExecContext(ctx, `
            INSERT INTO auth_user_mfa_codes (device_id, code, auth_user_id, device_expires_at, code_expires_at)
            VALUES ($1, $2, $3, NOW() + INTERVAL '1 hour', NOW() + INTERVAL '1 hour')`, testDeviceID, testCode, randUser.ID)
		require.NoError(t, err)

		// The first (max-1) wrong guesses report an invalid code.
		for i := 0; i < mfaMaxValidationAttempts-1; i++ {
			_, err = m.ValidateMFACode(ctx, testDeviceID, "000000")
			require.ErrorIs(t, err, ErrMFACodeInvalid)
		}

		// The guess that reaches the cap reports the device as locked out.
		_, err = m.ValidateMFACode(ctx, testDeviceID, "000000")
		require.ErrorIs(t, err, ErrMFAAttemptsExhausted)

		// Even the correct code is now rejected until a new one is issued.
		_, err = m.ValidateMFACode(ctx, testDeviceID, testCode)
		require.ErrorIs(t, err, ErrMFAAttemptsExhausted)
	})

	t.Run("Test issuing a new code resets the attempt counter", func(t *testing.T) {
		testDeviceID := "reset-device"
		oldCode := "111111"
		_, err := dbConnectionPool.ExecContext(ctx, `
            INSERT INTO auth_user_mfa_codes (device_id, code, auth_user_id, device_expires_at, code_expires_at, attempts)
            VALUES ($1, $2, $3, NOW() + INTERVAL '1 hour', NOW() + INTERVAL '1 hour', $4)`,
			testDeviceID, oldCode, randUser.ID, mfaMaxValidationAttempts)
		require.NoError(t, err)

		// The device starts locked out.
		_, err = m.ValidateMFACode(ctx, testDeviceID, oldCode)
		require.ErrorIs(t, err, ErrMFAAttemptsExhausted)

		// Issuing a new code (as the Resend code flow does) clears the counter.
		newCode := "222222"
		err = m.upsertMFACode(ctx, testDeviceID, randUser.ID, newCode)
		require.NoError(t, err)

		userID, err := m.ValidateMFACode(ctx, testDeviceID, newCode)
		require.NoError(t, err)
		assert.Equal(t, randUser.ID, userID)
	})

	t.Run("Test failed guesses against an expired code do not count toward the lockout", func(t *testing.T) {
		testDeviceID := "expired-code-device"
		testCode := "424243"
		// Seed an already-expired code (still present in the row, timestamp in the past).
		_, err := dbConnectionPool.ExecContext(ctx, `
            INSERT INTO auth_user_mfa_codes (device_id, code, auth_user_id, device_expires_at, code_expires_at, attempts)
            VALUES ($1, $2, $3, NOW() + INTERVAL '1 hour', NOW() - INTERVAL '1 minute', 0)`, testDeviceID, testCode, randUser.ID)
		require.NoError(t, err)

		// Many wrong guesses against the dead code stay "invalid" and never trigger a lockout,
		// because an expired code can never succeed. The remedy is to request a new code.
		for i := 0; i < mfaMaxValidationAttempts+2; i++ {
			_, err = m.ValidateMFACode(ctx, testDeviceID, "000000")
			require.ErrorIs(t, err, ErrMFACodeInvalid)
		}

		// The attempt counter never moved.
		mc, err := m.getByDeviceAndUser(ctx, testDeviceID, randUser.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, mc.Attempts)
	})

	t.Run("Test the last allowed attempt succeeds and clears the counter", func(t *testing.T) {
		testDeviceID := "boundary-device"
		testCode := "555555"
		// Seed the device one attempt below the cap: a correct code must still be accepted.
		_, err := dbConnectionPool.ExecContext(ctx, `
            INSERT INTO auth_user_mfa_codes (device_id, code, auth_user_id, device_expires_at, code_expires_at, attempts)
            VALUES ($1, $2, $3, NOW() + INTERVAL '1 hour', NOW() + INTERVAL '1 hour', $4)`,
			testDeviceID, testCode, randUser.ID, mfaMaxValidationAttempts-1)
		require.NoError(t, err)

		userID, err := m.ValidateMFACode(ctx, testDeviceID, testCode)
		require.NoError(t, err)
		assert.Equal(t, randUser.ID, userID)

		// A successful validation spends the code and resets the attempt counter.
		mc, err := m.getByDeviceAndUser(ctx, testDeviceID, randUser.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, mc.Attempts)
		assert.Empty(t, mc.Code)
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

	t.Run("Test error when deviceID or userID is empty", func(t *testing.T) {
		err := m.RememberDevice(ctx, "", "")
		require.ErrorContains(t, err, "device ID and user ID are required")
		err = m.RememberDevice(ctx, "deviceID", "")
		require.ErrorContains(t, err, "device ID and user ID are required")
		err = m.RememberDevice(ctx, "", "userID")
		require.ErrorContains(t, err, "device ID and user ID are required")
	})

	t.Run("Test updating device expiry", func(t *testing.T) {
		testDeviceID := "testDeviceID"
		testCode := "111333"
		_, err := dbConnectionPool.ExecContext(ctx, `
            INSERT INTO auth_user_mfa_codes (device_id, code, auth_user_id, device_expires_at, code_expires_at)
            VALUES ($1, $2, $3, NOW() - INTERVAL '1 hour', NOW() + INTERVAL '1 hour')`, testDeviceID, testCode, randUser.ID)
		require.NoError(t, err)

		err = m.RememberDevice(ctx, testDeviceID, randUser.ID)
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

		err = m.RememberDevice(ctx, testDeviceID, randUser.ID)
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

func Test_defaultMFAManager_ForgetAllDevices(t *testing.T) {
	ctx := context.Background()

	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, NewDefaultPasswordEncrypter(), false)
	otherUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, NewDefaultPasswordEncrypter(), false)

	m := newDefaultMFAManager(withMFADatabaseConnectionPool(dbConnectionPool))

	t.Run("Test error when userID is empty", func(t *testing.T) {
		err := m.ForgetAllDevices(ctx, "")
		require.EqualError(t, err, "user ID is required")
	})

	t.Run("Test forget all devices for the user without affecting other users", func(t *testing.T) {
		defer cleanup(t, ctx, dbConnectionPool)

		// Remember two devices for the user, and one for a different user.
		for _, deviceID := range []string{"deviceA", "deviceB"} {
			_, err := m.GenerateMFACode(ctx, deviceID, randUser.ID)
			require.NoError(t, err)
			require.NoError(t, m.RememberDevice(ctx, deviceID, randUser.ID))
		}
		_, err := m.GenerateMFACode(ctx, "deviceC", otherUser.ID)
		require.NoError(t, err)
		require.NoError(t, m.RememberDevice(ctx, "deviceC", otherUser.ID))

		// Forget every device for the user.
		err = m.ForgetAllDevices(ctx, randUser.ID)
		require.NoError(t, err)

		// Both of the user's devices are forgotten.
		for _, deviceID := range []string{"deviceA", "deviceB"} {
			mc, getErr := m.getByDeviceAndUser(ctx, deviceID, randUser.ID)
			require.NoError(t, getErr)
			require.NotNil(t, mc)
			require.Nil(t, mc.DeviceExpiresAt)
		}

		// The other user's device is untouched.
		mc, err := m.getByDeviceAndUser(ctx, "deviceC", otherUser.ID)
		require.NoError(t, err)
		require.NotNil(t, mc)
		require.True(t, mc.DeviceExpiresAt.After(time.Now()))
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

	t.Run("Test error when deviceID or userID is empty", func(t *testing.T) {
		err := m.resetDeviceExpiry(ctx, "", "")
		assert.EqualError(t, err, "device ID and user ID are required")
		err = m.resetDeviceExpiry(ctx, "deviceID", "")
		assert.EqualError(t, err, "device ID and user ID are required")
		err = m.resetDeviceExpiry(ctx, "", "userID")
		assert.EqualError(t, err, "device ID and user ID are required")
	})

	t.Run("Test device expiry reset", func(t *testing.T) {
		testDeviceID := "testDeviceID"
		testCode := "111333"
		_, err := dbConnectionPool.ExecContext(ctx, `
            INSERT INTO auth_user_mfa_codes (device_id, code, auth_user_id, code_expires_at)
            VALUES ($1, $2, $3, NOW() + INTERVAL '1 hour')`, testDeviceID, testCode, randUser.ID)
		require.NoError(t, err)

		err = m.resetDeviceExpiry(ctx, testDeviceID, randUser.ID)
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
