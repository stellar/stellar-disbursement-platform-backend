package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

const (
	passkeySessionCleanupJobName     = "passkey_session_cleanup_job"
	passkeySessionCleanupJobInterval = time.Minute * 30
)

type passkeySessionCleanupJob struct {
	model *data.PasskeySessionModel
}

func NewPasskeySessionCleanupJob(models *data.Models) Job {
	return &passkeySessionCleanupJob{
		model: models.PasskeySessions,
	}
}

func (j passkeySessionCleanupJob) Execute(ctx context.Context) error {
	if err := j.model.DeleteExpired(ctx); err != nil {
		return fmt.Errorf("deleting expired passkey sessions: %w", err)
	}
	return nil
}

func (j passkeySessionCleanupJob) GetInterval() time.Duration {
	return passkeySessionCleanupJobInterval
}

func (j passkeySessionCleanupJob) GetName() string {
	return passkeySessionCleanupJobName
}

func (j passkeySessionCleanupJob) IsJobMultiTenant() bool {
	return true
}

var _ Job = (*passkeySessionCleanupJob)(nil)
