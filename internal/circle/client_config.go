package circle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type ClientConfig struct {
	EncryptedAPIKey    *string   `db:"encrypted_api_key"`
	WalletID           *string   `db:"wallet_id"`
	EncrypterPublicKey *string   `db:"encrypter_public_key"`
	UpdatedAt          time.Time `db:"updated_at"`
	CreatedAt          time.Time `db:"created_at"`
}

//go:generate mockery --name=ClientConfigModelInterface --case=underscore --structname=MockClientConfigModel
type ClientConfigModelInterface interface {
	Upsert(ctx context.Context, configUpdate ClientConfigUpdate) error
	GetDecryptedAPIKey(ctx context.Context, passphrase string) (string, error)
	Get(ctx context.Context) (*ClientConfig, error)
}

type ClientConfigModel struct {
	DBConnectionPool db.DBConnectionPool
	Encrypter        utils.PrivateKeyEncrypter
}

func NewClientConfigModel(dbConnectionPool db.DBConnectionPool) *ClientConfigModel {
	return &ClientConfigModel{
		DBConnectionPool: dbConnectionPool,
		Encrypter:        &utils.DefaultPrivateKeyEncrypter{},
	}
}

// Upsert insert or update the client configuration for Circle into the database.
func (m *ClientConfigModel) Upsert(ctx context.Context, configUpdate ClientConfigUpdate) error {
	err := db.RunInTransaction(ctx, m.DBConnectionPool, nil, func(tx db.DBTransaction) error {
		existingConfig, err := m.get(ctx, tx)
		if err != nil {
			return fmt.Errorf("getting existing circle config: %w", err)
		}

		if existingConfig == nil {
			err = m.insert(ctx, tx, configUpdate)
			if err != nil {
				return fmt.Errorf("inserting new circle config: %w", err)
			}
		} else {
			err = m.update(ctx, tx, configUpdate)
			if err != nil {
				return fmt.Errorf("updating existing circle config: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("running transaction: %w", err)
	}

	return nil
}

// GetDecryptedAPIKey retrieves the decrypted API key from the database.
func (m *ClientConfigModel) GetDecryptedAPIKey(ctx context.Context, passphrase string) (string, error) {
	config, err := m.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("getting circle config: %w", err)
	}

	apiKey, err := m.Encrypter.Decrypt(*config.EncryptedAPIKey, passphrase)
	if err != nil {
		return "", fmt.Errorf("decrypting circle API key: %w", err)
	}

	return apiKey, nil
}

// Get retrieves the circle client config from the database if it exists.
func (m *ClientConfigModel) Get(ctx context.Context) (*ClientConfig, error) {
	return m.get(ctx, m.DBConnectionPool)
}

// get retrieves the circle client config from the database if it exists.
func (m *ClientConfigModel) get(ctx context.Context, sqlExec db.SQLExecuter) (*ClientConfig, error) {
	const q = `SELECT * FROM circle_client_config`
	var config ClientConfig
	err := sqlExec.GetContext(ctx, &config, q)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting circle config: %w", err)
	}
	return &config, nil
}

// insert inserts the circle client config into the database.
func (m *ClientConfigModel) insert(ctx context.Context, sqlExec db.SQLExecuter, config ClientConfigUpdate) error {
	if err := config.validateForInsert(); err != nil {
		return fmt.Errorf("invalid circle config for insert: %w", err)
	}
	const q = `
					INSERT INTO circle_client_config (encrypted_api_key, wallet_id, encrypter_public_key)
					VALUES ($1, $2, $3)
				`
	_, err := sqlExec.ExecContext(ctx, q, config.EncryptedAPIKey, config.WalletID, config.EncrypterPublicKey)
	if err != nil {
		return fmt.Errorf("inserting circle config: %w", err)
	}
	return nil
}

// update updates the circle client config in the database.
func (m *ClientConfigModel) update(ctx context.Context, sqlExec db.SQLExecuter, config ClientConfigUpdate) error {
	if err := config.validate(); err != nil {
		return fmt.Errorf("invalid circle config for update: %w", err)
	}

	query := `
		UPDATE
			circle_client_config
		SET
			%s
	`

	args := []interface{}{}
	fields := []string{}
	if config.WalletID != nil {
		fields = append(fields, "wallet_id = ?")
		args = append(args, config.WalletID)
	}

	if config.EncryptedAPIKey != nil {
		fields = append(fields, "encrypted_api_key = ?", "encrypter_public_key = ?")
		args = append(args, config.EncryptedAPIKey, config.EncrypterPublicKey)
	}

	query = m.DBConnectionPool.Rebind(fmt.Sprintf(query, strings.Join(fields, ", ")))

	_, err := sqlExec.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("error updating client config: %w", err)
	}

	return nil
}

var _ ClientConfigModelInterface = &ClientConfigModel{}

type ClientConfigUpdate struct {
	EncryptedAPIKey    *string `db:"encrypted_api_key"`
	WalletID           *string `db:"wallet_id"`
	EncrypterPublicKey *string `db:"encrypter_public_key"`
}

func (c ClientConfigUpdate) validate() error {
	if c.WalletID == nil && c.EncryptedAPIKey == nil {
		return fmt.Errorf("wallet_id or encrypted_api_key must be provided")
	}

	if c.EncryptedAPIKey != nil && c.EncrypterPublicKey == nil {
		return fmt.Errorf("encrypter_public_key must be provided if encrypted_api_key is provided")
	}

	return nil
}

func (c ClientConfigUpdate) validateForInsert() error {
	if c.WalletID == nil || c.EncryptedAPIKey == nil || c.EncrypterPublicKey == nil {
		return fmt.Errorf("wallet_id, encrypted_api_key, and encrypter_public_key must be provided")
	}
	return nil
}
