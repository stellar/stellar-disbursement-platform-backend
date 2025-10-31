package data

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_ReceiversWalletColumnNames(t *testing.T) {
	testCases := []struct {
		tableReference string
		resultAlias    string
		expected       string
	}{
		{
			tableReference: "",
			resultAlias:    "",
			expected: strings.Join([]string{
				"id",
				`receiver_id AS "receiver.id"`,
				`wallet_id AS "wallet.id"`,
				"otp_attempts",
				"otp_created_at",
				"otp_confirmed_at",
				"status",
				"status_history",
				"created_at",
				"updated_at",
				"invitation_sent_at",
				`COALESCE(anchor_platform_transaction_id, '') AS "anchor_platform_transaction_id"`,
				`COALESCE(stellar_address, '') AS "stellar_address"`,
				`COALESCE(stellar_memo, '') AS "stellar_memo"`,
				`COALESCE(stellar_memo_type::text, '') AS "stellar_memo_type"`,
				`COALESCE(otp, '') AS "otp"`,
				`COALESCE(otp_confirmed_with, '') AS "otp_confirmed_with"`,
			}, ",\n"),
		},
		{
			tableReference: "rw",
			resultAlias:    "",
			expected: strings.Join([]string{
				"rw.id",
				`rw.receiver_id AS "receiver.id"`,
				`rw.wallet_id AS "wallet.id"`,
				"rw.otp_attempts",
				"rw.otp_created_at",
				"rw.otp_confirmed_at",
				"rw.status",
				"rw.status_history",
				"rw.created_at",
				"rw.updated_at",
				"rw.invitation_sent_at",
				`COALESCE(rw.anchor_platform_transaction_id, '') AS "anchor_platform_transaction_id"`,
				`COALESCE(rw.stellar_address, '') AS "stellar_address"`,
				`COALESCE(rw.stellar_memo, '') AS "stellar_memo"`,
				`COALESCE(rw.stellar_memo_type::text, '') AS "stellar_memo_type"`,
				`COALESCE(rw.otp, '') AS "otp"`,
				`COALESCE(rw.otp_confirmed_with, '') AS "otp_confirmed_with"`,
			}, ",\n"),
		},
		{
			tableReference: "rw",
			resultAlias:    "receiver_wallets",
			expected: strings.Join([]string{
				`rw.id AS "receiver_wallets.id"`,
				`rw.receiver_id AS "receiver_wallets.receiver.id"`,
				`rw.wallet_id AS "receiver_wallets.wallet.id"`,
				`rw.otp_attempts AS "receiver_wallets.otp_attempts"`,
				`rw.otp_created_at AS "receiver_wallets.otp_created_at"`,
				`rw.otp_confirmed_at AS "receiver_wallets.otp_confirmed_at"`,
				`rw.status AS "receiver_wallets.status"`,
				`rw.status_history AS "receiver_wallets.status_history"`,
				`rw.created_at AS "receiver_wallets.created_at"`,
				`rw.updated_at AS "receiver_wallets.updated_at"`,
				`rw.invitation_sent_at AS "receiver_wallets.invitation_sent_at"`,
				`COALESCE(rw.anchor_platform_transaction_id, '') AS "receiver_wallets.anchor_platform_transaction_id"`,
				`COALESCE(rw.stellar_address, '') AS "receiver_wallets.stellar_address"`,
				`COALESCE(rw.stellar_memo, '') AS "receiver_wallets.stellar_memo"`,
				`COALESCE(rw.stellar_memo_type::text, '') AS "receiver_wallets.stellar_memo_type"`,
				`COALESCE(rw.otp, '') AS "receiver_wallets.otp"`,
				`COALESCE(rw.otp_confirmed_with, '') AS "receiver_wallets.otp_confirmed_with"`,
			}, ",\n"),
		},
	}

	for _, tc := range testCases {
		t.Run(testCaseNameForScanText(t, tc.tableReference, tc.resultAlias), func(t *testing.T) {
			actual := ReceiverWalletColumnNames(tc.tableReference, tc.resultAlias)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func Test_ReceiversWalletModelGetWithReceiverId(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})

	receiverWalletModel := ReceiverWalletModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns empty array when receiver does not exist", func(t *testing.T) {
		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		// Defer a rollback in case anything fails.
		defer func() {
			err = dbTx.Rollback()
			require.Error(t, err, "not in transaction")
		}()

		actual, errReceiver := receiverWalletModel.GetWithReceiverIDs(ctx, dbTx, ReceiverIDs{"invalid_id"})
		require.NoError(t, errReceiver)
		require.Empty(t, actual)

		err = dbTx.Commit()
		require.NoError(t, err)
	})

	t.Run("returns empty array when receiver does not have a receiver_wallet", func(t *testing.T) {
		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		// Defer a rollback in case anything fails.
		defer func() {
			err = dbTx.Rollback()
			require.Error(t, err, "not in transaction")
		}()

		actual, errReceiver := receiverWalletModel.GetWithReceiverIDs(ctx, dbTx, ReceiverIDs{receiver.ID})
		require.NoError(t, errReceiver)
		require.Empty(t, actual)

		err = dbTx.Commit()
		require.NoError(t, err)
	})

	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	wallet1 := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	receiverWallet1 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet1.ID, DraftReceiversWalletStatus)

	message1 := CreateMessageFixture(t, ctx, dbConnectionPool, &Message{
		Type:             message.MessengerTypeTwilioSMS,
		AssetID:          nil,
		ReceiverID:       receiver.ID,
		WalletID:         wallet1.ID,
		ReceiverWalletID: &receiverWallet1.ID,
		Status:           SuccessMessageStatus,
		CreatedAt:        time.Date(2023, 1, 10, 23, 40, 20, 1000, time.UTC),
	})

	message2 := CreateMessageFixture(t, ctx, dbConnectionPool, &Message{
		Type:             message.MessengerTypeTwilioSMS,
		AssetID:          nil,
		ReceiverID:       receiver.ID,
		WalletID:         wallet1.ID,
		ReceiverWalletID: &receiverWallet1.ID,
		Status:           SuccessMessageStatus,
		CreatedAt:        time.Date(2023, 2, 10, 23, 40, 20, 1000, time.UTC),
	})

	disbursementModel := DisbursementModel{dbConnectionPool: dbConnectionPool}
	disbursement := Disbursement{
		Status: DraftDisbursementStatus,
		Asset:  asset,
	}

	stellarTransactionID, err := utils.RandomString(64)
	require.NoError(t, err)
	stellarOperationID, err := utils.RandomString(32)
	require.NoError(t, err)

	paymentModel := PaymentModel{dbConnectionPool: dbConnectionPool}
	payment := Payment{
		Amount:               "50",
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Asset:                *asset,
	}

	t.Run("returns receiver_wallet without payments", func(t *testing.T) {
		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		// Defer a rollback in case anything fails.
		defer func() {
			err = dbTx.Rollback()
			require.Error(t, err, "not in transaction")
		}()

		actual, err := receiverWalletModel.GetWithReceiverIDs(ctx, dbTx, ReceiverIDs{receiver.ID})
		require.NoError(t, err)
		expected := []ReceiverWallet{
			{
				ID:       receiverWallet1.ID,
				Receiver: Receiver{ID: receiver.ID},
				Wallet: Wallet{
					ID:                wallet1.ID,
					Name:              wallet1.Name,
					Homepage:          wallet1.Homepage,
					SEP10ClientDomain: wallet1.SEP10ClientDomain,
					Enabled:           true,
				},
				StellarAddress:    receiverWallet1.StellarAddress,
				StellarMemo:       receiverWallet1.StellarMemo,
				StellarMemoType:   receiverWallet1.StellarMemoType,
				Status:            receiverWallet1.Status,
				CreatedAt:         receiverWallet1.CreatedAt,
				UpdatedAt:         receiverWallet1.CreatedAt,
				InvitedAt:         &message1.CreatedAt,
				LastMessageSentAt: &message2.CreatedAt,
				ReceiverWalletStats: ReceiverWalletStats{
					TotalPayments:     "0",
					PaymentsReceived:  "0",
					FailedPayments:    "0",
					CanceledPayments:  "0",
					RemainingPayments: "0",
					ReceivedAmounts:   nil,
				},
				SEP24TransactionID: receiverWallet1.SEP24TransactionID,
			},
		}
		assert.Equal(t, expected, actual)

		err = dbTx.Commit()
		require.NoError(t, err)
	})

	t.Run("returns receiver_wallet with payments", func(t *testing.T) {
		disbursement.Name = "disbursement 1"
		disbursement.Wallet = wallet1
		d := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		payment.Status = SuccessPaymentStatus
		payment.Disbursement = d
		payment.ReceiverWallet = receiverWallet1
		CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &payment)

		disbursement.Name = "disbursement 2"
		disbursement.Wallet = wallet1
		d = CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		payment.Status = DraftPaymentStatus
		payment.Disbursement = d
		payment.ReceiverWallet = receiverWallet1
		CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &payment)

		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		// Defer a rollback in case anything fails.
		defer func() {
			err = dbTx.Rollback()
			require.Error(t, err, "not in transaction")
		}()

		actual, err := receiverWalletModel.GetWithReceiverIDs(ctx, dbTx, ReceiverIDs{receiver.ID})
		require.NoError(t, err)
		expected := []ReceiverWallet{
			{
				ID:       receiverWallet1.ID,
				Receiver: Receiver{ID: receiver.ID},
				Wallet: Wallet{
					ID:                wallet1.ID,
					Name:              wallet1.Name,
					Homepage:          wallet1.Homepage,
					SEP10ClientDomain: wallet1.SEP10ClientDomain,
					Enabled:           true,
				},
				StellarAddress:    receiverWallet1.StellarAddress,
				StellarMemo:       receiverWallet1.StellarMemo,
				StellarMemoType:   receiverWallet1.StellarMemoType,
				Status:            receiverWallet1.Status,
				CreatedAt:         receiverWallet1.CreatedAt,
				UpdatedAt:         receiverWallet1.CreatedAt,
				InvitedAt:         &message1.CreatedAt,
				LastMessageSentAt: &message2.CreatedAt,
				ReceiverWalletStats: ReceiverWalletStats{
					TotalPayments:     "2",
					PaymentsReceived:  "1",
					FailedPayments:    "0",
					CanceledPayments:  "0",
					RemainingPayments: "1",
					ReceivedAmounts: []Amount{
						{
							AssetCode:      "USDC",
							AssetIssuer:    "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
							ReceivedAmount: "50.0000000",
						},
					},
				},
				SEP24TransactionID: receiverWallet1.SEP24TransactionID,
			},
		}
		assert.Equal(t, expected, actual)

		err = dbTx.Commit()
		require.NoError(t, err)
	})

	t.Run("returns multiple receiver_wallets", func(t *testing.T) {
		DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)

		disbursement.Name = "disbursement 1"
		disbursement.Wallet = wallet1
		d := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		payment.Status = SuccessPaymentStatus
		payment.Disbursement = d
		payment.ReceiverWallet = receiverWallet1
		CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &payment)

		disbursement.Name = "disbursement 2"
		disbursement.Wallet = wallet1
		d = CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		payment.Status = DraftPaymentStatus
		payment.Disbursement = d
		payment.ReceiverWallet = receiverWallet1
		CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &payment)

		wallet2 := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet2", "https://www.wallet2.com", "www.wallet2.com", "wallet2://")
		receiverWallet2 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet2.ID, DraftReceiversWalletStatus)

		message3 := CreateMessageFixture(t, ctx, dbConnectionPool, &Message{
			Type:             message.MessengerTypeTwilioSMS,
			AssetID:          nil,
			ReceiverID:       receiver.ID,
			WalletID:         wallet2.ID,
			ReceiverWalletID: &receiverWallet2.ID,
			Status:           SuccessMessageStatus,
			CreatedAt:        time.Date(2023, 1, 10, 23, 40, 20, 1000, time.UTC),
		})

		message4 := CreateMessageFixture(t, ctx, dbConnectionPool, &Message{
			Type:             message.MessengerTypeTwilioSMS,
			AssetID:          nil,
			ReceiverID:       receiver.ID,
			WalletID:         wallet2.ID,
			ReceiverWalletID: &receiverWallet2.ID,
			Status:           SuccessMessageStatus,
			CreatedAt:        time.Date(2023, 2, 10, 23, 40, 20, 1000, time.UTC),
		})

		disbursement.Name = "disbursement 3"
		disbursement.Wallet = wallet2
		d = CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		payment.Status = DraftPaymentStatus
		payment.Disbursement = d
		payment.ReceiverWallet = receiverWallet2
		CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &payment)

		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		// Defer a rollback in case anything fails.
		defer func() {
			err = dbTx.Rollback()
			require.Error(t, err, "not in transaction")
		}()

		actual, err := receiverWalletModel.GetWithReceiverIDs(ctx, dbTx, ReceiverIDs{receiver.ID})
		require.NoError(t, err)
		expected := []ReceiverWallet{
			{
				ID:       receiverWallet1.ID,
				Receiver: Receiver{ID: receiver.ID},
				Wallet: Wallet{
					ID:                wallet1.ID,
					Name:              wallet1.Name,
					Homepage:          wallet1.Homepage,
					SEP10ClientDomain: wallet1.SEP10ClientDomain,
					Enabled:           true,
				},
				StellarAddress:    receiverWallet1.StellarAddress,
				StellarMemo:       receiverWallet1.StellarMemo,
				StellarMemoType:   receiverWallet1.StellarMemoType,
				Status:            receiverWallet1.Status,
				CreatedAt:         receiverWallet1.CreatedAt,
				UpdatedAt:         receiverWallet1.CreatedAt,
				InvitedAt:         &message1.CreatedAt,
				LastMessageSentAt: &message2.CreatedAt,
				ReceiverWalletStats: ReceiverWalletStats{
					TotalPayments:     "2",
					PaymentsReceived:  "1",
					FailedPayments:    "0",
					CanceledPayments:  "0",
					RemainingPayments: "1",
					ReceivedAmounts: []Amount{
						{
							AssetCode:      "USDC",
							AssetIssuer:    "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
							ReceivedAmount: "50.0000000",
						},
					},
				},
				SEP24TransactionID: receiverWallet1.SEP24TransactionID,
			},
			{
				ID:       receiverWallet2.ID,
				Receiver: Receiver{ID: receiver.ID},
				Wallet: Wallet{
					ID:                wallet2.ID,
					Name:              wallet2.Name,
					Homepage:          wallet2.Homepage,
					SEP10ClientDomain: wallet2.SEP10ClientDomain,
					Enabled:           true,
				},
				StellarAddress:    receiverWallet2.StellarAddress,
				StellarMemo:       receiverWallet2.StellarMemo,
				StellarMemoType:   receiverWallet2.StellarMemoType,
				Status:            receiverWallet2.Status,
				CreatedAt:         receiverWallet2.CreatedAt,
				UpdatedAt:         receiverWallet2.CreatedAt,
				InvitedAt:         &message3.CreatedAt,
				LastMessageSentAt: &message4.CreatedAt,
				ReceiverWalletStats: ReceiverWalletStats{
					TotalPayments:     "1",
					PaymentsReceived:  "0",
					FailedPayments:    "0",
					CanceledPayments:  "0",
					RemainingPayments: "1",
					ReceivedAmounts: []Amount{
						{
							AssetCode:      "USDC",
							AssetIssuer:    "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
							ReceivedAmount: "0",
						},
					},
				},
				SEP24TransactionID: receiverWallet2.SEP24TransactionID,
			},
		}
		assert.Equal(t, expected, actual)

		err = dbTx.Commit()
		require.NoError(t, err)
	})

	DeleteAllMessagesFixtures(t, ctx, dbConnectionPool)
	DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
	DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)
	DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

	t.Run("returns receiver_wallet with session", func(t *testing.T) {
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet1.ID, DraftReceiversWalletStatus)

		message1 = CreateMessageFixture(t, ctx, dbConnectionPool, &Message{
			Type:             message.MessengerTypeTwilioSMS,
			AssetID:          nil,
			ReceiverID:       receiver.ID,
			WalletID:         wallet1.ID,
			ReceiverWalletID: &receiverWallet.ID,
			Status:           SuccessMessageStatus,
			CreatedAt:        time.Date(2023, 1, 10, 23, 40, 20, 1000, time.UTC),
		})

		message2 = CreateMessageFixture(t, ctx, dbConnectionPool, &Message{
			Type:             message.MessengerTypeTwilioSMS,
			AssetID:          nil,
			ReceiverID:       receiver.ID,
			WalletID:         wallet1.ID,
			ReceiverWalletID: &receiverWallet.ID,
			Status:           SuccessMessageStatus,
			CreatedAt:        time.Date(2023, 2, 10, 23, 40, 20, 1000, time.UTC),
		})

		// Initializing a new Tx.
		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		// Defer a rollback in case anything fails.
		defer func() {
			err = dbTx.Rollback()
			require.Error(t, err, "not in transaction")
		}()

		actual, err := receiverWalletModel.GetWithReceiverIDs(ctx, dbTx, ReceiverIDs{receiver.ID})
		require.NoError(t, err)
		expected := []ReceiverWallet{
			{
				ID:       receiverWallet.ID,
				Receiver: Receiver{ID: receiver.ID},
				Wallet: Wallet{
					ID:                wallet1.ID,
					Name:              wallet1.Name,
					Homepage:          wallet1.Homepage,
					SEP10ClientDomain: wallet1.SEP10ClientDomain,
					Enabled:           true,
				},
				StellarAddress:    receiverWallet.StellarAddress,
				StellarMemo:       receiverWallet.StellarMemo,
				StellarMemoType:   receiverWallet.StellarMemoType,
				Status:            receiverWallet.Status,
				CreatedAt:         receiverWallet.CreatedAt,
				UpdatedAt:         receiverWallet.CreatedAt,
				InvitedAt:         &message1.CreatedAt,
				LastMessageSentAt: &message2.CreatedAt,
				ReceiverWalletStats: ReceiverWalletStats{
					TotalPayments:     "0",
					PaymentsReceived:  "0",
					FailedPayments:    "0",
					CanceledPayments:  "0",
					RemainingPayments: "0",
					ReceivedAmounts:   nil,
				},
				SEP24TransactionID: receiverWallet.SEP24TransactionID,
			},
		}
		assert.Equal(t, expected, actual)

		// Commit the transaction.
		err = dbTx.Commit()
		require.NoError(t, err)
	})
}

