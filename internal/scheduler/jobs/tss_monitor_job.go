package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

type TSSMonitorJob struct {
	service *services.TSSMonitorService
}

const (
	TSSMonitorJobName            = "tss_monitor_job"
	TSSMonitorJobIntervalSeconds = 60
	TSSMonitorBatchSize          = 100
)

func NewTSSMonitorJob(models *data.Models) *TSSMonitorJob {
	return &TSSMonitorJob{service: services.NewTSSMonitorService(models)}
}

func (d TSSMonitorJob) GetInterval() time.Duration {
	return TSSMonitorJobIntervalSeconds * time.Second
}

func (d TSSMonitorJob) GetName() string {
	return TSSMonitorJobName
}

func (d TSSMonitorJob) Execute(ctx context.Context) error {
	log.Ctx(ctx).Infof("executing TSSMonitorJob ...")
	err := d.service.MonitorTransactions(ctx, TSSMonitorBatchSize)
	if err != nil {
		return fmt.Errorf("error executing TSSMonitorJob: %w", err)
	}
	return nil
}

var _ Job = new(PaymentsProcessorJob)
