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
	PatchAnchorPlatformTransactionsJobName            = "patch_anchor_platform_transactions"
	PatchAnchorPlatformTransactionsJobIntervalSeconds = 10
)

type PatchAnchorPlatformTransactionsJob struct {
	service *services.PatchAnchorPlatformTransactionService
}

func NewPatchAnchorPlatformTransactionsJob(apAPISvc anchorplatform.AnchorPlatformAPIServiceInterface, models *data.Models) *PatchAnchorPlatformTransactionsJob {
	svc, err := services.NewPatchAnchorPlatformTransactionService(apAPISvc, models)
	if err != nil {
		log.Fatalf("error instantiating anchor platform service: %v", err)
	}

	return &PatchAnchorPlatformTransactionsJob{service: svc}
}

func (j PatchAnchorPlatformTransactionsJob) GetName() string {
	return PatchAnchorPlatformTransactionsJobName
}

func (j PatchAnchorPlatformTransactionsJob) GetInterval() time.Duration {
	return time.Second * PatchAnchorPlatformTransactionsJobIntervalSeconds
}

func (j PatchAnchorPlatformTransactionsJob) Execute(ctx context.Context) error {
	if err := j.service.PatchTransactions(ctx); err != nil {
		err = fmt.Errorf("patching anchor platform transactions: %w", err)
		log.Ctx(ctx).Error(err)
		return err
	}
	return nil
}
