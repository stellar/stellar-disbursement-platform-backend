package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewSendReceiverWalletsSMSInvitationJob(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	anchorPlatformBaseSepURL := "http://localhost:8000"

	messageDryRunClient, err := message.NewDryRunClient()
	require.NoError(t, err)

	t.Run("exits with status 1 when Messenger Client is missing config", func(t *testing.T) {
		if os.Getenv("TEST_FATAL") == "1" {
			o := SendReceiverWalletsSMSInvitationJobOptions{
				Models:                         models,
				AnchorPlatformBaseSepURL:       anchorPlatformBaseSepURL,
				MaxInvitationSMSResendAttempts: 3,
			}

			NewSendReceiverWalletsSMSInvitationJob(o)
			return
		}

		// We're using a strategy to setup a cmd inside the test that calls the test itself and verifies if it exited with exit status '1'.
		// Ref: https://go.dev/talks/2014/testing.slide#23
		cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", t.Name()))
		cmd.Env = append(os.Environ(), "TEST_FATAL=1")

		err := cmd.Run()
		if exitError, ok := err.(*exec.ExitError); ok {
			assert.False(t, exitError.Success())
			return
		}

		t.Fatalf("process ran with err %v, want exit status 1", err)
	})

	t.Run("exits with status 1 when Base URL is empty", func(t *testing.T) {
		if os.Getenv("TEST_FATAL") == "1" {
			o := SendReceiverWalletsSMSInvitationJobOptions{
				Models:                         models,
				MessengerClient:                messageDryRunClient,
				AnchorPlatformBaseSepURL:       "",
				MaxInvitationSMSResendAttempts: 3,
			}

			NewSendReceiverWalletsSMSInvitationJob(o)
			return
		}

		// We're using a strategy to setup a cmd inside the test that calls the test itself and verifies if it exited with exit status '1'.
		// Ref: https://go.dev/talks/2014/testing.slide#23
		cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", t.Name()))
		cmd.Env = append(os.Environ(), "TEST_FATAL=1")

		err := cmd.Run()
		if exitError, ok := err.(*exec.ExitError); ok {
			assert.False(t, exitError.Success())
			return
		}

		t.Fatalf("process ran with err %v, want exit status 1", err)
	})

	t.Run("returns a job instance successfully", func(t *testing.T) {
		o := SendReceiverWalletsSMSInvitationJobOptions{
			Models:                         models,
			MessengerClient:                messageDryRunClient,
			AnchorPlatformBaseSepURL:       anchorPlatformBaseSepURL,
			MaxInvitationSMSResendAttempts: 3,
		}

		j := NewSendReceiverWalletsSMSInvitationJob(o)

		assert.NotNil(t, j)
	})
}

func Test_SendReceiverWalletsSMSInvitationJob(t *testing.T) {
	j := SendReceiverWalletsSMSInvitationJob{}

	assert.Equal(t, SendReceiverWalletsSMSInvitationJobName, j.GetName())
	assert.Equal(t, SendReceiverWalletsSMSInvitationJobIntervalSeconds*time.Second, j.GetInterval())
}

