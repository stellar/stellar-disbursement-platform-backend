package data

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stellar/go-stellar-sdk/protocols/horizon/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
)

func Test_AssetColumnNames(t *testing.T) {
	testCases := []struct {
		tableReference string
		resultAlias    string
		includeDates   bool
		expectedResult string
	}{
		{
			tableReference: "",
			resultAlias:    "",
			includeDates:   true,
			expectedResult: strings.Join([]string{
				"id",
				"code",
				"created_at",
				"updated_at",
				"deleted_at",
				`COALESCE(issuer, '') AS "issuer"`,
			}, ",\n"),
		},
		{
			tableReference: "",
			resultAlias:    "",
			includeDates:   false,
			expectedResult: strings.Join([]string{
				"id",
				"code",
				`COALESCE(issuer, '') AS "issuer"`,
			}, ",\n"),
		},
		{
			tableReference: "",
			resultAlias:    "asset",
			includeDates:   true,
			expectedResult: strings.Join([]string{
				`id AS "asset.id"`,
				`code AS "asset.code"`,
				`created_at AS "asset.created_at"`,
				`updated_at AS "asset.updated_at"`,
				`deleted_at AS "asset.deleted_at"`,
				`COALESCE(issuer, '') AS "asset.issuer"`,
			}, ",\n"),
		},
		{
			tableReference: "a",
			resultAlias:    "",
			includeDates:   true,
			expectedResult: strings.Join([]string{
				"a.id",
				"a.code",
				"a.created_at",
				"a.updated_at",
				"a.deleted_at",
				`COALESCE(a.issuer, '') AS "issuer"`,
			}, ",\n"),
		},
	}

	for _, tc := range testCases {
		t.Run(testCaseNameForScanText(t, tc.tableReference, tc.resultAlias), func(t *testing.T) {
			got := AssetColumnNames(tc.tableReference, tc.resultAlias, tc.includeDates)
			assert.Equal(t, tc.expectedResult, got)
		})
	}
}

func Test_Asset_IsNative(t *testing.T) {
	cases := []struct {
		asset    Asset
		isNative bool
	}{
		{Asset{Code: "XLM"}, true},
		{Asset{Code: "NATIVE"}, true},
		{Asset{Code: "ABC"}, false},
		{Asset{Issuer: "Issuer1", Code: "XLM"}, false},
		{Asset{Issuer: "Issuer1", Code: "NATIVE"}, false},
		{Asset{Issuer: "Issuer2", Code: "XYZ"}, false},
	}

	for _, c := range cases {
		got := c.asset.IsNative()
		if got != c.isNative {
			t.Errorf("Asset{%q, %q}.IsNative() == %t, want %t", c.asset.Issuer, c.asset.Code, got, c.isNative)
		}
	}
}

