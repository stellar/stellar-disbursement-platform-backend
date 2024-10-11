package services

import (
	"context"
	"fmt"
	"strconv"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

//go:generate mockery --name=DistributionAccountServiceInterface --case=underscore --structname=MockDistributionAccountService --filename=distribution_account_service.go
type DistributionAccountServiceInterface interface {
	GetBalances(context context.Context, account *schema.TransactionAccount) (map[data.Asset]float64, error)
	GetBalance(context context.Context, account *schema.TransactionAccount, asset data.Asset) (float64, error)
}

type DistributionAccountServiceOptions struct {
	HorizonClient horizonclient.ClientInterface
	CircleService circle.ServiceInterface
	NetworkType   utils.NetworkType
}

func (opts DistributionAccountServiceOptions) Validate() error {
	if opts.HorizonClient == nil {
		return fmt.Errorf("Horizon client cannot be nil")
	}

	if opts.CircleService == nil {
		return fmt.Errorf("Circle service cannot be nil")
	}

	err := opts.NetworkType.Validate()
	if err != nil {
		return fmt.Errorf("validating network type: %w", err)
	}

	return nil
}

type DistributionAccountService struct {
	strategies map[schema.AccountType]DistributionAccountServiceInterface
}

func NewDistributionAccountService(opts DistributionAccountServiceOptions) (*DistributionAccountService, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("validating options: %w", err)
	}

	stellarDistributionAccSvc := &StellarDistributionAccountService{
		horizonClient: opts.HorizonClient,
	}

	circleDistributionAccSvc := &CircleDistributionAccountService{
		CircleService: opts.CircleService,
		NetworkType:   opts.NetworkType,
	}

	strategies := map[schema.AccountType]DistributionAccountServiceInterface{
		schema.DistributionAccountStellarEnv:     stellarDistributionAccSvc,
		schema.DistributionAccountStellarDBVault: stellarDistributionAccSvc,
		schema.DistributionAccountCircleDBVault:  circleDistributionAccSvc,
	}
	return &DistributionAccountService{strategies: strategies}, nil
}

func (s *DistributionAccountService) GetBalance(ctx context.Context, account *schema.TransactionAccount, asset data.Asset) (float64, error) {
	return s.strategies[account.Type].GetBalance(ctx, account, asset)
}

func (s *DistributionAccountService) GetBalances(ctx context.Context, account *schema.TransactionAccount) (map[data.Asset]float64, error) {
	return s.strategies[account.Type].GetBalances(ctx, account)
}

var _ DistributionAccountServiceInterface = (*DistributionAccountService)(nil)

type StellarDistributionAccountService struct {
	horizonClient horizonclient.ClientInterface
}

var _ DistributionAccountServiceInterface = (*StellarDistributionAccountService)(nil)

func (s *StellarDistributionAccountService) GetBalances(_ context.Context, account *schema.TransactionAccount) (map[data.Asset]float64, error) {
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

func (s *StellarDistributionAccountService) GetBalance(ctx context.Context, account *schema.TransactionAccount, asset data.Asset) (float64, error) {
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

type CircleDistributionAccountService struct {
	CircleService circle.ServiceInterface
	NetworkType   utils.NetworkType
}

var _ DistributionAccountServiceInterface = (*CircleDistributionAccountService)(nil)

func (s *CircleDistributionAccountService) GetBalances(ctx context.Context, account *schema.TransactionAccount) (map[data.Asset]float64, error) {
	if !account.IsCircle() {
		return nil, fmt.Errorf("distribution account is not a Circle account")
	}
	if account.Status == schema.AccountStatusPendingUserActivation {
		return nil, fmt.Errorf("This organization's distribution account is in %s state, please complete the %s activation process to access this endpoint.", account.Status, account.Type.Platform())
	}

	businessBalances, err := s.CircleService.GetBusinessBalances(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting wallet by ID: %w", err)
	}

	balances := make(map[data.Asset]float64)
	for _, b := range businessBalances.Available {
		asset, err := circle.ParseStellarAsset(b.Currency, s.NetworkType)
		if err != nil {
			log.Ctx(ctx).Debugf("Ignoring balance for asset %s, as it's not supported by the SDP: %v", b.Currency, err)
			continue
		}

		assetBal, err := strconv.ParseFloat(b.Amount, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing balance to float: %w", err)
		}

		balances[asset] = assetBal
	}

	return balances, nil
}

func (s *CircleDistributionAccountService) GetBalance(ctx context.Context, account *schema.TransactionAccount, asset data.Asset) (float64, error) {
	accBalances, err := s.GetBalances(ctx, account)
	if err != nil {
		return 0, fmt.Errorf("getting balances for distribution account: %w", err)
	}

	asset = data.Asset{Code: asset.Code, Issuer: asset.Issuer} // scrub the other fields
	assetBalance, ok := accBalances[asset]
	if !ok {
		return 0, fmt.Errorf("balance for asset %v not found for distribution account", asset)
	}

	return assetBalance, nil
}
