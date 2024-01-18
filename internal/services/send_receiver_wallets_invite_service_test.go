package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_GetSignedRegistrationLink_SchemelessDeepLink(t *testing.T) {
	wdl := WalletDeepLink{
		DeepLink:                 "api-dev.vibrantapp.com/sdp-dev",
		AnchorPlatformBaseSepURL: "https://ap.localhost.com",
		OrganizationName:         "FOO Org",
		AssetCode:                "USDC",
		AssetIssuer:              "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
	}

	registrationLink, err := wdl.GetSignedRegistrationLink("SCTOVDWM3A7KLTXXIV6YXL6QRVUIIG4HHHIDDKPR4JUB3DGDIKI5VGA2")
	require.NoError(t, err)
	wantRegistrationLink := "https://api-dev.vibrantapp.com/sdp-dev?asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5&domain=ap.localhost.com&name=FOO+Org&signature=b40479041eea534a029c6aadf36f3bf6696aba9ff64684b558b9a412150b31fa8480ac7babcdef17cb445c1d105a761dbaa3599361c2d9e1d526fd4a5bac370a"
	require.Equal(t, wantRegistrationLink, registrationLink)

	wdl = WalletDeepLink{
		DeepLink:                 "https://www.beansapp.com/disbursements/registration?redirect=true",
		AnchorPlatformBaseSepURL: "https://ap.localhost.com",
		OrganizationName:         "FOO Org",
		AssetCode:                "USDC",
		AssetIssuer:              "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
	}

	registrationLink, err = wdl.GetSignedRegistrationLink("SCTOVDWM3A7KLTXXIV6YXL6QRVUIIG4HHHIDDKPR4JUB3DGDIKI5VGA2")
	require.NoError(t, err)
	wantRegistrationLink = "https://www.beansapp.com/disbursements/registration?asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5&domain=ap.localhost.com&name=FOO+Org&redirect=true&signature=8dd0a570bf5590a8e1a4983d413b5429ed504659543cf180fbf1b3ffbf0ea90083789a7c0c615d9cbddbe0c59f7555e6fd33fb5ca8f4685c821fc23ad7cd2f0d"
	require.Equal(t, wantRegistrationLink, registrationLink)
}

