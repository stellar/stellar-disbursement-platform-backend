package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/protocols/horizon"
	"github.com/stellar/go-stellar-sdk/protocols/horizon/base"
	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

func Test_DisbursementManagementService_GetDisbursementsWithCount(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	users := []*auth.User{
		{
			ID:        "john-doe",
			Email:     "john-doe@email.com",
			FirstName: "John",
			LastName:  "Doe",
		},
		{
			ID:        "jane-doe",
			Email:     "jane-doe@email.com",
			FirstName: "Jane",
			LastName:  "Doe",
		},
	}

	userRef := []UserReference{
		{
			ID:        users[0].ID,
			FirstName: users[0].FirstName,
			LastName:  users[0].LastName,
		},
		{
			ID:        users[1].ID,
			FirstName: users[1].FirstName,
			LastName:  users[1].LastName,
		},
	}

	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.
		On("GetUsersByID", mock.Anything, []string{users[0].ID, users[1].ID}, false).
		Return(users, nil)
	authManagerMock.
		On("GetUsersByID", mock.Anything, []string{users[1].ID, users[0].ID}, false).
		Return(users, nil)

	service := &DisbursementManagementService{
		Models:      models,
		AuthManager: authManagerMock,
	}

	ctx := context.Background()
	t.Run("disbursements list empty", func(t *testing.T) {
		resultWithTotal, err := service.GetDisbursementsWithCount(ctx, &data.QueryParams{})
		require.NoError(t, err)
		require.Equal(t, 0, resultWithTotal.Total)
		result, ok := resultWithTotal.Result.([]*data.Disbursement)
		require.True(t, ok)
		require.Equal(t, 0, len(result))
	})

	t.Run("get disbursements successfully", func(t *testing.T) {
		// create disbursements
		d1 := data.CreateDisbursementFixture(t, context.Background(), dbConnectionPool, models.Disbursements,
			&data.Disbursement{
				Name: "d1",
				StatusHistory: []data.DisbursementStatusHistoryEntry{
					{
						Status: data.DraftDisbursementStatus,
						UserID: users[0].ID,
					},
					{
						Status: data.StartedDisbursementStatus,
						UserID: users[1].ID,
					},
				},
			},
		)
		d2 := data.CreateDisbursementFixture(t, context.Background(), dbConnectionPool, models.Disbursements,
			&data.Disbursement{
				Name: "d2",
				StatusHistory: []data.DisbursementStatusHistoryEntry{
					{
						Status: data.DraftDisbursementStatus,
						UserID: users[1].ID,
					},
				},
			},
		)

		resultWithTotal, err := service.GetDisbursementsWithCount(ctx, &data.QueryParams{SortOrder: "asc", SortBy: "name"})
		require.NoError(t, err)
		require.Equal(t, 2, resultWithTotal.Total)
		result, ok := resultWithTotal.Result.([]*DisbursementWithUserMetadata)
		require.True(t, ok)
		require.Equal(t, 2, len(result))
		require.Equal(t, d1.ID, result[0].Disbursement.ID)
		require.Equal(t, d2.ID, result[1].Disbursement.ID)
		require.Equal(t, userRef[0], result[0].CreatedBy)
		require.Equal(t, userRef[1], result[0].StartedBy)
		require.Equal(t, userRef[1], result[1].CreatedBy)
		require.Equal(t, UserReference{}, result[1].StartedBy)
	})
}

func Test_DisbursementManagementService_GetDisbursementReceiversWithCount(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	service := DisbursementManagementService{Models: models}
	disbursement := data.CreateDisbursementFixture(t, context.Background(), dbConnectionPool, models.Disbursements, &data.Disbursement{})

	ctx := context.Background()
	t.Run("disbursements not found", func(t *testing.T) {
		resultWithTotal, err := service.GetDisbursementReceiversWithCount(ctx, "wrong-id", &data.QueryParams{})
		require.ErrorIs(t, err, ErrDisbursementNotFound)
		require.Nil(t, resultWithTotal)
	})

	t.Run("disbursements receivers list empty", func(t *testing.T) {
		resultWithTotal, err := service.GetDisbursementReceiversWithCount(ctx, disbursement.ID, &data.QueryParams{})
		require.NoError(t, err)
		require.Equal(t, 0, resultWithTotal.Total)
		result, ok := resultWithTotal.Result.([]*data.DisbursementReceiver)
		require.True(t, ok)
		require.Equal(t, 0, len(result))
	})

	t.Run("get disbursement receivers successfully", func(t *testing.T) {
		receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		rwDraft1 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, disbursement.Wallet.ID, data.DraftReceiversWalletStatus)
		rwDraft2 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, disbursement.Wallet.ID, data.DraftReceiversWalletStatus)
		data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rwDraft1,
			Disbursement:   disbursement,
			Asset:          *disbursement.Asset,
			Amount:         "100",
			Status:         data.DraftPaymentStatus,
		})
		data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rwDraft2,
			Disbursement:   disbursement,
			Asset:          *disbursement.Asset,
			Amount:         "200",
			Status:         data.DraftPaymentStatus,
		})

		resultWithTotal, err := service.GetDisbursementReceiversWithCount(ctx, disbursement.ID, &data.QueryParams{})
		require.NoError(t, err)
		require.Equal(t, 2, resultWithTotal.Total)
		result, ok := resultWithTotal.Result.([]*data.DisbursementReceiver)
		require.True(t, ok)
		require.Equal(t, 2, len(result))
	})
}

