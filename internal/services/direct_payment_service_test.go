package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/protocols/horizon/base"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

func TestDirectPaymentService_CreateDirectPayment_Scenarios(t *testing.T) {
	t.Parallel()

	dbConnectionPool := testutils.GetDBConnectionPool(t)
	ctx := context.Background()
	ctx = sdpcontext.SetTenantInContext(ctx, &schema.Tenant{ID: "battle-barge-001"})

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "BOLT", "GBXGQJWVLWOYHFLVTKWV5FGHA3LNYY2JQKM7OAJAUEQFU6LPCSEFVXON")
	usdc := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	enabledWallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Macragge Treasury", "https://macragge.com", "macragge.com", "macragge://")
	disabledWallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Calth Reserve", "https://calth.com", "calth.com", "calth://")

	_, err = dbConnectionPool.ExecContext(ctx, "UPDATE wallets SET enabled = false WHERE id = $1", disabledWallet.ID)
	require.NoError(t, err)

	_, err = dbConnectionPool.ExecContext(ctx,
		"INSERT INTO wallets_assets (wallet_id, asset_id) VALUES ($1, $2)",
		enabledWallet.ID, asset.ID)
	require.NoError(t, err)

	user := &auth.User{ID: "user-guilliman", Email: "roboute@imperium.gov"}

	distributionAccPubKey := "GAAHIL6ZW4QFNLCKALZ3YOIWPP4TXQ7B7J5IU7RLNVGQAV6GFDZHLDTA"
	stellarDistAccountDBVault := schema.NewDefaultStellarTransactionAccount(distributionAccPubKey)
	stellarDistAccountEnv := schema.NewStellarEnvTransactionAccount(distributionAccPubKey)

	t.Run("successful direct payment with registered receiver wallet", func(t *testing.T) {
		t.Cleanup(func() {
			data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
			data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		})

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			Email: "marneus.calgar@macragge.imperium",
		})
		data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, enabledWallet.ID, data.RegisteredReceiversWalletStatus)

		req := CreateDirectPaymentRequest{
			Amount:            "100.00",
			Asset:             AssetReference{ID: &asset.ID},
			Receiver:          ReceiverReference{ID: &receiver.ID},
			Wallet:            WalletReference{ID: &enabledWallet.ID},
			ExternalPaymentID: testutils.StringPtr("ULTRAMAR-001"),
		}

		horizonClientMock := &horizonclient.MockClient{}
		mockDistService := &mocks.MockDistributionAccountService{}
		mockEventProducer := events.NewMockProducer(t)

		horizonClientMock.On("AccountDetail", horizonclient.AccountRequest{
			AccountID: distributionAccPubKey,
		}).Return(horizon.Account{
			AccountID: distributionAccPubKey,
			Sequence:  123,
			Balances: []horizon.Balance{
				{
					Balance: "10000000",
					Asset: base.Asset{
						Code:   asset.Code,
						Issuer: asset.Issuer,
					},
				},
			},
		}, nil).Once()

		mockDistService.On("GetBalance", mock.Anything, &stellarDistAccountDBVault, *asset).Return(float64(1000), nil)

		mockEventProducer.On("WriteMessages", mock.Anything, mock.MatchedBy(func(msgs []events.Message) bool {
			if len(msgs) != 1 {
				return false
			}
			msg := msgs[0]
			return msg.Topic == events.PaymentReadyToPayTopic &&
				msg.Type == events.PaymentReadyToPayDirectPayment
		})).Return(nil)

		service := NewDirectPaymentService(models, mockEventProducer, mockDistService, engine.SubmitterEngine{
			HorizonClient: horizonClientMock,
		})

		payment, err := service.CreateDirectPayment(ctx, req, user, &stellarDistAccountDBVault)

		require.NoError(t, err)
		assert.NotNil(t, payment)
		assert.Equal(t, data.ReadyPaymentStatus, payment.Status)
		assert.Equal(t, "100.0000000", payment.Amount)
		assert.Equal(t, "ULTRAMAR-001", payment.ExternalPaymentID)
		assert.Nil(t, payment.Disbursement)

		mockDistService.AssertExpectations(t)
		mockEventProducer.AssertExpectations(t)
		horizonClientMock.AssertExpectations(t)
	})

	t.Run("fails when wallet is not enabled", func(t *testing.T) {
		t.Cleanup(func() {
			data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		})

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			Email: "uriel.ventris@ultramar.imperium",
		})

		req := CreateDirectPaymentRequest{
			Amount:   "100.00",
			Asset:    AssetReference{ID: &asset.ID},
			Receiver: ReceiverReference{ID: &receiver.ID},
			Wallet:   WalletReference{ID: &disabledWallet.ID},
		}

		horizonClientMock := &horizonclient.MockClient{}
		mockDistService := &mocks.MockDistributionAccountService{}
		mockEventProducer := events.NewMockProducer(t)

		service := NewDirectPaymentService(models, mockEventProducer, mockDistService, engine.SubmitterEngine{
			HorizonClient: horizonClientMock,
		})

		payment, err := service.CreateDirectPayment(ctx, req, user, &stellarDistAccountDBVault)

		require.Error(t, err)
		assert.Nil(t, payment)

		err = unwrapTransactionError(err)
		var walletErr WalletNotEnabledError
		assert.True(t, errors.As(err, &walletErr))
		assert.Contains(t, err.Error(), "Calth Reserve")

		mockDistService.AssertExpectations(t)
		mockEventProducer.AssertExpectations(t)
		horizonClientMock.AssertExpectations(t)
	})

	t.Run("fails when asset is not supported by wallet", func(t *testing.T) {
		t.Cleanup(func() {
			data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		})

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			Email: "cato.sicarius@ultramar.imperium",
		})

		req := CreateDirectPaymentRequest{
			Amount:   "100.00",
			Asset:    AssetReference{ID: &usdc.ID}, // USDC not associated with enabled wallet
			Receiver: ReceiverReference{ID: &receiver.ID},
			Wallet:   WalletReference{ID: &enabledWallet.ID},
		}

		horizonClientMock := &horizonclient.MockClient{}
		mockDistService := &mocks.MockDistributionAccountService{}
		mockEventProducer := events.NewMockProducer(t)

		service := NewDirectPaymentService(models, mockEventProducer, mockDistService, engine.SubmitterEngine{
			HorizonClient: horizonClientMock,
		})

		payment, err := service.CreateDirectPayment(ctx, req, user, &stellarDistAccountDBVault)

		require.Error(t, err)
		assert.Nil(t, payment)

		err = unwrapTransactionError(err)
		var assetErr AssetNotSupportedByWalletError
		assert.True(t, errors.As(err, &assetErr))
		assert.Contains(t, err.Error(), "USDC")
		assert.Contains(t, err.Error(), enabledWallet.Name)

		mockDistService.AssertExpectations(t)
		mockEventProducer.AssertExpectations(t)
		horizonClientMock.AssertExpectations(t)
	})

	t.Run("fails when balance is insufficient", func(t *testing.T) {
		t.Cleanup(func() {
			data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
			data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		})

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			Email: "titus.grael@ultramar.imperium",
		})
		data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, enabledWallet.ID, data.RegisteredReceiversWalletStatus)

		req := CreateDirectPaymentRequest{
			Amount:   "1000.00", // More than available balance
			Asset:    AssetReference{ID: &asset.ID},
			Receiver: ReceiverReference{ID: &receiver.ID},
			Wallet:   WalletReference{ID: &enabledWallet.ID},
		}

		horizonClientMock := &horizonclient.MockClient{}
		mockDistService := &mocks.MockDistributionAccountService{}
		mockEventProducer := events.NewMockProducer(t)

		horizonClientMock.On("AccountDetail", horizonclient.AccountRequest{
			AccountID: distributionAccPubKey,
		}).Return(horizon.Account{
			AccountID: distributionAccPubKey,
			Sequence:  123,
			Balances: []horizon.Balance{
				{
					Balance: "10000000",
					Asset: base.Asset{
						Code:   asset.Code,
						Issuer: asset.Issuer,
					},
				},
			},
		}, nil).Once()

		mockDistService.On("GetBalance", mock.Anything, &stellarDistAccountDBVault, *asset).Return(float64(100), nil)

		service := NewDirectPaymentService(models, mockEventProducer, mockDistService, engine.SubmitterEngine{
			HorizonClient: horizonClientMock,
		})

		payment, err := service.CreateDirectPayment(ctx, req, user, &stellarDistAccountDBVault)

		require.Error(t, err)
		assert.Nil(t, payment)

		err = unwrapTransactionError(err)
		var balanceErr InsufficientBalanceForDirectPaymentError
		assert.True(t, errors.As(err, &balanceErr))
		assert.Contains(t, err.Error(), "insufficient balance")
		assert.Contains(t, err.Error(), "1000.00")
		assert.Contains(t, err.Error(), "100.000000 available")

		mockDistService.AssertExpectations(t)
		mockEventProducer.AssertExpectations(t)
		horizonClientMock.AssertExpectations(t)
	})

	t.Run("fails when receiver wallet not ready for payment", func(t *testing.T) {
		t.Cleanup(func() {
			data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
			data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		})

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			Email: "john.doe@example.com",
		})
		data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, enabledWallet.ID, data.DraftReceiversWalletStatus)

		req := CreateDirectPaymentRequest{
			Amount:   "10.00",
			Asset:    AssetReference{ID: &asset.ID},
			Receiver: ReceiverReference{ID: &receiver.ID},
			Wallet:   WalletReference{ID: &enabledWallet.ID},
		}

		horizonClientMock := &horizonclient.MockClient{}
		mockDistService := &mocks.MockDistributionAccountService{}
		mockEventProducer := events.NewMockProducer(t)

		service := NewDirectPaymentService(models, mockEventProducer, mockDistService, engine.SubmitterEngine{
			HorizonClient: horizonClientMock,
		})

		payment, err := service.CreateDirectPayment(ctx, req, user, &stellarDistAccountDBVault)

		require.Error(t, err)
		assert.Nil(t, payment)

		err = unwrapTransactionError(err)
		var recvWalletNotReadyErr ReceiverWalletNotReadyForPaymentError
		assert.True(t, errors.As(err, &recvWalletNotReadyErr))
		assert.ErrorContains(t, err, "receiver wallet is not ready for payment, current status is DRAFT")
	})

	t.Run("fails with invalid asset reference", func(t *testing.T) {
		t.Cleanup(func() {
			data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		})

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			Email: "cassius.felix@ultramar.imperium",
		})

		req := CreateDirectPaymentRequest{
			Amount:   "100.00",
			Asset:    AssetReference{}, // Empty reference - invalid
			Receiver: ReceiverReference{ID: &receiver.ID},
			Wallet:   WalletReference{ID: &enabledWallet.ID},
		}

		horizonClientMock := &horizonclient.MockClient{}
		mockDistService := &mocks.MockDistributionAccountService{}
		mockEventProducer := events.NewMockProducer(t)

		service := NewDirectPaymentService(models, mockEventProducer, mockDistService, engine.SubmitterEngine{
			HorizonClient: horizonClientMock,
		})

		payment, err := service.CreateDirectPayment(ctx, req, user, &stellarDistAccountDBVault)

		require.Error(t, err)
		assert.Nil(t, payment)

		err = unwrapTransactionError(err)
		var validationErr ValidationError
		assert.True(t, errors.As(err, &validationErr))
		assert.Equal(t, EntityTypeAsset, validationErr.EntityType)
		assert.Equal(t, FieldReference, validationErr.Field)
		assert.Contains(t, validationErr.Message, "must be specified by id or type")

		mockDistService.AssertExpectations(t)
		mockEventProducer.AssertExpectations(t)
		horizonClientMock.AssertExpectations(t)
	})

	t.Run("fails when receiver does not exist", func(t *testing.T) {
		t.Cleanup(func() {
			data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		})

		req := CreateDirectPaymentRequest{
			Amount:   "100.00",
			Asset:    AssetReference{ID: &asset.ID},
			Receiver: ReceiverReference{ID: testutils.StringPtr("chaos-marine-001")}, // Non-existent receiver
			Wallet:   WalletReference{ID: &enabledWallet.ID},
		}

		horizonClientMock := &horizonclient.MockClient{}
		mockDistService := &mocks.MockDistributionAccountService{}
		mockEventProducer := events.NewMockProducer(t)

		service := NewDirectPaymentService(models, mockEventProducer, mockDistService, engine.SubmitterEngine{
			HorizonClient: horizonClientMock,
		})

		payment, err := service.CreateDirectPayment(ctx, req, user, &stellarDistAccountDBVault)

		require.Error(t, err)
		assert.Nil(t, payment)

		err = unwrapTransactionError(err)
		var notFoundErr NotFoundError
		assert.True(t, errors.As(err, &notFoundErr))
		assert.Equal(t, EntityTypeReceiver, notFoundErr.EntityType)
		assert.Equal(t, "chaos-marine-001", notFoundErr.Reference)

		mockDistService.AssertExpectations(t)
		mockEventProducer.AssertExpectations(t)
		horizonClientMock.AssertExpectations(t)
	})

	t.Run("user-managed wallet skips certain validations", func(t *testing.T) {
		t.Cleanup(func() {
			data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
			data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		})

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			Email: "varro.tigurius@ultramar.imperium",
		})

		userManagedWallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "User Wallet", "stellar.org", "stellar.org", "stellar://")
		data.MakeWalletUserManaged(t, ctx, dbConnectionPool, userManagedWallet.ID)

		_, err := dbConnectionPool.ExecContext(ctx,
			"INSERT INTO wallets_assets (wallet_id, asset_id) VALUES ($1, $2)",
			userManagedWallet.ID, asset.ID)
		require.NoError(t, err)

		rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, userManagedWallet.ID, data.RegisteredReceiversWalletStatus)

		wallet, err := models.ReceiverWallet.GetByID(ctx, dbConnectionPool, rw.ID)
		require.NoError(t, err)
		req := CreateDirectPaymentRequest{
			Amount:   "10000.00", // Large amount to test balance validation
			Asset:    AssetReference{ID: &asset.ID},
			Receiver: ReceiverReference{ID: &receiver.ID},
			Wallet:   WalletReference{Address: &wallet.StellarAddress},
		}

		horizonClientMock := &horizonclient.MockClient{}
		mockDistService := &mocks.MockDistributionAccountService{}
		mockEventProducer := events.NewMockProducer(t)

		horizonClientMock.On("AccountDetail", horizonclient.AccountRequest{
			AccountID: distributionAccPubKey,
		}).Return(horizon.Account{
			AccountID: distributionAccPubKey,
			Sequence:  123,
			Balances: []horizon.Balance{
				{
					Balance: "100000000",
					Asset: base.Asset{
						Code:   asset.Code,
						Issuer: asset.Issuer,
					},
				},
			},
		}, nil).Once()

		mockDistService.On("GetBalance", mock.Anything, &stellarDistAccountEnv, *asset).Return(float64(50000), nil)

		mockEventProducer.On("WriteMessages", mock.Anything, mock.MatchedBy(func(msgs []events.Message) bool {
			if len(msgs) != 1 {
				return false
			}
			msg := msgs[0]
			return msg.Topic == events.PaymentReadyToPayTopic &&
				msg.Type == events.PaymentReadyToPayDirectPayment
		})).Return(nil)

		service := NewDirectPaymentService(models, mockEventProducer, mockDistService, engine.SubmitterEngine{
			HorizonClient: horizonClientMock,
		})

		payment, err := service.CreateDirectPayment(ctx, req, user, &stellarDistAccountEnv)

		require.NoError(t, err)
		assert.NotNil(t, payment)
		assert.Equal(t, "10000.0000000", payment.Amount)
		assert.Equal(t, data.ReadyPaymentStatus, payment.Status)

		mockDistService.AssertExpectations(t)
		mockEventProducer.AssertExpectations(t)
		horizonClientMock.AssertExpectations(t)
	})

	t.Run("fails when receiver wallet does not exist", func(t *testing.T) {
		t.Cleanup(func() {
			data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		})

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			Email: "severus.agemman@ultramar.imperium",
		})

		req := CreateDirectPaymentRequest{
			Amount:   "100.00",
			Asset:    AssetReference{ID: &asset.ID},
			Receiver: ReceiverReference{ID: &receiver.ID},
			Wallet:   WalletReference{ID: &enabledWallet.ID},
		}

		horizonClientMock := &horizonclient.MockClient{}
		mockDistService := &mocks.MockDistributionAccountService{}
		mockEventProducer := events.NewMockProducer(t)

		service := NewDirectPaymentService(models, mockEventProducer, mockDistService, engine.SubmitterEngine{
			HorizonClient: horizonClientMock,
		})

		payment, err := service.CreateDirectPayment(ctx, req, user, &stellarDistAccountDBVault)

		require.Error(t, err)
		assert.Nil(t, payment)

		err = unwrapTransactionError(err)
		var rwErr *ReceiverWalletNotFoundError
		assert.True(t, errors.As(err, &rwErr))
		assert.Contains(t, err.Error(), "no receiver wallet")

		mockDistService.AssertExpectations(t)
		mockEventProducer.AssertExpectations(t)
		horizonClientMock.AssertExpectations(t)
	})

	t.Run("fails when receiver by email is not found", func(t *testing.T) {
		t.Cleanup(func() {
			data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		})

		req := CreateDirectPaymentRequest{
			Amount:   "100.00",
			Asset:    AssetReference{ID: &asset.ID},
			Receiver: ReceiverReference{Email: testutils.StringPtr("chaos@warp.void")}, // Non-existent email
			Wallet:   WalletReference{ID: &enabledWallet.ID},
		}

		horizonClientMock := &horizonclient.MockClient{}
		mockDistService := &mocks.MockDistributionAccountService{}
		mockEventProducer := events.NewMockProducer(t)

		service := NewDirectPaymentService(models, mockEventProducer, mockDistService, engine.SubmitterEngine{
			HorizonClient: horizonClientMock,
		})

		payment, err := service.CreateDirectPayment(ctx, req, user, &stellarDistAccountDBVault)

		require.Error(t, err)
		assert.Nil(t, payment)

		err = unwrapTransactionError(err)
		var notFoundErr NotFoundError
		assert.True(t, errors.As(err, &notFoundErr))
		assert.Equal(t, EntityTypeReceiver, notFoundErr.EntityType)
		assert.Contains(t, notFoundErr.Message, "no receiver found with contact info")

		mockDistService.AssertExpectations(t)
		mockEventProducer.AssertExpectations(t)
		horizonClientMock.AssertExpectations(t)
	})
}