func Test_GetByReceiverIDAndWalletDomain(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	receiverWalletModel := ReceiverWalletModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns error when receiver wallet does not exist", func(t *testing.T) {
		actual, errGetReceiverWallet := receiverWalletModel.GetByReceiverIDAndWalletDomain(ctx, "invalid_id", "invalid_domain", dbConnectionPool)
		require.Error(t, errGetReceiverWallet, "error querying receiver wallet: sql: no rows in result set")
		require.Empty(t, actual)
	})

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	// TODO update CreateReceiverWalletFixture to allow create a wallet with a ReceiverWallet object
	receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, DraftReceiversWalletStatus)
	query := `
			UPDATE 
				receiver_wallets rw
			SET
				otp = $1,
				otp_created_at = NOW()
			WHERE
				rw.id = $2
			RETURNING
				otp_created_at
		`
	err = dbConnectionPool.GetContext(ctx, &receiverWallet.OTPCreatedAt, query, "123456", receiverWallet.ID)
	require.NoError(t, err)

	t.Run("returns error when receiver wallet not found for receiver id", func(t *testing.T) {
		actual, errGetReceiverWallet := receiverWalletModel.GetByReceiverIDAndWalletDomain(ctx, "invalid_id", wallet.SEP10ClientDomain, dbConnectionPool)
		require.Error(t, errGetReceiverWallet, "error querying receiver wallet: sql: no rows in result set")
		require.Empty(t, actual)
	})

	t.Run("returns error when receiver wallet not found with wallet domain", func(t *testing.T) {
		actual, errGetReceiverWallet := receiverWalletModel.GetByReceiverIDAndWalletDomain(ctx, receiver.ID, "invalid_domain", dbConnectionPool)
		require.Error(t, errGetReceiverWallet, "error querying receiver wallet: sql: no rows in result set")
		require.Empty(t, actual)
	})

	t.Run("returns receiver_wallet", func(t *testing.T) {
		actual, err := receiverWalletModel.GetByReceiverIDAndWalletDomain(ctx, receiver.ID, wallet.SEP10ClientDomain, dbConnectionPool)
		require.NoError(t, err)

		expected := ReceiverWallet{
			ID:       receiverWallet.ID,
			Receiver: Receiver{ID: receiver.ID},
			Wallet: Wallet{
				ID:                wallet.ID,
				Name:              wallet.Name,
				Homepage:          wallet.Homepage,
				SEP10ClientDomain: wallet.SEP10ClientDomain,
				DeepLinkSchema:    wallet.DeepLinkSchema,
				Enabled:           true,
			},
			Status:             receiverWallet.Status,
			StellarAddress:     receiverWallet.StellarAddress,
			StellarMemo:        receiverWallet.StellarMemo,
			StellarMemoType:    receiverWallet.StellarMemoType,
			StatusHistory:      receiverWallet.StatusHistory,
			OTPCreatedAt:       receiverWallet.OTPCreatedAt,
			CreatedAt:          receiverWallet.CreatedAt,
			UpdatedAt:          actual.UpdatedAt,
			OTP:                "123456",
			OTPConfirmedAt:     nil,
			SEP24TransactionID: receiverWallet.SEP24TransactionID,
		}

		assert.Equal(t, expected, *actual)
	})
}

