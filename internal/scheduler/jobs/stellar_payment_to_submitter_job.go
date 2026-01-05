package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/paymentdispatchers"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	txSubStore "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

const (
	stellarPaymentToSubmitterJobName   = "stellar_payment_to_submitter_job"
	stellarPaymentToSubmitterBatchSize = 100
)

// stellarPaymentToSubmitterJob is a job that periodically sends any ready-to-pay SDP stellar payments to the transaction submission
// service.
type stellarPaymentToSubmitterJob struct {
	paymentToSubmitterSvc services.PaymentToSubmitterServiceInterface
	jobIntervalSeconds    int
	distAccountResolver   signing.DistributionAccountResolver
}

type StellarPaymentToSubmitterJobOptions struct {
	JobIntervalSeconds  int
	Models              *data.Models
	TSSDBConnectionPool db.DBConnectionPool
	DistAccountResolver signing.DistributionAccountResolver
}

func NewStellarPaymentToSubmitterJob(opts StellarPaymentToSubmitterJobOptions) Job {
	if opts.JobIntervalSeconds < DefaultMinimumJobIntervalSeconds {
		log.Fatalf("job interval is not set for %s. Instantiation failed", stellarPaymentToSubmitterJobName)
	}

	stellarPaymentDispatcher := paymentdispatchers.NewStellarPaymentDispatcher(
		opts.Models,
		txSubStore.NewTransactionModel(opts.TSSDBConnectionPool),
		opts.DistAccountResolver)

	return &stellarPaymentToSubmitterJob{
		paymentToSubmitterSvc: services.NewPaymentToSubmitterService(services.PaymentToSubmitterServiceOptions{
			Models:              opts.Models,
			DistAccountResolver: opts.DistAccountResolver,
			PaymentDispatcher:   stellarPaymentDispatcher,
		}),
		jobIntervalSeconds:  opts.JobIntervalSeconds,
		distAccountResolver: opts.DistAccountResolver,
	}
}

func (d stellarPaymentToSubmitterJob) IsJobMultiTenant() bool {
	return true
}

func (d stellarPaymentToSubmitterJob) GetInterval() time.Duration {
	if d.jobIntervalSeconds == 0 {
		log.Warnf("job interval is not set for %s. Using default interval: %d seconds", d.GetName(), DefaultMinimumJobIntervalSeconds)
		return DefaultMinimumJobIntervalSeconds * time.Second
	}
	return time.Duration(d.jobIntervalSeconds) * time.Second
}

func (d stellarPaymentToSubmitterJob) GetName() string {
	return stellarPaymentToSubmitterJobName
}

func (d stellarPaymentToSubmitterJob) Execute(ctx context.Context) error {
	distAccount, err := d.distAccountResolver.DistributionAccountFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting distribution account: %w", err)
	}

	if !distAccount.Type.IsStellar() {
		log.Ctx(ctx).Debug("distribution account is not a Stellar account. Skipping for current tenant")
		return nil
	}

	if payErr := d.paymentToSubmitterSvc.SendBatchPayments(ctx, stellarPaymentToSubmitterBatchSize); payErr != nil {
		return fmt.Errorf("executing paymentToSubmitterJob: %w", payErr)
	}
	return nil
}

var _ Job = (*stellarPaymentToSubmitterJob)(nil)
