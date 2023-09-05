package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

const (
	PaymentJobName             = "payment_to_submitter_job"
	PaymentsJobIntervalSeconds = 10
	PaymentsBatchSize          = 100
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
	return PaymentsJobIntervalSeconds * time.Second
}

func (d PaymentToSubmitterJob) GetName() string {
	return PaymentJobName
}

func (d PaymentToSubmitterJob) Execute(ctx context.Context) error {
	err := d.service.SendBatchPayments(ctx, PaymentsBatchSize)
	if err != nil {
		return fmt.Errorf("error executing PaymentToSubmitterJob: %w", err)
	}
	return nil
}
