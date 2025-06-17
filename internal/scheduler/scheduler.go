package scheduler

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/scheduler/jobs"
)

// Scheduler manages a list of jobs and executes them at their specified intervals.
// It uses a job queue to distribute jobs to workers.
type Scheduler struct {
	jobs               map[string]jobs.Job
	cancel             context.CancelFunc
	crashTrackerClient crashtracker.CrashTrackerClient
	tenantManager      tenant.ManagerInterface
	jobQueue           chan jobs.Job
	// enqueuedJobs is used to keep track of enqueued jobs to avoid enqueuing the same job multiple times in case it takes longer to execute than its interval.
	enqueuedJobs sync.Map
}

type SchedulerOptions struct {
	PaymentJobIntervalSeconds            int
	ReceiverInvitationJobIntervalSeconds int
}

type SchedulerJobRegisterOption func(*Scheduler)

// SchedulerWorkerCount is the number of workers that will be started to process jobs
const SchedulerWorkerCount = 5

// StartScheduler initializes and starts the scheduler. This method blocks until the scheduler is stopped.
func StartScheduler(adminDBConnectionPool db.DBConnectionPool, crashTrackerClient crashtracker.CrashTrackerClient, schedulerJobRegisters ...SchedulerJobRegisterOption) {
	// Call crash tracker FlushEvents to flush buffered events before the scheduler terminates
	defer crashTrackerClient.FlushEvents(2 * time.Second)
	// Call crash tracker Recover for recover from unhandled panics
	defer crashTrackerClient.Recover()

	ctx, cancel := context.WithCancel(context.Background())

	// create a channel to listen for a shutdown signal
	signalChan := make(chan os.Signal, 1)

	// register signal listeners for graceful shutdown
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	scheduler := newScheduler(cancel)
	// add crashTrackerClient to scheduler object
	scheduler.crashTrackerClient = crashTrackerClient
	scheduler.tenantManager = tenant.NewManager(tenant.WithDatabase(adminDBConnectionPool))

	// Registering jobs
	for _, schedulerJobRegister := range schedulerJobRegisters {
		schedulerJobRegister(scheduler)
	}

	scheduler.start(ctx)

	// wait for the shutdown signal here.
	<-signalChan

	scheduler.stop()
}

// newScheduler creates a new scheduler.
func newScheduler(cancel context.CancelFunc) *Scheduler {
	return &Scheduler{
		jobs:     make(map[string]jobs.Job),
		cancel:   cancel,
		jobQueue: make(chan jobs.Job),
	}
}

// addJob adds a job to the scheduler. This method does not start the job. To start the job, call start().
func (s *Scheduler) addJob(job jobs.Job) {
	log.Infof("registering job to scheduler [name: %s], [interval: %s], [isMultiTenant: %t]",
		job.GetName(), job.GetInterval(), job.IsJobMultiTenant())
	s.jobs[job.GetName()] = job
}

// start starts the scheduler and all jobs. This method blocks until the scheduler is stopped.
func (s *Scheduler) start(ctx context.Context) {
	if len(s.jobs) == 0 {
		log.Ctx(ctx).Info("No jobs to start")
		s.stop()
		return
	}
	log.Ctx(ctx).Infof("Starting scheduler with %d workers...", SchedulerWorkerCount)

	// 1. We start all the workers that will process jobs from the job queue.
	for i := 1; i <= SchedulerWorkerCount; i++ {
		// start a new worker passing a CrashTrackerClient clone to report errors when the job is executed
		go worker(ctx, i, s.crashTrackerClient.Clone(), s.tenantManager, s)
	}

	// 2. Enqueue jobs to jobQueue.
	// We start one goroutine per job but these are lightweight because they only wait for the ticker to tick then enqueue the job.
	for _, job := range s.jobs {
		go func(job jobs.Job) {
			ticker := time.NewTicker(job.GetInterval())
			for {
				select {
				case <-ticker.C:
					jobName := job.GetName()
					if _, alreadyEnqueued := s.enqueuedJobs.LoadOrStore(jobName, true); !alreadyEnqueued {
						log.Ctx(ctx).Debugf("Enqueuing job: %s", jobName)
						s.jobQueue <- job
					} else {
						log.Ctx(ctx).Debugf("Skipping job %s, already in queue", jobName)
					}
				case <-ctx.Done():
					ticker.Stop()
					return
				}
			}
		}(job)
	}
}

// stop uses the context to stop the scheduler and all jobs.
func (s *Scheduler) stop() {
	log.Info("Stopping scheduler...")
	s.cancel()
}

// worker is a goroutine that processes jobs from the job queue.
func worker(ctx context.Context, workerID int, crashTrackerClient crashtracker.CrashTrackerClient, tenantManager tenant.ManagerInterface, scheduler *Scheduler) {
	defer func() {
		if r := recover(); r != nil {
			log.Ctx(ctx).Errorf("Worker %d encountered a panic while processing a job: %v", workerID, r)
		}
	}()
	for {
		select {
		case job := <-scheduler.jobQueue:
			executeJob(ctx, job, workerID, crashTrackerClient, tenantManager)
			scheduler.enqueuedJobs.Delete(job.GetName()) // Remove job from tracking after execution
		case <-ctx.Done():
			log.Ctx(ctx).Infof("Worker %d stopping...", workerID)
			return
		}
	}
}

