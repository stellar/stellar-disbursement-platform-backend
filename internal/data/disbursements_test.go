package data

import (
	"context"
	"testing"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_DisbursementModelInsert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	disbursementModel := DisbursementModel{dbConnectionPool: dbConnectionPool}

	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")
	wallet.Assets = nil

	smsTemplate := "You have a new payment waiting for you from org x. Click on the link to register."

	disbursement := Disbursement{
		Name:   "disbursement1",
		Status: DraftDisbursementStatus,
		StatusHistory: []DisbursementStatusHistoryEntry{
			{
				Status: DraftDisbursementStatus,
				UserID: "user1",
			},
		},
		Asset:                          asset,
		Country:                        country,
		Wallet:                         wallet,
		VerificationField:              VerificationFieldDateOfBirth,
		SMSRegistrationMessageTemplate: smsTemplate,
	}

	t.Run("returns error when disbursement already exists is not found", func(t *testing.T) {
		_, err := disbursementModel.Insert(ctx, &disbursement)
		require.NoError(t, err)
		_, err = disbursementModel.Insert(ctx, &disbursement)
		require.Error(t, err)
		require.Equal(t, ErrRecordAlreadyExists, err)
	})

	t.Run("insert disbursement successfully", func(t *testing.T) {
		disbursement.Name = "disbursement2"
		id, err := disbursementModel.Insert(ctx, &disbursement)
		require.NoError(t, err)
		require.NotNil(t, id)

		actual, err := disbursementModel.Get(ctx, dbConnectionPool, id)
		require.NoError(t, err)

		assert.Equal(t, "disbursement2", actual.Name)
		assert.Equal(t, DraftDisbursementStatus, actual.Status)
		assert.Equal(t, asset, actual.Asset)
		assert.Equal(t, country, actual.Country)
		assert.Equal(t, wallet, actual.Wallet)
		assert.Equal(t, smsTemplate, actual.SMSRegistrationMessageTemplate)
		assert.Equal(t, 1, len(actual.StatusHistory))
		assert.Equal(t, DraftDisbursementStatus, actual.StatusHistory[0].Status)
		assert.Equal(t, "user1", actual.StatusHistory[0].UserID)
		assert.Equal(t, VerificationFieldDateOfBirth, actual.VerificationField)
	})
}

func Test_DisbursementModelCount(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	disbursementModel := DisbursementModel{dbConnectionPool: dbConnectionPool}

	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	disbursement := Disbursement{
		Status: DraftDisbursementStatus,
		StatusHistory: []DisbursementStatusHistoryEntry{
			{
				Status: DraftDisbursementStatus,
				UserID: "user1",
			},
		},
		Asset:   asset,
		Country: country,
		Wallet:  wallet,
	}

	t.Run("returns 0 when no disbursements exist", func(t *testing.T) {
		DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)
		count, err := disbursementModel.Count(ctx, dbConnectionPool, &QueryParams{})
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("returns count of disbursements", func(t *testing.T) {
		DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)

		disbursement.Name = "disbursement1"
		CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		disbursement.Name = "disbursement2"
		CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		count, err := disbursementModel.Count(ctx, dbConnectionPool, &QueryParams{})
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("returns count of disbursements", func(t *testing.T) {
		DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)

		disbursement.Name = "disbursement1"
		CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		disbursement.Name = "disbursement2"
		CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		count, err := disbursementModel.Count(ctx, dbConnectionPool, &QueryParams{Query: "2"})
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})
}

func Test_DisbursementModelGet(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	disbursementModel := DisbursementModel{dbConnectionPool: dbConnectionPool}

	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	disbursement := Disbursement{
		Name:   "disbursement1",
		Status: DraftDisbursementStatus,
		StatusHistory: []DisbursementStatusHistoryEntry{
			{
				Status: DraftDisbursementStatus,
				UserID: "user1",
			},
		},
		Asset:   asset,
		Country: country,
		Wallet:  wallet,
	}

	t.Run("returns error when disbursement does not exist", func(t *testing.T) {
		_, err := disbursementModel.Get(ctx, dbConnectionPool, "invalid")
		require.Error(t, err)
		require.Equal(t, ErrRecordNotFound, err)
	})

	t.Run("returns disbursement successfully", func(t *testing.T) {
		expected := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)
		actual, err := disbursementModel.Get(ctx, dbConnectionPool, expected.ID)
		require.NoError(t, err)

		assert.Equal(t, *expected, *actual)
	})
}