func Test_SendReceiverWalletsSMSInvitationJob_Execute(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	anchorPlatformBaseSepURL := "http://localhost:8000"
	stellarSecretKey := "SBUSPEKAZKLZSWHRSJ2HWDZUK6I3IVDUWA7JJZSGBLZ2WZIUJI7FPNB5"
	var maxInvitationSMSResendAttempts int64 = 3

	ctx := context.Background()

	t.Run("executes the service successfully", func(t *testing.T) {
		messengerClientMock := &message.MessengerClientMock{}
		crashTrackerClientMock := &crashtracker.MockCrashTrackerClient{}

		s, err := services.NewSendReceiverWalletInviteService(
			models,
			messengerClientMock,
			anchorPlatformBaseSepURL,
			stellarSecretKey,
			maxInvitationSMSResendAttempts,
			crashTrackerClientMock,
		)
		require.NoError(t, err)

		data.DeleteAllCountryFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)
		data.ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllMessagesFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)

		country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "ATL", "Atlantis")

		wallet1 := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet1", "https://wallet1.com", "www.wallet1.com", "wallet1://sdp")
		wallet2 := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet2", "https://wallet2.com", "www.wallet2.com", "wallet2://sdp")

		asset1 := data.CreateAssetFixture(t, ctx, dbConnectionPool, "FOO1", "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX")
		asset2 := data.CreateAssetFixture(t, ctx, dbConnectionPool, "FOO2", "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX")

		receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})

		disbursement1 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Country: country,
			Wallet:  wallet1,
			Status:  data.ReadyDisbursementStatus,
			Asset:   asset1,
		})

		disbursement2 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Country: country,
			Wallet:  wallet2,
			Status:  data.ReadyDisbursementStatus,
			Asset:   asset2,
		})

		rec1RW := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet1.ID, data.ReadyReceiversWalletStatus)
		data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet2.ID, data.RegisteredReceiversWalletStatus)

		rec2RW := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet2.ID, data.ReadyReceiversWalletStatus)

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Status:         data.ReadyPaymentStatus,
			Disbursement:   disbursement1,
			Asset:          *asset1,
			ReceiverWallet: rec1RW,
			Amount:         "1",
		})

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Status:         data.ReadyPaymentStatus,
			Disbursement:   disbursement2,
			Asset:          *asset2,
			ReceiverWallet: rec2RW,
			Amount:         "1",
		})

		walletDeepLink1 := services.WalletDeepLink{
			DeepLink:                 wallet1.DeepLinkSchema,
			AnchorPlatformBaseSepURL: anchorPlatformBaseSepURL,
			OrganizationName:         "MyCustomAid",
			AssetCode:                asset1.Code,
			AssetIssuer:              asset1.Issuer,
		}
		deepLink1, err := walletDeepLink1.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		contentWallet1 := fmt.Sprintf("You have a payment waiting for you from the MyCustomAid. Click %s to register.", deepLink1)

		walletDeepLink2 := services.WalletDeepLink{
			DeepLink:                 wallet2.DeepLinkSchema,
			AnchorPlatformBaseSepURL: anchorPlatformBaseSepURL,
			OrganizationName:         "MyCustomAid",
			AssetCode:                asset2.Code,
			AssetIssuer:              asset2.Issuer,
		}
		deepLink2, err := walletDeepLink2.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		contentWallet2 := fmt.Sprintf("You have a payment waiting for you from the MyCustomAid. Click %s to register.", deepLink2)

		mockErr := errors.New("unexpected error")
		messengerClientMock.
			On("SendMessage", message.Message{
				ToPhoneNumber: receiver1.PhoneNumber,
				Message:       contentWallet1,
			}).
			Return(mockErr).
			Once().
			On("SendMessage", message.Message{
				ToPhoneNumber: receiver2.PhoneNumber,
				Message:       contentWallet2,
			}).
			Return(nil).
			Once().
			On("MessengerType").
			Return(message.MessengerTypeTwilioSMS)

		mockMsg := fmt.Sprintf(
			"error sending message to receiver ID %s for receiver wallet ID %s using messenger type %s",
			receiver1.ID, rec1RW.ID, message.MessengerTypeTwilioSMS,
		)
		crashTrackerClientMock.On("LogAndReportErrors", ctx, mockErr, mockMsg).Once()

		err = s.SendInvite(ctx)
		require.NoError(t, err)

		q := `
			SELECT
				type, status, receiver_id, wallet_id, receiver_wallet_id,
				title_encrypted, text_encrypted, status_history
			FROM
				messages
			WHERE
				receiver_id = $1 AND wallet_id = $2 AND receiver_wallet_id = $3
		`
		var msg data.Message
		err = dbConnectionPool.GetContext(ctx, &msg, q, receiver1.ID, wallet1.ID, rec1RW.ID)
		require.NoError(t, err)

		assert.Equal(t, message.MessengerTypeTwilioSMS, msg.Type)
		assert.Equal(t, receiver1.ID, msg.ReceiverID)
		assert.Equal(t, wallet1.ID, msg.WalletID)
		assert.Equal(t, rec1RW.ID, *msg.ReceiverWalletID)
		assert.Equal(t, data.FailureMessageStatus, msg.Status)
		assert.Empty(t, msg.TitleEncrypted)
		assert.Equal(t, contentWallet1, msg.TextEncrypted)
		assert.Len(t, msg.StatusHistory, 2)
		assert.Equal(t, data.PendingMessageStatus, msg.StatusHistory[0].Status)
		assert.Equal(t, data.FailureMessageStatus, msg.StatusHistory[1].Status)
		assert.Nil(t, msg.AssetID)

		msg = data.Message{}
		err = dbConnectionPool.GetContext(ctx, &msg, q, receiver2.ID, wallet2.ID, rec2RW.ID)
		require.NoError(t, err)

		assert.Equal(t, message.MessengerTypeTwilioSMS, msg.Type)
		assert.Equal(t, receiver2.ID, msg.ReceiverID)
		assert.Equal(t, wallet2.ID, msg.WalletID)
		assert.Equal(t, rec2RW.ID, *msg.ReceiverWalletID)
		assert.Equal(t, data.SuccessMessageStatus, msg.Status)
		assert.Empty(t, msg.TitleEncrypted)
		assert.Equal(t, contentWallet2, msg.TextEncrypted)
		assert.Len(t, msg.StatusHistory, 2)
		assert.Equal(t, data.PendingMessageStatus, msg.StatusHistory[0].Status)
		assert.Equal(t, data.SuccessMessageStatus, msg.StatusHistory[1].Status)
		assert.Nil(t, msg.AssetID)
	})
}
