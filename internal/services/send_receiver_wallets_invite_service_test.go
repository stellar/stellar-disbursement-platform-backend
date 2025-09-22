package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_GetSignedRegistrationLink_SchemelessDeepLink(t *testing.T) {
	wdl := WalletDeepLink{
		DeepLink:         "api-dev.vibrantapp.com/sdp-dev",
		OrganizationName: "FOO Org",
		AssetCode:        "USDC",
		AssetIssuer:      "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
		TenantBaseURL:    "https://tenant.localhost.com",
	}

	registrationLink, err := wdl.GetSignedRegistrationLink("SCTOVDWM3A7KLTXXIV6YXL6QRVUIIG4HHHIDDKPR4JUB3DGDIKI5VGA2")
	require.NoError(t, err)
	wantRegistrationLink := "https://api-dev.vibrantapp.com/sdp-dev?asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5&domain=tenant.localhost.com&name=FOO+Org&signature=c6695a52ba8cc0ae2174023b116d4f726bc3d2c6d8d75a34336902ecbfa7eca07a059f44be503e3c4a71627aca66b05280b187e6614a0b130cf371328319ce0a"
	require.Equal(t, wantRegistrationLink, registrationLink)

	wdl = WalletDeepLink{
		DeepLink:         "https://www.beansapp.com/disbursements/registration?redirect=true",
		TenantBaseURL:    "https://tenant.localhost.com",
		OrganizationName: "FOO Org",
		AssetCode:        "USDC",
		AssetIssuer:      "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
	}

	registrationLink, err = wdl.GetSignedRegistrationLink("SCTOVDWM3A7KLTXXIV6YXL6QRVUIIG4HHHIDDKPR4JUB3DGDIKI5VGA2")
	require.NoError(t, err)
	wantRegistrationLink = "https://www.beansapp.com/disbursements/registration?asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5&domain=tenant.localhost.com&name=FOO+Org&redirect=true&signature=ab27744802e712716cc2c282cb08cb327f1ed75c334152879dd2b2d880eb0c5cf250deb8ae11510e1d4db00ee1f8c15bf940760464ae27a4140ecdc32304780d"
	require.Equal(t, wantRegistrationLink, registrationLink)
}

