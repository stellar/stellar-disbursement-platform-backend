package scheduler

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	jobQueue           chan jobs.Job
	crashTrackerClient crashtracker.CrashTrackerClient
}

type SchedulerOptions struct {
	MaxInvitationSMSResendAttempts int
}

type SchedulerJobRegisterOption func(*Scheduler)

// SchedulerWorkerCount is the number of workers that will be started to process jobs
const SchedulerWorkerCount = 5

// StartScheduler initializes and starts the scheduler. This method blocks until the scheduler is stopped.
func StartScheduler(crashTrackerClient crashtracker.CrashTrackerClient, schedulerJobRegisters ...SchedulerJobRegisterOption) {
	// Call crash tracker FlushEvents to flush buffered events before the scheduler terminates
	defer crashTrackerClient.FlushEvents(2 * time.Second)
	// Call crash tracker Recover for recover from unhandled panics
	defer crashTrackerClient.Recover()

	ctx, cancel := context.WithCancel(context.Background())

	// create a channel to listen for a shutdown signal
	signalChan := make(chan os.Signal, 1)

	// register signal listeners for graceful shutdown
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	scheduler := newScheduler(cancel)
	// add crashTrackerClient to scheduler object
	scheduler.crashTrackerClient = crashTrackerClient

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
		go worker(ctx, i, s.crashTrackerClient.Clone(), s.jobQueue)
	}

	// 2. Enqueue jobs to jobQueue.
	// We start one goroutine per job but these are lightweight because they only wait for the ticker to tick then enqueue the job.
	for _, job := range s.jobs {
		go func(job jobs.Job) {
			ticker := time.NewTicker(job.GetInterval())
			for {
				select {
				case <-ticker.C:
					log.Debugf("Enqueuing job: %s", job.GetName())
					s.jobQueue <- job
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
func worker(ctx context.Context, workerID int, crashTrackerClient crashtracker.CrashTrackerClient, jobQueue <-chan jobs.Job) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Worker %d encountered a panic while processing a job: %v", workerID, r)
		}
	}()
	for {
		select {
		case job := <-jobQueue:
			log.Debugf("Worker %d processing job: %s", workerID, job.GetName())
			if err := job.Execute(ctx); err != nil {
				msg := fmt.Sprintf("error processing job %s on worker %d", job.GetName(), workerID)
				// call crash tracker client to log and report error
				crashTrackerClient.LogAndReportErrors(ctx, err, msg)
			}
		case <-ctx.Done():
			log.Infof("Worker %d stopping...", workerID)
			return
		}
	}
}

func WithPaymentToSubmitterJobOption(models *data.Models) SchedulerJobRegisterOption {
	return func(s *Scheduler) {
		j := jobs.NewPaymentToSubmitterJob(models)
		log.Infof("registering %s job to scheduler", j.GetName())
		s.addJob(j)
	}
}

func WithAPAuthEnforcementJob(apService anchorplatform.AnchorPlatformAPIServiceInterface, monitorService monitor.MonitorServiceInterface, crashTrackerClient crashtracker.CrashTrackerClient) SchedulerJobRegisterOption {
	return func(s *Scheduler) {
		j, err := jobs.NewAnchorPlatformAuthMonitoringJob(apService, monitorService, crashTrackerClient)
		if err != nil {
			log.Errorf("error creating %s job: %s", j.GetName(), err)
		}
		log.Infof("registering %s job to scheduler", j.GetName())
		s.addJob(j)
	}
}

func WithPaymentFromSubmitterJobOption(models *data.Models) SchedulerJobRegisterOption {
	return func(s *Scheduler) {
		j := jobs.NewPaymentFromSubmitterJob(models)
		log.Infof("registering %s job to scheduler", j.GetName())
		s.addJob(j)
	}
}

func WithSendReceiverWalletsSMSInvitationJobOption(o jobs.SendReceiverWalletsSMSInvitationJobOptions) SchedulerJobRegisterOption {
	return func(s *Scheduler) {
		j := jobs.NewSendReceiverWalletsSMSInvitationJob(o)
		log.Infof("registering %s job to scheduler", j.GetName())
		s.addJob(j)
	}
}

func WithPatchAnchorPlatformTransactionsCompletionJobOption(apAPISvc anchorplatform.AnchorPlatformAPIServiceInterface, models *data.Models) SchedulerJobRegisterOption {
	return func(s *Scheduler) {
		j := jobs.NewPatchAnchorPlatformTransactionsCompletionJob(apAPISvc, models)
		log.Infof("registering %s job to scheduler", j.GetName())
		s.addJob(j)
	}
}

func WithReadyPaymentsCancellationJobOption(models *data.Models) SchedulerJobRegisterOption {
	return func(s *Scheduler) {
		j := jobs.NewReadyPaymentsCancellationJob(models)
		log.Infof("registering %s job to scheduler", j.GetName())
		s.addJob(j)
	}
}
