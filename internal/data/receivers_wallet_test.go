package data

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stellar/go/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

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

		actual, errReceiver := receiverWalletModel.GetWithReceiverIds(ctx, dbTx, ReceiverIDs{"invalid_id"})
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

		actual, errReceiver := receiverWalletModel.GetWithReceiverIds(ctx, dbTx, ReceiverIDs{receiver.ID})
		require.NoError(t, errReceiver)
		require.Empty(t, actual)

		err = dbTx.Commit()
		require.NoError(t, err)
	})

	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
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
		Status:  DraftDisbursementStatus,
		Asset:   asset,
		Country: country,
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

		actual, err := receiverWalletModel.GetWithReceiverIds(ctx, dbTx, ReceiverIDs{receiver.ID})
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
				StellarAddress:  receiverWallet1.StellarAddress,
				StellarMemo:     receiverWallet1.StellarMemo,
				StellarMemoType: receiverWallet1.StellarMemoType,
				Status:          receiverWallet1.Status,
				CreatedAt:       receiverWallet1.CreatedAt,
				UpdatedAt:       receiverWallet1.CreatedAt,
				InvitedAt:       &message1.CreatedAt,
				LastSmsSent:     &message2.CreatedAt,
				ReceiverWalletStats: ReceiverWalletStats{
					TotalPayments:     "0",
					PaymentsReceived:  "0",
					FailedPayments:    "0",
					CanceledPayments:  "0",
					RemainingPayments: "0",
					ReceivedAmounts:   nil,
				},
				AnchorPlatformTransactionID: receiverWallet1.AnchorPlatformTransactionID,
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

		actual, err := receiverWalletModel.GetWithReceiverIds(ctx, dbTx, ReceiverIDs{receiver.ID})
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
				StellarAddress:  receiverWallet1.StellarAddress,
				StellarMemo:     receiverWallet1.StellarMemo,
				StellarMemoType: receiverWallet1.StellarMemoType,
				Status:          receiverWallet1.Status,
				CreatedAt:       receiverWallet1.CreatedAt,
				UpdatedAt:       receiverWallet1.CreatedAt,
				InvitedAt:       &message1.CreatedAt,
				LastSmsSent:     &message2.CreatedAt,
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
				AnchorPlatformTransactionID: receiverWallet1.AnchorPlatformTransactionID,
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

		actual, err := receiverWalletModel.GetWithReceiverIds(ctx, dbTx, ReceiverIDs{receiver.ID})
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
				StellarAddress:  receiverWallet1.StellarAddress,
				StellarMemo:     receiverWallet1.StellarMemo,
				StellarMemoType: receiverWallet1.StellarMemoType,
				Status:          receiverWallet1.Status,
				CreatedAt:       receiverWallet1.CreatedAt,
				UpdatedAt:       receiverWallet1.CreatedAt,
				InvitedAt:       &message1.CreatedAt,
				LastSmsSent:     &message2.CreatedAt,
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
				AnchorPlatformTransactionID: receiverWallet1.AnchorPlatformTransactionID,
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
				StellarAddress:  receiverWallet2.StellarAddress,
				StellarMemo:     receiverWallet2.StellarMemo,
				StellarMemoType: receiverWallet2.StellarMemoType,
				Status:          receiverWallet2.Status,
				CreatedAt:       receiverWallet2.CreatedAt,
				UpdatedAt:       receiverWallet2.CreatedAt,
				InvitedAt:       &message3.CreatedAt,
				LastSmsSent:     &message4.CreatedAt,
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
				AnchorPlatformTransactionID: receiverWallet2.AnchorPlatformTransactionID,
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

		actual, err := receiverWalletModel.GetWithReceiverIds(ctx, dbTx, ReceiverIDs{receiver.ID})
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
				StellarAddress:  receiverWallet.StellarAddress,
				StellarMemo:     receiverWallet.StellarMemo,
				StellarMemoType: receiverWallet.StellarMemoType,
				Status:          receiverWallet.Status,
				CreatedAt:       receiverWallet.CreatedAt,
				UpdatedAt:       receiverWallet.CreatedAt,
				InvitedAt:       &message1.CreatedAt,
				LastSmsSent:     &message2.CreatedAt,
				ReceiverWalletStats: ReceiverWalletStats{
					TotalPayments:     "0",
					PaymentsReceived:  "0",
					FailedPayments:    "0",
					CanceledPayments:  "0",
					RemainingPayments: "0",
					ReceivedAmounts:   nil,
				},
				AnchorPlatformTransactionID: receiverWallet.AnchorPlatformTransactionID,
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
				SEP10ClientDomain: wallet.SEP10ClientDomain,
			},
			Status:                      receiverWallet.Status,
			StellarAddress:              receiverWallet.StellarAddress,
			StellarMemo:                 receiverWallet.StellarMemo,
			StellarMemoType:             receiverWallet.StellarMemoType,
			OTP:                         "123456",
			OTPCreatedAt:                receiverWallet.OTPCreatedAt,
			OTPConfirmedAt:              nil,
			AnchorPlatformTransactionID: receiverWallet.AnchorPlatformTransactionID,
		}

		assert.Equal(t, expected, *actual)
	})
}

