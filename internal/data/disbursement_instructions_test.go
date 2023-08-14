package data

import (
	"context"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_DisbursementInstructionModel_ProcessAll(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	disbursement := CreateDraftDisbursementFixture(t, ctx, dbConnectionPool, &DisbursementModel{dbConnectionPool: dbConnectionPool}, Disbursement{
		Name:    "disbursement1",
		Asset:   asset,
		Country: country,
		Wallet:  wallet,
	})

	di := NewDisbursementInstructionModel(dbConnectionPool)

	instruction1 := DisbursementInstruction{
		Phone:             "+380-12-345-671",
		Amount:            "100.01",
		ID:                "123456781",
		VerificationValue: "1990-01-01",
	}

	instruction2 := DisbursementInstruction{
		Phone:             "+380-12-345-672",
		Amount:            "100.02",
		ID:                "123456782",
		VerificationValue: "1990-01-02",
	}

	instruction3 := DisbursementInstruction{
		Phone:             "+380-12-345-673",
		Amount:            "100.03",
		ID:                "123456783",
		VerificationValue: "1990-01-03",
	}
	instructions := []*DisbursementInstruction{&instruction1, &instruction2, &instruction3}
	expectedPhoneNumbers := []string{instruction1.Phone, instruction2.Phone, instruction3.Phone}
	expectedExternalIDs := []string{instruction1.ID, instruction2.ID, instruction3.ID}
	expectedPayments := []string{instruction1.Amount, instruction2.Amount, instruction3.Amount}

	disbursementUpdate := &DisbursementUpdate{
		ID:          disbursement.ID,
		FileName:    "instructions.csv",
		FileContent: CreateInstructionsFixture(t, instructions),
	}

	t.Run("success", func(t *testing.T) {
		err := di.ProcessAll(ctx, "user-id", instructions, disbursement, disbursementUpdate, MaxInstructionsPerDisbursement)
		require.NoError(t, err)

		// Verify Receivers
		receivers, err := di.receiverModel.GetByPhoneNumbers(ctx, dbConnectionPool, []string{instruction1.Phone, instruction2.Phone, instruction3.Phone})
		require.NoError(t, err)
		assertEqualReceivers(t, expectedPhoneNumbers, expectedExternalIDs, receivers)

		// Verify ReceiverVerifications
		receiverVerifications, err := di.receiverVerificationModel.GetByReceiverIdsAndVerificationField(ctx, dbConnectionPool, []string{receivers[0].ID, receivers[1].ID, receivers[2].ID}, VerificationFieldDateOfBirth)
		require.NoError(t, err)
		assertEqualVerifications(t, instructions, receiverVerifications, receivers)

		// Verify ReceiverWallets
		receiverWallets, err := di.receiverWalletModel.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receivers[0].ID, receivers[1].ID, receivers[2].ID}, wallet.ID)
		require.NoError(t, err)
		assert.Len(t, receiverWallets, len(receivers))
		for _, receiverWallet := range receiverWallets {
			assert.Equal(t, wallet.ID, receiverWallet.Wallet.ID)
			assert.Equal(t, DraftReceiversWalletStatus, receiverWallet.Status)
		}

		// Verify Payments
		actualPayments := GetPaymentsByDisbursementID(t, ctx, dbConnectionPool, disbursement.ID)
		assert.Equal(t, expectedPayments, actualPayments)

		// Verify Disbursement
		actualDisbursement, err := di.disbursementModel.Get(ctx, dbConnectionPool, disbursement.ID)
		require.NoError(t, err)
		require.Equal(t, ReadyDisbursementStatus, actualDisbursement.Status)
		require.Equal(t, disbursementUpdate.FileContent, actualDisbursement.FileContent)
		require.Equal(t, disbursementUpdate.FileName, actualDisbursement.FileName)
	})

	t.Run("success - Not confirmed Verification Value updated", func(t *testing.T) {
		// process instructions for the first time
		err := di.ProcessAll(ctx, "user-id", instructions, disbursement, disbursementUpdate, MaxInstructionsPerDisbursement)
		require.NoError(t, err)

		instruction1.VerificationValue = "1990-01-04"
		err = di.ProcessAll(ctx, "user-id", instructions, disbursement, disbursementUpdate, MaxInstructionsPerDisbursement)
		require.NoError(t, err)

		// Verify Receivers
		receivers, err := di.receiverModel.GetByPhoneNumbers(ctx, dbConnectionPool, []string{instruction1.Phone, instruction2.Phone, instruction3.Phone})
		require.NoError(t, err)
		assertEqualReceivers(t, expectedPhoneNumbers, expectedExternalIDs, receivers)

		// Verify ReceiverVerifications
		receiverVerifications, err := di.receiverVerificationModel.GetByReceiverIdsAndVerificationField(ctx, dbConnectionPool, []string{receivers[0].ID, receivers[1].ID, receivers[2].ID}, VerificationFieldDateOfBirth)
		require.NoError(t, err)
		assertEqualVerifications(t, instructions, receiverVerifications, receivers)

		// Verify Disbursement
		actualDisbursement, err := di.disbursementModel.Get(ctx, dbConnectionPool, disbursement.ID)
		require.NoError(t, err)
		require.Equal(t, ReadyDisbursementStatus, actualDisbursement.Status)
		require.Equal(t, disbursementUpdate.FileContent, actualDisbursement.FileContent)
		require.Equal(t, disbursementUpdate.FileName, actualDisbursement.FileName)
	})

	t.Run("failure - Too many instructions", func(t *testing.T) {
		err := di.ProcessAll(ctx, "user-id", instructions, disbursement, disbursementUpdate, 2)
		require.EqualError(t, err, "maximum number of instructions exceeded")
	})

	t.Run("failure - Confirmed Verification Value not matching", func(t *testing.T) {
		// process instructions for the first time
		err := di.ProcessAll(ctx, "user-id", instructions, disbursement, disbursementUpdate, MaxInstructionsPerDisbursement)
		require.NoError(t, err)

		receivers, err := di.receiverModel.GetByPhoneNumbers(ctx, dbConnectionPool, []string{instruction1.Phone, instruction2.Phone, instruction3.Phone})
		require.NoError(t, err)
		receiversMap := make(map[string]*Receiver)
		for _, receiver := range receivers {
			receiversMap[receiver.PhoneNumber] = receiver
		}

		// confirm a verification
		ConfirmVerificationForRecipient(t, ctx, dbConnectionPool, receiversMap[instruction1.Phone].ID)

		// process instructions with mismatched verification values
		instruction1.VerificationValue = "1990-01-07"
		err = di.ProcessAll(ctx, "user-id", instructions, disbursement, disbursementUpdate, MaxInstructionsPerDisbursement)
		require.Error(t, err)
		assert.EqualError(t, err, "running atomic function in RunInTransactionWithResult: receiver verification mismatch: receiver verification for +380-12-345-671 doesn't match")
	})
}