func Test_DisbursementManagementService_StartDisbursement_success(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()
	ctx := context.Background()

	// Create models and basic DB entries
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	// Create fixtures: asset, wallet
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, assets.EURCAssetCode, assets.EURCAssetIssuerTestnet)
	wallet := data.CreateDefaultWalletFixture(t, ctx, dbConnectionPool)

	// Update context with tenant and auth token
	tnt := schema.Tenant{ID: "tenant-id"}
	ctx = sdpcontext.SetTenantInContext(context.Background(), &tnt)
	token := "token"
	ctx = sdpcontext.SetTokenInContext(ctx, token)

	// Create distribution accounts
	distributionAccPubKey := "GAAHIL6ZW4QFNLCKALZ3YOIWPP4TXQ7B7J5IU7RLNVGQAV6GFDZHLDTA"
	stellarDistAccountEnv := schema.NewStellarEnvTransactionAccount(distributionAccPubKey)
	stellarDistAccountDBVault := schema.NewDefaultStellarTransactionAccount(distributionAccPubKey)
	circleDistAccountDBVault := schema.TransactionAccount{
		CircleWalletID: "circle-wallet-id",
		Type:           schema.DistributionAccountCircleDBVault,
		Status:         schema.AccountStatusActive,
	}

	// IsOwner: these tests exercise transition mechanics; wallet-scoped authorization has its
	// own dedicated suite (Test_WalletScopedAuthorization).
	ownerUser := &auth.User{ID: "owner-user", Email: "owner@test.com", IsOwner: true}
	financialUser := &auth.User{ID: "financial-user", Email: "financial@test.com", IsOwner: true}

	// Shared mocks preparation
	prepareHorizonMockFn := func(mHorizonClient *horizonclient.MockClient) {
		mHorizonClient.
			On("AccountDetail", horizonclient.AccountRequest{AccountID: distributionAccPubKey}).
			Return(horizon.Account{
				Balances: []horizon.Balance{
					{
						Balance: "10000000",
						Asset:   base.Asset{Code: asset.Code, Issuer: asset.Issuer},
					},
				},
			}, nil).
			Once()
	}
	prepareCircleServiceMockFn := func(mCircleService *circle.MockService) {
		mCircleService.
			On("GetBusinessBalances", mock.Anything).
			Return(&circle.Balances{
				Available: []circle.Balance{
					{Currency: "EUR", Amount: "10000000.0"},
				},
			}, nil).
			Once()
	}

	testCases := []struct {
		name                string
		distributionAccount schema.TransactionAccount
		prepareMocksFn      func(mHorizonClient *horizonclient.MockClient, mCircleService *circle.MockService)
		approvalFlowEnabled bool
	}{
		{
			name:                "[DISTRIBUTION_ACCOUNT.STELLAR.ENV]successfully starts a disbursement",
			distributionAccount: stellarDistAccountEnv,
			approvalFlowEnabled: false,
			prepareMocksFn: func(mHorizonClient *horizonclient.MockClient, _ *circle.MockService) {
				prepareHorizonMockFn(mHorizonClient)
			},
		},
		{
			name:                "[DISTRIBUTION_ACCOUNT.STELLAR.ENV](APPROVAL_FLOW)successfully starts a disbursement",
			distributionAccount: stellarDistAccountEnv,
			approvalFlowEnabled: true,
			prepareMocksFn: func(mHorizonClient *horizonclient.MockClient, _ *circle.MockService) {
				prepareHorizonMockFn(mHorizonClient)
			},
		},
		{
			name:                "[DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT]successfully starts a disbursement",
			distributionAccount: stellarDistAccountDBVault,
			approvalFlowEnabled: false,
			prepareMocksFn: func(mHorizonClient *horizonclient.MockClient, _ *circle.MockService) {
				prepareHorizonMockFn(mHorizonClient)
			},
		},
		{
			name:                "[DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT](APPROVAL_FLOW)successfully starts a disbursement",
			distributionAccount: stellarDistAccountDBVault,
			approvalFlowEnabled: true,
			prepareMocksFn: func(mHorizonClient *horizonclient.MockClient, _ *circle.MockService) {
				prepareHorizonMockFn(mHorizonClient)
			},
		},
		{
			name:                "[DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT]successfully starts a disbursement",
			distributionAccount: circleDistAccountDBVault,
			approvalFlowEnabled: false,
			prepareMocksFn: func(mHorizonClient *horizonclient.MockClient, mCircleService *circle.MockService) {
				prepareCircleServiceMockFn(mCircleService)
			},
		},
		{
			name:                "[DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT](APPROVAL_FLOW)successfully starts a disbursement",
			distributionAccount: circleDistAccountDBVault,
			approvalFlowEnabled: true,
			prepareMocksFn: func(mHorizonClient *horizonclient.MockClient, mCircleService *circle.MockService) {
				prepareCircleServiceMockFn(mCircleService)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer data.DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)
			defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
			defer data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
			defer data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)

			user := ownerUser
			if tc.approvalFlowEnabled {
				user = financialUser

				// Enable approval workflow for org.
				isApprovalRequired := true
				err = models.Organizations.Update(ctx, &data.OrganizationUpdate{IsApprovalRequired: &isApprovalRequired})
				require.NoError(t, err)
				// rollback changes
				defer func() {
					isApprovalRequired = false
					err = models.Organizations.Update(ctx, &data.OrganizationUpdate{IsApprovalRequired: &isApprovalRequired})
					require.NoError(t, err)
				}()
			}

			// Create fixtures: disbursements
			readyDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
				Name:   "ready disbursement",
				Status: data.ReadyDisbursementStatus,
				Asset:  asset,
				Wallet: wallet,
				StatusHistory: []data.DisbursementStatusHistoryEntry{
					{UserID: ownerUser.ID, Status: data.DraftDisbursementStatus},
					{UserID: ownerUser.ID, Status: data.ReadyDisbursementStatus},
				},
			})

			// Create fixtures: receivers & receiver wallets
			// rDraft represents a receiver that is being added to the system for the first time
			rDraft := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
			rwDraft := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, rDraft.ID, wallet.ID, data.DraftReceiversWalletStatus)
			// rReady represents a receiver that is already in the systrem but doesn't have a Stellar wallet yet (didn't do SEP-24)
			rReady := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
			rwReady := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, rReady.ID, wallet.ID, data.ReadyReceiversWalletStatus)
			// rRegistered represents a receiver that is already in the system and has a Stellar wallet
			rRegistered := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
			rwRegistered := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, rRegistered.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

			receiverIDs := []string{rDraft.ID, rReady.ID, rRegistered.ID}
			t.Log(receiverIDs)

			// Create fixtures: payments
			pDraft := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
				ReceiverWallet: rwDraft,
				Disbursement:   readyDisbursement,
				Asset:          *asset,
				Amount:         "100",
				Status:         data.DraftPaymentStatus,
			})
			pReady := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
				ReceiverWallet: rwReady,
				Disbursement:   readyDisbursement,
				Asset:          *asset,
				Amount:         "200",
				Status:         data.DraftPaymentStatus,
			})
			pRegistered := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
				ReceiverWallet: rwRegistered,
				Disbursement:   readyDisbursement,
				Asset:          *asset,
				Amount:         "300",
				Status:         data.DraftPaymentStatus,
			})

			payments := []*data.Payment{pDraft, pReady, pRegistered}
			t.Log(payments)

			// Create mocks: call prepareMocksFn
			mHorizonClient := &horizonclient.MockClient{}
			defer mHorizonClient.AssertExpectations(t)
			mCircleService := circle.NewMockService(t)
			tc.prepareMocksFn(mHorizonClient, mCircleService)

			// Setup dependent services
			distAccSvc, err := NewDistributionAccountService(DistributionAccountServiceOptions{
				HorizonClient: mHorizonClient,
				CircleService: mCircleService,
				NetworkType:   utils.TestnetNetworkType,
			})
			require.NoError(t, err)
			service := &DisbursementManagementService{
				Models:                     models,
				DistributionAccountService: distAccSvc,
			}

			// 🚧 StartDisbursement
			err = service.StartDisbursement(ctx, readyDisbursement.ID, user, &tc.distributionAccount)
			require.NoError(t, err)

			// 👀 Assert status: Disbursement
			updatedDisbursement, err := models.Disbursements.Get(ctx, dbConnectionPool, readyDisbursement.ID)
			require.NoError(t, err)
			assert.Equal(t, data.StartedDisbursementStatus, updatedDisbursement.Status)
			assert.Equal(t, user.ID, updatedDisbursement.StatusHistory[2].UserID)
			assert.Equal(t, data.StartedDisbursementStatus, updatedDisbursement.StatusHistory[2].Status)

			// 👀 Assert status: ReceiverWallets
			receiverWallets, err := models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, models.DBConnectionPool, receiverIDs, wallet.ID)
			require.NoError(t, err)
			require.Equal(t, 3, len(receiverWallets))
			rwExpectedStatuses := map[string]data.ReceiversWalletStatus{
				rwDraft.ID:      data.ReadyReceiversWalletStatus,
				rwReady.ID:      data.ReadyReceiversWalletStatus,
				rwRegistered.ID: data.RegisteredReceiversWalletStatus,
			}
			for _, rw := range receiverWallets {
				require.Equal(t, rwExpectedStatuses[rw.ID], rw.Status)
			}

			// 👀 Assert status: Payments
			for _, p := range payments {
				payment, err := models.Payment.Get(ctx, p.ID, dbConnectionPool)
				require.NoError(t, err)
				require.Equal(t, data.ReadyPaymentStatus, payment.Status)
			}
		})
	}
}

