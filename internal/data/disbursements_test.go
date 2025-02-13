package data

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
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
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")
	wallet.Assets = nil

	smsTemplate := "You have a new payment waiting for you from org x. Click on the link to register."

	disbursement := Disbursement{
		Name:   "disbursement",
		Status: DraftDisbursementStatus,
		StatusHistory: []DisbursementStatusHistoryEntry{
			{
				Status: DraftDisbursementStatus,
				UserID: "user1",
			},
		},
		Asset:                               asset,
		Wallet:                              wallet,
		VerificationField:                   VerificationTypeDateOfBirth,
		ReceiverRegistrationMessageTemplate: smsTemplate,
		RegistrationContactType:             RegistrationContactTypePhone,
	}

	t.Run("🔴 fails to insert disbursements with non-unique name", func(t *testing.T) {
		defer DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)

		_, err := disbursementModel.Insert(ctx, &disbursement)
		require.NoError(t, err)
		_, err = disbursementModel.Insert(ctx, &disbursement)
		require.Error(t, err)
		require.Equal(t, ErrRecordAlreadyExists, err)
	})

	t.Run("🟢 successfully insert disbursement", func(t *testing.T) {
		defer DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)

		id, err := disbursementModel.Insert(ctx, &disbursement)
		require.NoError(t, err)
		require.NotNil(t, id)

		actual, err := disbursementModel.Get(ctx, dbConnectionPool, id)
		require.NoError(t, err)

		assert.Equal(t, "disbursement", actual.Name)
		assert.Equal(t, DraftDisbursementStatus, actual.Status)
		assert.Equal(t, asset, actual.Asset)
		assert.Equal(t, wallet, actual.Wallet)
		assert.Equal(t, smsTemplate, actual.ReceiverRegistrationMessageTemplate)
		assert.Equal(t, 1, len(actual.StatusHistory))
		assert.Equal(t, DraftDisbursementStatus, actual.StatusHistory[0].Status)
		assert.Equal(t, "user1", actual.StatusHistory[0].UserID)
		assert.Equal(t, VerificationTypeDateOfBirth, actual.VerificationField)
	})

	t.Run("🟢 successfully insert disbursement (empty:[VerificationField,ReceiverRegistrationMessageTemplate])", func(t *testing.T) {
		defer DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)

		d := disbursement
		d.ReceiverRegistrationMessageTemplate = ""
		d.VerificationField = ""

		id, err := disbursementModel.Insert(ctx, &d)
		require.NoError(t, err)
		require.NotNil(t, id)

		actual, err := disbursementModel.Get(ctx, dbConnectionPool, id)
		require.NoError(t, err)

		assert.Equal(t, "disbursement", actual.Name)
		assert.Equal(t, DraftDisbursementStatus, actual.Status)
		assert.Equal(t, asset, actual.Asset)
		assert.Equal(t, wallet, actual.Wallet)
		assert.Empty(t, actual.ReceiverRegistrationMessageTemplate)
		assert.Equal(t, 1, len(actual.StatusHistory))
		assert.Equal(t, DraftDisbursementStatus, actual.StatusHistory[0].Status)
		assert.Equal(t, "user1", actual.StatusHistory[0].UserID)
		assert.Empty(t, actual.VerificationField)
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
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	disbursement := Disbursement{
		Status: DraftDisbursementStatus,
		StatusHistory: []DisbursementStatusHistoryEntry{
			{
				Status: DraftDisbursementStatus,
				UserID: "user1",
			},
		},
		Asset:  asset,
		Wallet: wallet,
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
		Asset:  asset,
		Wallet: wallet,
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
		Asset:  asset,
		Wallet: wallet,
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
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	disbursement := Disbursement{
		Status: DraftDisbursementStatus,
		StatusHistory: []DisbursementStatusHistoryEntry{
			{
				Status: DraftDisbursementStatus,
				UserID: "user1",
			},
		},
		Asset:  asset,
		Wallet: wallet,
	}

	t.Run("returns empty list when no disbursements exist", func(t *testing.T) {
		DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)
		disbursements, err := disbursementModel.GetAll(ctx, dbConnectionPool, &QueryParams{}, QueryTypeSelectPaginated)
		require.NoError(t, err)
		assert.Equal(t, 0, len(disbursements))
	})

	t.Run("returns disbursements successfully", func(t *testing.T) {
		DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)

		disbursement.Name = "disbursement1"
		expected1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		disbursement.Name = "disbursement2"
		expected2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		actualDisbursements, err := disbursementModel.GetAll(ctx, dbConnectionPool, &QueryParams{}, QueryTypeSelectPaginated)
		require.NoError(t, err)
		assert.Len(t, actualDisbursements, 2)
		assert.Equal(t, []*Disbursement{expected2, expected1}, actualDisbursements)
	})

	t.Run("returns disbursements successfully with limit", func(t *testing.T) {
		DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)

		disbursement.Name = "disbursement1"
		expected1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		disbursement.Name = "disbursement2"
		CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &disbursement)

		actualDisbursements, err := disbursementModel.GetAll(ctx, dbConnectionPool, &QueryParams{Page: 1, PageLimit: 1}, QueryTypeSelectPaginated)
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

		actualDisbursements, err := disbursementModel.GetAll(ctx, dbConnectionPool, &QueryParams{Page: 2, PageLimit: 1}, QueryTypeSelectPaginated)
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

		actualDisbursements, err := disbursementModel.GetAll(ctx, dbConnectionPool,
			&QueryParams{SortBy: SortFieldName, SortOrder: SortOrderDESC},
			QueryTypeSelectPaginated)
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
		actualDisbursements, err := disbursementModel.GetAll(ctx, dbConnectionPool, &QueryParams{Filters: filters}, QueryTypeSelectPaginated)
		require.NoError(t, err)
		assert.Equal(t, 1, len(actualDisbursements))
		assert.Equal(t, []*Disbursement{expected1}, actualDisbursements)
	})

	t.Run("returns disbursements successfully with statuses parameter", func(t *testing.T) {
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
		actualDisbursements, err := disbursementModel.GetAll(ctx, dbConnectionPool,
			&QueryParams{Filters: filters, SortBy: SortFieldCreatedAt, SortOrder: SortOrderDESC},
			QueryTypeSelectPaginated)

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

		actualDisbursements, err := disbursementModel.GetAll(ctx, dbConnectionPool, &QueryParams{}, QueryTypeSelectPaginated)
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
		{Phone: "1234567890", ID: "1", Amount: "123.12", VerificationValue: "1995-02-20"},
		{Phone: "0987654321", ID: "2", Amount: "321", VerificationValue: "1974-07-19"},
		{Phone: "0987654321", ID: "3", Amount: "321", VerificationValue: "1974-07-19"},
	})

	t.Run("update instructions", func(t *testing.T) {
		err := disbursementModel.Update(ctx, dbConnectionPool, &DisbursementUpdate{
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
		err := disbursementModel.Update(ctx, dbConnectionPool, &DisbursementUpdate{
			FileContent: disbursementFileContent,
			FileName:    "instructions.csv",
		})
		require.ErrorContains(t, err, "disbursement ID is required")
	})

	t.Run("no file name in update", func(t *testing.T) {
		err := disbursementModel.Update(ctx, dbConnectionPool, &DisbursementUpdate{
			FileContent: disbursementFileContent,
			ID:          disbursement.ID,
		})
		require.ErrorContains(t, err, "file name is required")
	})

	t.Run("no file content in update", func(t *testing.T) {
		err := disbursementModel.Update(ctx, dbConnectionPool, &DisbursementUpdate{
			FileName: "instructions.csv",
			ID:       disbursement.ID,
		})
		require.ErrorContains(t, err, "file content is required")
	})

	t.Run("empty file content in update", func(t *testing.T) {
		err := disbursementModel.Update(ctx, dbConnectionPool, &DisbursementUpdate{
			FileName:    "instructions.csv",
			ID:          disbursement.ID,
			FileContent: []byte{},
		})
		require.ErrorContains(t, err, "file content is required")
	})

	t.Run("wrong disbursement ID", func(t *testing.T) {
		err := disbursementModel.Update(ctx, dbConnectionPool, &DisbursementUpdate{
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
			VerificationField: VerificationTypeDateOfBirth,
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
			VerificationField: VerificationTypeDateOfBirth,
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
			VerificationField: VerificationTypeDateOfBirth,
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
			VerificationField: VerificationTypeDateOfBirth,
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

func Test_DisbursementModel_Delete(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	models, outerErr := NewModels(dbConnectionPool)
	require.NoError(t, outerErr)

	disbursementModel := &DisbursementModel{dbConnectionPool: dbConnectionPool}
	ctx := context.Background()

	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")
	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN")

	t.Run("successfully deletes draft disbursement", func(t *testing.T) {
		disbursement := CreateDraftDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, Disbursement{
			Name:   uuid.NewString(),
			Asset:  asset,
			Wallet: wallet,
		})

		err := disbursementModel.Delete(ctx, dbConnectionPool, disbursement.ID)
		require.NoError(t, err)

		_, err = models.Disbursements.Get(ctx, dbConnectionPool, disbursement.ID)
		require.Error(t, err)
		assert.Equal(t, ErrRecordNotFound, err)
	})

	t.Run("successfully deletes ready disbursement", func(t *testing.T) {
		disbursement := CreateDraftDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, Disbursement{
			Name:   uuid.NewString(),
			Status: ReadyDisbursementStatus,
			Asset:  asset,
			Wallet: wallet,
		})

		err := disbursementModel.Delete(ctx, dbConnectionPool, disbursement.ID)
		require.NoError(t, err)

		_, err = models.Disbursements.Get(ctx, dbConnectionPool, disbursement.ID)
		require.Error(t, err)
		assert.Equal(t, ErrRecordNotFound, err)
	})

	t.Run("returns error when disbursement not found", func(t *testing.T) {
		err := disbursementModel.Delete(ctx, dbConnectionPool, "non-existent-id")
		require.Error(t, err)
		assert.EqualError(t, err, ErrRecordNotFound.Error())
	})

	t.Run("returns error when disbursement is not in draft status", func(t *testing.T) {
		disbursement := CreateDraftDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, Disbursement{
			Name:   uuid.NewString(),
			Status: StartedDisbursementStatus,
			Asset:  asset,
			Wallet: wallet,
		})

		err := disbursementModel.Delete(ctx, dbConnectionPool, disbursement.ID)
		require.Error(t, err)
		assert.EqualError(t, err, ErrRecordNotFound.Error())

		// Verify disbursement still exists
		_, err = models.Disbursements.Get(ctx, dbConnectionPool, disbursement.ID)
		require.NoError(t, err)
	})

	t.Run("returns error when disbursement has associated payments", func(t *testing.T) {
		disbursement := CreateDraftDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, Disbursement{
			Name:   uuid.NewString(),
			Asset:  asset,
			Wallet: wallet,
		})

		// Create a receiver and receiver wallet
		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, DraftReceiversWalletStatus)

		// Create an associated payment
		CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id",
			StellarOperationID:   "operation-id",
			Status:               SuccessPaymentStatus,
			Disbursement:         disbursement,
			Asset:                *asset,
			ReceiverWallet:       receiverWallet,
		})

		// Attempt to delete the disbursement
		err := disbursementModel.Delete(ctx, dbConnectionPool, disbursement.ID)
		require.Error(t, err)
		assert.ErrorContains(t, err, fmt.Sprintf("deleting disbursement %s because it has associated payments", disbursement.ID))

		// Verify disbursement still exists
		_, err = models.Disbursements.Get(ctx, dbConnectionPool, disbursement.ID)
		require.NoError(t, err)
	})
}

