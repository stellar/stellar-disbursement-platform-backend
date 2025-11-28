package services

import (
	"context"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func Test_SetupWalletsForProperNetwork(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	models, outerErr := data.NewModels(dbConnectionPool)
	require.NoError(t, outerErr)

	ctx := context.Background()
	t.Run("returns error when a invalid network is set", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		err := SetupWalletsForProperNetwork(ctx, dbConnectionPool, "invalid", DefaultWalletsNetworkMap)
		assert.EqualError(t, err, "invalid network provided")
	})

	t.Run("inserts new wallets when it doesn't exist", func(t *testing.T) {
		data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

		buf := new(strings.Builder)
		log.DefaultLogger.SetLevel(log.InfoLevel)
		log.DefaultLogger.SetOutput(buf)

		err := SetupWalletsForProperNetwork(ctx, dbConnectionPool, utils.PubnetNetworkType, DefaultWalletsNetworkMap)
		require.NoError(t, err)

		wallets, err := models.Wallets.GetAll(ctx)
		require.NoError(t, err)

		// Test only on Vesseo and Beans App. This will help adding wallets without breaking tests.
		var vesseo, beansApp data.Wallet

		for _, w := range wallets {
			if w.Name == "Vesseo" {
				vesseo = w
			} else if w.Name == "Beans App" {
				beansApp = w
			}
		}

		require.NotNil(t, vesseo, "Vesseo wallet not found")
		require.NotNil(t, beansApp, "Beans App wallet not found")

		assert.Equal(t, "Vesseo", vesseo.Name)
		assert.Equal(t, "Beans App", beansApp.Name)

		expectedLogs := []string{
			"updating/inserting wallets for the 'pubnet' network",
			"Name: Vesseo",
			"Homepage: https://vesseoapp.com",
			"Deep Link Schema: https://vesseoapp.com/disbursement",
			"SEP-10 Client Domain: vesseoapp.com",
			"Name: Beans App",
			"Homepage: https://beansapp.com",
		}

		logs := buf.String()
		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}
	})

	t.Run("updates and inserts wallets", func(t *testing.T) {
		data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

		data.CreateWalletFixture(t, ctx, dbConnectionPool, "Vesseo", "https://vesseoapp.com/old", "vesseoapp-old.com", "https://vesseoapp.com/old-disbursement")

		wallets, err := models.Wallets.GetAll(ctx)
		require.NoError(t, err)

		assert.Len(t, wallets, 1)
		assert.Equal(t, "Vesseo", wallets[0].Name)
		assert.Equal(t, "https://vesseoapp.com/old", wallets[0].Homepage)
		assert.Equal(t, "vesseoapp-old.com", wallets[0].SEP10ClientDomain)
		assert.Equal(t, "https://vesseoapp.com/old-disbursement", wallets[0].DeepLinkSchema)

		walletsNetworkMap := WalletsNetworkMapType{
			utils.PubnetNetworkType: {
				{
					Name:              "Vesseo",
					Homepage:          "https://vesseoapp.com",
					DeepLinkSchema:    "https://vesseoapp.com/disbursement",
					SEP10ClientDomain: "vesseoapp.com",
				},
				{
					Name:              "Beans App",
					Homepage:          "https://beansapp.com",
					DeepLinkSchema:    "https://www.beansapp.com/disbursements/registration?env=prod",
					SEP10ClientDomain: "api.beansapp.com",
				},
			},
		}

		buf := new(strings.Builder)
		log.DefaultLogger.SetLevel(log.InfoLevel)
		log.DefaultLogger.SetOutput(buf)

		err = SetupWalletsForProperNetwork(ctx, dbConnectionPool, utils.PubnetNetworkType, walletsNetworkMap)
		require.NoError(t, err)

		wallets, err = models.Wallets.GetAll(ctx)
		require.NoError(t, err)

		assert.Len(t, wallets, 2)

		// Find Beans App and Vesseo in the results (order might vary)
		var beansApp, vesseo data.Wallet
		for _, w := range wallets {
			if w.Name == "Beans App" {
				beansApp = w
			} else if w.Name == "Vesseo" {
				vesseo = w
			}
		}

		assert.Equal(t, "Beans App", beansApp.Name)
		assert.Equal(t, "https://beansapp.com", beansApp.Homepage)
		assert.Equal(t, "api.beansapp.com", beansApp.SEP10ClientDomain)
		assert.Equal(t, "https://www.beansapp.com/disbursements/registration?env=prod", beansApp.DeepLinkSchema)

		assert.Equal(t, "Vesseo", vesseo.Name)
		assert.Equal(t, "https://vesseoapp.com", vesseo.Homepage)
		assert.Equal(t, "vesseoapp.com", vesseo.SEP10ClientDomain)
		assert.Equal(t, "https://vesseoapp.com/disbursement", vesseo.DeepLinkSchema)

		expectedLogs := []string{
			"updating/inserting wallets for the 'pubnet' network",
			"Name: Beans App",
			"Homepage: https://beansapp.com",
			"Name: Vesseo",
			"Homepage: https://vesseoapp.com",
		}

		logs := buf.String()
		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}
	})

	t.Run("insert wallet assets", func(t *testing.T) {
		data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		// Create the USDC asset
		data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5")

		walletsNetworkMap := WalletsNetworkMapType{
			utils.PubnetNetworkType: {
				{
					Name:              "Vibrant Assist",
					Homepage:          "https://vibrantapp.com/vibrant-assist",
					DeepLinkSchema:    "https://vibrantapp.com/sdp-dev",
					SEP10ClientDomain: "vibrantapp.com",
					Assets: []data.Asset{
						{
							Code:   "USDC",
							Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
						},
					},
				},
				{
					Name:              "BOSS Money",
					Homepage:          "https://www.walletbyboss.com",
					DeepLinkSchema:    "https://www.walletbyboss.com",
					SEP10ClientDomain: "www.walletbyboss.com",
					Assets: []data.Asset{
						{
							Code:   "USDC",
							Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
						},
						{
							Code:   "XLM",
							Issuer: "",
						},
					},
				},
			},
		}

		err := SetupWalletsForProperNetwork(ctx, dbConnectionPool, utils.PubnetNetworkType, walletsNetworkMap)
		require.NoError(t, err)

		wallets, err := models.Wallets.GetAll(ctx)
		require.NoError(t, err)
		require.Len(t, wallets, 2)
		require.Equal(t, "BOSS Money", wallets[0].Name)
		require.Equal(t, "Vibrant Assist", wallets[1].Name)

		// validate BOSS Money wallet assets (only USDC for now)
		bossAssets, err := models.Wallets.GetAssets(ctx, wallets[0].ID)
		require.NoError(t, err)
		require.Len(t, bossAssets, 1)
		require.Equal(t, "USDC", bossAssets[0].Code)
		require.Equal(t, "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", bossAssets[0].Issuer)

		// validate Vibrant Assist wallet assets (USDC)
		vibrantAssets, err := models.Wallets.GetAssets(ctx, wallets[1].ID)
		require.NoError(t, err)
		assert.Len(t, vibrantAssets, 1)
		assert.Equal(t, "USDC", vibrantAssets[0].Code)
		assert.Equal(t, "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", vibrantAssets[0].Issuer)

		// now add XLM as an asset
		data.CreateAssetFixture(t, ctx, dbConnectionPool, "XLM", "")

		buf := new(strings.Builder)
		log.DefaultLogger.SetLevel(log.InfoLevel)
		log.DefaultLogger.SetOutput(buf)

		// run the setup function again
		err = SetupWalletsForProperNetwork(ctx, dbConnectionPool, utils.PubnetNetworkType, walletsNetworkMap)
		require.NoError(t, err)

		// validate BOSS Money wallet assets (USDC *and* XLM)
		bossAssets, err = models.Wallets.GetAssets(ctx, wallets[0].ID)
		require.NoError(t, err)
		assert.Len(t, bossAssets, 2)
		assert.Equal(t, "USDC", bossAssets[0].Code)
		assert.Equal(t, "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", bossAssets[0].Issuer)
		assert.Equal(t, "XLM", bossAssets[1].Code)
		assert.Equal(t, "", bossAssets[1].Issuer)

		// validate Vibrant Assist wallet assets (USDC)
		vibrantAssets, err = models.Wallets.GetAssets(ctx, wallets[1].ID)
		require.NoError(t, err)
		assert.Len(t, vibrantAssets, 1)
		assert.Equal(t, "USDC", vibrantAssets[0].Code)
		assert.Equal(t, "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", vibrantAssets[0].Issuer)

		expectedLogs := []string{
			"updating/inserting wallets for the 'pubnet' network",
			"Name: BOSS Money",
			"Homepage: https://www.walletbyboss.com",
			"Deep Link Schema: https://www.walletbyboss.com",
			"SEP-10 Client Domain: www.walletbyboss.com",
			"Assets:",
			"* USDC - GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			"* XLM",
			"Name: Vibrant Assist",
			"Homepage: https://vibrantapp.com/vibrant-assist",
			"Deep Link Schema: https://vibrantapp.com/sdp-dev",
			"SEP-10 Client Domain: vibrantapp.com",
			"Assets:",
			"* USDC - GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
		}

		logs := buf.String()
		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}
	})

	t.Run("futurenet wallets only register native assets", func(t *testing.T) {
		data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		data.CreateAssetFixture(t, ctx, dbConnectionPool, assets.XLMAssetCode, "")

		err := SetupWalletsForProperNetwork(ctx, dbConnectionPool, utils.FuturenetNetworkType, DefaultWalletsNetworkMap)
		require.NoError(t, err)

		wallets, err := models.Wallets.GetAll(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, wallets)

		for _, w := range wallets {
			walletAssets, err := models.Wallets.GetAssets(ctx, w.ID)
			require.NoError(t, err)

			for _, asset := range walletAssets {
				assert.Truef(t, asset.IsNative(), "wallet %s should not include non-native asset %s:%s", w.Name, asset.Code, asset.Issuer)
			}
		}
	})

	// Ensure the BOSS Money bug doesn't happen again on Testnet. Please refer to: https://stellarfoundation.slack.com/archives/C018BLTP2AU/p1686690282162189
	t.Run("duplicated constraint error", func(t *testing.T) {
		// creates the Vibrant Assist and BOSS Money wallets
		data.ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)

		walletNetworkMap := WalletsNetworkMapType{
			utils.TestnetNetworkType: {
				{
					Name:              "Boss Money",
					Homepage:          "https://www.walletbyboss.com",
					DeepLinkSchema:    "https://www.walletbyboss.com",
					SEP10ClientDomain: "www.walletbyboss.com",
				},
				{
					Name:              "Vibrant Assist",
					Homepage:          "https://vibrantapp.com",
					DeepLinkSchema:    "https://vibrantapp.com/sdp-dev",
					SEP10ClientDomain: "api-dev.vibrantapp.com",
				},
			},
		}

		err := SetupWalletsForProperNetwork(ctx, dbConnectionPool, utils.TestnetNetworkType, walletNetworkMap)

		// The problem was that in the DefaultWalletsNetworkMap, in the `testnet` key, we used the name `Boss Money` and not `BOSS Money`
		// to refer to the BOSS Money wallet. So the query tried to insert the `Boss Money` wallet, but since the `homepage` and `deep_link_schema`
		// were the same as the already inserted then, the insert statement resulted in a duplicated constraint error.
		assert.ErrorContains(t, err, `error upserting wallets: pq: duplicate key value violates unique constraint "wallets_homepage_key"`)

		// DefaultNetworkMap test - should NOT error
		data.ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)

		err = SetupWalletsForProperNetwork(ctx, dbConnectionPool, utils.TestnetNetworkType, DefaultWalletsNetworkMap)
		require.NoError(t, err)
	})
}