func Test_SendReceiverWalletInviteService(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	anchorPlatformBaseSepURL := "http://localhost:8000"
	stellarSecretKey := "SBUSPEKAZKLZSWHRSJ2HWDZUK6I3IVDUWA7JJZSGBLZ2WZIUJI7FPNB5"
	messengerClientMock := &message.MessengerClientMock{}
	messengerClientMock.
		On("MessengerType").
		Return(message.MessengerTypeTwilioSMS).
		Maybe()

	mockCrashTrackerClient := &crashtracker.MockCrashTrackerClient{}

	ctx := context.Background()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

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

	t.Run("returns error when service has wrong setup", func(t *testing.T) {
		_, err := NewSendReceiverWalletInviteService(models, nil, anchorPlatformBaseSepURL, stellarSecretKey, 3, mockCrashTrackerClient)
		assert.EqualError(t, err, "invalid service setup: messenger client can't be nil")

		_, err = NewSendReceiverWalletInviteService(models, messengerClientMock, "", stellarSecretKey, 3, mockCrashTrackerClient)
		assert.EqualError(t, err, "invalid service setup: anchorPlatformBaseSepURL can't be empty")
	})

	t.Run("inserts the failed sent message", func(t *testing.T) {
		s, err := NewSendReceiverWalletInviteService(models, messengerClientMock, anchorPlatformBaseSepURL, stellarSecretKey, 3, mockCrashTrackerClient)
		require.NoError(t, err)

		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllMessagesFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

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

		walletDeepLink1 := WalletDeepLink{
			DeepLink:                 wallet1.DeepLinkSchema,
			AnchorPlatformBaseSepURL: anchorPlatformBaseSepURL,
			OrganizationName:         "MyCustomAid",
			AssetCode:                asset1.Code,
			AssetIssuer:              asset1.Issuer,
		}
		deepLink1, err := walletDeepLink1.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		contentWallet1 := fmt.Sprintf("You have a payment waiting for you from the MyCustomAid. Click %s to register.", deepLink1)

		walletDeepLink2 := WalletDeepLink{
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
			Return(errors.New("unexpected error")).
			Once().
			On("SendMessage", message.Message{
				ToPhoneNumber: receiver2.PhoneNumber,
				Message:       contentWallet2,
			}).
			Return(nil).
			Once()

		mockMsg := fmt.Sprintf(
			"error sending message to receiver ID %s for receiver wallet ID %s using messenger type %s",
			receiver1.ID, rec1RW.ID, message.MessengerTypeTwilioSMS,
		)
		mockCrashTrackerClient.On("LogAndReportErrors", ctx, mockErr, mockMsg).Once()

		err = s.SendInvite(ctx)
		require.NoError(t, err)

		receivers, err := models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiver1.ID}, wallet1.ID)
		require.NoError(t, err)
		require.Len(t, receivers, 1)
		assert.Equal(t, rec1RW.ID, receivers[0].ID)
		assert.Nil(t, receivers[0].InvitationSentAt)

		receivers, err = models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiver2.ID}, wallet2.ID)
		require.NoError(t, err)
		require.Len(t, receivers, 1)
		assert.Equal(t, rec2RW.ID, receivers[0].ID)
		assert.NotNil(t, receivers[0].InvitationSentAt)

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

		mockCrashTrackerClient.AssertExpectations(t)
	})

	t.Run("send invite successfully", func(t *testing.T) {
		s, err := NewSendReceiverWalletInviteService(models, messengerClientMock, anchorPlatformBaseSepURL, stellarSecretKey, 3, mockCrashTrackerClient)
		require.NoError(t, err)

		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllMessagesFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

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

		walletDeepLink1 := WalletDeepLink{
			DeepLink:                 wallet1.DeepLinkSchema,
			AnchorPlatformBaseSepURL: anchorPlatformBaseSepURL,
			OrganizationName:         "MyCustomAid",
			AssetCode:                asset1.Code,
			AssetIssuer:              asset1.Issuer,
		}
		deepLink1, err := walletDeepLink1.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		contentWallet1 := fmt.Sprintf("You have a payment waiting for you from the MyCustomAid. Click %s to register.", deepLink1)

		walletDeepLink2 := WalletDeepLink{
			DeepLink:                 wallet2.DeepLinkSchema,
			AnchorPlatformBaseSepURL: anchorPlatformBaseSepURL,
			OrganizationName:         "MyCustomAid",
			AssetCode:                asset2.Code,
			AssetIssuer:              asset2.Issuer,
		}
		deepLink2, err := walletDeepLink2.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		contentWallet2 := fmt.Sprintf("You have a payment waiting for you from the MyCustomAid. Click %s to register.", deepLink2)

		messengerClientMock.
			On("SendMessage", message.Message{
				ToPhoneNumber: receiver1.PhoneNumber,
				Message:       contentWallet1,
			}).
			Return(nil).
			Once().
			On("SendMessage", message.Message{
				ToPhoneNumber: receiver2.PhoneNumber,
				Message:       contentWallet2,
			}).
			Return(nil).
			Once()

		err = s.SendInvite(ctx)
		require.NoError(t, err)

		receivers, err := models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiver1.ID}, wallet1.ID)
		require.NoError(t, err)
		require.Len(t, receivers, 1)
		assert.Equal(t, rec1RW.ID, receivers[0].ID)
		assert.NotNil(t, receivers[0].InvitationSentAt)

		receivers, err = models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiver2.ID}, wallet2.ID)
		require.NoError(t, err)
		require.Len(t, receivers, 1)
		assert.Equal(t, rec2RW.ID, receivers[0].ID)
		assert.NotNil(t, receivers[0].InvitationSentAt)

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
		assert.Equal(t, data.SuccessMessageStatus, msg.Status)
		assert.Empty(t, msg.TitleEncrypted)
		assert.Equal(t, contentWallet1, msg.TextEncrypted)
		assert.Len(t, msg.StatusHistory, 2)
		assert.Equal(t, data.PendingMessageStatus, msg.StatusHistory[0].Status)
		assert.Equal(t, data.SuccessMessageStatus, msg.StatusHistory[1].Status)
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

	t.Run("send invite successfully with custom invite message", func(t *testing.T) {
		s, err := NewSendReceiverWalletInviteService(models, messengerClientMock, anchorPlatformBaseSepURL, stellarSecretKey, 3, mockCrashTrackerClient)
		require.NoError(t, err)

		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllMessagesFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		rec1RW := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet1.ID, data.ReadyReceiversWalletStatus)
		data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet2.ID, data.RegisteredReceiversWalletStatus)

		rec2RW := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet2.ID, data.ReadyReceiversWalletStatus)

		customInvitationMessage := "My custom receiver wallet registration invite. MyOrg 👋"
		err = models.Organizations.Update(ctx, &data.OrganizationUpdate{SMSRegistrationMessageTemplate: &customInvitationMessage})
		require.NoError(t, err)

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

		walletDeepLink1 := WalletDeepLink{
			DeepLink:                 wallet1.DeepLinkSchema,
			AnchorPlatformBaseSepURL: anchorPlatformBaseSepURL,
			OrganizationName:         "MyCustomAid",
			AssetCode:                asset1.Code,
			AssetIssuer:              asset1.Issuer,
		}
		deepLink1, err := walletDeepLink1.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		contentWallet1 := fmt.Sprintf("%s %s", customInvitationMessage, deepLink1)

		walletDeepLink2 := WalletDeepLink{
			DeepLink:                 wallet2.DeepLinkSchema,
			AnchorPlatformBaseSepURL: anchorPlatformBaseSepURL,
			OrganizationName:         "MyCustomAid",
			AssetCode:                asset2.Code,
			AssetIssuer:              asset2.Issuer,
		}
		deepLink2, err := walletDeepLink2.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		contentWallet2 := fmt.Sprintf("%s %s", customInvitationMessage, deepLink2)

		messengerClientMock.
			On("SendMessage", message.Message{
				ToPhoneNumber: receiver1.PhoneNumber,
				Message:       contentWallet1,
			}).
			Return(nil).
			Once().
			On("SendMessage", message.Message{
				ToPhoneNumber: receiver2.PhoneNumber,
				Message:       contentWallet2,
			}).
			Return(nil).
			Once()

		err = s.SendInvite(ctx)
		require.NoError(t, err)

		receivers, err := models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiver1.ID}, wallet1.ID)
		require.NoError(t, err)
		require.Len(t, receivers, 1)
		assert.Equal(t, rec1RW.ID, receivers[0].ID)
		assert.NotNil(t, receivers[0].InvitationSentAt)

		receivers, err = models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiver2.ID}, wallet2.ID)
		require.NoError(t, err)
		require.Len(t, receivers, 1)
		assert.Equal(t, rec2RW.ID, receivers[0].ID)
		assert.NotNil(t, receivers[0].InvitationSentAt)

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
		assert.Equal(t, data.SuccessMessageStatus, msg.Status)
		assert.Empty(t, msg.TitleEncrypted)
		assert.Equal(t, contentWallet1, msg.TextEncrypted)
		assert.Len(t, msg.StatusHistory, 2)
		assert.Equal(t, data.PendingMessageStatus, msg.StatusHistory[0].Status)
		assert.Equal(t, data.SuccessMessageStatus, msg.StatusHistory[1].Status)
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

	t.Run("doesn't resend the invitation SMS when organization's SMS Resend Interval is nil and the invitation was already sent", func(t *testing.T) {
		s, err := NewSendReceiverWalletInviteService(models, messengerClientMock, anchorPlatformBaseSepURL, stellarSecretKey, 3, mockCrashTrackerClient)
		require.NoError(t, err)

		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllMessagesFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		rec1RW := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet1.ID, data.ReadyReceiversWalletStatus)
		data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet2.ID, data.RegisteredReceiversWalletStatus)

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Status:         data.ReadyPaymentStatus,
			Disbursement:   disbursement1,
			Asset:          *asset1,
			ReceiverWallet: rec1RW,
			Amount:         "1",
		})

		// Marking as sent
		var invitationSentAt time.Time
		const q = "UPDATE receiver_wallets SET invitation_sent_at = NOW() WHERE id = $1 RETURNING invitation_sent_at"
		err = dbConnectionPool.GetContext(ctx, &invitationSentAt, q, rec1RW.ID)
		require.NoError(t, err)

		err = models.Organizations.Update(ctx, &data.OrganizationUpdate{SMSResendInterval: new(int64)})
		require.NoError(t, err)

		err = s.SendInvite(ctx)
		require.NoError(t, err)

		receivers, err := models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiver1.ID}, wallet1.ID)
		require.NoError(t, err)
		require.Len(t, receivers, 1)
		assert.Equal(t, rec1RW.ID, receivers[0].ID)
		require.NotNil(t, receivers[0].InvitationSentAt)
		assert.Equal(t, invitationSentAt, *receivers[0].InvitationSentAt)
	})

	t.Run("doesn't resend the invitation SMS when receiver reached the maximum number of resend attempts", func(t *testing.T) {
		s, err := NewSendReceiverWalletInviteService(models, messengerClientMock, anchorPlatformBaseSepURL, stellarSecretKey, 3, mockCrashTrackerClient)
		require.NoError(t, err)

		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllMessagesFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		rec1RW := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet1.ID, data.ReadyReceiversWalletStatus)
		data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet2.ID, data.RegisteredReceiversWalletStatus)

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Status:         data.ReadyPaymentStatus,
			Disbursement:   disbursement1,
			Asset:          *asset1,
			ReceiverWallet: rec1RW,
			Amount:         "1",
		})

		// Marking as sent
		var invitationSentAt time.Time
		const q = "UPDATE receiver_wallets SET invitation_sent_at = NOW() - interval '8 days' WHERE id = $1 RETURNING invitation_sent_at"
		err = dbConnectionPool.GetContext(ctx, &invitationSentAt, q, rec1RW.ID)
		require.NoError(t, err)

		// Set the SMS Resend Interval
		var smsResendInterval int64 = 2
		err = models.Organizations.Update(ctx, &data.OrganizationUpdate{SMSResendInterval: &smsResendInterval})
		require.NoError(t, err)

		_ = data.CreateMessageFixture(t, ctx, dbConnectionPool, &data.Message{
			Type:             message.MessengerTypeDryRun,
			AssetID:          &asset1.ID,
			ReceiverID:       receiver1.ID,
			WalletID:         wallet1.ID,
			ReceiverWalletID: &rec1RW.ID,
			Status:           data.SuccessMessageStatus,
			CreatedAt:        invitationSentAt.AddDate(0, 0, int(smsResendInterval)),
			UpdatedAt:        invitationSentAt.AddDate(0, 0, int(smsResendInterval)),
		})

		_ = data.CreateMessageFixture(t, ctx, dbConnectionPool, &data.Message{
			Type:             message.MessengerTypeDryRun,
			AssetID:          &asset1.ID,
			ReceiverID:       receiver1.ID,
			WalletID:         wallet1.ID,
			ReceiverWalletID: &rec1RW.ID,
			Status:           data.SuccessMessageStatus,
			CreatedAt:        time.Now().AddDate(0, 0, int(smsResendInterval*2)),
			UpdatedAt:        time.Now().AddDate(0, 0, int(smsResendInterval*2)),
		})

		_ = data.CreateMessageFixture(t, ctx, dbConnectionPool, &data.Message{
			Type:             message.MessengerTypeDryRun,
			AssetID:          &asset1.ID,
			ReceiverID:       receiver1.ID,
			WalletID:         wallet1.ID,
			ReceiverWalletID: &rec1RW.ID,
			Status:           data.SuccessMessageStatus,
			CreatedAt:        time.Now().AddDate(0, 0, int(smsResendInterval*3)),
			UpdatedAt:        time.Now().AddDate(0, 0, int(smsResendInterval*3)),
		})

		err = s.SendInvite(ctx)
		require.NoError(t, err)

		receivers, err := models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiver1.ID}, wallet1.ID)
		require.NoError(t, err)
		require.Len(t, receivers, 1)
		assert.Equal(t, rec1RW.ID, receivers[0].ID)
		require.NotNil(t, receivers[0].InvitationSentAt)
		assert.Equal(t, invitationSentAt, *receivers[0].InvitationSentAt)
	})

	t.Run("doesn't resend invitation SMS when receiver is not in the resend period", func(t *testing.T) {
		s, err := NewSendReceiverWalletInviteService(models, messengerClientMock, anchorPlatformBaseSepURL, stellarSecretKey, 3, mockCrashTrackerClient)
		require.NoError(t, err)

		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllMessagesFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		rec1RW := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet1.ID, data.ReadyReceiversWalletStatus)
		data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet2.ID, data.RegisteredReceiversWalletStatus)

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Status:         data.ReadyPaymentStatus,
			Disbursement:   disbursement1,
			Asset:          *asset1,
			ReceiverWallet: rec1RW,
			Amount:         "1",
		})

		// Marking as sent
		var invitationSentAt time.Time
		const q = "UPDATE receiver_wallets SET invitation_sent_at = NOW() - interval '1 day' WHERE id = $1 RETURNING invitation_sent_at"
		err = dbConnectionPool.GetContext(ctx, &invitationSentAt, q, rec1RW.ID)
		require.NoError(t, err)

		// Set the SMS Resend Interval
		var smsResendInterval int64 = 2
		err = models.Organizations.Update(ctx, &data.OrganizationUpdate{SMSResendInterval: &smsResendInterval})
		require.NoError(t, err)

		err = s.SendInvite(ctx)
		require.NoError(t, err)

		receivers, err := models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiver1.ID}, wallet1.ID)
		require.NoError(t, err)
		require.Len(t, receivers, 1)
		assert.Equal(t, rec1RW.ID, receivers[0].ID)
		require.NotNil(t, receivers[0].InvitationSentAt)
		assert.Equal(t, invitationSentAt, *receivers[0].InvitationSentAt)
	})

	t.Run("successfully resend the invitation SMS", func(t *testing.T) {
		s, err := NewSendReceiverWalletInviteService(models, messengerClientMock, anchorPlatformBaseSepURL, stellarSecretKey, 3, mockCrashTrackerClient)
		require.NoError(t, err)

		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllMessagesFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		rec1RW := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet1.ID, data.ReadyReceiversWalletStatus)
		data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet2.ID, data.RegisteredReceiversWalletStatus)

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Status:         data.ReadyPaymentStatus,
			Disbursement:   disbursement1,
			Asset:          *asset1,
			ReceiverWallet: rec1RW,
			Amount:         "1",
		})

		// Marking as sent
		var invitationSentAt time.Time
		q := "UPDATE receiver_wallets SET invitation_sent_at = NOW() - interval '2 days' - interval '3 hours' WHERE id = $1 RETURNING invitation_sent_at"
		err = dbConnectionPool.GetContext(ctx, &invitationSentAt, q, rec1RW.ID)
		require.NoError(t, err)

		// Set the SMS Resend Interval
		var smsResendInterval int64 = 2
		err = models.Organizations.Update(ctx, &data.OrganizationUpdate{SMSResendInterval: &smsResendInterval, SMSRegistrationMessageTemplate: new(string)})
		require.NoError(t, err)

		walletDeepLink1 := WalletDeepLink{
			DeepLink:                 wallet1.DeepLinkSchema,
			AnchorPlatformBaseSepURL: anchorPlatformBaseSepURL,
			OrganizationName:         "MyCustomAid",
			AssetCode:                asset1.Code,
			AssetIssuer:              asset1.Issuer,
		}
		deepLink1, err := walletDeepLink1.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		contentWallet1 := fmt.Sprintf("You have a payment waiting for you from the MyCustomAid. Click %s to register.", deepLink1)

		messengerClientMock.
			On("SendMessage", message.Message{
				ToPhoneNumber: receiver1.PhoneNumber,
				Message:       contentWallet1,
			}).
			Return(nil).
			Once()

		err = s.SendInvite(ctx)
		require.NoError(t, err)

		receivers, err := models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiver1.ID}, wallet1.ID)
		require.NoError(t, err)
		require.Len(t, receivers, 1)
		assert.Equal(t, rec1RW.ID, receivers[0].ID)
		require.NotNil(t, receivers[0].InvitationSentAt)
		assert.Equal(t, invitationSentAt, *receivers[0].InvitationSentAt)

		q = `
			SELECT
				type, status, receiver_id, wallet_id, receiver_wallet_id,
				title_encrypted, text_encrypted, status_history
			FROM
				messages
			WHERE
				receiver_id = $1 AND wallet_id = $2 AND
				receiver_wallet_id = $3 AND created_at > $4
		`
		var msg data.Message
		err = dbConnectionPool.GetContext(ctx, &msg, q, receiver1.ID, wallet1.ID, rec1RW.ID, invitationSentAt)
		require.NoError(t, err)

		assert.Equal(t, message.MessengerTypeTwilioSMS, msg.Type)
		assert.Equal(t, receiver1.ID, msg.ReceiverID)
		assert.Equal(t, wallet1.ID, msg.WalletID)
		assert.Equal(t, rec1RW.ID, *msg.ReceiverWalletID)
		assert.Equal(t, data.SuccessMessageStatus, msg.Status)
		assert.Empty(t, msg.TitleEncrypted)
		assert.Equal(t, contentWallet1, msg.TextEncrypted)
		assert.Len(t, msg.StatusHistory, 2)
		assert.Equal(t, data.PendingMessageStatus, msg.StatusHistory[0].Status)
		assert.Equal(t, data.SuccessMessageStatus, msg.StatusHistory[1].Status)
		assert.Nil(t, msg.AssetID)
	})

	t.Run("send disbursement invite successfully", func(t *testing.T) {
		disbursement3 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Country:                        country,
			Wallet:                         wallet1,
			Status:                         data.ReadyDisbursementStatus,
			Asset:                          asset1,
			SMSRegistrationMessageTemplate: "SMS Registration Message template test disbursement 3:",
		})

		disbursement4 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Country:                        country,
			Wallet:                         wallet2,
			Status:                         data.ReadyDisbursementStatus,
			Asset:                          asset2,
			SMSRegistrationMessageTemplate: "SMS Registration Message template test disbursement 4:",
		})

		s, err := NewSendReceiverWalletInviteService(models, messengerClientMock, anchorPlatformBaseSepURL, stellarSecretKey, 3, mockCrashTrackerClient)
		require.NoError(t, err)

		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllMessagesFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		rec1RW := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet1.ID, data.ReadyReceiversWalletStatus)
		data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet2.ID, data.RegisteredReceiversWalletStatus)

		rec2RW := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet2.ID, data.ReadyReceiversWalletStatus)

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Status:         data.ReadyPaymentStatus,
			Disbursement:   disbursement3,
			Asset:          *asset1,
			ReceiverWallet: rec1RW,
			Amount:         "1",
		})

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Status:         data.ReadyPaymentStatus,
			Disbursement:   disbursement4,
			Asset:          *asset2,
			ReceiverWallet: rec2RW,
			Amount:         "1",
		})

		walletDeepLink1 := WalletDeepLink{
			DeepLink:                 wallet1.DeepLinkSchema,
			AnchorPlatformBaseSepURL: anchorPlatformBaseSepURL,
			OrganizationName:         "MyCustomAid",
			AssetCode:                asset1.Code,
			AssetIssuer:              asset1.Issuer,
		}
		deepLink1, err := walletDeepLink1.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		contentDisbursement3 := fmt.Sprintf("%s %s", disbursement3.SMSRegistrationMessageTemplate, deepLink1)

		walletDeepLink2 := WalletDeepLink{
			DeepLink:                 wallet2.DeepLinkSchema,
			AnchorPlatformBaseSepURL: anchorPlatformBaseSepURL,
			OrganizationName:         "MyCustomAid",
			AssetCode:                asset2.Code,
			AssetIssuer:              asset2.Issuer,
		}
		deepLink2, err := walletDeepLink2.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		contentDisbursement4 := fmt.Sprintf("%s %s", disbursement4.SMSRegistrationMessageTemplate, deepLink2)

		messengerClientMock.
			On("SendMessage", message.Message{
				ToPhoneNumber: receiver1.PhoneNumber,
				Message:       contentDisbursement3,
			}).
			Return(nil).
			Once().
			On("SendMessage", message.Message{
				ToPhoneNumber: receiver2.PhoneNumber,
				Message:       contentDisbursement4,
			}).
			Return(nil).
			Once()

		err = s.SendInvite(ctx)
		require.NoError(t, err)

		receivers, err := models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiver1.ID}, wallet1.ID)
		require.NoError(t, err)
		require.Len(t, receivers, 1)
		assert.Equal(t, rec1RW.ID, receivers[0].ID)
		assert.NotNil(t, receivers[0].InvitationSentAt)

		receivers, err = models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiver2.ID}, wallet2.ID)
		require.NoError(t, err)
		require.Len(t, receivers, 1)
		assert.Equal(t, rec2RW.ID, receivers[0].ID)
		assert.NotNil(t, receivers[0].InvitationSentAt)

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
		assert.Equal(t, data.SuccessMessageStatus, msg.Status)
		assert.Empty(t, msg.TitleEncrypted)
		assert.Equal(t, contentDisbursement3, msg.TextEncrypted)
		assert.Len(t, msg.StatusHistory, 2)
		assert.Equal(t, data.PendingMessageStatus, msg.StatusHistory[0].Status)
		assert.Equal(t, data.SuccessMessageStatus, msg.StatusHistory[1].Status)
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
		assert.Equal(t, contentDisbursement4, msg.TextEncrypted)
		assert.Len(t, msg.StatusHistory, 2)
		assert.Equal(t, data.PendingMessageStatus, msg.StatusHistory[0].Status)
		assert.Equal(t, data.SuccessMessageStatus, msg.StatusHistory[1].Status)
		assert.Nil(t, msg.AssetID)
	})

	t.Run("successfully resend the disbursement invitation SMS", func(t *testing.T) {
		disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Country:                        country,
			Wallet:                         wallet1,
			Status:                         data.ReadyDisbursementStatus,
			Asset:                          asset1,
			SMSRegistrationMessageTemplate: "SMS Registration Message template test disbursement:",
		})

		s, err := NewSendReceiverWalletInviteService(models, messengerClientMock, anchorPlatformBaseSepURL, stellarSecretKey, 3, mockCrashTrackerClient)
		require.NoError(t, err)

		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllMessagesFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		rec1RW := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet1.ID, data.ReadyReceiversWalletStatus)
		data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet2.ID, data.RegisteredReceiversWalletStatus)

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Status:         data.ReadyPaymentStatus,
			Disbursement:   disbursement,
			Asset:          *asset1,
			ReceiverWallet: rec1RW,
			Amount:         "1",
		})

		// Marking as sent
		var invitationSentAt time.Time
		q := "UPDATE receiver_wallets SET invitation_sent_at = NOW() - interval '2 days' - interval '3 hours' WHERE id = $1 RETURNING invitation_sent_at"
		err = dbConnectionPool.GetContext(ctx, &invitationSentAt, q, rec1RW.ID)
		require.NoError(t, err)

		// Set the SMS Resend Interval
		var smsResendInterval int64 = 2
		err = models.Organizations.Update(ctx, &data.OrganizationUpdate{SMSResendInterval: &smsResendInterval, SMSRegistrationMessageTemplate: new(string)})
		require.NoError(t, err)

		walletDeepLink1 := WalletDeepLink{
			DeepLink:                 wallet1.DeepLinkSchema,
			AnchorPlatformBaseSepURL: anchorPlatformBaseSepURL,
			OrganizationName:         "MyCustomAid",
			AssetCode:                asset1.Code,
			AssetIssuer:              asset1.Issuer,
		}
		deepLink1, err := walletDeepLink1.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		contentDisbursement := fmt.Sprintf("%s %s", disbursement.SMSRegistrationMessageTemplate, deepLink1)

		messengerClientMock.
			On("SendMessage", message.Message{
				ToPhoneNumber: receiver1.PhoneNumber,
				Message:       contentDisbursement,
			}).
			Return(nil).
			Once()

		err = s.SendInvite(ctx)
		require.NoError(t, err)

		receivers, err := models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiver1.ID}, wallet1.ID)
		require.NoError(t, err)
		require.Len(t, receivers, 1)
		assert.Equal(t, rec1RW.ID, receivers[0].ID)
		require.NotNil(t, receivers[0].InvitationSentAt)
		assert.Equal(t, invitationSentAt, *receivers[0].InvitationSentAt)

		q = `
			SELECT
				type, status, receiver_id, wallet_id, receiver_wallet_id,
				title_encrypted, text_encrypted, status_history
			FROM
				messages
			WHERE
				receiver_id = $1 AND wallet_id = $2 AND
				receiver_wallet_id = $3 AND created_at > $4
		`
		var msg data.Message
		err = dbConnectionPool.GetContext(ctx, &msg, q, receiver1.ID, wallet1.ID, rec1RW.ID, invitationSentAt)
		require.NoError(t, err)

		assert.Equal(t, message.MessengerTypeTwilioSMS, msg.Type)
		assert.Equal(t, receiver1.ID, msg.ReceiverID)
		assert.Equal(t, wallet1.ID, msg.WalletID)
		assert.Equal(t, rec1RW.ID, *msg.ReceiverWalletID)
		assert.Equal(t, data.SuccessMessageStatus, msg.Status)
		assert.Empty(t, msg.TitleEncrypted)
		assert.Equal(t, contentDisbursement, msg.TextEncrypted)
		assert.Len(t, msg.StatusHistory, 2)
		assert.Equal(t, data.PendingMessageStatus, msg.StatusHistory[0].Status)
		assert.Equal(t, data.SuccessMessageStatus, msg.StatusHistory[1].Status)
		assert.Nil(t, msg.AssetID)
	})

	messengerClientMock.AssertExpectations(t)
}

