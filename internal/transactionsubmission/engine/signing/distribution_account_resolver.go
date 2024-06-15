package signing

import (
	"context"
	"fmt"

	"github.com/stellar/go/strkey"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

var ErrDistributionAccountIsEmpty = fmt.Errorf("distribution account is empty")

// DistributionAccountResolver is an interface that provides the distribution iven the provided keyword.
//
//go:generate mockery --name=DistributionAccountResolver --case=underscore --structname=MockDistributionAccountResolver
type DistributionAccountResolver interface {
	DistributionAccount(ctx context.Context, tenantID string) (schema.TransactionAccount, error)
	DistributionAccountFromContext(ctx context.Context) (schema.TransactionAccount, error)
	HostDistributionAccount() schema.TransactionAccount
}

type DistributionAccountResolverOptions struct {
	AdminDBConnectionPool            db.DBConnectionPool
	HostDistributionAccountPublicKey string
	MTNDBConnectionPool              db.DBConnectionPool
}

func (c DistributionAccountResolverOptions) Validate() error {
	if c.AdminDBConnectionPool == nil {
		return fmt.Errorf("AdminDBConnectionPool cannot be nil")
	}

	if c.MTNDBConnectionPool == nil {
		return fmt.Errorf("MTNDBConnectionPool cannot be nil")
	}

	if c.HostDistributionAccountPublicKey == "" {
		return fmt.Errorf("HostDistributionAccountPublicKey cannot be empty")
	}
	if !strkey.IsValidEd25519PublicKey(c.HostDistributionAccountPublicKey) {
		return fmt.Errorf("HostDistributionAccountPublicKey is not a valid ed25519 public key")
	}

	return nil
}

func NewDistributionAccountResolver(config DistributionAccountResolverOptions) (DistributionAccountResolver, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validating config in NewDistributionAccountResolver: %w", err)
	}

	return &DistributionAccountResolverImpl{
		tenantManager:                 tenant.NewManager(tenant.WithDatabase(config.AdminDBConnectionPool)),
		hostDistributionAccountPubKey: config.HostDistributionAccountPublicKey,
		circleConfigModel:             circle.NewClientConfigModel(config.MTNDBConnectionPool),
	}, nil
}

var _ DistributionAccountResolver = (*DistributionAccountResolverImpl)(nil)

// DistributionAccountResolverImpl is a DistributionAccountResolver that resolves the distribution account from the database.
type DistributionAccountResolverImpl struct {
	tenantManager                 tenant.ManagerInterface
	hostDistributionAccountPubKey string
	circleConfigModel             circle.ClientConfigModelInterface
}

// DistributionAccount returns the tenant's distribution account stored in the database.
func (r *DistributionAccountResolverImpl) DistributionAccount(ctx context.Context, tenantID string) (schema.TransactionAccount, error) {
	tnt, err := r.tenantManager.GetTenantByID(ctx, tenantID)
	if err != nil {
		return schema.TransactionAccount{}, fmt.Errorf("getting tenant: %w", err)
	}
	tenant.SaveTenantInContext(ctx, tnt)
	return r.getDistributionAccount(ctx, tnt)
}

// DistributionAccountFromContext returns the tenant's distribution account from the tenant object stored in the context
// provided.
func (r *DistributionAccountResolverImpl) DistributionAccountFromContext(ctx context.Context) (schema.TransactionAccount, error) {
	tnt, err := tenant.GetTenantFromContext(ctx)
	if err != nil {
		return schema.TransactionAccount{}, fmt.Errorf("getting tenant: %w", err)
	}
	return r.getDistributionAccount(ctx, tnt)
}

// getDistributionAccount extracts the distribution account from the tenant if it exists.
func (r *DistributionAccountResolverImpl) getDistributionAccount(ctx context.Context, tnt *tenant.Tenant) (schema.TransactionAccount, error) {
	if tnt.DistributionAccountType == schema.DistributionAccountCircleDBVault {
		// 1. Circle Account
		cc, circleErr := r.circleConfigModel.Get(ctx)
		if circleErr != nil {
			return schema.TransactionAccount{}, fmt.Errorf("getting circle client config: %w", circleErr)
		}

		var walletID string
		if cc != nil && cc.WalletID != nil {
			walletID = *cc.WalletID
		}
		return schema.TransactionAccount{
			CircleWalletID: walletID,
			Type:           schema.DistributionAccountCircleDBVault,
			Status:         tnt.DistributionAccountStatus,
		}, nil
	} else {
		// 2. Stellar Account
		if tnt.DistributionAccountAddress == nil {
			return schema.TransactionAccount{}, ErrDistributionAccountIsEmpty
		}

		return schema.TransactionAccount{
			Address: *tnt.DistributionAccountAddress,
			Type:    tnt.DistributionAccountType,
			Status:  tnt.DistributionAccountStatus,
		}, nil

	}
}

// HostDistributionAccount returns the host distribution account from the database.
func (r *DistributionAccountResolverImpl) HostDistributionAccount() schema.TransactionAccount {
	return schema.NewDefaultHostAccount(r.hostDistributionAccountPubKey)
}
