package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

type AssetsNetworkMapType map[utils.NetworkType][]data.Asset

var StellarAssetsNetworkMap = AssetsNetworkMapType{
	utils.PubnetNetworkType:  []data.Asset{assets.EURCAssetPubnet, assets.USDCAssetPubnet, assets.XLMAsset},
	utils.TestnetNetworkType: []data.Asset{assets.EURCAssetTestnet, assets.USDCAssetTestnet, assets.XLMAsset},
}

var CircleAssetsNetworkMap = AssetsNetworkMapType{
	utils.PubnetNetworkType:  []data.Asset{assets.EURCAssetPubnet, assets.USDCAssetPubnet},
	utils.TestnetNetworkType: []data.Asset{assets.EURCAssetTestnet, assets.USDCAssetTestnet},
}

type AssetsNetworkByPlatformMapType map[schema.Platform]AssetsNetworkMapType

var AssetsNetworkByPlatformMap = AssetsNetworkByPlatformMapType{
	schema.StellarPlatform: StellarAssetsNetworkMap,
	schema.CirclePlatform:  CircleAssetsNetworkMap,
}

// SetupAssetsForProperNetwork updates and inserts assets for the given Network Passphrase (`network`). So it avoids the application having
// same asset code with multiple issuers.
func SetupAssetsForProperNetwork(ctx context.Context, dbConnectionPool db.DBConnectionPool, network utils.NetworkType, distAccPlatform schema.Platform) error {
	log.Ctx(ctx).Infof("updating/inserting assets for the '%s' network", network)

	assets, ok := AssetsNetworkByPlatformMap[distAccPlatform][network]
	if !ok {
		return fmt.Errorf("invalid network provided")
	}

	var codes, issuers []string

	for _, asset := range assets {
		codes = append(codes, asset.Code)
		issuers = append(issuers, asset.Issuer)
	}

	log.Ctx(ctx).Infof("Asset codes to be updated/inserted: %v", codes)
	err := db.RunInTransaction(ctx, dbConnectionPool, nil, func(dbTx db.DBTransaction) error {
		query := `
			WITH assets_to_update_or_insert AS (
				-- gather all assets passed as parameters for the query and turn into SQL rows
				SELECT UNNEST($1::text[]) AS code, UNNEST($2::text[]) AS issuer
			),
			existing_assets AS (
				-- gets all assets that the code appears in the codes passed as parameter for the query
				SELECT
					*
				FROM
					assets
				WHERE
					code = ANY($1::text[])
				FOR UPDATE
			),
			update_existing_assets AS (
				-- updates the existing assets resulted in 'existing_assets' CTE
				UPDATE
					assets a
				SET
					issuer = atui.issuer
				FROM
					existing_assets ea
					INNER JOIN assets_to_update_or_insert atui ON ea.code = atui.code
				WHERE
					a.id = ea.id AND a.issuer != atui.issuer
			)
			-- inserts assets in the database
			INSERT INTO assets 
				(code, issuer)
			SELECT
				atui.code, atui.issuer
			FROM
				assets_to_update_or_insert atui
			WHERE
				atui.code NOT IN (SELECT code FROM existing_assets)
		`

		_, err := dbTx.ExecContext(ctx, query, pq.Array(codes), pq.Array(issuers))
		if err != nil {
			return fmt.Errorf("error upserting assets: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("error upserting assets for the proper network: %w", err)
	}

	models, err := data.NewModels(dbConnectionPool)
	if err != nil {
		return fmt.Errorf("error getting models: %w", err)
	}

	allAssets, err := models.Assets.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("error getting all available assets on database: %w", err)
	}

	buf := new(strings.Builder)
	buf.WriteString(fmt.Sprintf("Updated list of assets for network %s:\n\n", network))
	for _, asset := range allAssets {
		buf.WriteString(fmt.Sprintf("\t * %s - %s\n", asset.Code, asset.Issuer))
	}
	log.Ctx(ctx).Info(buf.String())

	return nil
}
