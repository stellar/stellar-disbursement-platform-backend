package data

import (
	"context"
	"testing"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_AssetModelGet(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	assetModel := &AssetModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns error when asset is not found", func(t *testing.T) {
		_, err := assetModel.Get(ctx, "not-found")
		require.Error(t, err)
		require.Equal(t, ErrRecordNotFound, err)
	})

	t.Run("returns asset successfully", func(t *testing.T) {
		expected := CreateAssetFixture(t, ctx, dbConnectionPool.SqlxDB(), "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
		actual, err := assetModel.Get(ctx, expected.ID)
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	})
}

func Test_AssetModelGetByCodeAndIssuer(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	assetModel := &AssetModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns error when asset is not found", func(t *testing.T) {
		_, err := assetModel.GetByCodeAndIssuer(ctx, "invalid_code", "invalid_issuer")
		require.Error(t, err)
		require.Equal(t, ErrRecordNotFound, err)
	})

	t.Run("returns asset successfully", func(t *testing.T) {
		expected := CreateAssetFixture(t, ctx, dbConnectionPool.SqlxDB(), "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
		actual, err := assetModel.GetByCodeAndIssuer(ctx, expected.Code, expected.Issuer)
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	})
}

func Test_AssetModelGetAll(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	assetModel := &AssetModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns all assets successfully", func(t *testing.T) {
		expected := ClearAndCreateAssetFixtures(t, ctx, dbConnectionPool.SqlxDB())
		actual, err := assetModel.GetAll(ctx)
		require.NoError(t, err)

		assert.Equal(t, expected, actual)
	})

	t.Run("returns empty array when no assets", func(t *testing.T) {
		DeleteAllAssetFixtures(t, ctx, dbConnectionPool.SqlxDB())
		actual, err := assetModel.GetAll(ctx)
		require.NoError(t, err)

		assert.Equal(t, []Asset{}, actual)
	})
}

func Test_AssetModelInsert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	assetModel := &AssetModel{dbConnectionPool: dbConnectionPool}

	t.Run("inserts asset successfully", func(t *testing.T) {
		DeleteAllAssetFixtures(t, ctx, dbConnectionPool.SqlxDB())
		code := "USDT"
		issuer := "GBVHJTRLQRMIHRYTXZQOPVYCVVH7IRJN3DOFT7VC6U75CBWWBVDTWURG"

		asset, err := assetModel.Insert(ctx, dbConnectionPool, code, issuer)
		require.NoError(t, err)
		assert.NotNil(t, asset)

		insertedAsset, err := assetModel.Get(ctx, asset.ID)
		require.NoError(t, err)
		assert.NotNil(t, insertedAsset)
	})

	t.Run("re-create a deleted asset", func(t *testing.T) {
		DeleteAllAssetFixtures(t, ctx, dbConnectionPool.SqlxDB())
		code := "USDT"
		issuer := "GBVHJTRLQRMIHRYTXZQOPVYCVVH7IRJN3DOFT7VC6U75CBWWBVDTWURG"

		usdt, err := assetModel.Insert(ctx, dbConnectionPool, code, issuer)
		require.NoError(t, err)
		assert.NotNil(t, usdt)

		usdc, err := assetModel.Insert(ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
		require.NoError(t, err)
		assert.NotNil(t, usdt)

		_, err = assetModel.SoftDelete(ctx, dbConnectionPool, usdc.ID)
		require.NoError(t, err)

		_, err = assetModel.SoftDelete(ctx, dbConnectionPool, usdt.ID)
		require.NoError(t, err)

		usdcDB, err := assetModel.Get(ctx, usdc.ID)
		require.NoError(t, err)
		assert.NotNil(t, usdcDB.DeletedAt)

		reCreatedUSDT, err := assetModel.Insert(ctx, dbConnectionPool, code, issuer)
		require.NoError(t, err)
		assert.NotNil(t, reCreatedUSDT)

		usdtDB, err := assetModel.Get(ctx, usdt.ID)
		require.NoError(t, err)

		assert.NotNil(t, usdtDB)

		assert.Equal(t, usdtDB.ID, usdt.ID)
		assert.Equal(t, usdtDB.Code, usdt.Code)
		assert.Equal(t, usdtDB.Issuer, usdt.Issuer)

		assert.Equal(t, usdtDB.ID, reCreatedUSDT.ID)
		assert.Equal(t, usdtDB.Code, reCreatedUSDT.Code)
		assert.Equal(t, usdtDB.Issuer, reCreatedUSDT.Issuer)

		usdcDB, err = assetModel.Get(ctx, usdc.ID)
		require.NoError(t, err)
		assert.NotNil(t, usdcDB.DeletedAt)
	})

	t.Run("does not insert the same asset again", func(t *testing.T) {
		DeleteAllAssetFixtures(t, ctx, dbConnectionPool.SqlxDB())
		code := "USDT"
		issuer := "GBVHJTRLQRMIHRYTXZQOPVYCVVH7IRJN3DOFT7VC6U75CBWWBVDTWURG"

		asset, err := assetModel.Insert(ctx, dbConnectionPool, code, issuer)
		require.NoError(t, err)
		assert.NotNil(t, asset)

		duplicatedAsset, err := assetModel.Insert(ctx, dbConnectionPool, code, issuer)
		assert.EqualError(t, err, "error inserting asset: sql: no rows in result set")
		assert.Nil(t, duplicatedAsset)
	})
}