func Test_ReceiverWallet_UpdateOTPByReceiverContactInfoAndWalletDomain(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)

	ctx := context.Background()
	receiverWalletModel := ReceiverWalletModel{dbConnectionPool: dbConnectionPool}
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "http://home.test", "home.test", "wallet123://")

	// Define test cases
	testCases := []struct {
		name                string
		setupReceiverWallet func(t *testing.T, receiver Receiver)
		contactInfo         func(r Receiver, contactType ReceiverContactType) string
		clientDomain        string
		expectedRows        int
		expectedAttempts    int
	}{
		{
			name: "does not update OTP for a receiver wallet with a different contact info",
			setupReceiverWallet: func(t *testing.T, receiver Receiver) {
				_ = CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)
			},
			contactInfo: func(r Receiver, contactType ReceiverContactType) string {
				return "invalid_contact_info"
			},
			clientDomain: wallet.SEP10ClientDomain,
			expectedRows: 0,
		},
		{
			name: "does not update OTP for a receiver wallet with a different client domain",
			setupReceiverWallet: func(t *testing.T, receiver Receiver) {
				_ = CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)
			},
			contactInfo: func(r Receiver, contactType ReceiverContactType) string {
				return r.ContactByType(contactType)
			},
			clientDomain: "foo-bar",
			expectedRows: 0,
		},
		{
			name: "does not update OTP for a confirmed receiver wallet",
			setupReceiverWallet: func(t *testing.T, receiver Receiver) {
				rw := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)
				// Confirm OTP
				q := `UPDATE receiver_wallets SET otp_confirmed_at = NOW() WHERE id = $1`
				_, err := dbConnectionPool.ExecContext(ctx, q, rw.ID)
				require.NoError(t, err)
			},
			contactInfo: func(r Receiver, contactType ReceiverContactType) string {
				return r.ContactByType(contactType)
			},
			clientDomain: wallet.SEP10ClientDomain,
			expectedRows: 0,
		},
		{
			name: "🎉 successfully updates OTP for an unconfirmed receiver wallet",
			setupReceiverWallet: func(t *testing.T, receiver Receiver) {
				_ = CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)
			},
			contactInfo: func(r Receiver, contactType ReceiverContactType) string {
				return r.ContactByType(contactType)
			},
			clientDomain:     wallet.SEP10ClientDomain,
			expectedRows:     1,
			expectedAttempts: 0,
		},
		{
			name: "🎉 successfully renews OTP for an unconfirmed receiver wallet",
			setupReceiverWallet: func(t *testing.T, receiver Receiver) {
				// Create a receiver with a different contact info toi make sure they will not be picked by the query
				receiverNoOp := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{PhoneNumber: "+141555550000", Email: "zoopbar@test.com"})
				rwNoOp := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverNoOp.ID, wallet.ID, ReadyReceiversWalletStatus)

				// Confirm OTP for the first receiver
				q := `UPDATE receiver_wallets SET otp_confirmed_at = NOW() WHERE id = $1`
				_, err := dbConnectionPool.ExecContext(ctx, q, rwNoOp.ID)
				require.NoError(t, err)

				_ = CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)
			},
			contactInfo: func(r Receiver, contactType ReceiverContactType) string {
				return r.ContactByType(contactType)
			},
			clientDomain:     wallet.SEP10ClientDomain,
			expectedRows:     1,
			expectedAttempts: 0,
		},
	}

	// Prepare test data
	phoneNumber := "+141555555555"
	email := "test@example.com"

	// Run test cases
	for _, contactType := range GetAllReceiverContactTypes() {
		receiverInsert := &Receiver{}
		switch contactType {
		case ReceiverContactTypeSMS:
			receiverInsert.PhoneNumber = phoneNumber
		case ReceiverContactTypeEmail:
			receiverInsert.Email = email
		}

		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%s", contactType, tc.name), func(t *testing.T) {
				defer DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
				defer DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

				receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, receiverInsert)
				tc.setupReceiverWallet(t, *receiver)

				otp, err := utils.RandomString(6, utils.NumberBytes)
				require.NoError(t, err)

				contactInfo := tc.contactInfo(*receiver, contactType)
				rowsUpdated, err := receiverWalletModel.UpdateOTPByReceiverContactInfoAndWalletDomain(ctx, contactInfo, tc.clientDomain, otp)
				require.NoError(t, err)
				assert.Equal(t, tc.expectedRows, rowsUpdated)

				if tc.expectedRows > 0 {
					var actualOTP string
					var actualAttempts int
					q := `SELECT otp, otp_attempts FROM receiver_wallets WHERE receiver_id = $1 AND wallet_id = $2`
					err := dbConnectionPool.QueryRowxContext(ctx, q, receiver.ID, wallet.ID).Scan(&actualOTP, &actualAttempts)
					require.NoError(t, err)
					assert.Equal(t, otp, actualOTP)
					assert.Equal(t, tc.expectedAttempts, actualAttempts)
				}
			})
		}
	}
}