func Test_DisbursementManagementService_StartDisbursement_failure(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	tnt := schema.Tenant{ID: "tenant-id"}
	ctx := sdpcontext.SetTenantInContext(context.Background(), &tnt)
	token := "token"
	ctx = sdpcontext.SetTokenInContext(ctx, token)

	// Create fixtures: asset, wallet
	asset := data.GetAssetFixture(t, ctx, dbConnectionPool, data.FixtureAssetUSDC)
	distributionAccPubKey := "GAAHIL6ZW4QFNLCKALZ3YOIWPP4TXQ7B7J5IU7RLNVGQAV6GFDZHLDTA"
	distributionAcc := schema.NewDefaultStellarTransactionAccount(distributionAccPubKey)

	// create fixtures
	wallet := data.CreateDefaultWalletFixture(t, ctx, dbConnectionPool)

	// Failure-path actor: owner bypasses wallet-scoped authorization, so each subtest reaches
	// its intended failure (authz itself is covered by Test_WalletScopedAuthorization).
	failureUser := &auth.User{ID: "failure-user", Email: "failure@test.com", IsOwner: true}

	// Create fixtures: disbursements
	draftDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:   "draft disbursement",
		Status: data.DraftDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
	})

	// Create fixtures: receivers, receiver wallets
	receiverReady := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	rwReady := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverReady.ID, wallet.ID, data.ReadyReceiversWalletStatus)

	hAccRequest := horizonclient.AccountRequest{AccountID: distributionAccPubKey}

	t.Run("returns an error if the disbursement doesn't exist", func(t *testing.T) {
		service := DisbursementManagementService{Models: models}

		err = service.StartDisbursement(context.Background(), "not-found-id", nil, &distributionAcc)
		require.ErrorIs(t, err, ErrDisbursementNotFound)
	})

	t.Run("returns an error if the disbursement's wallet is disabled", func(t *testing.T) {
		service := DisbursementManagementService{Models: models}

		data.EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, false, wallet.ID)
		defer data.EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, true, wallet.ID)
		err = service.StartDisbursement(context.Background(), draftDisbursement.ID, failureUser, &distributionAcc)
		require.ErrorIs(t, err, ErrDisbursementWalletDisabled)
	})

	t.Run("returns an error if the disbursement status is not READY", func(t *testing.T) {
		service := DisbursementManagementService{Models: models}

		err = service.StartDisbursement(context.Background(), draftDisbursement.ID, failureUser, &distributionAcc)
		require.ErrorIs(t, err, ErrDisbursementNotReadyToStart)
	})

	t.Run("(APPROVAL FLOW ENABLED) returns an error if the disbursement is started by its creator", func(t *testing.T) {
		service := DisbursementManagementService{Models: models}

		userID := "9ae68f09-cad9-4311-9758-4ff59d2e9e6d"
		disbursement := data.CreateDisbursementFixture(t, context.Background(), dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name:   "disbursement #1",
			Status: data.ReadyDisbursementStatus,
			Asset:  asset,
			Wallet: wallet,
			StatusHistory: []data.DisbursementStatusHistoryEntry{
				{
					Status: data.DraftDisbursementStatus,
					UserID: userID,
				},
				{
					Status: data.ReadyDisbursementStatus,
					UserID: userID,
				},
			},
		})

		user := &auth.User{
			ID:      userID,
			Email:   "email@email.com",
			IsOwner: true, // wallet authz covered by Test_WalletScopedAuthorization
		}

		// Enable approval workflow for org.
		isApprovalRequired := true
		err = models.Organizations.Update(ctx, &data.OrganizationUpdate{IsApprovalRequired: &isApprovalRequired})
		require.NoError(t, err)

		err = service.StartDisbursement(ctx, disbursement.ID, user, &distributionAcc)
		require.ErrorIs(t, err, ErrDisbursementStartedByCreator)

		// rollback changes
		isApprovalRequired = false
		err = models.Organizations.Update(ctx, &data.OrganizationUpdate{IsApprovalRequired: &isApprovalRequired})
		require.NoError(t, err)
	})

	t.Run("returns an error if the distribution account has insufficient balance", func(t *testing.T) {
		usdt := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDT", "GBVHJTRLQRMIHRYTXZQOPVYCVVH7IRJN3DOFT7VC6U75CBWWBVDTWURG")

		disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name:   "disbursement - balance insufficient",
			Status: data.StartedDisbursementStatus,
			Asset:  usdt,
			Wallet: wallet,
		})
		// should consider this payment since it's the same asset
		data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rwReady,
			Disbursement:   disbursement,
			Asset:          *usdt,
			Amount:         "1100",
			Status:         data.PendingPaymentStatus,
		})

		disbursement2 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name:   "disbursement #4",
			Status: data.StartedDisbursementStatus,
			Asset:  asset,
			Wallet: wallet,
		})
		// should NOT consider this payment since it's NOT the same asset
		data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rwReady,
			Disbursement:   disbursement2,
			Asset:          *asset,
			Amount:         "5555555",
			Status:         data.PendingPaymentStatus,
		})

		disbursementInsufficientBalance := data.CreateDisbursementFixture(t, context.Background(), dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name:   "disbursement - insufficient balance",
			Status: data.ReadyDisbursementStatus,
			Asset:  usdt,
			Wallet: wallet,
		})
		data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rwReady,
			Disbursement:   disbursementInsufficientBalance,
			Asset:          *usdt,
			Amount:         "22222",
			Status:         data.ReadyPaymentStatus,
		})

		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		// Create Mocks
		hMock := &horizonclient.MockClient{}
		defer hMock.AssertExpectations(t)
		hMock.On("AccountDetail", hAccRequest).Return(horizon.Account{
			Balances: []horizon.Balance{
				{
					Balance: "11111",
					Asset: base.Asset{
						Code:   usdt.Code,
						Issuer: usdt.Issuer,
					},
				},
			},
		}, nil).Once()

		// Setup dependent services
		distAccSvc, err := NewDistributionAccountService(DistributionAccountServiceOptions{
			HorizonClient: hMock,
			CircleService: &circle.Service{},
			NetworkType:   utils.TestnetNetworkType,
		})
		require.NoError(t, err)

		// Create service
		service := &DisbursementManagementService{
			Models:                     models,
			DistributionAccountService: distAccSvc,
		}

		err = service.StartDisbursement(ctx, disbursementInsufficientBalance.ID, failureUser, &distributionAcc)
		expectedErr := InsufficientBalanceError{
			DisbursementAsset:   *usdt,
			DistributionAddress: distributionAcc.ID(),
			DisbursementID:      disbursementInsufficientBalance.ID,
			AvailableBalance:    decimal.NewFromFloat(11111.0),
			DisbursementAmount:  decimal.NewFromFloat(22222.0),
			TotalPendingAmount:  decimal.NewFromFloat(1100.0),
		}

		require.ErrorContains(t, err, fmt.Sprintf("validating balance for disbursement: %v", expectedErr))

		// PendingTotal includes payments associated with 'readyDisbursement' that were moved from the draft to ready status
		expectedErrStr := fmt.Sprintf("the disbursement %s failed due to an account balance (11111.00) that was insufficient to fulfill new amount (22222.00) along with the pending amount (1100.00). To complete this action, your distribution account (stellar:GAAHIL6ZW4QFNLCKALZ3YOIWPP4TXQ7B7J5IU7RLNVGQAV6GFDZHLDTA) needs to be recharged with at least 12211.00 USDT", disbursementInsufficientBalance.ID)
		assert.Contains(t, buf.String(), expectedErrStr)
	})
}

