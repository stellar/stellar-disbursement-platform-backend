package data

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

func Test_DisbursementInstructionModel_ProcessAll(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	disbursement := CreateDraftDisbursementFixture(t, ctx, dbConnectionPool, &DisbursementModel{dbConnectionPool: dbConnectionPool}, Disbursement{
		Name:   "disbursement1",
		Asset:  asset,
		Wallet: wallet,
	})

	emailDisbursement := CreateDraftDisbursementFixture(t, ctx, dbConnectionPool, &DisbursementModel{dbConnectionPool: dbConnectionPool}, Disbursement{
		Name:                    "disbursement2",
		Asset:                   asset,
		Wallet:                  wallet,
		RegistrationContactType: RegistrationContactTypeEmail,
	})

	di := NewDisbursementInstructionModel(dbConnectionPool)

	smsInstruction1 := DisbursementInstruction{
		Phone:             "+380-12-345-671",
		Amount:            "100.01",
		ID:                "123456781",
		VerificationValue: "1990-01-01",
	}

	smsInstruction2 := DisbursementInstruction{
		Phone:             "+380-12-345-672",
		Amount:            "100.02",
		ID:                "123456782",
		VerificationValue: "1990-01-02",
	}

	smsInstruction3 := DisbursementInstruction{
		Phone:             "+380-12-345-673",
		Amount:            "100.03",
		ID:                "123456783",
		VerificationValue: "1990-01-03",
		ExternalPaymentId: "abc123",
	}

	emailInstruction1 := DisbursementInstruction{
		Email:             "receiver1@stellar.org",
		Amount:            "100.01",
		ID:                "123456781",
		VerificationValue: "1990-01-01",
	}

	emailInstruction2 := DisbursementInstruction{
		Email:             "receiver2@stellar.org",
		Amount:            "100.02",
		ID:                "123456782",
		VerificationValue: "1990-01-02",
	}

	emailInstruction3 := DisbursementInstruction{
		Email:             "receiver3@stellar.org",
		Amount:            "100.03",
		ID:                "123456783",
		VerificationValue: "1990-01-03",
	}

	smsInstructions := []*DisbursementInstruction{&smsInstruction1, &smsInstruction2, &smsInstruction3}
	emailInstructions := []*DisbursementInstruction{&emailInstruction1, &emailInstruction2, &emailInstruction3}
	expectedPhoneNumbers := []string{smsInstruction1.Phone, smsInstruction2.Phone, smsInstruction3.Phone}
	expectedEmails := []string{emailInstruction1.Email, emailInstruction2.Email, emailInstruction3.Email}
	expectedExternalIDs := []string{smsInstruction1.ID, smsInstruction2.ID, smsInstruction3.ID}
	expectedPayments := []string{smsInstruction1.Amount, smsInstruction2.Amount, smsInstruction3.Amount}
	expectedExternalPaymentIDs := []string{smsInstruction3.ExternalPaymentId}

	disbursementUpdate := &DisbursementUpdate{
		ID:          disbursement.ID,
		FileName:    "instructions.csv",
		FileContent: CreateInstructionsFixture(t, smsInstructions),
	}

	knownWalletDisbursement := CreateDraftDisbursementFixture(t, ctx, dbConnectionPool, &DisbursementModel{dbConnectionPool: dbConnectionPool}, Disbursement{
		Name:                    "disbursement with provided receiver wallets",
		Asset:                   asset,
		Wallet:                  wallet,
		RegistrationContactType: RegistrationContactTypePhoneAndWalletAddress,
	})

	knownWalletDisbursementUpdate := func(instructions []*DisbursementInstruction) *DisbursementUpdate {
		return &DisbursementUpdate{
			ID:          knownWalletDisbursement.ID,
			FileName:    "instructions.csv",
			FileContent: CreateInstructionsFixture(t, instructions),
		}
	}

	cleanup := func() {
		DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
	}

	t.Run("failure - invalid wallet address for known wallet address instructions", func(t *testing.T) {
		defer cleanup()

		instructions := []*DisbursementInstruction{
			{
				WalletAddress: "GCVL44LFV3BFI627ABY3YRITFBRJVXUQVPLXQ3LISMI5UVKS5LHWTPT6",
				Amount:        "100.01",
				ID:            "1",
				Phone:         "+380-12-345-679",
			},
		}

		err := di.ProcessAll(ctx, DisbursementInstructionsOpts{
			UserID:                  "user-id",
			Instructions:            instructions,
			Disbursement:            knownWalletDisbursement,
			DisbursementUpdate:      knownWalletDisbursementUpdate(instructions),
			MaxNumberOfInstructions: 10,
		})
		assert.ErrorContains(t, err, "validating receiver wallet update: invalid stellar address")
	})

	t.Run("failure - receiver wallet address mismatch for known wallet address instructions", func(t *testing.T) {
		defer cleanup()

		firstInstruction := []*DisbursementInstruction{
			{
				WalletAddress: "GCVL44LFV3BFI627ABY3YRITFBRJVXUQVPLXQ3LISMI5UVKS5LHWTPT7",
				Amount:        "100.01",
				ID:            "1",
				Phone:         "+380-12-345-671",
			},
		}
		update := knownWalletDisbursementUpdate(firstInstruction)
		err := di.ProcessAll(ctx, DisbursementInstructionsOpts{
			UserID:                  "user-id",
			Instructions:            firstInstruction,
			Disbursement:            knownWalletDisbursement,
			DisbursementUpdate:      update,
			MaxNumberOfInstructions: 10,
		})
		require.NoError(t, err)

		mismatchAddressInstruction := []*DisbursementInstruction{
			{
				WalletAddress: "GC524YE6Z6ISMNLHWFYXQZRR5DTF2A75DYE5TE6G7UMZJ6KZRNVHPOQS",
				Amount:        "100.02",
				ID:            "1",
				Phone:         "+380-12-345-671",
			},
		}
		mismatchUpdate := knownWalletDisbursementUpdate(mismatchAddressInstruction)
		err = di.ProcessAll(ctx, DisbursementInstructionsOpts{
			UserID:                  "user-id",
			Instructions:            mismatchAddressInstruction,
			Disbursement:            knownWalletDisbursement,
			DisbursementUpdate:      mismatchUpdate,
			MaxNumberOfInstructions: 10,
		})
		assert.ErrorIs(t, err, ErrReceiverWalletAddressMismatch)
	})

	t.Run("success - known wallet address instructions", func(t *testing.T) {
		defer cleanup()

		instructions := []*DisbursementInstruction{
			{
				WalletAddress: "GCVL44LFV3BFI627ABY3YRITFBRJVXUQVPLXQ3LISMI5UVKS5LHWTPT7",
				Amount:        "100.01",
				ID:            "1",
				Phone:         "+380-12-345-671",
			},
			{
				WalletAddress: "GC524YE6Z6ISMNLHWFYXQZRR5DTF2A75DYE5TE6G7UMZJ6KZRNVHPOQS",
				Amount:        "100.02",
				ID:            "2",
				Phone:         "+380-12-345-672",
			},
		}

		update := knownWalletDisbursementUpdate(instructions)
		err := di.ProcessAll(ctx, DisbursementInstructionsOpts{
			UserID:                  "user-id",
			Instructions:            instructions,
			Disbursement:            knownWalletDisbursement,
			DisbursementUpdate:      update,
			MaxNumberOfInstructions: 10,
		})
		require.NoError(t, err)

		// Verify Receivers
		receivers, err := di.receiverModel.GetByContacts(ctx, dbConnectionPool, instructions[0].Phone, instructions[1].Phone)
		require.NoError(t, err)
		assertEqualReceivers(t, []string{instructions[0].Phone, instructions[1].Phone}, []string{"1", "2"}, receivers)

		// Verify Receiver Verifications
		receiver1Verifications, err := di.receiverVerificationModel.GetAllByReceiverId(ctx, dbConnectionPool, receivers[0].ID)
		require.NoError(t, err)
		assert.Len(t, receiver1Verifications, 0)
		receiver2Verifications, err := di.receiverVerificationModel.GetAllByReceiverId(ctx, dbConnectionPool, receivers[1].ID)
		require.NoError(t, err)
		assert.Len(t, receiver2Verifications, 0)

		// Verify Receiver Wallets
		receiverWallets, err := di.receiverWalletModel.GetWithReceiverIds(ctx, dbConnectionPool, []string{receivers[0].ID, receivers[1].ID})
		require.NoError(t, err)
		assert.Len(t, receiverWallets, 2)
		for _, receiverWallet := range receiverWallets {
			assert.Equal(t, wallet.ID, receiverWallet.Wallet.ID)
			assert.Contains(t, []string{instructions[0].WalletAddress, instructions[1].WalletAddress}, receiverWallet.StellarAddress)
			assert.Equal(t, RegisteredReceiversWalletStatus, receiverWallet.Status)
		}

		// Verify Payments
		actualPayments := GetPaymentsByDisbursementID(t, ctx, dbConnectionPool, knownWalletDisbursement.ID)
		assert.Len(t, actualPayments, 2)
		assert.Contains(t, actualPayments, instructions[0].Amount)
		assert.Contains(t, actualPayments, instructions[1].Amount)

		actualExternalPaymentIDs := GetExternalPaymentIDsByDisbursementID(t, ctx, dbConnectionPool, knownWalletDisbursement.ID)
		assert.Len(t, actualExternalPaymentIDs, 0)

		// Verify Disbursement
		actualDisbursement, err := di.disbursementModel.Get(ctx, dbConnectionPool, knownWalletDisbursement.ID)
		require.NoError(t, err)
		assert.Equal(t, ReadyDisbursementStatus, actualDisbursement.Status)
		assert.Equal(t, update.FileContent, actualDisbursement.FileContent)
		assert.Equal(t, update.FileName, actualDisbursement.FileName)
	})

	t.Run("success - sms instructions", func(t *testing.T) {
		defer cleanup()

		err := di.ProcessAll(ctx, DisbursementInstructionsOpts{
			UserID:                  "user-id",
			Instructions:            smsInstructions,
			Disbursement:            disbursement,
			DisbursementUpdate:      disbursementUpdate,
			MaxNumberOfInstructions: MaxInstructionsPerDisbursement,
		})
		require.NoError(t, err)

		// Verify Receivers
		receivers, err := di.receiverModel.GetByContacts(ctx, dbConnectionPool, smsInstruction1.Phone, smsInstruction2.Phone, smsInstruction3.Phone)
		require.NoError(t, err)
		assertEqualReceivers(t, expectedPhoneNumbers, expectedExternalIDs, receivers)

		// Verify ReceiverVerifications
		receiverVerifications, err := di.receiverVerificationModel.GetByReceiverIDsAndVerificationField(ctx, dbConnectionPool, []string{receivers[0].ID, receivers[1].ID, receivers[2].ID}, VerificationTypeDateOfBirth)
		require.NoError(t, err)
		assertEqualVerifications(t, smsInstructions, receiverVerifications, receivers)

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

		actualExternalPaymentIDs := GetExternalPaymentIDsByDisbursementID(t, ctx, dbConnectionPool, disbursement.ID)
		assert.Equal(t, expectedExternalPaymentIDs, actualExternalPaymentIDs)

		// Verify Disbursement
		actualDisbursement, err := di.disbursementModel.Get(ctx, dbConnectionPool, disbursement.ID)
		require.NoError(t, err)
		require.Equal(t, ReadyDisbursementStatus, actualDisbursement.Status)
		require.Equal(t, disbursementUpdate.FileContent, actualDisbursement.FileContent)
		require.Equal(t, disbursementUpdate.FileName, actualDisbursement.FileName)
	})

	t.Run("success - email instructions", func(t *testing.T) {
		defer cleanup()

		err := di.ProcessAll(ctx, DisbursementInstructionsOpts{
			UserID:                  "user-id",
			Instructions:            emailInstructions,
			Disbursement:            emailDisbursement,
			DisbursementUpdate:      disbursementUpdate,
			MaxNumberOfInstructions: MaxInstructionsPerDisbursement,
		})
		require.NoError(t, err)

		// Verify Receivers
		receivers, err := di.receiverModel.GetByContacts(ctx, dbConnectionPool, emailInstruction1.Email, emailInstruction2.Email, emailInstruction3.Email)
		require.NoError(t, err)
		assert.Len(t, receivers, len(expectedEmails))
		for _, actual := range receivers {
			assert.Empty(t, actual.PhoneNumber)
			assert.Contains(t, expectedEmails, actual.Email)
			assert.Contains(t, expectedExternalIDs, actual.ExternalID)
		}
	})

	t.Run("failure - email instructions without email fields", func(t *testing.T) {
		defer cleanup()

		err := di.ProcessAll(ctx, DisbursementInstructionsOpts{
			UserID:                  "user-id",
			Instructions:            smsInstructions,
			Disbursement:            emailDisbursement,
			DisbursementUpdate:      disbursementUpdate,
			MaxNumberOfInstructions: MaxInstructionsPerDisbursement,
		})
		require.ErrorContains(t, err, "has no contact information for contact type EMAIL")
	})

	t.Run("failure - email instructions with email and phone fields", func(t *testing.T) {
		defer cleanup()

		emailAndSMSInstructions := []*DisbursementInstruction{
			{
				Phone:             "+380-12-345-671",
				Email:             "receiver1@stellar.org",
				Amount:            "100.01",
				ID:                "123456781",
				VerificationValue: "1990-01-01",
			},
		}

		err := di.ProcessAll(ctx, DisbursementInstructionsOpts{
			UserID:                  "user-id",
			Instructions:            emailAndSMSInstructions,
			Disbursement:            disbursement,
			DisbursementUpdate:      disbursementUpdate,
			MaxNumberOfInstructions: MaxInstructionsPerDisbursement,
		})
		errorMsg := "processing receivers: resolving contact information for instruction with ID %s: phone and email are both provided"
		assert.ErrorContains(t, err, fmt.Sprintf(errorMsg, emailAndSMSInstructions[0].ID))
	})

	t.Run("success - Not confirmed Verification Value updated", func(t *testing.T) {
		defer cleanup()

		// process instructions for the first time
		err := di.ProcessAll(ctx, DisbursementInstructionsOpts{
			UserID:                  "user-id",
			Instructions:            smsInstructions,
			Disbursement:            disbursement,
			DisbursementUpdate:      disbursementUpdate,
			MaxNumberOfInstructions: MaxInstructionsPerDisbursement,
		})
		require.NoError(t, err)

		smsInstruction1.VerificationValue = "1990-01-04"
		err = di.ProcessAll(ctx, DisbursementInstructionsOpts{
			UserID:                  "user-id",
			Instructions:            smsInstructions,
			Disbursement:            disbursement,
			DisbursementUpdate:      disbursementUpdate,
			MaxNumberOfInstructions: MaxInstructionsPerDisbursement,
		})
		require.NoError(t, err)

		// Verify Receivers
		receivers, err := di.receiverModel.GetByContacts(ctx, dbConnectionPool, smsInstruction1.Phone, smsInstruction2.Phone, smsInstruction3.Phone)
		require.NoError(t, err)
		assertEqualReceivers(t, expectedPhoneNumbers, expectedExternalIDs, receivers)

		// Verify ReceiverVerifications
		receiverVerifications, err := di.receiverVerificationModel.GetByReceiverIDsAndVerificationField(ctx, dbConnectionPool, []string{receivers[0].ID, receivers[1].ID, receivers[2].ID}, VerificationTypeDateOfBirth)
		require.NoError(t, err)
		assertEqualVerifications(t, smsInstructions, receiverVerifications, receivers)

		// Verify Disbursement
		actualDisbursement, err := di.disbursementModel.Get(ctx, dbConnectionPool, disbursement.ID)
		require.NoError(t, err)
		require.Equal(t, ReadyDisbursementStatus, actualDisbursement.Status)
		require.Equal(t, disbursementUpdate.FileContent, actualDisbursement.FileContent)
		require.Equal(t, disbursementUpdate.FileName, actualDisbursement.FileName)
	})

	t.Run("success - existing receiver wallet", func(t *testing.T) {
		defer cleanup()

		// New instructions
		readyDisbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, &DisbursementModel{dbConnectionPool: dbConnectionPool}, &Disbursement{
			Name:   "readyDisbursement",
			Wallet: wallet,
			Asset:  asset,
			Status: ReadyDisbursementStatus,
		})

		newInstruction1 := DisbursementInstruction{
			Phone:             "+380-12-345-674",
			Amount:            "100.04",
			ID:                "123456784",
			VerificationValue: "1990-01-04",
		}

		newInstruction2 := DisbursementInstruction{
			Phone:             "+380-12-345-675",
			Amount:            "100.05",
			ID:                "123456785",
			VerificationValue: "1990-01-05",
		}

		newInstruction3 := DisbursementInstruction{
			Phone:             "+380-12-345-676",
			Amount:            "100.06",
			ID:                "123456786",
			VerificationValue: "1990-01-06",
		}
		newInstructions := []*DisbursementInstruction{&newInstruction1, &newInstruction2, &newInstruction3}
		newExpectedPhoneNumbers := []string{newInstruction1.Phone, newInstruction2.Phone, newInstruction3.Phone}
		newExpectedExternalIDs := []string{newInstruction1.ID, newInstruction2.ID, newInstruction3.ID}

		readyDisbursementUpdate := &DisbursementUpdate{
			ID:          readyDisbursement.ID,
			FileName:    "newInstructions.csv",
			FileContent: CreateInstructionsFixture(t, newInstructions),
		}

		err := di.ProcessAll(ctx, DisbursementInstructionsOpts{
			UserID:                  "user-id",
			Instructions:            newInstructions,
			Disbursement:            readyDisbursement,
			DisbursementUpdate:      readyDisbursementUpdate,
			MaxNumberOfInstructions: MaxInstructionsPerDisbursement,
		})
		require.NoError(t, err)

		receivers, err := di.receiverModel.GetByContacts(ctx, dbConnectionPool, newInstruction1.Phone, newInstruction2.Phone, newInstruction3.Phone)
		require.NoError(t, err)
		assertEqualReceivers(t, newExpectedPhoneNumbers, newExpectedExternalIDs, receivers)

		receiverWallets, err := di.receiverWalletModel.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receivers[0].ID, receivers[1].ID, receivers[2].ID}, wallet.ID)
		require.NoError(t, err)

		// Set invitation_sent_at = NOW()
		for _, receiverWallet := range receiverWallets {
			result, updateErr := dbConnectionPool.ExecContext(ctx, "UPDATE receiver_wallets SET invitation_sent_at = NOW() WHERE id = $1", receiverWallet.ID)
			require.NoError(t, updateErr)
			updatedRowsAffected, rowsErr := result.RowsAffected()
			require.NoError(t, rowsErr)
			assert.Equal(t, int64(1), updatedRowsAffected)
		}

		// Update Receiver Waller Status to Ready
		err = di.receiverWalletModel.UpdateStatusByDisbursementID(ctx, dbConnectionPool, readyDisbursement.ID, DraftReceiversWalletStatus, ReadyReceiversWalletStatus)
		require.NoError(t, err)

		// receivers[2] - Update Receiver Waller Status to Registered
		result, err := dbConnectionPool.ExecContext(ctx, "UPDATE receiver_wallets SET status = $1 WHERE receiver_id = $2", RegisteredReceiversWalletStatus, receivers[2].ID)
		require.NoError(t, err)
		updatedRowsAffected, err := result.RowsAffected()
		require.NoError(t, err)
		assert.Equal(t, int64(1), updatedRowsAffected)

		receiverWallets, err = di.receiverWalletModel.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receivers[0].ID, receivers[1].ID, receivers[2].ID}, wallet.ID)
		require.NoError(t, err)
		for _, receiverWallet := range receiverWallets {
			assert.Equal(t, wallet.ID, receiverWallet.Wallet.ID)
			assert.NotNil(t, receiverWallet.InvitationSentAt)
		}

		err = di.ProcessAll(ctx, DisbursementInstructionsOpts{
			UserID:                  "user-id",
			Instructions:            newInstructions,
			Disbursement:            readyDisbursement,
			DisbursementUpdate:      readyDisbursementUpdate,
			MaxNumberOfInstructions: MaxInstructionsPerDisbursement,
		})
		require.NoError(t, err)

		// Verify ReceiverWallets
		receiverWallets, err = di.receiverWalletModel.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receivers[0].ID, receivers[1].ID}, wallet.ID)
		require.NoError(t, err)
		assert.Len(t, receiverWallets, 2)
		for _, receiverWallet := range receiverWallets {
			assert.Equal(t, ReadyReceiversWalletStatus, receiverWallet.Status)
			assert.Nil(t, receiverWallet.InvitationSentAt)
		}

		receiverWallets, err = di.receiverWalletModel.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receivers[2].ID}, wallet.ID)
		require.NoError(t, err)
		assert.Len(t, receiverWallets, 1)
		assert.Equal(t, RegisteredReceiversWalletStatus, receiverWallets[0].Status)
		assert.NotNil(t, receiverWallets[0].InvitationSentAt)
	})

	t.Run("failure - Too many instructions", func(t *testing.T) {
		err := di.ProcessAll(ctx, DisbursementInstructionsOpts{
			UserID:                  "user-id",
			Instructions:            smsInstructions,
			Disbursement:            disbursement,
			DisbursementUpdate:      disbursementUpdate,
			MaxNumberOfInstructions: 2,
		})
		require.EqualError(t, err, "maximum number of instructions exceeded")
	})

	t.Run("failure - Confirmed Verification Value not matching", func(t *testing.T) {
		defer cleanup()

		// process instructions for the first time
		err := di.ProcessAll(ctx, DisbursementInstructionsOpts{
			UserID:                  "user-id",
			Instructions:            smsInstructions,
			Disbursement:            disbursement,
			DisbursementUpdate:      disbursementUpdate,
			MaxNumberOfInstructions: MaxInstructionsPerDisbursement,
		})
		require.NoError(t, err)

		receivers, err := di.receiverModel.GetByContacts(ctx, dbConnectionPool, smsInstruction1.Phone, smsInstruction2.Phone, smsInstruction3.Phone)
		require.Len(t, receivers, 3)
		require.NoError(t, err)
		receiversMap := make(map[string]*Receiver)
		for _, receiver := range receivers {
			receiversMap[receiver.PhoneNumber] = receiver
		}

		// confirm a verification
		ConfirmVerificationForRecipient(t, ctx, dbConnectionPool, receiversMap[smsInstruction3.Phone].ID)

		// process instructions with mismatched verification values
		smsInstruction3.VerificationValue = "1990-01-07"
		err = di.ProcessAll(ctx, DisbursementInstructionsOpts{
			UserID:                  "user-id",
			Instructions:            smsInstructions,
			Disbursement:            disbursement,
			DisbursementUpdate:      disbursementUpdate,
			MaxNumberOfInstructions: MaxInstructionsPerDisbursement,
		})
		require.Error(t, err)
		assert.EqualError(t, err, "running atomic function in RunInTransactionWithResult: processing receiver verifications: receiver verification mismatch: receiver verification for +380-12-345-673 doesn't match. Check instruction with ID 123456783")
	})
}

func assertEqualReceivers(t *testing.T, expectedPhones, expectedExternalIDs []string, actualReceivers []*Receiver) {
	assert.Len(t, actualReceivers, len(expectedPhones))

	for _, actual := range actualReceivers {
		assert.NotNil(t, actual.PhoneNumber)
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
			confirmed_at = NOW(),
			confirmed_by_id = $1,
			confirmed_by_type = 'RECEIVER'
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

func GetExternalPaymentIDsByDisbursementID(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, disbursementID string) []string {
	query := `
	SELECT
		p.external_payment_id
	FROM	
		payments p
		WHERE p.disbursement_id = $1
	`
	var externalPaymentIDRefs []sql.NullString
	err := dbConnectionPool.SelectContext(ctx, &externalPaymentIDRefs, query, disbursementID)
	require.NoError(t, err)

	var externalPaymentIDs []string
	for _, v := range externalPaymentIDRefs {
		if v.String != "" {
			externalPaymentIDs = append(externalPaymentIDs, v.String)
		}
	}

	return externalPaymentIDs
}
