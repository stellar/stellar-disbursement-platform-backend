package services

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/stellar/go/clients/horizonclient"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
)

type DistributionAccountServiceInterface interface {
	GetBalances(ctx context.Context) (map[string]float64, error)
	GetBalance(ctx context.Context, asset data.Asset) (float64, error)
}

type StellarNativeDistributionAccountService struct {
	distributionAccountResolver signing.DistributionAccountResolver
	horizonClient               horizonclient.ClientInterface
}

var _ DistributionAccountServiceInterface = new(StellarNativeDistributionAccountService)

// need factory method that takes into account the distribution account type and returns the appropriate service implementation

func NewStellarNativeDistributionAccountService(distributionAccountResolver signing.DistributionAccountResolver) *StellarNativeDistributionAccountService {
	return &StellarNativeDistributionAccountService{
		distributionAccountResolver: distributionAccountResolver,
	}
}

func (s *StellarNativeDistributionAccountService) GetBalances(ctx context.Context) (map[string]float64, error) {
	distributionAccount, err := s.distributionAccountResolver.DistributionAccountFromContext(ctx)
	if err != nil {
		return nil, err
	}

	account, err := s.horizonClient.AccountDetail(horizonclient.AccountRequest{AccountID: distributionAccount.Address})
	if err != nil {
		return nil, err
	}

	var balances = make(map[string]float64)
	for _, b := range account.Balances {
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

func (s *StellarNativeDistributionAccountService) GetBalance(ctx context.Context, asset data.Asset) (float64, error) {
	accBalances, err := s.GetBalances(ctx)
	if err != nil {
		return 0, fmt.Errorf("getting balances for distribution account: %w", err)
	}

	assetMapID := strings.ToUpper(asset.Code) + ":" + strings.ToUpper(asset.Issuer)
	if assetBalance, ok := accBalances[assetMapID]; ok {
		return assetBalance, nil
	}

	return 0, fmt.Errorf("balance for asset %s not found for distribution account", asset)
}