func Test_Asset_Equals(t *testing.T) {
	cases := []struct {
		asset1         Asset
		asset2         Asset
		expectedResult bool
	}{
		{Asset{Code: "XLM"}, Asset{Code: "XLM"}, true},
		{Asset{Code: "NATIVE"}, Asset{Code: "XLM"}, true},
		{Asset{Code: "NATIVE"}, Asset{Code: "native"}, false},
		{Asset{Code: "XLM"}, Asset{Code: "xlm"}, false},
		{Asset{Code: "XLM"}, Asset{Code: "ABC"}, false},
		{Asset{Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", Code: "USDC"}, Asset{Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", Code: "usdc"}, false},
		{Asset{Issuer: "gbbD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", Code: "USDC"}, Asset{Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", Code: "USDC"}, true},
		{Asset{Issuer: "Issuer1", Code: "ABC"}, Asset{Issuer: "Issuer2", Code: "ABC"}, false},
		{Asset{Issuer: "Issuer1", Code: "ABC"}, Asset{Issuer: "Issuer1", Code: "XYZ"}, false},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("Case %d", i), func(t *testing.T) {
			got := c.asset1.Equals(c.asset2)
			if got != c.expectedResult {
				t.Errorf("Asset{%q, %q}.Equals(Asset{%q, %q}) == %t, want %t", c.asset1.Issuer, c.asset1.Code, c.asset2.Issuer, c.asset2.Code, got, c.expectedResult)
			}
		})
	}
}

func Test_Asset_EqualsHorizonAsset(t *testing.T) {
	testCases := []struct {
		name           string
		localAsset     Asset
		horizonAsset   base.Asset
		expectedResult bool
	}{
		{
			name:           "🟢 XLM alias is equal to native type",
			localAsset:     Asset{Code: "XLM"},
			horizonAsset:   base.Asset{Type: "native"},
			expectedResult: true,
		},
		{
			name:           "🔴 xlm alias is not equal to native type",
			localAsset:     Asset{Code: "xlm"},
			horizonAsset:   base.Asset{Type: "native"},
			expectedResult: false,
		},
		{
			name:           "🟢 issued assets are equal",
			localAsset:     Asset{Code: "USDC", Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"},
			horizonAsset:   base.Asset{Type: "credit_alphanum4", Code: "USDC", Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"},
			expectedResult: true,
		},
		{
			name:           "🟢 issued assets with different case in issuer are equal",
			localAsset:     Asset{Code: "USDC", Issuer: "gbbD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"},
			horizonAsset:   base.Asset{Type: "credit_alphanum4", Code: "USDC", Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"},
			expectedResult: true,
		},
		{
			name:           "🔴 issued assets with different case in code are not equal",
			localAsset:     Asset{Code: "usdc", Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"},
			horizonAsset:   base.Asset{Type: "credit_alphanum4", Code: "USdc", Issuer: "gbbD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"},
			expectedResult: false,
		},
		{
			name:           "🟢 NATIVE asset alias is equal to native type",
			localAsset:     Asset{Code: "NATIVE"},
			horizonAsset:   base.Asset{Type: "native"},
			expectedResult: true,
		},
		{
			name:           "🔴 native asset alias is not equal to native type",
			localAsset:     Asset{Code: "native"},
			horizonAsset:   base.Asset{Type: "native"},
			expectedResult: false,
		},
		{
			name:           "🔴 issued asset is not equal to native asset",
			localAsset:     Asset{Code: "USDC", Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"},
			horizonAsset:   base.Asset{Type: "native"},
			expectedResult: false,
		},
		{
			name:           "🔴 issued asset is not equal to issued asset with different code",
			localAsset:     Asset{Code: "USDC", Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"},
			horizonAsset:   base.Asset{Type: "credit_alphanum4", Code: "EUROC", Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"},
			expectedResult: false,
		},
		{
			name:           "🔴 issued asset is not equal to issued asset with different issuer",
			localAsset:     Asset{Code: "USDC", Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"},
			horizonAsset:   base.Asset{Type: "credit_alphanum4", Code: "USDC", Issuer: "another-issuer"},
			expectedResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.localAsset.EqualsHorizonAsset(tc.horizonAsset)
			assert.Equal(t, tc.expectedResult, got)
		})
	}
}

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
		expected := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
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
		expected := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
		actual, err := assetModel.GetByCodeAndIssuer(ctx, expected.Code, expected.Issuer)
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	})
}

func Test_AssetModelExistsByCodeOrID(t *testing.T) {
	models := SetupModels(t)
	ctx := context.Background()

	t.Run("returns false when asset does not exist", func(t *testing.T) {
		exists, err := models.Assets.ExistsByCodeOrID(ctx, "NONEXISTENT")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("returns true when asset exists by code", func(t *testing.T) {
		asset := CreateAssetFixture(t, ctx, models.DBConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

		exists, err := models.Assets.ExistsByCodeOrID(ctx, asset.Code)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("returns true when asset exists by ID", func(t *testing.T) {
		asset := CreateAssetFixture(t, ctx, models.DBConnectionPool, "XLM", "")

		exists, err := models.Assets.ExistsByCodeOrID(ctx, asset.ID)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("returns false for soft-deleted asset", func(t *testing.T) {
		asset := CreateAssetFixture(t, ctx, models.DBConnectionPool, "DELETED", "GBVHJTRLQRMIHRYTXZQOPVYCVVH7IRJN3DOFT7VC6U75CBWWBVDTWURG")
		_, err := models.Assets.SoftDelete(ctx, models.DBConnectionPool, asset.ID)
		require.NoError(t, err)

		exists, err := models.Assets.ExistsByCodeOrID(ctx, asset.Code)
		require.NoError(t, err)
		assert.False(t, exists)

		exists, err = models.Assets.ExistsByCodeOrID(ctx, asset.ID)
		require.NoError(t, err)
		assert.False(t, exists)
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
		expected := ClearAndCreateAssetFixtures(t, ctx, dbConnectionPool)
		actual, err := assetModel.GetAll(ctx)
		require.NoError(t, err)

		assert.Equal(t, expected, actual)
	})

	t.Run("returns empty array when no assets", func(t *testing.T) {
		DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		actual, err := assetModel.GetAll(ctx)
		require.NoError(t, err)

		assert.Equal(t, []Asset{}, actual)
	})
}

func Test_AssetModelGetByWalletID(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	assetModel := &AssetModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns all assets associated with a walletID successfully", func(t *testing.T) {
		assets := ClearAndCreateAssetFixtures(t, ctx, dbConnectionPool)
		require.Equal(t, 2, len(assets))

		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "walletA", "https://www.a.com", "www.a.com", "a://")
		require.NotNil(t, wallet)

		AssociateAssetWithWalletFixture(t, ctx, dbConnectionPool, assets[0].ID, wallet.ID)

		actual, err := assetModel.GetByWalletID(ctx, wallet.ID)
		require.NoError(t, err)
		require.Len(t, actual, 1)
		require.Equal(t, assets[0].ID, actual[0].ID)
		require.Equal(t, assets[0].Code, actual[0].Code)
		require.Equal(t, assets[0].Issuer, actual[0].Issuer)
	})

	t.Run("returns empty array when no assets associated with walletID", func(t *testing.T) {
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "walletB", "https://www.b.com", "www.b.com", "b://")

		actual, err := assetModel.GetByWalletID(ctx, wallet.ID)
		require.NoError(t, err)

		assert.Equal(t, []Asset{}, actual)
	})
}

func Test_AssetModel_Insert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	assetModel := &AssetModel{dbConnectionPool: dbConnectionPool}

	t.Run("inserts asset successfully", func(t *testing.T) {
		DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
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
		DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
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

	t.Run("asset insertion is idempotent", func(t *testing.T) {
		DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		code := "USDT"
		issuer := "GBVHJTRLQRMIHRYTXZQOPVYCVVH7IRJN3DOFT7VC6U75CBWWBVDTWURG"

		asset, err := assetModel.Insert(ctx, dbConnectionPool, code, issuer)
		require.NoError(t, err)
		assert.NotNil(t, asset)

		idempotentAsset, err := assetModel.Insert(ctx, dbConnectionPool, code, issuer)
		require.NoError(t, err)
		assert.NotNil(t, idempotentAsset)
		assert.Equal(t, asset.Code, idempotentAsset.Code)
		assert.Equal(t, asset.Issuer, idempotentAsset.Issuer)
		assert.Equal(t, asset.DeletedAt, idempotentAsset.DeletedAt)
		assert.Empty(t, asset.DeletedAt)
	})

	t.Run("creates the stellar native asset successfully", func(t *testing.T) {
		DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		asset, err := assetModel.Insert(ctx, dbConnectionPool, "XLM", "")
		require.NoError(t, err)
		assert.NotNil(t, asset)

		assert.Equal(t, "XLM", asset.Code)
		assert.Empty(t, asset.Issuer)
	})

	t.Run("does not create an asset with empty issuer (unless it's XLM)", func(t *testing.T) {
		DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		asset, err := assetModel.Insert(ctx, dbConnectionPool, "USDC", "")
		assert.ErrorContains(t, err, `error inserting asset: pq: new row for relation "assets" violates check constraint "asset_issuer_length_check"`)
		assert.Nil(t, asset)
	})

	t.Run("does not create an asset with a invalid issuer", func(t *testing.T) {
		DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		asset, err := assetModel.Insert(ctx, dbConnectionPool, "USDC", "INVALID")
		assert.ErrorContains(t, err, `error inserting asset: pq: new row for relation "assets" violates check constraint "asset_issuer_length_check"`)
		assert.Nil(t, asset)

		asset, err = assetModel.Insert(ctx, dbConnectionPool, "XLM", "INVALID")
		assert.ErrorContains(t, err, `error inserting asset: pq: new row for relation "assets" violates check constraint "asset_issuer_length_check"`)
		assert.Nil(t, asset)
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
		require.ErrorContains(t, err, "error getting or creating asset: pq: new row for relation \"assets\" violates check constraint \"asset_issuer_length_check\"")
		assert.Empty(t, asset)
	})

	t.Run("creates asset successfully", func(t *testing.T) {
		asset, err := assetModel.GetOrCreate(ctx, "F001", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
		require.NoError(t, err)
		assert.Equal(t, "F001", asset.Code)
		assert.Equal(t, "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV", asset.Issuer)
	})

	t.Run("returns asset successfully", func(t *testing.T) {
		expected := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
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
		DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		expected := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

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
		DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		_, err := assetModel.SoftDelete(ctx, dbConnectionPool, "non-existant")
		require.Error(t, err)
	})
}

func Test_GetAssetsPerReceiverWallet(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)
	ctx := context.Background()

	models, err := NewModels(dbConnectionPool)
	require.NoError(t, err)

	// 1. Create assets, wallets and disbursements:
	asset1 := CreateAssetFixture(t, ctx, dbConnectionPool, "FOO1", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	asset2 := CreateAssetFixture(t, ctx, dbConnectionPool, "FOO2", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	walletA := CreateWalletFixture(t, ctx, dbConnectionPool, "walletA", "https://www.a.com", "www.a.com", "a://")
	walletB := CreateWalletFixture(t, ctx, dbConnectionPool, "walletB", "https://www.b.com", "www.b.com", "b://")
	walletC := CreateWalletFixture(t, ctx, dbConnectionPool, "walletC", "https://www.c.com", "www.c.com", "c://")
	walletD := CreateWalletFixture(t, ctx, dbConnectionPool, "walletD", "https://www.d.com", "www.d.com", "d://")

	disbursementA1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet:                              walletA,
		Status:                              ReadyDisbursementStatus,
		Asset:                               asset1,
		ReceiverRegistrationMessageTemplate: "Disbursement SMS Registration Message Template A1",
		VerificationField:                   VerificationTypeDateOfBirth,
	})
	disbursementA2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet:                              walletA,
		Status:                              ReadyDisbursementStatus,
		Asset:                               asset2,
		ReceiverRegistrationMessageTemplate: "Disbursement SMS Registration Message Template A2",
		VerificationField:                   VerificationTypeNationalID,
	})
	disbursementB1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet:                              walletB,
		Status:                              ReadyDisbursementStatus,
		Asset:                               asset1,
		ReceiverRegistrationMessageTemplate: "Disbursement SMS Registration Message Template B1",
		VerificationField:                   VerificationTypePin,
	})
	disbursementB2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet:                              walletB,
		Status:                              ReadyDisbursementStatus,
		Asset:                               asset2,
		ReceiverRegistrationMessageTemplate: "Disbursement SMS Registration Message Template B2",
		VerificationField:                   VerificationTypeYearMonth,
	})
	disbursementC1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet:                              walletC,
		Status:                              ReadyDisbursementStatus,
		Asset:                               asset1,
		ReceiverRegistrationMessageTemplate: "Disbursement SMS Registration Message Template C1",
		VerificationField:                   VerificationTypeDateOfBirth,
	})
	disbursementC2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet:                              walletC,
		Status:                              ReadyDisbursementStatus,
		Asset:                               asset2,
		ReceiverRegistrationMessageTemplate: "Disbursement SMS Registration Message Template C2",
		VerificationField:                   VerificationTypeNationalID,
	})
	disbursementD1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet:                              walletD,
		Status:                              ReadyDisbursementStatus,
		Asset:                               asset1,
		ReceiverRegistrationMessageTemplate: "Disbursement SMS Registration Message Template D1",
		VerificationField:                   VerificationTypePin,
	})
	disbursementD2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet:                              walletD,
		Status:                              ReadyDisbursementStatus,
		Asset:                               asset2,
		ReceiverRegistrationMessageTemplate: "Disbursement SMS Registration Message Template D2",
		VerificationField:                   VerificationTypeYearMonth,
	})

	// 2. Create receivers, and receiver wallets:
	receiverX := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverY := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})

	receiverWalletXA := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverX.ID, walletA.ID, DraftReceiversWalletStatus)
	receiverWalletXB := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverX.ID, walletB.ID, DraftReceiversWalletStatus)

	// ReceiverY uses walletC and walletD (different stellar address from receiverX)
	receiverWalletYC := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverY.ID, walletC.ID, DraftReceiversWalletStatus)
	receiverWalletYD := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverY.ID, walletD.ID, DraftReceiversWalletStatus)

	// 3. Create payments:
	// paymentXA1 - walletA, asset1 for receiverX on their receiverWalletA
	_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: receiverWalletXA,
		Disbursement:   disbursementA1,
		Asset:          *asset1,
		Status:         ReadyPaymentStatus,
		Amount:         "1",
	})

	var invitationSentAt time.Time
	const q = "UPDATE receiver_wallets SET invitation_sent_at = NOW() WHERE id = $1 RETURNING invitation_sent_at"
	err = dbConnectionPool.GetContext(ctx, &invitationSentAt, q, receiverWalletXA.ID)
	require.NoError(t, err)

	now := time.Now()
	_ = CreateMessageFixture(t, ctx, dbConnectionPool, &Message{
		Type:             message.MessengerTypeDryRun,
		AssetID:          &asset1.ID,
		ReceiverID:       receiverX.ID,
		WalletID:         walletA.ID,
		ReceiverWalletID: &receiverWalletXA.ID,
		TextEncrypted:    "Message",
		Status:           SuccessMessageStatus,
		StatusHistory: []MessageStatusHistoryEntry{
			{
				StatusMessage: nil,
				Status:        SuccessMessageStatus,
				Timestamp:     now.AddDate(0, 0, 1),
			},
		},
		CreatedAt: now.AddDate(0, 0, 1),
		UpdatedAt: now.AddDate(0, 0, 1),
	})

	_ = CreateMessageFixture(t, ctx, dbConnectionPool, &Message{
		Type:             message.MessengerTypeDryRun,
		AssetID:          &asset1.ID,
		ReceiverID:       receiverX.ID,
		WalletID:         walletA.ID,
		ReceiverWalletID: &receiverWalletXA.ID,
		TextEncrypted:    "Message",
		Status:           SuccessMessageStatus,
		StatusHistory: []MessageStatusHistoryEntry{
			{
				StatusMessage: nil,
				Status:        SuccessMessageStatus,
				Timestamp:     now.AddDate(0, 0, 2),
			},
		},
		CreatedAt: now.AddDate(0, 0, 2),
		UpdatedAt: now.AddDate(0, 0, 2),
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

	// Payments for receiverY (using walletC and walletD)
	_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: receiverWalletYC,
		Disbursement:   disbursementC2,
		Asset:          *asset2,
		Status:         ReadyPaymentStatus,
		UpdatedAt:      time.Date(2024, 1, 6, 0, 0, 0, 0, time.UTC),
		Amount:         "1",
	})

	_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: receiverWalletYC,
		Disbursement:   disbursementC1,
		Asset:          *asset1,
		Status:         ReadyPaymentStatus,
		UpdatedAt:      time.Date(2024, 2, 5, 0, 0, 0, 0, time.UTC),
		Amount:         "1",
	})

	_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: receiverWalletYD,
		Disbursement:   disbursementD1,
		Asset:          *asset1,
		Status:         ReadyPaymentStatus,
		UpdatedAt:      time.Date(2024, 1, 7, 0, 0, 0, 0, time.UTC),
		Amount:         "1",
	})

	_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: receiverWalletYD,
		Disbursement:   disbursementD2,
		Asset:          *asset2,
		Status:         ReadyPaymentStatus,
		UpdatedAt:      time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC),
		Amount:         "1",
	})

	gotLatestAssetsPerRW, err := models.Assets.GetAssetsPerReceiverWallet(ctx, receiverWalletXA, receiverWalletXB, receiverWalletYC, receiverWalletYD)
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
				ReceiverWalletStats: ReceiverWalletStats{
					TotalInvitationResentAttempts: 2,
				},
				InvitationSentAt: &invitationSentAt,
			},
			WalletID: walletA.ID,
			Asset:    *asset1,
			DisbursementReceiverRegistrationMsgTemplate: &disbursementA1.ReceiverRegistrationMessageTemplate,
			VerificationField: disbursementA1.VerificationField,
		},
		{
			ReceiverWallet: ReceiverWallet{
				ID: receiverWalletXA.ID,
				Receiver: Receiver{
					ID:          receiverX.ID,
					Email:       receiverX.Email,
					PhoneNumber: receiverX.PhoneNumber,
				},
				InvitationSentAt: &invitationSentAt,
			},
			WalletID: walletA.ID,
			Asset:    *asset2,
			DisbursementReceiverRegistrationMsgTemplate: &disbursementA2.ReceiverRegistrationMessageTemplate,
			VerificationField: disbursementA2.VerificationField,
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
			DisbursementReceiverRegistrationMsgTemplate: &disbursementB1.ReceiverRegistrationMessageTemplate,
			VerificationField: disbursementB1.VerificationField,
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
			DisbursementReceiverRegistrationMsgTemplate: &disbursementB2.ReceiverRegistrationMessageTemplate,
			VerificationField: disbursementB2.VerificationField,
		},
		{
			ReceiverWallet: ReceiverWallet{
				ID: receiverWalletYC.ID,
				Receiver: Receiver{
					ID:          receiverY.ID,
					Email:       receiverY.Email,
					PhoneNumber: receiverY.PhoneNumber,
				},
			},
			WalletID: walletC.ID,
			Asset:    *asset1,
			DisbursementReceiverRegistrationMsgTemplate: &disbursementC1.ReceiverRegistrationMessageTemplate,
			VerificationField: disbursementC1.VerificationField,
		},
		{
			ReceiverWallet: ReceiverWallet{
				ID: receiverWalletYC.ID,
				Receiver: Receiver{
					ID:          receiverY.ID,
					Email:       receiverY.Email,
					PhoneNumber: receiverY.PhoneNumber,
				},
			},
			WalletID: walletC.ID,
			Asset:    *asset2,
			DisbursementReceiverRegistrationMsgTemplate: &disbursementC2.ReceiverRegistrationMessageTemplate,
			VerificationField: disbursementC2.VerificationField,
		},
		{
			ReceiverWallet: ReceiverWallet{
				ID: receiverWalletYD.ID,
				Receiver: Receiver{
					ID:          receiverY.ID,
					Email:       receiverY.Email,
					PhoneNumber: receiverY.PhoneNumber,
				},
			},
			WalletID: walletD.ID,
			Asset:    *asset1,
			DisbursementReceiverRegistrationMsgTemplate: &disbursementD1.ReceiverRegistrationMessageTemplate,
			VerificationField: disbursementD1.VerificationField,
		},
		{
			ReceiverWallet: ReceiverWallet{
				ID: receiverWalletYD.ID,
				Receiver: Receiver{
					ID:          receiverY.ID,
					Email:       receiverY.Email,
					PhoneNumber: receiverY.PhoneNumber,
				},
			},
			WalletID: walletD.ID,
			Asset:    *asset2,
			DisbursementReceiverRegistrationMsgTemplate: &disbursementD2.ReceiverRegistrationMessageTemplate,
			VerificationField: disbursementD2.VerificationField,
		},
	}

	assert.ElementsMatch(t, wantLatestAssetsPerRW, gotLatestAssetsPerRW)
}