func Test_SendReceiverWalletInviteService_shouldSendInvitationSMS(t *testing.T) {
	var maxInvitationSMSResendAttempts int64 = 3
	s := SendReceiverWalletInviteService{maxInvitationSMSResendAttempts: maxInvitationSMSResendAttempts}
	ctx := context.Background()

	t.Run("returns true when user never received the invitation SMS", func(t *testing.T) {
		org := data.Organization{SMSResendInterval: nil}
		rwa := data.ReceiverWalletAsset{
			ReceiverWallet: data.ReceiverWallet{
				InvitationSentAt: nil,
			},
		}
		got := s.shouldSendInvitationSMS(ctx, &org, &rwa)
		assert.True(t, got)
	})

	t.Run("returns false when user received the invitation SMS and organization's SMS Resend Interval is not set", func(t *testing.T) {
		invitationSentAt := time.Now()
		org := data.Organization{SMSResendInterval: nil}
		rwa := data.ReceiverWalletAsset{
			ReceiverWallet: data.ReceiverWallet{
				InvitationSentAt: &invitationSentAt,
			},
		}
		got := s.shouldSendInvitationSMS(ctx, &org, &rwa)
		assert.False(t, got)
	})

	t.Run("returns false when receiver reached the maximum number of SMS resend attempts", func(t *testing.T) {
		var smsResendInterval int64 = 2
		invitationSentAt := time.Now()
		org := data.Organization{SMSResendInterval: &smsResendInterval}
		rwa := data.ReceiverWalletAsset{
			ReceiverWallet: data.ReceiverWallet{
				InvitationSentAt: &invitationSentAt,
				Receiver: data.Receiver{
					ID:          "receiver-ID",
					PhoneNumber: "+123456789",
				},
				ReceiverWalletStats: data.ReceiverWalletStats{
					TotalInvitationSMSResentAttempts: maxInvitationSMSResendAttempts,
				},
			},
			WalletID: "wallet-ID",
		}

		getEntries := log.DefaultLogger.StartTest(log.DebugLevel)

		got := s.shouldSendInvitationSMS(ctx, &org, &rwa)
		assert.False(t, got)

		entries := getEntries()
		require.Len(t, entries, 1)
		assert.Equal(
			t,
			"the invitation message was not resent to the receiver because the maximum number of SMS resend attempts has been reached: Phone Number: +12...789 - Receiver ID receiver-ID - Wallet ID wallet-ID - Total Invitation SMS resent 3 - Maximum attempts 3",
			entries[0].Message,
		)
	})

	t.Run("returns false when the receiver is not in the period to resend the SMS", func(t *testing.T) {
		var smsResendInterval int64 = 2
		invitationSentAt := time.Now().AddDate(0, 0, -int(smsResendInterval-1))
		org := data.Organization{SMSResendInterval: &smsResendInterval}
		rwa := data.ReceiverWalletAsset{
			ReceiverWallet: data.ReceiverWallet{
				InvitationSentAt: &invitationSentAt,
				Receiver: data.Receiver{
					ID:          "receiver-ID",
					PhoneNumber: "+123456789",
				},
				ReceiverWalletStats: data.ReceiverWalletStats{
					TotalInvitationSMSResentAttempts: 1,
				},
			},
			WalletID: "wallet-ID",
		}

		getEntries := log.DefaultLogger.StartTest(log.DebugLevel)

		got := s.shouldSendInvitationSMS(ctx, &org, &rwa)
		assert.False(t, got)

		entries := getEntries()
		require.Len(t, entries, 1)
		assert.Equal(
			t,
			fmt.Sprintf(
				"the invitation message was not automatically resent to the receiver because the receiver is not in the resend period: Phone Number: +12...789 - Receiver ID receiver-ID - Wallet ID wallet-ID - Last Invitation Sent At %s - SMS Resend Interval 2 day(s)",
				invitationSentAt.Format(time.RFC1123),
			),
			entries[0].Message,
		)
	})

	t.Run("returns true when receiver meets the requirements to resend SMS", func(t *testing.T) {
		var smsResendInterval int64 = 2

		// 2 days after receiving the first invitation
		invitationSentAt := time.Now().Add((-25 * 2) * time.Hour)
		org := data.Organization{SMSResendInterval: &smsResendInterval}
		rwa := data.ReceiverWalletAsset{
			ReceiverWallet: data.ReceiverWallet{
				InvitationSentAt: &invitationSentAt,
				ReceiverWalletStats: data.ReceiverWalletStats{
					TotalInvitationSMSResentAttempts: 0,
				},
			},
		}
		got := s.shouldSendInvitationSMS(ctx, &org, &rwa)
		assert.True(t, got)

		// 4 days after receiving the first invitation
		invitationSentAt = invitationSentAt.AddDate(0, 0, -int(smsResendInterval))
		rwa = data.ReceiverWalletAsset{
			ReceiverWallet: data.ReceiverWallet{
				InvitationSentAt: &invitationSentAt,
				ReceiverWalletStats: data.ReceiverWalletStats{
					TotalInvitationSMSResentAttempts: 1,
				},
			},
		}
		got = s.shouldSendInvitationSMS(ctx, &org, &rwa)
		assert.True(t, got)

		// 6 days after receiving the first invitation
		invitationSentAt = invitationSentAt.AddDate(0, 0, -int(smsResendInterval))
		rwa = data.ReceiverWalletAsset{
			ReceiverWallet: data.ReceiverWallet{
				InvitationSentAt: &invitationSentAt,
				ReceiverWalletStats: data.ReceiverWalletStats{
					TotalInvitationSMSResentAttempts: 2,
				},
			},
		}
		got = s.shouldSendInvitationSMS(ctx, &org, &rwa)
		assert.True(t, got)

		// 8 days after receiving the first invitation - we don't resend because it reached the maximum number of attempts
		invitationSentAt = invitationSentAt.AddDate(0, 0, -int(smsResendInterval))
		rwa = data.ReceiverWalletAsset{
			ReceiverWallet: data.ReceiverWallet{
				InvitationSentAt: &invitationSentAt,
				ReceiverWalletStats: data.ReceiverWalletStats{
					TotalInvitationSMSResentAttempts: 3,
				},
			},
		}
		got = s.shouldSendInvitationSMS(ctx, &org, &rwa)
		assert.False(t, got)
	})
}

