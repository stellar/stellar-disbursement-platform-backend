package circle

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
)

func Test_ClientConfigModel_Upsert_Update(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	ccm := &ClientConfigModel{DBConnectionPool: dbConnectionPool}

	walletID := "the_wallet_id"
	encryptedAPIKey := "the_encrypted_api_key"
	encrypterPublicKey := "the_encrypter_public_key"

	updatedWalletID := "another_wallet_id"
	updatedEncryptedAPIKey := "another_encrypted_api_key"
	updatedEncrypterPublicKey := "another_encrypter_public_key"

	outerErr = ccm.insert(ctx, dbConnectionPool, ClientConfigUpdate{
		WalletID:           &walletID,
		EncryptedAPIKey:    &encryptedAPIKey,
		EncrypterPublicKey: &encrypterPublicKey,
	})
	require.NoError(t, outerErr)

	t.Run("update existing config", func(t *testing.T) {
		// Ensure there is an existing config
		cc, err := ccm.Get(ctx)
		require.NoError(t, err)

		// Verify the existing config
		assert.Equal(t, walletID, *cc.WalletID)
		assert.Equal(t, encryptedAPIKey, *cc.EncryptedAPIKey)
		assert.Equal(t, encrypterPublicKey, *cc.EncrypterPublicKey)

		err = ccm.Upsert(ctx, ClientConfigUpdate{
			WalletID:           &updatedWalletID,
			EncryptedAPIKey:    &updatedEncryptedAPIKey,
			EncrypterPublicKey: &updatedEncrypterPublicKey,
		})
		assert.NoError(t, err)

		cc, err = ccm.Get(ctx)
		require.NoError(t, err)
		require.NotNil(t, cc)
		assert.Equal(t, updatedWalletID, *cc.WalletID)
		assert.Equal(t, updatedEncryptedAPIKey, *cc.EncryptedAPIKey)
		assert.Equal(t, updatedEncrypterPublicKey, *cc.EncrypterPublicKey)
	})

	t.Run("return error on validation failure", func(t *testing.T) {
		err := ccm.Upsert(ctx, ClientConfigUpdate{
			WalletID:           nil,
			EncryptedAPIKey:    nil,
			EncrypterPublicKey: nil,
		})
		assert.Error(t, err)
		assert.ErrorContains(t, err, "invalid circle config for update: wallet_id or encrypted_api_key must be provided")
	})
}

func Test_ClientConfigModel_Upsert_Insert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	ccm := &ClientConfigModel{DBConnectionPool: dbConnectionPool}

	walletID := "the_wallet_id"
	encryptedAPIKey := "the_encrypted_api_key"
	encrypterPublicKey := "the_encrypter_public_key"

	t.Run("return error on validation failure for no values", func(t *testing.T) {
		err := ccm.Upsert(ctx, ClientConfigUpdate{
			WalletID:           nil,
			EncryptedAPIKey:    nil,
			EncrypterPublicKey: nil,
		})
		assert.Error(t, err)
		assert.ErrorContains(t, err, "invalid circle config for insert: wallet_id, encrypted_api_key, and encrypter_public_key must be provided")
	})

	t.Run("return error on validation failure for partial values", func(t *testing.T) {
		err := ccm.Upsert(ctx, ClientConfigUpdate{
			WalletID:           &walletID,
			EncryptedAPIKey:    nil,
			EncrypterPublicKey: nil,
		})
		assert.Error(t, err)
		assert.ErrorContains(t, err, "invalid circle config for insert: wallet_id, encrypted_api_key, and encrypter_public_key must be provided")
	})

	t.Run("insert new config", func(t *testing.T) {
		// Ensure there is an existing config
		cc, err := ccm.Get(ctx)
		assert.NoError(t, err)
		assert.Nil(t, cc)

		err = ccm.Upsert(ctx, ClientConfigUpdate{
			WalletID:           &walletID,
			EncryptedAPIKey:    &encryptedAPIKey,
			EncrypterPublicKey: &encrypterPublicKey,
		})
		assert.NoError(t, err)

		cc, err = ccm.Get(ctx)
		require.NoError(t, err)
		require.NotNil(t, cc)
		assert.Equal(t, walletID, *cc.WalletID)
		assert.Equal(t, encryptedAPIKey, *cc.EncryptedAPIKey)
		assert.Equal(t, encrypterPublicKey, *cc.EncrypterPublicKey)
	})
}

