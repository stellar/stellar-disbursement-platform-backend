package services

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/support/log"
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

	ctx := context.Background()

	t.Run("[Stellar,Invalidnet] returns error when a invalid network is set", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		err := SetupAssetsForProperNetwork(ctx, dbConnectionPool, "Invalidnet", schema.StellarPlatform)
		assert.EqualError(t, err, "invalid network provided")
	})

	t.Run("[Stellar,Testnet] inserts new assets", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		allAssets, err := models.Assets.GetAll(ctx)
		require.NoError(t, err)
		assert.Len(t, allAssets, 0)

		buf := new(strings.Builder)
		log.DefaultLogger.SetLevel(log.InfoLevel)
		log.DefaultLogger.SetOutput(buf)

		err = SetupAssetsForProperNetwork(ctx, dbConnectionPool, utils.TestnetNetworkType, schema.StellarPlatform)
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
			fmt.Sprintf("Asset codes to be updated/inserted: %v", []string{assets.EURCAssetCode, assets.USDCAssetCode, assets.XLMAssetCode}),
			fmt.Sprintf("* %s - %s", assets.EURCAssetCode, assets.EURCAssetIssuerTestnet),
			fmt.Sprintf("* %s - %s", assets.USDCAssetCode, assets.USDCAssetIssuerTestnet),
			fmt.Sprintf("* %s - %s", assets.XLMAssetCode, ""),
		}

		logs := buf.String()
		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}
	})

	t.Run("[Stellar,Testnet] updates existing asset with wrong issuer and inserts new assets", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		// Start with EURC:{randomIssuer}
		randomIssuer := keypair.MustRandom().Address()
		data.CreateAssetFixture(t, ctx, dbConnectionPool, assets.EURCAssetCode, randomIssuer)

		allAssets, err := models.Assets.GetAll(ctx)
		require.NoError(t, err)
		assert.Len(t, allAssets, 1)
		assert.Equal(t, assets.EURCAssetCode, allAssets[0].Code)
		assert.Equal(t, randomIssuer, allAssets[0].Issuer)

		buf := new(strings.Builder)
		log.DefaultLogger.SetLevel(log.InfoLevel)
		log.DefaultLogger.SetOutput(buf)

		err = SetupAssetsForProperNetwork(ctx, dbConnectionPool, utils.TestnetNetworkType, schema.StellarPlatform)
		require.NoError(t, err)

		allAssets, err = models.Assets.GetAll(ctx)
		require.NoError(t, err)

		assert.Len(t, allAssets, 3)
		assert.Equal(t, assets.EURCAssetCode, allAssets[0].Code)
		assert.Equal(t, assets.EURCAssetIssuerTestnet, allAssets[0].Issuer) // <--- Issuer was updated
		assert.Equal(t, assets.USDCAssetCode, allAssets[1].Code)
		assert.Equal(t, assets.USDCAssetIssuerTestnet, allAssets[1].Issuer)
		assert.Equal(t, assets.XLMAssetCode, allAssets[2].Code)
		assert.Empty(t, allAssets[2].Issuer)

		expectedLogs := []string{
			"updating/inserting assets for the 'testnet' network",
			fmt.Sprintf("Asset codes to be updated/inserted: %v", []string{assets.EURCAssetCode, assets.USDCAssetCode, assets.XLMAssetCode}),
			fmt.Sprintf("* %s - %s", assets.EURCAssetCode, assets.EURCAssetIssuerTestnet),
			fmt.Sprintf("* %s - %s", assets.USDCAssetCode, assets.USDCAssetIssuerTestnet),
			fmt.Sprintf("* %s - %s", assets.XLMAssetCode, ""),
		}

		logs := buf.String()
		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}
	})

	t.Run("[Stellar,Futurenet] updates existing asset with wrong issuer and inserts new assets", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		allAssets, err := models.Assets.GetAll(ctx)
		require.NoError(t, err)
		assert.Len(t, allAssets, 0)

		buf := new(strings.Builder)
		log.DefaultLogger.SetLevel(log.InfoLevel)
		log.DefaultLogger.SetOutput(buf)

		err = SetupAssetsForProperNetwork(ctx, dbConnectionPool, utils.FuturenetNetworkType, schema.StellarPlatform)
		require.NoError(t, err)

		allAssets, err = models.Assets.GetAll(ctx)
		require.NoError(t, err)

		assert.Len(t, allAssets, 1)
		assert.Equal(t, assets.XLMAssetCode, allAssets[0].Code)
		assert.Empty(t, allAssets[0].Issuer)

		expectedLogs := []string{
			"updating/inserting assets for the 'futurenet' network",
			fmt.Sprintf("Asset codes to be updated/inserted: %v", []string{assets.XLMAssetCode}),
			fmt.Sprintf("* %s - %s", assets.XLMAssetCode, ""),
		}

		logs := buf.String()
		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}
	})

	t.Run("[Stellar,Pubnet] doesn't change the asset when it's not in the StellarAssetsNetworkMap", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		// ARST should not get updated since it's not in the StellarAssetsNetworkMap
		pubnetARSTIssuer := keypair.MustRandom().Address()
		arstAssetCode := "ARST"
		data.CreateAssetFixture(t, ctx, dbConnectionPool, arstAssetCode, pubnetARSTIssuer)

		allAssets, err := models.Assets.GetAll(ctx)
		require.NoError(t, err)
		assert.Len(t, allAssets, 1)
		assert.Equal(t, arstAssetCode, allAssets[0].Code)
		assert.Equal(t, pubnetARSTIssuer, allAssets[0].Issuer)

		buf := new(strings.Builder)
		log.DefaultLogger.SetLevel(log.InfoLevel)
		log.DefaultLogger.SetOutput(buf)

		err = SetupAssetsForProperNetwork(ctx, dbConnectionPool, utils.PubnetNetworkType, schema.StellarPlatform)
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
			fmt.Sprintf("Asset codes to be updated/inserted: %v", []string{assets.EURCAssetCode, assets.USDCAssetCode, assets.XLMAssetCode}),
			fmt.Sprintf("* %s - %s", arstAssetCode, pubnetARSTIssuer),
			fmt.Sprintf("* %s - %s", assets.EURCAssetCode, assets.EURCAssetIssuerPubnet),
			fmt.Sprintf("* %s - %s", assets.USDCAssetCode, assets.USDCAssetIssuerPubnet),
			fmt.Sprintf("* %s - %s", assets.XLMAssetCode, ""),
		}

		logs := buf.String()
		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}
	})

	t.Run("[Circle,Testnet] inserts new assets", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		allAssets, err := models.Assets.GetAll(ctx)
		require.NoError(t, err)
		assert.Len(t, allAssets, 0)

		buf := new(strings.Builder)
		log.DefaultLogger.SetLevel(log.InfoLevel)
		log.DefaultLogger.SetOutput(buf)

		err = SetupAssetsForProperNetwork(ctx, dbConnectionPool, utils.TestnetNetworkType, schema.CirclePlatform)
		require.NoError(t, err)

		allAssets, err = models.Assets.GetAll(ctx)
		require.NoError(t, err)

		assert.Len(t, allAssets, 2)
		assert.Equal(t, assets.EURCAssetCode, allAssets[0].Code)
		assert.Equal(t, assets.EURCAssetIssuerTestnet, allAssets[0].Issuer)
		assert.Equal(t, assets.USDCAssetCode, allAssets[1].Code)
		assert.Equal(t, assets.USDCAssetIssuerTestnet, allAssets[1].Issuer)

		expectedLogs := []string{
			"updating/inserting assets for the 'testnet' network",
			fmt.Sprintf("Asset codes to be updated/inserted: %v", []string{assets.EURCAssetCode, assets.USDCAssetCode}),
			fmt.Sprintf("* %s - %s", assets.EURCAssetCode, assets.EURCAssetIssuerTestnet),
			fmt.Sprintf("* %s - %s", assets.USDCAssetCode, assets.USDCAssetIssuerTestnet),
		}

		logs := buf.String()
		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}
	})

	t.Run("[Circle,Pubnet] updates existing asset with wrong issuer and inserts new assets", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		// Start with USDC:{randomIssuer}
		randomIssuer := keypair.MustRandom().Address()
		data.CreateAssetFixture(t, ctx, dbConnectionPool, assets.EURCAssetCode, randomIssuer)

		allAssets, err := models.Assets.GetAll(ctx)
		require.NoError(t, err)
		assert.Len(t, allAssets, 1)
		assert.Equal(t, assets.EURCAssetCode, allAssets[0].Code)
		assert.Equal(t, randomIssuer, allAssets[0].Issuer)

		buf := new(strings.Builder)
		log.DefaultLogger.SetLevel(log.InfoLevel)
		log.DefaultLogger.SetOutput(buf)

		err = SetupAssetsForProperNetwork(ctx, dbConnectionPool, utils.PubnetNetworkType, schema.CirclePlatform)
		require.NoError(t, err)

		allAssets, err = models.Assets.GetAll(ctx)
		require.NoError(t, err)

		assert.Len(t, allAssets, 2)
		assert.Equal(t, assets.EURCAssetCode, allAssets[0].Code)
		assert.Equal(t, assets.EURCAssetIssuerPubnet, allAssets[0].Issuer) // <--- Issuer was updated
		assert.Equal(t, assets.USDCAssetCode, allAssets[1].Code)
		assert.Equal(t, assets.USDCAssetIssuerPubnet, allAssets[1].Issuer)

		expectedLogs := []string{
			"updating/inserting assets for the 'pubnet' network",
			fmt.Sprintf("Asset codes to be updated/inserted: %v", []string{assets.EURCAssetCode, assets.USDCAssetCode}),
			fmt.Sprintf("* %s - %s", assets.EURCAssetCode, assets.EURCAssetIssuerPubnet),
			fmt.Sprintf("* %s - %s", assets.USDCAssetCode, assets.USDCAssetIssuerPubnet),
		}

		logs := buf.String()
		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}
	})
}
