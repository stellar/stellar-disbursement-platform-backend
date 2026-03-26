package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

const (
	sponsoredTransactionFromSubmitterJobName   = "sponsored_transaction_from_submitter_job"
	sponsoredTransactionFromSubmitterBatchSize = 100
)

// sponsoredTransactionFromSubmitterJob is a job that periodically monitors TSS sponsored transactions that were complete and sync their status
// with SDP sponsored_transactions table.
type sponsoredTransactionFromSubmitterJob struct {
	service            services.SponsoredTransactionFromSubmitterServiceInterface
	jobIntervalSeconds int
}

func NewSponsoredTransactionFromSubmitterJob(sponsoredTransactionJobInterval int, models *data.Models, tssDBConnectionPool db.DBConnectionPool) Job {
	if sponsoredTransactionJobInterval < DefaultMinimumJobIntervalSeconds {
		log.Fatalf("job interval for %s is set below the minimum %d. Instantiation failed", sponsoredTransactionFromSubmitterJobName, DefaultMinimumJobIntervalSeconds)
	}
	return &sponsoredTransactionFromSubmitterJob{
		service:            services.NewSponsoredTransactionFromSubmitterService(models, tssDBConnectionPool),
		jobIntervalSeconds: sponsoredTransactionJobInterval,
	}
}

func (j sponsoredTransactionFromSubmitterJob) GetInterval() time.Duration {
	return time.Duration(j.jobIntervalSeconds) * time.Second
}

func (j sponsoredTransactionFromSubmitterJob) GetName() string {
	return sponsoredTransactionFromSubmitterJobName
}

func (j sponsoredTransactionFromSubmitterJob) IsJobMultiTenant() bool {
	return true
}

func (j sponsoredTransactionFromSubmitterJob) Execute(ctx context.Context) error {
	t, err := sdpcontext.GetTenantFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting tenant from context for %s: %w", sponsoredTransactionFromSubmitterJobName, err)
	}
	if err := j.service.SyncBatchTransactions(ctx, sponsoredTransactionFromSubmitterBatchSize, t.ID); err != nil {
		return fmt.Errorf("executing sponsoredTransactionFromSubmitterJob: %w", err)
	}
	return nil
}

var _ Job = (*sponsoredTransactionFromSubmitterJob)(nil)
