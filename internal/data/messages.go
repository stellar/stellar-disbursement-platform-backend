package data

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
)

type MessageStatus string

var (
	PendingMessageStatus MessageStatus = "PENDING"
	SuccessMessageStatus MessageStatus = "SUCCESS"
	FailureMessageStatus MessageStatus = "FAILURE"
)

type MessageModel struct {
	dbConnectionPool db.DBConnectionPool
}

type Message struct {
	ID               string                `db:"id"`
	Type             message.MessengerType `db:"type"`
	AssetID          *string               `db:"asset_id"`
	ReceiverID       string                `db:"receiver_id"`
	WalletID         string                `db:"wallet_id"`
	ReceiverWalletID *string               `db:"receiver_wallet_id"`
	TextEncrypted    string                `db:"text_encrypted"`
	TitleEncrypted   string                `db:"title_encrypted"`
	Status           MessageStatus         `db:"status"`
	StatusHistory    MessageStatusHistory  `db:"status_history"`
	CreatedAt        time.Time             `db:"created_at"`
	UpdatedAt        time.Time             `db:"updated_at"`
}

type MessageInsert struct {
	Type             message.MessengerType
	AssetID          *string
	ReceiverID       string
	WalletID         string
	ReceiverWalletID *string
	TextEncrypted    string
	TitleEncrypted   string
	Status           MessageStatus
	StatusHistory    MessageStatusHistory
}

type MessageStatusHistoryEntry struct {
	StatusMessage *string       `json:"status_message"`
	Status        MessageStatus `json:"status"`
	Timestamp     time.Time     `json:"timestamp"`
}

type MessageStatusHistory []MessageStatusHistoryEntry

// Value implements the driver.Valuer interface.
func (msh MessageStatusHistory) Value() (driver.Value, error) {
	var statusHistoryJSON []string
	for _, sh := range msh {
		shJSONBytes, err := json.Marshal(sh)
		if err != nil {
			return nil, fmt.Errorf("error converting status history to json for message: %w", err)
		}
		statusHistoryJSON = append(statusHistoryJSON, string(shJSONBytes))
	}

	return pq.Array(statusHistoryJSON).Value()
}

// Scan implements the sql.Scanner interface.
func (msh *MessageStatusHistory) Scan(src interface{}) error {
	var statusHistoryJSON []string
	if err := pq.Array(&statusHistoryJSON).Scan(src); err != nil {
		return fmt.Errorf("error scanning status history value: %w", err)
	}

	for _, sh := range statusHistoryJSON {
		var shEntry MessageStatusHistoryEntry
		err := json.Unmarshal([]byte(sh), &shEntry)
		if err != nil {
			return fmt.Errorf("error unmarshaling status_history column: %w", err)
		}
		*msh = append(*msh, shEntry)
	}

	return nil
}

func (m *MessageModel) Insert(ctx context.Context, newMsg *MessageInsert) (*Message, error) {
	const query = `
		INSERT INTO messages
			(
				type, asset_id, receiver_id, wallet_id, receiver_wallet_id,
				text_encrypted, title_encrypted, status, status_history
			)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING
			*
	`
	var msg Message
	err := m.dbConnectionPool.GetContext(ctx, &msg, query, newMsg.Type, newMsg.AssetID, newMsg.ReceiverID, newMsg.WalletID, newMsg.ReceiverWalletID, newMsg.TextEncrypted, newMsg.TitleEncrypted, newMsg.Status, newMsg.StatusHistory)
	if err != nil {
		return nil, fmt.Errorf("error inserting message: %w", err)
	}

	return &msg, nil
}

func (m *MessageModel) BulkInsert(ctx context.Context, sqlExec db.SQLExecuter, newMsgs []*MessageInsert) error {
	var (
		types, receiverIDs, walletIDs             pq.StringArray
		encryptedTexts, encryptedTitles, statuses pq.StringArray
		assetIDs, receiverWalletIDs               []sql.NullString
	)

	for _, newMsg := range newMsgs {
		types = append(types, string(newMsg.Type))

		assetID := ""
		if newMsg.AssetID != nil {
			assetID = *newMsg.AssetID
		}
		assetIDs = append(assetIDs, sql.NullString{
			String: assetID,
			Valid:  (newMsg.AssetID != nil && *newMsg.AssetID != ""),
		})

		receiverIDs = append(receiverIDs, newMsg.ReceiverID)
		walletIDs = append(walletIDs, newMsg.WalletID)

		receiverWalletID := ""
		if newMsg.ReceiverWalletID != nil {
			receiverWalletID = *newMsg.ReceiverWalletID
		}
		receiverWalletIDs = append(receiverWalletIDs, sql.NullString{
			String: receiverWalletID,
			Valid:  (newMsg.ReceiverWalletID != nil && *newMsg.ReceiverWalletID != ""),
		})

		encryptedTexts = append(encryptedTexts, newMsg.TextEncrypted)
		encryptedTitles = append(encryptedTitles, newMsg.TitleEncrypted)
		statuses = append(statuses, string(newMsg.Status))
	}

	const insertQuery = `
		INSERT INTO messages
			(
				type, asset_id, receiver_id, wallet_id, receiver_wallet_id,
				text_encrypted, title_encrypted, status
			)
		SELECT
			UNNEST($1::message_type[]) AS type, UNNEST($2::text[]) AS asset_id, UNNEST($3::text[]) AS receiver_id, UNNEST($4::text[]) AS wallet_id,
			UNNEST($5::text[]) AS receiver_wallet_id, UNNEST($6::text[]) AS text_encrypted, UNNEST($7::text[]) AS title_encrypted,
			UNNEST($8::message_status[]) AS status
		RETURNING
			id
	`

	var newMsgIDs []string
	err := sqlExec.SelectContext(ctx, &newMsgIDs, insertQuery, types, pq.Array(assetIDs), receiverIDs, walletIDs, pq.Array(receiverWalletIDs), encryptedTexts, encryptedTitles, statuses)
	if err != nil {
		return fmt.Errorf("error inserting messages in BulkInsert: %w", err)
	}

	const updateQuery = `
		UPDATE
			messages
		SET
			status_history = array_append(status_history, create_message_status_history(updated_at, status, NULL)),
			updated_at = NOW()
		WHERE
			id = ANY($1::text[])
			AND status != 'PENDING'
	`
	_, err = sqlExec.ExecContext(ctx, updateQuery, pq.Array(newMsgIDs))
	if err != nil {
		return fmt.Errorf("error update messages status history: %w", err)
	}

	return nil
}
