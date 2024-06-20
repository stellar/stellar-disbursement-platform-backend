package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

const (
	patchAnchorPlatformTransactionsCompletionJobName = "patch_anchor_platform_transactions_completion"
)

type patchAnchorPlatformTransactionsCompletionJob struct {
	service            services.PatchAnchorPlatformTransactionCompletionServiceInterface
	jobIntervalSeconds int
}

func NewPatchAnchorPlatformTransactionsCompletionJob(paymentJobInterval int, apAPISvc anchorplatform.AnchorPlatformAPIServiceInterface, models *data.Models) Job {
	if paymentJobInterval < DefaultMinimumJobIntervalSeconds {
		log.Fatalf("job interval is not set for %s. Instantiation failed", patchAnchorPlatformTransactionsCompletionJobName)
	}
	svc, err := services.NewPatchAnchorPlatformTransactionCompletionService(apAPISvc, models)
	if err != nil {
		log.Fatalf("instantiating anchor platform service: %v", err)
	}

	return &patchAnchorPlatformTransactionsCompletionJob{
		service:            svc,
		jobIntervalSeconds: paymentJobInterval,
	}
}

func (j patchAnchorPlatformTransactionsCompletionJob) GetName() string {
	return patchAnchorPlatformTransactionsCompletionJobName
}

func (j patchAnchorPlatformTransactionsCompletionJob) GetInterval() time.Duration {
	return time.Duration(j.jobIntervalSeconds) * time.Second
}

func (j patchAnchorPlatformTransactionsCompletionJob) Execute(ctx context.Context) error {
	t, tenantErr := tenant.GetTenantFromContext(ctx)
	if tenantErr != nil {
		return fmt.Errorf("getting tenant from context: %w", tenantErr)
	}
	if t.DistributionAccountType.IsCircle() {
		log.Ctx(ctx).Debugf("Skipping job %s for tenant %s as it uses a Circle Distribution account", j.GetName(), t.ID)
		return nil
	}

	if err := j.service.PatchAPTransactionsForPayments(ctx); err != nil {
		err = fmt.Errorf("patching anchor platform transactions completion: %w", err)
		log.Ctx(ctx).Error(err)
		return err
	}
	return nil
}

func (j patchAnchorPlatformTransactionsCompletionJob) IsJobMultiTenant() bool {
	return true
}

var _ Job = (*patchAnchorPlatformTransactionsCompletionJob)(nil)
