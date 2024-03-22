package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

const (
	PaymentToSubmitterJobName   = "payment_to_submitter_job"
	PaymentToSubmitterBatchSize = 100
)

// PaymentToSubmitterJob is a job that periodically sends any ready-to-pay SDP payments to the transaction submission
// service.
type PaymentToSubmitterJob struct {
	service            services.PaymentToSubmitterServiceInterface
	jobIntervalSeconds int
}

func (d PaymentToSubmitterJob) IsJobMultiTenant() bool {
	return true
}

func (d PaymentToSubmitterJob) GetInterval() time.Duration {
	return time.Duration(d.jobIntervalSeconds) * time.Second
}

func (d PaymentToSubmitterJob) GetName() string {
	return PaymentToSubmitterJobName
}

func (d PaymentToSubmitterJob) Execute(ctx context.Context) error {
	err := d.service.SendBatchPayments(ctx, PaymentToSubmitterBatchSize)
	if err != nil {
		return fmt.Errorf("error executing PaymentToSubmitterJob: %w", err)
	}
	return nil
}

func NewPaymentToSubmitterJob(jobIntervalSeconds int, models *data.Models, tssDBConnectionPool db.DBConnectionPool) *PaymentToSubmitterJob {
	return &PaymentToSubmitterJob{
		service:            services.NewPaymentToSubmitterService(models, tssDBConnectionPool),
		jobIntervalSeconds: jobIntervalSeconds,
	}
}

var _ Job = (*PaymentToSubmitterJob)(nil)