// Test_DisbursementManagementService_StartDisbursement_multiWalletIsolation is a regression
// test for a real bug found live: validateBalanceForDisbursement's "pending amount" query had
// no wallet filter at all, so a busy/over-committed wallet A could wrongly block a disbursement
// on a completely separate, healthy wallet B just by tenant-wide coincidence. Two wallets, two
// disbursements: wallet A is deliberately over-committed and must fail; wallet B has ample
// headroom and must succeed even though wallet A's pending amount alone would exceed it.
func Test_DisbursementManagementService_StartDisbursement_multiWalletIsolation(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	asset := data.GetAssetFixture(t, ctx, dbConnectionPool, data.FixtureAssetUSDC)
	wallet := data.CreateDefaultWalletFixture(t, ctx, dbConnectionPool)
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	rwReady := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

	user := &auth.User{ID: "owner-user", Email: "owner@test.com", IsOwner: true}

	walletAAddress := "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV"
	walletBAddress := "GBVHJTRLQRMIHRYTXZQOPVYCVVH7IRJN3DOFT7VC6U75CBWWBVDTWURG"

	distWalletA, err := models.DistributionWallets.Insert(ctx, dbConnectionPool, data.DistributionWalletInsert{
		Name: "Wallet A (over-committed)", AccountType: schema.DistributionAccountStellarDBVault,
	})
	require.NoError(t, err)
	distWalletA, err = models.DistributionWallets.UpdateAddress(ctx, dbConnectionPool, distWalletA.ID, walletAAddress)
	require.NoError(t, err)

	distWalletB, err := models.DistributionWallets.Insert(ctx, dbConnectionPool, data.DistributionWalletInsert{
		Name: "Wallet B (healthy)", AccountType: schema.DistributionAccountStellarDBVault,
	})
	require.NoError(t, err)
	distWalletB, err = models.DistributionWallets.UpdateAddress(ctx, dbConnectionPool, distWalletB.ID, walletBAddress)
	require.NoError(t, err)

	// Wallet A: an existing in-progress payment (80) already commits most of its balance (100).
	disbursementA := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name: "wallet A - existing commitment", Status: data.StartedDisbursementStatus,
		Asset: asset, Wallet: wallet, SourceWalletID: distWalletA.ID,
	})
	data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwReady, Disbursement: disbursementA, Asset: *asset,
		Amount: "80", Status: data.PendingPaymentStatus,
	})
	newDisbursementA := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name: "wallet A - new (should fail)", Status: data.ReadyDisbursementStatus,
		Asset: asset, Wallet: wallet, SourceWalletID: distWalletA.ID,
	})
	data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwReady, Disbursement: newDisbursementA, Asset: *asset,
		Amount: "30", Status: data.ReadyPaymentStatus,
	})

	// Wallet B: no pre-existing commitments; a modest new disbursement fits comfortably.
	newDisbursementB := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name: "wallet B - new (should succeed)", Status: data.ReadyDisbursementStatus,
		Asset: asset, Wallet: wallet, SourceWalletID: distWalletB.ID,
	})
	data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwReady, Disbursement: newDisbursementB, Asset: *asset,
		Amount: "25", Status: data.ReadyPaymentStatus,
	})

	mockDistAccSvc := mocks.NewMockDistributionAccountService(t)
	// Wallet A holds 100; Wallet B holds 100 too - but only A has a pre-existing 80 commitment.
	mockDistAccSvc.On("GetBalance", mock.Anything,
		mock.MatchedBy(func(a *schema.TransactionAccount) bool { return a.Address == walletAAddress }),
		mock.AnythingOfType("data.Asset")).
		Return(decimal.NewFromInt(100), nil)
	mockDistAccSvc.On("GetBalance", mock.Anything,
		mock.MatchedBy(func(a *schema.TransactionAccount) bool { return a.Address == walletBAddress }),
		mock.AnythingOfType("data.Asset")).
		Return(decimal.NewFromInt(100), nil)

	service := &DisbursementManagementService{
		Models:                     models,
		DistributionAccountService: mockDistAccSvc,
	}

	accountA := &schema.TransactionAccount{Address: walletAAddress, Type: schema.DistributionAccountStellarDBVault}
	accountB := &schema.TransactionAccount{Address: walletBAddress, Type: schema.DistributionAccountStellarDBVault}

	t.Run("wallet A: over-committed, correctly fails", func(t *testing.T) {
		err := service.StartDisbursement(ctx, newDisbursementA.ID, user, accountA)
		require.Error(t, err)
		require.ErrorContains(t, err, "insufficient")
		require.ErrorContains(t, err, walletAAddress)
	})

	t.Run("wallet B: healthy, succeeds even though wallet A alone is already over its own balance", func(t *testing.T) {
		err := service.StartDisbursement(ctx, newDisbursementB.ID, user, accountB)
		require.NoError(t, err)
	})
}

