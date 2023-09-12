package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
)

const (
	apAuthMonitoringJobName            = "anchor_platform_auth_monitoring_job"
	apAuthMonitoringJobIntervalSeconds = 300
)

// AnchorPlatformAuthMonitoringJob is a job that periodically monitors the Anchor Platform's to make sure it has the
// authentication enforcement enabled.
type AnchorPlatformAuthMonitoringJob struct {
	apService          anchorplatform.AnchorPlatformAPIServiceInterface
	monitorService     monitor.MonitorServiceInterface
	crashTrackerClient crashtracker.CrashTrackerClient
}

// NewPaymentService creates a new PaymentService
func NewAnchorPlatformAuthMonitoringJob(apService anchorplatform.AnchorPlatformAPIServiceInterface, monitorService monitor.MonitorServiceInterface, crashTrackerClient crashtracker.CrashTrackerClient) (*AnchorPlatformAuthMonitoringJob, error) {
	if apService == nil {
		return nil, fmt.Errorf("apService cannot be nil")
	}
	if monitorService == nil {
		return nil, fmt.Errorf("monitorService cannot be nil")
	}
	if crashTrackerClient == nil {
		return nil, fmt.Errorf("crashTrackerClient cannot be nil")
	}

	return &AnchorPlatformAuthMonitoringJob{
		apService:          apService,
		monitorService:     monitorService,
		crashTrackerClient: crashTrackerClient,
	}, nil
}

func (job AnchorPlatformAuthMonitoringJob) GetInterval() time.Duration {
	return apAuthMonitoringJobIntervalSeconds * time.Second
}

func (job AnchorPlatformAuthMonitoringJob) GetName() string {
	return apAuthMonitoringJobName
}

func (job AnchorPlatformAuthMonitoringJob) Execute(ctx context.Context) error {
	log.Ctx(ctx).Debugf("executing AnchorPlatformAuthMonitoringJob ...")
	isProtected, err := job.apService.IsAnchorProtectedByAuth(ctx)
	if err != nil {
		return fmt.Errorf("executing AnchorPlatformAuthMonitoringJob: %w", err)
	}

	if !isProtected {
		job.crashTrackerClient.LogAndReportMessages(ctx, "Anchor platform is not enforcing authentication")
		err = job.monitorService.MonitorCounters(monitor.AnchorPlatformAuthProtectionMissingCounterTag, nil)
		if err != nil {
			log.Ctx(ctx).Errorf("Error trying to monitor disbursement counter: %s", err)
		}
	} else {
		log.Ctx(ctx).Info("🎉 Anchor platform authentication was ensured successfully")
		err = job.monitorService.MonitorCounters(monitor.AnchorPlatformAuthProtectionEnsuredCounterTag, nil)
		if err != nil {
			log.Ctx(ctx).Errorf("Error trying to monitor disbursement counter: %s", err)
		}
	}

	return nil
}

var _ Job = new(AnchorPlatformAuthMonitoringJob)