func Test_DisbursementColumnNames(t *testing.T) {
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
				"name",
				"status",
				"status_history",
				"file_content",
				"created_at",
				"updated_at",
				"registration_contact_type",
				"receiver_registration_message_template",
				`COALESCE(verification_field::text, '') AS "verification_field"`,
				`COALESCE(file_name, '') AS "file_name"`,
				`COALESCE(receiver_registration_message_template, '') AS "receiver_registration_message_template"`,
			}, ",\n"),
		},
		{
			tableReference: "d",
			resultAlias:    "",
			expected: strings.Join([]string{
				"d.id",
				"d.name",
				"d.status",
				"d.status_history",
				"d.file_content",
				"d.created_at",
				"d.updated_at",
				"d.registration_contact_type",
				"d.receiver_registration_message_template",
				`COALESCE(d.verification_field::text, '') AS "verification_field"`,
				`COALESCE(d.file_name, '') AS "file_name"`,
				`COALESCE(d.receiver_registration_message_template, '') AS "receiver_registration_message_template"`,
			}, ",\n"),
		},
		{
			tableReference: "d",
			resultAlias:    "disbursement",
			expected: strings.Join([]string{
				`d.id AS "disbursement.id"`,
				`d.name AS "disbursement.name"`,
				`d.status AS "disbursement.status"`,
				`d.status_history AS "disbursement.status_history"`,
				`d.file_content AS "disbursement.file_content"`,
				`d.created_at AS "disbursement.created_at"`,
				`d.updated_at AS "disbursement.updated_at"`,
				`d.registration_contact_type AS "disbursement.registration_contact_type"`,
				`d.receiver_registration_message_template AS "disbursement.receiver_registration_message_template"`,
				`COALESCE(d.verification_field::text, '') AS "disbursement.verification_field"`,
				`COALESCE(d.file_name, '') AS "disbursement.file_name"`,
				`COALESCE(d.receiver_registration_message_template, '') AS "disbursement.receiver_registration_message_template"`,
			}, ",\n"),
		},
	}

	for _, tc := range testCases {
		t.Run(testCaseNameForScanText(t, tc.tableReference, tc.resultAlias), func(t *testing.T) {
			actual := DisbursementColumnNames(tc.tableReference, tc.resultAlias)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

// testCaseNameForScanText returns a string that can be used as the name of a test case.
// It is used to create a test case name for a given table reference and result alias.
func testCaseNameForScanText(t *testing.T, tableReference, resultAlias string) string {
	t.Helper()
	originalColName := "{column_name}"
	scanText := originalColName
	if tableReference != "" {
		scanText = fmt.Sprintf("%s.%s", tableReference, scanText)
	}
	if resultAlias != "" {
		scanText = fmt.Sprintf("%s AS %s.%s", scanText, resultAlias, originalColName)
	}
	return scanText
}

func Test_DisbursementModel_CompleteIfNecessary(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, outerErr := NewModels(dbConnectionPool)
	require.NoError(t, outerErr)

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
			VerificationField: VerificationTypeDateOfBirth,
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

		disbursementIDs, err := models.Disbursements.CompleteIfNecessary(ctx, dbConnectionPool)
		assert.NoError(t, err)
		assert.Empty(t, disbursementIDs)

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
			VerificationField: VerificationTypeDateOfBirth,
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

		disbursementIDs, err := models.Disbursements.CompleteIfNecessary(ctx, dbConnectionPool)
		assert.NoError(t, err)
		assert.Empty(t, disbursementIDs)

		startedDisbursement, err = models.Disbursements.Get(ctx, dbConnectionPool, startedDisbursement.ID)
		require.NoError(t, err)
		assert.Equal(t, StartedDisbursementStatus, startedDisbursement.Status)
	})

	t.Run("completes all started disbursements after payments are successful / canceled", func(t *testing.T) {
		disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Name:              "disbursement 1",
			Status:            StartedDisbursementStatus,
			Asset:             asset,
			Wallet:            wallet,
			VerificationField: VerificationTypeDateOfBirth,
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
			VerificationField: VerificationTypeDateOfBirth,
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
			Status:               CanceledPaymentStatus,
			Disbursement:         disbursement2,
			Asset:                *asset,
			ReceiverWallet:       receiverWallet,
		})

		disbursementIDs, err := models.Disbursements.CompleteIfNecessary(ctx, dbConnectionPool)
		assert.NoError(t, err)
		assert.Len(t, disbursementIDs, 2)
		assert.Equal(t, []string{disbursement1.ID, disbursement2.ID}, disbursementIDs)

		disbursement1, err = models.Disbursements.Get(ctx, dbConnectionPool, disbursement1.ID)
		require.NoError(t, err)
		assert.Equal(t, CompletedDisbursementStatus, disbursement1.Status)

		disbursement2, err = models.Disbursements.Get(ctx, dbConnectionPool, disbursement2.ID)
		require.NoError(t, err)
		assert.Equal(t, CompletedDisbursementStatus, disbursement2.Status)
	})
}