func Test_DisbursementManagementService_PauseDisbursement(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	models, outerErr := data.NewModels(dbConnectionPool)
	require.NoError(t, outerErr)

	ctx := context.Background()

	tnt := schema.Tenant{ID: "tenant-id"}
	ctx = sdpcontext.SetTenantInContext(ctx, &tnt)

	token := "token"
	ctx = sdpcontext.SetTokenInContext(ctx, token)

	user := &auth.User{
		ID:      "user-id",
		Email:   "email@email.com",
		IsOwner: true, // wallet authz covered by Test_WalletScopedAuthorization
	}

	asset := data.GetAssetFixture(t, ctx, dbConnectionPool, data.FixtureAssetUSDC)

	hMock := &horizonclient.MockClient{}
	distributionAccPubKey := "ABC"
	distributionAcc := schema.NewDefaultStellarTransactionAccount(distributionAccPubKey)
	distAccSvc, err := NewDistributionAccountService(DistributionAccountServiceOptions{
		HorizonClient: hMock,
		CircleService: &circle.Service{},
		NetworkType:   utils.TestnetNetworkType,
	})
	require.NoError(t, err)

	service := &DisbursementManagementService{
		Models:                     models,
		DistributionAccountService: distAccSvc,
	}

	// create fixtures
	wallet := data.CreateDefaultWalletFixture(t, ctx, dbConnectionPool)

	// create disbursements
	readyDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:   "ready disbursement",
		Status: data.ReadyDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
	})

	startedDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:   "started disbursement",
		Status: data.StartedDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
	})

	// create disbursement receivers
	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver3 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver4 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})

	rwRegistered1 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, data.RegisteredReceiversWalletStatus)
	rwRegistered2 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, data.RegisteredReceiversWalletStatus)
	rwRegistered3 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver3.ID, wallet.ID, data.RegisteredReceiversWalletStatus)
	rwRegistered4 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver4.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	paymentPending1 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwRegistered1,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "100",
		Status:         data.PendingPaymentStatus,
	})
	paymentPending2 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwRegistered2,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "200",
		Status:         data.PendingPaymentStatus,
	})
	paymentReady1 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwRegistered3,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "300",
		Status:         data.ReadyPaymentStatus,
	})
	paymentReady2 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwRegistered4,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "400",
		Status:         data.ReadyPaymentStatus,
	})

	t.Run("disbursement doesn't exist", func(t *testing.T) {
		id := "5e1f1c7f5b6c9c0001c1b1b1"

		err := service.PauseDisbursement(ctx, id, user)
		require.ErrorIs(t, err, ErrDisbursementNotFound)
	})

	t.Run("disbursement not ready to pause", func(t *testing.T) {
		err := service.PauseDisbursement(ctx, readyDisbursement.ID, user)
		require.ErrorIs(t, err, ErrDisbursementNotReadyToPause)
	})

	t.Run("disbursement paused", func(t *testing.T) {
		hMock.On(
			"AccountDetail", horizonclient.AccountRequest{AccountID: distributionAccPubKey},
		).Return(horizon.Account{
			Balances: []horizon.Balance{
				{
					Balance: "10000",
					Asset: base.Asset{
						Code:   asset.Code,
						Issuer: asset.Issuer,
					},
				},
			},
		}, nil).Once()

		err := service.PauseDisbursement(ctx, startedDisbursement.ID, user)
		require.NoError(t, err)

		// check disbursement status
		disbursement, err := models.Disbursements.Get(ctx, models.DBConnectionPool, startedDisbursement.ID)
		require.NoError(t, err)
		require.Equal(t, data.PausedDisbursementStatus, disbursement.Status)

		// check pending payments are still pending.
		for _, p := range []*data.Payment{paymentPending1, paymentPending2} {
			payment, innerErr := models.Payment.Get(ctx, p.ID, dbConnectionPool)
			require.NoError(t, innerErr)
			require.Equal(t, data.PendingPaymentStatus, payment.Status)
		}

		// check ready payments are paused.
		for _, p := range []*data.Payment{paymentReady1, paymentReady2} {
			payment, innerErr := models.Payment.Get(ctx, p.ID, dbConnectionPool)
			require.NoError(t, innerErr)
			require.Equal(t, data.PausedPaymentStatus, payment.Status)
		}

		// change the disbursement back to started
		err = service.StartDisbursement(ctx, startedDisbursement.ID, user, &distributionAcc)
		require.NoError(t, err)

		// check disbursement is started again
		disbursement, err = models.Disbursements.Get(ctx, models.DBConnectionPool, startedDisbursement.ID)
		require.NoError(t, err)
		require.Equal(t, data.StartedDisbursementStatus, disbursement.Status)
	})

	t.Run("start -> pause -> start -> pause", func(t *testing.T) {
		hMock.On(
			"AccountDetail", horizonclient.AccountRequest{AccountID: distributionAccPubKey},
		).Return(horizon.Account{
			Balances: []horizon.Balance{
				{
					Balance: "10000",
					Asset: base.Asset{
						Code:   asset.Code,
						Issuer: asset.Issuer,
					},
				},
			},
		}, nil).Once()

		// 1. Pause Disbursement
		err := service.PauseDisbursement(ctx, startedDisbursement.ID, user)
		require.NoError(t, err)

		// check disbursement is paused
		disbursement, err := models.Disbursements.Get(ctx, models.DBConnectionPool, startedDisbursement.ID)
		require.NoError(t, err)
		require.Equal(t, data.PausedDisbursementStatus, disbursement.Status)

		// check pending payments are still pending.
		for _, p := range []*data.Payment{paymentPending1, paymentPending2} {
			payment, innerErr := models.Payment.Get(ctx, p.ID, dbConnectionPool)
			require.NoError(t, innerErr)
			require.Equal(t, data.PendingPaymentStatus, payment.Status)
		}

		// check ready payments are paused.
		for _, p := range []*data.Payment{paymentReady1, paymentReady2} {
			payment, innerErr := models.Payment.Get(ctx, p.ID, dbConnectionPool)
			require.NoError(t, innerErr)
			require.Equal(t, data.PausedPaymentStatus, payment.Status)
		}

		// 2. Start disbursement again
		err = service.StartDisbursement(ctx, startedDisbursement.ID, user, &distributionAcc)
		require.NoError(t, err)

		// check disbursement is started again
		disbursement, err = models.Disbursements.Get(ctx, models.DBConnectionPool, startedDisbursement.ID)
		require.NoError(t, err)
		require.Equal(t, data.StartedDisbursementStatus, disbursement.Status)

		// check pending payments are still pending.
		for _, p := range []*data.Payment{paymentPending1, paymentPending2} {
			payment, innerErr := models.Payment.Get(ctx, p.ID, dbConnectionPool)
			require.NoError(t, innerErr)
			require.Equal(t, data.PendingPaymentStatus, payment.Status)
		}

		// check paused payments are back to ready.
		for _, p := range []*data.Payment{paymentReady1, paymentReady2} {
			payment, innerErr := models.Payment.Get(ctx, p.ID, dbConnectionPool)
			require.NoError(t, innerErr)
			require.Equal(t, data.ReadyPaymentStatus, payment.Status)
		}

		// 3. Pause disbursement again
		err = service.PauseDisbursement(ctx, startedDisbursement.ID, user)
		require.NoError(t, err)

		// check disbursement is paused
		disbursement, err = models.Disbursements.Get(ctx, models.DBConnectionPool, startedDisbursement.ID)
		require.NoError(t, err)
		require.Equal(t, data.PausedDisbursementStatus, disbursement.Status)

		// check pending payments are still pending.
		for _, p := range []*data.Payment{paymentPending1, paymentPending2} {
			payment, innerErr := models.Payment.Get(ctx, p.ID, dbConnectionPool)
			require.NoError(t, innerErr)
			require.Equal(t, data.PendingPaymentStatus, payment.Status)
		}

		// check ready payments are paused again.
		for _, p := range []*data.Payment{paymentReady1, paymentReady2} {
			payment, innerErr := models.Payment.Get(ctx, p.ID, dbConnectionPool)
			require.NoError(t, innerErr)
			require.Equal(t, data.PausedPaymentStatus, payment.Status)
		}
	})

	hMock.AssertExpectations(t)
}