func TestDirectPaymentService_CreateDirectPayment_CircleAccount(t *testing.T) {
	t.Parallel()

	dbConnectionPool := testutils.GetDBConnectionPool(t)
	ctx := context.Background()
	ctx = sdpcontext.SetTenantInContext(ctx, &schema.Tenant{ID: "battle-barge-001"})

	t.Cleanup(func() {
		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
	})

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "EURC", "GBVHJTRLQRMIHRYTXZQOPVYCVVH7IRJN3DOFT7VC6U75CBWWBVDTWURG")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Test Wallet", "https://testdomain.com", "testdomain.com", "test://")

	_, err = dbConnectionPool.ExecContext(ctx,
		"INSERT INTO wallets_assets (wallet_id, asset_id) VALUES ($1, $2)",
		wallet.ID, asset.ID)
	require.NoError(t, err)

	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
		Email: "perturabo@iron.warriors",
	})
	data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	user := &auth.User{ID: "user-dorn", Email: "rogal@imperial.fists"}

	circleDistAccount := schema.TransactionAccount{
		CircleWalletID: "circle-wallet-fortify",
		Type:           schema.DistributionAccountCircleDBVault,
		Status:         schema.AccountStatusActive,
	}

	req := CreateDirectPaymentRequest{
		Amount:            "500.00",
		Asset:             AssetReference{ID: &asset.ID},
		Receiver:          ReceiverReference{ID: &receiver.ID},
		Wallet:            WalletReference{ID: &wallet.ID},
		ExternalPaymentID: testutils.StringPtr("SIEGE-IRON-001"),
	}

	mockDistService := &mocks.MockDistributionAccountService{}
	mockEventProducer := events.NewMockProducer(t)
	horizonClientMock := &horizonclient.MockClient{}

	mockDistService.On("GetBalance", mock.Anything, &circleDistAccount, *asset).Return(float64(1000), nil)

	mockEventProducer.On("WriteMessages", mock.Anything, mock.MatchedBy(func(msgs []events.Message) bool {
		if len(msgs) != 1 {
			return false
		}
		msg := msgs[0]
		return msg.Topic == events.CirclePaymentReadyToPayTopic &&
			msg.Type == events.PaymentReadyToPayDirectPayment
	})).Return(nil)

	service := NewDirectPaymentService(models, mockEventProducer, mockDistService, engine.SubmitterEngine{
		HorizonClient: horizonClientMock,
	})

	payment, err := service.CreateDirectPayment(ctx, req, user, &circleDistAccount)

	require.NoError(t, err)
	assert.NotNil(t, payment)
	assert.Equal(t, data.ReadyPaymentStatus, payment.Status)
	assert.Equal(t, "500.0000000", payment.Amount)
	assert.Equal(t, "SIEGE-IRON-001", payment.ExternalPaymentID)
	assert.Nil(t, payment.Disbursement)

	mockDistService.AssertExpectations(t)
	mockEventProducer.AssertExpectations(t)
	horizonClientMock.AssertExpectations(t)
}

