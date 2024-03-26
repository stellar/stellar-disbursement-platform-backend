package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
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

func NewPaymentToSubmitterJob(jobIntervalSeconds int, models *data.Models, tssDBConnectionPool db.DBConnectionPool) Job {
	if jobIntervalSeconds < DefaultMinimumJobIntervalSeconds {
		log.Fatalf("job interval is not set for %s. Instantiation failed", paymentToSubmitterJobName)
	}
	return &paymentToSubmitterJob{
		paymentToSubmitterSvc: services.NewPaymentToSubmitterService(models, tssDBConnectionPool),
		jobIntervalSeconds:    jobIntervalSeconds,
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