func Test_ReceiverWallet_GetAllPendingRegistration(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, setupErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, setupErr)
	defer dbConnectionPool.Close()

	models, setupErr := NewModels(dbConnectionPool)
	require.NoError(t, setupErr)

	ctx := context.Background()

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	wallet1 := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet1", "https://wallet1.com", "www.wallet.com", "wallet1://")
	wallet2 := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet2", "https://wallet2.com", "www.wallet2.com", "wallet2://")
	wallet3 := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet3", "https://wallet3.com", "www.wallet3.com", "wallet3://")
	wallet4 := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet4", "https://wallet4.com", "www.wallet4.com", "wallet4://")
	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet: wallet1,
		Asset:  asset,
		Status: StartedDisbursementStatus,
	})
	disbursement2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet: wallet2,
		Asset:  asset,
		Status: StartedDisbursementStatus,
	})
	disbursement3 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet: wallet3,
		Asset:  asset,
		Status: StartedDisbursementStatus,
	})
	disbursement4 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet: wallet4,
		Asset:  asset,
		Status: StartedDisbursementStatus,
	})

	t.Run("gets all receiver wallets pending registration", func(t *testing.T) {
		DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		rw1 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet1.ID, DraftReceiversWalletStatus)
		rw2 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet2.ID, RegisteredReceiversWalletStatus)
		rw3 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet3.ID, ReadyReceiversWalletStatus)
		rw4 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet4.ID, ReadyReceiversWalletStatus)

		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "100",
			Status:         ReadyPaymentStatus,
			Disbursement:   disbursement1,
			Asset:          *asset,
			ReceiverWallet: rw1,
		})
		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "100",
			Status:         ReadyPaymentStatus,
			Disbursement:   disbursement2,
			Asset:          *asset,
			ReceiverWallet: rw2,
		})
		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "100",
			Status:         ReadyPaymentStatus,
			Disbursement:   disbursement3,
			Asset:          *asset,
			ReceiverWallet: rw3,
		})
		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "100",
			Status:         ReadyPaymentStatus,
			Disbursement:   disbursement4,
			Asset:          *asset,
			ReceiverWallet: rw4,
		})

		var invitationSentAt time.Time
		const q = `UPDATE receiver_wallets SET invitation_sent_at = NOW() WHERE id = $1 RETURNING invitation_sent_at`
		err := dbConnectionPool.GetContext(ctx, &invitationSentAt, q, rw4.ID)
		require.NoError(t, err)

		// If you pass only rw1 and rw3 IDs as parameters this function will only return these receiver wallets. That's why
		// we need to pass all IDs.
		rws, err := models.ReceiverWallet.GetAllPendingRegistrations(ctx, dbConnectionPool)
		require.NoError(t, err)

		expectedRWs := []*ReceiverWallet{
			{
				ID: rw3.ID,
				Receiver: Receiver{
					ID:          receiver.ID,
					PhoneNumber: receiver.PhoneNumber,
					Email:       receiver.Email,
				},
				Wallet: Wallet{
					ID:   wallet3.ID,
					Name: wallet3.Name,
				},
			},
			{
				ID: rw4.ID,
				Receiver: Receiver{
					ID:          receiver.ID,
					PhoneNumber: receiver.PhoneNumber,
					Email:       receiver.Email,
				},
				Wallet: Wallet{
					ID:   wallet4.ID,
					Name: wallet4.Name,
				},
				InvitationSentAt: &invitationSentAt,
			},
		}

		assert.Len(t, rws, 2)
		assert.ElementsMatch(t, rws, expectedRWs)
	})
}

func Test_ReceiverWallet_GetAllPendingRegistrationByReceiverWalletIDs(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	wallet1 := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet1", "https://wallet1.com", "www.wallet.com", "wallet1://")
	wallet2 := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet2", "https://wallet2.com", "www.wallet2.com", "wallet2://")
	wallet3 := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet3", "https://wallet3.com", "www.wallet3.com", "wallet3://")
	wallet4 := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet4", "https://wallet4.com", "www.wallet4.com", "wallet4://")
	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet: wallet1,
		Asset:  asset,
		Status: StartedDisbursementStatus,
	})
	disbursement2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet: wallet2,
		Asset:  asset,
		Status: StartedDisbursementStatus,
	})
	disbursement3 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet: wallet3,
		Asset:  asset,
		Status: StartedDisbursementStatus,
	})
	disbursement4 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet: wallet4,
		Asset:  asset,
		Status: StartedDisbursementStatus,
	})

	t.Run("gets all receiver wallets pending registration", func(t *testing.T) {
		DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		rw1 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet1.ID, DraftReceiversWalletStatus)
		rw2 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet2.ID, RegisteredReceiversWalletStatus)
		rw3 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet3.ID, ReadyReceiversWalletStatus)
		rw4 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet4.ID, ReadyReceiversWalletStatus)

		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "100",
			Status:         ReadyPaymentStatus,
			Disbursement:   disbursement1,
			Asset:          *asset,
			ReceiverWallet: rw1,
		})
		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "100",
			Status:         ReadyPaymentStatus,
			Disbursement:   disbursement2,
			Asset:          *asset,
			ReceiverWallet: rw2,
		})
		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "100",
			Status:         ReadyPaymentStatus,
			Disbursement:   disbursement3,
			Asset:          *asset,
			ReceiverWallet: rw3,
		})
		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "100",
			Status:         ReadyPaymentStatus,
			Disbursement:   disbursement4,
			Asset:          *asset,
			ReceiverWallet: rw4,
		})

		var invitationSentAt time.Time
		const q = `UPDATE receiver_wallets SET invitation_sent_at = NOW() WHERE id = $1 RETURNING invitation_sent_at`
		err := dbConnectionPool.GetContext(ctx, &invitationSentAt, q, rw4.ID)
		require.NoError(t, err)

		// If you pass only rw1 and rw3 IDs as parameters this function will only return these receiver wallets. That's why
		// we need to pass all IDs.
		rws, err := models.ReceiverWallet.GetAllPendingRegistrationByReceiverWalletIDs(ctx, dbConnectionPool, []string{rw1.ID, rw2.ID, rw3.ID, rw4.ID})
		require.NoError(t, err)

		expectedRWs := []*ReceiverWallet{
			{
				ID: rw3.ID,
				Receiver: Receiver{
					ID:          receiver.ID,
					PhoneNumber: receiver.PhoneNumber,
					Email:       receiver.Email,
				},
				Wallet: Wallet{
					ID:   wallet3.ID,
					Name: wallet3.Name,
				},
			},
			{
				ID: rw4.ID,
				Receiver: Receiver{
					ID:          receiver.ID,
					PhoneNumber: receiver.PhoneNumber,
					Email:       receiver.Email,
				},
				Wallet: Wallet{
					ID:   wallet4.ID,
					Name: wallet4.Name,
				},
				InvitationSentAt: &invitationSentAt,
			},
		}

		assert.Len(t, rws, 2)
		assert.ElementsMatch(t, rws, expectedRWs)
	})

	t.Run("ensures no receivers duplication for the same wallet", func(t *testing.T) {
		DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		rw1 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet1.ID, ReadyReceiversWalletStatus)
		rw2 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet2.ID, ReadyReceiversWalletStatus)

		// Wallet 1 Disbursements
		disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Wallet: wallet1,
			Asset:  asset,
			Status: StartedDisbursementStatus,
		})
		disbursement2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Wallet: wallet1,
			Asset:  asset,
			Status: StartedDisbursementStatus,
		})

		// Wallet 2 Disbursement
		disbursement3 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Wallet: wallet2,
			Asset:  asset,
			Status: StartedDisbursementStatus,
		})

		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "100",
			Status:         ReadyPaymentStatus,
			Disbursement:   disbursement1,
			Asset:          *asset,
			ReceiverWallet: rw1,
		})
		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "100",
			Status:         ReadyPaymentStatus,
			Disbursement:   disbursement2,
			Asset:          *asset,
			ReceiverWallet: rw1,
		})
		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "100",
			Status:         ReadyPaymentStatus,
			Disbursement:   disbursement3,
			Asset:          *asset,
			ReceiverWallet: rw2,
		})

		rws, err := models.ReceiverWallet.GetAllPendingRegistrationByReceiverWalletIDs(ctx, dbConnectionPool, []string{rw1.ID, rw2.ID})
		require.NoError(t, err)

		expectedRWs := []*ReceiverWallet{
			{
				ID: rw1.ID,
				Receiver: Receiver{
					ID:          receiver.ID,
					PhoneNumber: receiver.PhoneNumber,
					Email:       receiver.Email,
				},
				Wallet: Wallet{
					ID:   wallet1.ID,
					Name: wallet1.Name,
				},
			},
			{
				ID: rw2.ID,
				Receiver: Receiver{
					ID:          receiver.ID,
					PhoneNumber: receiver.PhoneNumber,
					Email:       receiver.Email,
				},
				Wallet: Wallet{
					ID:   wallet2.ID,
					Name: wallet2.Name,
				},
			},
		}
		assert.Len(t, rws, 2)
		assert.ElementsMatch(t, rws, expectedRWs)
	})
}