func Test_SendReceiverWalletInviteService_SendInvite(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	tenantBaseURL := "http://localhost:8000"
	tenantInfo := &schema.Tenant{ID: uuid.NewString(), Name: "TestTenant", BaseURL: &tenantBaseURL}
	ctx := sdpcontext.SetTenantInContext(context.Background(), tenantInfo)

	stellarSecretKey := "SBUSPEKAZKLZSWHRSJ2HWDZUK6I3IVDUWA7JJZSGBLZ2WZIUJI7FPNB5"
	messageDispatcherMock := message.NewMockMessageDispatcher(t)

	embeddedWalletServiceMock := mocks.NewMockEmbeddedWalletService(t)

	mockCrashTrackerClient := &crashtracker.MockCrashTrackerClient{}

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	wallet1 := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet1", "https://wallet1.com", "www.wallet1.com", "wallet1://sdp")
	wallet2 := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet2", "https://wallet2.com", "www.wallet2.com", "wallet2://sdp")

	walletEmbedded := data.CreateWalletFixture(t, ctx, dbConnectionPool, "EmbeddedWallet", tenantBaseURL, tenantBaseURL, "SELF")
	data.MakeWalletEmbedded(t, ctx, dbConnectionPool, walletEmbedded.ID)

	asset1 := data.CreateAssetFixture(t, ctx, dbConnectionPool, "FOO1", "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX")
	asset2 := data.CreateAssetFixture(t, ctx, dbConnectionPool, "FOO2", "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX")

	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverEmailOnly := data.InsertReceiverFixture(t, ctx, dbConnectionPool, &data.ReceiverInsert{
		Email: utils.StringPtr("emailJWP5O@randomemail.com"),
	})
	receiverPhoneOnly := data.InsertReceiverFixture(t, ctx, dbConnectionPool, &data.ReceiverInsert{
		PhoneNumber: utils.StringPtr("1234567890"),
	})

	disbursement1 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Wallet: wallet1,
		Status: data.ReadyDisbursementStatus,
		Asset:  asset1,
	})

	disbursement2 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Wallet: wallet2,
		Status: data.ReadyDisbursementStatus,
		Asset:  asset2,
	})

	disbursementEmbedded := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Wallet: walletEmbedded,
		Status: data.ReadyDisbursementStatus,
		Asset:  asset1,
	})

	t.Run("returns error when service has wrong setup", func(t *testing.T) {
		_, err := NewSendReceiverWalletInviteService(models, nil, nil, stellarSecretKey, 3, mockCrashTrackerClient)
		assert.EqualError(t, err, "invalid service setup: messenger dispatcher can't be nil")
	})

	t.Run("inserts the failed sent message", func(t *testing.T) {
		s, err := NewSendReceiverWalletInviteService(models, messageDispatcherMock, embeddedWalletServiceMock, stellarSecretKey, 3, mockCrashTrackerClient)
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
			DeepLink:         wallet1.DeepLinkSchema,
			TenantBaseURL:    tenantBaseURL,
			OrganizationName: "MyCustomAid",
			AssetCode:        asset1.Code,
			AssetIssuer:      asset1.Issuer,
		}
		deepLink1, err := walletDeepLink1.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		contentWallet1 := fmt.Sprintf("You have a payment waiting for you from the MyCustomAid. Click %s to register.", deepLink1)
		titleWallet1 := "You have a payment waiting for you from " + walletDeepLink1.OrganizationName

		walletDeepLink2 := WalletDeepLink{
			DeepLink:         wallet2.DeepLinkSchema,
			TenantBaseURL:    tenantBaseURL,
			OrganizationName: "MyCustomAid",
			AssetCode:        asset2.Code,
			AssetIssuer:      asset2.Issuer,
		}
		deepLink2, err := walletDeepLink2.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		contentWallet2 := fmt.Sprintf("You have a payment waiting for you from the MyCustomAid. Click %s to register.", deepLink2)
		titleWallet2 := "You have a payment waiting for you from " + walletDeepLink2.OrganizationName

		mockErr := errors.New("unexpected error")
		messageDispatcherMock.
			On("SendMessage", mock.Anything, message.Message{
				Type:          message.MessageTypeReceiverInvitation,
				ToPhoneNumber: receiver1.PhoneNumber,
				ToEmail:       receiver1.Email,
				Body:          contentWallet1,
				Title:         titleWallet1,
				TemplateVariables: map[string]string{
					"OrganizationName": walletDeepLink1.OrganizationName,
					"RegistrationLink": deepLink1,
				},
			}, []message.MessageChannel{message.MessageChannelSMS, message.MessageChannelEmail}).
			Return(message.MessengerTypeTwilioSMS, errors.New("unexpected error")).
			Once().
			On("SendMessage", mock.Anything, message.Message{
				Type:          message.MessageTypeReceiverInvitation,
				ToPhoneNumber: receiver2.PhoneNumber,
				ToEmail:       receiver2.Email,
				Body:          contentWallet2,
				Title:         titleWallet2,
				TemplateVariables: map[string]string{
					"OrganizationName": walletDeepLink2.OrganizationName,
					"RegistrationLink": deepLink2,
				},
			}, []message.MessageChannel{message.MessageChannelSMS, message.MessageChannelEmail}).
			Return(message.MessengerTypeTwilioSMS, nil).
			Once()

		mockMsg := fmt.Sprintf(
			"error sending message to receiver ID %s for receiver wallet ID %s using messenger type %s",
			receiver1.ID, rec1RW.ID, message.MessengerTypeTwilioSMS,
		)
		mockCrashTrackerClient.On("LogAndReportErrors", ctx, mockErr, mockMsg).Once()

		reqs := []schemas.EventReceiverWalletInvitationData{
			{
				ReceiverWalletID: rec1RW.ID,
			},
			{
				ReceiverWalletID: rec2RW.ID,
			},
		}

		err = s.SendInvite(ctx, reqs...)
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
		assert.Equal(t, titleWallet1, msg.TitleEncrypted)
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
		assert.Equal(t, titleWallet2, msg.TitleEncrypted)
		assert.Equal(t, contentWallet2, msg.TextEncrypted)
		assert.Len(t, msg.StatusHistory, 2)
		assert.Equal(t, data.PendingMessageStatus, msg.StatusHistory[0].Status)
		assert.Equal(t, data.SuccessMessageStatus, msg.StatusHistory[1].Status)
		assert.Nil(t, msg.AssetID)

		mockCrashTrackerClient.AssertExpectations(t)
	})

	t.Run("send invite successfully", func(t *testing.T) {
		s, err := NewSendReceiverWalletInviteService(models, messageDispatcherMock, embeddedWalletServiceMock, stellarSecretKey, 3, mockCrashTrackerClient)
		require.NoError(t, err)

		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllMessagesFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		rec1RW := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverPhoneOnly.ID, wallet1.ID, data.ReadyReceiversWalletStatus)
		data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverPhoneOnly.ID, wallet2.ID, data.RegisteredReceiversWalletStatus)

		rec2RW := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverEmailOnly.ID, wallet2.ID, data.ReadyReceiversWalletStatus)

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
			DeepLink:         wallet1.DeepLinkSchema,
			TenantBaseURL:    tenantBaseURL,
			OrganizationName: "MyCustomAid",
			AssetCode:        asset1.Code,
			AssetIssuer:      asset1.Issuer,
		}
		deepLink1, err := walletDeepLink1.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		contentWallet1 := fmt.Sprintf("You have a payment waiting for you from the MyCustomAid. Click %s to register.", deepLink1)
		// titleWallet1 := "You have a payment waiting for you from " + walletDeepLink1.OrganizationName

		walletDeepLink2 := WalletDeepLink{
			DeepLink:         wallet2.DeepLinkSchema,
			TenantBaseURL:    tenantBaseURL,
			OrganizationName: "MyCustomAid",
			AssetCode:        asset2.Code,
			AssetIssuer:      asset2.Issuer,
		}
		deepLink2, err := walletDeepLink2.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		contentWallet2 := fmt.Sprintf("You have a payment waiting for you from the MyCustomAid. Click %s to register.", deepLink2)
		titleWallet2 := "You have a payment waiting for you from " + walletDeepLink2.OrganizationName

		messageDispatcherMock.
			On("SendMessage", mock.Anything, message.Message{
				Type:          message.MessageTypeReceiverInvitation,
				ToPhoneNumber: receiverPhoneOnly.PhoneNumber,
				Body:          contentWallet1,
				TemplateVariables: map[string]string{
					"OrganizationName": walletDeepLink1.OrganizationName,
					"RegistrationLink": deepLink1,
				},
			}, []message.MessageChannel{message.MessageChannelSMS, message.MessageChannelEmail}).
			Return(message.MessengerTypeTwilioSMS, nil).
			Once().
			On("SendMessage", mock.Anything, message.Message{
				Type:    message.MessageTypeReceiverInvitation,
				ToEmail: receiverEmailOnly.Email,
				Body:    contentWallet2,
				Title:   titleWallet2,
				TemplateVariables: map[string]string{
					"OrganizationName": walletDeepLink2.OrganizationName,
					"RegistrationLink": deepLink2,
				},
			}, []message.MessageChannel{message.MessageChannelSMS, message.MessageChannelEmail}).
			Return(message.MessengerTypeAWSEmail, nil).
			Once()

		reqs := []schemas.EventReceiverWalletInvitationData{
			{
				ReceiverWalletID: rec1RW.ID,
			},
			{
				ReceiverWalletID: rec2RW.ID,
			},
		}

		err = s.SendInvite(ctx, reqs...)
		require.NoError(t, err)

		receivers, err := models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiverPhoneOnly.ID}, wallet1.ID)
		require.NoError(t, err)
		require.Len(t, receivers, 1)
		assert.Equal(t, rec1RW.ID, receivers[0].ID)
		assert.NotNil(t, receivers[0].InvitationSentAt)

		receivers, err = models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiverEmailOnly.ID}, wallet2.ID)
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
		err = dbConnectionPool.GetContext(ctx, &msg, q, receiverPhoneOnly.ID, wallet1.ID, rec1RW.ID)
		require.NoError(t, err)

		assert.Equal(t, message.MessengerTypeTwilioSMS, msg.Type)
		assert.Equal(t, receiverPhoneOnly.ID, msg.ReceiverID)
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
		err = dbConnectionPool.GetContext(ctx, &msg, q, receiverEmailOnly.ID, wallet2.ID, rec2RW.ID)
		require.NoError(t, err)

		assert.Equal(t, message.MessengerTypeAWSEmail, msg.Type)
		assert.Equal(t, receiverEmailOnly.ID, msg.ReceiverID)
		assert.Equal(t, wallet2.ID, msg.WalletID)
		assert.Equal(t, rec2RW.ID, *msg.ReceiverWalletID)
		assert.Equal(t, data.SuccessMessageStatus, msg.Status)
		assert.Equal(t, titleWallet2, msg.TitleEncrypted)
		assert.Equal(t, contentWallet2, msg.TextEncrypted)
		assert.Len(t, msg.StatusHistory, 2)
		assert.Equal(t, data.PendingMessageStatus, msg.StatusHistory[0].Status)
		assert.Equal(t, data.SuccessMessageStatus, msg.StatusHistory[1].Status)
		assert.Nil(t, msg.AssetID)
	})

	t.Run("send invite successfully with embedded wallet deep link", func(t *testing.T) {
		s, err := NewSendReceiverWalletInviteService(models, messageDispatcherMock, embeddedWalletServiceMock, stellarSecretKey, 3, mockCrashTrackerClient)
		require.NoError(t, err)

		mockToken := uuid.New().String()
		embeddedWalletServiceMock.On("CreateInvitationToken", mock.Anything).Return(mockToken, nil).Once()

		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllMessagesFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		recRW := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverPhoneOnly.ID, walletEmbedded.ID, data.ReadyReceiversWalletStatus)

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Status:         data.ReadyPaymentStatus,
			Disbursement:   disbursementEmbedded,
			Asset:          *asset1,
			ReceiverWallet: recRW,
			Amount:         "1",
		})

		messageDispatcherMock.
			On("SendMessage", mock.Anything, mock.MatchedBy(func(msg message.Message) bool {
				return msg.ToPhoneNumber == receiverPhoneOnly.PhoneNumber &&
					strings.Contains(msg.Body, "You have a payment waiting for you from the MyCustomAid") &&
					strings.Contains(msg.Body, "wallet?asset=FOO1-GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX") &&
					strings.Contains(msg.Body, "token=") &&
					strings.Contains(msg.Body, "signature=")
			}), []message.MessageChannel{message.MessageChannelSMS, message.MessageChannelEmail}).
			Return(message.MessengerTypeTwilioSMS, nil).
			Once()

		reqs := []schemas.EventReceiverWalletInvitationData{
			{
				ReceiverWalletID: recRW.ID,
			},
		}

		err = s.SendInvite(ctx, reqs...)
		require.NoError(t, err)

		receivers, err := models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiverPhoneOnly.ID}, walletEmbedded.ID)
		require.NoError(t, err)
		require.Len(t, receivers, 1)
		assert.Equal(t, recRW.ID, receivers[0].ID)
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
		err = dbConnectionPool.GetContext(ctx, &msg, q, receiverPhoneOnly.ID, walletEmbedded.ID, recRW.ID)
		require.NoError(t, err)

		assert.Equal(t, message.MessengerTypeTwilioSMS, msg.Type)
		assert.Equal(t, receiverPhoneOnly.ID, msg.ReceiverID)
		assert.Equal(t, walletEmbedded.ID, msg.WalletID)
		assert.Equal(t, recRW.ID, *msg.ReceiverWalletID)
		assert.Equal(t, data.SuccessMessageStatus, msg.Status)
		assert.Empty(t, msg.TitleEncrypted)

		assert.Contains(t, msg.TextEncrypted, "You have a payment waiting for you from the MyCustomAid")
		assert.Contains(t, msg.TextEncrypted, "wallet?asset=FOO1-GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX")
		assert.Contains(t, msg.TextEncrypted, "token=")
		assert.Contains(t, msg.TextEncrypted, "signature=")
		assert.Len(t, msg.StatusHistory, 2)
		assert.Equal(t, data.PendingMessageStatus, msg.StatusHistory[0].Status)
		assert.Equal(t, data.SuccessMessageStatus, msg.StatusHistory[1].Status)
		assert.Nil(t, msg.AssetID)
	})

	t.Run("skips embedded wallet when embedded wallet service is nil", func(t *testing.T) {
		s, err := NewSendReceiverWalletInviteService(models, messageDispatcherMock, nil, stellarSecretKey, 3, mockCrashTrackerClient)
		require.NoError(t, err)

		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllMessagesFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		recRW := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverPhoneOnly.ID, walletEmbedded.ID, data.ReadyReceiversWalletStatus)

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Status:         data.ReadyPaymentStatus,
			Disbursement:   disbursementEmbedded,
			Asset:          *asset1,
			ReceiverWallet: recRW,
			Amount:         "1",
		})

		reqs := []schemas.EventReceiverWalletInvitationData{
			{
				ReceiverWalletID: recRW.ID,
			},
		}

		err = s.SendInvite(ctx, reqs...)
		require.NoError(t, err)

		q := `SELECT COUNT(*) FROM messages WHERE receiver_id = $1 AND wallet_id = $2`
		var messageCount int
		err = dbConnectionPool.GetContext(ctx, &messageCount, q, receiverPhoneOnly.ID, walletEmbedded.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, messageCount)

		receivers, err := models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiverPhoneOnly.ID}, walletEmbedded.ID)
		require.NoError(t, err)
		require.Len(t, receivers, 1)
		assert.Nil(t, receivers[0].InvitationSentAt)
	})

	t.Run("send invite successfully with custom invite message", func(t *testing.T) {
		s, err := NewSendReceiverWalletInviteService(models, messageDispatcherMock, embeddedWalletServiceMock, stellarSecretKey, 3, mockCrashTrackerClient)
		require.NoError(t, err)

		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllMessagesFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		rec1RW := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet1.ID, data.ReadyReceiversWalletStatus)
		data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet2.ID, data.RegisteredReceiversWalletStatus)

		rec2RW := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet2.ID, data.ReadyReceiversWalletStatus)

		customInvitationMessage := "My custom receiver wallet registration invite. MyOrg 👋"
		err = models.Organizations.Update(ctx, &data.OrganizationUpdate{ReceiverRegistrationMessageTemplate: &customInvitationMessage})
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
			DeepLink:         wallet1.DeepLinkSchema,
			TenantBaseURL:    tenantBaseURL,
			OrganizationName: "MyCustomAid",
			AssetCode:        asset1.Code,
			AssetIssuer:      asset1.Issuer,
		}
		deepLink1, err := walletDeepLink1.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		contentWallet1 := fmt.Sprintf("%s %s", customInvitationMessage, deepLink1)
		titleWallet1 := "You have a payment waiting for you from " + walletDeepLink1.OrganizationName

		walletDeepLink2 := WalletDeepLink{
			DeepLink:         wallet2.DeepLinkSchema,
			TenantBaseURL:    tenantBaseURL,
			OrganizationName: "MyCustomAid",
			AssetCode:        asset2.Code,
			AssetIssuer:      asset2.Issuer,
		}
		deepLink2, err := walletDeepLink2.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		contentWallet2 := fmt.Sprintf("%s %s", customInvitationMessage, deepLink2)
		titleWallet2 := "You have a payment waiting for you from " + walletDeepLink2.OrganizationName

		messageDispatcherMock.
			On("SendMessage", mock.Anything, message.Message{
				Type:          message.MessageTypeReceiverInvitation,
				ToPhoneNumber: receiver1.PhoneNumber,
				ToEmail:       receiver1.Email,
				Body:          contentWallet1,
				Title:         titleWallet1,
				TemplateVariables: map[string]string{
					"OrganizationName": walletDeepLink1.OrganizationName,
					"RegistrationLink": deepLink1,
				},
			}, []message.MessageChannel{message.MessageChannelSMS, message.MessageChannelEmail}).
			Return(message.MessengerTypeTwilioSMS, nil).
			Once().
			On("SendMessage", mock.Anything, message.Message{
				Type:          message.MessageTypeReceiverInvitation,
				ToPhoneNumber: receiver2.PhoneNumber,
				ToEmail:       receiver2.Email,
				Body:          contentWallet2,
				Title:         titleWallet2,
				TemplateVariables: map[string]string{
					"OrganizationName": walletDeepLink2.OrganizationName,
					"RegistrationLink": deepLink2,
				},
			}, []message.MessageChannel{message.MessageChannelSMS, message.MessageChannelEmail}).
			Return(message.MessengerTypeTwilioSMS, nil).
			Once()

		reqs := []schemas.EventReceiverWalletInvitationData{
			{
				ReceiverWalletID: rec1RW.ID,
			},
			{
				ReceiverWalletID: rec2RW.ID,
			},
		}

		err = s.SendInvite(ctx, reqs...)
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
		assert.Equal(t, titleWallet1, msg.TitleEncrypted)
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
		assert.Equal(t, titleWallet1, msg.TitleEncrypted)
		assert.Equal(t, contentWallet2, msg.TextEncrypted)
		assert.Len(t, msg.StatusHistory, 2)
		assert.Equal(t, data.PendingMessageStatus, msg.StatusHistory[0].Status)
		assert.Equal(t, data.SuccessMessageStatus, msg.StatusHistory[1].Status)
		assert.Nil(t, msg.AssetID)
	})

	t.Run("doesn't resend the invitation SMS when organization's SMS Resend Interval is nil and the invitation was already sent", func(t *testing.T) {
		s, err := NewSendReceiverWalletInviteService(models, messageDispatcherMock, embeddedWalletServiceMock, stellarSecretKey, 3, mockCrashTrackerClient)
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

		err = models.Organizations.Update(ctx, &data.OrganizationUpdate{ReceiverInvitationResendIntervalDays: new(int64)})
		require.NoError(t, err)

		reqs := []schemas.EventReceiverWalletInvitationData{
			{
				ReceiverWalletID: rec1RW.ID,
			},
		}

		err = s.SendInvite(ctx, reqs...)
		require.NoError(t, err)

		receivers, err := models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiver1.ID}, wallet1.ID)
		require.NoError(t, err)
		require.Len(t, receivers, 1)
		assert.Equal(t, rec1RW.ID, receivers[0].ID)
		require.NotNil(t, receivers[0].InvitationSentAt)
		assert.Equal(t, invitationSentAt, *receivers[0].InvitationSentAt)
	})

	t.Run("doesn't resend the invitation SMS when receiver reached the maximum number of resend attempts", func(t *testing.T) {
		s, err := NewSendReceiverWalletInviteService(models, messageDispatcherMock, embeddedWalletServiceMock, stellarSecretKey, 3, mockCrashTrackerClient)
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
		err = models.Organizations.Update(ctx, &data.OrganizationUpdate{ReceiverInvitationResendIntervalDays: &smsResendInterval})
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

		reqs := []schemas.EventReceiverWalletInvitationData{
			{
				ReceiverWalletID: rec1RW.ID,
			},
		}

		err = s.SendInvite(ctx, reqs...)
		require.NoError(t, err)

		receivers, err := models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiver1.ID}, wallet1.ID)
		require.NoError(t, err)
		require.Len(t, receivers, 1)
		assert.Equal(t, rec1RW.ID, receivers[0].ID)
		require.NotNil(t, receivers[0].InvitationSentAt)
		assert.Equal(t, invitationSentAt, *receivers[0].InvitationSentAt)
	})

	t.Run("doesn't resend invitation SMS when receiver is not in the resend period", func(t *testing.T) {
		s, err := NewSendReceiverWalletInviteService(models, messageDispatcherMock, embeddedWalletServiceMock, stellarSecretKey, 3, mockCrashTrackerClient)
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
		err = models.Organizations.Update(ctx, &data.OrganizationUpdate{ReceiverInvitationResendIntervalDays: &smsResendInterval})
		require.NoError(t, err)

		reqs := []schemas.EventReceiverWalletInvitationData{
			{
				ReceiverWalletID: rec1RW.ID,
			},
		}

		err = s.SendInvite(ctx, reqs...)
		require.NoError(t, err)

		receivers, err := models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiver1.ID}, wallet1.ID)
		require.NoError(t, err)
		require.Len(t, receivers, 1)
		assert.Equal(t, rec1RW.ID, receivers[0].ID)
		require.NotNil(t, receivers[0].InvitationSentAt)
		assert.Equal(t, invitationSentAt, *receivers[0].InvitationSentAt)
	})

	t.Run("successfully resend the invitation SMS", func(t *testing.T) {
		s, err := NewSendReceiverWalletInviteService(models, messageDispatcherMock, embeddedWalletServiceMock, stellarSecretKey, 3, mockCrashTrackerClient)
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
		err = models.Organizations.Update(ctx, &data.OrganizationUpdate{ReceiverInvitationResendIntervalDays: &smsResendInterval, ReceiverRegistrationMessageTemplate: new(string)})
		require.NoError(t, err)

		walletDeepLink1 := WalletDeepLink{
			DeepLink:         wallet1.DeepLinkSchema,
			TenantBaseURL:    tenantBaseURL,
			OrganizationName: "MyCustomAid",
			AssetCode:        asset1.Code,
			AssetIssuer:      asset1.Issuer,
		}
		deepLink1, err := walletDeepLink1.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		contentWallet1 := fmt.Sprintf("You have a payment waiting for you from the MyCustomAid. Click %s to register.", deepLink1)
		titleWallet1 := "You have a payment waiting for you from " + walletDeepLink1.OrganizationName

		messageDispatcherMock.
			On("SendMessage", mock.Anything, message.Message{
				Type:          message.MessageTypeReceiverInvitation,
				ToPhoneNumber: receiver1.PhoneNumber,
				ToEmail:       receiver1.Email,
				Body:          contentWallet1,
				Title:         titleWallet1,
				TemplateVariables: map[string]string{
					"OrganizationName": walletDeepLink1.OrganizationName,
					"RegistrationLink": deepLink1,
				},
			}, []message.MessageChannel{message.MessageChannelSMS, message.MessageChannelEmail}).
			Return(message.MessengerTypeTwilioSMS, nil).
			Once()

		reqs := []schemas.EventReceiverWalletInvitationData{
			{
				ReceiverWalletID: rec1RW.ID,
			},
		}

		err = s.SendInvite(ctx, reqs...)
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
		assert.Equal(t, titleWallet1, msg.TitleEncrypted)
		assert.Equal(t, contentWallet1, msg.TextEncrypted)
		assert.Len(t, msg.StatusHistory, 2)
		assert.Equal(t, data.PendingMessageStatus, msg.StatusHistory[0].Status)
		assert.Equal(t, data.SuccessMessageStatus, msg.StatusHistory[1].Status)
		assert.Nil(t, msg.AssetID)
	})

	t.Run("send disbursement invite successfully", func(t *testing.T) {
		disbursement3 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Wallet:                              wallet1,
			Status:                              data.ReadyDisbursementStatus,
			Asset:                               asset1,
			ReceiverRegistrationMessageTemplate: "SMS Registration Message template test disbursement 3:",
		})

		disbursement4 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Wallet:                              wallet2,
			Status:                              data.ReadyDisbursementStatus,
			Asset:                               asset2,
			ReceiverRegistrationMessageTemplate: "SMS Registration Message template test disbursement 4:",
		})

		s, err := NewSendReceiverWalletInviteService(models, messageDispatcherMock, embeddedWalletServiceMock, stellarSecretKey, 3, mockCrashTrackerClient)
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
			DeepLink:         wallet1.DeepLinkSchema,
			TenantBaseURL:    tenantBaseURL,
			OrganizationName: "MyCustomAid",
			AssetCode:        asset1.Code,
			AssetIssuer:      asset1.Issuer,
		}
		deepLink1, err := walletDeepLink1.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		contentDisbursement3 := fmt.Sprintf("%s %s", disbursement3.ReceiverRegistrationMessageTemplate, deepLink1)
		titleDisbursement3 := "You have a payment waiting for you from " + walletDeepLink1.OrganizationName

		walletDeepLink2 := WalletDeepLink{
			DeepLink:         wallet2.DeepLinkSchema,
			TenantBaseURL:    tenantBaseURL,
			OrganizationName: "MyCustomAid",
			AssetCode:        asset2.Code,
			AssetIssuer:      asset2.Issuer,
		}
		deepLink2, err := walletDeepLink2.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		contentDisbursement4 := fmt.Sprintf("%s %s", disbursement4.ReceiverRegistrationMessageTemplate, deepLink2)
		titleDisbursement4 := "You have a payment waiting for you from " + walletDeepLink2.OrganizationName

		messageDispatcherMock.
			On("SendMessage", mock.Anything, message.Message{
				Type:          message.MessageTypeReceiverInvitation,
				ToPhoneNumber: receiver1.PhoneNumber,
				ToEmail:       receiver1.Email,
				Body:          contentDisbursement3,
				Title:         titleDisbursement3,
				TemplateVariables: map[string]string{
					"OrganizationName": walletDeepLink1.OrganizationName,
					"RegistrationLink": deepLink1,
				},
			}, []message.MessageChannel{message.MessageChannelSMS, message.MessageChannelEmail}).
			Return(message.MessengerTypeTwilioSMS, nil).
			Once().
			On("SendMessage", mock.Anything, message.Message{
				Type:          message.MessageTypeReceiverInvitation,
				ToPhoneNumber: receiver2.PhoneNumber,
				ToEmail:       receiver2.Email,
				Body:          contentDisbursement4,
				Title:         titleDisbursement4,
				TemplateVariables: map[string]string{
					"OrganizationName": walletDeepLink2.OrganizationName,
					"RegistrationLink": deepLink2,
				},
			}, []message.MessageChannel{message.MessageChannelSMS, message.MessageChannelEmail}).
			Return(message.MessengerTypeTwilioSMS, nil).
			Once()

		reqs := []schemas.EventReceiverWalletInvitationData{
			{
				ReceiverWalletID: rec1RW.ID,
			},
			{
				ReceiverWalletID: rec2RW.ID,
			},
		}

		err = s.SendInvite(ctx, reqs...)
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
		assert.Equal(t, titleDisbursement3, msg.TitleEncrypted)
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
		assert.Equal(t, titleDisbursement4, msg.TitleEncrypted)
		assert.Equal(t, contentDisbursement4, msg.TextEncrypted)
		assert.Len(t, msg.StatusHistory, 2)
		assert.Equal(t, data.PendingMessageStatus, msg.StatusHistory[0].Status)
		assert.Equal(t, data.SuccessMessageStatus, msg.StatusHistory[1].Status)
		assert.Nil(t, msg.AssetID)
	})

	t.Run("successfully resend the disbursement invitation SMS", func(t *testing.T) {
		disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Wallet:                              wallet1,
			Status:                              data.ReadyDisbursementStatus,
			Asset:                               asset1,
			ReceiverRegistrationMessageTemplate: "SMS Registration Message template test disbursement:",
		})

		s, err := NewSendReceiverWalletInviteService(models, messageDispatcherMock, embeddedWalletServiceMock, stellarSecretKey, 3, mockCrashTrackerClient)
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
		err = models.Organizations.Update(ctx, &data.OrganizationUpdate{ReceiverInvitationResendIntervalDays: &smsResendInterval, ReceiverRegistrationMessageTemplate: new(string)})
		require.NoError(t, err)

		walletDeepLink1 := WalletDeepLink{
			DeepLink:         wallet1.DeepLinkSchema,
			TenantBaseURL:    tenantBaseURL,
			OrganizationName: "MyCustomAid",
			AssetCode:        asset1.Code,
			AssetIssuer:      asset1.Issuer,
		}
		deepLink1, err := walletDeepLink1.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		contentDisbursement := fmt.Sprintf("%s %s", disbursement.ReceiverRegistrationMessageTemplate, deepLink1)
		titleDisbursement := "You have a payment waiting for you from " + walletDeepLink1.OrganizationName

		messageDispatcherMock.
			On("SendMessage", mock.Anything, message.Message{
				Type:          message.MessageTypeReceiverInvitation,
				ToPhoneNumber: receiver1.PhoneNumber,
				ToEmail:       receiver1.Email,
				Body:          contentDisbursement,
				Title:         titleDisbursement,
				TemplateVariables: map[string]string{
					"OrganizationName": walletDeepLink1.OrganizationName,
					"RegistrationLink": deepLink1,
				},
			}, []message.MessageChannel{message.MessageChannelSMS, message.MessageChannelEmail}).
			Return(message.MessengerTypeTwilioSMS, nil).
			Once()

		reqs := []schemas.EventReceiverWalletInvitationData{
			{
				ReceiverWalletID: rec1RW.ID,
			},
		}

		err = s.SendInvite(ctx, reqs...)
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
		assert.Equal(t, titleDisbursement, msg.TitleEncrypted)
		assert.Equal(t, contentDisbursement, msg.TextEncrypted)
		assert.Len(t, msg.StatusHistory, 2)
		assert.Equal(t, data.PendingMessageStatus, msg.StatusHistory[0].Status)
		assert.Equal(t, data.SuccessMessageStatus, msg.StatusHistory[1].Status)
		assert.Nil(t, msg.AssetID)
	})

	messageDispatcherMock.AssertExpectations(t)
}