func TestDirectPaymentService_calculatePendingAmountForAsset(t *testing.T) {
	t.Parallel()
	dbConnectionPool := testutils.GetDBConnectionPool(t)
	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	asset1 := data.CreateAssetFixture(t, ctx, dbConnectionPool, "LASGUN", "GBXGQJWVLWOYHFLVTKWV5FGHA3LNYY2JQKM7OAJAUEQFU6LPCSEFVXON")
	asset2 := data.CreateAssetFixture(t, ctx, dbConnectionPool, "MELTA", "GBRPYHIL2CI3FNQ4BXLFMNDLFJUNPU2HY3ZMFSHONUCEOASW7QC7OX2H")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Arsenal", "https://arsenal.com", "arsenal.com", "arsenal://")
	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:   "Siege of Terra",
		Status: data.StartedDisbursementStatus,
		Asset:  asset1,
		Wallet: wallet,
	})
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	service := NewDirectPaymentService(models, nil, nil, engine.SubmitterEngine{})

	type payment struct {
		asset  *data.Asset
		amount string
		status data.PaymentStatus
	}

	tests := []struct {
		name           string
		payments       []payment
		targetAsset    *data.Asset
		expectedAmount float64
	}{
		{
			name: "sum for all in-progress statuses",
			payments: []payment{
				{asset1, "100.00", data.ReadyPaymentStatus},
				{asset1, "200.00", data.PendingPaymentStatus},
				{asset1, "300.00", data.PausedPaymentStatus},
				{asset1, "999.00", data.DraftPaymentStatus},    // ignored
				{asset1, "888.00", data.SuccessPaymentStatus},  // ignored
				{asset1, "500.00", data.FailedPaymentStatus},   // ignored
				{asset1, "600.00", data.CanceledPaymentStatus}, // ignored
			},
			targetAsset:    asset1,
			expectedAmount: 600.00,
		},
		{
			name: "other assets ignored",
			payments: []payment{
				{asset1, "50.00", data.PendingPaymentStatus},
				{asset2, "999.99", data.PendingPaymentStatus},
			},
			targetAsset:    asset1,
			expectedAmount: 50.00,
		},
		{
			name:           "zero sum for no in-progress payments",
			payments:       []payment{},
			targetAsset:    asset1,
			expectedAmount: 0.0,
		},
		{
			name: "zero-amount payment is included",
			payments: []payment{
				{asset1, "0.00", data.ReadyPaymentStatus},
				{asset1, "10.00", data.PausedPaymentStatus},
			},
			targetAsset:    asset1,
			expectedAmount: 10.00,
		},
		{
			name: "multiple assets and mixed statuses",
			payments: []payment{
				{asset1, "100.00", data.ReadyPaymentStatus},
				{asset1, "200.00", data.SuccessPaymentStatus},
				{asset2, "300.00", data.PausedPaymentStatus},
				{asset1, "400.00", data.PendingPaymentStatus},
				{asset2, "500.00", data.DraftPaymentStatus},
			},
			targetAsset:    asset1,
			expectedAmount: 500.00,
		},
		{
			name:           "empty payments table",
			payments:       nil,
			targetAsset:    asset1,
			expectedAmount: 0.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tx, err := dbConnectionPool.BeginTxx(ctx, nil)
			t.Cleanup(func() {
				data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
				err = tx.Rollback()
				require.NoError(t, err)
			})

			for _, p := range tc.payments {
				createPayment(t, ctx, dbConnectionPool, models, rw, disbursement, *p.asset, p.amount, p.status)
			}

			require.NoError(t, err)
			total, err := service.calculatePendingAmountForAsset(ctx, tx, *tc.targetAsset)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedAmount, total)
		})
	}
}

