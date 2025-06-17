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
	embeddedWalletFromSubmitterJobName   = "embedded_wallet_from_submitter_job"
	embeddedWalletFromSubmitterBatchSize = 100
)

// embeddedWalletFromSubmitterJob is a job that periodically monitors TSS transactions that were complete and sync their status
// with SDP for embedded wallets.
type embeddedWalletFromSubmitterJob struct {
	service            services.EmbeddedWalletFromSubmitterServiceInterface
	jobIntervalSeconds int
}

func NewEmbeddedWalletFromSubmitterJob(embeddedWalletJobInterval int, models *data.Models, tssDBConnectionPool db.DBConnectionPool, networkPassphrase string) Job {
	if embeddedWalletJobInterval < DefaultMinimumJobIntervalSeconds {
		log.Fatalf("job interval is not set for %s. Instantiation failed", embeddedWalletFromSubmitterJobName)
	}
	return &embeddedWalletFromSubmitterJob{
		service:            services.NewEmbeddedWalletFromSubmitterService(models, tssDBConnectionPool, networkPassphrase),
		jobIntervalSeconds: embeddedWalletJobInterval,
	}
}

func (j embeddedWalletFromSubmitterJob) GetInterval() time.Duration {
	return time.Duration(j.jobIntervalSeconds) * time.Second
}

func (j embeddedWalletFromSubmitterJob) GetName() string {
	return embeddedWalletFromSubmitterJobName
}

func (j embeddedWalletFromSubmitterJob) IsJobMultiTenant() bool {
	return true
}

func (j embeddedWalletFromSubmitterJob) Execute(ctx context.Context) error {
	t, err := tenant.GetTenantFromContext(ctx)
	if err != nil {
		return fmt.Errorf("error getting tenant from context for %s: %w", embeddedWalletFromSubmitterJobName, err)
	}
	if err := j.service.SyncBatchTransactions(ctx, embeddedWalletFromSubmitterBatchSize, t.ID); err != nil {
		return fmt.Errorf("error executing embeddedWalletFromSubmitterJob: %w", err)
	}
	return nil
}

var _ Job = (*embeddedWalletFromSubmitterJob)(nil)