func Test_UpdateReceiverWallet(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	receiverWalletModel := ReceiverWalletModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns error when receiver wallet does not exist", func(t *testing.T) {
		err := receiverWalletModel.UpdateReceiverWallet(ctx, ReceiverWallet{ID: "invalid_id", Status: DraftReceiversWalletStatus}, dbConnectionPool)
		require.ErrorIs(t, err, ErrRecordNotFound)
	})

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")
	receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, DraftReceiversWalletStatus)

	t.Run("returns error when status is not valid", func(t *testing.T) {
		receiverWallet.Status = "invalid_status"
		err := receiverWalletModel.UpdateReceiverWallet(ctx, *receiverWallet, dbConnectionPool)
		require.Error(t, err, "querying receiver wallet: sql: no rows in result set")
	})

	t.Run("successfuly update receiver wallet", func(t *testing.T) {
		receiverWallet.AnchorPlatformTransactionID = "test-anchor-tx-platform-id"
		receiverWallet.StellarAddress = "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444"
		receiverWallet.StellarMemo = "123456"
		receiverWallet.StellarMemoType = "id"
		receiverWallet.Status = RegisteredReceiversWalletStatus
		now := time.Now()
		receiverWallet.OTPConfirmedAt = &now

		err := receiverWalletModel.UpdateReceiverWallet(ctx, *receiverWallet, dbConnectionPool)
		require.NoError(t, err)

		// validate if the receiver wallet has been updated
		query := `
			SELECT
				rw.status,
				rw.anchor_platform_transaction_id,
				rw.stellar_address,
				rw.stellar_memo,
				rw.stellar_memo_type,
				otp_confirmed_at
			FROM
				receiver_wallets rw
			WHERE
				rw.id = $1
		`
		receiverWalletUpdated := ReceiverWallet{}
		err = dbConnectionPool.GetContext(ctx, &receiverWalletUpdated, query, receiverWallet.ID)
		require.NoError(t, err)

		assert.Equal(t, RegisteredReceiversWalletStatus, receiverWalletUpdated.Status)
		assert.Equal(t, "test-anchor-tx-platform-id", receiverWalletUpdated.AnchorPlatformTransactionID)
		assert.Equal(t, "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444", receiverWalletUpdated.StellarAddress)
		assert.Equal(t, "123456", receiverWalletUpdated.StellarMemo)
		assert.Equal(t, "id", receiverWalletUpdated.StellarMemoType)
		assert.WithinDuration(t, now, *receiverWalletUpdated.OTPConfirmedAt, 100*time.Millisecond)
	})
}