func TestDirectPaymentService_CreateDirectPayment_Success(t *testing.T) {
	t.Parallel()
	dbConnectionPool := testutils.GetDBConnectionPool(t)

	ctx := context.Background()
	ctx = sdpcontext.SetTenantInContext(ctx, &schema.Tenant{ID: "battle-barge-001"})
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "XLM", "")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Test Wallet", "https://testdomain.com", "testdomain.com", "test://")

	_, err = dbConnectionPool.ExecContext(ctx,
		"INSERT INTO wallets_assets (wallet_id, asset_id) VALUES ($1, $2)",
		wallet.ID, asset.ID)
	require.NoError(t, err)

	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
		Email: "horus@warmaster.terra",
	})

	user := &auth.User{ID: "user-vespasian", Email: "vespasian@emperor.terra"}
	distributionAccount := &schema.TransactionAccount{
		Type: schema.DistributionAccountStellarDBVault,
	}

	service := NewDirectPaymentService(models, nil, nil, engine.SubmitterEngine{})

	mockDistService := &mocks.MockDistributionAccountService{}
	mockDistService.On("GetBalance", mock.Anything, distributionAccount, *asset).Return(100.0, nil)
	service.DistributionAccountService = mockDistService
	_ = data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	req := CreateDirectPaymentRequest{
		Amount: "50.00",
		Asset: AssetReference{
			ID: &asset.ID,
		},
		Receiver: ReceiverReference{
			ID: &receiver.ID,
		},
		Wallet: WalletReference{
			ID: &wallet.ID,
		},
		ExternalPaymentID: testutils.StringPtr("PAY_HORUS_001"),
	}

	payment, err := service.CreateDirectPayment(ctx, req, user, distributionAccount)
	require.NoError(t, err)
	assert.NotNil(t, payment)
	assert.Equal(t, "50.0000000", payment.Amount)
	assert.Equal(t, data.PaymentTypeDirect, payment.Type)
	assert.Equal(t, data.ReadyPaymentStatus, payment.Status)
	assert.Equal(t, "PAY_HORUS_001", payment.ExternalPaymentID)
	assert.Nil(t, payment.Disbursement)
	assert.Equal(t, asset.ID, payment.Asset.ID)

	mockDistService.AssertExpectations(t)
}

