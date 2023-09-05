package services

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	txSubStore "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

func Test_PaymentToSubmitterService_SendBatchPayments(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	tssModel := txSubStore.NewTransactionModel(models.DBConnectionPool)

	service := NewPaymentToSubmitterService(models)
	ctx := context.Background()

	// create fixtures
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool,
		"My Wallet",
		"https://www.wallet.com",
		"www.wallet.com",
		"wallet1://")
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool,
		"USDC",
		"GDUCE34WW5Z34GMCEPURYANUCUP47J6NORJLKC6GJNMDLN4ZI4PMI2MG")
	country := data.CreateCountryFixture(t, ctx, dbConnectionPool,
		"FRA",
		"France")

	// create disbursements
	startedDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:    "ready disbursement",
		Status:  data.StartedDisbursementStatus,
		Asset:   asset,
		Wallet:  wallet,
		Country: country,
	})

	// create disbursement receivers
	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver3 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver4 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})

	rw1 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, data.RegisteredReceiversWalletStatus)
	rw2 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, data.RegisteredReceiversWalletStatus)
	rw3 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver3.ID, wallet.ID, data.RegisteredReceiversWalletStatus)
	rwReady := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver4.ID, wallet.ID, data.ReadyReceiversWalletStatus)

	payment1 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rw1,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "100",
		Status:         data.ReadyPaymentStatus,
	})
	payment2 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rw2,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "200",
		Status:         data.ReadyPaymentStatus,
	})
	payment3 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rw3,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "300",
		Status:         data.ReadyPaymentStatus,
	})
	payment4 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwReady,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "400",
		Status:         data.ReadyPaymentStatus,
	})

	t.Run("send payments", func(t *testing.T) {
		err = service.SendBatchPayments(ctx, 5)
		require.NoError(t, err)

		// payments that can be sent
		var payment *data.Payment
		for _, p := range []*data.Payment{payment1, payment2, payment3} {
			payment, err = models.Payment.Get(ctx, p.ID, dbConnectionPool)
			require.NoError(t, err)
			require.Equal(t, data.PendingPaymentStatus, payment.Status)
		}

		// payments that can't be sent (rw status is not REGISTERED)
		payment, err = models.Payment.Get(ctx, payment4.ID, dbConnectionPool)
		require.NoError(t, err)
		require.Equal(t, data.ReadyPaymentStatus, payment.Status)

		// validate transactions
		var transactions []*txSubStore.Transaction
		transactions, err = tssModel.GetAllByPaymentIDs(ctx, []string{payment1.ID, payment2.ID, payment3.ID, payment4.ID})
		require.NoError(t, err)
		require.Len(t, transactions, 3)

		expectedPayments := map[string]*data.Payment{
			payment1.ID: payment1,
			payment2.ID: payment2,
			payment3.ID: payment3,
		}

		for _, tx := range transactions {
			require.Equal(t, txSubStore.TransactionStatusPending, tx.Status)
			require.Equal(t, expectedPayments[tx.ExternalID].Asset.Code, tx.AssetCode)
			require.Equal(t, expectedPayments[tx.ExternalID].Asset.Issuer, tx.AssetIssuer)
			require.Equal(t, expectedPayments[tx.ExternalID].Amount, strconv.FormatFloat(tx.Amount, 'f', 7, 32))
			require.Equal(t, expectedPayments[tx.ExternalID].ReceiverWallet.StellarAddress, tx.Destination)
			require.Equal(t, expectedPayments[tx.ExternalID].ID, tx.ExternalID)
		}
	})

	t.Run("send payments with native asset", func(t *testing.T) {
		nativeAsset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "XLM", "")

		startedDisbursementNA := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name:    "started disbursement Native Asset",
			Status:  data.StartedDisbursementStatus,
			Asset:   nativeAsset,
			Wallet:  wallet,
			Country: country,
		})

		paymentNA1 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rw1,
			Disbursement:   startedDisbursementNA,
			Asset:          *nativeAsset,
			Amount:         "100",
			Status:         data.ReadyPaymentStatus,
		})

		paymentNA2 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rw2,
			Disbursement:   startedDisbursementNA,
			Asset:          *nativeAsset,
			Amount:         "100",
			Status:         data.ReadyPaymentStatus,
		})

		err = service.SendBatchPayments(ctx, 5)
		require.NoError(t, err)

		for _, p := range []*data.Payment{paymentNA1, paymentNA2} {
			payment, err := models.Payment.Get(ctx, p.ID, dbConnectionPool)
			require.NoError(t, err)
			assert.Equal(t, data.PendingPaymentStatus, payment.Status)
		}

		transactions, err := tssModel.GetAllByPaymentIDs(ctx, []string{paymentNA1.ID, paymentNA2.ID})
		require.NoError(t, err)
		require.Len(t, transactions, 2)

		expectedPayments := map[string]*data.Payment{
			paymentNA1.ID: paymentNA1,
			paymentNA2.ID: paymentNA2,
		}

		for _, tx := range transactions {
			assert.Equal(t, txSubStore.TransactionStatusPending, tx.Status)
			assert.Equal(t, expectedPayments[tx.ExternalID].Asset.Code, tx.AssetCode)
			assert.Empty(t, tx.AssetIssuer)
			assert.Equal(t, expectedPayments[tx.ExternalID].Amount, strconv.FormatFloat(tx.Amount, 'f', 7, 32))
			assert.Equal(t, expectedPayments[tx.ExternalID].ReceiverWallet.StellarAddress, tx.Destination)
			assert.Equal(t, expectedPayments[tx.ExternalID].ID, tx.ExternalID)
		}
	})
}

