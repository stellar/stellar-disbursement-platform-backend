package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

const (
	sendReceiverWalletsSMSInvitationJobName = "send_receiver_wallets_sms_invitation_job"
)

type SendReceiverWalletsSMSInvitationJobOptions struct {
	Models                         *data.Models
	MessengerClient                message.MessengerClient
	MaxInvitationSMSResendAttempts int64
	Sep10SigningPrivateKey         string
	CrashTrackerClient             crashtracker.CrashTrackerClient
	JobIntervalSeconds             int
}

// sendReceiverWalletsSMSInvitationJob is a job that periodically sends SMS invitations to receiver wallets.
type sendReceiverWalletsSMSInvitationJob struct {
	service            *services.SendReceiverWalletInviteService
	jobIntervalSeconds int
}

func (j sendReceiverWalletsSMSInvitationJob) GetName() string {
	return sendReceiverWalletsSMSInvitationJobName
}

func (j sendReceiverWalletsSMSInvitationJob) GetInterval() time.Duration {
	return time.Duration(j.jobIntervalSeconds) * time.Second
}

func (j sendReceiverWalletsSMSInvitationJob) IsJobMultiTenant() bool {
	return true
}

func (j sendReceiverWalletsSMSInvitationJob) Execute(ctx context.Context) error {
	if err := j.service.SendInvite(ctx); err != nil {
		err = fmt.Errorf("error sending invitation SMS to receiver wallets: %w", err)
		log.Ctx(ctx).Error(err)
		return err
	}
	return nil
}

func NewSendReceiverWalletsSMSInvitationJob(options SendReceiverWalletsSMSInvitationJobOptions) Job {
	if options.JobIntervalSeconds < DefaultMinimumJobIntervalSeconds {
		log.Fatalf("job interval is not set for %s. Instantiation failed", sendReceiverWalletsSMSInvitationJobName)
	}
	s, err := services.NewSendReceiverWalletInviteService(
		options.Models,
		options.MessengerClient,
		options.Sep10SigningPrivateKey,
		options.MaxInvitationSMSResendAttempts,
		options.CrashTrackerClient,
	)
	if err != nil {
		log.Fatalf("error instantiating service: %s", err.Error())
	}

	return &sendReceiverWalletsSMSInvitationJob{
		service:            s,
		jobIntervalSeconds: options.JobIntervalSeconds,
	}
}

var _ Job = (*sendReceiverWalletsSMSInvitationJob)(nil)