func Test_WalletDeepLink_isNativeAsset(t *testing.T) {
	testCases := []struct {
		name        string
		assetCode   string
		assetIssuer string
		wantResult  bool
	}{
		{
			name:        "🟢 XLM without issuer should be native asset",
			assetCode:   "XLM",
			assetIssuer: "",
			wantResult:  true,
		},
		{
			name:        "🟢 xLm without issuer should be native asset (case insensitive)",
			assetCode:   "XLM",
			assetIssuer: "",
			wantResult:  true,
		},
		{
			name:        "🔴 XLM with issuer should NOT be native asset",
			assetCode:   "XLM",
			assetIssuer: "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX",
			wantResult:  false,
		},
		{
			name:        "🟢 native without issuer should be native asset",
			assetCode:   "native",
			assetIssuer: "",
			wantResult:  true,
		},
		{
			name:        "🟢 NaTiVe without issuer should be native asset (case insensitive)",
			assetCode:   "NaTiVe",
			assetIssuer: "",
			wantResult:  true,
		},
		{
			name:        "🔴 native with issuer should NOT be native asset",
			assetCode:   "native",
			assetIssuer: "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX",
			wantResult:  false,
		},
		{
			name:        "🔴 USDC with issuer should NOT be native asset",
			assetCode:   "USDC",
			assetIssuer: "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX",
			wantResult:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wdl := WalletDeepLink{
				AssetCode:   tc.assetCode,
				AssetIssuer: tc.assetIssuer,
			}

			gotResult := wdl.isNativeAsset()
			assert.Equal(t, tc.wantResult, gotResult)
		})
	}
}