func unwrapTransactionError(err error) error {
	var txErr *db.TransactionExecutionError
	if errors.As(err, &txErr) {
		return txErr.Unwrap()
	}
	return err
}

func createPayment(
	t *testing.T,
	ctx context.Context,
	db db.DBConnectionPool,
	models *data.Models,
	rw *data.ReceiverWallet,
	disbursement *data.Disbursement,
	asset data.Asset,
	amount string,
	status data.PaymentStatus,
) {
	data.CreatePaymentFixture(t, ctx, db, models.Payment, &data.Payment{
		ReceiverWallet: rw,
		Disbursement:   disbursement,
		Asset:          asset,
		Amount:         amount,
		Status:         status,
	})
}

func TestDirectPaymentService_CreateDirectPayment_WithVerifiedReceiver(t *testing.T) {
	t.Parallel()

	dbConnectionPool := testutils.GetDBConnectionPool(t)
	ctx := context.Background()
	ctx = sdpcontext.SetTenantInContext(ctx, &schema.Tenant{ID: "test-tenant-001"})

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "TEST", "GBXGQJWVLWOYHFLVTKWV5FGHA3LNYY2JQKM7OAJAUEQFU6LPCSEFVXON")

	sep24Wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "SEP24 Wallet", "https://sep24.com", "sep24.com", "sep24://")
	_, err = dbConnectionPool.ExecContext(ctx, "UPDATE wallets SET user_managed = false WHERE id = $1", sep24Wallet.ID)
	require.NoError(t, err)

	_, err = dbConnectionPool.ExecContext(ctx, "INSERT INTO wallets_assets (wallet_id, asset_id) VALUES ($1, $2)", sep24Wallet.ID, asset.ID)
	require.NoError(t, err)

	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
		Email: "verified@example.com",
	})

	_, err = dbConnectionPool.ExecContext(ctx, `
		INSERT INTO receiver_verifications (receiver_id, verification_field, hashed_value)
		VALUES ($1, $2, $3)
	`, receiver.ID, data.VerificationTypeDateOfBirth, "hashed_dob_value")
	require.NoError(t, err)

	user := &auth.User{ID: "test-user", Email: "admin@test.com"}
	distAccount := &schema.TransactionAccount{
		Address: "GDUKZH7LPVPDNWJ5JHQFAR4J5DQWQK3F3H2O5XZZ7MXUX7K3RBQNQOKT",
		Type:    schema.DistributionAccountStellarEnv,
		Status:  schema.AccountStatusActive,
	}

	mockDistAccountService := &mocks.MockDistributionAccountService{}
	mockDistAccountService.On("GetBalance", mock.Anything, distAccount, mock.MatchedBy(func(a data.Asset) bool {
		return a.ID == asset.ID
	})).Return(1000.0, nil).Once()

	mockHorizonClient := &horizonclient.MockClient{}
	mockAccountReq := horizonclient.AccountRequest{AccountID: distAccount.Address}
	mockHorizonClient.On("AccountDetail", mockAccountReq).Return(horizon.Account{
		Balances: []horizon.Balance{
			{Asset: base.Asset{Type: "native"}},
			{Asset: base.Asset{Type: "credit_alphanum4", Code: asset.Code, Issuer: asset.Issuer}},
		},
	}, nil).Once()

	service := &DirectPaymentService{
		Models:                     models,
		EventProducer:              nil,
		DistributionAccountService: mockDistAccountService,
		Resolvers:                  NewResolverFactory(models),
		SubmitterEngine: engine.SubmitterEngine{
			HorizonClient: mockHorizonClient,
		},
	}

	t.Run("creates_receiver_wallet_for_verified_receiver_with_sep24_wallet", func(t *testing.T) {
		receiverWallets, getErr := models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiver.ID}, sep24Wallet.ID)
		require.NoError(t, getErr)
		require.Len(t, receiverWallets, 0, "No receiver wallet should exist initially")

		req := CreateDirectPaymentRequest{
			Amount: "100.00",
			Asset: AssetReference{
				ID: &asset.ID,
			},
			Receiver: ReceiverReference{
				ID: &receiver.ID,
			},
			Wallet: WalletReference{
				ID: &sep24Wallet.ID,
			},
		}

		payment, paymentErr := service.CreateDirectPayment(ctx, req, user, distAccount)
		require.NoError(t, paymentErr)
		require.NotNil(t, payment)

		assert.Equal(t, receiver.ID, payment.ReceiverWallet.Receiver.ID)
		assert.Equal(t, asset.ID, payment.Asset.ID)
		assert.Equal(t, "100.0000000", payment.Amount)
		assert.Equal(t, data.PaymentTypeDirect, payment.Type)

		receiverWallets, getErr2 := models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, dbConnectionPool, []string{receiver.ID}, sep24Wallet.ID)
		require.NoError(t, getErr2)
		require.Len(t, receiverWallets, 1, "Receiver wallet should have been created")

		createdRW := receiverWallets[0]
		assert.Equal(t, receiver.ID, createdRW.Receiver.ID)
		assert.Equal(t, sep24Wallet.ID, createdRW.Wallet.ID)
		assert.Equal(t, data.ReadyReceiversWalletStatus, createdRW.Status)
	})

	t.Run("does_not_create_receiver_wallet_for_user_managed_wallet", func(t *testing.T) {
		userManagedWallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "User Wallet", "https://user.com", "user.com", "user://")
		_, updateErr := dbConnectionPool.ExecContext(ctx, "UPDATE wallets SET user_managed = true WHERE id = $1", userManagedWallet.ID)
		require.NoError(t, updateErr)

		req := CreateDirectPaymentRequest{
			Amount: "100.00",
			Asset: AssetReference{
				ID: &asset.ID,
			},
			Receiver: ReceiverReference{
				ID: &receiver.ID,
			},
			Wallet: WalletReference{
				ID: &userManagedWallet.ID,
			},
		}

		payment, paymentErr := service.CreateDirectPayment(ctx, req, user, distAccount)
		require.Error(t, paymentErr)
		require.Nil(t, payment)

		require.Contains(t, paymentErr.Error(), "no receiver wallet")
	})

	t.Run("does_not_create_receiver_wallet_for_receiver_without_verifications", func(t *testing.T) {
		unverifiedReceiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			Email: "unverified@example.com",
		})

		req := CreateDirectPaymentRequest{
			Amount: "100.00",
			Asset: AssetReference{
				ID: &asset.ID,
			},
			Receiver: ReceiverReference{
				ID: &unverifiedReceiver.ID,
			},
			Wallet: WalletReference{
				ID: &sep24Wallet.ID,
			},
		}

		payment, paymentErr := service.CreateDirectPayment(ctx, req, user, distAccount)
		require.Error(t, paymentErr)
		require.Nil(t, payment)

		require.Contains(t, paymentErr.Error(), "no receiver wallet")
	})

	mockDistAccountService.AssertExpectations(t)
	mockHorizonClient.AssertExpectations(t)
}

