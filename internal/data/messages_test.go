package data

import (
	"context"
	"testing"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_MessageModel_Insert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	t.Run("inserts a new message successfully", func(t *testing.T) {
		mm := &MessageModel{dbConnectionPool: dbConnectionPool}

		asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")

		statusHistory := MessageStatusHistory{
			{
				Status:    PendingMessageStatus,
				Timestamp: time.Now().UTC(),
			},
		}

		msg, err := mm.Insert(ctx, &MessageInsert{
			Type:           message.MessengerTypeTwilioSMS,
			AssetID:        &asset.ID,
			ReceiverID:     receiver.ID,
			WalletID:       wallet.ID,
			TextEncrypted:  "text encrypted",
			TitleEncrypted: "title encrypted",
			Status:         PendingMessageStatus,
			StatusHistory:  statusHistory,
		})
		require.NoError(t, err)

		assert.NotEmpty(t, msg.ID)
		assert.Equal(t, message.MessengerTypeTwilioSMS, msg.Type)
		assert.Equal(t, PendingMessageStatus, msg.Status)
		assert.Equal(t, statusHistory, msg.StatusHistory)
		assert.Equal(t, asset.ID, *msg.AssetID)
		assert.Equal(t, receiver.ID, msg.ReceiverID)
		assert.Equal(t, wallet.ID, msg.WalletID)
		assert.Equal(t, "text encrypted", msg.TextEncrypted)
		assert.Equal(t, "title encrypted", msg.TitleEncrypted)
		assert.NotEmpty(t, msg.CreatedAt)
		assert.NotEmpty(t, msg.UpdatedAt)
	})
}

func Test_MessageModel_BulkInsert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	mm := &MessageModel{dbConnectionPool: dbConnectionPool}

	t.Run("inserts a new messages successfully", func(t *testing.T) {
		asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, DraftReceiversWalletStatus)

		err := mm.BulkInsert(ctx, dbConnectionPool, []*MessageInsert{
			{
				Type:             message.MessengerTypeTwilioSMS,
				AssetID:          nil,
				ReceiverID:       receiver.ID,
				WalletID:         wallet.ID,
				ReceiverWalletID: &receiverWallet.ID,
				TextEncrypted:    "text encrypted",
				TitleEncrypted:   "title encrypted",
				Status:           SuccessMessageStatus,
			},
			{
				Type:             message.MessengerTypeTwilioSMS,
				AssetID:          &asset.ID,
				ReceiverID:       receiver.ID,
				WalletID:         wallet.ID,
				ReceiverWalletID: nil,
				TextEncrypted:    "text encrypted",
				TitleEncrypted:   "title encrypted",
				Status:           PendingMessageStatus,
			},
			{
				Type:             message.MessengerTypeTwilioSMS,
				AssetID:          &asset.ID,
				ReceiverID:       receiver.ID,
				WalletID:         wallet.ID,
				ReceiverWalletID: &receiverWallet.ID,
				TextEncrypted:    "text encrypted",
				TitleEncrypted:   "title encrypted",
				Status:           FailureMessageStatus,
			},
		})
		require.NoError(t, err)

		const q = `
			SELECT
				id, type, asset_id, receiver_id, wallet_id, receiver_wallet_id,
				text_encrypted, title_encrypted, status, status_history,
				created_at, updated_at
			FROM
				messages
			ORDER BY
				status::text
		`

		var messages []Message
		err = dbConnectionPool.SelectContext(ctx, &messages, q)
		require.NoError(t, err)

		assert.Len(t, messages, 3)

		// Failure
		assert.Equal(t, message.MessengerTypeTwilioSMS, messages[0].Type)
		assert.Equal(t, asset.ID, *messages[0].AssetID)
		assert.Equal(t, receiver.ID, messages[0].ReceiverID)
		assert.Equal(t, wallet.ID, messages[0].WalletID)
		assert.Equal(t, message.MessengerTypeTwilioSMS, messages[0].Type)
		assert.Equal(t, receiverWallet.ID, *messages[0].ReceiverWalletID)
		assert.Equal(t, "text encrypted", messages[0].TextEncrypted)
		assert.Equal(t, "title encrypted", messages[0].TitleEncrypted)
		assert.Equal(t, FailureMessageStatus, messages[0].Status)
		assert.Len(t, messages[0].StatusHistory, 2)
		assert.Equal(t, PendingMessageStatus, messages[0].StatusHistory[0].Status)
		assert.Equal(t, FailureMessageStatus, messages[0].StatusHistory[1].Status)

		// Pending
		assert.Equal(t, message.MessengerTypeTwilioSMS, messages[1].Type)
		assert.Equal(t, asset.ID, *messages[1].AssetID)
		assert.Equal(t, receiver.ID, messages[1].ReceiverID)
		assert.Equal(t, wallet.ID, messages[1].WalletID)
		assert.Equal(t, message.MessengerTypeTwilioSMS, messages[1].Type)
		assert.Nil(t, messages[1].ReceiverWalletID)
		assert.Equal(t, "text encrypted", messages[1].TextEncrypted)
		assert.Equal(t, "title encrypted", messages[1].TitleEncrypted)
		assert.Equal(t, PendingMessageStatus, messages[1].Status)
		assert.Len(t, messages[1].StatusHistory, 1)
		assert.Equal(t, PendingMessageStatus, messages[1].StatusHistory[0].Status)

		// Success
		assert.Equal(t, message.MessengerTypeTwilioSMS, messages[2].Type)
		assert.Nil(t, messages[2].AssetID)
		assert.Equal(t, receiver.ID, messages[2].ReceiverID)
		assert.Equal(t, wallet.ID, messages[2].WalletID)
		assert.Equal(t, message.MessengerTypeTwilioSMS, messages[2].Type)
		assert.Equal(t, receiverWallet.ID, *messages[2].ReceiverWalletID)
		assert.Equal(t, "text encrypted", messages[2].TextEncrypted)
		assert.Equal(t, "title encrypted", messages[2].TitleEncrypted)
		assert.Equal(t, SuccessMessageStatus, messages[2].Status)
		assert.Len(t, messages[2].StatusHistory, 2)
		assert.Equal(t, PendingMessageStatus, messages[2].StatusHistory[0].Status)
		assert.Equal(t, SuccessMessageStatus, messages[2].StatusHistory[1].Status)
	})
}
