package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/protocols/horizon/base"
	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
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
	tnt := tenant.Tenant{ID: "tenant-id"}
	ctx = tenant.SaveTenantInContext(context.Background(), &tnt)
	token := "token"
	ctx = context.WithValue(ctx, middleware.TokenContextKey, token)

	// Create distribution accounts
	distributionAccPubKey := "GAAHIL6ZW4QFNLCKALZ3YOIWPP4TXQ7B7J5IU7RLNVGQAV6GFDZHLDTA"
	stellarDistAccountEnv := schema.NewStellarEnvTransactionAccount(distributionAccPubKey)
	stellarDistAccountDBVault := schema.NewDefaultStellarTransactionAccount(distributionAccPubKey)
	circleDistAccountDBVault := schema.TransactionAccount{
		CircleWalletID: "circle-wallet-id",
		Type:           schema.DistributionAccountCircleDBVault,
		Status:         schema.AccountStatusActive,
	}

	ownerUser := &auth.User{ID: "owner-user", Email: "owner@test.com"}
	financialUser := &auth.User{ID: "financial-user", Email: "financial@test.com"}

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

			// Create mocks: events producer
			mEventProducer := events.NewMockProducer(t)
			mEventProducer.
				On("WriteMessages", ctx, mock.AnythingOfType("[]events.Message")).
				Run(func(args mock.Arguments) {
					msgs, ok := args.Get(1).([]events.Message)
					require.True(t, ok)
					require.Len(t, msgs, 2)

					// Validating send invite msg
					sendInviteMsg := msgs[0]
					assert.Equal(t, events.ReceiverWalletNewInvitationTopic, sendInviteMsg.Topic)
					assert.Equal(t, readyDisbursement.ID, sendInviteMsg.Key)
					assert.Equal(t, events.BatchReceiverWalletInvitationType, sendInviteMsg.Type)
					assert.Equal(t, tnt.ID, sendInviteMsg.TenantID)

					eventData, ok := sendInviteMsg.Data.([]schemas.EventReceiverWalletInvitationData)
					require.True(t, ok)
					require.Len(t, eventData, 2)
					wantElements := []schemas.EventReceiverWalletInvitationData{
						{ReceiverWalletID: rwDraft.ID}, // <--- invitation for the receiver that is being included in the system for the first time
						{ReceiverWalletID: rwReady.ID}, // <--- invitation for the receiver that is already in the system but doesn't have a Stellar wallet yet
					}
					assert.ElementsMatch(t, wantElements, eventData)

					var expectedTopic string
					switch tc.distributionAccount.Type.Platform() {
					case schema.CirclePlatform:
						expectedTopic = events.CirclePaymentReadyToPayTopic
					case schema.StellarPlatform:
						expectedTopic = events.PaymentReadyToPayTopic
					}

					// Validating payments ready to pay msg
					paymentsReadyToPayMsg := msgs[1]
					assert.Equal(t, events.Message{
						Topic:    expectedTopic,
						Key:      readyDisbursement.ID,
						TenantID: tnt.ID,
						Type:     events.PaymentReadyToPayDisbursementStarted,
						Data: schemas.EventPaymentsReadyToPayData{
							TenantID: tnt.ID,
							Payments: []schemas.PaymentReadyToPay{
								{ID: pRegistered.ID},
							},
						},
					}, paymentsReadyToPayMsg)
				}).
				Return(nil).
				Once()

			// Setup dependent services
			distAccSvc, err := NewDistributionAccountService(DistributionAccountServiceOptions{
				HorizonClient: mHorizonClient,
				CircleService: mCircleService,
				NetworkType:   utils.TestnetNetworkType,
			})
			require.NoError(t, err)
			service := &DisbursementManagementService{
				Models:                     models,
				EventProducer:              mEventProducer,
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

	tnt := tenant.Tenant{ID: "tenant-id"}
	ctx := tenant.SaveTenantInContext(context.Background(), &tnt)
	token := "token"
	ctx = context.WithValue(ctx, middleware.TokenContextKey, token)

	// Create fixtures: asset, wallet
	asset := data.GetAssetFixture(t, ctx, dbConnectionPool, data.FixtureAssetUSDC)
	distributionAccPubKey := "GAAHIL6ZW4QFNLCKALZ3YOIWPP4TXQ7B7J5IU7RLNVGQAV6GFDZHLDTA"
	distributionAcc := schema.NewDefaultStellarTransactionAccount(distributionAccPubKey)

	// create fixtures
	wallet := data.CreateDefaultWalletFixture(t, ctx, dbConnectionPool)

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
	receiverRegistered := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	rwRegistered := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverRegistered.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	hAccRequest := horizonclient.AccountRequest{AccountID: distributionAccPubKey}
	hAccResponse := horizon.Account{
		Balances: []horizon.Balance{
			{
				Balance: "10000000",
				Asset: base.Asset{
					Code:   asset.Code,
					Issuer: asset.Issuer,
				},
			},
		},
	}

	t.Run("returns an error if the disbursement doesn't exist", func(t *testing.T) {
		service := DisbursementManagementService{Models: models}

		err = service.StartDisbursement(context.Background(), "not-found-id", nil, &distributionAcc)
		require.ErrorIs(t, err, ErrDisbursementNotFound)
	})

	t.Run("returns an error if the disbursement's wallet is disabled", func(t *testing.T) {
		service := DisbursementManagementService{Models: models}

		data.EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, false, wallet.ID)
		defer data.EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, true, wallet.ID)
		err = service.StartDisbursement(context.Background(), draftDisbursement.ID, nil, &distributionAcc)
		require.ErrorIs(t, err, ErrDisbursementWalletDisabled)
	})

	t.Run("returns an error if the disbursement status is not READY", func(t *testing.T) {
		service := DisbursementManagementService{Models: models}

		err = service.StartDisbursement(context.Background(), draftDisbursement.ID, nil, &distributionAcc)
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
			ID:    userID,
			Email: "email@email.com",
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

	t.Run("returns an error if the distribution account has insuficcient balance", func(t *testing.T) {
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

		err = service.StartDisbursement(ctx, disbursementInsufficientBalance.ID, nil, &distributionAcc)
		expectedErr := InsufficientBalanceError{
			DisbursementAsset:   *usdt,
			DistributionAddress: distributionAcc.ID(),
			DisbursementID:      disbursementInsufficientBalance.ID,
			AvailableBalance:    11111.0,
			DisbursementAmount:  22222.0,
			TotalPendingAmount:  1100.0,
		}

		require.ErrorContains(t, err, fmt.Sprintf("validating balance for disbursement: %v", expectedErr))

		// PendingTotal includes payments associated with 'readyDisbursement' that were moved from the draft to ready status
		expectedErrStr := fmt.Sprintf("the disbursement %s failed due to an account balance (11111.00) that was insufficient to fulfill new amount (22222.00) along with the pending amount (1100.00). To complete this action, your distribution account (stellar:GAAHIL6ZW4QFNLCKALZ3YOIWPP4TXQ7B7J5IU7RLNVGQAV6GFDZHLDTA) needs to be recharged with at least 12211.00 USDT", disbursementInsufficientBalance.ID)
		assert.Contains(t, buf.String(), expectedErrStr)
	})

	t.Run("logs and reports to the crashTracker when the eventProducer fails", func(t *testing.T) {
		userID := "9ae68f09-cad9-4311-9758-4ff59d2e9e6d"
		statusHistory := []data.DisbursementStatusHistoryEntry{
			{
				Status: data.DraftDisbursementStatus,
				UserID: userID,
			},
			{
				Status: data.ReadyDisbursementStatus,
				UserID: userID,
			},
		}
		disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name:          "disbursement #3",
			Status:        data.ReadyDisbursementStatus,
			Asset:         asset,
			Wallet:        wallet,
			StatusHistory: statusHistory,
		})

		payment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rwRegistered,
			Disbursement:   disbursement,
			Asset:          *asset,
			Amount:         "100",
			Status:         data.ReadyPaymentStatus,
		})

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rwReady,
			Disbursement:   disbursement,
			Asset:          *asset,
			Amount:         "100",
			Status:         data.ReadyPaymentStatus,
		})

		expectedMessages := []events.Message{
			{
				Topic:    events.ReceiverWalletNewInvitationTopic,
				Key:      disbursement.ID,
				TenantID: tnt.ID,
				Type:     events.BatchReceiverWalletInvitationType,
				Data: []schemas.EventReceiverWalletInvitationData{
					{ReceiverWalletID: rwReady.ID}, // Receiver that can receive SMS
				},
			},
			{
				Topic:    events.PaymentReadyToPayTopic,
				Key:      disbursement.ID,
				TenantID: tnt.ID,
				Type:     events.PaymentReadyToPayDisbursementStarted,
				Data: schemas.EventPaymentsReadyToPayData{
					TenantID: tnt.ID,
					Payments: []schemas.PaymentReadyToPay{
						{ID: payment.ID},
					},
				},
			},
		}

		// Create Mocks
		hMock := &horizonclient.MockClient{}
		defer hMock.AssertExpectations(t)
		hMock.On("AccountDetail", hAccRequest).Return(hAccResponse, nil).Once()
		producerErr := errors.New("unexpected WriteMessages error")
		mockEventProducer := events.NewMockProducer(t)
		mockEventProducer.
			On("WriteMessages", ctx, expectedMessages).
			Return(producerErr).
			Once()
		mCrashTracker := &crashtracker.MockCrashTrackerClient{}
		mCrashTracker.
			On("LogAndReportErrors", mock.Anything, mock.Anything, "writing messages after disbursement start on event producer").
			Run(func(args mock.Arguments) {
				err := args.Get(1).(error)
				assert.ErrorIs(t, err, producerErr)
			}).
			Once()

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
			EventProducer:              mockEventProducer,
			CrashTrackerClient:         mCrashTracker,
			DistributionAccountService: distAccSvc,
		}

		user := &auth.User{
			ID:    "user-id",
			Email: "email@email.com",
		}

		err = service.StartDisbursement(ctx, disbursement.ID, user, &distributionAcc)
		assert.NoError(t, err)
	})

	t.Run("doesn't produce message when there are no payments ready to pay", func(t *testing.T) {
		userID := "9ae68f09-cad9-4311-9758-4ff59d2e9e6d"
		statusHistory := []data.DisbursementStatusHistoryEntry{
			{
				Status: data.DraftDisbursementStatus,
				UserID: userID,
			},
			{
				Status: data.ReadyDisbursementStatus,
				UserID: userID,
			},
		}
		disbursement := data.CreateDisbursementFixture(t, context.Background(), dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name:          "disbursement with no payments ready to pay",
			Status:        data.ReadyDisbursementStatus,
			Asset:         asset,
			Wallet:        wallet,
			StatusHistory: statusHistory,
		})

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rwReady,
			Disbursement:   disbursement,
			Asset:          *asset,
			Amount:         "100",
			Status:         data.ReadyPaymentStatus,
		})

		// Create Mocks
		hMock := &horizonclient.MockClient{}
		defer hMock.AssertExpectations(t)
		hMock.On("AccountDetail", hAccRequest).Return(hAccResponse, nil).Once()
		mockEventProducer := events.NewMockProducer(t)
		mockEventProducer.
			On("WriteMessages", ctx, []events.Message{
				{
					Topic:    events.ReceiverWalletNewInvitationTopic,
					Key:      disbursement.ID,
					TenantID: tnt.ID,
					Type:     events.BatchReceiverWalletInvitationType,
					Data: []schemas.EventReceiverWalletInvitationData{
						{ReceiverWalletID: rwReady.ID}, // Receiver that can receive SMS
					},
				},
			}).
			Return(nil).
			Once()

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
			EventProducer:              mockEventProducer,
			DistributionAccountService: distAccSvc,
		}

		getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		user := &auth.User{ID: "user-id", Email: "email@email.com"}

		err = service.StartDisbursement(ctx, disbursement.ID, user, &distributionAcc)
		require.NoError(t, err)

		entries := getEntries()
		require.Len(t, entries, 5)
		assert.Contains(t, fmt.Sprintf("no payments ready to pay for disbursement ID %s", disbursement.ID), entries[3].Message)
	})

	t.Run("returns error when tenant is not in the context", func(t *testing.T) {
		ctxWithoutTenant := context.Background()

		userID := "9ae68f09-cad9-4311-9758-4ff59d2e9e6d"
		statusHistory := []data.DisbursementStatusHistoryEntry{
			{
				Status: data.DraftDisbursementStatus,
				UserID: userID,
			},
			{
				Status: data.ReadyDisbursementStatus,
				UserID: userID,
			},
		}
		disbursement := data.CreateDisbursementFixture(t, ctxWithoutTenant, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name:          "disbursement #5",
			Status:        data.ReadyDisbursementStatus,
			Asset:         asset,
			Wallet:        wallet,
			StatusHistory: statusHistory,
		})

		_ = data.CreatePaymentFixture(t, ctxWithoutTenant, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rwRegistered,
			Disbursement:   disbursement,
			Asset:          *asset,
			Amount:         "100",
			Status:         data.ReadyPaymentStatus,
		})

		user := &auth.User{ID: "user-id", Email: "email@email.com"}

		// Create Mocks
		hMock := &horizonclient.MockClient{}
		defer hMock.AssertExpectations(t)
		hMock.On("AccountDetail", hAccRequest).Return(hAccResponse, nil).Once()

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

		err = service.StartDisbursement(ctxWithoutTenant, disbursement.ID, user, &distributionAcc)
		assert.ErrorContains(t, err, "creating new message: getting tenant from context: tenant not found in context")
	})

	t.Run("logs when couldn't write message because EventProducer is nil", func(t *testing.T) {
		userID := "9ae68f09-cad9-4311-9758-4ff59d2e9e6d"
		statusHistory := []data.DisbursementStatusHistoryEntry{
			{
				Status: data.DraftDisbursementStatus,
				UserID: userID,
			},
			{
				Status: data.ReadyDisbursementStatus,
				UserID: userID,
			},
		}
		disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name:          "disbursement #6",
			Status:        data.ReadyDisbursementStatus,
			Asset:         asset,
			Wallet:        wallet,
			StatusHistory: statusHistory,
		})

		paymentReady := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rwRegistered,
			Disbursement:   disbursement,
			Asset:          *asset,
			Amount:         "100",
			Status:         data.ReadyPaymentStatus,
		})

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rwReady,
			Disbursement:   disbursement,
			Asset:          *asset,
			Amount:         "100",
			Status:         data.ReadyPaymentStatus,
		})

		getEntries := log.DefaultLogger.StartTest(log.ErrorLevel)

		user := &auth.User{ID: "user-id", Email: "email@email.com"}

		// Create Mocks
		hMock := &horizonclient.MockClient{}
		defer hMock.AssertExpectations(t)
		hMock.On("AccountDetail", hAccRequest).Return(hAccResponse, nil).Once()

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
			EventProducer:              nil, // <----- EventProducer is nil
			DistributionAccountService: distAccSvc,
		}

		err = service.StartDisbursement(ctx, disbursement.ID, user, &distributionAcc)
		require.NoError(t, err)

		msgs := []events.Message{
			{
				Topic:    events.ReceiverWalletNewInvitationTopic,
				Key:      disbursement.ID,
				TenantID: tnt.ID,
				Type:     events.BatchReceiverWalletInvitationType,
				Data: []schemas.EventReceiverWalletInvitationData{
					{
						ReceiverWalletID: rwReady.ID,
					},
				},
			},
			{
				Topic:    events.PaymentReadyToPayTopic,
				Key:      disbursement.ID,
				TenantID: tnt.ID,
				Type:     events.PaymentReadyToPayDisbursementStarted,
				Data: schemas.EventPaymentsReadyToPayData{
					TenantID: tnt.ID,
					Payments: []schemas.PaymentReadyToPay{
						{
							ID: paymentReady.ID,
						},
					},
				},
			},
		}

		entries := getEntries()
		require.Len(t, entries, 1)
		assert.Contains(t, fmt.Sprintf("event producer is nil, could not publish messages %+v", msgs), entries[0].Message)
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

	mockEventProducer := events.MockProducer{}
	defer mockEventProducer.AssertExpectations(t)

	ctx := context.Background()

	tnt := tenant.Tenant{ID: "tenant-id"}
	ctx = tenant.SaveTenantInContext(ctx, &tnt)

	token := "token"
	ctx = context.WithValue(ctx, middleware.TokenContextKey, token)

	user := &auth.User{
		ID:    "user-id",
		Email: "email@email.com",
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
		EventProducer:              &mockEventProducer,
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

		mockEventProducer.
			On("WriteMessages", ctx, []events.Message{
				{
					Topic:    events.PaymentReadyToPayTopic,
					Key:      startedDisbursement.ID,
					TenantID: tnt.ID,
					Type:     events.PaymentReadyToPayDisbursementStarted,
					Data: schemas.EventPaymentsReadyToPayData{
						TenantID: tnt.ID,
						Payments: []schemas.PaymentReadyToPay{
							{
								ID: paymentReady1.ID,
							},
							{
								ID: paymentReady2.ID,
							},
						},
					},
				},
			}).
			Return(nil).
			Once()

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

		mockEventProducer.
			On("WriteMessages", ctx, []events.Message{
				{
					Topic:    events.PaymentReadyToPayTopic,
					Key:      startedDisbursement.ID,
					TenantID: tnt.ID,
					Type:     events.PaymentReadyToPayDisbursementStarted,
					Data: schemas.EventPaymentsReadyToPayData{
						TenantID: tnt.ID,
						Payments: []schemas.PaymentReadyToPay{
							{
								ID: paymentReady1.ID,
							},
							{
								ID: paymentReady2.ID,
							},
						},
					},
				},
			}).
			Return(nil).
			Once()

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
		Amount:         "10",
		Status:         data.PendingPaymentStatus,
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
			AvailableBalance:    99.99,
			DisbursementAmount:  90.00,
			TotalPendingAmount:  10.00,
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
					Return(0.0, errors.New("GetBalance error")).
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
					Return(99.99, nil).
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
					Return(99.99, nil).
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
					Return(99.99, nil).
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
					Return(100.00, nil).
					Once()
			},
		},
		{
			name:                "🟢[DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT] successfully validate ballance for disbursement",
			disbursementAccount: stellarDistAccountDBVault,
			prepareMocksFn: func(mDistAccService *mocks.MockDistributionAccountService) {
				mDistAccService.
					On("GetBalance", ctx, &stellarDistAccountDBVault, *asset).
					Return(100.00, nil).
					Once()
			},
		},
		{
			name:                "🟢[DISTRIBUTION_ACCOUNT.CIRCLE_DB_VAULT] successfully validate ballance for disbursement",
			disbursementAccount: circleDistAccountDBVault,
			prepareMocksFn: func(mDistAccService *mocks.MockDistributionAccountService) {
				mDistAccService.
					On("GetBalance", ctx, &circleDistAccountDBVault, *asset).
					Return(100.00, nil).
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