// Test_DisbursementManagementService_CancelDisbursement covers the acceptance criteria for the
// bulk "cancel disbursement" action: it's only reachable from DRAFT/READY (never-started), it
// cancels all of the disbursement's own (DRAFT) payments along with the disbursement itself, and
// a STARTED disbursement is correctly rejected (PauseDisbursement is the right action there
// instead, since on-chain submissions may already be in flight). Wrong-wallet authorization is
// covered by Test_WalletScopedAuthorization.
func Test_DisbursementManagementService_CancelDisbursement(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	user := &auth.User{
		ID:      "user-id",
		Email:   "email@email.com",
		IsOwner: true, // wallet authz covered by Test_WalletScopedAuthorization
	}

	asset := data.GetAssetFixture(t, ctx, dbConnectionPool, data.FixtureAssetUSDC)
	wallet := data.CreateDefaultWalletFixture(t, ctx, dbConnectionPool)

	service := &DisbursementManagementService{Models: models}

	t.Run("disbursement doesn't exist", func(t *testing.T) {
		err := service.CancelDisbursement(ctx, "5e1f1c7f5b6c9c0001c1b1b1", user)
		require.ErrorIs(t, err, ErrDisbursementNotFound)
	})

	t.Run("started disbursement can't be canceled", func(t *testing.T) {
		startedDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name: "started disbursement", Status: data.StartedDisbursementStatus, Asset: asset, Wallet: wallet,
		})

		err := service.CancelDisbursement(ctx, startedDisbursement.ID, user)
		require.ErrorIs(t, err, ErrDisbursementNotReadyToCancel)

		// State unchanged.
		got, gErr := models.Disbursements.Get(ctx, dbConnectionPool, startedDisbursement.ID)
		require.NoError(t, gErr)
		require.Equal(t, data.StartedDisbursementStatus, got.Status)
	})

	t.Run("paused disbursement can't be canceled", func(t *testing.T) {
		pausedDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name: "paused disbursement", Status: data.PausedDisbursementStatus, Asset: asset, Wallet: wallet,
		})

		err := service.CancelDisbursement(ctx, pausedDisbursement.ID, user)
		require.ErrorIs(t, err, ErrDisbursementNotReadyToCancel)
	})

	t.Run("draft disbursement is canceled along with its draft payments", func(t *testing.T) {
		draftDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name: "draft disbursement to cancel", Status: data.DraftDisbursementStatus, Asset: asset, Wallet: wallet,
		})
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.DraftReceiversWalletStatus)
		payment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rw, Disbursement: draftDisbursement, Asset: *asset, Amount: "42", Status: data.DraftPaymentStatus,
		})

		err := service.CancelDisbursement(ctx, draftDisbursement.ID, user)
		require.NoError(t, err)

		got, gErr := models.Disbursements.Get(ctx, dbConnectionPool, draftDisbursement.ID)
		require.NoError(t, gErr)
		require.Equal(t, data.CanceledDisbursementStatus, got.Status)

		gotPayment, pErr := models.Payment.Get(ctx, payment.ID, dbConnectionPool)
		require.NoError(t, pErr)
		require.Equal(t, data.CanceledPaymentStatus, gotPayment.Status)

		// Cancellation is terminal: canceling again is rejected, not silently re-applied.
		err = service.CancelDisbursement(ctx, draftDisbursement.ID, user)
		require.ErrorIs(t, err, ErrDisbursementNotReadyToCancel)
	})

	t.Run("ready disbursement (never started) is canceled along with its draft payments and stops counting toward wallet-balance accounting", func(t *testing.T) {
		readyDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name: "ready disbursement to cancel", Status: data.ReadyDisbursementStatus, Asset: asset, Wallet: wallet,
		})
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.DraftReceiversWalletStatus)
		payment1 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rw, Disbursement: readyDisbursement, Asset: *asset, Amount: "10", Status: data.DraftPaymentStatus,
		})
		payment2 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rw, Disbursement: readyDisbursement, Asset: *asset, Amount: "20", Status: data.DraftPaymentStatus,
		})

		err := service.CancelDisbursement(ctx, readyDisbursement.ID, user)
		require.NoError(t, err)

		got, gErr := models.Disbursements.Get(ctx, dbConnectionPool, readyDisbursement.ID)
		require.NoError(t, gErr)
		require.Equal(t, data.CanceledDisbursementStatus, got.Status)

		for _, p := range []*data.Payment{payment1, payment2} {
			gotPayment, pErr := models.Payment.Get(ctx, p.ID, dbConnectionPool)
			require.NoError(t, pErr)
			require.Equal(t, data.CanceledPaymentStatus, gotPayment.Status)
		}

		// Canceled payments never appear in "in progress" accounting - the exact mechanism
		// validateBalanceForDisbursement uses to compute pending commitments against a wallet's
		// balance for OTHER disbursements' start attempts (see the dedicated same-wallet test
		// below for why this specific disbursement's payments were never counted here to begin
		// with, cancellation or not).
		inProgress, ipErr := models.Payment.GetAll(ctx, &data.QueryParams{
			Filters: map[data.FilterKey]interface{}{
				data.FilterKeyStatus:          data.PaymentInProgressStatuses(),
				data.FilterKeySourceWalletIDs: []string{readyDisbursement.SourceWalletID},
			},
		}, dbConnectionPool, data.QueryTypeSelectAll)
		require.NoError(t, ipErr)
		for _, p := range inProgress {
			require.NotEqual(t, payment1.ID, p.ID)
			require.NotEqual(t, payment2.ID, p.ID)
		}

		// An event was recorded in the outbox for the cancellation (reusing the frozen
		// "disbursement.rejected" type - see ACTION-ITEMS.md for why no new event type was added).
		var eventCount int
		countErr := dbConnectionPool.GetContext(ctx, &eventCount,
			`SELECT count(*) FROM events WHERE event_type = 'disbursement.rejected' AND payload::text LIKE '%' || $1 || '%'`,
			readyDisbursement.ID)
		require.NoError(t, countErr)
		require.Equal(t, 1, eventCount)
	})
}