func Test_TrustlineNotFoundError_Error(t *testing.T) {
	err := TrustlineNotFoundError{
		Asset: data.Asset{
			Code:   "USDC",
			Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
		},
		DistributionAccount: "GDSPHTXJIMA762ZXHPPR5QR3ZA6CT7M3QQHYAFUDIBB5AJL2DM5F4OKS",
	}

	expectedMsg := "distribution account GDSPHTXJIMA762ZXHPPR5QR3ZA6CT7M3QQHYAFUDIBB5AJL2DM5F4OKS does not have a trustline for asset USDC:GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"
	assert.Equal(t, expectedMsg, err.Error())
}

func Test_AccountNotFoundError_Error(t *testing.T) {
	err := AccountNotFoundError{
		Address: "GDSPHTXJIMA762ZXHPPR5QR3ZA6CT7M3QQHYAFUDIBB5AJL2DM5F4OKS",
	}

	expectedMsg := "distribution account GDSPHTXJIMA762ZXHPPR5QR3ZA6CT7M3QQHYAFUDIBB5AJL2DM5F4OKS not found on the Stellar network"
	assert.Equal(t, expectedMsg, err.Error())
}

func Test_CircleAccountNotActivatedError_Error(t *testing.T) {
	testCases := []struct {
		name        string
		accountType string
		status      string
		expected    string
	}{
		{
			name:        "Circle wallet pending",
			accountType: "CIRCLE",
			status:      string(schema.AccountStatusPendingUserActivation),
			expected:    "Circle distribution account is in PENDING_USER_ACTIVATION state, please complete the CIRCLE activation process",
		},
		{
			name:        "Circle account inactive",
			accountType: "CIRCLE",
			status:      "INACTIVE",
			expected:    "Circle distribution account is in INACTIVE state, please complete the CIRCLE activation process",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := CircleAccountNotActivatedError{
				AccountType: tc.accountType,
				Status:      tc.status,
			}
			assert.Equal(t, tc.expected, err.Error())
		})
	}
}

