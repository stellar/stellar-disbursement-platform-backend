package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

const (
	sepNonceCleanupJobName     = "sep_nonce_cleanup_job"
	sepNonceCleanupJobInterval = time.Minute * 30
)

type sepNonceCleanupJob struct {
	model *data.SEPNonceModel
}

func NewSEPNonceCleanupJob(models *data.Models) Job {
	return &sepNonceCleanupJob{
		model: models.SEPNonces,
	}
}

func (j sepNonceCleanupJob) Execute(ctx context.Context) error {
	if err := j.model.DeleteExpired(ctx); err != nil {
		return fmt.Errorf("deleting expired sep nonces: %w", err)
	}
	return nil
}

func (j sepNonceCleanupJob) GetInterval() time.Duration {
	return sepNonceCleanupJobInterval
}

func (j sepNonceCleanupJob) GetName() string {
	return sepNonceCleanupJobName
}

func (j sepNonceCleanupJob) IsJobMultiTenant() bool {
	return true
}

var _ Job = (*sepNonceCleanupJob)(nil)