func Test_DisbursementModelGetByName(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	disbursementModel := DisbursementModel{dbConnectionPool: dbConnectionPool}

	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	disbursement := Disbursement{
		Name:   "disbursement1",
		Status: DraftDisbursementStatus,
		StatusHistory: []DisbursementStatusHistoryEntry{
			{
				Status: DraftDisbursementStatus,
				UserID: "user1",
			},
		},
		Asset:   asset,
		Country: country,
		Wallet:  wallet,
	}

	t.Run("returns error when disbursement does not exist", func(t *testing.T) {
		_, err := disbursementModel.GetByName(ctx, dbConnectionPool, "invalid")
		require.Error(t, err)
		require.Equal(t, ErrRecordNotFound, err)
	})

	t.Run("returns disbursement get by name successfully", func(t *testing.T) {
		expected := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)
		actual, err := disbursementModel.GetByName(ctx, dbConnectionPool, expected.Name)
		require.NoError(t, err)

		assert.Equal(t, *expected, *actual)
	})
}

func Test_DisbursementModelGetAll(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	disbursementModel := DisbursementModel{dbConnectionPool: dbConnectionPool}
	paymentModel := PaymentModel{dbConnectionPool: dbConnectionPool}

	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	disbursement := Disbursement{
		Status: DraftDisbursementStatus,
		StatusHistory: []DisbursementStatusHistoryEntry{
			{
				Status: DraftDisbursementStatus,
				UserID: "user1",
			},
		},
		Asset:   asset,
		Country: country,
		Wallet:  wallet,
	}

	t.Run("returns empty list when no disbursements exist", func(t *testing.T) {
		DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)
		disbursements, err := disbursementModel.GetAll(ctx, dbConnectionPool, &QueryParams{})
		require.NoError(t, err)
		assert.Equal(t, 0, len(disbursements))
	})

	t.Run("returns disbursements successfully", func(t *testing.T) {
		DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)

		disbursement.Name = "disbursement1"
		expected1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		disbursement.Name = "disbursement2"
		expected2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		actualDisbursements, err := disbursementModel.GetAll(ctx, dbConnectionPool, &QueryParams{})
		require.NoError(t, err)
		assert.Equal(t, 2, len(actualDisbursements))
		assert.Equal(t, []*Disbursement{expected1, expected2}, actualDisbursements)
	})

	t.Run("returns disbursements successfully with limit", func(t *testing.T) {
		DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)

		disbursement.Name = "disbursement1"
		expected1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		disbursement.Name = "disbursement2"
		CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		actualDisbursements, err := disbursementModel.GetAll(ctx, dbConnectionPool, &QueryParams{Page: 1, PageLimit: 1})
		require.NoError(t, err)
		assert.Equal(t, 1, len(actualDisbursements))
		assert.Equal(t, []*Disbursement{expected1}, actualDisbursements)
	})

	t.Run("returns disbursements successfully with offset", func(t *testing.T) {
		DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)

		disbursement.Name = "disbursement1"
		CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		disbursement.Name = "disbursement2"
		expected2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		actualDisbursements, err := disbursementModel.GetAll(ctx, dbConnectionPool, &QueryParams{Page: 2, PageLimit: 1})
		require.NoError(t, err)
		assert.Equal(t, 1, len(actualDisbursements))
		assert.Equal(t, []*Disbursement{expected2}, actualDisbursements)
	})

	t.Run("returns disbursements successfully with order", func(t *testing.T) {
		DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)

		disbursement.Name = "disbursement1"
		expected1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		disbursement.Name = "disbursement2"
		expected2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		actualDisbursements, err := disbursementModel.GetAll(ctx, dbConnectionPool, &QueryParams{SortBy: SortFieldName, SortOrder: SortOrderDESC})
		require.NoError(t, err)
		assert.Equal(t, 2, len(actualDisbursements))
		assert.Equal(t, []*Disbursement{expected2, expected1}, actualDisbursements)
	})

	t.Run("returns disbursements successfully with filter", func(t *testing.T) {
		DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)

		disbursement.Name = "disbursement1"
		disbursement.Status = DraftDisbursementStatus
		expected1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		disbursement.Name = "disbursement2"
		disbursement.Status = CompletedDisbursementStatus
		CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		filters := map[FilterKey]interface{}{
			FilterKeyStatus: []DisbursementStatus{DraftDisbursementStatus},
		}
		actualDisbursements, err := disbursementModel.GetAll(ctx, dbConnectionPool, &QueryParams{Filters: filters})
		require.NoError(t, err)
		assert.Equal(t, 1, len(actualDisbursements))
		assert.Equal(t, []*Disbursement{expected1}, actualDisbursements)
	})

	t.Run("returns disbursements successfully with statuses parameter ", func(t *testing.T) {
		DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)

		disbursement.Name = "disbursement1"
		disbursement.Status = DraftDisbursementStatus
		disbursement.CreatedAt = time.Date(2023, 1, 30, 0, 0, 0, 0, time.UTC)
		expected1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		disbursement.Name = "disbursement2"
		disbursement.Status = CompletedDisbursementStatus
		disbursement.CreatedAt = time.Date(2023, 3, 30, 0, 0, 0, 0, time.UTC)
		expected2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		filters := map[FilterKey]interface{}{
			FilterKeyStatus: []DisbursementStatus{DraftDisbursementStatus, CompletedDisbursementStatus},
		}
		actualDisbursements, err := disbursementModel.GetAll(ctx, dbConnectionPool, &QueryParams{Filters: filters, SortBy: SortFieldCreatedAt, SortOrder: SortOrderDESC})

		require.NoError(t, err)
		assert.Equal(t, 2, len(actualDisbursements))
		assert.Equal(t, []*Disbursement{expected2, expected1}, actualDisbursements)
	})
	t.Run("returns disbursements successfully with stats", func(t *testing.T) {
		DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)

		expectedDisbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)
		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, DraftReceiversWalletStatus)
		CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &Payment{
			ReceiverWallet: receiverWallet,
			Disbursement:   expectedDisbursement,
			Asset:          *asset,
			Amount:         "100",
			Status:         SuccessPaymentStatus,
		})
		CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &Payment{
			ReceiverWallet: receiverWallet,
			Disbursement:   expectedDisbursement,
			Asset:          *asset,
			Amount:         "150.05",
			Status:         DraftPaymentStatus,
		})
		CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &Payment{
			ReceiverWallet: receiverWallet,
			Disbursement:   expectedDisbursement,
			Asset:          *asset,
			Amount:         "020.50",
			Status:         FailedPaymentStatus,
		})
		CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &Payment{
			ReceiverWallet: receiverWallet,
			Disbursement:   expectedDisbursement,
			Asset:          *asset,
			Amount:         "020.50",
			Status:         CanceledPaymentStatus,
		})

		expectedStats := &DisbursementStats{}
		expectedStats.TotalPayments = 4
		expectedStats.SuccessfulPayments = 1
		expectedStats.FailedPayments = 1
		expectedStats.CanceledPayments = 1
		expectedStats.RemainingPayments = 1
		expectedStats.TotalAmount = "291.05"
		expectedStats.AmountDisbursed = "100.00"
		expectedStats.AverageAmount = "72.76"

		expectedDisbursement.DisbursementStats = expectedStats

		actualDisbursements, err := disbursementModel.GetAll(ctx, dbConnectionPool, &QueryParams{})
		require.NoError(t, err)
		assert.Equal(t, 1, len(actualDisbursements))
		assert.Equal(t, []*Disbursement{expectedDisbursement}, actualDisbursements)
	})
}