func Test_ClientConfigModel_get(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	ccm := &ClientConfigModel{DBConnectionPool: dbConnectionPool}

	walletID := "the_wallet_id"
	encryptedAPIKey := "the_encrypted_api_key"
	encrypterPublicKey := "the_encrypter_public_key"

	t.Run("retrieve existing config successfully", func(t *testing.T) {
		tx := testutils.BeginTxWithRollback(t, ctx, dbConnectionPool)

		// Insert a record to retrieve
		insertErr := ccm.insert(ctx, tx, ClientConfigUpdate{
			WalletID:           &walletID,
			EncryptedAPIKey:    &encryptedAPIKey,
			EncrypterPublicKey: &encrypterPublicKey,
		})
		require.NoError(t, insertErr)

		config, err := ccm.get(ctx, tx)
		require.NoError(t, err)
		require.NotNil(t, config)
		assert.Equal(t, walletID, *config.WalletID)
		assert.Equal(t, encryptedAPIKey, *config.EncryptedAPIKey)
		assert.Equal(t, encrypterPublicKey, *config.EncrypterPublicKey)
	})

	t.Run("return nil if no config exists", func(t *testing.T) {
		tx := testutils.BeginTxWithRollback(t, ctx, dbConnectionPool)

		config, err := ccm.get(ctx, tx)
		require.NoError(t, err)
		assert.Nil(t, config)
	})
}

