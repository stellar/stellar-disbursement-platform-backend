package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type WalletInfo struct {
	Name, Homepage, DeepLinkSchema, SEP10ClientDomain string
}

type WalletsNetworkMapType map[utils.NetworkType][]WalletInfo

var DefaultWalletsNetworkMap = WalletsNetworkMapType{
	utils.PubnetNetworkType: {
		{
			Name:              "Vibrant Assist",
			Homepage:          "https://vibrantapp.com/assist",
			DeepLinkSchema:    "https://vibrantapp.com/sdp",
			SEP10ClientDomain: "api.vibrantapp.com",
		},
		// {
		// 	Name:              "Beans App",
		// 	Homepage:          "https://www.beansapp.com/disbursements",
		// 	DeepLinkSchema:    "https://www.beansapp.com/disbursements/registration?redirect=true",
		// 	SEP10ClientDomain: "api.beansapp.com",
		// },
	},
	utils.TestnetNetworkType: {
		{
			Name:              "Demo Wallet",
			Homepage:          "https://demo-wallet.stellar.org",
			DeepLinkSchema:    "https://demo-wallet.stellar.org",
			SEP10ClientDomain: "demo-wallet-server.stellar.org",
		},
	},
}

// SetupWalletsForProperNetwork updates and inserts wallets for the given Network Passphrase (`network`). So it avoids the application having
// wallets that doesn't support the given network.
func SetupWalletsForProperNetwork(ctx context.Context, dbConnectionPool db.DBConnectionPool, network utils.NetworkType, walletsNetworkMap WalletsNetworkMapType) error {
	log.Ctx(ctx).Infof("updating/inserting wallets for the '%s' network\n\n", network)

	wallets, ok := walletsNetworkMap[network]
	if !ok {
		return fmt.Errorf("invalid network provided")
	}

	var names, homepages, deepLinkSchemas, sep10ClientDomains []string

	separator := strings.Repeat("-", 20)
	buf := new(strings.Builder)
	buf.WriteString("wallets that will be updated or inserted:\n\n")
	for _, wallet := range wallets {
		names = append(names, wallet.Name)
		homepages = append(homepages, wallet.Homepage)
		deepLinkSchemas = append(deepLinkSchemas, wallet.DeepLinkSchema)
		sep10ClientDomains = append(sep10ClientDomains, wallet.SEP10ClientDomain)

		buf.WriteString(fmt.Sprintf("%s\n%s\n\n", wallet.Name, separator))
	}

	log.Ctx(ctx).Info(buf.String())

	err := db.RunInTransaction(ctx, dbConnectionPool, nil, func(dbTx db.DBTransaction) error {
		query := `
			WITH wallets_to_update_or_insert AS (
				-- gather all wallets passed as parameters for the query and turn into SQL rows
				SELECT
					UNNEST($1::text[]) AS name, UNNEST($2::text[]) AS homepage,
					UNNEST($3::text[]) AS deep_link_schema, UNNEST($4::text[]) AS sep_10_client_domain
			),
			existing_wallets AS (
				-- gets all wallets that the name appears in the names passed as parameter for the query
				SELECT
					*
				FROM
					wallets
				WHERE
					name = ANY($1::text[])
				FOR UPDATE
			),
			update_existing_wallets AS (
				-- updates the existing wallets resulted in 'existing_wallets' CTE
				UPDATE
					wallets w
				SET
					homepage = wtui.homepage,
					deep_link_schema = wtui.deep_link_schema,
					sep_10_client_domain = wtui.sep_10_client_domain
				FROM
					existing_wallets ew
					INNER JOIN wallets_to_update_or_insert wtui ON ew.name = wtui.name
				WHERE
					w.id = ew.id
			)
			-- inserts wallets in the database
			INSERT INTO wallets 
				(name, homepage, deep_link_schema, sep_10_client_domain)
			SELECT
				wtui.name, wtui.homepage, wtui.deep_link_schema, wtui.sep_10_client_domain
			FROM
				wallets_to_update_or_insert wtui
			WHERE
				wtui.name NOT IN (SELECT name FROM existing_wallets)
		`

		_, err := dbTx.ExecContext(ctx, query, pq.Array(names), pq.Array(homepages), pq.Array(deepLinkSchemas), pq.Array(sep10ClientDomains))
		if err != nil {
			return fmt.Errorf("error upserting wallets: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("error upserting wallets for the proper network: %w", err)
	}

	models, err := data.NewModels(dbConnectionPool)
	if err != nil {
		return fmt.Errorf("error getting models: %w", err)
	}

	allWallets, err := models.Wallets.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("error getting all available wallets on database: %w", err)
	}

	buf.Reset()
	buf.WriteString(fmt.Sprintf("Registered wallets for network %s:\n\n", network))
	for _, wallet := range allWallets {
		buf.WriteString(fmt.Sprintf("Name: %s\nHomepage: %s\nDeep Link Schema: %s\nSEP-10 Client Domain: %s\n%s\n\n", wallet.Name, wallet.Homepage, wallet.DeepLinkSchema, wallet.SEP10ClientDomain, separator))
	}

	log.Ctx(ctx).Info(buf.String())

	return nil
}
