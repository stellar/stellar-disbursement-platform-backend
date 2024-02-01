package services

import (
	"context"
	"fmt"
	"html/template"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"golang.org/x/exp/slices"
)

type SendReceiverWalletInviteService struct {
	messengerClient                message.MessengerClient
	models                         *data.Models
	anchorPlatformBaseSepURL       string
	maxInvitationSMSResendAttempts int64
	sep10SigningPrivateKey         string
	crashTrackerClient             crashtracker.CrashTrackerClient
}

func (s SendReceiverWalletInviteService) validate() error {
	if s.messengerClient == nil {
		return fmt.Errorf("messenger client can't be nil")
	}

	if s.anchorPlatformBaseSepURL == "" {
		return fmt.Errorf("anchorPlatformBaseSepURL can't be empty")
	}

	return nil
}

// SendInvite sends the invitation’s deep link to the wallet’s application.
// The approach to sending the invitation is to send the deep link for each asset the wallet will pay based on the payment.
// For instance, the Wallet Foo is in two Ready Payments, one with USDC and the other with EUROC.
// So the receiver who has a Stellar Address pending registration (status:READY) in this wallet will receive both invites for USDC and EUROC.
// This would not impact the user receiving both token amounts. It's only for the registration process.
func (s SendReceiverWalletInviteService) SendInvite(ctx context.Context) error {
	// Get the organization entry to get the Org name and SMSRegistrationMessageTemplate
	organization, err := s.models.Organizations.Get(ctx)
	if err != nil {
		return fmt.Errorf("error getting organization: %w", err)
	}

	// Debug purposes
	if organization.SMSResendInterval == nil {
		log.Ctx(ctx).Debug("automatic resend invitation SMS is deactivated. Set a valid value to the organization's sms_resend_interval to activate it.")
	}

	orgSMSRegistrationMessageTemplate := organization.SMSRegistrationMessageTemplate
	if !strings.Contains(orgSMSRegistrationMessageTemplate, "{{.RegistrationLink}}") {
		orgSMSRegistrationMessageTemplate = fmt.Sprintf("%s {{.RegistrationLink}}", strings.TrimSpace(orgSMSRegistrationMessageTemplate))
	}

	// Execute the template early so we avoid hitting the database to query the other info
	msgTemplate, err := template.New("").Parse(orgSMSRegistrationMessageTemplate)
	if err != nil {
		return fmt.Errorf("error parsing organization SMS registration message template: %w", err)
	}

	wallets, err := s.models.Wallets.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("error getting all wallets: %w", err)
	}

	walletsMap := make(map[string]data.Wallet, len(wallets))
	for _, wallet := range wallets {
		walletsMap[wallet.ID] = wallet
	}

	receiverWallets, err := s.models.ReceiverWallet.GetAllPendingRegistration(ctx)
	if err != nil {
		return fmt.Errorf("error getting receiver wallets pending registration: %w", err)
	}

	receiverWalletsAsset, err := s.models.Assets.GetAssetsPerReceiverWallet(ctx, receiverWallets...)
	if err != nil {
		return fmt.Errorf("error getting all assets: %w", err)
	}

	msgsToInsert := []*data.MessageInsert{}
	receiverWalletIDs := []string{}
	// TODO: improve this code adding go routines
	for _, rwa := range receiverWalletsAsset {
		if !s.shouldSendInvitationSMS(ctx, organization, &rwa) {
			continue
		}

		wallet := walletsMap[rwa.WalletID]

		wdl := WalletDeepLink{
			DeepLink:                 wallet.DeepLinkSchema,
			AnchorPlatformBaseSepURL: s.anchorPlatformBaseSepURL,
			OrganizationName:         organization.Name,
			AssetCode:                rwa.Asset.Code,
			AssetIssuer:              rwa.Asset.Issuer,
		}

		registrationLink, err := wdl.GetSignedRegistrationLink(s.sep10SigningPrivateKey)
		if err != nil {
			log.Ctx(ctx).Errorf(
				"error getting signed registration link to receiver wallet ID %s for wallet ID %s and asset ID %s: %s",
				rwa.ReceiverWallet.ID, wallet.ID, rwa.Asset.ID, err.Error(),
			)
			continue
		}

		disbursementSMSRegistrationMessageTemplate := rwa.DisbursementSMSTemplate
		if disbursementSMSRegistrationMessageTemplate != nil && *disbursementSMSRegistrationMessageTemplate != "" {
			if !strings.Contains(*disbursementSMSRegistrationMessageTemplate, "{{.RegistrationLink}}") {
				*disbursementSMSRegistrationMessageTemplate = fmt.Sprintf("%s {{.RegistrationLink}}", strings.TrimSpace(*disbursementSMSRegistrationMessageTemplate))
			}

			msgTemplate, err = template.New("").Parse(*disbursementSMSRegistrationMessageTemplate)
			if err != nil {
				return fmt.Errorf("error parsing disbursement SMS registration message template: %w", err)
			}
		}

		content := new(strings.Builder)
		err = msgTemplate.Execute(content, struct {
			OrganizationName string
			RegistrationLink template.HTML
		}{
			OrganizationName: organization.Name,
			RegistrationLink: template.HTML(registrationLink),
		})
		if err != nil {
			return fmt.Errorf("error executing registration message template: %w", err)
		}

		msg := message.Message{
			ToPhoneNumber: rwa.ReceiverWallet.Receiver.PhoneNumber,
			Message:       content.String(),
		}

		assetID := rwa.Asset.ID
		receiverWalletID := rwa.ReceiverWallet.ID
		messageType := s.messengerClient.MessengerType()
		msgToInsert := &data.MessageInsert{
			Type:             messageType,
			AssetID:          &assetID,
			ReceiverID:       rwa.ReceiverWallet.Receiver.ID,
			WalletID:         wallet.ID,
			ReceiverWalletID: &receiverWalletID,
			TextEncrypted:    content.String(),
		}

		// We assume that the message will be sent at first
		msgToInsert.Status = data.SuccessMessageStatus
		if err := s.messengerClient.SendMessage(msg); err != nil {
			msg := fmt.Sprintf(
				"error sending message to receiver ID %s for receiver wallet ID %s using messenger type %s",
				rwa.ReceiverWallet.Receiver.ID, rwa.ReceiverWallet.ID, messageType,
			)
			// call crash tracker client to log and report error
			s.crashTrackerClient.LogAndReportErrors(ctx, err, msg)
			msgToInsert.Status = data.FailureMessageStatus
		}

		msgsToInsert = append(msgsToInsert, msgToInsert)

		// We don't want to update the `invitation_sent_at` for receiver wallets that we've sent the invitation SMS
		// because there's no way to calculate how many times we've resent the invitation SMS since
		// the first invitation if we update it.
		if rwa.ReceiverWallet.InvitationSentAt == nil && msgToInsert.Status == data.SuccessMessageStatus {
			receiverWalletIDs = append(receiverWalletIDs, rwa.ReceiverWallet.ID)
		}
	}

	return db.RunInTransaction(ctx, s.models.DBConnectionPool, nil, func(dbTx db.DBTransaction) error {
		if _, err := s.models.ReceiverWallet.UpdateInvitationSentAt(ctx, dbTx, receiverWalletIDs...); err != nil {
			return fmt.Errorf("updating receiver wallets' invitation sent at: %w", err)
		}

		if err := s.models.Message.BulkInsert(ctx, dbTx, msgsToInsert); err != nil {
			return fmt.Errorf("error inserting messages in the database: %w", err)
		}

		return nil
	})
}

