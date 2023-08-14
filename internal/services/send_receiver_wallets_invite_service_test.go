package services

import (
	"context"
	"errors"
	"fmt"
	"testing"

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
		Return(message.MessengerTypeTwilioSMS)

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
		_, err := NewSendReceiverWalletInviteService(models, nil, anchorPlatformBaseSepURL, stellarSecretKey, 3, 2, mockCrashTrackerClient)
		assert.EqualError(t, err, "invalid service setup: messenger client can't be nil")

		_, err = NewSendReceiverWalletInviteService(models, messengerClientMock, "", stellarSecretKey, 3, 2, mockCrashTrackerClient)
		assert.EqualError(t, err, "invalid service setup: anchorPlatformBaseSepURL can't be empty")
	})

	t.Run("inserts the failed sent message", func(t *testing.T) {
		s, err := NewSendReceiverWalletInviteService(models, messengerClientMock, anchorPlatformBaseSepURL, stellarSecretKey, 3, 2, mockCrashTrackerClient)
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
		s, err := NewSendReceiverWalletInviteService(models, messengerClientMock, anchorPlatformBaseSepURL, stellarSecretKey, 3, 2, mockCrashTrackerClient)
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
				ToPhoneNumber: receiver1.PhoneNumber,
				Message:       contentWallet2,
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
		err = dbConnectionPool.GetContext(ctx, &msg, q, receiver1.ID, wallet2.ID, rec1RW.ID)
		require.NoError(t, err)

		assert.Equal(t, message.MessengerTypeTwilioSMS, msg.Type)
		assert.Equal(t, receiver1.ID, msg.ReceiverID)
		assert.Equal(t, wallet2.ID, msg.WalletID)
		assert.Equal(t, rec1RW.ID, *msg.ReceiverWalletID)
		assert.Equal(t, data.SuccessMessageStatus, msg.Status)
		assert.Empty(t, msg.TitleEncrypted)
		assert.Equal(t, contentWallet2, msg.TextEncrypted)
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
}

func Test_WalletDeepLink_BaseURL(t *testing.T) {
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

	// asset issuer needs to be empty if it's native (XLM)
	wdl.AssetCode = "XLM"
	wdl.AssetIssuer = "GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX"
	err = wdl.validate()
	require.EqualError(t, err, "asset issuer should be empty for XLM, but is GCKGCKZ2PFSCRQXREJMTHAHDMOZQLS2R4V5LZ6VLU53HONH5FI6ACBSX")

	// Successful for native (XLM) assets 🎉
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
			wantResult: "wallet://sdp?asset=XLM&domain=foo.bar&name=Foo+Bar+Org",
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

		expected := "wallet://sdp?asset=XLM&domain=foo.bar&name=Foo+Bar+Org&signature=d3ffb7c9f78d2131b5be4e3a1302cfe87685706e36f6f1115e4b28bb940cc75532d56ab1d5c5f3481f210021811510290735858ea35b88e26cd5a115f7ea450b"
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

		expected := "wallet://sdp?asset=XLM&domain=foo.bar&name=Foo+Bar+Org&signature=d3ffb7c9f78d2131b5be4e3a1302cfe87685706e36f6f1115e4b28bb940cc75532d56ab1d5c5f3481f210021811510290735858ea35b88e26cd5a115f7ea450b"
		actual, err := wdl.GetSignedRegistrationLink(stellarSecretKey)
		require.NoError(t, err)
		require.Equal(t, expected, actual)

		isValid, err := utils.VerifySignedURL(actual, stellarPublicKey)
		require.NoError(t, err)
		require.True(t, isValid)
	})
}
