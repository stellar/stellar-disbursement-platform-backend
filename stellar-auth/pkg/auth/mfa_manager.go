package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

type MFAManager interface {
	MFADeviceRemembered(ctx context.Context, deviceID, userID string) (bool, error)
	GenerateMFACode(ctx context.Context, deviceID, userID string) (string, error)
	ValidateMFACode(ctx context.Context, deviceID, code string) (string, error)
	RememberDevice(ctx context.Context, deviceID, userID string) error
	ForgetAllDevices(ctx context.Context, userID string) error
}

// defaultMFAManager
type defaultMFAManager struct {
	dbConnectionPool db.DBConnectionPool
}

const (
	mfaCodeMaxLength         = 6
	mfaDeviceExpiryHours     = time.Hour * 24 * 7 // 7 days
	mfaCodeExpiryMinutes     = time.Minute * 5    // 5 minutes
	mfaMaxValidationAttempts = 5                  // failed validations allowed per device before a new code is required
)

var (
	ErrMFACodeInvalid         = errors.New("MFA code is invalid")
	ErrMFANoCodeForUserDevice = errors.New("no MFA code for user and device")
	ErrMFAAttemptsExhausted   = errors.New("too many incorrect MFA code attempts")
)

type mfaCode struct {
	DeviceID        string     `db:"device_id"`
	UserID          string     `db:"auth_user_id"`
	Code            string     `db:"code"`
	DeviceExpiresAt *time.Time `db:"device_expires_at"`
	CodeExpiresAt   *time.Time `db:"code_expires_at"`
	Attempts        int        `db:"attempts"`
}

// MFADeviceRemembered checks if the device is remembered for the user.
func (m *defaultMFAManager) MFADeviceRemembered(ctx context.Context, deviceID, userID string) (bool, error) {
	mc, err := m.getByDeviceAndUser(ctx, deviceID, userID)
	if err != nil {
		if errors.Is(err, ErrMFANoCodeForUserDevice) {
			return false, nil
		}
		return false, fmt.Errorf("error validating MFA device for token string %s and device ID %s: %w", userID, deviceID, err)
	}

	// 1. Device Exists: ❌ | Device Valid: – |
	// 2. Device Exists: ✅ | Device Valid: – |
	// 3. Device Exists: ✅ | Device Valid: ❌
	if mc == nil ||
		mc.DeviceExpiresAt == nil ||
		(mc.DeviceExpiresAt != nil && mc.DeviceExpiresAt.Before(time.Now())) {
		return false, nil
	}

	// 4. Device Exists: ✅ | Device Valid: ✅
	return true, nil
}

// GenerateMFACode generates a new MFA code for the user and device.
func (m *defaultMFAManager) GenerateMFACode(ctx context.Context, deviceID, userID string) (string, error) {
	mc, err := m.getByDeviceAndUser(ctx, deviceID, userID)
	if err != nil && !errors.Is(err, ErrMFANoCodeForUserDevice) {
		return "", fmt.Errorf("error validating MFA device for user ID %s and device ID %s: %w", userID, deviceID, err)
	}

	// 1. Device Exists: ❌ |  Code Exists: -  | Code Valid: -
	// 2. Device Exists: ✅ |  Code Exists: ❌ | Code Valid: -
	// 3. Device Exists: ✅ |  Code Exists: ✅ | Code Valid: ❌
	//    ⤷ Persist & send new code
	if mc == nil || mc.Code == "" || (mc.CodeExpiresAt != nil && mc.CodeExpiresAt.Before(time.Now())) {
		return m.generateAndUpdateMFACode(ctx, deviceID, userID)
	}

	// 4. Device Exists: ✅ |  Code Exists: ✅ | Code Valid: ✅
	//    ⤷ Explicitly expire the old code and generate a new one.
	if mc.CodeExpiresAt != nil && mc.CodeExpiresAt.After(time.Now()) {
		log.Ctx(ctx).Infof("expiring a valid MFA code for device ID %s and user ID %s", deviceID, userID)
		err = m.expireMFACode(ctx, deviceID, mc.Code)
		if err != nil {
			return "", fmt.Errorf("expiring MFA code for device ID %s and code %s: %w", deviceID, mc.Code, err)
		}
		return m.generateAndUpdateMFACode(ctx, deviceID, userID)
	}

	return "", nil
}

