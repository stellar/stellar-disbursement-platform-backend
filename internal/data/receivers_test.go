package data

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ReceiversModelGet(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, DraftReceiversWalletStatus)

	disbursementModel := DisbursementModel{dbConnectionPool: dbConnectionPool}
	disbursement := Disbursement{
		Status:  DraftDisbursementStatus,
		Asset:   asset,
		Country: country,
		Wallet:  wallet,
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
		ReceiverWallet:       receiverWallet,
	}

	receiverModel := ReceiverModel{}

	t.Run("returns error when receiver does not exist", func(t *testing.T) {
		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		// Defer a rollback in case anything fails.
		defer func() {
			err = dbTx.Rollback()
			require.Error(t, err, "not in transaction")
		}()

		gotReceiver, err := receiverModel.Get(ctx, dbTx, "invalid_id")
		require.Error(t, err)
		require.ErrorIs(t, ErrRecordNotFound, err)
		require.Nil(t, gotReceiver)

		err = dbTx.Commit()
		require.NoError(t, err)
	})

	t.Run("returns receiver without payments", func(t *testing.T) {
		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)

		// Defer a rollback in case anything fails.
		defer func() {
			err = dbTx.Rollback()
			require.Error(t, err, "not in transaction")
		}()

		actual, err := receiverModel.Get(ctx, dbTx, receiver.ID)
		require.NoError(t, err)

		expected := Receiver{
			ID:          receiver.ID,
			ExternalID:  receiver.ExternalID,
			Email:       receiver.Email,
			PhoneNumber: receiver.PhoneNumber,
			CreatedAt:   receiver.CreatedAt,
			UpdatedAt:   receiver.UpdatedAt,
			ReceiverStats: ReceiverStats{
				TotalPayments:      "0",
				SuccessfulPayments: "0",
				FailedPayments:     "0",
				CanceledPayments:   "0",
				RemainingPayments:  "0",
				RegisteredWallets:  "0",
				ReceivedAmounts:    nil,
			},
		}
		assert.Equal(t, expected, *actual)

		err = dbTx.Commit()
		require.NoError(t, err)
	})

	t.Run("returns receiver with payment", func(t *testing.T) {
		disbursement.Name = "disbursement 1"
		d := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		payment.Status = DraftPaymentStatus
		payment.Disbursement = d
		CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &payment)

		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		// Defer a rollback in case anything fails.
		defer func() {
			err = dbTx.Rollback()
			require.Error(t, err, "not in transaction")
		}()

		actual, err := receiverModel.Get(ctx, dbTx, receiver.ID)
		require.NoError(t, err)
		expected := Receiver{
			ID:          receiver.ID,
			ExternalID:  receiver.ExternalID,
			Email:       receiver.Email,
			PhoneNumber: receiver.PhoneNumber,
			CreatedAt:   receiver.CreatedAt,
			UpdatedAt:   receiver.UpdatedAt,
			ReceiverStats: ReceiverStats{
				TotalPayments:      "1",
				SuccessfulPayments: "0",
				FailedPayments:     "0",
				CanceledPayments:   "0",
				RemainingPayments:  "1",
				RegisteredWallets:  "0",
				ReceivedAmounts: []Amount{
					{
						AssetCode:      "USDC",
						AssetIssuer:    "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
						ReceivedAmount: "0",
					},
				},
			},
		}
		assert.Equal(t, expected, *actual)

		err = dbTx.Commit()
		require.NoError(t, err)
	})

	t.Run("returns receiver with successful payment", func(t *testing.T) {
		DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)

		disbursement.Name = "disbursement 1"
		d := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		payment.Status = DraftPaymentStatus
		payment.Disbursement = d
		CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &payment)

		disbursement.Name = "disbursement 2"
		d = CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		payment.Status = SuccessPaymentStatus
		payment.Disbursement = d
		CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &payment)

		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		// Defer a rollback in case anything fails.
		defer func() {
			err = dbTx.Rollback()
			require.Error(t, err, "not in transaction")
		}()

		actual, err := receiverModel.Get(ctx, dbTx, receiver.ID)
		require.NoError(t, err)
		expected := Receiver{
			ID:          receiver.ID,
			ExternalID:  receiver.ExternalID,
			Email:       receiver.Email,
			PhoneNumber: receiver.PhoneNumber,
			CreatedAt:   receiver.CreatedAt,
			UpdatedAt:   receiver.UpdatedAt,
			ReceiverStats: ReceiverStats{
				TotalPayments:      "2",
				SuccessfulPayments: "1",
				FailedPayments:     "0",
				CanceledPayments:   "0",
				RemainingPayments:  "1",
				RegisteredWallets:  "0",
				ReceivedAmounts: []Amount{
					{
						AssetCode:      "USDC",
						AssetIssuer:    "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
						ReceivedAmount: "50.0000000",
					},
				},
			},
		}
		assert.Equal(t, expected, *actual)

		err = dbTx.Commit()
		require.NoError(t, err)
	})

	t.Run("returns receiver with multiple assets", func(t *testing.T) {
		DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)

		disbursement.Name = "disbursement 1"
		d := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		payment.Status = DraftPaymentStatus
		payment.Disbursement = d
		CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &payment)

		asset2 := CreateAssetFixture(t, ctx, dbConnectionPool, "EURT", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
		disbursement.Name = "disbursement 2"
		disbursement.Asset = asset2
		d = CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		payment.Status = SuccessPaymentStatus
		payment.Disbursement = d
		payment.Asset = *asset2
		CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &payment)

		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		// Defer a rollback in case anything fails.
		defer func() {
			err = dbTx.Rollback()
			require.Error(t, err, "not in transaction")
		}()

		actual, err := receiverModel.Get(ctx, dbTx, receiver.ID)
		require.NoError(t, err)
		expected := Receiver{
			ID:          receiver.ID,
			ExternalID:  receiver.ExternalID,
			Email:       receiver.Email,
			PhoneNumber: receiver.PhoneNumber,
			CreatedAt:   receiver.CreatedAt,
			UpdatedAt:   receiver.UpdatedAt,
			ReceiverStats: ReceiverStats{
				TotalPayments:      "2",
				SuccessfulPayments: "1",
				FailedPayments:     "0",
				CanceledPayments:   "0",
				RemainingPayments:  "1",
				RegisteredWallets:  "0",
				ReceivedAmounts: []Amount{
					{
						AssetCode:      "EURT",
						AssetIssuer:    "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
						ReceivedAmount: "50.0000000",
					},
					{
						AssetCode:      "USDC",
						AssetIssuer:    "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
						ReceivedAmount: "0",
					},
				},
			},
		}
		assert.Equal(t, expected, *actual)

		err = dbTx.Commit()
		require.NoError(t, err)
	})

	t.Run("returns receiver using db transaction", func(t *testing.T) {
		DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)

		disbursement.Name = "disbursement 1"
		disbursement.Asset = asset
		d := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		payment.Status = DraftPaymentStatus
		payment.Disbursement = d
		payment.Asset = *asset
		CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &payment)

		// Initializing a new Tx.
		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)

		// Defer a rollback in case anything fails.
		defer func() {
			err = dbTx.Rollback()
			require.Error(t, err, "not in transaction")
		}()

		actual, err := receiverModel.Get(ctx, dbTx, receiver.ID)
		require.NoError(t, err)
		expected := Receiver{
			ID:          receiver.ID,
			ExternalID:  receiver.ExternalID,
			Email:       receiver.Email,
			PhoneNumber: receiver.PhoneNumber,
			CreatedAt:   receiver.CreatedAt,
			UpdatedAt:   receiver.UpdatedAt,
			ReceiverStats: ReceiverStats{
				TotalPayments:      "1",
				SuccessfulPayments: "0",
				FailedPayments:     "0",
				CanceledPayments:   "0",
				RemainingPayments:  "1",
				RegisteredWallets:  "0",
				ReceivedAmounts: []Amount{
					{
						AssetCode:      "USDC",
						AssetIssuer:    "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
						ReceivedAmount: "0",
					},
				},
			},
		}

		assert.Equal(t, expected, *actual)

		// Commit the transaction.
		commitErr := dbTx.Commit()
		require.NoError(t, commitErr)
	})
}