func Test_CircleAssetNotSupportedError_Error(t *testing.T) {
	err := CircleAssetNotSupportedError{
		Asset: data.Asset{
			Code:   "EUROC",
			Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
		},
	}

	expectedMsg := "asset EUROC is not supported by Circle for this distribution account"
	assert.Equal(t, expectedMsg, err.Error())
}

func Test_WalletNotEnabledError_Error(t *testing.T) {
	err := WalletNotEnabledError{
		WalletName: "Vibrant Assist",
	}

	expectedMsg := "wallet 'Vibrant Assist' is not enabled for payments"
	assert.Equal(t, expectedMsg, err.Error())
}

func Test_ReceiverWalletNotFoundError_Error(t *testing.T) {
	err := ReceiverWalletNotFoundError{
		ReceiverID: "receiver-123",
		WalletID:   "wallet-456",
	}

	expectedMsg := "no receiver wallet: receiver=receiver-123 wallet=wallet-456"
	assert.Equal(t, expectedMsg, err.Error())
}

func Test_ReceiverWalletNotReadyForPaymentError_Error(t *testing.T) {
	testCases := []struct {
		name   string
		status data.ReceiversWalletStatus
	}{
		{
			name:   "Draft status",
			status: data.DraftReceiversWalletStatus,
		},
		{
			name:   "Ready status",
			status: data.ReadyReceiversWalletStatus,
		},
		{
			name:   "Flagged status",
			status: data.FlaggedReceiversWalletStatus,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ReceiverWalletNotReadyForPaymentError{
				CurrentStatus: tc.status,
			}
			expectedMsg := "receiver wallet is not ready for payment, current status is " + string(tc.status)
			assert.Equal(t, expectedMsg, err.Error())
		})
	}
}