// ValidateMFACode checks if the MFA code is valid for the device ID and returns the user ID.
// Each failed validation increments a per-device counter; once mfaMaxValidationAttempts is
// reached the device is locked out (ErrMFAAttemptsExhausted) until a new code is issued.
func (m *defaultMFAManager) ValidateMFACode(ctx context.Context, deviceID, code string) (string, error) {
	mc, err := m.getByDeviceAndCode(ctx, deviceID, code)
	if err != nil {
		if errors.Is(err, ErrMFANoCodeForUserDevice) {
			// The submitted code matches no pending code for the device. Count the failed
			// attempt against the device's live code and lock out once the cap is reached.
			return "", m.failValidation(ctx, deviceID)
		}
		return "", fmt.Errorf("error validating MFA code for device ID %s: %w", deviceID, err)
	}

	// The device is already locked out: reject even a correct code until a new one is issued.
	if mc.Attempts >= mfaMaxValidationAttempts {
		return "", ErrMFAAttemptsExhausted
	}

	if mc.Code == code && mc.CodeExpiresAt != nil && mc.CodeExpiresAt.After(time.Now()) {
		err = m.expireMFACode(ctx, deviceID, code)
		if err != nil {
			return "", fmt.Errorf("error expiring MFA code for device ID %s and code %s: %w", deviceID, code, err)
		}
		return mc.UserID, nil
	}

	// The code matched a row but was expired: count it as a failed attempt too.
	return "", m.failValidation(ctx, deviceID)
}

// failValidation records a failed MFA validation for the device and returns the error to
// surface: ErrMFAAttemptsExhausted once the attempt cap is reached, otherwise ErrMFACodeInvalid.
func (m *defaultMFAManager) failValidation(ctx context.Context, deviceID string) error {
	exhausted, err := m.registerFailedMFAAttempt(ctx, deviceID)
	if err != nil {
		return err
	}
	if exhausted {
		return ErrMFAAttemptsExhausted
	}
	return ErrMFACodeInvalid
}

// RememberDevice updates the device expiry for the device.
func (m *defaultMFAManager) RememberDevice(ctx context.Context, deviceID, userID string) error {
	err := m.resetDeviceExpiry(ctx, deviceID, userID)
	if err != nil {
		return fmt.Errorf("error updating device expiry for device ID %s and user ID %s: %w", deviceID, userID, err)
	}
	return nil
}

// ForgetDevice expires the device for the user.
func (m *defaultMFAManager) ForgetDevice(ctx context.Context, deviceID, userID string) error {
	if deviceID == "" || userID == "" {
		return fmt.Errorf("device ID and user ID are required")
	}

	const query = `
		UPDATE auth_user_mfa_codes
		SET device_expires_at = null
		WHERE device_id = $1 AND auth_user_id = $2
	`
	_, err := m.dbConnectionPool.ExecContext(ctx, query, deviceID, userID)
	if err != nil {
		return fmt.Errorf("error expiring device for device ID %s and user ID %s: %w", deviceID, userID, err)
	}
	return nil
}

// ForgetAllDevices expires every remembered device for the user, revoking all
// trusted "remember me" sessions. It is used when the user's password changes.
func (m *defaultMFAManager) ForgetAllDevices(ctx context.Context, userID string) error {
	if userID == "" {
		return fmt.Errorf("user ID is required")
	}

	const query = `
		UPDATE auth_user_mfa_codes
		SET device_expires_at = null
		WHERE auth_user_id = $1
	`
	_, err := m.dbConnectionPool.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("error expiring all devices for user ID %s: %w", userID, err)
	}
	return nil
}

// registerFailedMFAAttempt increments the failed-attempt counter for the device's live MFA
// code and reports whether the per-device attempt cap has now been reached.
func (m *defaultMFAManager) registerFailedMFAAttempt(ctx context.Context, deviceID string) (bool, error) {
	if deviceID == "" {
		return false, fmt.Errorf("device ID is required")
	}
	const query = `
		UPDATE auth_user_mfa_codes
		SET attempts = LEAST(attempts + 1, $2)
		WHERE device_id = $1 AND code_expires_at > NOW()
		RETURNING attempts
	`
	var attempts int
	err := m.dbConnectionPool.GetContext(ctx, &attempts, query, deviceID, mfaMaxValidationAttempts)
	if err != nil {
		// No live code matched (expired or absent): nothing to count against, not a lockout.
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("error registering failed MFA attempt for device ID %s: %w", deviceID, err)
	}
	return attempts >= mfaMaxValidationAttempts, nil
}

// getByDeviceAndUser gets the MFA code for the user and device.
func (m *defaultMFAManager) getByDeviceAndUser(ctx context.Context, deviceID, userID string) (*mfaCode, error) {
	if deviceID == "" || userID == "" {
		return nil, fmt.Errorf("device ID and user ID are required")
	}
	const query = `
		SELECT
		    device_id,
		    auth_user_id,
		    COALESCE(code, '') AS code,
		    device_expires_at,
		    code_expires_at,
		    attempts
		FROM
		    auth_user_mfa_codes
		WHERE
		    device_id = $1 AND
		    auth_user_id = $2
	`
	var mc mfaCode
	err := m.dbConnectionPool.GetContext(ctx, &mc, query, deviceID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrMFANoCodeForUserDevice
		}
		return nil, fmt.Errorf("error fetching MFA code for device ID %s and user ID %s: %w", deviceID, userID, err)
	}

	return &mc, nil
}