func Test_ReceiversModelCount(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	receiverModel := ReceiverModel{}

	t.Run("returns 0 when no receivers exist", func(t *testing.T) {
		dbTx, innerErr := dbConnectionPool.BeginTxx(ctx, nil)
		// Defer a rollback in case anything fails.
		defer func() {
			innerErr = dbTx.Rollback()
			require.Error(t, innerErr, "not in transaction")
		}()

		count, innerErr := receiverModel.Count(ctx, dbTx, &QueryParams{})
		require.NoError(t, innerErr)
		assert.Equal(t, 0, count)

		innerErr = dbTx.Commit()
		require.NoError(t, innerErr)
	})

	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	receiver1 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiver2 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})

	CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, DraftReceiversWalletStatus)
	CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, RegisteredReceiversWalletStatus)

	t.Run("returns count of receivers", func(t *testing.T) {
		dbTx, innerErr := dbConnectionPool.BeginTxx(ctx, nil)
		// Defer a rollback in case anything fails.
		defer func() {
			innerErr = dbTx.Rollback()
			require.Error(t, innerErr, "not in transaction")
		}()

		count, innerErr := receiverModel.Count(ctx, dbTx, &QueryParams{})
		require.NoError(t, innerErr)
		assert.Equal(t, 2, count)

		innerErr = dbTx.Commit()
		require.NoError(t, innerErr)
	})

	t.Run("returns count of receivers with filter", func(t *testing.T) {
		dbTx, innerErr := dbConnectionPool.BeginTxx(ctx, nil)
		// Defer a rollback in case anything fails.
		defer func() {
			innerErr = dbTx.Rollback()
			require.Error(t, innerErr, "not in transaction")
		}()

		filters := map[FilterKey]interface{}{
			FilterKeyStatus: DraftReceiversWalletStatus,
		}
		count, innerErr := receiverModel.Count(ctx, dbTx, &QueryParams{Filters: filters})
		require.NoError(t, innerErr)
		assert.Equal(t, 1, count)

		innerErr = dbTx.Commit()
		require.NoError(t, innerErr)
	})

	t.Run("returns count of receivers with session", func(t *testing.T) {
		// Initializing a new Tx.
		dbTx, innerErr := dbConnectionPool.BeginTxx(ctx, nil)
		// Defer a rollback in case anything fails.
		defer func() {
			innerErr = dbTx.Rollback()
			require.Error(t, innerErr, "not in transaction")
		}()

		count, innerErr := receiverModel.Count(ctx, dbTx, &QueryParams{})
		require.NoError(t, innerErr)
		assert.Equal(t, 2, count)

		// Commit the transaction.
		innerErr = dbTx.Commit()
		require.NoError(t, innerErr)
	})
}