func Test_DisbursementModel_Update(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	disbursementModel := &DisbursementModel{dbConnectionPool: dbConnectionPool}

	disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, &DisbursementModel{dbConnectionPool: dbConnectionPool}, &Disbursement{
		Name: "disbursement1",
	})

	disbursementFileContent := CreateInstructionsFixture(t, []*DisbursementInstruction{
		{"1234567890", "1", "123.12", "1995-02-20", nil},
		{"0987654321", "2", "321", "1974-07-19", nil},
		{"0987654321", "3", "321", "1974-07-19", nil},
	})

	t.Run("update instructions", func(t *testing.T) {
		err := disbursementModel.Update(ctx, &DisbursementUpdate{
			ID:          disbursement.ID,
			FileContent: disbursementFileContent,
			FileName:    "instructions.csv",
		})
		require.NoError(t, err)
		actual, err := disbursementModel.Get(ctx, dbConnectionPool, disbursement.ID)
		require.NoError(t, err)
		require.Equal(t, "instructions.csv", actual.FileName)
		require.NotEmpty(t, actual.FileContent)
		require.Equal(t, disbursementFileContent, actual.FileContent)
	})

	t.Run("no disbursement ID in update", func(t *testing.T) {
		err := disbursementModel.Update(ctx, &DisbursementUpdate{
			FileContent: disbursementFileContent,
			FileName:    "instructions.csv",
		})
		require.ErrorContains(t, err, "disbursement ID is required")
	})

	t.Run("no file name in update", func(t *testing.T) {
		err := disbursementModel.Update(ctx, &DisbursementUpdate{
			FileContent: disbursementFileContent,
			ID:          disbursement.ID,
		})
		require.ErrorContains(t, err, "file name is required")
	})

	t.Run("no file content in update", func(t *testing.T) {
		err := disbursementModel.Update(ctx, &DisbursementUpdate{
			FileName: "instructions.csv",
			ID:       disbursement.ID,
		})
		require.ErrorContains(t, err, "file content is required")
	})

	t.Run("empty file content in update", func(t *testing.T) {
		err := disbursementModel.Update(ctx, &DisbursementUpdate{
			FileName:    "instructions.csv",
			ID:          disbursement.ID,
			FileContent: []byte{},
		})
		require.ErrorContains(t, err, "file content is required")
	})

	t.Run("wrong disbursement ID", func(t *testing.T) {
		err := disbursementModel.Update(ctx, &DisbursementUpdate{
			FileName:    "instructions.csv",
			ID:          "9e0ff65f-f6e9-46e9-bf03-dc46723e3bfb",
			FileContent: disbursementFileContent,
		})
		require.ErrorContains(t, err, "disbursement 9e0ff65f-f6e9-46e9-bf03-dc46723e3bfb was not updated")
	})
}