// shouldSendInvitationSMS returns true if we should send the invitation SMS to the receiver. It will be used to either
// send the invitation for the first time, or to resend it automatically according with the organization's SMS Resend
// Interval and the maximum number of SMS resend attempts.

func (s SendReceiverWalletInviteService) shouldSendInvitationSMS(ctx context.Context, organization *data.Organization, rwa *data.ReceiverWalletAsset) bool {
	truncatedPhoneNumber := utils.TruncateString(rwa.ReceiverWallet.Receiver.PhoneNumber, 3)

	// We've never sent a Invitation SMS
	if rwa.ReceiverWallet.InvitationSentAt == nil {
		return true
	}

	// If organization's SMS Resend Interval is nil and we've sent the invitation message to the receiver, we won't resend it.
	if organization.SMSResendInterval == nil && rwa.ReceiverWallet.InvitationSentAt != nil {
		log.Ctx(ctx).Debugf(
			"the invitation message was not automatically resent to the receiver %s with phone number %s because the organization's SMS Resend Interval is nil",
			rwa.ReceiverWallet.Receiver.ID, truncatedPhoneNumber)
		return false
	}

	// The organizations has a interval to automatic resend the Invitation SMS.
	if organization.SMSResendInterval != nil {
		// Check if the receiver wallet reached the maximum number of SMS resend attempts.
		if rwa.ReceiverWallet.ReceiverWalletStats.TotalInvitationSMSResentAttempts >= s.maxInvitationSMSResendAttempts {
			log.Ctx(ctx).Debugf(
				"the invitation message was not resent to the receiver because the maximum number of SMS resend attempts has been reached: Phone Number: %s - Receiver ID %s - Wallet ID %s - Total Invitation SMS resent %d - Maximum attempts %d",
				truncatedPhoneNumber,
				rwa.ReceiverWallet.Receiver.ID,
				rwa.WalletID,
				rwa.ReceiverWallet.ReceiverWalletStats.TotalInvitationSMSResentAttempts,
				s.maxInvitationSMSResendAttempts,
			)
			return false
		}

		// Check if it's in the period to resend it.
		resendPeriod := time.Now().
			AddDate(0, 0, -int(*organization.SMSResendInterval*(rwa.ReceiverWallet.ReceiverWalletStats.TotalInvitationSMSResentAttempts+1)))
		if !rwa.ReceiverWallet.InvitationSentAt.Before(resendPeriod) {
			log.Ctx(ctx).Debugf(
				"the invitation message was not automatically resent to the receiver because the receiver is not in the resend period: Phone Number: %s - Receiver ID %s - Wallet ID %s - Last Invitation Sent At %s - SMS Resend Interval %d day(s)",
				truncatedPhoneNumber,
				rwa.ReceiverWallet.Receiver.ID,
				rwa.WalletID,
				rwa.ReceiverWallet.InvitationSentAt.Format(time.RFC1123),
				*organization.SMSResendInterval,
			)
			return false
		}
	}

	return true
}

