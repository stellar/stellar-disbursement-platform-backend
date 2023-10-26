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
	ReadyPaymentsCancellationJobName     = "ready_payments_cancellation"
	ReadyPaymentsCancellationJobInterval = 5
)

type ReadyPaymentsCancellationJob struct {
	service services.ReadyPaymentsCancellationServiceInterface
}

func (j ReadyPaymentsCancellationJob) GetName() string {
	return ReadyPaymentsCancellationJobName
}

func (j ReadyPaymentsCancellationJob) GetInterval() time.Duration {
	return time.Minute * ReadyPaymentsCancellationJobInterval
}

func (j ReadyPaymentsCancellationJob) Execute(ctx context.Context) error {
	if err := j.service.CancelReadyPayments(ctx); err != nil {
		err = fmt.Errorf("error cancelling ready payments: %w", err)
		log.Ctx(ctx).Error(err)
		return err
	}
	return nil
}

func NewReadyPaymentsCancellationJob(models *data.Models) *ReadyPaymentsCancellationJob {
	s := services.NewReadyPaymentsCancellationService(models)
	return &ReadyPaymentsCancellationJob{
		service: s,
	}
}