// getByDeviceAndCode gets the MFA code for the device and code.
func (m *defaultMFAManager) getByDeviceAndCode(ctx context.Context, deviceID, code string) (*mfaCode, error) {
	if deviceID == "" || code == "" {
		return nil, fmt.Errorf("device ID and code are required")
	}
	const query = `
		SELECT
		    device_id,
		    auth_user_id,
		    COALESCE(code, '') AS code,
		    device_expires_at,
		    code_expires_at,
		    attempts
		FROM
		    auth_user_mfa_codes
		WHERE
		    device_id = $1 AND
		    code = $2
	`
	var mc mfaCode
	err := m.dbConnectionPool.GetContext(ctx, &mc, query, deviceID, code)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrMFANoCodeForUserDevice
		}
		return nil, fmt.Errorf("error fetching MFA code for device ID %s: %w", deviceID, err)
	}

	return &mc, nil
}

// generateAndUpdateMFACode generates a new MFA code and upserts it for the user and device.
func (m *defaultMFAManager) generateAndUpdateMFACode(ctx context.Context, deviceID, userID string) (string, error) {
	code, err := generateMFACode()
	if err != nil {
		return "", fmt.Errorf("error generating MFA code for user ID %s and device ID %s: %w", userID, deviceID, err)
	}
	err = m.upsertMFACode(ctx, deviceID, userID, code)
	if err != nil {
		return "", fmt.Errorf("error updating MFA code for user ID %s and device ID %s: %w", userID, deviceID, err)
	}
	return code, nil
}

// upsertMFACode upserts the MFA code for the user and device.
func (m *defaultMFAManager) upsertMFACode(ctx context.Context, deviceID, userID, code string) error {
	if deviceID == "" || userID == "" || code == "" {
		return fmt.Errorf("device ID, user ID and code are required")
	}
	// A freshly issued code always starts with a clean attempt counter, so the "Resend code"
	// flow lifts any prior lockout for the device.
	const query = `
		INSERT INTO auth_user_mfa_codes (auth_user_id, device_id, code, code_expires_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (auth_user_id, device_id)
		DO UPDATE SET code = $3, code_expires_at = $4, attempts = 0
	`
	_, err := m.dbConnectionPool.ExecContext(ctx, query, userID, deviceID, code, time.Now().Add(mfaCodeExpiryMinutes))
	if err != nil {
		return fmt.Errorf("error upserting MFA code for user ID %s and device ID %s: %w", userID, deviceID, err)
	}
	return nil
}

// resetDeviceExpiry resets the device expiry for the user and device.
func (m *defaultMFAManager) resetDeviceExpiry(ctx context.Context, deviceID, userID string) error {
	if deviceID == "" || userID == "" {
		return fmt.Errorf("device ID and user ID are required")
	}
	const query = `
		UPDATE auth_user_mfa_codes
		SET device_expires_at = $1
		WHERE device_id = $2 AND auth_user_id = $3
	`
	_, err := m.dbConnectionPool.ExecContext(ctx, query, time.Now().Add(mfaDeviceExpiryHours), deviceID, userID)
	if err != nil {
		return fmt.Errorf("error updating device expiry for device ID %s and user ID %s: %w", deviceID, userID, err)
	}
	return nil
}

// expireMFACode expires the MFA code for the user and device.
func (m *defaultMFAManager) expireMFACode(ctx context.Context, deviceID, code string) error {
	if deviceID == "" || code == "" {
		return fmt.Errorf("device ID and code are required")
	}
	const query = `
		UPDATE auth_user_mfa_codes
		SET code = null, code_expires_at = null, attempts = 0
		WHERE device_id = $1 AND code = $2
	`
	_, err := m.dbConnectionPool.ExecContext(ctx, query, deviceID, code)
	if err != nil {
		return fmt.Errorf("error expiring MFA code for device ID %s and code %s: %w", deviceID, code, err)
	}
	return nil
}

// generateMFACode generate a random 6-digit MFA code.
func generateMFACode() (string, error) {
	code := ""
	for i := 0; i < mfaCodeMaxLength; i++ {
		randomDigit, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", fmt.Errorf("error generating random digit for MFA code: %w", err)
		}
		code += fmt.Sprintf("%d", randomDigit)
	}
	return code, nil
}

type defaultMFAManagerOption func(m *defaultMFAManager)

func newDefaultMFAManager(options ...defaultMFAManagerOption) *defaultMFAManager {
	mfaManager := &defaultMFAManager{}

	for _, option := range options {
		option(mfaManager)
	}

	return mfaManager
}

func withMFADatabaseConnectionPool(dbConnectionPool db.DBConnectionPool) defaultMFAManagerOption {
	return func(a *defaultMFAManager) {
		a.dbConnectionPool = dbConnectionPool
	}
}

// Ensuring that defaultMFAManager is implementing MFAManager interface
var _ MFAManager = (*defaultMFAManager)(nil)