func Test_PaymentToSubmitterService_ValidatePaymentReadyForSending(t *testing.T) {
	testCases := []struct {
		name          string
		payment       *data.Payment
		expectedError string
	}{
		{
			name: "valid payment",
			payment: &data.Payment{
				Status: data.ReadyPaymentStatus,
				ReceiverWallet: &data.ReceiverWallet{
					Status:         data.RegisteredReceiversWalletStatus,
					StellarAddress: "destination_1",
				},
				Disbursement: &data.Disbursement{
					Status: data.StartedDisbursementStatus,
				},
				ID: "1",
				Asset: data.Asset{
					Code:   "USDC",
					Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN",
				},
				Amount: "100.0",
			},
			expectedError: "",
		},
		{
			name: "invalid payment status",
			payment: &data.Payment{
				ID:     "123",
				Status: data.PendingPaymentStatus,
			},
			expectedError: "payment 123 is not in READY state",
		},
		{
			name: "invalid receiver wallet status",
			payment: &data.Payment{
				ID:     "123",
				Status: data.ReadyPaymentStatus,
				ReceiverWallet: &data.ReceiverWallet{
					ID:     "321",
					Status: data.ReadyReceiversWalletStatus,
				},
			},
			expectedError: "receiver wallet 321 for payment 123 is not in REGISTERED state",
		},
		{
			name: "invalid disbursement status",
			payment: &data.Payment{
				ID:     "123",
				Status: data.ReadyPaymentStatus,
				ReceiverWallet: &data.ReceiverWallet{
					Status: data.RegisteredReceiversWalletStatus,
				},
				Disbursement: &data.Disbursement{
					ID:     "321",
					Status: data.ReadyDisbursementStatus,
				},
			},
			expectedError: "disbursement 321 for payment 123 is not in STARTED state",
		},
		{
			name: "payment ID is empty",
			payment: &data.Payment{
				Status: data.ReadyPaymentStatus,
				ReceiverWallet: &data.ReceiverWallet{
					Status: data.RegisteredReceiversWalletStatus,
				},
				Disbursement: &data.Disbursement{
					Status: data.StartedDisbursementStatus,
				},
			},
			expectedError: "payment ID is empty for Payment",
		},
		{
			name: "payment asset code is empty",
			payment: &data.Payment{
				ID:     "123",
				Status: data.ReadyPaymentStatus,
				ReceiverWallet: &data.ReceiverWallet{
					Status: data.RegisteredReceiversWalletStatus,
				},
				Disbursement: &data.Disbursement{
					Status: data.StartedDisbursementStatus,
				},
			},
			expectedError: "payment asset code is empty for payment 123",
		},
		{
			name: "payment asset issuer is empty",
			payment: &data.Payment{
				ID:     "123",
				Status: data.ReadyPaymentStatus,
				ReceiverWallet: &data.ReceiverWallet{
					Status: data.RegisteredReceiversWalletStatus,
				},
				Disbursement: &data.Disbursement{
					Status: data.StartedDisbursementStatus,
				},
				Asset: data.Asset{
					Code: "USDC",
				},
			},
			expectedError: "payment asset issuer is empty for payment 123",
		},
		{
			name: "payment amount is invalid",
			payment: &data.Payment{
				ID:     "123",
				Status: data.ReadyPaymentStatus,
				ReceiverWallet: &data.ReceiverWallet{
					Status: data.RegisteredReceiversWalletStatus,
				},
				Disbursement: &data.Disbursement{
					Status: data.StartedDisbursementStatus,
				},
				Asset: data.Asset{
					Code:   "USDC",
					Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN",
				},
			},
			expectedError: "payment amount is invalid for payment 123",
		},
		{
			name: "payment receiver wallet stellar address is empty",
			payment: &data.Payment{
				ID:     "123",
				Status: data.ReadyPaymentStatus,
				ReceiverWallet: &data.ReceiverWallet{
					Status: data.RegisteredReceiversWalletStatus,
				},
				Disbursement: &data.Disbursement{
					Status: data.StartedDisbursementStatus,
				},
				Asset: data.Asset{
					Code:   "USDC",
					Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN",
				},
				Amount: "100.0",
			},
			expectedError: "payment receiver wallet stellar address is empty for payment 123",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePaymentReadyForSending(tc.payment)
			if tc.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.expectedError)
			}
		})
	}
}