func Test_ReceiverWallet_UpdateOTPByReceiverContactInfoAndWalletDomain(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

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
	}{
		{
			name: "does not update OTP for a receiver wallet with a different contact info",
			setupReceiverWallet: func(t *testing.T, receiver Receiver) {
				_ = CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)
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
				_ = CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)
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
				_ = CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)
			},
			contactInfo: func(r Receiver, contactType ReceiverContactType) string {
				return r.ContactByType(contactType)
			},
			clientDomain: wallet.SEP10ClientDomain,
			expectedRows: 1,
		},
		{
			name: "🎉 successfully renews OTP for an unconfirmed receiver wallet",
			setupReceiverWallet: func(t *testing.T, receiver Receiver) {
				// Create a receiver with a different contact info toi make sure they will not be picked by the query
				receiverNoOp := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{PhoneNumber: "+141555550000", Email: "zoopbar@test.com"})
				rwNoOp := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverNoOp.ID, wallet.ID, RegisteredReceiversWalletStatus)

				// Confirm OTP for the first receiver
				q := `UPDATE receiver_wallets SET otp_confirmed_at = NOW() WHERE id = $1`
				_, err := dbConnectionPool.ExecContext(ctx, q, rwNoOp.ID)
				require.NoError(t, err)

				_ = CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)
			},
			contactInfo: func(r Receiver, contactType ReceiverContactType) string {
				return r.ContactByType(contactType)
			},
			clientDomain: wallet.SEP10ClientDomain,
			expectedRows: 1,
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
					q := `SELECT otp FROM receiver_wallets WHERE receiver_id = $1 AND wallet_id = $2`
					var dbOTP string
					err := dbConnectionPool.GetContext(ctx, &dbOTP, q, receiver.ID, wallet.ID)
					require.NoError(t, err)
					assert.Equal(t, otp, dbOTP)
				}
			})
		}
	}
}

