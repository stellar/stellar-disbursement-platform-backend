package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

type PaymentsProcessorJob struct {
	service services.SendPaymentsServiceInterface
}

const (
	PaymentJobName             = "payments_processor_job"
	PaymentsJobIntervalSeconds = 10
	PaymentsBatchSize          = 100
)

func NewPaymentsProcessorJob(models *data.Models) *PaymentsProcessorJob {
	return &PaymentsProcessorJob{service: services.NewSendPaymentsService(models)}
}

func (d PaymentsProcessorJob) GetInterval() time.Duration {
	return PaymentsJobIntervalSeconds * time.Second
}

func (d PaymentsProcessorJob) GetName() string {
	return PaymentJobName
}

func (d PaymentsProcessorJob) Execute(ctx context.Context) error {
	err := d.service.SendBatchPayments(ctx, PaymentsBatchSize)
	if err != nil {
		return fmt.Errorf("error executing PaymentsProcessorJob: %w", err)
	}
	return nil
}

var _ Job = new(PaymentsProcessorJob)
