package services

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_SetupAssetsForProperNetwork(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	platform := schema.StellarPlatform

	ctx := context.Background()

	t.Run("returns error when a invalid network is set", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		err := SetupAssetsForProperNetwork(ctx, dbConnectionPool, "invalid", platform)
		assert.EqualError(t, err, "invalid network provided")
	})

	t.Run("inserts new assets when it doesn't exist", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		buf := new(strings.Builder)
		log.DefaultLogger.SetLevel(log.InfoLevel)
		log.DefaultLogger.SetOutput(buf)

		err := SetupAssetsForProperNetwork(ctx, dbConnectionPool, utils.TestnetNetworkType, platform)
		require.NoError(t, err)

		allAssets, err := models.Assets.GetAll(ctx)
		require.NoError(t, err)

		assert.Len(t, allAssets, 3)
		assert.Equal(t, assets.EURCAssetCode, allAssets[0].Code)
		assert.Equal(t, assets.EURCAssetIssuerTestnet, allAssets[0].Issuer)
		assert.Equal(t, assets.USDCAssetCode, allAssets[1].Code)
		assert.Equal(t, assets.USDCAssetIssuerTestnet, allAssets[1].Issuer)
		assert.Equal(t, assets.XLMAssetCode, allAssets[2].Code)
		assert.Empty(t, allAssets[2].Issuer)

		expectedLogs := []string{
			"updating/inserting assets for the 'testnet' network",
			fmt.Sprintf("Code: %s", assets.EURCAssetCode),
			fmt.Sprintf("Code: %s", assets.USDCAssetCode),
			fmt.Sprintf("Code: %s", assets.XLMAssetCode),
			fmt.Sprintf("Issuer: %s", assets.EURCAssetIssuerTestnet),
			fmt.Sprintf("Issuer: %s", assets.USDCAssetIssuerTestnet),
			"Issuer: ",
		}

		logs := buf.String()
		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}
	})

	t.Run("updates and inserts assets", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		data.CreateAssetFixture(t, ctx, dbConnectionPool, assets.EURCAssetCode, assets.EURCAssetIssuerTestnet)

		assert.NotEqual(t, assets.EURCAssetIssuerTestnet, assets.EURCAssetIssuerPubnet)

		allAssets, err := models.Assets.GetAll(ctx)
		require.NoError(t, err)
		assert.Len(t, allAssets, 1)
		assert.Equal(t, assets.EURCAssetCode, allAssets[0].Code)
		assert.Equal(t, assets.EURCAssetIssuerTestnet, allAssets[0].Issuer)

		buf := new(strings.Builder)
		log.DefaultLogger.SetLevel(log.InfoLevel)
		log.DefaultLogger.SetOutput(buf)

		err = SetupAssetsForProperNetwork(ctx, dbConnectionPool, utils.TestnetNetworkType, platform)
		require.NoError(t, err)

		allAssets, err = models.Assets.GetAll(ctx)
		require.NoError(t, err)

		assert.Len(t, allAssets, 3)
		assert.Equal(t, assets.EURCAssetCode, allAssets[0].Code)
		assert.Equal(t, assets.EURCAssetIssuerTestnet, allAssets[0].Issuer)
		assert.Equal(t, assets.USDCAssetCode, allAssets[1].Code)
		assert.Equal(t, assets.USDCAssetIssuerTestnet, allAssets[1].Issuer)
		assert.Equal(t, assets.XLMAssetCode, allAssets[2].Code)
		assert.Empty(t, allAssets[2].Issuer)

		expectedLogs := []string{
			"updating/inserting assets for the 'testnet' network",
			fmt.Sprintf("Code: %s", assets.EURCAssetCode),
			fmt.Sprintf("Code: %s", assets.USDCAssetCode),
			fmt.Sprintf("Code: %s", assets.XLMAssetCode),
			fmt.Sprintf("Issuer: %s", assets.EURCAssetIssuerTestnet),
			fmt.Sprintf("Issuer: %s", assets.USDCAssetIssuerTestnet),
			"Issuer: ",
		}

		logs := buf.String()
		fmt.Println(logs)
		fmt.Println(expectedLogs)
		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}
	})

	t.Run("doesn't change the asset when it's not in the assetsNetworkMap", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		data.CreateAssetFixture(t, ctx, dbConnectionPool, assets.EURCAssetCode, assets.EURCAssetIssuerPubnet)

		pubnetARSTIssuer := keypair.MustRandom().Address()
		arstAssetCode := "ARST"
		data.CreateAssetFixture(t, ctx, dbConnectionPool, arstAssetCode, pubnetARSTIssuer)

		assert.NotEqual(t, assets.EURCAssetIssuerTestnet, assets.EURCAssetIssuerPubnet)

		allAssets, err := models.Assets.GetAll(ctx)
		require.NoError(t, err)
		assert.Len(t, allAssets, 2)
		assert.Equal(t, arstAssetCode, allAssets[0].Code)
		assert.Equal(t, pubnetARSTIssuer, allAssets[0].Issuer)
		assert.Equal(t, assets.EURCAssetCode, allAssets[1].Code)
		assert.Equal(t, assets.EURCAssetIssuerPubnet, allAssets[1].Issuer)

		buf := new(strings.Builder)
		log.DefaultLogger.SetLevel(log.InfoLevel)
		log.DefaultLogger.SetOutput(buf)

		err = SetupAssetsForProperNetwork(ctx, dbConnectionPool, utils.PubnetNetworkType, platform)
		require.NoError(t, err)

		allAssets, err = models.Assets.GetAll(ctx)
		require.NoError(t, err)
		assert.Len(t, allAssets, 4)
		assert.Equal(t, arstAssetCode, allAssets[0].Code)
		assert.Equal(t, pubnetARSTIssuer, allAssets[0].Issuer)
		assert.Equal(t, assets.EURCAssetCode, allAssets[1].Code)
		assert.Equal(t, assets.EURCAssetIssuerPubnet, allAssets[1].Issuer)
		assert.Equal(t, assets.USDCAssetCode, allAssets[2].Code)
		assert.Equal(t, assets.USDCAssetIssuerPubnet, allAssets[2].Issuer)
		assert.Equal(t, assets.XLMAssetCode, allAssets[3].Code)
		assert.Empty(t, allAssets[3].Issuer)

		expectedLogs := []string{
			"updating/inserting assets for the 'pubnet' network",
			fmt.Sprintf("Code: %s", arstAssetCode),
			fmt.Sprintf("Code: %s", assets.EURCAssetCode),
			fmt.Sprintf("Code: %s", assets.USDCAssetCode),
			fmt.Sprintf("Code: %s", assets.XLMAssetCode),
			fmt.Sprintf("Issuer: %s", pubnetARSTIssuer),
			fmt.Sprintf("Issuer: %s", assets.EURCAssetIssuerPubnet),
			fmt.Sprintf("Issuer: %s", assets.USDCAssetIssuerPubnet),
			"Issuer: ",
		}

		logs := buf.String()
		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}
	})

	t.Run("inserts Circle-only assets when specifying platform", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		allAssets, err := models.Assets.GetAll(ctx)
		require.NoError(t, err)
		assert.Len(t, allAssets, 0)

		buf := new(strings.Builder)
		log.DefaultLogger.SetLevel(log.InfoLevel)
		log.DefaultLogger.SetOutput(buf)

		err = SetupAssetsForProperNetwork(ctx, dbConnectionPool, utils.PubnetNetworkType, schema.CirclePlatform)
		require.NoError(t, err)

		allAssets, err = models.Assets.GetAll(ctx)
		require.NoError(t, err)
		assert.Len(t, allAssets, 2)
		assert.Equal(t, assets.EURCAssetCode, allAssets[0].Code)
		assert.Equal(t, assets.EURCAssetIssuerPubnet, allAssets[0].Issuer)
		assert.Equal(t, assets.USDCAssetCode, allAssets[1].Code)
		assert.Equal(t, assets.USDCAssetIssuerPubnet, allAssets[1].Issuer)

		expectedLogs := []string{
			"updating/inserting assets for the 'pubnet' network",
			fmt.Sprintf("Code: %s", assets.EURCAssetCode),
			fmt.Sprintf("Code: %s", assets.USDCAssetCode),
			fmt.Sprintf("Issuer: %s", assets.EURCAssetIssuerPubnet),
			fmt.Sprintf("Issuer: %s", assets.USDCAssetIssuerPubnet),
		}

		logs := buf.String()
		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}
	})
}
