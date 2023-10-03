package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

const (
	patchAnchorPlatformTransactionsCompletionJobName            = "patch_anchor_platform_transactions_completion"
	patchAnchorPlatformTransactionsCompletionJobIntervalSeconds = 10
)

type PatchAnchorPlatformTransactionsCompletionJob struct {
	service *services.PatchAnchorPlatformTransactionCompletionService
}

func NewPatchAnchorPlatformTransactionsCompletionJob(apAPISvc anchorplatform.AnchorPlatformAPIServiceInterface, models *data.Models) *PatchAnchorPlatformTransactionsCompletionJob {
	svc, err := services.NewPatchAnchorPlatformTransactionCompletionService(apAPISvc, models)
	if err != nil {
		log.Fatalf("instantiating anchor platform service: %v", err)
	}

	return &PatchAnchorPlatformTransactionsCompletionJob{service: svc}
}

func (j PatchAnchorPlatformTransactionsCompletionJob) GetName() string {
	return patchAnchorPlatformTransactionsCompletionJobName
}

func (j PatchAnchorPlatformTransactionsCompletionJob) GetInterval() time.Duration {
	return time.Second * patchAnchorPlatformTransactionsCompletionJobIntervalSeconds
}

func (j PatchAnchorPlatformTransactionsCompletionJob) Execute(ctx context.Context) error {
	if err := j.service.PatchTransactionsCompletion(ctx); err != nil {
		err = fmt.Errorf("patching anchor platform transactions completion: %w", err)
		log.Ctx(ctx).Error(err)
		return err
	}
	return nil
}
