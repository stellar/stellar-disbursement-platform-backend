package data

import (
	"context"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/require"
)

func Test_DisbursementReceiverModel_Count(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	disbursementModel := &DisbursementModel{dbConnectionPool: dbConnectionPool}
	disbursementReceiverModel := &DisbursementReceiverModel{dbConnectionPool: dbConnectionPool}
	paymentModel := &PaymentModel{dbConnectionPool: dbConnectionPool}

	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, disbursementModel, &Disbursement{
		Country: country,
		Wallet:  wallet,
		Status:  ReadyDisbursementStatus,
		Asset:   asset,
	})

	receiver1 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiver2 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})

	rwDraft1 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, DraftReceiversWalletStatus)
	rwDraft2 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, DraftReceiversWalletStatus)

	require.NotNil(t, rwDraft1)
	require.NotNil(t, rwDraft2)

	t.Run("no receivers for disbursement 1", func(t *testing.T) {
		count, err := disbursementReceiverModel.Count(ctx, dbConnectionPool, disbursement1.ID)
		require.NoError(t, err)
		require.Equal(t, 0, count)
	})

	t.Run("count receivers for disbursement 1", func(t *testing.T) {
		CreatePaymentFixture(t, ctx, dbConnectionPool, paymentModel, &Payment{
			ReceiverWallet: rwDraft1,
			Disbursement:   disbursement1,
			Asset:          *asset,
			Amount:         "100",
			Status:         DraftPaymentStatus,
		})
		CreatePaymentFixture(t, ctx, dbConnectionPool, paymentModel, &Payment{
			ReceiverWallet: rwDraft2,
			Disbursement:   disbursement1,
			Asset:          *asset,
			Amount:         "200",
			Status:         DraftPaymentStatus,
		})

		count, err := disbursementReceiverModel.Count(ctx, dbConnectionPool, disbursement1.ID)
		require.NoError(t, err)
		require.Equal(t, 2, count)
	})
}

func Test_DisbursementReceiverModel_GetAll(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	disbursementModel := &DisbursementModel{dbConnectionPool: dbConnectionPool}
	disbursementReceiverModel := &DisbursementReceiverModel{dbConnectionPool: dbConnectionPool}
	paymentModel := &PaymentModel{dbConnectionPool: dbConnectionPool}

	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, disbursementModel, &Disbursement{
		Country: country,
		Wallet:  wallet,
		Status:  ReadyDisbursementStatus,
		Asset:   asset,
	})

	receiver1 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiver2 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})

	rwDraft1 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, DraftReceiversWalletStatus)
	rwDraft2 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, DraftReceiversWalletStatus)

	require.NotNil(t, rwDraft1)
	require.NotNil(t, rwDraft2)

	t.Run("no receivers for disbursement 1", func(t *testing.T) {
		receivers, err := disbursementReceiverModel.GetAll(ctx, dbConnectionPool, &QueryParams{}, disbursement1.ID)
		require.NoError(t, err)
		require.Equal(t, 0, len(receivers))
	})

	t.Run("get all receivers for disbursement 1", func(t *testing.T) {
		CreatePaymentFixture(t, ctx, dbConnectionPool, paymentModel, &Payment{
			ReceiverWallet: rwDraft1,
			Disbursement:   disbursement1,
			Asset:          *asset,
			Amount:         "100",
			Status:         DraftPaymentStatus,
		})
		CreatePaymentFixture(t, ctx, dbConnectionPool, paymentModel, &Payment{
			ReceiverWallet: rwDraft2,
			Disbursement:   disbursement1,
			Asset:          *asset,
			Amount:         "200",
			Status:         DraftPaymentStatus,
		})

		receivers, err := disbursementReceiverModel.GetAll(ctx, dbConnectionPool, &QueryParams{}, disbursement1.ID)
		require.NoError(t, err)
		require.Equal(t, 2, len(receivers))
	})
}