func Test_ReceiverWallet_GetAllPendingRegistrationByDisbursementID(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()

	receiver1 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiver2 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiver3 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiver4 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet1", "https://wallet1.com", "www.wallet.com", "wallet1://")
	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet: wallet,
		Asset:  asset,
		Status: StartedDisbursementStatus,
	})

	t.Run("gets all receiver wallets pending registration by disbursement ID", func(t *testing.T) {
		DeleteAllMessagesFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		rw1 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, RegisteredReceiversWalletStatus)
		rw2 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, RegisteredReceiversWalletStatus)
		rw3 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver3.ID, wallet.ID, ReadyReceiversWalletStatus)
		rw4 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver4.ID, wallet.ID, ReadyReceiversWalletStatus)

		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "100",
			Status:         ReadyPaymentStatus,
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: rw1,
		})
		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "100",
			Status:         ReadyPaymentStatus,
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: rw2,
		})
		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "100",
			Status:         ReadyPaymentStatus,
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: rw3,
		})
		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "100",
			Status:         ReadyPaymentStatus,
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: rw4,
		})

		var invitationSentAt time.Time
		const q = `UPDATE receiver_wallets SET invitation_sent_at = NOW() WHERE id = $1 RETURNING invitation_sent_at`
		err := dbConnectionPool.GetContext(ctx, &invitationSentAt, q, rw4.ID)
		require.NoError(t, err)

		rws, err := models.ReceiverWallet.GetAllPendingRegistrationByDisbursementID(ctx, dbConnectionPool, disbursement.ID)
		require.NoError(t, err)

		expectedRWs := []*ReceiverWallet{
			{
				ID: rw3.ID,
				Receiver: Receiver{
					ID:          receiver3.ID,
					PhoneNumber: receiver3.PhoneNumber,
					Email:       receiver3.Email,
				},
				Wallet: Wallet{
					ID:   wallet.ID,
					Name: wallet.Name,
				},
			},
			{
				ID: rw4.ID,
				Receiver: Receiver{
					ID:          receiver4.ID,
					PhoneNumber: receiver4.PhoneNumber,
					Email:       receiver4.Email,
				},
				Wallet: Wallet{
					ID:   wallet.ID,
					Name: wallet.Name,
				},
				InvitationSentAt: &invitationSentAt,
			},
		}

		assert.Len(t, rws, 2)
		assert.ElementsMatch(t, rws, expectedRWs)
	})
}

func Test_GetByStellarAccountAndMemo(t *testing.T) {
	emptyString := ""

	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	receiverWalletModel := ReceiverWalletModel{dbConnectionPool: dbConnectionPool}
	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	t.Run("returns error when receiver wallet does not exist", func(t *testing.T) {
		actual, innerErr := receiverWalletModel.GetByStellarAccountAndMemo(ctx, "GCRSI42IC7WSW6N46LWPAHQWFI6MLGPBN3BYQ2WMNJ43GNRTIEYCAD6O", wallet.SEP10ClientDomain, &emptyString)
		require.ErrorIs(t, innerErr, ErrRecordNotFound)
		require.Empty(t, actual)
	})

	receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)

	t.Run("wont find the result if stellar address is provided but memo is not", func(t *testing.T) {
		actual, innerErr := receiverWalletModel.GetByStellarAccountAndMemo(ctx, receiverWallet.StellarAddress, wallet.SEP10ClientDomain, &emptyString)
		require.ErrorIs(t, innerErr, ErrRecordNotFound)
		require.Empty(t, actual)
	})

	t.Run("wont find the result if memo is provided but stellar address is not", func(t *testing.T) {
		actual, innerErr := receiverWalletModel.GetByStellarAccountAndMemo(ctx, "", wallet.SEP10ClientDomain, &receiverWallet.StellarMemo)
		require.ErrorIs(t, innerErr, ErrRecordNotFound)
		require.Empty(t, actual)
	})

	t.Run("returns receiver_wallet when both stellar account and memo are provided", func(t *testing.T) {
		actual, innerErr := receiverWalletModel.GetByStellarAccountAndMemo(ctx, receiverWallet.StellarAddress, wallet.SEP10ClientDomain, &receiverWallet.StellarMemo)
		require.NoError(t, innerErr)

		expected := ReceiverWallet{
			ID:       receiverWallet.ID,
			Receiver: Receiver{ID: receiver.ID},
			Wallet: Wallet{
				ID:                wallet.ID,
				Name:              wallet.Name,
				Homepage:          wallet.Homepage,
				SEP10ClientDomain: wallet.SEP10ClientDomain,
				DeepLinkSchema:    wallet.DeepLinkSchema,
				Enabled:           true,
			},
			OTP:                receiverWallet.OTP,
			Status:             receiverWallet.Status,
			StatusHistory:      actual.StatusHistory,
			OTPCreatedAt:       actual.OTPCreatedAt,
			OTPConfirmedAt:     actual.OTPConfirmedAt,
			OTPConfirmedWith:   actual.OTPConfirmedWith,
			CreatedAt:          actual.CreatedAt,
			UpdatedAt:          actual.UpdatedAt,
			StellarAddress:     receiverWallet.StellarAddress,
			StellarMemo:        receiverWallet.StellarMemo,
			StellarMemoType:    receiverWallet.StellarMemoType,
			SEP24TransactionID: receiverWallet.SEP24TransactionID,
			InvitationSentAt:   actual.InvitationSentAt,
		}

		assert.Equal(t, expected, *actual)
	})

	query := `UPDATE receiver_wallets SET stellar_memo = NULL, stellar_memo_type = NULL WHERE id = $1`
	_, err = dbConnectionPool.ExecContext(ctx, query, receiverWallet.ID)
	require.NoError(t, err)

	t.Run("returns receiver_wallet when stellar account is provided and memo is null", func(t *testing.T) {
		actual, err := receiverWalletModel.GetByStellarAccountAndMemo(ctx, receiverWallet.StellarAddress, wallet.SEP10ClientDomain, &emptyString)
		require.NoError(t, err)

		expected := ReceiverWallet{
			ID:       receiverWallet.ID,
			Receiver: Receiver{ID: receiver.ID},
			Wallet: Wallet{
				ID:                wallet.ID,
				Name:              wallet.Name,
				Homepage:          wallet.Homepage,
				SEP10ClientDomain: wallet.SEP10ClientDomain,
				DeepLinkSchema:    wallet.DeepLinkSchema,
				Enabled:           true,
			},
			Status:             receiverWallet.Status,
			OTP:                receiverWallet.OTP,
			StatusHistory:      actual.StatusHistory,
			OTPCreatedAt:       actual.OTPCreatedAt,
			OTPConfirmedAt:     actual.OTPConfirmedAt,
			OTPConfirmedWith:   actual.OTPConfirmedWith,
			CreatedAt:          actual.CreatedAt,
			UpdatedAt:          actual.UpdatedAt,
			StellarAddress:     receiverWallet.StellarAddress,
			StellarMemo:        "",
			StellarMemoType:    "",
			SEP24TransactionID: receiverWallet.SEP24TransactionID,
			InvitationSentAt:   actual.InvitationSentAt,
		}

		assert.Equal(t, expected, *actual)
	})

	t.Run("won't find a result if stellar account and memo are provided, but the DB memo is NULL", func(t *testing.T) {
		actual, err := receiverWalletModel.GetByStellarAccountAndMemo(ctx, receiverWallet.StellarAddress, wallet.SEP10ClientDomain, &receiverWallet.StellarMemo)
		require.ErrorIs(t, err, ErrRecordNotFound)
		require.Empty(t, actual)
	})
}