func assertEqualReceivers(t *testing.T, expectedPhones, expectedExternalIDs []string, actualReceivers []*Receiver) {
	assert.Len(t, actualReceivers, len(expectedPhones))

	for _, actual := range actualReceivers {
		assert.Contains(t, expectedPhones, actual.PhoneNumber)
		assert.Contains(t, expectedExternalIDs, actual.ExternalID)
	}
}

func assertEqualVerifications(t *testing.T, expectedInstructions []*DisbursementInstruction, actualVerifications []*ReceiverVerification, receivers []*Receiver) {
	assert.Len(t, actualVerifications, len(expectedInstructions))

	instructionsMap := make(map[string]*DisbursementInstruction)
	for _, instruction := range expectedInstructions {
		instructionsMap[instruction.Phone] = instruction
	}
	phonesByReceiverId := make(map[string]string)
	for _, receiver := range receivers {
		phonesByReceiverId[receiver.ID] = receiver.PhoneNumber
	}

	for _, actual := range actualVerifications {
		instruction := instructionsMap[phonesByReceiverId[actual.ReceiverID]]
		verified := CompareVerificationValue(actual.HashedValue, instruction.VerificationValue)
		assert.True(t, verified)
	}
}

func ConfirmVerificationForRecipient(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, receiverID string) {
	query := `
		UPDATE
			receiver_verifications
		SET
			confirmed_at = now()
		WHERE
			receiver_id = $1
		`
	_, err := dbConnectionPool.ExecContext(ctx, query, receiverID)
	require.NoError(t, err)
}

func GetPaymentsByDisbursementID(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, disbursementID string) []string {
	query := `
		SELECT
			ROUND(p.amount, 2)
		FROM	
			payments p
			WHERE p.disbursement_id = $1
		`
	var payments []string
	err := dbConnectionPool.SelectContext(ctx, &payments, query, disbursementID)
	require.NoError(t, err)
	return payments
}
