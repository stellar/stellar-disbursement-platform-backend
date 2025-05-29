package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func TestDirectPaymentService_CreateDirectPayment_Scenarios(t *testing.T) {
	t.Parallel()
	dbConnectionPool := testutils.GetDBConnectionPool(t)

	ctx := context.Background()
	ctx = tenant.SaveTenantInContext(ctx, &tenant.Tenant{ID: "battle-barge-001"})

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	// Create shared assets and wallets
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "BOLT", "GBXGQJWVLWOYHFLVTKWV5FGHA3LNYY2JQKM7OAJAUEQFU6LPCSEFVXON")
	xlm := data.CreateAssetFixture(t, ctx, dbConnectionPool, "XLM", "")

	enabledWallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Macragge Treasury", "https://macragge.com", "macragge.com", "macragge://")
	disabledWallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Calth Reserve", "https://calth.com", "calth.com", "calth://")

	// Disable the wallet
	_, err = dbConnectionPool.ExecContext(ctx, "UPDATE wallets SET enabled = false WHERE id = $1", disabledWallet.ID)
	require.NoError(t, err)

	// Associate asset with enabled wallet only
	_, err = dbConnectionPool.ExecContext(ctx,
		"INSERT INTO wallets_assets (wallet_id, asset_id) VALUES ($1, $2)",
		enabledWallet.ID, asset.ID)
	require.NoError(t, err)

	user := &auth.User{ID: "user-guilliman", Email: "roboute@imperium.gov"}

	tests := []struct {
		name                string
		setupTestData       func() (CreateDirectPaymentRequest, *data.Receiver) // Returns request and receiver for cleanup
		distributionAccount *schema.TransactionAccount
		setupMocks          func(*mocks.MockDistributionAccountService, *events.MockProducer)
		assertResult        func(*testing.T, *data.Payment, error)
	}{
		{
			name: "successful direct payment - receiver wallet exists and is registered",
			setupTestData: func() (CreateDirectPaymentRequest, *data.Receiver) {
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
				return req, receiver
			},
			distributionAccount: &schema.TransactionAccount{Type: schema.DistributionAccountStellarDBVault},
			setupMocks: func(distService *mocks.MockDistributionAccountService, eventProducer *events.MockProducer) {
				distService.On("GetBalance", mock.Anything, mock.Anything, *asset).Return(float64(1000), nil)
				eventProducer.On("WriteMessages", mock.Anything, mock.MatchedBy(func(msgs []events.Message) bool {
					return len(msgs) == 1 && msgs[0].Topic == events.PaymentReadyToPayTopic
				})).Return(nil)
			},
			assertResult: func(t *testing.T, payment *data.Payment, err error) {
				require.NoError(t, err)
				assert.NotNil(t, payment)
				assert.Equal(t, data.ReadyPaymentStatus, payment.Status)
				assert.Equal(t, "100.0000000", payment.Amount)
				assert.Equal(t, "ULTRAMAR-001", payment.ExternalPaymentID)
			},
		},
		{
			name: "fails - wallet not enabled",
			setupTestData: func() (CreateDirectPaymentRequest, *data.Receiver) {
				receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
					Email: "uriel.ventris@ultramar.imperium",
				})

				req := CreateDirectPaymentRequest{
					Amount:   "100.00",
					Asset:    AssetReference{ID: &asset.ID},
					Receiver: ReceiverReference{ID: &receiver.ID},
					Wallet:   WalletReference{ID: &disabledWallet.ID},
				}
				return req, receiver
			},
			distributionAccount: &schema.TransactionAccount{Type: schema.DistributionAccountStellarDBVault},
			setupMocks:          func(*mocks.MockDistributionAccountService, *events.MockProducer) {},
			assertResult: func(t *testing.T, payment *data.Payment, err error) {
				require.Error(t, err)
				err = unwrapTransactionError(err)
				var walletErr WalletNotEnabledError
				assert.True(t, errors.As(err, &walletErr))
				assert.Contains(t, err.Error(), "Calth Reserve")
			},
		},
		{
			name: "fails - asset not supported by wallet",
			setupTestData: func() (CreateDirectPaymentRequest, *data.Receiver) {
				receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
					Email: "cato.sicarius@ultramar.imperium",
				})

				req := CreateDirectPaymentRequest{
					Amount:   "100.00",
					Asset:    AssetReference{ID: &xlm.ID},
					Receiver: ReceiverReference{ID: &receiver.ID},
					Wallet:   WalletReference{ID: &enabledWallet.ID},
				}
				return req, receiver
			},
			distributionAccount: &schema.TransactionAccount{Type: schema.DistributionAccountStellarDBVault},
			setupMocks:          func(*mocks.MockDistributionAccountService, *events.MockProducer) {},
			assertResult: func(t *testing.T, payment *data.Payment, err error) {
				require.Error(t, err)
				err = unwrapTransactionError(err)
				var assetErr AssetNotSupportedByWalletError
				assert.True(t, errors.As(err, &assetErr))
				assert.Contains(t, err.Error(), "XLM")
				assert.Contains(t, err.Error(), "Macragge Treasury")
			},
		},
		{
			name: "fails - insufficient balance",
			setupTestData: func() (CreateDirectPaymentRequest, *data.Receiver) {
				receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
					Email: "titus.grael@ultramar.imperium",
				})
				data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, enabledWallet.ID, data.RegisteredReceiversWalletStatus)

				req := CreateDirectPaymentRequest{
					Amount:   "1000.00",
					Asset:    AssetReference{ID: &asset.ID},
					Receiver: ReceiverReference{ID: &receiver.ID},
					Wallet:   WalletReference{ID: &enabledWallet.ID},
				}
				return req, receiver
			},
			distributionAccount: &schema.TransactionAccount{Type: schema.DistributionAccountStellarDBVault},
			setupMocks: func(distService *mocks.MockDistributionAccountService, eventProducer *events.MockProducer) {
				distService.On("GetBalance", mock.Anything, mock.Anything, *asset).Return(float64(100), nil)
			},
			assertResult: func(t *testing.T, payment *data.Payment, err error) {
				require.Error(t, err)
				err = unwrapTransactionError(err)
				var balanceErr InsufficientBalanceForDirectPaymentError
				assert.True(t, errors.As(err, &balanceErr))
				assert.Contains(t, err.Error(), "insufficient balance")
				assert.Contains(t, err.Error(), "1000.00")
				assert.Contains(t, err.Error(), "100.00 available")
			},
		},
		{
			name: "fails - invalid asset reference",
			setupTestData: func() (CreateDirectPaymentRequest, *data.Receiver) {
				receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
					Email: "cassius.felix@ultramar.imperium",
				})

				req := CreateDirectPaymentRequest{
					Amount:   "100.00",
					Asset:    AssetReference{}, // Empty reference
					Receiver: ReceiverReference{ID: &receiver.ID},
					Wallet:   WalletReference{ID: &enabledWallet.ID},
				}
				return req, receiver
			},
			distributionAccount: &schema.TransactionAccount{Type: schema.DistributionAccountStellarDBVault},
			setupMocks:          func(*mocks.MockDistributionAccountService, *events.MockProducer) {},
			assertResult: func(t *testing.T, payment *data.Payment, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "asset must be specified")
			},
		},
		{
			name: "fails - non-existent receiver",
			setupTestData: func() (CreateDirectPaymentRequest, *data.Receiver) {
				req := CreateDirectPaymentRequest{
					Amount:   "100.00",
					Asset:    AssetReference{ID: &asset.ID},
					Receiver: ReceiverReference{ID: testutils.StringPtr("chaos-marine-001")},
					Wallet:   WalletReference{ID: &enabledWallet.ID},
				}
				return req, nil // No receiver to clean up
			},
			distributionAccount: &schema.TransactionAccount{Type: schema.DistributionAccountStellarDBVault},
			setupMocks:          func(*mocks.MockDistributionAccountService, *events.MockProducer) {},
			assertResult: func(t *testing.T, payment *data.Payment, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "record not found")
			},
		},
		{
			name: "user-managed wallet - skips balance validation",
			setupTestData: func() (CreateDirectPaymentRequest, *data.Receiver) {
				receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
					Email: "varro.tigurius@ultramar.imperium",
				})

				// Create user-managed wallet
				userManagedWallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "User Wallet", "stellar.org", "stellar.org", "stellar://")
				data.MakeWalletUserManaged(t, ctx, dbConnectionPool, userManagedWallet.ID)

				// Create receiver wallet with specific address
				rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, userManagedWallet.ID, data.RegisteredReceiversWalletStatus)

				// Update with the specific stellar address
				stellarAddress := "GBXGQJWVLWOYHFLVTKWV5FGHA3LNYY2JQKM7OAJAUEQFU6LPCSEFVXON"
				err := models.ReceiverWallet.Update(ctx, rw.ID, data.ReceiverWalletUpdate{
					StellarAddress: stellarAddress,
				}, dbConnectionPool)
				require.NoError(t, err)

				req := CreateDirectPaymentRequest{
					Amount:   "10000.00",
					Asset:    AssetReference{ID: &asset.ID},
					Receiver: ReceiverReference{ID: &receiver.ID},
					Wallet:   WalletReference{Address: &stellarAddress},
				}
				return req, receiver
			},
			distributionAccount: &schema.TransactionAccount{Type: schema.DistributionAccountStellarEnv},
			setupMocks: func(distService *mocks.MockDistributionAccountService, eventProducer *events.MockProducer) {
				eventProducer.On("WriteMessages", mock.Anything, mock.Anything).Return(nil)
			},
			assertResult: func(t *testing.T, payment *data.Payment, err error) {
				require.NoError(t, err)
				assert.NotNil(t, payment)
				assert.Equal(t, "10000.0000000", payment.Amount)
				assert.Equal(t, data.ReadyPaymentStatus, payment.Status)
			},
		},
		{
			name: "fails - receiver wallet not found",
			setupTestData: func() (CreateDirectPaymentRequest, *data.Receiver) {
				receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
					Email: "severus.agemman@ultramar.imperium",
				})
				// Note: NOT creating a receiver wallet

				req := CreateDirectPaymentRequest{
					Amount:   "100.00",
					Asset:    AssetReference{ID: &asset.ID},
					Receiver: ReceiverReference{ID: &receiver.ID},
					Wallet:   WalletReference{ID: &enabledWallet.ID},
				}
				return req, receiver
			},
			distributionAccount: &schema.TransactionAccount{Type: schema.DistributionAccountStellarDBVault},
			setupMocks:          func(distService *mocks.MockDistributionAccountService, eventProducer *events.MockProducer) {},
			assertResult: func(t *testing.T, payment *data.Payment, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "receiver wallet not found")
				assert.Contains(t, err.Error(), "receiver must be registered with this wallet")
			},
		},
		{
			name: "fails - receiver by email not found",
			setupTestData: func() (CreateDirectPaymentRequest, *data.Receiver) {
				req := CreateDirectPaymentRequest{
					Amount:   "100.00",
					Asset:    AssetReference{ID: &asset.ID},
					Receiver: ReceiverReference{Email: testutils.StringPtr("chaos@warp.void")},
					Wallet:   WalletReference{ID: &enabledWallet.ID},
				}
				return req, nil // No receiver to clean up
			},
			distributionAccount: &schema.TransactionAccount{Type: schema.DistributionAccountStellarDBVault},
			setupMocks:          func(*mocks.MockDistributionAccountService, *events.MockProducer) {},
			assertResult: func(t *testing.T, payment *data.Payment, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "no receiver found with contact info")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, receiver := tc.setupTestData()

			t.Cleanup(func() {
				data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
				if receiver != nil {
					data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
				}
			})

			service := NewDirectPaymentService(models, dbConnectionPool)

			mockDistService := &mocks.MockDistributionAccountService{}
			mockEventProducer := events.NewMockProducer(t)

			service.DistributionAccountService = mockDistService
			service.EventProducer = mockEventProducer

			tc.setupMocks(mockDistService, mockEventProducer)

			payment, err := service.CreateDirectPayment(ctx, req, user, tc.distributionAccount)
			tc.assertResult(t, payment, err)

			mockDistService.AssertExpectations(t)
			mockEventProducer.AssertExpectations(t)
		})
	}
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

	service := NewDirectPaymentService(models, dbConnectionPool)

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
				{asset1, "999.00", data.DraftPaymentStatus},   // ignored
				{asset1, "888.00", data.SuccessPaymentStatus}, // ignored
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
			name: "terminal and draft statuses ignored",
			payments: []payment{
				{asset1, "400.00", data.SuccessPaymentStatus},
				{asset1, "500.00", data.FailedPaymentStatus},
				{asset1, "600.00", data.CanceledPaymentStatus},
				{asset1, "700.00", data.DraftPaymentStatus},
				{asset1, "777.00", data.PausedPaymentStatus}, // counted
			},
			targetAsset:    asset1,
			expectedAmount: 777.00,
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
				tx.Rollback()
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
	ctx = tenant.SaveTenantInContext(ctx, &tenant.Tenant{ID: "battle-barge-001"})
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

	service := NewDirectPaymentService(models, dbConnectionPool)

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
	assert.Equal(t, data.PaymentTypeDirect, payment.PaymentType)
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
