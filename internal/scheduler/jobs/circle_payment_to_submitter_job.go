package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/paymentdispatchers"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
)

const (
	circlePaymentToSubmitterJobName   = "circle_payment_to_submitter_job"
	circlePaymentToSubmitterBatchSize = 100
)

// circlePaymentToSubmitterJob is a job that periodically sends any ready-to-pay SDP payments to the transaction submission
// service.
type circlePaymentToSubmitterJob struct {
	paymentToSubmitterSvc services.PaymentToSubmitterServiceInterface
	jobIntervalSeconds    int
	distAccountResolver   signing.DistributionAccountResolver
}

type CirclePaymentToSubmitterJobOptions struct {
	JobIntervalSeconds  int
	Models              *data.Models
	DistAccountResolver signing.DistributionAccountResolver
	CircleService       circle.ServiceInterface
}

func NewCirclePaymentToSubmitterJob(opts CirclePaymentToSubmitterJobOptions) Job {
	if opts.JobIntervalSeconds < DefaultMinimumJobIntervalSeconds {
		log.Fatalf("job interval is not set for %s. Instantiation failed", circlePaymentToSubmitterJobName)
	}

	circlePaymentDispatcher := paymentdispatchers.NewCirclePaymentDispatcher(opts.Models, opts.CircleService, opts.DistAccountResolver)

	return &circlePaymentToSubmitterJob{
		paymentToSubmitterSvc: services.NewPaymentToSubmitterService(services.PaymentToSubmitterServiceOptions{
			Models:              opts.Models,
			DistAccountResolver: opts.DistAccountResolver,
			PaymentDispatcher:   circlePaymentDispatcher,
		}),
		jobIntervalSeconds:  opts.JobIntervalSeconds,
		distAccountResolver: opts.DistAccountResolver,
	}
}

func (d circlePaymentToSubmitterJob) IsJobMultiTenant() bool {
	return true
}

func (d circlePaymentToSubmitterJob) GetInterval() time.Duration {
	if d.jobIntervalSeconds == 0 {
		log.Warnf("job interval is not set for %s. Using default interval: %d seconds", d.GetName(), DefaultMinimumJobIntervalSeconds)
		return DefaultMinimumJobIntervalSeconds * time.Second
	}
	return time.Duration(d.jobIntervalSeconds) * time.Second
}

func (d circlePaymentToSubmitterJob) GetName() string {
	return circlePaymentToSubmitterJobName
}

func (d circlePaymentToSubmitterJob) Execute(ctx context.Context) error {
	distAccount, err := d.distAccountResolver.DistributionAccountFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting distribution account: %w", err)
	}

	if !distAccount.Type.IsCircle() {
		return fmt.Errorf("distribution account is not a Stellar account. Skipping for current tenant")
	}

	if payErr := d.paymentToSubmitterSvc.SendBatchPayments(ctx, circlePaymentToSubmitterBatchSize); payErr != nil {
		return fmt.Errorf("executing circlePaymentToSubmitterJob: %w", payErr)
	}
	return nil
}

var _ Job = (*circlePaymentToSubmitterJob)(nil)