func Test_VerifyReceiverWalletOTP(t *testing.T) {
	ctx := context.Background()
	receiverWalletModel := ReceiverWalletModel{}

	expiredOTPCreatedAt := time.Now().Add(-OTPExpirationTimeMinutes * time.Minute).Add(-time.Second) // expired 1 second ago
	validOTPTime := time.Now()

	testCases := []struct {
		name              string
		networkPassphrase string
		attemptedOTP      string
		otp               string
		otpCreatedAt      time.Time
		wantErr           error
	}{
		// mismatching OTP fails:
		{
			name:              "mismatching OTP fails",
			networkPassphrase: network.TestNetworkPassphrase,
			attemptedOTP:      "123123",
			otp:               "123456",
			otpCreatedAt:      time.Time{}, // invalid created_at
			wantErr:           fmt.Errorf("otp does not match with value saved in the database"),
		},
		{
			name:              "mismatching OTP fails when passing the TestnetAlwaysValidOTP in Pubnet",
			networkPassphrase: network.PublicNetworkPassphrase,
			attemptedOTP:      TestnetAlwaysValidOTP,
			otp:               "123456",
			otpCreatedAt:      time.Time{}, // invalid created_at
			wantErr:           fmt.Errorf("otp does not match with value saved in the database"),
		},
		{
			name:              "mismatching OTP succeeds when passing the TestnetAlwaysValidOTP in Testnet",
			networkPassphrase: network.TestNetworkPassphrase,
			attemptedOTP:      TestnetAlwaysValidOTP,
			otp:               "123456",
			otpCreatedAt:      time.Time{}, // invalid created_at
			wantErr:           nil,
		},

		// matching OTP fails when its created_at date is invalid:
		{
			name:              "matching OTP fails when its created_at date is invalid",
			networkPassphrase: network.TestNetworkPassphrase,
			attemptedOTP:      "123456",
			otp:               "123456",
			otpCreatedAt:      time.Time{}, // invalid created_at
			wantErr:           fmt.Errorf("otp does not have a valid created_at time"),
		},
		{
			name:              "matching OTP fails when its created_at date is invalid and we pass TestnetAlwaysValidOTP in Pubnet",
			networkPassphrase: network.PublicNetworkPassphrase,
			attemptedOTP:      TestnetAlwaysValidOTP,
			otp:               TestnetAlwaysValidOTP,
			otpCreatedAt:      time.Time{}, // invalid created_at
			wantErr:           fmt.Errorf("otp does not have a valid created_at time"),
		},
		{
			name:              "matching OTP succeeds when its created_at date is invalid but we pass TestnetAlwaysValidOTP in Testnet",
			networkPassphrase: network.TestNetworkPassphrase,
			attemptedOTP:      TestnetAlwaysValidOTP,
			otp:               "123456",
			otpCreatedAt:      time.Time{}, // invalid created_at
			wantErr:           nil,
		},

		// returns error when otp is expired:
		{
			name:              "matching OTP fails when OTP is expired",
			networkPassphrase: network.TestNetworkPassphrase,
			attemptedOTP:      "123456",
			otp:               "123456",
			otpCreatedAt:      expiredOTPCreatedAt,
			wantErr:           fmt.Errorf("otp is expired"),
		},
		{
			name:              "matching OTP fails when OTP is expired and we pass TestnetAlwaysValidOTP in Pubnet",
			networkPassphrase: network.PublicNetworkPassphrase,
			attemptedOTP:      TestnetAlwaysValidOTP,
			otp:               TestnetAlwaysValidOTP,
			otpCreatedAt:      expiredOTPCreatedAt,
			wantErr:           fmt.Errorf("otp is expired"),
		},
		{
			name:              "matching OTP fails when OTP is expired but we pass TestnetAlwaysValidOTP in Testnet",
			networkPassphrase: network.TestNetworkPassphrase,
			attemptedOTP:      TestnetAlwaysValidOTP,
			otp:               "123456",
			otpCreatedAt:      expiredOTPCreatedAt,
			wantErr:           nil,
		},

		// OTP is valid 🎉
		{
			name:              "OTP is valid 🎉",
			networkPassphrase: network.TestNetworkPassphrase,
			attemptedOTP:      "123456",
			otp:               "123456",
			otpCreatedAt:      validOTPTime,
			wantErr:           nil,
		},
		{
			name:              "OTP is valid 🎉 also when we pass TestnetAlwaysValidOTP in Pubnet",
			networkPassphrase: network.PublicNetworkPassphrase,
			attemptedOTP:      TestnetAlwaysValidOTP,
			otp:               TestnetAlwaysValidOTP,
			otpCreatedAt:      validOTPTime,
			wantErr:           nil,
		},
		{
			name:              "OTP is valid 🎉 also when we pass TestnetAlwaysValidOTP in Testnet",
			networkPassphrase: network.TestNetworkPassphrase,
			attemptedOTP:      TestnetAlwaysValidOTP,
			otp:               TestnetAlwaysValidOTP,
			otpCreatedAt:      validOTPTime,
			wantErr:           nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			receiverWallet := ReceiverWallet{
				OTP:          tc.otp,
				OTPCreatedAt: &tc.otpCreatedAt,
			}
			err := receiverWalletModel.VerifyReceiverWalletOTP(ctx, tc.networkPassphrase, receiverWallet, tc.attemptedOTP)
			if tc.wantErr != nil {
				assert.Equal(t, tc.wantErr, err)
			} else {
				assert.NoError(t, err)
			}
		})
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
		actual, innerErr := receiverWalletModel.GetByStellarAccountAndMemo(ctx, "GCRSI42IC7WSW6N46LWPAHQWFI6MLGPBN3BYQ2WMNJ43GNRTIEYCAD6O", "", wallet.SEP10ClientDomain)
		require.ErrorIs(t, innerErr, ErrRecordNotFound)
		require.Empty(t, actual)
	})

	receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, DraftReceiversWalletStatus)
	results, err := receiverWalletModel.UpdateOTPByReceiverContactInfoAndWalletDomain(ctx, receiver.PhoneNumber, wallet.SEP10ClientDomain, "123456")
	require.NoError(t, err)
	require.Equal(t, 1, results)

	t.Run("wont find the result if stellar address is provided but memo is not", func(t *testing.T) {
		actual, innerErr := receiverWalletModel.GetByStellarAccountAndMemo(ctx, receiverWallet.StellarAddress, "", wallet.SEP10ClientDomain)
		require.ErrorIs(t, innerErr, ErrRecordNotFound)
		require.Empty(t, actual)
	})

	t.Run("wont find the result if memo is provided but stellar address is not", func(t *testing.T) {
		actual, innerErr := receiverWalletModel.GetByStellarAccountAndMemo(ctx, "", receiverWallet.StellarMemo, wallet.SEP10ClientDomain)
		require.ErrorIs(t, innerErr, ErrRecordNotFound)
		require.Empty(t, actual)
	})

	t.Run("returns receiver_wallet when both stellar account and memo are provided", func(t *testing.T) {
		actual, innerErr := receiverWalletModel.GetByStellarAccountAndMemo(ctx, receiverWallet.StellarAddress, receiverWallet.StellarMemo, wallet.SEP10ClientDomain)
		require.NoError(t, innerErr)

		expected := ReceiverWallet{
			ID:       receiverWallet.ID,
			Receiver: Receiver{ID: receiver.ID},
			Wallet: Wallet{
				ID:       wallet.ID,
				Name:     wallet.Name,
				Homepage: wallet.Homepage,
			},
			Status:                      receiverWallet.Status,
			OTP:                         "123456",
			OTPCreatedAt:                actual.OTPCreatedAt,
			StellarAddress:              receiverWallet.StellarAddress,
			StellarMemo:                 receiverWallet.StellarMemo,
			StellarMemoType:             receiverWallet.StellarMemoType,
			AnchorPlatformTransactionID: receiverWallet.AnchorPlatformTransactionID,
		}

		assert.Equal(t, expected, *actual)
	})

	query := `UPDATE receiver_wallets SET stellar_memo = NULL, stellar_memo_type = NULL WHERE id = $1`
	_, err = dbConnectionPool.ExecContext(ctx, query, receiverWallet.ID)
	require.NoError(t, err)

	t.Run("returns receiver_wallet when stellar account is provided and memo is null", func(t *testing.T) {
		actual, err := receiverWalletModel.GetByStellarAccountAndMemo(ctx, receiverWallet.StellarAddress, "", wallet.SEP10ClientDomain)
		require.NoError(t, err)

		expected := ReceiverWallet{
			ID:       receiverWallet.ID,
			Receiver: Receiver{ID: receiver.ID},
			Wallet: Wallet{
				ID:       wallet.ID,
				Name:     wallet.Name,
				Homepage: wallet.Homepage,
			},
			Status:                      receiverWallet.Status,
			OTP:                         "123456",
			OTPCreatedAt:                actual.OTPCreatedAt,
			StellarAddress:              receiverWallet.StellarAddress,
			StellarMemo:                 "",
			StellarMemoType:             "",
			AnchorPlatformTransactionID: receiverWallet.AnchorPlatformTransactionID,
		}

		assert.Equal(t, expected, *actual)
	})

	t.Run("won't find a result if stellar account and memo are provided, but the DB memo is NULL", func(t *testing.T) {
		actual, err := receiverWalletModel.GetByStellarAccountAndMemo(ctx, receiverWallet.StellarAddress, receiverWallet.StellarMemo, wallet.SEP10ClientDomain)
		require.ErrorIs(t, err, ErrRecordNotFound)
		require.Empty(t, actual)
	})
}