func Test_PaymentToSubmitterService_RetryPayment(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	tssModel := txSubStore.NewTransactionModel(models.DBConnectionPool)

	service := NewPaymentToSubmitterService(models)

	// clean test db
	data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllCountryFixtures(t, ctx, dbConnectionPool)

	// create fixtures
	country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "BRA", "Brazil")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GDUCE34WW5Z34GMCEPURYANUCUP47J6NORJLKC6GJNMDLN4ZI4PMI2MG")

	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:    "started disbursement",
		Status:  data.StartedDisbursementStatus,
		Asset:   asset,
		Wallet:  wallet,
		Country: country,
	})

	payment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		Amount:         "100",
		Status:         data.ReadyPaymentStatus,
		Disbursement:   disbursement,
		ReceiverWallet: receiverWallet,
		Asset:          *asset,
	})

	err = service.SendBatchPayments(ctx, 1)
	require.NoError(t, err)

	paymentDB, err := models.Payment.Get(ctx, payment.ID, dbConnectionPool)
	require.NoError(t, err)
	assert.Equal(t, data.PendingPaymentStatus, paymentDB.Status)

	transactions, err := tssModel.GetAllByPaymentIDs(ctx, []string{payment.ID})
	require.NoError(t, err)
	assert.Len(t, transactions, 1)

	transaction := transactions[0]
	assert.Equal(t, payment.ID, transaction.ExternalID)
	assert.Equal(t, txSubStore.TransactionStatusPending, transaction.Status)

	// Marking the transaction as failed
	transaction.Status = txSubStore.TransactionStatusProcessing
	_, err = tssModel.UpdateStatusToError(ctx, *transaction, "Failing Test")
	require.NoError(t, err)

	transactions, err = tssModel.GetAllByPaymentIDs(ctx, []string{payment.ID})
	require.NoError(t, err)
	assert.Len(t, transactions, 1)

	transaction = transactions[0]
	assert.Equal(t, payment.ID, transaction.ExternalID)
	assert.Equal(t, txSubStore.TransactionStatusError, transaction.Status)

	err = models.Payment.Update(ctx, dbConnectionPool, paymentDB, &data.PaymentUpdate{
		Status:               data.FailedPaymentStatus,
		StellarTransactionID: "stellar-transaction-id-2",
	})
	require.NoError(t, err)
	paymentDB, err = models.Payment.Get(ctx, paymentDB.ID, dbConnectionPool)
	require.NoError(t, err)
	assert.Equal(t, data.FailedPaymentStatus, paymentDB.Status)

	err = models.Payment.RetryFailedPayments(ctx, "email@test.com", paymentDB.ID)
	require.NoError(t, err)
	paymentDB, err = models.Payment.Get(ctx, paymentDB.ID, dbConnectionPool)
	require.NoError(t, err)
	assert.Equal(t, data.ReadyPaymentStatus, paymentDB.Status)

	// insert a new transaction for the same payment
	err = service.SendBatchPayments(ctx, 1)
	require.NoError(t, err)

	paymentDB, err = models.Payment.Get(ctx, payment.ID, dbConnectionPool)
	require.NoError(t, err)
	assert.Equal(t, data.PendingPaymentStatus, paymentDB.Status)

	transactions, err = tssModel.GetAllByPaymentIDs(ctx, []string{payment.ID})
	require.NoError(t, err)
	assert.Len(t, transactions, 2)

	transaction1 := transactions[0]
	transaction2 := transactions[1]
	assert.Equal(t, txSubStore.TransactionStatusError, transaction1.Status)
	assert.Equal(t, txSubStore.TransactionStatusPending, transaction2.Status)
}
