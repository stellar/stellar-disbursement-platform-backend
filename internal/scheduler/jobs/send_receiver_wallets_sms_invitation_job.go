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
	SendReceiverWalletsSMSInvitationJobName            = "send_receiver_wallets_sms_invitation_job"
	SendReceiverWalletsSMSInvitationJobIntervalSeconds = 5
)

// SendReceiverWalletsSMSInvitationJob is a job that periodically sends SMS invitations to receiver wallets.
type SendReceiverWalletsSMSInvitationJobOptions struct {
	AnchorPlatformBaseSepURL       string
	Models                         *data.Models
	MessengerClient                message.MessengerClient
	MaxInvitationSMSResendAttempts int64
	Sep10SigningPrivateKey         string
	CrashTrackerClient             crashtracker.CrashTrackerClient
}

var _ Job = (*SendReceiverWalletsSMSInvitationJob)(nil)

type SendReceiverWalletsSMSInvitationJob struct {
	service *services.SendReceiverWalletInviteService
}

func (j SendReceiverWalletsSMSInvitationJob) GetName() string {
	return SendReceiverWalletsSMSInvitationJobName
}

func (j SendReceiverWalletsSMSInvitationJob) GetInterval() time.Duration {
	return time.Second * SendReceiverWalletsSMSInvitationJobIntervalSeconds
}

func (j SendReceiverWalletsSMSInvitationJob) Execute(ctx context.Context) error {
	if err := j.service.SendInvite(ctx); err != nil {
		err = fmt.Errorf("error sending invitation SMS to receiver wallets: %w", err)
		log.Ctx(ctx).Error(err)
		return err
	}
	return nil
}

func NewSendReceiverWalletsSMSInvitationJob(options SendReceiverWalletsSMSInvitationJobOptions) *SendReceiverWalletsSMSInvitationJob {
	s, err := services.NewSendReceiverWalletInviteService(
		options.Models,
		options.MessengerClient,
		options.AnchorPlatformBaseSepURL,
		options.Sep10SigningPrivateKey,
		options.MaxInvitationSMSResendAttempts,
		options.CrashTrackerClient,
	)
	if err != nil {
		log.Fatalf("error instantiating service: %s", err.Error())
	}

	return &SendReceiverWalletsSMSInvitationJob{service: s}
}