func Test_RetryInvitationSMS(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	receiverWalletModel := ReceiverWalletModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns error when receiver wallet does not exist", func(t *testing.T) {
		receiverWallet, err := receiverWalletModel.RetryInvitationMessage(ctx, dbConnectionPool, "invalid_id")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrRecordNotFound)
		require.Empty(t, receiverWallet)
	})

	t.Run("returns error when receiver wallet is registered", func(t *testing.T) {
		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		rw := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)

		receiverWallet, err := receiverWalletModel.RetryInvitationMessage(ctx, dbConnectionPool, rw.ID)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrRecordNotFound)
		require.Empty(t, receiverWallet)
	})

	t.Run("successfuly retry invitation", func(t *testing.T) {
		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		rw := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)

		receiverWallet, err := receiverWalletModel.RetryInvitationMessage(ctx, dbConnectionPool, rw.ID)
		require.NoError(t, err)
		assert.Nil(t, receiverWallet.InvitationSentAt)
	})
}

func Test_ReceiverWalletModelUpdateInvitationSentAt(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	receiverWalletModel := ReceiverWalletModel{dbConnectionPool: dbConnectionPool}

	t.Run("doesn't update when there's no receiver wallet IDs", func(t *testing.T) {
		receiverWallets, err := receiverWalletModel.UpdateInvitationSentAt(ctx, dbConnectionPool)
		require.NoError(t, err)
		assert.Empty(t, receiverWallets)
	})

	t.Run("doesn't update receiver wallets not in the READY status", func(t *testing.T) {
		DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)

		receiverWallets, err := receiverWalletModel.UpdateInvitationSentAt(ctx, dbConnectionPool, receiverWallet.ID)
		require.NoError(t, err)
		assert.Empty(t, receiverWallets)

		var invitationSentAt time.Time
		const q = "UPDATE receiver_wallets SET invitation_sent_at = NOW() - interval '2 days' WHERE id = $1 RETURNING invitation_sent_at"
		err = dbConnectionPool.GetContext(ctx, &invitationSentAt, q, receiverWallet.ID)
		require.NoError(t, err)

		receiverWallets, err = receiverWalletModel.UpdateInvitationSentAt(ctx, dbConnectionPool, receiverWallet.ID)
		require.NoError(t, err)
		assert.Empty(t, receiverWallets)

		receiverWalletsDB, err := receiverWalletModel.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiver.ID}, wallet.ID)
		require.NoError(t, err)
		require.Len(t, receiverWalletsDB, 1)
		assert.Equal(t, receiverWallet.ID, receiverWalletsDB[0].ID)
		assert.Equal(t, invitationSentAt, *receiverWalletsDB[0].InvitationSentAt)
	})

	t.Run("updates invitation sent at successfully", func(t *testing.T) {
		DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)

		receiverWallets, err := receiverWalletModel.UpdateInvitationSentAt(ctx, dbConnectionPool, receiverWallet.ID)
		require.NoError(t, err)
		require.Len(t, receiverWallets, 1)
		assert.Equal(t, receiverWallet.ID, receiverWallets[0].ID)
	})

	t.Run("updates invitation sent at when is already set", func(t *testing.T) {
		DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)

		var invitationSentAt time.Time
		const q = "UPDATE receiver_wallets SET invitation_sent_at = NOW() - interval '2 days' WHERE id = $1 RETURNING invitation_sent_at"
		err := dbConnectionPool.GetContext(ctx, &invitationSentAt, q, receiverWallet.ID)
		require.NoError(t, err)

		receiverWallets, err := receiverWalletModel.UpdateInvitationSentAt(ctx, dbConnectionPool, receiverWallet.ID)
		require.NoError(t, err)
		require.Len(t, receiverWallets, 1)
		assert.Equal(t, receiverWallet.ID, receiverWallets[0].ID)
		require.NotNil(t, receiverWallets[0].InvitationSentAt)
		assert.True(t, invitationSentAt.Before(*receiverWallets[0].InvitationSentAt))
	})
}

func Test_ReceiverWalletUpdate_Validate(t *testing.T) {
	testCases := []struct {
		name   string
		update ReceiverWalletUpdate
		err    string
	}{
		{
			name:   "empty update",
			update: ReceiverWalletUpdate{},
			err:    "no values provided to update receiver wallet",
		},
		{
			name: "invalid stellar address",
			update: ReceiverWalletUpdate{
				StellarAddress: "invalid",
			},
			err: "invalid stellar address",
		},
		{
			name: "invalid status",
			update: ReceiverWalletUpdate{
				Status: "invalid",
			},
			err: "validating status: invalid receiver wallet status \"invalid\"",
		},
		{
			name: "OTPConfirmedAt set without OTPConfirmedWith",
			update: ReceiverWalletUpdate{
				OTPConfirmedAt: time.Now(),
			},
			err: "OTPConfirmedWith is required when OTPConfirmedAt is provided",
		},
		{
			name: "OTPConfirmedWith set without OTPConfirmedAt",
			update: ReceiverWalletUpdate{
				OTPConfirmedWith: "test@email.com",
			},
			err: "OTPConfirmedAt is required when OTPConfirmedWith is provided",
		},
		{
			name: "valid update with public key",
			update: ReceiverWalletUpdate{
				Status:           RegisteredReceiversWalletStatus,
				StellarAddress:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
				OTPConfirmedAt:   time.Now(),
				OTPConfirmedWith: "test@email.com",
			},
			err: "",
		},
		{
			name: "valid update with contract address",
			update: ReceiverWalletUpdate{
				Status:           RegisteredReceiversWalletStatus,
				StellarAddress:   "CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53",
				OTPConfirmedAt:   time.Now(),
				OTPConfirmedWith: "test@email.com",
			},
			err: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.update.Validate()
			if tc.err == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.err)
			}
		})
	}
}