func Test_DisbursementModel_CompleteDisbursements(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := NewModels(dbConnectionPool)
	require.NoError(t, err)

	DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
	DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)
	DeleteAllCountryFixtures(t, ctx, dbConnectionPool)
	DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
	DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
	DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
	DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

	country := CreateCountryFixture(t, ctx, dbConnectionPool, "BRA", "Brazil")
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)

	t.Run("does not complete not started disbursement", func(t *testing.T) {
		readyDisbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Name:              "disbursement ready",
			Status:            ReadyDisbursementStatus,
			Asset:             asset,
			Wallet:            wallet,
			Country:           country,
			VerificationField: VerificationFieldDateOfBirth,
		})

		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id",
			StellarOperationID:   "operation-id",
			Status:               SuccessPaymentStatus,
			Disbursement:         readyDisbursement,
			Asset:                *asset,
			ReceiverWallet:       receiverWallet,
		})

		err = models.Disbursements.CompleteDisbursements(ctx, dbConnectionPool, []string{readyDisbursement.ID})
		require.NoError(t, err)

		readyDisbursement, err = models.Disbursements.Get(ctx, dbConnectionPool, readyDisbursement.ID)
		require.NoError(t, err)
		assert.Equal(t, ReadyDisbursementStatus, readyDisbursement.Status)
	})

	t.Run("does not complete started disbursement if not all payments are not completed", func(t *testing.T) {
		startedDisbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Name:              "disbursement started",
			Status:            StartedDisbursementStatus,
			Asset:             asset,
			Wallet:            wallet,
			Country:           country,
			VerificationField: VerificationFieldDateOfBirth,
		})

		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               SuccessPaymentStatus,
			Disbursement:         startedDisbursement,
			Asset:                *asset,
			ReceiverWallet:       receiverWallet,
		})

		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-2",
			StellarOperationID:   "operation-id-2",
			Status:               FailedPaymentStatus,
			Disbursement:         startedDisbursement,
			Asset:                *asset,
			ReceiverWallet:       receiverWallet,
		})

		err = models.Disbursements.CompleteDisbursements(ctx, dbConnectionPool, []string{startedDisbursement.ID})
		require.NoError(t, err)

		startedDisbursement, err = models.Disbursements.Get(ctx, dbConnectionPool, startedDisbursement.ID)
		require.NoError(t, err)
		assert.Equal(t, StartedDisbursementStatus, startedDisbursement.Status)
	})

	t.Run("completes all started disbursements after payments are successful", func(t *testing.T) {
		disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Name:              "disbursement 1",
			Status:            StartedDisbursementStatus,
			Asset:             asset,
			Wallet:            wallet,
			Country:           country,
			VerificationField: VerificationFieldDateOfBirth,
		})

		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id",
			StellarOperationID:   "operation-id",
			Status:               SuccessPaymentStatus,
			Disbursement:         disbursement1,
			Asset:                *asset,
			ReceiverWallet:       receiverWallet,
		})

		disbursement2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Name:              "disbursement 2",
			Status:            StartedDisbursementStatus,
			Asset:             asset,
			Wallet:            wallet,
			Country:           country,
			VerificationField: VerificationFieldDateOfBirth,
		})

		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               SuccessPaymentStatus,
			Disbursement:         disbursement2,
			Asset:                *asset,
			ReceiverWallet:       receiverWallet,
		})

		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-2",
			StellarOperationID:   "operation-id-2",
			Status:               SuccessPaymentStatus,
			Disbursement:         disbursement2,
			Asset:                *asset,
			ReceiverWallet:       receiverWallet,
		})

		err = models.Disbursements.CompleteDisbursements(ctx, dbConnectionPool, []string{disbursement1.ID, disbursement2.ID})
		require.NoError(t, err)

		disbursement1, err = models.Disbursements.Get(ctx, dbConnectionPool, disbursement1.ID)
		require.NoError(t, err)
		assert.Equal(t, CompletedDisbursementStatus, disbursement1.Status)

		disbursement2, err = models.Disbursements.Get(ctx, dbConnectionPool, disbursement2.ID)
		require.NoError(t, err)
		assert.Equal(t, CompletedDisbursementStatus, disbursement2.Status)
	})
}
