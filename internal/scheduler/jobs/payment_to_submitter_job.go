package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

const (
	PaymentToSubmitterJobName            = "payment_to_submitter_job"
	PaymentToSubmitterJobIntervalSeconds = 10
	PaymentToSubmitterBatchSize          = 100
)

// PaymentToSubmitterJob is a job that periodically sends any ready-to-pay SDP payments to the transaction submission
// service.
type PaymentToSubmitterJob struct {
	service services.PaymentToSubmitterServiceInterface
}

var _ Job = (*PaymentToSubmitterJob)(nil)

func NewPaymentToSubmitterJob(models *data.Models) *PaymentToSubmitterJob {
	return &PaymentToSubmitterJob{service: services.NewPaymentToSubmitterService(models)}
}

func (d PaymentToSubmitterJob) GetInterval() time.Duration {
	return PaymentToSubmitterJobIntervalSeconds * time.Second
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