func Test_ReceiversModelGetAll(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	receiverModel := ReceiverModel{}

	t.Run("returns empty list when no receiver exist", func(t *testing.T) {
		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		// Defer a rollback in case anything fails.
		defer func() {
			err = dbTx.Rollback()
			require.Error(t, err, "not in transaction")
		}()

		receivers, err := receiverModel.GetAll(ctx, dbTx, &QueryParams{})
		require.NoError(t, err)
		assert.Equal(t, 0, len(receivers))

		err = dbTx.Commit()
		require.NoError(t, err)
	})

	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	date := time.Date(2023, 1, 10, 23, 40, 20, 1431, time.UTC)
	receiver1Email := "receiver1@mock.com"
	receiver1 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{
		Email:       &receiver1Email,
		PhoneNumber: "+99991111",
		ExternalID:  "external-id-1",
		CreatedAt:   &date,
		UpdatedAt:   &date,
	})

	date = time.Date(2023, 3, 10, 23, 40, 20, 1431, time.UTC)
	receiver2Email := "receiver2@mock.com"
	receiver2 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{
		Email:       &receiver2Email,
		PhoneNumber: "+99992222",
		ExternalID:  "external-id-2",
		CreatedAt:   &date,
		UpdatedAt:   &date,
	})

	CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, DraftReceiversWalletStatus)
	CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, RegisteredReceiversWalletStatus)

	t.Run("returns receiver successfully", func(t *testing.T) {
		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		// Defer a rollback in case anything fails.
		defer func() {
			err = dbTx.Rollback()
			require.Error(t, err, "not in transaction")
		}()

		actualReceivers, err := receiverModel.GetAll(ctx, dbTx, &QueryParams{SortBy: SortFieldCreatedAt, SortOrder: SortOrderASC})
		require.NoError(t, err)
		assert.Equal(t, 2, len(actualReceivers))

		ar, err := json.Marshal(actualReceivers)
		require.NoError(t, err)

		wantJson := fmt.Sprintf(`[
				{
					"id": %q,
					"email": "receiver1@mock.com",
					"external_id": "external-id-1",
					"phone_number": "+99991111",
					"created_at":   %q,
					"updated_at":   %q,
					"total_payments": "0",
					"successful_payments": "0",
					"failed_payments": "0",
					"canceled_payments": "0",
					"remaining_payments": "0",
					"registered_wallets":"0"
				},
				{
					"id": %q,
					"email": "receiver2@mock.com",
					"external_id": "external-id-2",
					"phone_number": "+99992222",
					"created_at":   %q,
					"updated_at":   %q,
					"total_payments": "0",
					"successful_payments": "0",
					"failed_payments": "0",
					"canceled_payments": "0",
					"remaining_payments": "0",
					"registered_wallets":"1"
				}
			]`, receiver1.ID, receiver1.CreatedAt.Format(time.RFC3339Nano), receiver1.UpdatedAt.Format(time.RFC3339Nano),
			receiver2.ID, receiver2.CreatedAt.Format(time.RFC3339Nano), receiver2.UpdatedAt.Format(time.RFC3339Nano))
		assert.JSONEq(t, wantJson, string(ar))

		err = dbTx.Commit()
		require.NoError(t, err)
	})

	t.Run("returns receivers successfully with limit", func(t *testing.T) {
		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		// Defer a rollback in case anything fails.
		defer func() {
			err = dbTx.Rollback()
			require.Error(t, err, "not in transaction")
		}()

		actualReceivers, err := receiverModel.GetAll(ctx, dbTx, &QueryParams{
			SortBy:    SortFieldCreatedAt,
			SortOrder: SortOrderASC,
			Page:      1,
			PageLimit: 1,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, len(actualReceivers))

		ar, err := json.Marshal(actualReceivers)
		require.NoError(t, err)

		wantJson := fmt.Sprintf(`[
			{
				"id": %q,
				"email": "receiver1@mock.com",
				"external_id": "external-id-1",
				"phone_number": "+99991111",
				"created_at":   %q,
				"updated_at":   %q,
				"total_payments": "0",
				"successful_payments": "0",
				"failed_payments": "0",
				"canceled_payments": "0",
				"remaining_payments": "0",
				"registered_wallets":"0"
			}
		]`, receiver1.ID, receiver1.CreatedAt.Format(time.RFC3339Nano), receiver1.UpdatedAt.Format(time.RFC3339Nano))
		assert.JSONEq(t, wantJson, string(ar))

		err = dbTx.Commit()
		require.NoError(t, err)
	})

	t.Run("returns receivers successfully with offset", func(t *testing.T) {
		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		// Defer a rollback in case anything fails.
		defer func() {
			err = dbTx.Rollback()
			require.Error(t, err, "not in transaction")
		}()

		actualReceivers, err := receiverModel.GetAll(ctx, dbTx, &QueryParams{
			SortBy:    SortFieldCreatedAt,
			SortOrder: SortOrderASC,
			Page:      2,
			PageLimit: 1,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, len(actualReceivers))

		ar, err := json.Marshal(actualReceivers)
		require.NoError(t, err)

		wantJson := fmt.Sprintf(`[
			{
				"id": %q,
				"email": "receiver2@mock.com",
				"external_id": "external-id-2",
				"phone_number": "+99992222",
				"created_at":   %q,
				"updated_at":   %q,
				"total_payments": "0",
				"successful_payments": "0",
				"failed_payments": "0",
				"canceled_payments": "0",
				"remaining_payments": "0",
				"registered_wallets":"1"
			}
		]`, receiver2.ID, receiver2.CreatedAt.Format(time.RFC3339Nano), receiver2.UpdatedAt.Format(time.RFC3339Nano))
		assert.JSONEq(t, wantJson, string(ar))

		err = dbTx.Commit()
		require.NoError(t, err)
	})

	t.Run("returns receivers successfully with status filter", func(t *testing.T) {
		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		// Defer a rollback in case anything fails.
		defer func() {
			if err != nil {
				err = dbTx.Rollback()
				require.NoError(t, err, "not in transaction")
			}
		}()

		filters := map[FilterKey]interface{}{
			FilterKeyStatus: DraftReceiversWalletStatus,
		}
		actualReceivers, err := receiverModel.GetAll(ctx, dbTx, &QueryParams{Filters: filters})
		require.NoError(t, err)
		assert.Equal(t, 1, len(actualReceivers))

		ar, err := json.Marshal(actualReceivers)
		require.NoError(t, err)

		wantJson := fmt.Sprintf(`[
			{
				"id": %q,
				"email": "receiver1@mock.com",
				"external_id": "external-id-1",
				"phone_number": "+99991111",
				"created_at":   %q,
				"updated_at":   %q,
				"total_payments": "0",
				"successful_payments": "0",
				"failed_payments": "0",
				"canceled_payments": "0",
				"remaining_payments": "0",
				"registered_wallets":"0"
			}
		]`, receiver1.ID, receiver1.CreatedAt.Format(time.RFC3339Nano), receiver1.UpdatedAt.Format(time.RFC3339Nano))
		assert.JSONEq(t, wantJson, string(ar))

		err = dbTx.Commit()
		require.NoError(t, err)
	})

	t.Run("returns receivers successfully with query filter email", func(t *testing.T) {
		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		// Defer a rollback in case anything fails.
		defer func() {
			err = dbTx.Rollback()
			require.Error(t, err, "not in transaction")
		}()

		actualReceivers, err := receiverModel.GetAll(ctx, dbTx, &QueryParams{Query: receiver1Email})
		require.NoError(t, err)
		assert.Equal(t, 1, len(actualReceivers))

		ar, err := json.Marshal(actualReceivers)
		require.NoError(t, err)

		wantJson := fmt.Sprintf(`[
			{
				"id": %q,
				"email": "receiver1@mock.com",
				"external_id": "external-id-1",
				"phone_number": "+99991111",
				"created_at":   %q,
				"updated_at":   %q,
				"total_payments": "0",
				"successful_payments": "0",
				"failed_payments": "0",
				"canceled_payments": "0",
				"remaining_payments": "0",
				"registered_wallets":"0"	
			}
		]`, receiver1.ID, receiver1.CreatedAt.Format(time.RFC3339Nano), receiver1.UpdatedAt.Format(time.RFC3339Nano))
		assert.JSONEq(t, wantJson, string(ar))

		err = dbTx.Commit()
		require.NoError(t, err)
	})

	t.Run("returns receivers successfully with query filter phone number", func(t *testing.T) {
		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		// Defer a rollback in case anything fails.
		defer func() {
			err = dbTx.Rollback()
			require.Error(t, err, "not in transaction")
		}()

		actualReceivers, err := receiverModel.GetAll(ctx, dbTx, &QueryParams{Query: "+99992222"})
		require.NoError(t, err)
		assert.Equal(t, 1, len(actualReceivers))

		ar, err := json.Marshal(actualReceivers)
		require.NoError(t, err)

		wantJson := fmt.Sprintf(`[
			{
				"id": %q,
				"email": "receiver2@mock.com",
				"external_id": "external-id-2",
				"phone_number": "+99992222",
				"created_at":   %q,
				"updated_at":   %q,
				"total_payments": "0",
				"successful_payments": "0",
				"failed_payments": "0",
				"canceled_payments": "0",
				"remaining_payments": "0",
				"registered_wallets":"1"
			}
		]`, receiver2.ID, receiver2.CreatedAt.Format(time.RFC3339Nano), receiver2.UpdatedAt.Format(time.RFC3339Nano))
		assert.JSONEq(t, wantJson, string(ar))

		err = dbTx.Commit()
		require.NoError(t, err)
	})

	t.Run("returns receivers successfully with date filter", func(t *testing.T) {
		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		// Defer a rollback in case anything fails.
		defer func() {
			err = dbTx.Rollback()
			require.Error(t, err, "not in transaction")
		}()

		filters := map[FilterKey]interface{}{
			FilterKeyCreatedAtAfter:  "2023-01-01",
			FilterKeyCreatedAtBefore: "2023-03-01",
		}
		actualReceivers, err := receiverModel.GetAll(ctx, dbTx, &QueryParams{Filters: filters})
		require.NoError(t, err)
		assert.Equal(t, 1, len(actualReceivers))

		ar, err := json.Marshal(actualReceivers)
		require.NoError(t, err)

		wantJson := fmt.Sprintf(`[
			{
				"id": %q,
				"email": "receiver1@mock.com",
				"external_id": "external-id-1",		
				"phone_number": "+99991111",
				"created_at":   %q,
				"updated_at":   %q,
				"total_payments": "0",
				"successful_payments": "0",
				"failed_payments": "0",
				"canceled_payments": "0",
				"remaining_payments": "0",
				"registered_wallets":"0"
			}
		]`, receiver1.ID, receiver1.CreatedAt.Format(time.RFC3339Nano), receiver1.UpdatedAt.Format(time.RFC3339Nano))
		assert.JSONEq(t, wantJson, string(ar))

		err = dbTx.Commit()
		require.NoError(t, err)
	})

	t.Run("returns receiver successfully with session", func(t *testing.T) {
		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		// Defer a rollback in case anything fails.
		defer func() {
			err = dbTx.Rollback()
			require.Error(t, err, "not in transaction")
		}()

		actualReceivers, err := receiverModel.GetAll(ctx, dbTx, &QueryParams{SortBy: SortFieldCreatedAt, SortOrder: SortOrderASC})
		require.NoError(t, err)
		assert.Equal(t, 2, len(actualReceivers))

		ar, err := json.Marshal(actualReceivers)
		require.NoError(t, err)

		wantJson := fmt.Sprintf(`[
				{
					"id": %q,
					"email": "receiver1@mock.com",
					"external_id": "external-id-1",	
					"phone_number": "+99991111",
					"created_at":   %q,
					"updated_at":   %q,
					"total_payments": "0",
					"successful_payments": "0",
					"failed_payments": "0",
					"canceled_payments": "0",
					"remaining_payments": "0",
					"registered_wallets":"0"
				},
				{
					"id": %q,
					"email": "receiver2@mock.com",
					"external_id": "external-id-2",
					"phone_number": "+99992222",
					"created_at":   %q,
					"updated_at":   %q,
					"total_payments": "0",
					"successful_payments": "0",
					"failed_payments": "0",
					"canceled_payments": "0",
					"remaining_payments": "0",
					"registered_wallets":"1"
				}
			]`, receiver1.ID, receiver1.CreatedAt.Format(time.RFC3339Nano), receiver1.UpdatedAt.Format(time.RFC3339Nano),
			receiver2.ID, receiver2.CreatedAt.Format(time.RFC3339Nano), receiver2.UpdatedAt.Format(time.RFC3339Nano))

		assert.JSONEq(t, wantJson, string(ar))

		// Commit the transaction.
		commitErr := dbTx.Commit()
		require.NoError(t, commitErr)
	})
}

func Test_ReceiversModel_GetAll_makeSureReceiversWithMultipleWalletsWillReturnASingleResult(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	receiverModel := ReceiverModel{}

	wallet1 := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")
	wallet2 := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet2", "https://www.wallet2.com", "www.wallet2.com", "wallet2://")

	receiver1Email := "receiver1@mock.com"
	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{
		Email:       &receiver1Email,
		PhoneNumber: "+99991111",
		ExternalID:  "external-id-1",
	})

	CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet1.ID, ReadyReceiversWalletStatus)
	CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet2.ID, RegisteredReceiversWalletStatus)

	receivers, err := receiverModel.GetAll(ctx, dbConnectionPool, &QueryParams{})
	require.NoError(t, err)

	assert.Len(t, receivers, 1)
}