func Test_SendReceiverWalletInviteService_shouldSendInvitation(t *testing.T) {
	var maxInvitationResendAttempts int64 = 3
	s := SendReceiverWalletInviteService{maxInvitationResendAttempts: maxInvitationResendAttempts}
	ctx := context.Background()

	t.Run("returns true when user never received the invitation SMS", func(t *testing.T) {
		org := data.Organization{ReceiverInvitationResendIntervalDays: nil}
		rwa := data.ReceiverWalletAsset{
			ReceiverWallet: data.ReceiverWallet{
				InvitationSentAt: nil,
				Receiver: data.Receiver{
					PhoneNumber: "+380443973607",
				},
			},
		}
		got := s.shouldSendInvitation(ctx, &org, &rwa)
		assert.True(t, got)
	})

	t.Run("returns false when user received the invitation SMS and organization's SMS Resend Interval is not set", func(t *testing.T) {
		invitationSentAt := time.Now()
		org := data.Organization{ReceiverInvitationResendIntervalDays: nil}
		rwa := data.ReceiverWalletAsset{
			ReceiverWallet: data.ReceiverWallet{
				InvitationSentAt: &invitationSentAt,
			},
		}
		got := s.shouldSendInvitation(ctx, &org, &rwa)
		assert.False(t, got)
	})

	t.Run("returns false when receiver reached the maximum number of message resend attempts", func(t *testing.T) {
		var msgResendInterval int64 = 2
		invitationSentAt := time.Now()
		org := data.Organization{ReceiverInvitationResendIntervalDays: &msgResendInterval}
		rwa := data.ReceiverWalletAsset{
			ReceiverWallet: data.ReceiverWallet{
				InvitationSentAt: &invitationSentAt,
				Receiver: data.Receiver{
					ID:          "receiver-ID",
					PhoneNumber: "+123456789",
				},
				ReceiverWalletStats: data.ReceiverWalletStats{
					TotalInvitationResentAttempts: maxInvitationResendAttempts,
				},
			},
			WalletID: "wallet-ID",
		}

		getEntries := log.DefaultLogger.StartTest(log.DebugLevel)

		got := s.shouldSendInvitation(ctx, &org, &rwa)
		assert.False(t, got)

		entries := getEntries()
		require.Len(t, entries, 1)
		assert.Equal(
			t,
			"the invitation message was not resent to the receiver because the maximum number of message resend attempts has been reached: Receiver ID receiver-ID - Wallet ID wallet-ID - Total Invitation resent 3 - Maximum attempts 3",
			entries[0].Message,
		)
	})

	t.Run("returns false when the receiver is not in the period to resend the message", func(t *testing.T) {
		var smsResendInterval int64 = 2
		invitationSentAt := time.Now().AddDate(0, 0, -int(smsResendInterval-1))
		org := data.Organization{ReceiverInvitationResendIntervalDays: &smsResendInterval}
		rwa := data.ReceiverWalletAsset{
			ReceiverWallet: data.ReceiverWallet{
				InvitationSentAt: &invitationSentAt,
				Receiver: data.Receiver{
					ID:          "receiver-ID",
					PhoneNumber: "+123456789",
				},
				ReceiverWalletStats: data.ReceiverWalletStats{
					TotalInvitationResentAttempts: 1,
				},
			},
			WalletID: "wallet-ID",
		}

		getEntries := log.DefaultLogger.StartTest(log.DebugLevel)

		got := s.shouldSendInvitation(ctx, &org, &rwa)
		assert.False(t, got)

		entries := getEntries()
		require.Len(t, entries, 1)
		assert.Equal(
			t,
			fmt.Sprintf(
				"the invitation message was not automatically resent to the receiver because the receiver is not in the resend period: Receiver ID receiver-ID - Wallet ID wallet-ID - Last Invitation Sent At %s - Receiver Invitation Resend Interval 2 day(s)",
				invitationSentAt.Format(time.RFC1123),
			),
			entries[0].Message,
		)
	})

	t.Run("returns true when receiver meets the requirements to resend SMS", func(t *testing.T) {
		var smsResendInterval int64 = 2

		// 2 days after receiving the first invitation
		invitationSentAt := time.Now().Add((-25 * 2) * time.Hour)
		org := data.Organization{ReceiverInvitationResendIntervalDays: &smsResendInterval}
		rwa := data.ReceiverWalletAsset{
			ReceiverWallet: data.ReceiverWallet{
				InvitationSentAt: &invitationSentAt,
				ReceiverWalletStats: data.ReceiverWalletStats{
					TotalInvitationResentAttempts: 0,
				},
				Receiver: data.Receiver{
					PhoneNumber: "+380443973607",
				},
			},
		}
		got := s.shouldSendInvitation(ctx, &org, &rwa)
		assert.True(t, got)

		// 4 days after receiving the first invitation
		invitationSentAt = invitationSentAt.AddDate(0, 0, -int(smsResendInterval))
		rwa = data.ReceiverWalletAsset{
			ReceiverWallet: data.ReceiverWallet{
				InvitationSentAt: &invitationSentAt,
				ReceiverWalletStats: data.ReceiverWalletStats{
					TotalInvitationResentAttempts: 1,
				},
				Receiver: data.Receiver{
					PhoneNumber: "+380443973607",
				},
			},
		}
		got = s.shouldSendInvitation(ctx, &org, &rwa)
		assert.True(t, got)

		// 6 days after receiving the first invitation
		invitationSentAt = invitationSentAt.AddDate(0, 0, -int(smsResendInterval))
		rwa = data.ReceiverWalletAsset{
			ReceiverWallet: data.ReceiverWallet{
				InvitationSentAt: &invitationSentAt,
				ReceiverWalletStats: data.ReceiverWalletStats{
					TotalInvitationResentAttempts: 2,
				},
				Receiver: data.Receiver{
					PhoneNumber: "+380443973607",
				},
			},
		}
		got = s.shouldSendInvitation(ctx, &org, &rwa)
		assert.True(t, got)

		// 8 days after receiving the first invitation - we don't resend because it reached the maximum number of attempts
		invitationSentAt = invitationSentAt.AddDate(0, 0, -int(smsResendInterval))
		rwa = data.ReceiverWalletAsset{
			ReceiverWallet: data.ReceiverWallet{
				InvitationSentAt: &invitationSentAt,
				ReceiverWalletStats: data.ReceiverWalletStats{
					TotalInvitationResentAttempts: 3,
				},
				Receiver: data.Receiver{
					PhoneNumber: "+380443973607",
				},
			},
		}
		got = s.shouldSendInvitation(ctx, &org, &rwa)
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
			wantErr:    fmt.Errorf("base URL for tenant can't be empty"),
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
		{
			link:       "https://test.com/foo",
			wantResult: "test.com",
			wantErr:    nil,
		},
		{
			link:       "https://test.com:8000",
			wantResult: "test.com:8000",
			wantErr:    nil,
		},
		{
			link:       "https://test.com:8000/foo",
			wantResult: "test.com:8000",
			wantErr:    nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.link, func(t *testing.T) {
			wdl := WalletDeepLink{
				TenantBaseURL: tc.link,
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
	require.EqualError(t, err, "tenant base URL can't be empty")

	// toml file domain can't be empty (different setup)
	wdl.DeepLink = "wallet://"
	wdl.Route = "sdp"
	err = wdl.validate()
	require.EqualError(t, err, "tenant base URL can't be empty")

	// organization name can't be empty
	wdl.TenantBaseURL = "foo.bar"
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
				DeepLink:         "wallet://",
				Route:            "sdp", // route added separated from the deep link
				TenantBaseURL:    "foo.bar",
				OrganizationName: "Foo Bar Org",
				AssetCode:        "FOO",
				AssetIssuer:      "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX",
			},
			wantResult: "wallet://sdp?asset=FOO-GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX&domain=foo.bar&name=Foo+Bar+Org",
		},
		{
			name: "🎉 successful for native (XLM) assets",
			walletDeepLink: WalletDeepLink{
				DeepLink:         "wallet://sdp", // route added directly to the deep link
				TenantBaseURL:    "foo.bar",
				OrganizationName: "Foo Bar Org",
				AssetCode:        "XLM",
			},
			wantResult: "wallet://sdp?asset=native&domain=foo.bar&name=Foo+Bar+Org",
		},
		{
			name: "🎉 successful for deeplink with query params",
			walletDeepLink: WalletDeepLink{
				DeepLink:         "wallet://sdp?custom=true",
				TenantBaseURL:    "foo.bar",
				OrganizationName: "Foo Bar Org",
				AssetCode:        "FOO",
				AssetIssuer:      "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX",
			},
			wantResult: "wallet://sdp?asset=FOO-GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX&custom=true&domain=foo.bar&name=Foo+Bar+Org",
		},
		{
			name: "🎉 successful for deeplink with regular URL",
			walletDeepLink: WalletDeepLink{
				DeepLink:         "https://test.com",
				TenantBaseURL:    "foo.bar",
				OrganizationName: "Foo Bar Org",
				AssetCode:        "FOO",
				AssetIssuer:      "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX",
			},
			wantResult: "https://test.com?asset=FOO-GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX&domain=foo.bar&name=Foo+Bar+Org",
		},
		{
			name: "🎉 successful for tenant base URL that contains a port",
			walletDeepLink: WalletDeepLink{
				DeepLink:         "https://test.com",
				TenantBaseURL:    "http://foo.bar:8000",
				OrganizationName: "Foo Bar Org",
				AssetCode:        "FOO",
				AssetIssuer:      "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX",
			},
			wantResult: "https://test.com?asset=FOO-GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX&domain=foo.bar%3A8000&name=Foo+Bar+Org",
		},
		{
			name: "🎉 successful for embedded wallet hosted by SDP",
			walletDeepLink: WalletDeepLink{
				DeepLink:         "http://foo.bar:8000",
				TenantBaseURL:    "http://foo.bar:8000",
				Route:            "wallet",
				OrganizationName: "Foo Bar Org",
				AssetCode:        "FOO",
				AssetIssuer:      "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX",
				Token:            "123",
				SelfHosted:       true,
			},
			wantResult: "http://foo.bar:8000/wallet?asset=FOO-GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX&token=123",
		},
		{
			name: "🎉 successful for embedded wallet hosted elsewhere",
			walletDeepLink: WalletDeepLink{
				DeepLink:         "https://test.com",
				TenantBaseURL:    "http://foo.bar:8000",
				OrganizationName: "Foo Bar Org",
				AssetCode:        "FOO",
				AssetIssuer:      "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX",
				Token:            "123",
			},
			wantResult: "https://test.com?asset=FOO-GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX&domain=foo.bar%3A8000&name=Foo+Bar+Org&token=123",
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
			DeepLink:         "wallet://",
			Route:            "sdp",
			TenantBaseURL:    "foo.bar",
			OrganizationName: "Foo Bar Org",
			AssetCode:        "FOO",
			AssetIssuer:      "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX",
		}

		actual, err := wdl.GetSignedRegistrationLink("invalid-secret-key")
		require.Empty(t, actual)
		require.EqualError(t, err, "error signing registration link: error parsing stellar private key: base32 decode failed: illegal base32 data at input byte 18")
	})

	t.Run("Successful for non-native assets 🎉", func(t *testing.T) {
		wdl := WalletDeepLink{
			DeepLink:         "wallet://sdp",
			TenantBaseURL:    "foo.bar",
			OrganizationName: "Foo Bar Org",
			AssetCode:        "FOO",
			AssetIssuer:      "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX",
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
			DeepLink:         "wallet://",
			Route:            "sdp",
			TenantBaseURL:    "foo.bar",
			OrganizationName: "Foo Bar Org",
			AssetCode:        "XLM",
		}

		expected := "wallet://sdp?asset=native&domain=foo.bar&name=Foo+Bar+Org&signature=972a3012e18f107e0bf951f5acc757df953c3bbbe668a2d2652bf2445a759132f6af303df063f69d1a862b7ab419813554b201837795648f6175c9d9d72cf60f"
		actual, err := wdl.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		require.Equal(t, expected, actual)

		isValid, err := utils.VerifySignedURL(actual, stellarPublicKey)
		require.NoError(t, err)
		require.True(t, isValid)
	})

	t.Run("Successful for native (XLM) assets and TenantBaseURL with https:// schema 🎉", func(t *testing.T) {
		wdl := WalletDeepLink{
			DeepLink:         "wallet://sdp",
			TenantBaseURL:    "https://foo.bar",
			OrganizationName: "Foo Bar Org",
			AssetCode:        "XLM",
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