func Test_ClientConfigModel_update(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	ccm := &ClientConfigModel{DBConnectionPool: dbConnectionPool}

	walletID := "the_wallet_id"
	encryptedAPIKey := "the_encrypted_api_key"
	encrypterPublicKey := "the_encrypter_public_key"

	updatedWalletID := "another_wallet_id"
	updatedEncryptedAPIKey := "another_encrypted_api_key"
	updatedEncrypterPublicKey := "another_encrypter_public_key"

	// Insert a record to update
	insertErr := ccm.insert(ctx, dbConnectionPool, ClientConfigUpdate{
		WalletID:           &walletID,
		EncryptedAPIKey:    &encryptedAPIKey,
		EncrypterPublicKey: &encrypterPublicKey,
	})
	require.NoError(t, insertErr)

	t.Run("return error if no fields are provided", func(t *testing.T) {
		config := ClientConfigUpdate{}
		err := ccm.update(ctx, dbConnectionPool, config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid circle config for update: wallet_id or encrypted_api_key must be provided")
	})

	t.Run("update wallet_id successfully", func(t *testing.T) {
		tx := testutils.BeginTxWithRollback(t, ctx, dbConnectionPool)

		config := ClientConfigUpdate{WalletID: &updatedWalletID}

		err := ccm.update(ctx, tx, config)
		require.NoError(t, err)

		cc, err := ccm.get(ctx, tx)
		assert.NoError(t, err)
		assert.Equal(t, updatedWalletID, *cc.WalletID)
		assert.Equal(t, encryptedAPIKey, *cc.EncryptedAPIKey)
		assert.Equal(t, encrypterPublicKey, *cc.EncrypterPublicKey)
	})

	t.Run("updates encrypted_api_key and encrypter_public_key successfully", func(t *testing.T) {
		tx := testutils.BeginTxWithRollback(t, ctx, dbConnectionPool)

		err := ccm.update(ctx, dbConnectionPool, ClientConfigUpdate{
			EncryptedAPIKey:    &updatedEncryptedAPIKey,
			EncrypterPublicKey: &updatedEncrypterPublicKey,
		})
		require.NoError(t, err)

		cc, err := ccm.get(ctx, tx)
		assert.NoError(t, err)
		assert.Equal(t, walletID, *cc.WalletID)
		assert.Equal(t, updatedEncryptedAPIKey, *cc.EncryptedAPIKey)
		assert.Equal(t, updatedEncrypterPublicKey, *cc.EncrypterPublicKey)
	})

	t.Run("updates both wallet_id and encrypted_api_key with encrypter_public_key successfully", func(t *testing.T) {
		tx := testutils.BeginTxWithRollback(t, ctx, dbConnectionPool)

		err := ccm.update(ctx, dbConnectionPool, ClientConfigUpdate{
			WalletID:           &updatedWalletID,
			EncryptedAPIKey:    &updatedEncryptedAPIKey,
			EncrypterPublicKey: &updatedEncrypterPublicKey,
		})
		require.NoError(t, err)

		cc, err := ccm.get(ctx, tx)
		assert.NoError(t, err)
		assert.Equal(t, updatedWalletID, *cc.WalletID)
		assert.Equal(t, updatedEncryptedAPIKey, *cc.EncryptedAPIKey)
		assert.Equal(t, updatedEncrypterPublicKey, *cc.EncrypterPublicKey)
	})
}

func Test_ClientConfigModel_insert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	ccm := &ClientConfigModel{DBConnectionPool: dbConnectionPool}

	walletID := "the_wallet_id"
	encryptedAPIKey := "the_encrypted_api_key"
	encrypterPublicKey := "the_encrypter_public_key"

	t.Run("insert successfully", func(t *testing.T) {
		tx := testutils.BeginTxWithRollback(t, ctx, dbConnectionPool)

		config := ClientConfigUpdate{
			WalletID:           &walletID,
			EncryptedAPIKey:    &encryptedAPIKey,
			EncrypterPublicKey: &encrypterPublicKey,
		}

		err := ccm.insert(ctx, tx, config)
		require.NoError(t, err)

		cc, err := ccm.get(ctx, tx)
		assert.NoError(t, err)
		assert.Equal(t, walletID, *cc.WalletID)
		assert.Equal(t, encryptedAPIKey, *cc.EncryptedAPIKey)
		assert.Equal(t, encrypterPublicKey, *cc.EncrypterPublicKey)
	})

	t.Run("insert fails with missing encrypted_api_key", func(t *testing.T) {
		tx := testutils.BeginTxWithRollback(t, ctx, dbConnectionPool)

		config := ClientConfigUpdate{
			WalletID:           &walletID,
			EncryptedAPIKey:    nil,
			EncrypterPublicKey: &encrypterPublicKey,
		}

		err := ccm.insert(ctx, tx, config)
		assert.Error(t, err)
		assert.Contains(t,
			err.Error(),
			"invalid circle config for insert: wallet_id, encrypted_api_key, and encrypter_public_key must be provided")
	})

	t.Run("insert fails with missing wallet_id", func(t *testing.T) {
		tx := testutils.BeginTxWithRollback(t, ctx, dbConnectionPool)

		config := ClientConfigUpdate{
			WalletID:           nil,
			EncryptedAPIKey:    &encryptedAPIKey,
			EncrypterPublicKey: &encrypterPublicKey,
		}

		err := ccm.insert(ctx, tx, config)
		assert.Error(t, err)
		assert.Contains(t,
			err.Error(),
			"invalid circle config for insert: wallet_id, encrypted_api_key, and encrypter_public_key must be provided")
	})

	t.Run("insert fails with missing encrypter_public_key", func(t *testing.T) {
		tx := testutils.BeginTxWithRollback(t, ctx, dbConnectionPool)

		config := ClientConfigUpdate{
			WalletID:           &walletID,
			EncryptedAPIKey:    &encryptedAPIKey,
			EncrypterPublicKey: nil,
		}

		err := ccm.insert(ctx, tx, config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid circle config for insert: wallet_id, encrypted_api_key, and encrypter_public_key must be provided")
	})

	t.Run("insert fails when inserting a second record", func(t *testing.T) {
		tx := testutils.BeginTxWithRollback(t, ctx, dbConnectionPool)

		config := ClientConfigUpdate{
			WalletID:           &walletID,
			EncryptedAPIKey:    &encryptedAPIKey,
			EncrypterPublicKey: &encrypterPublicKey,
		}

		err := ccm.insert(ctx, tx, config)
		require.NoError(t, err)

		err = ccm.insert(ctx, tx, config)
		assert.ErrorContains(t, err, "inserting circle config: pq: circle_client_config must contain exactly one row")
	})
}

func Test_ClientConfigUpdate_Validate(t *testing.T) {
	walletID := "wallet_id"
	encryptedAPIKey := "encrypted_api_key"
	encrypterPublicKey := "encrypter_public_key"

	tests := []struct {
		name    string
		input   ClientConfigUpdate
		wantErr error
	}{
		{
			name:    "both wallet_id and encrypted_api_key are nil",
			input:   ClientConfigUpdate{},
			wantErr: errors.New("wallet_id or encrypted_api_key must be provided"),
		},
		{
			name:    "encrypted_api_key is provided without encrypter_public_key",
			input:   ClientConfigUpdate{EncryptedAPIKey: &encryptedAPIKey},
			wantErr: errors.New("encrypter_public_key must be provided if encrypted_api_key is provided"),
		},
		{
			name:  "wallet_id is provided without encrypted_api_key",
			input: ClientConfigUpdate{WalletID: &walletID},
		},
		{
			name:  "both wallet_id and encrypted_api_key are provided with encrypter_public_key",
			input: ClientConfigUpdate{WalletID: &walletID, EncryptedAPIKey: &encryptedAPIKey, EncrypterPublicKey: &encrypterPublicKey},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.validate()
			if tt.wantErr != nil {
				assert.Equal(t, tt.wantErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
