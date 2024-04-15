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

// anchorPlatformAuthMonitoringJob is a job that periodically monitors the Anchor Platform's to make sure it has the
// authentication enforcement enabled.
type anchorPlatformAuthMonitoringJob struct {
	apService          anchorplatform.AnchorPlatformAPIServiceInterface
	monitorService     monitor.MonitorServiceInterface
	crashTrackerClient crashtracker.CrashTrackerClient
}

// NewAnchorPlatformAuthMonitoringJob is a factory method that creates a new instance of anchorPlatformAuthMonitoringJob.
func NewAnchorPlatformAuthMonitoringJob(apService anchorplatform.AnchorPlatformAPIServiceInterface, monitorService monitor.MonitorServiceInterface, crashTrackerClient crashtracker.CrashTrackerClient) (Job, error) {
	if apService == nil {
		return nil, fmt.Errorf("apService cannot be nil")
	}
	if monitorService == nil {
		return nil, fmt.Errorf("monitorService cannot be nil")
	}
	if crashTrackerClient == nil {
		return nil, fmt.Errorf("crashTrackerClient cannot be nil")
	}

	return &anchorPlatformAuthMonitoringJob{
		apService:          apService,
		monitorService:     monitorService,
		crashTrackerClient: crashTrackerClient,
	}, nil
}

func (job anchorPlatformAuthMonitoringJob) GetInterval() time.Duration {
	return apAuthMonitoringJobIntervalSeconds * time.Second
}

func (job anchorPlatformAuthMonitoringJob) GetName() string {
	return apAuthMonitoringJobName
}

func (job anchorPlatformAuthMonitoringJob) Execute(ctx context.Context) error {
	log.Ctx(ctx).Debugf("executing anchorPlatformAuthMonitoringJob ...")
	isProtected, err := job.apService.IsAnchorProtectedByAuth(ctx)
	if err != nil {
		return fmt.Errorf("executing anchorPlatformAuthMonitoringJob: %w", err)
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

func (job anchorPlatformAuthMonitoringJob) IsJobMultiTenant() bool {
	return false
}

var _ Job = new(anchorPlatformAuthMonitoringJob)
