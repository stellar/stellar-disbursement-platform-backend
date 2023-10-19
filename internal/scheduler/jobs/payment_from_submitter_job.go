package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

const (
	PaymentFromSubmitterJobName            = "payment_from_submitter_job"
	PaymentFromSubmitterJobIntervalSeconds = 10
	PaymentFromSubmitterBatchSize          = 100
)

// PaymentFromSubmitterJob is a job that periodically monitors TSS transactions that were complete and sync their status
// with SDP.
type PaymentFromSubmitterJob struct {
	service *services.PaymentFromSubmitterService
}

var _ Job = (*PaymentFromSubmitterJob)(nil)

func NewPaymentFromSubmitterJob(models *data.Models) *PaymentFromSubmitterJob {
	return &PaymentFromSubmitterJob{service: services.NewPaymentFromSubmitterService(models)}
}

func (d PaymentFromSubmitterJob) GetInterval() time.Duration {
	return PaymentFromSubmitterJobIntervalSeconds * time.Second
}

func (d PaymentFromSubmitterJob) GetName() string {
	return PaymentFromSubmitterJobName
}

func (d PaymentFromSubmitterJob) Execute(ctx context.Context) error {
	err := d.service.MonitorTransactions(ctx, PaymentFromSubmitterBatchSize)
	if err != nil {
		return fmt.Errorf("error executing PaymentFromSubmitterJob: %w", err)
	}
	return nil
}