func Test_ReceiverWalletModelUpdateAnchorPlatformTransactionSyncedAt(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	receiverWalletModel := ReceiverWalletModel{dbConnectionPool: dbConnectionPool}

	t.Run("doesn't update when there's no receiver wallet IDs", func(t *testing.T) {
		receiverWallets, err := receiverWalletModel.UpdateAnchorPlatformTransactionSyncedAt(ctx, dbConnectionPool)
		require.NoError(t, err)
		assert.Empty(t, receiverWallets)
	})

	t.Run("doesn't update receiver wallets not in the REGISTERED status", func(t *testing.T) {
		DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)

		receiverWallets, err := receiverWalletModel.UpdateAnchorPlatformTransactionSyncedAt(ctx, dbConnectionPool, receiverWallet.ID)
		require.NoError(t, err)
		assert.Empty(t, receiverWallets)
	})

	t.Run("updates anchor platform transaction synced at successfully", func(t *testing.T) {
		DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)

		receiverWallets, err := receiverWalletModel.UpdateAnchorPlatformTransactionSyncedAt(ctx, dbConnectionPool, receiverWallet.ID)
		require.NoError(t, err)
		require.Len(t, receiverWallets, 1)
		assert.Equal(t, receiverWallet.ID, receiverWallets[0].ID)
	})

	t.Run("doesn't updates anchor platform transaction synced at when is already synced", func(t *testing.T) {
		DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)

		const q = "UPDATE receiver_wallets SET anchor_platform_transaction_synced_at = NOW() WHERE id = $1"
		_, err := dbConnectionPool.ExecContext(ctx, q, receiverWallet.ID)
		require.NoError(t, err)

		receiverWallets, err := receiverWalletModel.UpdateAnchorPlatformTransactionSyncedAt(ctx, dbConnectionPool, receiverWallet.ID)
		require.NoError(t, err)
		assert.Empty(t, receiverWallets)
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