func NewSendReceiverWalletInviteService(models *data.Models, messengerClient message.MessengerClient, anchorPlatformBaseSepURL, sep10SigningPrivateKey string, maxInvitationSMSResendAttempts int64, crashTrackerClient crashtracker.CrashTrackerClient) (*SendReceiverWalletInviteService, error) {
	s := &SendReceiverWalletInviteService{
		messengerClient:                messengerClient,
		models:                         models,
		anchorPlatformBaseSepURL:       anchorPlatformBaseSepURL,
		maxInvitationSMSResendAttempts: maxInvitationSMSResendAttempts,
		sep10SigningPrivateKey:         sep10SigningPrivateKey,
		crashTrackerClient:             crashTrackerClient,
	}

	if err := s.validate(); err != nil {
		return nil, fmt.Errorf("invalid service setup: %w", err)
	}

	return s, nil
}

type WalletDeepLink struct {
	// DeepLink is the deep link used to open the wallet invitation link.
	DeepLink string
	// Route is an optional parameter that can be used to specify the route to open in the wallet, in case it's not already present in the DeepLink.
	Route string // (optional)
	// AnchorPlatformBaseSepURL is the base URL of the /.well-known/stellar.toml file.
	AnchorPlatformBaseSepURL string
	// OrganizationName is the name of the organization that is sending the invitation.
	OrganizationName string
	// AssetCode is the code of the Stellar asset that the receiver will be able to receive.
	AssetCode string
	// AssetIssuer is the issuer of the Stellar asset that the receiver will be able to receive.
	AssetIssuer string
}

func (wdl WalletDeepLink) isNativeAsset() bool {
	return wdl.AssetIssuer == "" &&
		slices.Contains([]string{"XLM", "NATIVE"}, strings.ToUpper(wdl.AssetCode))
}

func (wdl WalletDeepLink) assetName() string {
	if wdl.isNativeAsset() {
		return "native"
	}

	return wdl.AssetCode + "-" + wdl.AssetIssuer
}

// BaseURLWithRoute returns the base URL of the deep link with the route appended.
func (wdl WalletDeepLink) BaseURLWithRoute() (string, error) {
	if wdl.DeepLink == "" {
		return "", fmt.Errorf("DeepLink can't be empty")
	}

	deepLink, err := url.Parse(wdl.DeepLink)
	if err != nil {
		return "", fmt.Errorf("error parsing DeepLink: %w", err)
	}

	if deepLink.Scheme == "" {
		deepLink.Scheme = "https"
	}

	if deepLink.Host == "" && deepLink.Path == "" && wdl.Route == "" {
		return "", fmt.Errorf("the deep link needs to have a valid host, or path, or route")
	}

	if wdl.Route != "" {
		if deepLink.Path == "" && deepLink.Host == "" {
			deepLink.Path = wdl.Route
		} else {
			deepLink.Path = path.Join(deepLink.Path, wdl.Route)
		}
	}

	return deepLink.String(), nil
}

