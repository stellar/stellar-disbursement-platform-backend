package services

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/stellar/go/clients/horizonclient"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

type DistributionAccountServiceInterface interface {
	GetBalances(ctx context.Context, account *schema.DistributionAccount) (map[string]float64, error)
	GetBalance(ctx context.Context, account *schema.DistributionAccount, asset data.Asset) (float64, error)
}

type DistributionAccountService struct {
	strategies map[schema.DistributionAccountType]DistributionAccountServiceInterface
}

func NewDistributionAccountService(horizonClient horizonclient.ClientInterface) *DistributionAccountService {
	stellarNativeDistributionAccSvc := NewStellarNativeDistributionAccountService(horizonClient)

	strategies := map[schema.DistributionAccountType]DistributionAccountServiceInterface{
		schema.DistributionAccountTypeEnvStellar:     stellarNativeDistributionAccSvc,
		schema.DistributionAccountTypeDBVaultStellar: stellarNativeDistributionAccSvc,
		// schema.DistributionAccountTypeDBVaultCircle: Add Circle distribution account service
	}

	return &DistributionAccountService{
		strategies: strategies,
	}
}

func (s *DistributionAccountService) GetBalance(ctx context.Context, account *schema.DistributionAccount, asset data.Asset) (float64, error) {
	return s.strategies[account.Type].GetBalance(ctx, account, asset)
}

func (s *DistributionAccountService) GetBalances(ctx context.Context, account *schema.DistributionAccount) (map[string]float64, error) {
	return s.strategies[account.Type].GetBalances(ctx, account)
}

var _ DistributionAccountServiceInterface = new(DistributionAccountService)

type StellarNativeDistributionAccountService struct {
	horizonClient horizonclient.ClientInterface
}

var _ DistributionAccountServiceInterface = new(StellarNativeDistributionAccountService)

func NewStellarNativeDistributionAccountService(horizonClient horizonclient.ClientInterface) *StellarNativeDistributionAccountService {
	return &StellarNativeDistributionAccountService{
		horizonClient: horizonClient,
	}
}

func (s *StellarNativeDistributionAccountService) GetBalances(_ context.Context, account *schema.DistributionAccount) (map[string]float64, error) {
	accountDetails, err := s.horizonClient.AccountDetail(horizonclient.AccountRequest{AccountID: account.Address})
	if err != nil {
		return nil, fmt.Errorf("cannot get details for account from Horizon: %w", err)
	}

	var balances = make(map[string]float64)
	for _, b := range accountDetails.Balances {
		var asset string
		if b.Asset.Type == "native" {
			asset = "XLM:native"
		} else {
			asset = strings.ToUpper(b.Asset.Code) + ":" + strings.ToUpper(b.Asset.Issuer)
		}

		assetBal, parseAssetBalErr := strconv.ParseFloat(b.Balance, 64)
		if parseAssetBalErr != nil {
			return nil, parseAssetBalErr
		}

		if _, ok := balances[asset]; ok {
			return nil, fmt.Errorf("duplicate balance for asset %s found for distribution account", asset)
		}
		balances[asset] = assetBal
	}

	return balances, nil
}

func (s *StellarNativeDistributionAccountService) GetBalance(ctx context.Context, account *schema.DistributionAccount, asset data.Asset) (float64, error) {
	accBalances, err := s.GetBalances(ctx, account)
	if err != nil {
		return 0, fmt.Errorf("getting balances for distribution account: %w", err)
	}

	assetIssuer := strings.ToUpper(asset.Issuer)
	if assetIssuer == "" {
		assetIssuer = "native"
	}

	assetMapID := strings.ToUpper(asset.Code) + ":" + assetIssuer
	if assetBalance, ok := accBalances[assetMapID]; ok {
		return assetBalance, nil
	}

	return 0, fmt.Errorf("balance for asset %s not found for distribution account", assetMapID)
}
