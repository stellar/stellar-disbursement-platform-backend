package integrationtests

import (
	"context"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/require"
)

func Test_validationAfterProcessDisbursement(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("disbursement not found", func(t *testing.T) {
		err = validateExpectationsAfterProcessDisbursement(ctx, "invalid_id", models, dbConnectionPool)
		require.EqualError(t, err, "error getting disbursement: record not found")
	})

	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	t.Run("invalid disbursement status", func(t *testing.T) {
		invalidDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name:    "Invalid Disbursement",
			Status:  data.CompletedDisbursementStatus,
			Asset:   asset,
			Wallet:  wallet,
			Country: country,
		})

		err = validateExpectationsAfterProcessDisbursement(ctx, invalidDisbursement.ID, models, dbConnectionPool)
		require.EqualError(t, err, "invalid status for disbursement after process disbursement")
	})

	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:    "disbursement 1",
		Status:  data.ReadyDisbursementStatus,
		Asset:   asset,
		Wallet:  wallet,
		Country: country,
	})

	t.Run("disbursement receivers not found", func(t *testing.T) {
		err = validateExpectationsAfterProcessDisbursement(ctx, disbursement.ID, models, dbConnectionPool)
		require.EqualError(t, err, "error getting receivers from disbursement: receivers not found")
	})

	t.Run("invalid receiver wallet status", func(t *testing.T) {
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.FlaggedReceiversWalletStatus)

		data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:         "50",
			Status:         data.DraftPaymentStatus,
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: receiverWallet,
		})

		err = validateExpectationsAfterProcessDisbursement(ctx, disbursement.ID, models, dbConnectionPool)
		require.EqualError(t, err, "invalid status for receiver_wallet after process disbursement")
	})

	t.Run("invalid payment status", func(t *testing.T) {
		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.DraftReceiversWalletStatus)

		data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:         "50",
			Status:         data.FailedPaymentStatus,
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: receiverWallet,
		})

		err = validateExpectationsAfterProcessDisbursement(ctx, disbursement.ID, models, dbConnectionPool)
		require.EqualError(t, err, "invalid status for payment after process disbursement")
	})

	t.Run("successfull validation", func(t *testing.T) {
		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.DraftReceiversWalletStatus)

		data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:         "50",
			Status:         data.DraftPaymentStatus,
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: receiverWallet,
		})

		err = validateExpectationsAfterProcessDisbursement(ctx, disbursement.ID, models, dbConnectionPool)
		require.NoError(t, err)
	})
}

func Test_validationAfterStartDisbursement(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("disbursement not found", func(t *testing.T) {
		err = validateExpectationsAfterStartDisbursement(ctx, "invalid_id", models, dbConnectionPool)
		require.EqualError(t, err, "error getting disbursement: record not found")
	})

	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	t.Run("invalid disbursement status", func(t *testing.T) {
		invalidDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name:    "Invalid Disbursement",
			Status:  data.CompletedDisbursementStatus,
			Asset:   asset,
			Wallet:  wallet,
			Country: country,
		})

		err = validateExpectationsAfterStartDisbursement(ctx, invalidDisbursement.ID, models, dbConnectionPool)
		require.EqualError(t, err, "invalid status for disbursement after start disbursement")
	})

	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:    "disbursement 1",
		Status:  data.StartedDisbursementStatus,
		Asset:   asset,
		Wallet:  wallet,
		Country: country,
	})

	t.Run("disbursement receivers not found", func(t *testing.T) {
		err = validateExpectationsAfterStartDisbursement(ctx, disbursement.ID, models, dbConnectionPool)
		require.EqualError(t, err, "error getting receivers from disbursement: receivers not found")
	})

	t.Run("invalid receiver wallet status", func(t *testing.T) {
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.FlaggedReceiversWalletStatus)

		data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:         "50",
			Status:         data.DraftPaymentStatus,
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: receiverWallet,
		})

		err = validateExpectationsAfterStartDisbursement(ctx, disbursement.ID, models, dbConnectionPool)
		require.EqualError(t, err, "invalid status for receiver_wallet after start disbursement")
	})

	t.Run("invalid payment status", func(t *testing.T) {
		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

		data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:         "50",
			Status:         data.FailedPaymentStatus,
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: receiverWallet,
		})

		err = validateExpectationsAfterStartDisbursement(ctx, disbursement.ID, models, dbConnectionPool)
		require.EqualError(t, err, "invalid status for payment after start disbursement")
	})

	t.Run("successfull validation", func(t *testing.T) {
		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

		data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:         "50",
			Status:         data.ReadyPaymentStatus,
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: receiverWallet,
		})

		err = validateExpectationsAfterStartDisbursement(ctx, disbursement.ID, models, dbConnectionPool)
		require.NoError(t, err)
	})
}