func Test_WalletDeepLink_assetName(t *testing.T) {
	testCases := []struct {
		name        string
		assetCode   string
		assetIssuer string
		wantResult  string
	}{
		{
			name:        "'XLM' native asset",
			assetCode:   "XLM",
			assetIssuer: "",
			wantResult:  "native",
		},
		{
			name:        "'XLM' with an issuer",
			assetCode:   "XLM",
			assetIssuer: "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX",
			wantResult:  "XLM-GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX",
		},
		{
			name:        "'native' native asset",
			assetCode:   "native",
			assetIssuer: "",
			wantResult:  "native",
		},
		{
			name:        "'native' with an issuer",
			assetCode:   "native",
			assetIssuer: "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX",
			wantResult:  "native-GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX",
		},
		{
			name:        "USDC",
			assetCode:   "USDC",
			assetIssuer: "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX",
			wantResult:  "USDC-GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wdl := WalletDeepLink{
				AssetCode:   tc.assetCode,
				AssetIssuer: tc.assetIssuer,
			}

			gotResult := wdl.assetName()
			assert.Equal(t, tc.wantResult, gotResult)
		})
	}
}

func Test_WalletDeepLink_BaseURLWithRoute(t *testing.T) {
	testCases := []struct {
		name       string
		deepLink   string
		route      string
		wantResult string
		wantErr    error
	}{
		{
			name:    "empty deep link and route",
			wantErr: fmt.Errorf("DeepLink can't be empty"),
		},
		{
			name:       "deep link with path [without schema] (empty route param)",
			deepLink:   "api-dev.vibrantapp.com",
			wantResult: "https://api-dev.vibrantapp.com",
			wantErr:    nil,
		},
		{
			name:       "deep link with path [without schema] (with route param)",
			deepLink:   "api-dev.vibrantapp.com",
			route:      "foo",
			wantResult: "https://api-dev.vibrantapp.com/foo",
			wantErr:    nil,
		},
		{
			name:       "deep link with path [without schema] {embedded route} (empty route param)",
			deepLink:   "api-dev.vibrantapp.com/foo",
			wantResult: "https://api-dev.vibrantapp.com/foo",
			wantErr:    nil,
		},
		{
			name:       "deep link with path [without schema] {embedded route} (with route param)",
			deepLink:   "api-dev.vibrantapp.com/foo",
			route:      "bar",
			wantResult: "https://api-dev.vibrantapp.com/foo/bar",
			wantErr:    nil,
		},
		{
			name:       "deep link with path [with schema] (empty route param)",
			deepLink:   "https://api-dev.vibrantapp.com",
			wantResult: "https://api-dev.vibrantapp.com",
			wantErr:    nil,
		},
		{
			name:       "deep link with path [with schema] (with route param)",
			deepLink:   "https://api-dev.vibrantapp.com",
			route:      "foo",
			wantResult: "https://api-dev.vibrantapp.com/foo",
			wantErr:    nil,
		},
		{
			name:       "deep link with path [with schema] {embedded route} (empty route param)",
			deepLink:   "https://api-dev.vibrantapp.com/foo",
			wantResult: "https://api-dev.vibrantapp.com/foo",
			wantErr:    nil,
		},
		{
			name:       "deep link with path [with schema] {embedded route} (with route param)",
			deepLink:   "https://api-dev.vibrantapp.com/foo",
			route:      "bar",
			wantResult: "https://api-dev.vibrantapp.com/foo/bar",
			wantErr:    nil,
		},
		{
			name:       "deep link with path [with schema] {embedded route}",
			deepLink:   "https://api-dev.vibrantapp.com/foo",
			route:      "bar",
			wantResult: "https://api-dev.vibrantapp.com/foo/bar",
			wantErr:    nil,
		},
		{
			name:     "deep link without path [ONLY schema]",
			deepLink: "vibrant+aid://",
			wantErr:  fmt.Errorf("the deep link needs to have a valid host, or path, or route"),
		},
		{
			name:       "deep link with path [with schema] {embedded route} (with route param)",
			deepLink:   "vibrant+aid://foo",
			wantResult: "vibrant+aid://foo",
			wantErr:    nil,
		},
		{
			name:       "deep link with path [with schema] {embedded route} (with route param)",
			deepLink:   "vibrant+aid://foo",
			route:      "bar",
			wantResult: "vibrant+aid://foo/bar",
			wantErr:    nil,
		},
		{
			name:       "deep link [with query params]",
			deepLink:   "vibrant+aid://foo?redirect=true",
			wantResult: "vibrant+aid://foo?redirect=true",
			wantErr:    nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wdl := WalletDeepLink{
				DeepLink: tc.deepLink,
				Route:    tc.route,
			}

			gotBaseURLWithRoute, err := wdl.BaseURLWithRoute()
			assert.Equal(t, tc.wantErr, err)
			assert.Equal(t, tc.wantResult, gotBaseURLWithRoute)
		})
	}
}

