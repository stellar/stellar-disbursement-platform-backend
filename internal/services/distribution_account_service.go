package services

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/stellar/go/clients/horizonclient"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

//go:generate mockery --name=DistributionAccountServiceInterface --case=underscore --structname=MockDistributionAccountService --filename=distribution_account_service.go
type DistributionAccountServiceInterface interface {
	GetBalances(account *schema.DistributionAccount) (map[data.Asset]float64, error)
	GetBalance(account *schema.DistributionAccount, asset data.Asset) (float64, error)
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

func (s *DistributionAccountService) GetBalance(account *schema.DistributionAccount, asset data.Asset) (float64, error) {
	return s.strategies[account.Type].GetBalance(account, asset)
}

func (s *DistributionAccountService) GetBalances(account *schema.DistributionAccount) (map[data.Asset]float64, error) {
	return s.strategies[account.Type].GetBalances(account)
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

func (s *StellarNativeDistributionAccountService) GetBalances(account *schema.DistributionAccount) (map[data.Asset]float64, error) {
	accountDetails, err := s.horizonClient.AccountDetail(horizonclient.AccountRequest{AccountID: account.Address})
	if err != nil {
		return nil, fmt.Errorf("getting details for account from Horizon: %w", err)
	}

	balances := make(map[data.Asset]float64)
	for _, b := range accountDetails.Balances {
		var code, issuer string
		if b.Asset.Type == "native" {
			code = assets.XLMAssetCode
		} else {
			code = strings.ToUpper(b.Asset.Code)
			issuer = strings.ToUpper(b.Asset.Issuer)
		}

		assetBal, parseAssetBalErr := strconv.ParseFloat(b.Balance, 64)
		if parseAssetBalErr != nil {
			return nil, fmt.Errorf("parsing balance to float: %w", parseAssetBalErr)
		}

		balances[data.Asset{
			Code:   code,
			Issuer: issuer,
		}] = assetBal
	}

	return balances, nil
}

func (s *StellarNativeDistributionAccountService) GetBalance(account *schema.DistributionAccount, asset data.Asset) (float64, error) {
	accBalances, err := s.GetBalances(account)
	if err != nil {
		return 0, fmt.Errorf("getting balances for distribution account: %w", err)
	}

	code := asset.Code
	var issuer string
	if !asset.IsNative() {
		issuer = asset.Issuer
	}

	if assetBalance, ok := accBalances[data.Asset{
		Code:   code,
		Issuer: issuer,
	}]; ok {
		return assetBalance, nil
	}

	return 0, fmt.Errorf("balance for asset %s not found for distribution account", asset)
}
