package services

import (
	"context"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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
		On("GetUsersByID", mock.Anything, []string{users[0].ID, users[1].ID}).
		Return(users, nil)
	authManagerMock.
		On("GetUsersByID", mock.Anything, []string{users[1].ID, users[0].ID}).
		Return(users, nil)

	service := NewDisbursementManagementService(models, models.DBConnectionPool, authManagerMock)

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

	service := NewDisbursementManagementService(models, models.DBConnectionPool, nil)
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

func Test_DisbursementManagementService_StartDisbursement(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	token := "token"
	ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)

	authManagerMock := &auth.AuthManagerMock{}
	service := NewDisbursementManagementService(models, models.DBConnectionPool, authManagerMock)

	// create fixtures
	wallet := data.CreateDefaultWalletFixture(t, ctx, dbConnectionPool)
	asset := data.GetAssetFixture(t, ctx, dbConnectionPool, data.FixtureAssetUSDC)
	country := data.GetCountryFixture(t, ctx, dbConnectionPool, data.FixtureCountryUKR)

	// create disbursements
	draftDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:    "draft disbursement",
		Status:  data.DraftDisbursementStatus,
		Asset:   asset,
		Wallet:  wallet,
		Country: country,
	})

	readyDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:    "ready disbursement",
		Status:  data.ReadyDisbursementStatus,
		Asset:   asset,
		Wallet:  wallet,
		Country: country,
	})

	// create disbursement receivers
	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver3 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver4 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})

	receiverIds := []string{receiver1.ID, receiver2.ID, receiver3.ID, receiver4.ID}

	rwDraft1 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, data.DraftReceiversWalletStatus)
	rwDraft2 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, data.DraftReceiversWalletStatus)
	rwReady := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver3.ID, wallet.ID, data.ReadyReceiversWalletStatus)
	rwRegistered := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver4.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	payment1 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwDraft1,
		Disbursement:   readyDisbursement,
		Asset:          *asset,
		Amount:         "100",
		Status:         data.DraftPaymentStatus,
	})
	payment2 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwDraft2,
		Disbursement:   readyDisbursement,
		Asset:          *asset,
		Amount:         "200",
		Status:         data.DraftPaymentStatus,
	})
	payment3 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwReady,
		Disbursement:   readyDisbursement,
		Asset:          *asset,
		Amount:         "300",
		Status:         data.DraftPaymentStatus,
	})
	payment4 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwRegistered,
		Disbursement:   readyDisbursement,
		Asset:          *asset,
		Amount:         "400",
		Status:         data.DraftPaymentStatus,
	})

	payments := []*data.Payment{payment1, payment2, payment3, payment4}

	t.Run("disbursement doesn't exist", func(t *testing.T) {
		id := "5e1f1c7f5b6c9c0001c1b1b1"

		err = service.StartDisbursement(context.Background(), id)
		require.ErrorIs(t, err, ErrDisbursementNotFound)
	})

	t.Run("disbursement wallet is disabled", func(t *testing.T) {
		data.EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, false, wallet.ID)
		defer data.EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, true, wallet.ID)
		err = service.StartDisbursement(context.Background(), draftDisbursement.ID)
		require.ErrorIs(t, err, ErrDisbursementWalletDisabled)
	})

	t.Run("disbursement not ready to start", func(t *testing.T) {
		err = service.StartDisbursement(context.Background(), draftDisbursement.ID)
		require.ErrorIs(t, err, ErrDisbursementNotReadyToStart)
	})

	t.Run("disbursement can't be started by its creator", func(t *testing.T) {
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
			Name:          "disbursement #1",
			Status:        data.ReadyDisbursementStatus,
			Asset:         asset,
			Wallet:        wallet,
			Country:       country,
			StatusHistory: statusHistory,
		})

		user := &auth.User{
			ID:    userID,
			Email: "email@email.com",
		}

		authManagerMock.
			On("GetUser", ctx, token).
			Return(user, nil).
			Once()

		// Enable approval workflow for org.
		isApprovalRequired := true
		err = models.Organizations.Update(ctx, &data.OrganizationUpdate{IsApprovalRequired: &isApprovalRequired})
		require.NoError(t, err)

		err = service.StartDisbursement(ctx, disbursement.ID)
		require.ErrorIs(t, err, ErrDisbursementStartedByCreator)

		// rollback changes
		isApprovalRequired = false
		err = models.Organizations.Update(ctx, &data.OrganizationUpdate{IsApprovalRequired: &isApprovalRequired})
		require.NoError(t, err)
	})

	t.Run("disbursement started with approval workflow", func(t *testing.T) {
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
			Name:          "disbursement #2",
			Status:        data.ReadyDisbursementStatus,
			Asset:         asset,
			Wallet:        wallet,
			Country:       country,
			StatusHistory: statusHistory,
		})

		user := &auth.User{
			ID:    "another user id",
			Email: "email@email.com",
		}

		authManagerMock.
			On("GetUser", ctx, token).
			Return(user, nil).
			Once()

		// Enable approval workflow for org.
		isApprovalRequired := true
		err = models.Organizations.Update(ctx, &data.OrganizationUpdate{IsApprovalRequired: &isApprovalRequired})
		require.NoError(t, err)

		err = service.StartDisbursement(ctx, disbursement.ID)
		require.NoError(t, err)

		// check disbursement status
		disbursement, err = models.Disbursements.Get(context.Background(), models.DBConnectionPool, disbursement.ID)
		require.NoError(t, err)
		require.Equal(t, data.StartedDisbursementStatus, disbursement.Status)

		// rollback changes
		isApprovalRequired = false
		err = models.Organizations.Update(ctx, &data.OrganizationUpdate{IsApprovalRequired: &isApprovalRequired})
		require.NoError(t, err)
	})

	t.Run("disbursement started", func(t *testing.T) {
		user := &auth.User{
			ID:    "user-id",
			Email: "email@email.com",
		}

		authManagerMock.
			On("GetUser", ctx, token).
			Return(user, nil).
			Once()

		err = service.StartDisbursement(ctx, readyDisbursement.ID)
		require.NoError(t, err)

		// check disbursement status
		disbursement, err := models.Disbursements.Get(context.Background(), models.DBConnectionPool, readyDisbursement.ID)
		require.NoError(t, err)
		require.Equal(t, data.StartedDisbursementStatus, disbursement.Status)

		// check disbursement history
		require.Equal(t, disbursement.StatusHistory[1].UserID, user.ID)

		// check receivers wallets status
		receiverWallets, err := models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, models.DBConnectionPool, receiverIds, wallet.ID)
		require.NoError(t, err)
		require.Equal(t, 4, len(receiverWallets))
		rwExpectedStatuses := map[string]data.ReceiversWalletStatus{
			rwDraft1.ID:     data.ReadyReceiversWalletStatus,
			rwDraft2.ID:     data.ReadyReceiversWalletStatus,
			rwReady.ID:      data.ReadyReceiversWalletStatus,
			rwRegistered.ID: data.RegisteredReceiversWalletStatus,
		}
		for _, rw := range receiverWallets {
			require.Equal(t, rwExpectedStatuses[rw.ID], rw.Status)
		}

		// check payments status
		for _, p := range payments {
			payment, err := models.Payment.Get(ctx, p.ID, dbConnectionPool)
			require.NoError(t, err)
			require.Equal(t, data.ReadyPaymentStatus, payment.Status)
		}
	})

	authManagerMock.AssertExpectations(t)
}