func Test_WalletDeepLink_TomlFileDomain(t *testing.T) {
	testCases := []struct {
		link       string
		wantResult string
		wantErr    error
	}{
		{
			link:       "",
			wantResult: "",
			wantErr:    fmt.Errorf("AnchorPlatformBaseSepURL can't be empty"),
		},
		{
			link:       "test.com",
			wantResult: "test.com",
			wantErr:    nil,
		},
		{
			link:       "https://test.com",
			wantResult: "test.com",
			wantErr:    nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.link, func(t *testing.T) {
			wdl := WalletDeepLink{
				AnchorPlatformBaseSepURL: tc.link,
			}

			result, err := wdl.TomlFileDomain()
			require.Equal(t, tc.wantErr, err)
			require.Equal(t, tc.wantResult, result)
		})
	}
}

func Test_WalletDeepLink_validate(t *testing.T) {
	// wallet schema can't be empty
	wdl := WalletDeepLink{}
	err := wdl.validate()
	require.EqualError(t, err, "wallet schema can't be empty")

	// we need a host, a path or a route
	wdl.DeepLink = "wallet://"
	err = wdl.validate()
	require.EqualError(t, err, "can't generate a valid base URL for the deep link: the deep link needs to have a valid host, or path, or route")

	// toml file domain can't be empty
	wdl.DeepLink = "wallet://sdp"
	err = wdl.validate()
	require.EqualError(t, err, "toml file domain can't be empty")

	// toml file domain can't be empty (different setup)
	wdl.DeepLink = "wallet://"
	wdl.Route = "sdp"
	err = wdl.validate()
	require.EqualError(t, err, "toml file domain can't be empty")

	// organization name can't be empty
	wdl.AnchorPlatformBaseSepURL = "foo.bar"
	err = wdl.validate()
	require.EqualError(t, err, "organization name can't be empty")

	// asset code can't be empty
	wdl.OrganizationName = "Foo Bar Org"
	err = wdl.validate()
	require.EqualError(t, err, "asset code can't be empty")

	// asset issuer can't be empty if it's not native (XLM)
	wdl.AssetCode = "FOO"
	err = wdl.validate()
	require.EqualError(t, err, "asset issuer can't be empty unless the asset code is XLM")

	// asset issuer needs to be a valid Ed25519PublicKey
	wdl.AssetIssuer = "BAR"
	err = wdl.validate()
	require.EqualError(t, err, "asset issuer is not a valid Ed25519 public key BAR")

	// Successful for non-native assets 🎉
	wdl.AssetIssuer = "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX"
	err = wdl.validate()
	require.NoError(t, err)

	// Successful for native (XLM) assets 🎉
	wdl.AssetCode = "XLM"
	wdl.AssetIssuer = ""
	err = wdl.validate()
	require.NoError(t, err)
}

