package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
)

const (
	walletCreationToSubmitterJobName   = "wallet_creation_to_submitter_job"
	walletCreationToSubmitterBatchSize = 100
)

type walletCreationToSubmitterJob struct {
	service             services.WalletCreationToSubmitterServiceInterface
	jobIntervalSeconds  int
	distAccountResolver signing.DistributionAccountResolver
}

type WalletCreationToSubmitterJobOptions struct {
	JobIntervalSeconds  int
	Models              *data.Models
	TSSDBConnectionPool db.DBConnectionPool
	DistAccountResolver signing.DistributionAccountResolver
}

func NewWalletCreationToSubmitterJob(opts WalletCreationToSubmitterJobOptions) Job {
	if opts.JobIntervalSeconds < DefaultMinimumJobIntervalSeconds {
		log.Fatalf("job interval for %s is set below the minimum %d. Instantiation failed", walletCreationToSubmitterJobName, DefaultMinimumJobIntervalSeconds)
	}

	service, err := services.NewWalletCreationToSubmitterService(services.WalletCreationToSubmitterServiceOptions{
		Models:              opts.Models,
		TSSDBConnectionPool: opts.TSSDBConnectionPool,
	})
	if err != nil {
		log.Fatalf("creating embedded wallet to submitter service: %v", err)
	}

	return &walletCreationToSubmitterJob{
		service:             service,
		jobIntervalSeconds:  opts.JobIntervalSeconds,
		distAccountResolver: opts.DistAccountResolver,
	}
}

func (j walletCreationToSubmitterJob) GetInterval() time.Duration {
	return time.Duration(j.jobIntervalSeconds) * time.Second
}

func (j walletCreationToSubmitterJob) GetName() string {
	return walletCreationToSubmitterJobName
}

func (j walletCreationToSubmitterJob) IsJobMultiTenant() bool {
	return true
}

func (j walletCreationToSubmitterJob) Execute(ctx context.Context) error {
	distAccount, err := j.distAccountResolver.DistributionAccountFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting distribution account: %w", err)
	}

	if !distAccount.Type.IsStellar() {
		log.Ctx(ctx).Debug("distribution account is not a Stellar account. Skipping wallet creation submission for current tenant")
		return nil
	}

	if err := j.service.SendBatchWalletCreations(ctx, walletCreationToSubmitterBatchSize); err != nil {
		return fmt.Errorf("executing walletCreationToSubmitterJob: %w", err)
	}

	return nil
}

var _ Job = (*walletCreationToSubmitterJob)(nil)