func Test_ReceiverWalletModel_Update(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	receiverWalletModel := ReceiverWalletModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns error when update is empty", func(t *testing.T) {
		err := receiverWalletModel.Update(ctx, "some-id", ReceiverWalletUpdate{}, dbConnectionPool)
		assert.EqualError(t, err, "validating receiver wallet update: no values provided to update receiver wallet")
	})

	t.Run("returns error when receiver wallet does not exist", func(t *testing.T) {
		update := ReceiverWalletUpdate{
			Status: RegisteredReceiversWalletStatus,
		}
		err := receiverWalletModel.Update(ctx, "invalid_id", update, dbConnectionPool)
		require.ErrorIs(t, err, ErrRecordNotFound)
	})

	t.Run("returns error when receiver wallet status is not valid", func(t *testing.T) {
		update := ReceiverWalletUpdate{
			Status: "invalid",
		}
		err := receiverWalletModel.Update(ctx, "some id", update, dbConnectionPool)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validating receiver wallet update: validating status: invalid receiver wallet status \"invalid\"")
	})

	contractAddress := "CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53"

	t.Run("returns error when stored stellar address is contract and memo update attempted", func(t *testing.T) {
		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)

		_, err := dbConnectionPool.ExecContext(ctx, "UPDATE receiver_wallets SET stellar_address = $1, stellar_memo = NULL, stellar_memo_type = NULL WHERE id = $2", contractAddress, receiverWallet.ID)
		require.NoError(t, err)

		update := ReceiverWalletUpdate{
			StellarMemo:     utils.Ptr("123456"),
			StellarMemoType: utils.Ptr(schema.MemoTypeID),
		}

		err = receiverWalletModel.Update(ctx, receiverWallet.ID, update, dbConnectionPool)
		require.ErrorIs(t, err, ErrMemosNotSupportedForContractAddresses)
	})

	t.Run("returns error when incoming stellar address is contract and memo update attempted", func(t *testing.T) {
		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)

		update := ReceiverWalletUpdate{
			StellarAddress:  contractAddress,
			StellarMemo:     utils.Ptr("123456"),
			StellarMemoType: utils.Ptr(schema.MemoTypeID),
		}

		err := receiverWalletModel.Update(ctx, receiverWallet.ID, update, dbConnectionPool)
		require.ErrorIs(t, err, ErrMemosNotSupportedForContractAddresses)
	})

	t.Run("successfully updates receiver wallet", func(t *testing.T) {
		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, DraftReceiversWalletStatus)

		now := time.Now()

		update := ReceiverWalletUpdate{
			Status:             RegisteredReceiversWalletStatus,
			SEP24TransactionID: "test-tx-id",
			StellarAddress:     "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
			StellarMemo:        utils.Ptr("123456"),
			StellarMemoType:    utils.Ptr(schema.MemoTypeID),
			OTPConfirmedAt:     now,
			OTPConfirmedWith:   "test@stellar.org",
		}

		err := receiverWalletModel.Update(ctx, receiverWallet.ID, update, dbConnectionPool)
		require.NoError(t, err)

		// Verify the update
		query := `
			SELECT
				rw.status,
				rw.anchor_platform_transaction_id,
				rw.stellar_address,
				rw.stellar_memo,
				rw.stellar_memo_type,
				rw.otp_confirmed_at,
				rw.otp_confirmed_with
			FROM
				receiver_wallets rw
			WHERE
				rw.id = $1
		`
		var updated ReceiverWallet
		err = dbConnectionPool.GetContext(ctx, &updated, query, receiverWallet.ID)
		require.NoError(t, err)

		assert.Equal(t, RegisteredReceiversWalletStatus, updated.Status)
		assert.Equal(t, "test-tx-id", updated.SEP24TransactionID)
		assert.Equal(t, "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444", updated.StellarAddress)
		assert.Equal(t, "123456", updated.StellarMemo)
		assert.Equal(t, schema.MemoTypeID, updated.StellarMemoType)
		assert.WithinDuration(t, now.UTC(), updated.OTPConfirmedAt.UTC(), time.Microsecond)
		assert.Equal(t, "test@stellar.org", updated.OTPConfirmedWith)

		// Verify status history was updated
		var statusHistory ReceiversWalletStatusHistory
		err = dbConnectionPool.GetContext(ctx, &statusHistory, "SELECT status_history FROM receiver_wallets WHERE id = $1", receiverWallet.ID)
		require.NoError(t, err)
		assert.Equal(t, RegisteredReceiversWalletStatus, statusHistory[0].Status)
	})
}

func Test_ReceiverWalletModel_GetByIDs(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	receiverWalletModel := ReceiverWalletModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns error when no receiver wallet IDs are provided", func(t *testing.T) {
		rws, err := receiverWalletModel.GetByIDs(ctx, dbConnectionPool)
		assert.EqualError(t, err, "no receiver wallet IDs provided")
		assert.Empty(t, rws)
	})

	t.Run("returns no receiver wallets when IDs are invalid", func(t *testing.T) {
		rws, err := receiverWalletModel.GetByIDs(ctx, dbConnectionPool, "invalid_id")
		require.NoError(t, err)
		assert.Empty(t, rws)
	})

	t.Run("🎉successfully return receiver wallet when it exists", func(t *testing.T) {
		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, DraftReceiversWalletStatus)

		rws, err := receiverWalletModel.GetByIDs(ctx, dbConnectionPool, receiverWallet.ID)
		require.NoError(t, err)
		require.Len(t, rws, 1)
		assert.Equal(t, receiverWallet.ID, rws[0].ID)
	})
}

func Test_ReceiverWalletModel_UpdateStatusToReady(t *testing.T) {
	ctx := context.Background()
	models := SetupModels(t)
	rwModel := models.ReceiverWallet
	dbcp := models.DBConnectionPool

	receiver := CreateReceiverFixture(t, ctx, dbcp, &Receiver{})
	wallet := CreateDefaultWalletFixture(t, ctx, dbcp) // not user-managed

	t.Run("record not found", func(t *testing.T) {
		err := rwModel.UpdateStatusToReady(ctx, "non-existent-id")
		require.ErrorIs(t, err, ErrRecordNotFound)
	})

	t.Run("status not REGISTERED", func(t *testing.T) {
		rw := CreateReceiverWalletFixture(t, ctx, dbcp, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)
		t.Cleanup(func() { DeleteAllReceiverWalletsFixtures(t, ctx, dbcp) })

		err := rwModel.UpdateStatusToReady(ctx, rw.ID)
		require.ErrorIs(t, err, ErrWalletNotRegistered)
	})

	t.Run("user-managed wallet", func(t *testing.T) {
		userManagedWallet := CreateWalletFixture(t, ctx, dbcp, "User Managed Wallet", "stellar.org", "stellar.org", "stellar://")
		_, err := dbcp.ExecContext(ctx, `UPDATE wallets SET user_managed = TRUE WHERE id = $1`, userManagedWallet.ID)
		require.NoError(t, err)

		rw := CreateReceiverWalletFixture(t, ctx, dbcp, receiver.ID, userManagedWallet.ID, RegisteredReceiversWalletStatus)
		t.Cleanup(func() { DeleteAllReceiverWalletsFixtures(t, ctx, dbcp) })

		err = rwModel.UpdateStatusToReady(ctx, rw.ID)
		require.ErrorIs(t, err, ErrUnregisterUserManagedWallet)
	})

	t.Run("payments in progress", func(t *testing.T) {
		rw := CreateReceiverWalletFixture(t, ctx, dbcp, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)
		dis := CreateDisbursementFixture(t, ctx, dbcp, models.Disbursements, &Disbursement{})

		CreatePaymentFixture(t, ctx, dbcp, models.Payment, &Payment{
			Amount:         "50",
			Asset:          *dis.Asset,
			Status:         ReadyPaymentStatus,
			ReceiverWallet: rw,
			Disbursement:   dis,
		})
		t.Cleanup(func() {
			DeleteAllPaymentsFixtures(t, ctx, dbcp)
			DeleteAllReceiverWalletsFixtures(t, ctx, dbcp)
		})

		err := rwModel.UpdateStatusToReady(ctx, rw.ID)
		require.ErrorIs(t, err, ErrPaymentsInProgressForWallet)
	})

	t.Run("REGISTERED → READY happy-path", func(t *testing.T) {
		rw := CreateReceiverWalletFixture(t, ctx, dbcp, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)
		t.Cleanup(func() { DeleteAllReceiverWalletsFixtures(t, ctx, dbcp) })

		require.NoError(t, rwModel.UpdateStatusToReady(ctx, rw.ID))

		rw, err := rwModel.GetByID(ctx, dbcp, rw.ID)
		require.NoError(t, err)
		assert.Equal(t, ReadyReceiversWalletStatus, rw.Status)
		assert.Empty(t, rw.StellarAddress)
		assert.Empty(t, rw.StellarMemo)
		assert.Empty(t, rw.StellarMemoType)
		assert.Empty(t, rw.InvitationSentAt)
		assert.Empty(t, rw.OTP)
		assert.Empty(t, rw.OTPConfirmedAt)
		assert.Empty(t, rw.OTPConfirmedWith)
		assert.Empty(t, rw.OTPCreatedAt)
		assert.Empty(t, rw.SEP24TransactionID)
	})
}