func Test_WalletDeepLink_GetUnsignedRegistrationLink(t *testing.T) {
	testCases := []struct {
		name            string
		walletDeepLink  WalletDeepLink
		wantResult      string
		wantErrContains string
	}{
		{
			name:            "returns error if WalletDeepLink validation fails",
			wantErrContains: "validating WalletDeepLink: wallet schema can't be empty",
		},
		{
			name: "🎉 successful for non-native assets",
			walletDeepLink: WalletDeepLink{
				DeepLink:                 "wallet://",
				Route:                    "sdp", // route added separated from the deep link
				AnchorPlatformBaseSepURL: "foo.bar",
				OrganizationName:         "Foo Bar Org",
				AssetCode:                "FOO",
				AssetIssuer:              "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX",
			},
			wantResult: "wallet://sdp?asset=FOO-GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX&domain=foo.bar&name=Foo+Bar+Org",
		},
		{
			name: "🎉 successful for native (XLM) assets",
			walletDeepLink: WalletDeepLink{
				DeepLink:                 "wallet://sdp", // route added directly to the deep link
				AnchorPlatformBaseSepURL: "foo.bar",
				OrganizationName:         "Foo Bar Org",
				AssetCode:                "XLM",
			},
			wantResult: "wallet://sdp?asset=native&domain=foo.bar&name=Foo+Bar+Org",
		},
		{
			name: "🎉 successful for deeplink with query params",
			walletDeepLink: WalletDeepLink{
				DeepLink:                 "wallet://sdp?custom=true",
				AnchorPlatformBaseSepURL: "foo.bar",
				OrganizationName:         "Foo Bar Org",
				AssetCode:                "FOO",
				AssetIssuer:              "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX",
			},
			wantResult: "wallet://sdp?asset=FOO-GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX&custom=true&domain=foo.bar&name=Foo+Bar+Org",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotResult, err := tc.walletDeepLink.GetUnsignedRegistrationLink()
			if tc.wantErrContains != "" {
				assert.Empty(t, gotResult)
				assert.Contains(t, err.Error(), tc.wantErrContains)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantResult, gotResult)
			}
		})
	}
}