func Test_ReceiversModel_ParseReceiverIDs(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	receiverModel := ReceiverModel{}

	receiver1 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiver2 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})

	dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
	require.NoError(t, err)
	// Defer a rollback in case anything fails.
	defer func() {
		err = dbTx.Rollback()
		require.Error(t, err, "not in transaction")
	}()
	receivers, err := receiverModel.GetAll(ctx, dbTx, &QueryParams{SortBy: SortFieldCreatedAt, SortOrder: SortOrderASC})
	require.NoError(t, err)

	receiverIds := receiverModel.ParseReceiverIDs(receivers)
	expectedIds := ReceiverIDs{receiver1.ID, receiver2.ID}
	assert.Equal(t, expectedIds, receiverIds)

	err = dbTx.Commit()
	require.NoError(t, err)
}

func Test_DeleteByPhoneNumber(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := NewModels(dbConnectionPool)
	require.NoError(t, err)

	// 0. returns ErrNotFound for users that don't exist:
	t.Run("User does not exist", func(t *testing.T) {
		err = models.Receiver.DeleteByPhoneNumber(ctx, dbConnectionPool, "+14152222222")
		require.ErrorIs(t, err, ErrRecordNotFound)
	})

	// 1. Create country, asset, and wallet (won't be deleted)
	country := CreateCountryFixture(t, ctx, dbConnectionPool, "ATL", "Atlantis")
	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "FOO1", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "walletA", "https://www.a.com", "www.a.com", "a://")

	// 2. Create receiverX (that will be deleted) and all receiverX dependent resources that will also be deleted:
	receiverX := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverWalletX := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverX.ID, wallet.ID, DraftReceiversWalletStatus)
	_ = CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, ReceiverVerificationInsert{
		ReceiverID:        receiverX.ID,
		VerificationField: VerificationFieldDateOfBirth,
		VerificationValue: "1990-01-01",
	})
	messageX := CreateMessageFixture(t, ctx, dbConnectionPool, &Message{
		Type:             message.MessengerTypeTwilioSMS,
		AssetID:          nil,
		ReceiverID:       receiverX.ID,
		WalletID:         wallet.ID,
		ReceiverWalletID: &receiverWalletX.ID,
		Status:           SuccessMessageStatus,
		CreatedAt:        time.Date(2023, 1, 10, 23, 40, 20, 1000, time.UTC),
	})
	disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Country: country,
		Wallet:  wallet,
		Status:  ReadyDisbursementStatus,
		Asset:   asset,
	})
	paymentX1 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: receiverWalletX,
		Disbursement:   disbursement1,
		Asset:          *asset,
		Status:         ReadyPaymentStatus,
		Amount:         "1",
	})

	// 3. Create receiverY (that will not be deleted) and all receiverY dependent resources that will not be deleted:
	receiverY := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverWalletY := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverY.ID, wallet.ID, DraftReceiversWalletStatus)
	_ = CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, ReceiverVerificationInsert{
		ReceiverID:        receiverY.ID,
		VerificationField: VerificationFieldDateOfBirth,
		VerificationValue: "1990-01-01",
	})
	messageY := CreateMessageFixture(t, ctx, dbConnectionPool, &Message{
		Type:             message.MessengerTypeTwilioSMS,
		AssetID:          nil,
		ReceiverID:       receiverY.ID,
		WalletID:         wallet.ID,
		ReceiverWalletID: &receiverWalletY.ID,
		Status:           SuccessMessageStatus,
		CreatedAt:        time.Date(2023, 1, 10, 23, 40, 20, 1000, time.UTC),
	})
	disbursement2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Country: country,
		Wallet:  wallet,
		Status:  ReadyDisbursementStatus,
		Asset:   asset,
	})
	paymentY2 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: receiverWalletY,
		Disbursement:   disbursement2,
		Asset:          *asset,
		Status:         ReadyPaymentStatus,
		Amount:         "1",
	})

	paymentX2 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: receiverWalletX,
		Disbursement:   disbursement2,
		Asset:          *asset,
		Status:         ReadyPaymentStatus,
		Amount:         "1",
	}) // This payment will be deleted along with the remaining receiverX-related data

	// 4. Delete receiverX
	err = models.Receiver.DeleteByPhoneNumber(ctx, dbConnectionPool, receiverX.PhoneNumber)
	require.NoError(t, err)

	type testCase struct {
		name       string
		query      string
		args       []interface{}
		wantExists bool
	}

	// 5. Prepare assertions to make sure `DeleteByPhoneNumber` DID DELETE receiverX-related data:
	didDeleteTestCases := []testCase{
		{
			name:       "DID DELETE: receiverX",
			query:      "SELECT EXISTS(SELECT 1 FROM receivers WHERE id = $1)",
			args:       []interface{}{receiverX.ID},
			wantExists: false,
		},
		{
			name:       "DID DELETE: receiverWalletX",
			query:      "SELECT EXISTS(SELECT 1 FROM receiver_wallets WHERE id = $1)",
			args:       []interface{}{receiverWalletX.ID},
			wantExists: false,
		},
		{
			name:       "DID DELETE: receiverVerificationX",
			query:      "SELECT EXISTS(SELECT 1 FROM receiver_verifications WHERE receiver_id = $1)",
			args:       []interface{}{receiverX.ID},
			wantExists: false,
		},
		{
			name:       "DID DELETE: messageX",
			query:      "SELECT EXISTS(SELECT 1 FROM messages WHERE id = $1)",
			args:       []interface{}{messageX.ID},
			wantExists: false,
		},
		{
			name:       "DID DELETE: paymentX",
			query:      "SELECT EXISTS(SELECT 1 FROM payments WHERE id = ANY($1))",
			args:       []interface{}{pq.Array([]string{paymentX1.ID, paymentX2.ID})},
			wantExists: false,
		},
		{
			name:       "DID DELETE: disbursement1",
			query:      "SELECT EXISTS(SELECT 1 FROM disbursements WHERE id = $1)",
			args:       []interface{}{disbursement1.ID},
			wantExists: false,
		},
	}

	// 6. Prepare assertions to make sure `DeleteByPhoneNumber` DID NOT DELETE receiverY-related data:
	didNotDeleteTestCases := []testCase{
		{
			name:       "DID NOT DELETE: receiverY",
			query:      "SELECT EXISTS(SELECT 1 FROM receivers WHERE id = $1)",
			args:       []interface{}{receiverY.ID},
			wantExists: true,
		},
		{
			name:       "DID NOT DELETE: receiverWalletY",
			query:      "SELECT EXISTS(SELECT 1 FROM receiver_wallets WHERE id = $1)",
			args:       []interface{}{receiverWalletY.ID},
			wantExists: true,
		},
		{
			name:       "DID NOT DELETE: receiverVerificationY",
			query:      "SELECT EXISTS(SELECT 1 FROM receiver_verifications WHERE receiver_id = $1)",
			args:       []interface{}{receiverY.ID},
			wantExists: true,
		},
		{
			name:       "DID NOT DELETE: messageY",
			query:      "SELECT EXISTS(SELECT 1 FROM messages WHERE id = $1)",
			args:       []interface{}{messageY.ID},
			wantExists: true,
		},
		{
			name:       "DID NOT DELETE: paymentY2",
			query:      "SELECT EXISTS(SELECT 1 FROM payments WHERE id = $1)",
			args:       []interface{}{paymentY2.ID},
			wantExists: true,
		},
		{
			name:       "DID NOT DELETE: paymentX2",
			query:      "SELECT EXISTS(SELECT 1 FROM disbursements WHERE id = $1)",
			args:       []interface{}{disbursement2.ID},
			wantExists: true,
		},
	}

	// 7. Run assertions
	testCases := append(didDeleteTestCases, didNotDeleteTestCases...)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var exists bool
			err = dbConnectionPool.QueryRowxContext(ctx, tc.query, tc.args...).Scan(&exists)
			require.NoError(t, err)
			require.Equal(t, tc.wantExists, exists)
		})
	}
}