// executeJob executes a job and reports any errors to the crash tracker.
func executeJob(ctx context.Context, job jobs.Job, workerID int, crashTrackerClient crashtracker.CrashTrackerClient, tenantManager tenant.ManagerInterface) {
	// Handle multi-tenant jobs.
	if job.IsJobMultiTenant() {
		tenants, err := tenantManager.GetAllTenants(ctx, nil)
		if err != nil {
			msg := fmt.Sprintf("error getting all tenants for job %s on worker %d", job.GetName(), workerID)
			crashTrackerClient.LogAndReportErrors(ctx, err, msg)
			return
		}
		for _, t := range tenants {
			log.Ctx(ctx).Debugf("Processing job %s for tenant %s on worker %d", job.GetName(), t.ID, workerID)
			tenantCtx := tenant.SaveTenantInContext(ctx, &t)
			if err = job.Execute(tenantCtx); err != nil {
				msg := fmt.Sprintf("error processing job %s for tenant %s on worker %d", job.GetName(), t.ID, workerID)
				crashTrackerClient.LogAndReportErrors(tenantCtx, err, msg)
			}
		}
	} else {
		log.Ctx(ctx).Debugf("Processing job %s on worker %d", job.GetName(), workerID)
		if err := job.Execute(ctx); err != nil {
			msg := fmt.Sprintf("error processing job %s on worker %d", job.GetName(), workerID)
			crashTrackerClient.LogAndReportErrors(ctx, err, msg)
		}
	}
}

func WithAPAuthEnforcementJob(apService anchorplatform.AnchorPlatformAPIServiceInterface, monitorService monitor.MonitorServiceInterface, crashTrackerClient crashtracker.CrashTrackerClient) SchedulerJobRegisterOption {
	return func(s *Scheduler) {
		j, err := jobs.NewAnchorPlatformAuthMonitoringJob(apService, monitorService, crashTrackerClient)
		if err != nil {
			log.Errorf("error creating %s job: %s", j.GetName(), err)
		}
		s.addJob(j)
	}
}

func WithReadyPaymentsCancellationJobOption(models *data.Models) SchedulerJobRegisterOption {
	return func(s *Scheduler) {
		j := jobs.NewReadyPaymentsCancellationJob(models)
		s.addJob(j)
	}
}

func WithCirclePaymentToSubmitterJobOption(options jobs.CirclePaymentToSubmitterJobOptions) SchedulerJobRegisterOption {
	return func(s *Scheduler) {
		j := jobs.NewCirclePaymentToSubmitterJob(options)
		s.addJob(j)
	}
}

func WithStellarPaymentToSubmitterJobOption(options jobs.StellarPaymentToSubmitterJobOptions) SchedulerJobRegisterOption {
	return func(s *Scheduler) {
		j := jobs.NewStellarPaymentToSubmitterJob(options)
		s.addJob(j)
	}
}

func WithCircleReconciliationJobOption(options jobs.CircleReconciliationJobOptions) SchedulerJobRegisterOption {
	return func(s *Scheduler) {
		j := jobs.NewCircleReconciliationJob(options)
		s.addJob(j)
	}
}

func WithPaymentFromSubmitterJobOption(paymentJobInterval int, models *data.Models, tssDBConnectionPool db.DBConnectionPool) SchedulerJobRegisterOption {
	return func(s *Scheduler) {
		j := jobs.NewPaymentFromSubmitterJob(paymentJobInterval, models, tssDBConnectionPool)
		s.addJob(j)
	}
}

func WithEmbeddedWalletFromSubmitterJobOption(embeddedWalletJobInterval int, models *data.Models, tssDBConnectionPool db.DBConnectionPool, networkPassphrase string) SchedulerJobRegisterOption {
	return func(s *Scheduler) {
		j := jobs.NewEmbeddedWalletFromSubmitterJob(embeddedWalletJobInterval, models, tssDBConnectionPool, networkPassphrase)
		s.addJob(j)
	}
}

func WithSendReceiverWalletsInvitationJobOption(o jobs.SendReceiverWalletsInvitationJobOptions) SchedulerJobRegisterOption {
	return func(s *Scheduler) {
		j := jobs.NewSendReceiverWalletsInvitationJob(o)
		s.addJob(j)
	}
}

func WithPatchAnchorPlatformTransactionsCompletionJobOption(paymentJobInterval int, apAPISvc anchorplatform.AnchorPlatformAPIServiceInterface, models *data.Models) SchedulerJobRegisterOption {
	return func(s *Scheduler) {
		j := jobs.NewPatchAnchorPlatformTransactionsCompletionJob(paymentJobInterval, apAPISvc, models)
		s.addJob(j)
	}
}