func Test_AssetModelGetOrCreate(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	assetModel := &AssetModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns error when issuer is invalid", func(t *testing.T) {
		asset, err := assetModel.GetOrCreate(ctx, "FOO1", "invalid_issuer")
		require.EqualError(t, err, "error getting or creating asset: pq: new row for relation \"assets\" violates check constraint \"asset_issuer_length_check\"")
		assert.Empty(t, asset)
	})

	t.Run("creates asset successfully", func(t *testing.T) {
		asset, err := assetModel.GetOrCreate(ctx, "F001", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
		require.NoError(t, err)
		assert.Equal(t, "F001", asset.Code)
		assert.Equal(t, "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV", asset.Issuer)
	})

	t.Run("returns asset successfully", func(t *testing.T) {
		expected := CreateAssetFixture(t, ctx, dbConnectionPool.SqlxDB(), "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
		asset, err := assetModel.GetOrCreate(ctx, expected.Code, expected.Issuer)
		require.NoError(t, err)
		assert.Equal(t, expected.ID, asset.ID)
	})
}

func Test_AssetModelSoftDelete(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	assetModel := &AssetModel{dbConnectionPool: dbConnectionPool}

	t.Run("delete successful", func(t *testing.T) {
		DeleteAllAssetFixtures(t, ctx, dbConnectionPool.SqlxDB())
		expected := CreateAssetFixture(t, ctx, dbConnectionPool.SqlxDB(), "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

		asset, err := assetModel.SoftDelete(ctx, dbConnectionPool, expected.ID)
		require.NoError(t, err)
		assert.NotNil(t, asset)
		assert.NotNil(t, asset.DeletedAt)
		deletedAt := asset.DeletedAt

		deletedAsset, err := assetModel.Get(ctx, expected.ID)
		require.NoError(t, err)
		assert.NotNil(t, deletedAsset)
		assert.Equal(t, deletedAsset.DeletedAt, deletedAt)
	})

	t.Run("delete unsuccessful, cannot find asset", func(t *testing.T) {
		DeleteAllAssetFixtures(t, ctx, dbConnectionPool.SqlxDB())

		_, err := assetModel.SoftDelete(ctx, dbConnectionPool, "non-existant")
		require.Error(t, err)
	})
}

func Test_GetAssetsPerReceiverWallet(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := NewModels(dbConnectionPool)
	require.NoError(t, err)

	// 1. Create assets, wallets and disbursements:
	country := CreateCountryFixture(t, ctx, dbConnectionPool, "ATL", "Atlantis")

	asset1 := CreateAssetFixture(t, ctx, dbConnectionPool, "FOO1", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	asset2 := CreateAssetFixture(t, ctx, dbConnectionPool, "FOO2", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	walletA := CreateWalletFixture(t, ctx, dbConnectionPool, "walletA", "https://www.a.com", "www.a.com", "a://")
	walletB := CreateWalletFixture(t, ctx, dbConnectionPool, "walletB", "https://www.b.com", "www.b.com", "b://")

	disbursementA1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Country: country,
		Wallet:  walletA,
		Status:  ReadyDisbursementStatus,
		Asset:   asset1,
	})
	disbursementA2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Country: country,
		Wallet:  walletA,
		Status:  ReadyDisbursementStatus,
		Asset:   asset2,
	})
	disbursementB1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Country: country,
		Wallet:  walletB,
		Status:  ReadyDisbursementStatus,
		Asset:   asset1,
	})
	disbursementB2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Country: country,
		Wallet:  walletB,
		Status:  ReadyDisbursementStatus,
		Asset:   asset2,
	})

	// 2. Create receivers, and receiver wallets:
	receiverX := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverY := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})

	receiverWalletXA := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverX.ID, walletA.ID, DraftReceiversWalletStatus)
	receiverWalletXB := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverX.ID, walletB.ID, DraftReceiversWalletStatus)
	receiverWalletYA := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverY.ID, walletA.ID, DraftReceiversWalletStatus)
	receiverWalletYB := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverY.ID, walletB.ID, DraftReceiversWalletStatus)

	// 3. Create payments:
	// paymentXA1 - walletA, asset1 for receiverX on their receiverWalletA
	_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: receiverWalletXA,
		Disbursement:   disbursementA1,
		Asset:          *asset1,
		Status:         ReadyPaymentStatus,
		Amount:         "1",
	})

	// paymentXA2 - walletA, asset2 for receiverX on their receiverWalletA
	_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: receiverWalletXA,
		Disbursement:   disbursementA2,
		Asset:          *asset2,
		Status:         ReadyPaymentStatus,
		Amount:         "1",
	})

	// paymentXA2 - walletA, asset2 for receiverX on their receiverWalletA - This should be ignored
	_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: receiverWalletXA,
		Disbursement:   disbursementA2,
		Asset:          *asset2,
		Status:         ReadyPaymentStatus,
		Amount:         "1",
	})

	// paymentXB2 - walletB, asset2 for receiverX on their receiverWalletB
	_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: receiverWalletXB,
		Disbursement:   disbursementB2,
		Asset:          *asset2,
		Status:         ReadyPaymentStatus,
		Amount:         "1",
	})

	// paymentXB1 - walletB, asset1 for receiverX on their receiverWalletB
	time.Sleep(10 * time.Millisecond)
	_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: receiverWalletXB,
		Disbursement:   disbursementB1,
		Asset:          *asset1,
		Status:         ReadyPaymentStatus,
		Amount:         "1",
	})

	// paymentYA2 - walletA, asset2 for receiverY on their receiverWalletA
	_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: receiverWalletYA,
		Disbursement:   disbursementA2,
		Asset:          *asset2,
		Status:         ReadyPaymentStatus,
		UpdatedAt:      time.Date(2024, 1, 6, 0, 0, 0, 0, time.UTC),
		Amount:         "1",
	})

	// paymentYA1 - walletA, asset1 for receiverY on their receiverWalletA
	_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: receiverWalletYA,
		Disbursement:   disbursementA1,
		Asset:          *asset1,
		Status:         ReadyPaymentStatus,
		UpdatedAt:      time.Date(2024, 2, 5, 0, 0, 0, 0, time.UTC),
		Amount:         "1",
	})

	// paymentYB1 - walletB, asset1 for receiverY on their receiverWalletB
	_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: receiverWalletYB,
		Disbursement:   disbursementB1,
		Asset:          *asset1,
		Status:         ReadyPaymentStatus,
		UpdatedAt:      time.Date(2024, 1, 7, 0, 0, 0, 0, time.UTC),
		Amount:         "1",
	})

	// paymentYB2 - walletB, asset2 for receiverY on their receiverWalletB
	_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: receiverWalletYB,
		Disbursement:   disbursementB2,
		Asset:          *asset2,
		Status:         ReadyPaymentStatus,
		UpdatedAt:      time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC),
		Amount:         "1",
	})

	gotLatestAssetsPerRW, err := models.Assets.GetAssetsPerReceiverWallet(ctx, receiverWalletXA, receiverWalletXB, receiverWalletYA, receiverWalletYB)
	require.NoError(t, err)
	require.Len(t, gotLatestAssetsPerRW, 8)

	wantLatestAssetsPerRW := []ReceiverWalletAsset{
		{
			ReceiverWallet: ReceiverWallet{
				ID: receiverWalletXA.ID,
				Receiver: Receiver{
					ID:          receiverX.ID,
					Email:       receiverX.Email,
					PhoneNumber: receiverX.PhoneNumber,
				},
			},
			WalletID: walletA.ID,
			Asset:    *asset1,
		},
		{
			ReceiverWallet: ReceiverWallet{
				ID: receiverWalletXA.ID,
				Receiver: Receiver{
					ID:          receiverX.ID,
					Email:       receiverX.Email,
					PhoneNumber: receiverX.PhoneNumber,
				},
			},
			WalletID: walletA.ID,
			Asset:    *asset2,
		},
		{
			ReceiverWallet: ReceiverWallet{
				ID: receiverWalletXB.ID,
				Receiver: Receiver{
					ID:          receiverX.ID,
					Email:       receiverX.Email,
					PhoneNumber: receiverX.PhoneNumber,
				},
			},
			WalletID: walletB.ID,
			Asset:    *asset1,
		},
		{
			ReceiverWallet: ReceiverWallet{
				ID: receiverWalletXB.ID,
				Receiver: Receiver{
					ID:          receiverX.ID,
					Email:       receiverX.Email,
					PhoneNumber: receiverX.PhoneNumber,
				},
			},
			WalletID: walletB.ID,
			Asset:    *asset2,
		},
		{
			ReceiverWallet: ReceiverWallet{
				ID: receiverWalletYA.ID,
				Receiver: Receiver{
					ID:          receiverY.ID,
					Email:       receiverY.Email,
					PhoneNumber: receiverY.PhoneNumber,
				},
			},
			WalletID: walletA.ID,
			Asset:    *asset1,
		},
		{
			ReceiverWallet: ReceiverWallet{
				ID: receiverWalletYA.ID,
				Receiver: Receiver{
					ID:          receiverY.ID,
					Email:       receiverY.Email,
					PhoneNumber: receiverY.PhoneNumber,
				},
			},
			WalletID: walletA.ID,
			Asset:    *asset2,
		},
		{
			ReceiverWallet: ReceiverWallet{
				ID: receiverWalletYB.ID,
				Receiver: Receiver{
					ID:          receiverY.ID,
					Email:       receiverY.Email,
					PhoneNumber: receiverY.PhoneNumber,
				},
			},
			WalletID: walletB.ID,
			Asset:    *asset1,
		},
		{
			ReceiverWallet: ReceiverWallet{
				ID: receiverWalletYB.ID,
				Receiver: Receiver{
					ID:          receiverY.ID,
					Email:       receiverY.Email,
					PhoneNumber: receiverY.PhoneNumber,
				},
			},
			WalletID: walletB.ID,
			Asset:    *asset2,
		},
	}

	assert.ElementsMatch(t, wantLatestAssetsPerRW, gotLatestAssetsPerRW)
}
