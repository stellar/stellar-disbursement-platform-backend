package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

const (
	sendReceiverWalletsInvitationJobName = "send_receiver_wallets_invitation_job"
)

type SendReceiverWalletsInvitationJobOptions struct {
	Models                      *data.Models
	MessageDispatcher           message.MessageDispatcherInterface
	MaxInvitationResendAttempts int64
	Sep10SigningPrivateKey      string
	CrashTrackerClient          crashtracker.CrashTrackerClient
	JobIntervalSeconds          int
}

// sendReceiverWalletsInvitationJob is a job that periodically sends invitations to receiver wallets.
type sendReceiverWalletsInvitationJob struct {
	service            *services.SendReceiverWalletInviteService
	jobIntervalSeconds int
}

func (j sendReceiverWalletsInvitationJob) GetName() string {
	return sendReceiverWalletsInvitationJobName
}

func (j sendReceiverWalletsInvitationJob) GetInterval() time.Duration {
	return time.Duration(j.jobIntervalSeconds) * time.Second
}

func (j sendReceiverWalletsInvitationJob) IsJobMultiTenant() bool {
	return true
}

func (j sendReceiverWalletsInvitationJob) Execute(ctx context.Context) error {
	if err := j.service.SendInvite(ctx); err != nil {
		err = fmt.Errorf("error sending invitation to receiver wallets: %w", err)
		log.Ctx(ctx).Error(err)
		return err
	}
	return nil
}

func NewSendReceiverWalletsInvitationJob(options SendReceiverWalletsInvitationJobOptions) Job {
	if options.JobIntervalSeconds < DefaultMinimumJobIntervalSeconds {
		log.Fatalf("job interval is not set for %s. Instantiation failed", sendReceiverWalletsInvitationJobName)
	}
	s, err := services.NewSendReceiverWalletInviteService(
		options.Models,
		options.MessageDispatcher,
		options.Sep10SigningPrivateKey,
		options.MaxInvitationResendAttempts,
		options.CrashTrackerClient,
	)
	if err != nil {
		log.Fatalf("error instantiating service: %s", err.Error())
	}

	return &sendReceiverWalletsInvitationJob{
		service:            s,
		jobIntervalSeconds: options.JobIntervalSeconds,
	}
}

var _ Job = (*sendReceiverWalletsInvitationJob)(nil)