func Test_DisbursementManagementService_PauseDisbursement(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	models, outerErr := data.NewModels(dbConnectionPool)
	require.NoError(t, outerErr)

	token := "token"
	ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)

	user := &auth.User{
		ID:    "user-id",
		Email: "email@email.com",
	}
	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.
		On("GetUser", ctx, token).
		Return(user, nil)

	service := NewDisbursementManagementService(models, models.DBConnectionPool, authManagerMock)

	// create fixtures
	wallet := data.CreateDefaultWalletFixture(t, ctx, dbConnectionPool)
	asset := data.GetAssetFixture(t, ctx, dbConnectionPool, data.FixtureAssetUSDC)
	country := data.GetCountryFixture(t, ctx, dbConnectionPool, data.FixtureCountryUSA)

	// create disbursements
	readyDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:    "ready disbursement",
		Status:  data.ReadyDisbursementStatus,
		Asset:   asset,
		Wallet:  wallet,
		Country: country,
	})

	startedDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:    "started disbursement",
		Status:  data.StartedDisbursementStatus,
		Asset:   asset,
		Wallet:  wallet,
		Country: country,
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

		err := service.PauseDisbursement(ctx, id)
		require.ErrorIs(t, err, ErrDisbursementNotFound)
	})

	t.Run("disbursement not ready to pause", func(t *testing.T) {
		err := service.PauseDisbursement(ctx, readyDisbursement.ID)
		require.ErrorIs(t, err, ErrDisbursementNotReadyToPause)
	})

	t.Run("disbursement paused", func(t *testing.T) {
		err := service.PauseDisbursement(ctx, startedDisbursement.ID)
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
		err = service.StartDisbursement(ctx, startedDisbursement.ID)
		require.NoError(t, err)

		// check disbursement is started again
		disbursement, err = models.Disbursements.Get(ctx, models.DBConnectionPool, startedDisbursement.ID)
		require.NoError(t, err)
		require.Equal(t, data.StartedDisbursementStatus, disbursement.Status)
	})

	t.Run("start -> pause -> start -> pause", func(t *testing.T) {
		// 1. Pause Disbursement
		err := service.PauseDisbursement(ctx, startedDisbursement.ID)
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
		err = service.StartDisbursement(ctx, startedDisbursement.ID)
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
		err = service.PauseDisbursement(ctx, startedDisbursement.ID)
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

	authManagerMock.AssertExpectations(t)
}
