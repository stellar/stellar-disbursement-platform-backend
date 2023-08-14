package services

import (
	"context"
	"strings"
	"testing"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_SetupWalletsForProperNetwork(t *testing.T) {
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

		err = SetupWalletsForProperNetwork(ctx, dbConnectionPool, "invalid", DefaultWalletsNetworkMap)
		assert.EqualError(t, err, "invalid network provided")
	})

	t.Run("inserts new wallets when it doesn't exist", func(t *testing.T) {
		data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

		buf := new(strings.Builder)
		log.DefaultLogger.SetLevel(log.InfoLevel)
		log.DefaultLogger.SetOutput(buf)

		err = SetupWalletsForProperNetwork(ctx, dbConnectionPool, utils.PubnetNetworkType, DefaultWalletsNetworkMap)
		require.NoError(t, err)

		wallets, err := models.Wallets.GetAll(ctx)
		require.NoError(t, err)

		assert.Len(t, wallets, 1)
		// assert.Equal(t, "Beans App", wallets[0].Name)
		assert.Equal(t, "Vibrant Assist", wallets[0].Name)

		expectedLogs := []string{
			"updating/inserting wallets for the 'pubnet' network",
			// "Name: Beans App",
			// "Homepage: https://www.beansapp.com/disbursements",
			// "Deep Link Schema: https://www.beansapp.com/disbursements/registration?redirect=true",
			// "SEP-10 Client Domain: api.beansapp.com",
			"Name: Vibrant Assist",
			"Homepage: https://vibrantapp.com/assist",
			"Deep Link Schema: https://vibrantapp.com/sdp",
			"SEP-10 Client Domain: api.vibrantapp.com",
		}

		logs := buf.String()
		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}
	})

	t.Run("updates and inserts wallets", func(t *testing.T) {
		data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

		data.CreateWalletFixture(t, ctx, dbConnectionPool, "Vibrant Assist", "https://vibrantapp.com", "api-dev.vibrantapp.com", "https://vibrantapp.com/sdp-dev")

		wallets, err := models.Wallets.GetAll(ctx)
		require.NoError(t, err)

		assert.Len(t, wallets, 1)
		assert.Equal(t, "Vibrant Assist", wallets[0].Name)
		assert.Equal(t, "https://vibrantapp.com", wallets[0].Homepage)
		assert.Equal(t, "api-dev.vibrantapp.com", wallets[0].SEP10ClientDomain)
		assert.Equal(t, "https://vibrantapp.com/sdp-dev", wallets[0].DeepLinkSchema)

		walletsNetworkMap := WalletsNetworkMapType{
			utils.PubnetNetworkType: {
				{
					Name:              "Vibrant Assist",
					Homepage:          "https://vibrantapp.com/vibrant-assist",
					DeepLinkSchema:    "https://aidpubnet.netlify.app",
					SEP10ClientDomain: "api.vibrantapp.com",
				},
				{
					Name:              "BOSS Money",
					Homepage:          "https://www.walletbyboss.com",
					DeepLinkSchema:    "https://www.walletbyboss.com",
					SEP10ClientDomain: "www.walletbyboss.com",
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
		assert.Equal(t, "BOSS Money", wallets[0].Name)
		assert.Equal(t, "https://www.walletbyboss.com", wallets[0].Homepage)
		assert.Equal(t, "www.walletbyboss.com", wallets[0].SEP10ClientDomain)
		assert.Equal(t, "https://www.walletbyboss.com", wallets[0].DeepLinkSchema)

		assert.Equal(t, "Vibrant Assist", wallets[1].Name)
		assert.Equal(t, "https://vibrantapp.com/vibrant-assist", wallets[1].Homepage)
		assert.Equal(t, "api.vibrantapp.com", wallets[1].SEP10ClientDomain)
		assert.Equal(t, "https://aidpubnet.netlify.app", wallets[1].DeepLinkSchema)

		expectedLogs := []string{
			"updating/inserting wallets for the 'pubnet' network",
			"Name: BOSS Money",
			"Homepage: https://www.walletbyboss.com",
			"Deep Link Schema: https://www.walletbyboss.com",
			"SEP-10 Client Domain: www.walletbyboss.com",
			"Name: Vibrant Assist",
			"Homepage: https://vibrantapp.com/vibrant-assist",
			"Deep Link Schema: https://aidpubnet.netlify.app",
			"SEP-10 Client Domain: api.vibrantapp.com",
		}

		logs := buf.String()
		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
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
		assert.EqualError(t, err, `error upserting wallets for the proper network: running atomic function in RunInTransactionWithResult: error upserting wallets: pq: duplicate key value violates unique constraint "wallets_homepage_key"`)

		// DefaultNetworkMap test - should NOT error
		data.ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)

		err = SetupWalletsForProperNetwork(ctx, dbConnectionPool, utils.TestnetNetworkType, DefaultWalletsNetworkMap)
		require.NoError(t, err)
	})
}
