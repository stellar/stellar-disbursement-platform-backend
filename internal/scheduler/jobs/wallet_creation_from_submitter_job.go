package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

const (
	walletCreationFromSubmitterJobName   = "wallet_creation_from_submitter_job"
	walletCreationFromSubmitterBatchSize = 100
)

// walletCreationFromSubmitterJob is a job that periodically monitors TSS wallet creation transactions that were complete and sync their status
// with SDP for embedded wallets.
type walletCreationFromSubmitterJob struct {
	service            services.WalletCreationFromSubmitterServiceInterface
	jobIntervalSeconds int
}

func NewWalletCreationFromSubmitterJob(walletCreationJobInterval int, models *data.Models, tssDBConnectionPool db.DBConnectionPool, networkPassphrase string) Job {
	if walletCreationJobInterval < DefaultMinimumJobIntervalSeconds {
		log.Fatalf("job interval for %s is set below the minimum %d. Instantiation failed", walletCreationFromSubmitterJobName, DefaultMinimumJobIntervalSeconds)
	}
	return &walletCreationFromSubmitterJob{
		service:            services.NewWalletCreationFromSubmitterService(models, tssDBConnectionPool, networkPassphrase),
		jobIntervalSeconds: walletCreationJobInterval,
	}
}

func (j walletCreationFromSubmitterJob) GetInterval() time.Duration {
	return time.Duration(j.jobIntervalSeconds) * time.Second
}

func (j walletCreationFromSubmitterJob) GetName() string {
	return walletCreationFromSubmitterJobName
}

func (j walletCreationFromSubmitterJob) IsJobMultiTenant() bool {
	return true
}

func (j walletCreationFromSubmitterJob) Execute(ctx context.Context) error {
	t, err := tenant.GetTenantFromContext(ctx)
	if err != nil {
		return fmt.Errorf("error getting tenant from context for %s: %w", walletCreationFromSubmitterJobName, err)
	}
	if err := j.service.SyncBatchTransactions(ctx, walletCreationFromSubmitterBatchSize, t.ID); err != nil {
		return fmt.Errorf("error executing walletCreationFromSubmitterJob: %w", err)
	}
	return nil
}

var _ Job = (*walletCreationFromSubmitterJob)(nil)
