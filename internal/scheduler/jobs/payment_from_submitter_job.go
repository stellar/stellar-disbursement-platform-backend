package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

const (
	paymentFromSubmitterJobName   = "payment_from_submitter_job"
	paymentFromSubmitterBatchSize = 100
)

// paymentFromSubmitterJob is a job that periodically monitors TSS transactions that were complete and sync their status
// with SDP.
type paymentFromSubmitterJob struct {
	service            services.PaymentFromSubmitterServiceInterface
	jobIntervalSeconds int
}

func NewPaymentFromSubmitterJob(paymentJobInterval int, models *data.Models, tssDBConnectionPool db.DBConnectionPool) Job {
	if paymentJobInterval < DefaultMinimumJobIntervalSeconds {
		log.Fatalf("job interval is not set for %s. Instantiation failed", paymentFromSubmitterJobName)
	}
	return &paymentFromSubmitterJob{
		service:            services.NewPaymentFromSubmitterService(models, tssDBConnectionPool),
		jobIntervalSeconds: paymentJobInterval,
	}
}

func (d paymentFromSubmitterJob) GetInterval() time.Duration {
	return time.Duration(d.jobIntervalSeconds) * time.Second
}

func (d paymentFromSubmitterJob) GetName() string {
	return paymentFromSubmitterJobName
}

func (d paymentFromSubmitterJob) IsJobMultiTenant() bool {
	return true
}

func (d paymentFromSubmitterJob) Execute(ctx context.Context) error {
	t, err := tenant.GetTenantFromContext(ctx)
	if err != nil {
		return fmt.Errorf("error getting tenant from context for %s: %w", paymentFromSubmitterJobName, err)
	}
	if !t.DistributionAccountType.IsStellar() {
		log.Ctx(ctx).Debugf("Skipping job %s for tenant %s as it uses a %s Distribution account", d.GetName(), t.ID, t.DistributionAccountType.Platform())
		return nil
	}
	err = d.service.SyncBatchTransactions(ctx, paymentFromSubmitterBatchSize, t.ID)
	if err != nil {
		return fmt.Errorf("error executing paymentFromSubmitterJob: %w", err)
	}
	return nil
}

var _ Job = (*paymentFromSubmitterJob)(nil)
