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

func NewPaymentFromSubmitterJob(models *data.Models, tssDBConnectionPool db.DBConnectionPool) *PaymentFromSubmitterJob {
	return &PaymentFromSubmitterJob{service: services.NewPaymentFromSubmitterService(models, tssDBConnectionPool)}
}

func (d PaymentFromSubmitterJob) GetInterval() time.Duration {
	return PaymentFromSubmitterJobIntervalSeconds * time.Second
}

func (d PaymentFromSubmitterJob) GetName() string {
	return PaymentFromSubmitterJobName
}

func (d PaymentFromSubmitterJob) IsJobMultiTenant() bool {
	return true
}

func (d PaymentFromSubmitterJob) Execute(ctx context.Context) error {
	err := d.service.SyncBatchTransactions(ctx, PaymentFromSubmitterBatchSize)
	if err != nil {
		return fmt.Errorf("error executing PaymentFromSubmitterJob: %w", err)
	}
	return nil
}