func TestReceiverWalletModel_HasPaymentsInProgress(t *testing.T) {
	ctx := context.Background()
	models := SetupModels(t)
	dbPool := models.DBConnectionPool

	receiver := CreateReceiverFixture(t, ctx, dbPool, &Receiver{})
	wallet := CreateDefaultWalletFixture(t, ctx, dbPool)
	rw := CreateReceiverWalletFixture(t, ctx, dbPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)
	d := CreateDisbursementFixture(t, ctx, dbPool, models.Disbursements, &Disbursement{})

	cases := []struct {
		status PaymentStatus
		want   bool
	}{
		{PendingPaymentStatus, true},
		{ReadyPaymentStatus, true},
		{PausedPaymentStatus, true},
		{DraftPaymentStatus, false},
		{CanceledPaymentStatus, false},
		{FailedPaymentStatus, false},
		{SuccessPaymentStatus, false},
	}

	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			CreatePaymentFixture(t, ctx, dbPool, models.Payment, &Payment{
				Amount:         "100",
				Asset:          *d.Asset,
				Status:         tc.status,
				ReceiverWallet: rw,
				Disbursement:   d,
			})

			t.Cleanup(func() {
				DeleteAllPaymentsFixtures(t, ctx, dbPool)
			})

			got, err := models.ReceiverWallet.HasPaymentsInProgress(ctx, dbPool, rw.ID)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func Test_ReceiverWalletModel_Insert_StellarAddressConstraint(t *testing.T) {
	ctx := context.Background()
	models := SetupModels(t)
	dbConnectionPool := models.DBConnectionPool

	stellarAddress := "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444"

	t.Run("insert without stellar_address succeeds", func(t *testing.T) {
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})

		insert := ReceiverWalletInsert{
			ReceiverID: receiver.ID,
			WalletID:   wallet.ID,
		}

		rwID, err := models.ReceiverWallet.GetOrInsertReceiverWallet(ctx, dbConnectionPool, insert)
		require.NoError(t, err)
		require.NotEmpty(t, rwID)

		DeleteAllFixtures(t, ctx, dbConnectionPool)
	})

	t.Run("insert + update with unique stellar_address succeeds", func(t *testing.T) {
		wallet1 := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		wallet2 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})

		insert := ReceiverWalletInsert{
			ReceiverID: wallet2.ID,
			WalletID:   wallet1.ID,
		}

		rwID, err := models.ReceiverWallet.GetOrInsertReceiverWallet(ctx, dbConnectionPool, insert)
		require.NoError(t, err)
		require.NotEmpty(t, rwID)

		update := ReceiverWalletUpdate{
			StellarAddress: stellarAddress,
		}

		err = models.ReceiverWallet.Update(ctx, rwID, update, dbConnectionPool)
		require.NoError(t, err)

		DeleteAllFixtures(t, ctx, dbConnectionPool)
	})

	t.Run("same receiver can use same stellar_address across different wallets", func(t *testing.T) {
		wallet1 := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		wallet2 := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet2", "https://www.wallet2.com", "www.wallet2.com", "wallet2://")

		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})

		insert1 := ReceiverWalletInsert{
			ReceiverID: receiver.ID,
			WalletID:   wallet1.ID,
		}

		rwID1, err := models.ReceiverWallet.GetOrInsertReceiverWallet(ctx, dbConnectionPool, insert1)
		require.NoError(t, err)

		update1 := ReceiverWalletUpdate{
			StellarAddress: stellarAddress,
		}

		err = models.ReceiverWallet.Update(ctx, rwID1, update1, dbConnectionPool)
		require.NoError(t, err)

		insert2 := ReceiverWalletInsert{
			ReceiverID: receiver.ID,
			WalletID:   wallet2.ID,
		}

		rwID2, err := models.ReceiverWallet.GetOrInsertReceiverWallet(ctx, dbConnectionPool, insert2)
		require.NoError(t, err)

		update2 := ReceiverWalletUpdate{
			StellarAddress: stellarAddress,
		}

		err = models.ReceiverWallet.Update(ctx, rwID2, update2, dbConnectionPool)
		require.NoError(t, err, "Same receiver should be able to use same stellar address across different wallets")

		DeleteAllFixtures(t, ctx, dbConnectionPool)
	})

	t.Run("different receivers cannot use same stellar_address", func(t *testing.T) {
		wallet1 := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		wallet2 := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet2", "https://www.wallet2.com", "www.wallet2.com", "wallet2://")

		receiver1 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiver2 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})

		insert1 := ReceiverWalletInsert{
			ReceiverID: receiver1.ID,
			WalletID:   wallet1.ID,
		}

		rwID1, err := models.ReceiverWallet.GetOrInsertReceiverWallet(ctx, dbConnectionPool, insert1)
		require.NoError(t, err)

		update1 := ReceiverWalletUpdate{
			StellarAddress: stellarAddress,
		}

		err = models.ReceiverWallet.Update(ctx, rwID1, update1, dbConnectionPool)
		require.NoError(t, err)

		insert2 := ReceiverWalletInsert{
			ReceiverID: receiver2.ID,
			WalletID:   wallet2.ID,
		}

		rwID2, err := models.ReceiverWallet.GetOrInsertReceiverWallet(ctx, dbConnectionPool, insert2)
		require.NoError(t, err)

		update2 := ReceiverWalletUpdate{
			StellarAddress: stellarAddress,
		}

		err = models.ReceiverWallet.Update(ctx, rwID2, update2, dbConnectionPool)
		require.Error(t, err)
		require.Contains(t, err.Error(), "wallet address already in use")

		DeleteAllFixtures(t, ctx, dbConnectionPool)
	})

	t.Run("null stellar_address always succeeds", func(t *testing.T) {
		wallet1 := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		wallet2 := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet2", "https://www.wallet2.com", "www.wallet2.com", "wallet2://")

		receiver1 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiver2 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})

		insert1 := ReceiverWalletInsert{
			ReceiverID: receiver1.ID,
			WalletID:   wallet1.ID,
		}

		rwID1, err := models.ReceiverWallet.GetOrInsertReceiverWallet(ctx, dbConnectionPool, insert1)
		require.NoError(t, err)

		insert2 := ReceiverWalletInsert{
			ReceiverID: receiver2.ID,
			WalletID:   wallet2.ID,
		}

		rwID2, err := models.ReceiverWallet.GetOrInsertReceiverWallet(ctx, dbConnectionPool, insert2)
		require.NoError(t, err)

		_, err = dbConnectionPool.ExecContext(ctx, "DELETE FROM receiver_wallets WHERE id IN ($1, $2)", rwID1, rwID2)
		require.NoError(t, err)
	})

	t.Run("direct database insert with conflicting stellar_address fails", func(t *testing.T) {
		wallet1 := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		wallet2 := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet2", "https://www.wallet2.com", "www.wallet2.com", "wallet2://")

		receiver1 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiver2 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})

		query1 := `
			INSERT INTO receiver_wallets (receiver_id, wallet_id, stellar_address)
			VALUES ($1, $2, $3)
			RETURNING id
		`

		var rwID1 string
		err := dbConnectionPool.GetContext(ctx, &rwID1, query1, receiver1.ID, wallet1.ID, stellarAddress)
		require.NoError(t, err)

		query2 := `
			INSERT INTO receiver_wallets (receiver_id, wallet_id, stellar_address)
			VALUES ($1, $2, $3)
			RETURNING id
		`

		var rwID2 string
		err = dbConnectionPool.GetContext(ctx, &rwID2, query2, receiver2.ID, wallet2.ID, stellarAddress)
		require.Error(t, err)
		require.Contains(t, err.Error(), "already belongs to another receiver")

		DeleteAllFixtures(t, ctx, dbConnectionPool)
	})

	t.Run("update existing receiver_wallet to conflicting stellar_address fails", func(t *testing.T) {
		wallet1 := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		wallet2 := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet2", "https://www.wallet2.com", "www.wallet2.com", "wallet2://")

		receiver1 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiver2 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})

		insert1 := ReceiverWalletInsert{
			ReceiverID: receiver1.ID,
			WalletID:   wallet1.ID,
		}

		rwID1, err := models.ReceiverWallet.GetOrInsertReceiverWallet(ctx, dbConnectionPool, insert1)
		require.NoError(t, err)

		insert2 := ReceiverWalletInsert{
			ReceiverID: receiver2.ID,
			WalletID:   wallet2.ID,
		}

		rwID2, err := models.ReceiverWallet.GetOrInsertReceiverWallet(ctx, dbConnectionPool, insert2)
		require.NoError(t, err)

		update1 := ReceiverWalletUpdate{
			StellarAddress: stellarAddress,
		}

		err = models.ReceiverWallet.Update(ctx, rwID1, update1, dbConnectionPool)
		require.NoError(t, err)

		update2 := ReceiverWalletUpdate{
			StellarAddress: stellarAddress,
		}

		err = models.ReceiverWallet.Update(ctx, rwID2, update2, dbConnectionPool)
		require.Error(t, err)
		require.Contains(t, err.Error(), "wallet address already in use")

		DeleteAllFixtures(t, ctx, dbConnectionPool)
	})
}