// Test_DisbursementManagementService_CancelDisbursement_sameWalletBalanceIsolation documents a
// real scope boundary: validateBalanceForDisbursement only counts payments in
// PaymentInProgressStatuses (READY/PENDING/PAUSED) as "pending" commitments against a wallet's
// balance - and a disbursement only reaches those payment statuses once it has been STARTED. A
// DRAFT/READY disbursement's payments are always DraftPaymentStatus, which was never counted in
// the first place. So canceling disbursement A (DRAFT/READY, never started) is provably inert
// with respect to whether disbursement B (on the SAME wallet) can start: the outcome for B is
// identical whether A is left alone or canceled. Genuine "release of reserved capacity" only
// applies to a disbursement that already reserved capacity by being STARTED, which is
// intentionally out of scope for cancellation (on-chain submissions may already be in flight) -
// see ACTION-ITEMS.md for the full write-up.
func Test_DisbursementManagementService_CancelDisbursement_sameWalletBalanceIsolation(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	asset := data.GetAssetFixture(t, ctx, dbConnectionPool, data.FixtureAssetUSDC)
	wallet := data.CreateDefaultWalletFixture(t, ctx, dbConnectionPool)
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	rwReady := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

	user := &auth.User{ID: "owner-user", Email: "owner@test.com", IsOwner: true}

	distWalletAddress := "GDGZWPLLHX7TQFRIZDCWMFDR6L5NHY4EOTZ7YMHDXRBMBYNPNW3ZIVQE"
	distWallet, err := models.DistributionWallets.Insert(ctx, dbConnectionPool, data.DistributionWalletInsert{
		Name: "Wallet (balance isolation)", AccountType: schema.DistributionAccountStellarDBVault,
	})
	require.NoError(t, err)
	distWallet, err = models.DistributionWallets.UpdateAddress(ctx, dbConnectionPool, distWallet.ID, distWalletAddress)
	require.NoError(t, err)

	// Balance is 100. Disbursement A (READY, never started) has an 80 draft payment - large
	// enough that it WOULD block disbursement B (30) if it counted, but it never does.
	disbursementA := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name: "A - not started, big amount", Status: data.ReadyDisbursementStatus,
		Asset: asset, Wallet: wallet, SourceWalletID: distWallet.ID,
	})
	data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwReady, Disbursement: disbursementA, Asset: *asset,
		Amount: "80", Status: data.DraftPaymentStatus,
	})

	newDisbursementBCount := 0
	newDisbursementB := func() *data.Disbursement {
		newDisbursementBCount++
		d := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name:   fmt.Sprintf("B - modest, should always fit #%d", newDisbursementBCount),
			Status: data.ReadyDisbursementStatus,
			Asset:  asset, Wallet: wallet, SourceWalletID: distWallet.ID,
		})
		data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rwReady, Disbursement: d, Asset: *asset,
			Amount: "30", Status: data.DraftPaymentStatus,
		})
		return d
	}

	mockDistAccSvc := mocks.NewMockDistributionAccountService(t)
	mockDistAccSvc.On("GetBalance", mock.Anything,
		mock.MatchedBy(func(a *schema.TransactionAccount) bool { return a.Address == distWalletAddress }),
		mock.AnythingOfType("data.Asset")).
		Return(decimal.NewFromInt(100), nil)

	service := &DisbursementManagementService{
		Models:                     models,
		DistributionAccountService: mockDistAccSvc,
	}
	account := &schema.TransactionAccount{Address: distWalletAddress, Type: schema.DistributionAccountStellarDBVault}

	disbursementBBefore := newDisbursementB()
	t.Run("B starts successfully with A left alone (A's draft payment was never counted)", func(t *testing.T) {
		err := service.StartDisbursement(ctx, disbursementBBefore.ID, user, account)
		require.NoError(t, err)
	})

	require.NoError(t, service.CancelDisbursement(ctx, disbursementA.ID, user))

	disbursementBAfter := newDisbursementB()
	t.Run("B starts successfully after A is canceled too - identical outcome", func(t *testing.T) {
		err := service.StartDisbursement(ctx, disbursementBAfter.ID, user, account)
		require.NoError(t, err)
	})
}