func (wdl WalletDeepLink) TomlFileDomain() (string, error) {
	if wdl.AnchorPlatformBaseSepURL == "" {
		return "", fmt.Errorf("AnchorPlatformBaseSepURL can't be empty")
	}

	anchorPlatformBaseSepURL := wdl.AnchorPlatformBaseSepURL
	if !strings.Contains(anchorPlatformBaseSepURL, "://") {
		anchorPlatformBaseSepURL = "http://" + anchorPlatformBaseSepURL
	}

	anchorURL, err := url.Parse(anchorPlatformBaseSepURL)
	if err != nil {
		return "", fmt.Errorf("error parsing AnchorPlatformBaseSepURL '%s': %w", anchorPlatformBaseSepURL, err)
	}

	return anchorURL.Hostname(), nil
}

// validate will make sure all the parameters are set correctly.
func (wdl WalletDeepLink) validate() error {
	if wdl.DeepLink == "" {
		return fmt.Errorf("wallet schema can't be empty")
	}

	_, err := wdl.BaseURLWithRoute()
	if err != nil {
		return fmt.Errorf("can't generate a valid base URL for the deep link: %w", err)
	}

	if wdl.AnchorPlatformBaseSepURL == "" {
		return fmt.Errorf("toml file domain can't be empty")
	}

	if wdl.OrganizationName == "" {
		return fmt.Errorf("organization name can't be empty")
	}

	if wdl.AssetCode == "" {
		return fmt.Errorf("asset code can't be empty")
	}

	// not XLM:
	if !wdl.isNativeAsset() {
		if wdl.AssetIssuer == "" {
			return fmt.Errorf("asset issuer can't be empty unless the asset code is XLM")
		}

		if !strkey.IsValidEd25519PublicKey(wdl.AssetIssuer) {
			return fmt.Errorf("asset issuer is not a valid Ed25519 public key %s", wdl.AssetIssuer)
		}

		return nil
	}

	return nil
}

// GetUnsignedRegistrationLink creates a deep link for the wallet registration using the format below:
// <deep_link></route>?<domain>&<name>&<asset>.
func (wdl WalletDeepLink) GetUnsignedRegistrationLink() (string, error) {
	if err := wdl.validate(); err != nil {
		return "", fmt.Errorf("validating WalletDeepLink: %w", err)
	}

	tomlFileDomain, err := wdl.TomlFileDomain()
	if err != nil {
		return "", fmt.Errorf("getting WalletDeepLink toml file domain: %w", err)
	}

	baseURLWithRoute, err := wdl.BaseURLWithRoute()
	if err != nil {
		return "", fmt.Errorf("getting WalletDeepLink base URL: %w", err)
	}

	u, err := url.Parse(baseURLWithRoute)
	if err != nil {
		return "", fmt.Errorf("parsing DeepLink: %w", err)
	}

	q := u.Query()
	q.Add("domain", tomlFileDomain)
	q.Add("name", wdl.OrganizationName)
	q.Add("asset", wdl.assetName())

	u.RawQuery = q.Encode()

	return u.String(), nil
}

// GetSignedRegistrationLink will return the registration link accompanied with an extra query parameter containing the
// signature of the registration link, where the signature is created using the stellarSecretKey with the unsigned link
// as the message, keeping in mind that the insigned link query parameters were sorted in alphabetical order to generate
// the signature.
func (wdl WalletDeepLink) GetSignedRegistrationLink(stellarSecretKey string) (string, error) {
	unsignedRegistrationLink, err := wdl.GetUnsignedRegistrationLink()
	if err != nil {
		return "", fmt.Errorf("error getting unsigned registration link: %w", err)
	}

	signedRegistrationLink, err := utils.SignURL(stellarSecretKey, unsignedRegistrationLink)
	if err != nil {
		return "", fmt.Errorf("error signing registration link: %w", err)
	}

	return signedRegistrationLink, nil
}