func Test_validationAfterReceiverRegistration(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("receiver wallet not found", func(t *testing.T) {
		err = validateExpectationsAfterReceiverRegistration(ctx, models, "invalid_stellar_account", "invalid_stellar_memo", "invalid_client_domain")
		require.EqualError(t, err, "error getting receiver wallet with stellar account: no receiver wallet could be found in GetByStellarAccountAndMemo: record not found")
	})

	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	t.Run("invalid receiver wallet status validation", func(t *testing.T) {
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

		err = validateExpectationsAfterReceiverRegistration(ctx, models, receiverWallet.StellarAddress, receiverWallet.StellarMemo, wallet.SEP10ClientDomain)
		require.EqualError(t, err, "invalid status for receiver_wallet after receiver registration")
	})

	t.Run("successfull validation", func(t *testing.T) {
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

		err = validateExpectationsAfterReceiverRegistration(ctx, models, receiverWallet.StellarAddress, receiverWallet.StellarMemo, wallet.SEP10ClientDomain)
		require.NoError(t, err)
	})
}

func Test_validateStellarTransaction(t *testing.T) {
	mockReceiverAccount := "GD44L3Q6NYRFPVOX4CJUUV63QEOOU3R5JNQJBLR6WWXFWYHEGK2YVBQ7"
	mockassetCode := "USDC"
	mockassetIssuer := "GBZF7AS3TBASAL5RQ7ECJODFWFLBDCKJK5SMPUCO5R36CJUIZRWQJTGB"
	mockAmount := "0.1"

	t.Run("error transaction not successful", func(t *testing.T) {
		err := validateStellarTransaction(&PaymentHorizon{
			TransactionSuccessful: false,
		}, mockReceiverAccount, mockassetCode, mockassetIssuer, mockAmount)
		require.EqualError(t, err, "transaction was not successful on horizon network")
	})

	t.Run("error wrong receiver account", func(t *testing.T) {
		err := validateStellarTransaction(&PaymentHorizon{
			TransactionSuccessful: true,
			ReceiverAccount:       "invalidReceiver",
		}, mockReceiverAccount, mockassetCode, mockassetIssuer, mockAmount)
		require.EqualError(t, err, "transaction sent to wrong receiver account")
	})

	t.Run("error wrong amount", func(t *testing.T) {
		err := validateStellarTransaction(&PaymentHorizon{
			TransactionSuccessful: true,
			ReceiverAccount:       "GD44L3Q6NYRFPVOX4CJUUV63QEOOU3R5JNQJBLR6WWXFWYHEGK2YVBQ7",
			Amount:                "20",
		}, mockReceiverAccount, mockassetCode, mockassetIssuer, mockAmount)
		require.EqualError(t, err, "transaction with wrong amount")
	})

	t.Run("error wrong asset code", func(t *testing.T) {
		err := validateStellarTransaction(&PaymentHorizon{
			TransactionSuccessful: true,
			ReceiverAccount:       "GD44L3Q6NYRFPVOX4CJUUV63QEOOU3R5JNQJBLR6WWXFWYHEGK2YVBQ7",
			Amount:                "0.1",
			AssetCode:             "invalidCode",
			AssetIssuer:           "GBZF7AS3TBASAL5RQ7ECJODFWFLBDCKJK5SMPUCO5R36CJUIZRWQJTGB",
		}, mockReceiverAccount, mockassetCode, mockassetIssuer, mockAmount)
		require.EqualError(t, err, "transaction with wrong disbursed asset")
	})

	t.Run("error wrong asset issuer", func(t *testing.T) {
		err := validateStellarTransaction(&PaymentHorizon{
			TransactionSuccessful: true,
			ReceiverAccount:       "GD44L3Q6NYRFPVOX4CJUUV63QEOOU3R5JNQJBLR6WWXFWYHEGK2YVBQ7",
			Amount:                "0.1",
			AssetCode:             "USDC",
			AssetIssuer:           "invalidIssuer",
		}, mockReceiverAccount, mockassetCode, mockassetIssuer, mockAmount)
		require.EqualError(t, err, "transaction with wrong disbursed asset")
	})

	t.Run("successful validation", func(t *testing.T) {
		err := validateStellarTransaction(&PaymentHorizon{
			TransactionSuccessful: true,
			ReceiverAccount:       "GD44L3Q6NYRFPVOX4CJUUV63QEOOU3R5JNQJBLR6WWXFWYHEGK2YVBQ7",
			Amount:                "0.1",
			AssetCode:             "USDC",
			AssetIssuer:           "GBZF7AS3TBASAL5RQ7ECJODFWFLBDCKJK5SMPUCO5R36CJUIZRWQJTGB",
		}, mockReceiverAccount, mockassetCode, mockassetIssuer, mockAmount)
		require.NoError(t, err)
	})
}
