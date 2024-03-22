package jobs

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"

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
	paymentToSubmitterSvc              services.PaymentToSubmitterServiceInterface
	patchAnchorPlatformTransactionsSvc services.PatchAnchorPlatformTransactionCompletionServiceInterface
	jobIntervalSeconds                 int
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
	err := d.paymentToSubmitterSvc.SendBatchPayments(ctx, PaymentToSubmitterBatchSize)
	if err != nil {
		return fmt.Errorf("error executing PaymentToSubmitterJob: %w", err)
	}
	return nil
}

func NewPaymentToSubmitterJob(jobIntervalSeconds int, models *data.Models, tssDBConnectionPool db.DBConnectionPool, apAPISvc anchorplatform.AnchorPlatformAPIServiceInterface) *PaymentToSubmitterJob {
	apAPIService, err := services.NewPatchAnchorPlatformTransactionCompletionService(apAPISvc, models)
	if err != nil {
		log.Fatalf("instantiating anchor platform service: %v", err)
	}
	return &PaymentToSubmitterJob{
		paymentToSubmitterSvc:              services.NewPaymentToSubmitterService(models, tssDBConnectionPool),
		patchAnchorPlatformTransactionsSvc: apAPIService,
		jobIntervalSeconds:                 jobIntervalSeconds,
	}
}

var _ Job = (*PaymentToSubmitterJob)(nil)