func Test_AssetNotSupportedByWalletError_Error(t *testing.T) {
	err := AssetNotSupportedByWalletError{
		AssetCode:  "EUROC",
		WalletName: "Vibrant Assist",
	}

	expectedMsg := "asset 'EUROC' is not supported by wallet 'Vibrant Assist'"
	assert.Equal(t, expectedMsg, err.Error())
}

func Test_InsufficientBalanceForDirectPaymentError_Error(t *testing.T) {
	testCases := []struct {
		name               string
		requestedAmount    float64
		availableBalance   float64
		totalPendingAmount float64
		assetCode          string
		expectedError      string
	}{
		{
			name:               "Simple insufficient balance",
			requestedAmount:    100.0,
			availableBalance:   50.0,
			totalPendingAmount: 0.0,
			assetCode:          "USDC",
			expectedError:      "insufficient balance for direct payment: requested 100.000000 USDC, but only 50.000000 available (0.000000 in pending payments). Need 50.000000 more USDC",
		},
		{
			name:               "Insufficient with pending payments",
			requestedAmount:    100.0,
			availableBalance:   120.0,
			totalPendingAmount: 30.0,
			assetCode:          "USDC",
			expectedError:      "insufficient balance for direct payment: requested 100.000000 USDC, but only 120.000000 available (30.000000 in pending payments). Need 10.000000 more USDC",
		},
		{
			name:               "Large amount with pending",
			requestedAmount:    1000.50,
			availableBalance:   500.25,
			totalPendingAmount: 200.75,
			assetCode:          "XLM",
			expectedError:      "insufficient balance for direct payment: requested 1000.500000 XLM, but only 500.250000 available (200.750000 in pending payments). Need 701.000000 more XLM",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := InsufficientBalanceForDirectPaymentError{
				Asset: data.Asset{
					Code:   tc.assetCode,
					Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
				},
				RequestedAmount:    tc.requestedAmount,
				AvailableBalance:   tc.availableBalance,
				TotalPendingAmount: tc.totalPendingAmount,
			}
			assert.Equal(t, tc.expectedError, err.Error())
		})
	}
}
