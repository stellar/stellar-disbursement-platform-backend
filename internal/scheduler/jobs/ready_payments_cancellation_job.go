package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

const (
	readyPaymentsCancellationJobName     = "ready_payments_cancellation"
	readyPaymentsCancellationJobInterval = 5
)

type readyPaymentsCancellationJob struct {
	service services.ReadyPaymentsCancellationServiceInterface
}

func (j readyPaymentsCancellationJob) GetName() string {
	return readyPaymentsCancellationJobName
}

func (j readyPaymentsCancellationJob) GetInterval() time.Duration {
	return time.Minute * readyPaymentsCancellationJobInterval
}

func (j readyPaymentsCancellationJob) Execute(ctx context.Context) error {
	if err := j.service.CancelReadyPayments(ctx); err != nil {
		err = fmt.Errorf("error cancelling ready payments: %w", err)
		log.Ctx(ctx).Error(err)
		return err
	}
	return nil
}

func (j readyPaymentsCancellationJob) IsJobMultiTenant() bool {
	return true
}

func NewReadyPaymentsCancellationJob(models *data.Models) Job {
	s := services.NewReadyPaymentsCancellationService(models)
	return &readyPaymentsCancellationJob{
		service: s,
	}
}

var _ Job = new(readyPaymentsCancellationJob)
