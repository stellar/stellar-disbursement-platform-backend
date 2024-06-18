package services

import (
	"context"
	"fmt"
	"strconv"

	"github.com/stellar/go/clients/horizonclient"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

//go:generate mockery --name=DistributionAccountServiceInterface --case=underscore --structname=MockDistributionAccountService --filename=distribution_account_service.go
type DistributionAccountServiceInterface interface {
	GetBalances(context context.Context, account *schema.TransactionAccount) (map[data.Asset]float64, error)
	GetBalance(context context.Context, account *schema.TransactionAccount, asset data.Asset) (float64, error)
}

type DistributionAccountServiceOptions struct {
	HorizonClient horizonclient.ClientInterface
}

type DistributionAccountService struct {
	strategies map[schema.AccountType]DistributionAccountServiceInterface
}

func NewDistributionAccountService(opts DistributionAccountServiceOptions) *DistributionAccountService {
	stellarNativeDistributionAccSvc := &StellarNativeDistributionAccountService{
		horizonClient: opts.HorizonClient,
	}

	strategies := map[schema.AccountType]DistributionAccountServiceInterface{
		schema.DistributionAccountStellarEnv:     stellarNativeDistributionAccSvc,
		schema.DistributionAccountStellarDBVault: stellarNativeDistributionAccSvc,
		// TODO [SDP-1232]: schema.DistributionAccountCircleDBVault: Add Circle distribution account service
	}

	return &DistributionAccountService{
		strategies: strategies,
	}
}

func (s *DistributionAccountService) GetBalance(ctx context.Context, account *schema.TransactionAccount, asset data.Asset) (float64, error) {
	return s.strategies[account.Type].GetBalance(ctx, account, asset)
}

func (s *DistributionAccountService) GetBalances(ctx context.Context, account *schema.TransactionAccount) (map[data.Asset]float64, error) {
	return s.strategies[account.Type].GetBalances(ctx, account)
}

var _ DistributionAccountServiceInterface = (*DistributionAccountService)(nil)

type StellarNativeDistributionAccountService struct {
	horizonClient horizonclient.ClientInterface
}

var _ DistributionAccountServiceInterface = (*StellarNativeDistributionAccountService)(nil)

func (s *StellarNativeDistributionAccountService) GetBalances(_ context.Context, account *schema.TransactionAccount) (map[data.Asset]float64, error) {
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
			code = b.Asset.Code
			issuer = b.Asset.Issuer
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

func (s *StellarNativeDistributionAccountService) GetBalance(ctx context.Context, account *schema.TransactionAccount, asset data.Asset) (float64, error) {
	accBalances, err := s.GetBalances(ctx, account)
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