func Test_WalletDeepLink_GetSignedRegistrationLink(t *testing.T) {
	stellarPublicKey := "GBFDUUZ5ZYC6RAPOQLM7IYXLFHYTMCYXBGM7NIC4EE2MWOSGIYCOSN5F"
	stellarSecretKey := "SBUSPEKAZKLZSWHRSJ2HWDZUK6I3IVDUWA7JJZSGBLZ2WZIUJI7FPNB5"

	t.Run("fails if something is wrong with the WalletDeepLink object", func(t *testing.T) {
		wdl := WalletDeepLink{}
		actual, err := wdl.GetSignedRegistrationLink(stellarSecretKey)
		require.Empty(t, actual)
		require.EqualError(t, err, "error getting unsigned registration link: validating WalletDeepLink: wallet schema can't be empty")
	})

	t.Run("fails if the private key is invalid", func(t *testing.T) {
		wdl := WalletDeepLink{
			DeepLink:                 "wallet://",
			Route:                    "sdp",
			AnchorPlatformBaseSepURL: "foo.bar",
			OrganizationName:         "Foo Bar Org",
			AssetCode:                "FOO",
			AssetIssuer:              "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX",
		}

		actual, err := wdl.GetSignedRegistrationLink("invalid-secret-key")
		require.Empty(t, actual)
		require.EqualError(t, err, "error signing registration link: error parsing stellar private key: base32 decode failed: illegal base32 data at input byte 18")
	})

	t.Run("Successful for non-native assets 🎉", func(t *testing.T) {
		wdl := WalletDeepLink{
			DeepLink:                 "wallet://sdp",
			AnchorPlatformBaseSepURL: "foo.bar",
			OrganizationName:         "Foo Bar Org",
			AssetCode:                "FOO",
			AssetIssuer:              "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX",
		}

		expected := "wallet://sdp?asset=FOO-GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX&domain=foo.bar&name=Foo+Bar+Org&signature=361b0c0e6094dc35e0baa8ccae99bac1bdddc099e8bf6f68f4045e15b99c96d1a39c5343bb010a0b34f29a3490d233d43e3e2f5e537cf52d85f62deb75b2150d"
		actual, err := wdl.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		require.Equal(t, expected, actual)

		isValid, err := utils.VerifySignedURL(actual, stellarPublicKey)
		require.NoError(t, err)
		require.True(t, isValid)
	})

	t.Run("Successful for native (XLM) assets 🎉", func(t *testing.T) {
		wdl := WalletDeepLink{
			DeepLink:                 "wallet://",
			Route:                    "sdp",
			AnchorPlatformBaseSepURL: "foo.bar",
			OrganizationName:         "Foo Bar Org",
			AssetCode:                "XLM",
		}

		expected := "wallet://sdp?asset=native&domain=foo.bar&name=Foo+Bar+Org&signature=972a3012e18f107e0bf951f5acc757df953c3bbbe668a2d2652bf2445a759132f6af303df063f69d1a862b7ab419813554b201837795648f6175c9d9d72cf60f"
		actual, err := wdl.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		require.Equal(t, expected, actual)

		isValid, err := utils.VerifySignedURL(actual, stellarPublicKey)
		require.NoError(t, err)
		require.True(t, isValid)
	})

	t.Run("Successful for native (XLM) assets and AnchorPlatformBaseSepURL with https:// schema 🎉", func(t *testing.T) {
		wdl := WalletDeepLink{
			DeepLink:                 "wallet://sdp",
			AnchorPlatformBaseSepURL: "https://foo.bar",
			OrganizationName:         "Foo Bar Org",
			AssetCode:                "XLM",
		}

		expected := "wallet://sdp?asset=native&domain=foo.bar&name=Foo+Bar+Org&signature=972a3012e18f107e0bf951f5acc757df953c3bbbe668a2d2652bf2445a759132f6af303df063f69d1a862b7ab419813554b201837795648f6175c9d9d72cf60f"
		actual, err := wdl.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		require.Equal(t, expected, actual)

		isValid, err := utils.VerifySignedURL(actual, stellarPublicKey)
		require.NoError(t, err)
		require.True(t, isValid)
	})
}
