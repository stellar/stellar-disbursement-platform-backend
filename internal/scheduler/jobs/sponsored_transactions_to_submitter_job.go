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
	sponsoredTransactionsToSubmitterJobName   = "sponsored_transactions_to_submitter_job"
	sponsoredTransactionsToSubmitterBatchSize = 100
)

type sponsoredTransactionsToSubmitterJob struct {
	service             services.SponsoredTransactionsToSubmitterServiceInterface
	jobIntervalSeconds  int
	distAccountResolver signing.DistributionAccountResolver
}

type SponsoredTransactionsToSubmitterJobOptions struct {
	JobIntervalSeconds  int
	Models              *data.Models
	TSSDBConnectionPool db.DBConnectionPool
	DistAccountResolver signing.DistributionAccountResolver
}

func NewSponsoredTransactionsToSubmitterJob(opts SponsoredTransactionsToSubmitterJobOptions) Job {
	if opts.JobIntervalSeconds < DefaultMinimumJobIntervalSeconds {
		log.Fatalf("job interval for %s is set below the minimum %d. Instantiation failed", sponsoredTransactionsToSubmitterJobName, DefaultMinimumJobIntervalSeconds)
	}

	service, err := services.NewSponsoredTransactionsToSubmitterService(services.SponsoredTransactionsToSubmitterServiceOptions{
		Models:              opts.Models,
		TSSDBConnectionPool: opts.TSSDBConnectionPool,
	})
	if err != nil {
		log.Fatalf("creating sponsored transactions to submitter service: %v", err)
	}

	return &sponsoredTransactionsToSubmitterJob{
		service:             service,
		jobIntervalSeconds:  opts.JobIntervalSeconds,
		distAccountResolver: opts.DistAccountResolver,
	}
}

func (j sponsoredTransactionsToSubmitterJob) GetInterval() time.Duration {
	return time.Duration(j.jobIntervalSeconds) * time.Second
}

func (j sponsoredTransactionsToSubmitterJob) GetName() string {
	return sponsoredTransactionsToSubmitterJobName
}

func (j sponsoredTransactionsToSubmitterJob) IsJobMultiTenant() bool {
	return true
}

func (j sponsoredTransactionsToSubmitterJob) Execute(ctx context.Context) error {
	distAccount, err := j.distAccountResolver.DistributionAccountFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting distribution account: %w", err)
	}

	if !distAccount.Type.IsStellar() {
		log.Ctx(ctx).Debug("distribution account is not a Stellar account. Skipping sponsored transaction submission for current tenant")
		return nil
	}

	if err := j.service.SendBatchSponsoredTransactions(ctx, sponsoredTransactionsToSubmitterBatchSize); err != nil {
		return fmt.Errorf("executing sponsoredTransactionsToSubmitterJob: %w", err)
	}

	return nil
}

var _ Job = (*sponsoredTransactionsToSubmitterJob)(nil)
