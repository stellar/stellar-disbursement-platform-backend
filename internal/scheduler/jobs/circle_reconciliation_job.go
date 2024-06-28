package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
)

const (
	circleReconciliationJobName            = "circle_reconciliation_job"
	circleReconciliationJobIntervalSeconds = 30
)

type CircleReconciliationJobOptions struct {
	Models              *data.Models
	DistAccountResolver signing.DistributionAccountResolver
	CircleService       circle.ServiceInterface
}

func NewCircleReconciliationJob(opts CircleReconciliationJobOptions) Job {
	return &circleReconciliationJob{
		jobIntervalSeconds: circleReconciliationJobIntervalSeconds,
		reconciliationService: &services.CircleReconciliationService{
			Models:              opts.Models,
			DistAccountResolver: opts.DistAccountResolver,
			CircleService:       opts.CircleService,
		},
	}
}

type circleReconciliationJob struct {
	jobIntervalSeconds    int
	reconciliationService services.CircleReconciliationServiceInterface
}

func (j circleReconciliationJob) IsJobMultiTenant() bool {
	return true
}

func (j circleReconciliationJob) GetInterval() time.Duration {
	jobIntervalSeconds := j.jobIntervalSeconds
	if j.jobIntervalSeconds == 0 {
		log.Warnf("job interval is not set for %s. Using default interval: %d seconds", j.GetName(), DefaultMinimumJobIntervalSeconds)
		jobIntervalSeconds = DefaultMinimumJobIntervalSeconds
	}
	return time.Duration(jobIntervalSeconds) * time.Second
}

func (j circleReconciliationJob) GetName() string {
	return circleReconciliationJobName
}

func (j circleReconciliationJob) Execute(ctx context.Context) error {
	err := j.reconciliationService.Reconcile(ctx)
	if err != nil {
		return fmt.Errorf("executing Job %s: %w", j.GetName(), err)
	}
	return nil
}

var _ Job = (*circleReconciliationJob)(nil)
