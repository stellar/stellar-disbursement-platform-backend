package services

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	t.Run("returns error when a invalid network is set", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		err := SetupAssetsForProperNetwork(ctx, dbConnectionPool, "invalid", DefaultAssetsNetworkMap)
		assert.EqualError(t, err, "invalid network provided")
	})

	t.Run("inserts new assets when it doesn't exist", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		buf := new(strings.Builder)
		log.DefaultLogger.SetLevel(log.InfoLevel)
		log.DefaultLogger.SetOutput(buf)

		err := SetupAssetsForProperNetwork(ctx, dbConnectionPool, utils.TestnetNetworkType, DefaultAssetsNetworkMap)
		require.NoError(t, err)

		assets, err := models.Assets.GetAll(ctx)
		require.NoError(t, err)

		assert.Len(t, assets, 2)
		assert.Equal(t, "USDC", assets[0].Code)
		assert.Equal(t, "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", assets[0].Issuer)
		assert.Equal(t, "XLM", assets[1].Code)
		assert.Empty(t, assets[1].Issuer)

		expectedLogs := []string{
			"updating/inserting assets for the 'testnet' network",
			"Code: USDC",
			"Issuer: GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			"Code: XLM",
			"Issuer: ",
		}

		logs := buf.String()
		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}
	})

	t.Run("updates and inserts assets", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		pubnetEUROCIssuer := keypair.MustRandom().Address()
		data.CreateAssetFixture(t, ctx, dbConnectionPool, "EUROC", pubnetEUROCIssuer)

		testnetUSDCIssuer := keypair.MustRandom().Address()
		testnetEUROCIssuer := keypair.MustRandom().Address()

		assert.NotEqual(t, testnetEUROCIssuer, pubnetEUROCIssuer)

		assets, err := models.Assets.GetAll(ctx)
		require.NoError(t, err)
		assert.Len(t, assets, 1)
		assert.Equal(t, "EUROC", assets[0].Code)
		assert.Equal(t, pubnetEUROCIssuer, assets[0].Issuer)

		assetsNetworkMap := AssetsNetworkMapType{
			utils.TestnetNetworkType: []data.Asset{
				{Code: "EUROC", Issuer: testnetEUROCIssuer},
				{Code: "USDC", Issuer: testnetUSDCIssuer},
			},
		}

		buf := new(strings.Builder)
		log.DefaultLogger.SetLevel(log.InfoLevel)
		log.DefaultLogger.SetOutput(buf)

		err = SetupAssetsForProperNetwork(ctx, dbConnectionPool, utils.TestnetNetworkType, assetsNetworkMap)
		require.NoError(t, err)

		assets, err = models.Assets.GetAll(ctx)
		require.NoError(t, err)

		assert.Len(t, assets, 2)
		assert.Equal(t, "EUROC", assets[0].Code)
		assert.Equal(t, testnetEUROCIssuer, assets[0].Issuer)
		assert.Equal(t, "USDC", assets[1].Code)
		assert.Equal(t, testnetUSDCIssuer, assets[1].Issuer)

		expectedLogs := []string{
			"updating/inserting assets for the 'testnet' network",
			"Code: EUROC",
			"Code: USDC",
			fmt.Sprintf("Issuer: %s", testnetEUROCIssuer),
			fmt.Sprintf("Issuer: %s", testnetEUROCIssuer),
		}

		logs := buf.String()
		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}
	})

	t.Run("doesn't change the asset when it's not in the assetsNetworkMap", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		testnetEUROCIssuer := keypair.MustRandom().Address()
		data.CreateAssetFixture(t, ctx, dbConnectionPool, "EUROC", testnetEUROCIssuer)

		pubnetARSTIssuer := keypair.MustRandom().Address()
		data.CreateAssetFixture(t, ctx, dbConnectionPool, "ARST", pubnetARSTIssuer)

		pubnetUSDCIssuer := keypair.MustRandom().Address()
		pubnetEUROCIssuer := keypair.MustRandom().Address()

		assert.NotEqual(t, testnetEUROCIssuer, pubnetEUROCIssuer)

		assets, err := models.Assets.GetAll(ctx)
		require.NoError(t, err)
		assert.Len(t, assets, 2)
		assert.Equal(t, "ARST", assets[0].Code)
		assert.Equal(t, pubnetARSTIssuer, assets[0].Issuer)
		assert.Equal(t, "EUROC", assets[1].Code)
		assert.Equal(t, testnetEUROCIssuer, assets[1].Issuer)

		assetsNetworkMap := AssetsNetworkMapType{
			utils.PubnetNetworkType: []data.Asset{
				{Code: "EUROC", Issuer: pubnetEUROCIssuer},
				{Code: "USDC", Issuer: pubnetUSDCIssuer},
			},
		}

		buf := new(strings.Builder)
		log.DefaultLogger.SetLevel(log.InfoLevel)
		log.DefaultLogger.SetOutput(buf)

		err = SetupAssetsForProperNetwork(ctx, dbConnectionPool, utils.PubnetNetworkType, assetsNetworkMap)
		require.NoError(t, err)

		assets, err = models.Assets.GetAll(ctx)
		require.NoError(t, err)
		assert.Len(t, assets, 3)
		assert.Equal(t, "ARST", assets[0].Code)
		assert.Equal(t, pubnetARSTIssuer, assets[0].Issuer)
		assert.Equal(t, "EUROC", assets[1].Code)
		assert.Equal(t, pubnetEUROCIssuer, assets[1].Issuer)
		assert.Equal(t, "USDC", assets[2].Code)
		assert.Equal(t, pubnetUSDCIssuer, assets[2].Issuer)

		expectedLogs := []string{
			"updating/inserting assets for the 'pubnet' network",
			"Code: ARST",
			"Code: EUROC",
			"Code: USDC",
			fmt.Sprintf("Issuer: %s", pubnetARSTIssuer),
			fmt.Sprintf("Issuer: %s", pubnetEUROCIssuer),
			fmt.Sprintf("Issuer: %s", pubnetUSDCIssuer),
		}

		logs := buf.String()
		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}
	})
}
