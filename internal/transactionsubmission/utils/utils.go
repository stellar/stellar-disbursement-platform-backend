package utils

import (
	"context"
	"fmt"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	sdpUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// AcquireAdvisoryLock attempt to acquire an advisory lock on the provided lockKey, returns true if acquired, or false
// not.
func AcquireAdvisoryLock(ctx context.Context, dbConnectionPool db.DBConnectionPool, lockKey int) (bool, error) {
	tssAdvisoryLockAcquired := false
	sqlQuery := "SELECT pg_try_advisory_lock($1)"
	err := dbConnectionPool.QueryRowxContext(ctx, sqlQuery, lockKey).Scan(&tssAdvisoryLockAcquired)
	if err != nil {
		return false, fmt.Errorf("querying pg_try_advisory_lock(%v): %w", lockKey, err)
	}
	return tssAdvisoryLockAcquired, nil
}

type PrivateKeyEncrypter interface {
	Encrypt(message string, passphrase string) (string, error)
	Decrypt(message string, passphrase string) (string, error)
}

type DefaultPrivateKeyEncrypter struct{}

func (e DefaultPrivateKeyEncrypter) Encrypt(message, passphrase string) (string, error) {
	return sdpUtils.Encrypt(message, passphrase)
}

func (e DefaultPrivateKeyEncrypter) Decrypt(message, passphrase string) (string, error) {
	return sdpUtils.Decrypt(message, passphrase)
}

// Making sure that DefaultPrivateKeyEncrypter implements PrivateKeyEncrypter
var _ PrivateKeyEncrypter = (*DefaultPrivateKeyEncrypter)(nil)
