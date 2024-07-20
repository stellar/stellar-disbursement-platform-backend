package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
)

const (
	paymentToSubmitterJobName   = "payment_to_submitter_job"
	paymentToSubmitterBatchSize = 100
)

// paymentToSubmitterJob is a job that periodically sends any ready-to-pay SDP payments to the transaction submission
// service.
type paymentToSubmitterJob struct {
	paymentToSubmitterSvc services.PaymentToSubmitterServiceInterface
	jobIntervalSeconds    int
}

type PaymentToSubmitterJobOptions struct {
	JobIntervalSeconds  int
	Models              *data.Models
	TSSDBConnectionPool db.DBConnectionPool
	DistAccountResolver signing.DistributionAccountResolver
	CircleService       circle.ServiceInterface
}

func NewPaymentToSubmitterJob(opts PaymentToSubmitterJobOptions) Job {
	if opts.JobIntervalSeconds < DefaultMinimumJobIntervalSeconds {
		log.Fatalf("job interval is not set for %s. Instantiation failed", paymentToSubmitterJobName)
	}
	return &paymentToSubmitterJob{
		paymentToSubmitterSvc: services.NewPaymentToSubmitterService(services.PaymentToSubmitterServiceOptions{
			Models:              opts.Models,
			TSSDBConnectionPool: opts.TSSDBConnectionPool,
			DistAccountResolver: opts.DistAccountResolver,
			CircleService:       opts.CircleService,
		}),
		jobIntervalSeconds: opts.JobIntervalSeconds,
	}
}

func (d paymentToSubmitterJob) IsJobMultiTenant() bool {
	return true
}

func (d paymentToSubmitterJob) GetInterval() time.Duration {
	if d.jobIntervalSeconds == 0 {
		log.Warnf("job interval is not set for %s. Using default interval: %d seconds", d.GetName(), DefaultMinimumJobIntervalSeconds)
		return DefaultMinimumJobIntervalSeconds * time.Second
	}
	return time.Duration(d.jobIntervalSeconds) * time.Second
}

func (d paymentToSubmitterJob) GetName() string {
	return paymentToSubmitterJobName
}

func (d paymentToSubmitterJob) Execute(ctx context.Context) error {
	err := d.paymentToSubmitterSvc.SendBatchPayments(ctx, paymentToSubmitterBatchSize)
	if err != nil {
		return fmt.Errorf("error executing paymentToSubmitterJob: %w", err)
	}
	return nil
}

var _ Job = (*paymentToSubmitterJob)(nil)
