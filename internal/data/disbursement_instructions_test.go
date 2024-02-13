package data

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_DisbursementInstructionModel_ProcessAll(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	eventProducerMock := events.MockProducer{}
	tnt := tenant.Tenant{ID: "tenant-id"}

	ctx = tenant.SaveTenantInContext(ctx, &tnt)

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

	externalPaymentID := "abc123"
	instruction3 := DisbursementInstruction{
		Phone:             "+380-12-345-673",
		Amount:            "100.03",
		ID:                "123456783",
		VerificationValue: "1990-01-03",
		ExternalPaymentId: &externalPaymentID,
	}
	instructions := []*DisbursementInstruction{&instruction1, &instruction2, &instruction3}
	expectedPhoneNumbers := []string{instruction1.Phone, instruction2.Phone, instruction3.Phone}
	expectedExternalIDs := []string{instruction1.ID, instruction2.ID, instruction3.ID}
	expectedPayments := []string{instruction1.Amount, instruction2.Amount, instruction3.Amount}
	expectedExternalPaymentIDs := []string{*instruction3.ExternalPaymentId}

	disbursementUpdate := &DisbursementUpdate{
		ID:          disbursement.ID,
		FileName:    "instructions.csv",
		FileContent: CreateInstructionsFixture(t, instructions),
	}

	t.Run("success", func(t *testing.T) {
		var eventData []schemas.EventReceiverWalletSMSInvitationData
		eventProducerMock.
			On("WriteMessages", ctx, mock.AnythingOfType("[]events.Message")).
			Run(func(args mock.Arguments) {
				msgs := args.Get(1).([]events.Message)
				require.Len(t, msgs, 1)

				msg := msgs[0]

				assert.Equal(t, events.ReceiverWalletNewInvitationTopic, msg.Topic)
				assert.Equal(t, disbursement.ID, msg.Key)
				assert.Equal(t, events.BatchReceiverWalletSMSInvitationType, msg.Type)
				assert.Equal(t, tnt.ID, msg.TenantID)

				var ok bool
				eventData, ok = msg.Data.([]schemas.EventReceiverWalletSMSInvitationData)
				require.True(t, ok)
				assert.Len(t, eventData, 3)
			}).
			Return(nil).
			Once()

		err := di.ProcessAll(ctx, "user-id", instructions, disbursement, disbursementUpdate, MaxInstructionsPerDisbursement, &eventProducerMock)
		require.NoError(t, err)

		// Verify Receivers
		receivers, err := di.receiverModel.GetByPhoneNumbers(ctx, dbConnectionPool, []string{instruction1.Phone, instruction2.Phone, instruction3.Phone})
		require.NoError(t, err)
		assertEqualReceivers(t, expectedPhoneNumbers, expectedExternalIDs, receivers)

		// Verify ReceiverVerifications
		receiverVerifications, err := di.receiverVerificationModel.GetByReceiverIDsAndVerificationField(ctx, dbConnectionPool, []string{receivers[0].ID, receivers[1].ID, receivers[2].ID}, VerificationFieldDateOfBirth)
		require.NoError(t, err)
		assertEqualVerifications(t, instructions, receiverVerifications, receivers)

		// Verify ReceiverWallets
		receiverWallets, err := di.receiverWalletModel.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receivers[0].ID, receivers[1].ID, receivers[2].ID}, wallet.ID)
		require.NoError(t, err)
		assert.Len(t, receiverWallets, len(receivers))

		expectedEventData := make([]schemas.EventReceiverWalletSMSInvitationData, 0, len(receivers))
		for _, receiverWallet := range receiverWallets {
			assert.Equal(t, wallet.ID, receiverWallet.Wallet.ID)
			assert.Equal(t, DraftReceiversWalletStatus, receiverWallet.Status)
			expectedEventData = append(expectedEventData, schemas.EventReceiverWalletSMSInvitationData{ReceiverWalletID: receiverWallet.ID})
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

		// Verify the published events
		assert.Equal(t, expectedEventData, eventData)
	})

	t.Run("success - Not confirmed Verification Value updated", func(t *testing.T) {
		DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)

		eventProducerMock.
			On("WriteMessages", ctx, mock.AnythingOfType("[]events.Message")).
			Run(func(args mock.Arguments) {
				msgs := args.Get(1).([]events.Message)
				require.Len(t, msgs, 1)

				msg := msgs[0]

				assert.Equal(t, events.ReceiverWalletNewInvitationTopic, msg.Topic)
				assert.Equal(t, disbursement.ID, msg.Key)
				assert.Equal(t, events.BatchReceiverWalletSMSInvitationType, msg.Type)
				assert.Equal(t, tnt.ID, msg.TenantID)

				eventData := msg.Data.([]schemas.EventReceiverWalletSMSInvitationData)
				assert.Len(t, eventData, 3)
			}).
			Return(nil).
			Once().
			On("WriteMessages", ctx, mock.AnythingOfType("[]events.Message")).
			Run(func(args mock.Arguments) {
				msgs := args.Get(1).([]events.Message)
				require.Len(t, msgs, 1)

				msg := msgs[0]

				assert.Equal(t, events.ReceiverWalletNewInvitationTopic, msg.Topic)
				assert.Equal(t, disbursement.ID, msg.Key)
				assert.Equal(t, events.BatchReceiverWalletSMSInvitationType, msg.Type)
				assert.Equal(t, tnt.ID, msg.TenantID)

				eventData := msg.Data.([]schemas.EventReceiverWalletSMSInvitationData)
				assert.Len(t, eventData, 0)
			}).
			Return(nil).
			Once()

		// process instructions for the first time
		err := di.ProcessAll(ctx, "user-id", instructions, disbursement, disbursementUpdate, MaxInstructionsPerDisbursement, &eventProducerMock)
		require.NoError(t, err)

		instruction1.VerificationValue = "1990-01-04"
		err = di.ProcessAll(ctx, "user-id", instructions, disbursement, disbursementUpdate, MaxInstructionsPerDisbursement, &eventProducerMock)
		require.NoError(t, err)

		// Verify Receivers
		receivers, err := di.receiverModel.GetByPhoneNumbers(ctx, dbConnectionPool, []string{instruction1.Phone, instruction2.Phone, instruction3.Phone})
		require.NoError(t, err)
		assertEqualReceivers(t, expectedPhoneNumbers, expectedExternalIDs, receivers)

		// Verify ReceiverVerifications
		receiverVerifications, err := di.receiverVerificationModel.GetByReceiverIDsAndVerificationField(ctx, dbConnectionPool, []string{receivers[0].ID, receivers[1].ID, receivers[2].ID}, VerificationFieldDateOfBirth)
		require.NoError(t, err)
		assertEqualVerifications(t, instructions, receiverVerifications, receivers)

		// Verify Disbursement
		actualDisbursement, err := di.disbursementModel.Get(ctx, dbConnectionPool, disbursement.ID)
		require.NoError(t, err)
		require.Equal(t, ReadyDisbursementStatus, actualDisbursement.Status)
		require.Equal(t, disbursementUpdate.FileContent, actualDisbursement.FileContent)
		require.Equal(t, disbursementUpdate.FileName, actualDisbursement.FileName)
	})

	t.Run("success - existing receiver wallet", func(t *testing.T) {
		// New instructions
		readyDisbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, &DisbursementModel{dbConnectionPool: dbConnectionPool}, &Disbursement{
			Name:    "readyDisbursement",
			Country: country,
			Wallet:  wallet,
			Asset:   asset,
			Status:  ReadyDisbursementStatus,
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

		var eventDataFirstRun, eventDataLastRun, expectedEventDataFirstRun, expectedEventDataLastRun []schemas.EventReceiverWalletSMSInvitationData
		eventProducerMock.
			On("WriteMessages", ctx, mock.AnythingOfType("[]events.Message")).
			Run(func(args mock.Arguments) {
				msgs := args.Get(1).([]events.Message)
				require.Len(t, msgs, 1)

				msg := msgs[0]

				assert.Equal(t, events.ReceiverWalletNewInvitationTopic, msg.Topic)
				assert.Equal(t, readyDisbursement.ID, msg.Key)
				assert.Equal(t, events.BatchReceiverWalletSMSInvitationType, msg.Type)
				assert.Equal(t, tnt.ID, msg.TenantID)

				var ok bool
				eventDataFirstRun, ok = msg.Data.([]schemas.EventReceiverWalletSMSInvitationData)
				require.True(t, ok)
				assert.Len(t, eventDataFirstRun, 3)
			}).
			Return(nil).
			Once().
			On("WriteMessages", ctx, mock.AnythingOfType("[]events.Message")).
			Run(func(args mock.Arguments) {
				msgs := args.Get(1).([]events.Message)
				require.Len(t, msgs, 1)

				msg := msgs[0]

				assert.Equal(t, events.ReceiverWalletNewInvitationTopic, msg.Topic)
				assert.Equal(t, readyDisbursement.ID, msg.Key)
				assert.Equal(t, events.BatchReceiverWalletSMSInvitationType, msg.Type)
				assert.Equal(t, tnt.ID, msg.TenantID)

				var ok bool
				eventDataLastRun, ok = msg.Data.([]schemas.EventReceiverWalletSMSInvitationData)
				require.True(t, ok)
				assert.Len(t, eventDataLastRun, 2)
			}).
			Return(nil).
			Once()

		err := di.ProcessAll(ctx, "user-id", newInstructions, readyDisbursement, readyDisbursementUpdate, MaxInstructionsPerDisbursement, &eventProducerMock)
		require.NoError(t, err)

		receivers, err := di.receiverModel.GetByPhoneNumbers(ctx, dbConnectionPool, []string{newInstruction1.Phone, newInstruction2.Phone, newInstruction3.Phone})
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
			expectedEventDataFirstRun = append(expectedEventDataFirstRun, schemas.EventReceiverWalletSMSInvitationData{ReceiverWalletID: receiverWallet.ID})
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

		err = di.ProcessAll(ctx, "user-id", newInstructions, readyDisbursement, readyDisbursementUpdate, MaxInstructionsPerDisbursement, &eventProducerMock)
		require.NoError(t, err)

		// Verify ReceiverWallets
		receiverWallets, err = di.receiverWalletModel.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receivers[0].ID, receivers[1].ID}, wallet.ID)
		require.NoError(t, err)
		assert.Len(t, receiverWallets, 2)
		for _, receiverWallet := range receiverWallets {
			assert.Equal(t, ReadyReceiversWalletStatus, receiverWallet.Status)
			assert.Nil(t, receiverWallet.InvitationSentAt)
			expectedEventDataLastRun = append(expectedEventDataLastRun, schemas.EventReceiverWalletSMSInvitationData{ReceiverWalletID: receiverWallet.ID})
		}

		receiverWallets, err = di.receiverWalletModel.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receivers[2].ID}, wallet.ID)
		require.NoError(t, err)
		assert.Len(t, receiverWallets, 1)
		assert.Equal(t, RegisteredReceiversWalletStatus, receiverWallets[0].Status)
		assert.NotNil(t, receiverWallets[0].InvitationSentAt)

		// Verify event messages published
		assert.Equal(t, expectedEventDataFirstRun, eventDataFirstRun)
		assert.Equal(t, expectedEventDataLastRun, eventDataLastRun)
	})

	t.Run("failure - Too many instructions", func(t *testing.T) {
		err := di.ProcessAll(ctx, "user-id", instructions, disbursement, disbursementUpdate, 2, &eventProducerMock)
		require.EqualError(t, err, "maximum number of instructions exceeded")
	})

	t.Run("failure - Confirmed Verification Value not matching", func(t *testing.T) {
		DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)

		eventProducerMock.
			On("WriteMessages", ctx, mock.AnythingOfType("[]events.Message")).
			Run(func(args mock.Arguments) {
				msgs := args.Get(1).([]events.Message)
				require.Len(t, msgs, 1)

				msg := msgs[0]

				assert.Equal(t, events.ReceiverWalletNewInvitationTopic, msg.Topic)
				assert.Equal(t, disbursement.ID, msg.Key)
				assert.Equal(t, events.BatchReceiverWalletSMSInvitationType, msg.Type)
				assert.Equal(t, tnt.ID, msg.TenantID)

				eventData := msg.Data.([]schemas.EventReceiverWalletSMSInvitationData)
				assert.Len(t, eventData, 3)
			}).
			Return(nil).
			Once()

		// process instructions for the first time
		err := di.ProcessAll(ctx, "user-id", instructions, disbursement, disbursementUpdate, MaxInstructionsPerDisbursement, &eventProducerMock)
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
		err = di.ProcessAll(ctx, "user-id", instructions, disbursement, disbursementUpdate, MaxInstructionsPerDisbursement, &eventProducerMock)
		require.Error(t, err)
		assert.EqualError(t, err, "running atomic function in RunInTransactionWithResult: receiver verification mismatch: receiver verification for +380-12-345-671 doesn't match")
	})

	t.Run("failure - error writing message", func(t *testing.T) {
		DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)

		eventProducerMock.
			On("WriteMessages", ctx, mock.AnythingOfType("[]events.Message")).
			Run(func(args mock.Arguments) {
				msgs := args.Get(1).([]events.Message)
				require.Len(t, msgs, 1)

				msg := msgs[0]

				assert.Equal(t, events.ReceiverWalletNewInvitationTopic, msg.Topic)
				assert.Equal(t, disbursement.ID, msg.Key)
				assert.Equal(t, events.BatchReceiverWalletSMSInvitationType, msg.Type)
				assert.Equal(t, tnt.ID, msg.TenantID)

				eventData, ok := msg.Data.([]schemas.EventReceiverWalletSMSInvitationData)
				require.True(t, ok)
				assert.Len(t, eventData, 3)
			}).
			Return(errors.New("unexpected error")).
			Once()

		err := di.ProcessAll(ctx, "user-id", instructions, disbursement, disbursementUpdate, MaxInstructionsPerDisbursement, &eventProducerMock)
		assert.Error(t, err)

		// Assert no receivers were registered
		receivers, err := di.receiverModel.GetByPhoneNumbers(ctx, dbConnectionPool, []string{instruction1.Phone, instruction2.Phone, instruction3.Phone})
		require.NoError(t, err)
		assert.Empty(t, receivers)
	})

	t.Run("logs when couldn't write message because EventProducer is nil", func(t *testing.T) {
		DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)

		getEntries := log.DefaultLogger.StartTest(log.ErrorLevel)

		err := di.ProcessAll(ctx, "user-id", instructions, disbursement, disbursementUpdate, MaxInstructionsPerDisbursement, nil)
		require.NoError(t, err)

		// Assert no receivers were registered
		receivers, err := di.receiverModel.GetByPhoneNumbers(ctx, dbConnectionPool, []string{instruction1.Phone, instruction2.Phone, instruction3.Phone})
		require.NoError(t, err)
		assert.Len(t, receivers, 3)

		entries := getEntries()
		require.Len(t, entries, 1)
		assert.Contains(t, entries[0].Message, "Message{")
	})

	t.Run("failure - getting tenant from context", func(t *testing.T) {
		DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)

		ctxWithoutTenant := context.Background()

		err := di.ProcessAll(ctxWithoutTenant, "user-id", instructions, disbursement, disbursementUpdate, MaxInstructionsPerDisbursement, &eventProducerMock)
		assert.EqualError(t, err, "running atomic function in RunInTransactionWithResult: creating event producer message: getting tenant from context: tenant not found in context")

		// Assert no receivers were registered
		receivers, err := di.receiverModel.GetByPhoneNumbers(ctxWithoutTenant, dbConnectionPool, []string{instruction1.Phone, instruction2.Phone, instruction3.Phone})
		require.NoError(t, err)
		assert.Empty(t, receivers)
	})

	eventProducerMock.AssertExpectations(t)
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