func Test_DisbursementManagementService_validateBalanceForDisbursement(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	// Create fixtures
	models, outerErr := data.NewModels(dbConnectionPool)
	require.NoError(t, outerErr)
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")
	receiverReady := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	rwReady := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverReady.ID, wallet.ID, data.ReadyReceiversWalletStatus)
	disbursementOld := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Wallet: wallet,
		Status: data.ReadyDisbursementStatus,
		Asset:  asset,
	})
	_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwReady,
		Disbursement:   disbursementOld,
		Asset:          *asset,
		Amount:         "8",
		Status:         data.PendingPaymentStatus,
	})
	// Direct Payment
	_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwReady,
		Asset:          *asset,
		Amount:         "2",
		Type:           data.PaymentTypeDirect,
		Status:         data.ReadyPaymentStatus,
	})
	disbursementNew := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Wallet: wallet,
		Status: data.ReadyDisbursementStatus,
		Asset:  asset,
	})
	_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwReady,
		Disbursement:   disbursementNew,
		Asset:          *asset,
		Amount:         "90",
		Status:         data.DraftPaymentStatus,
	})
	disbursementNew, err := models.Disbursements.GetWithStatistics(ctx, disbursementNew.ID)
	require.NoError(t, err)

	// Create distribution accounts
	distributionAccPubKey := "GAAHIL6ZW4QFNLCKALZ3YOIWPP4TXQ7B7J5IU7RLNVGQAV6GFDZHLDTA"
	stellarDistAccountEnv := schema.NewStellarEnvTransactionAccount(distributionAccPubKey)
	stellarDistAccountDBVault := schema.NewDefaultStellarTransactionAccount(distributionAccPubKey)
	circleDistAccountDBVault := schema.TransactionAccount{
		CircleWalletID: "circle-wallet-id",
		Type:           schema.DistributionAccountCircleDBVault,
		Status:         schema.AccountStatusActive,
	}

	expectedInsufficientBalanceErr := func(account schema.TransactionAccount) InsufficientBalanceError {
		return InsufficientBalanceError{
			DisbursementAsset:   *asset,
			DistributionAddress: account.ID(),
			DisbursementID:      disbursementNew.ID,
			AvailableBalance:    decimal.NewFromFloat(99.99),
			DisbursementAmount:  decimal.NewFromFloat(90.00),
			TotalPendingAmount:  decimal.NewFromFloat(10.00),
		}
	}

	// test cases
	testCases := []struct {
		name                string
		disbursementAccount schema.TransactionAccount
		prepareMocksFn      func(mDistAccService *mocks.MockDistributionAccountService)
		availableBalance    string
		expectedErrContains string
	}{
		{
			name:                "return an error when GetBalance fails",
			disbursementAccount: stellarDistAccountEnv,
			prepareMocksFn: func(mDistAccService *mocks.MockDistributionAccountService) {
				mDistAccService.
					On("GetBalance", ctx, &stellarDistAccountEnv, *asset).
					Return(decimal.Zero, errors.New("GetBalance error")).
					Once()
			},
			expectedErrContains: fmt.Sprintf("getting balance for asset (%s,%s) on distribution account %v: GetBalance error", asset.Code, asset.Issuer, stellarDistAccountEnv),
		},
		{
			name:                "🔴[DISTRIBUTION_ACCOUNT.STELLAR.ENV] insufficient ballance for disbursement",
			disbursementAccount: stellarDistAccountEnv,
			prepareMocksFn: func(mDistAccService *mocks.MockDistributionAccountService) {
				mDistAccService.
					On("GetBalance", ctx, &stellarDistAccountEnv, *asset).
					Return(decimal.NewFromFloat(99.99), nil).
					Once()
			},
			expectedErrContains: expectedInsufficientBalanceErr(stellarDistAccountEnv).Error(),
		},
		{
			name:                "🔴[DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT] insufficient ballance for disbursement",
			disbursementAccount: stellarDistAccountDBVault,
			prepareMocksFn: func(mDistAccService *mocks.MockDistributionAccountService) {
				mDistAccService.
					On("GetBalance", ctx, &stellarDistAccountDBVault, *asset).
					Return(decimal.NewFromFloat(99.99), nil).
					Once()
			},
			expectedErrContains: expectedInsufficientBalanceErr(stellarDistAccountDBVault).Error(),
		},
		{
			name:                "🔴[DISTRIBUTION_ACCOUNT.CIRCLE_DB_VAULT] insufficient ballance for disbursement",
			disbursementAccount: circleDistAccountDBVault,
			prepareMocksFn: func(mDistAccService *mocks.MockDistributionAccountService) {
				mDistAccService.
					On("GetBalance", ctx, &circleDistAccountDBVault, *asset).
					Return(decimal.NewFromFloat(99.99), nil).
					Once()
			},
			expectedErrContains: expectedInsufficientBalanceErr(circleDistAccountDBVault).Error(),
		},
		{
			name:                "🟢[DISTRIBUTION_ACCOUNT.STELLAR.ENV] successfully validate ballance for disbursement",
			disbursementAccount: stellarDistAccountEnv,
			prepareMocksFn: func(mDistAccService *mocks.MockDistributionAccountService) {
				mDistAccService.
					On("GetBalance", ctx, &stellarDistAccountEnv, *asset).
					Return(decimal.NewFromFloat(100.00), nil).
					Once()
			},
		},
		{
			name:                "🟢[DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT] successfully validate ballance for disbursement",
			disbursementAccount: stellarDistAccountDBVault,
			prepareMocksFn: func(mDistAccService *mocks.MockDistributionAccountService) {
				mDistAccService.
					On("GetBalance", ctx, &stellarDistAccountDBVault, *asset).
					Return(decimal.NewFromFloat(100.00), nil).
					Once()
			},
		},
		{
			name:                "🟢[DISTRIBUTION_ACCOUNT.CIRCLE_DB_VAULT] successfully validate ballance for disbursement",
			disbursementAccount: circleDistAccountDBVault,
			prepareMocksFn: func(mDistAccService *mocks.MockDistributionAccountService) {
				mDistAccService.
					On("GetBalance", ctx, &circleDistAccountDBVault, *asset).
					Return(decimal.NewFromFloat(100.00), nil).
					Once()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
			require.NoError(t, err)
			defer func() {
				err = dbTx.Rollback()
				require.NoError(t, err)
			}()

			mDistAccService := mocks.NewMockDistributionAccountService(t)
			tc.prepareMocksFn(mDistAccService)
			svc := &DisbursementManagementService{
				Models:                     models,
				DistributionAccountService: mDistAccService,
			}

			err = svc.validateBalanceForDisbursement(ctx, dbTx, &tc.disbursementAccount, disbursementNew)

			if tc.expectedErrContains != "" {
				require.ErrorContains(t, err, tc.expectedErrContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_DisbursementManagementService_validateBalanceForDisbursement_AmountDisbursed(t *testing.T) {
	ctx := context.Background()
	models := data.SetupModels(t)
	dbConnectionPool := models.DBConnectionPool

	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	// Create a distribution account
	distributionAccPubKey := "GAAHIL6ZW4QFNLCKALZ3YOIWPP4TXQ7B7J5IU7RLNVGQAV6GFDZHLDTA"
	stellarDistAccount := schema.NewStellarEnvTransactionAccount(distributionAccPubKey)

	testCases := []struct {
		name                string
		totalAmount         string
		amountDisbursed     string
		availableBalance    float64
		expectedErrContains string
		description         string
	}{
		{
			name:             "🟢 New disbursement: empty AmountDisbursed",
			totalAmount:      "100.00",
			amountDisbursed:  "",
			availableBalance: 150.00,
			description:      "New disbursement with empty AmountDisbursed should use full TotalAmount",
		},
		{
			name:             "🟢 Resumed disbursement: sufficient balance for remaining amount",
			totalAmount:      "100.00",
			amountDisbursed:  "60.00",
			availableBalance: 50.00, // More than remaining 40.00
			description:      "Resumed disbursement should subtract AmountDisbursed from TotalAmount",
		},
		{
			name:                "🔴 Resumed disbursement: insufficient balance for remaining amount",
			totalAmount:         "100.00",
			amountDisbursed:     "60.00",
			availableBalance:    30.00, // Less than remaining 40.00
			expectedErrContains: "insufficient to fulfill new amount (40.00)",
			description:         "Resumed disbursement should fail when balance < remaining amount",
		},
		{
			name:             "🟢 Resumed disbursement: zero remaining (fully disbursed)",
			totalAmount:      "100.00",
			amountDisbursed:  "100.00",
			availableBalance: 1.00, // Any amount should work when remaining is 0
			description:      "Fully disbursed amount should result in 0 remaining amount needed",
		},
		{
			name:                "🔴 Invalid AmountDisbursed format",
			totalAmount:         "100.00",
			amountDisbursed:     "invalid",
			availableBalance:    150.00,
			expectedErrContains: "cannot convert amount disbursed invalid",
			description:         "Invalid AmountDisbursed should return parse error",
		},
		{
			name:             "🟢 AmountDisbursed with decimal precision",
			totalAmount:      "100.50",
			amountDisbursed:  "60.25",
			availableBalance: 50.00, // More than remaining 40.25
			description:      "Should handle decimal precision correctly",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create disbursement with test data
			disbursement := &data.Disbursement{
				ID:    "test-disbursement-id",
				Asset: asset,
				DisbursementStats: &data.DisbursementStats{
					TotalAmount:     tc.totalAmount,
					AmountDisbursed: tc.amountDisbursed,
				},
			}

			dbTx := testutils.BeginTxWithRollback(t, ctx, dbConnectionPool)

			mDistAccService := mocks.NewMockDistributionAccountService(t)
			mDistAccService.
				On("GetBalance", ctx, &stellarDistAccount, *asset).
				Return(decimal.NewFromFloat(tc.availableBalance), nil).
				Once()

			svc := &DisbursementManagementService{
				Models:                     models,
				DistributionAccountService: mDistAccService,
			}

			err := svc.validateBalanceForDisbursement(ctx, dbTx, &stellarDistAccount, disbursement)

			if tc.expectedErrContains != "" {
				require.Error(t, err, "Expected error for case: %s", tc.description)
				assert.Contains(t, err.Error(), tc.expectedErrContains, "Error message should contain expected text for case: %s", tc.description)
			} else {
				assert.NoError(t, err, "Expected success for case: %s", tc.description)
			}
		})
	}
}