func Test_ReceiversModel_Update(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	receiverModel := ReceiverModel{}

	email, externalID := "receiver@email.com", "externalID"
	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{Email: &email, ExternalID: externalID})

	resetReceiver := func(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, receiverID string) {
		q := `
			UPDATE receivers SET email = $1, external_id = $2 WHERE id = $3
		`
		_, err = sqlExec.ExecContext(ctx, q, email, externalID, receiverID)
		require.NoError(t, err)
	}

	t.Run("returns error when no value is provided", func(t *testing.T) {
		resetReceiver(t, ctx, dbConnectionPool, receiver.ID)

		err = receiverModel.Update(ctx, dbConnectionPool, receiver.ID, ReceiverUpdate{
			Email:      "",
			ExternalId: "",
		})
		assert.EqualError(t, err, "provide at least one of these values: Email or ExternalID")
	})

	t.Run("returns error when email is invalid", func(t *testing.T) {
		resetReceiver(t, ctx, dbConnectionPool, receiver.ID)

		err = receiverModel.Update(ctx, dbConnectionPool, receiver.ID, ReceiverUpdate{
			Email:      "invalid",
			ExternalId: "",
		})
		assert.EqualError(t, err, `error validating email: the provided email is not valid`)
	})

	t.Run("updates email name successfully", func(t *testing.T) {
		resetReceiver(t, ctx, dbConnectionPool, receiver.ID)

		receiver, err = receiverModel.Get(ctx, dbConnectionPool, receiver.ID)
		require.NoError(t, err)
		assert.Equal(t, email, *receiver.Email)
		assert.Equal(t, externalID, receiver.ExternalID)

		err = receiverModel.Update(ctx, dbConnectionPool, receiver.ID, ReceiverUpdate{
			Email:      "updated_email@email.com",
			ExternalId: "",
		})
		require.NoError(t, err)

		receiver, err = receiverModel.Get(ctx, dbConnectionPool, receiver.ID)
		require.NoError(t, err)
		assert.NotEqual(t, email, *receiver.Email)
		assert.Equal(t, "updated_email@email.com", *receiver.Email)
		assert.Equal(t, externalID, receiver.ExternalID)
	})

	t.Run("updates external ID successfully", func(t *testing.T) {
		resetReceiver(t, ctx, dbConnectionPool, receiver.ID)

		receiver, err = receiverModel.Get(ctx, dbConnectionPool, receiver.ID)
		require.NoError(t, err)
		assert.Equal(t, email, *receiver.Email)
		assert.Equal(t, externalID, receiver.ExternalID)

		err := receiverModel.Update(ctx, dbConnectionPool, receiver.ID, ReceiverUpdate{
			Email:      "updated_email@email.com",
			ExternalId: "newExternalID",
		})
		require.NoError(t, err)

		receiver, err = receiverModel.Get(ctx, dbConnectionPool, receiver.ID)
		require.NoError(t, err)
		assert.NotEqual(t, email, *receiver.Email)
		assert.Equal(t, "updated_email@email.com", *receiver.Email)
		assert.NotEqual(t, externalID, receiver.ExternalID)
		assert.Equal(t, "newExternalID", receiver.ExternalID)
	})
}
