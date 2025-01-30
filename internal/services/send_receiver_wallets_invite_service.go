package services

import (
	"context"
	"fmt"
	"html/template"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type SendReceiverWalletInviteServiceInterface interface {
	SendInvite(ctx context.Context, receiverWalletInvitationData ...schemas.EventReceiverWalletInvitationData) error
}

type SendReceiverWalletInviteService struct {
	messageDispatcher           message.MessageDispatcherInterface
	Models                      *data.Models
	maxInvitationResendAttempts int64
	sep10SigningPrivateKey      string
	crashTrackerClient          crashtracker.CrashTrackerClient
}

var _ SendReceiverWalletInviteServiceInterface = new(SendReceiverWalletInviteService)

func (s SendReceiverWalletInviteService) validate() error {
	if s.messageDispatcher == nil {
		return fmt.Errorf("messenger dispatcher can't be nil")
	}

	return nil
}

// SendInvite sends the invitation’s deep link to the wallet’s application.
// The approach to sending the invitation is to send the deep link for each asset the wallet will pay based on the payment.
// For instance, the Wallet Foo is in two Ready Payments, one with USDC and the other with EUROC.
// So the receiver who has a Stellar Address pending registration (status:READY) in this wallet will receive both invites for USDC and EUROC.
// This would not impact the user receiving both token amounts. It's only for the registration process.
func (s SendReceiverWalletInviteService) SendInvite(ctx context.Context, receiverWalletInvitationData ...schemas.EventReceiverWalletInvitationData) error {
	if s.Models == nil {
		return fmt.Errorf("SendReceiverWalletInviteService.Models cannot be nil")
	}

	currentTenant, err := tenant.GetTenantFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting tenant from context: %w", err)
	}
	if currentTenant.BaseURL == nil {
		return fmt.Errorf("tenant base URL cannot be nil for tenant %s", currentTenant.ID)
	}

	// Get the organization entry to get the Org name and ReceiverRegistrationMessageTemplate
	organization, err := s.Models.Organizations.Get(ctx)
	if err != nil {
		return fmt.Errorf("getting organization: %w", err)
	}

	// Debug purposes
	if organization.ReceiverInvitationResendIntervalDays == nil {
		log.Ctx(ctx).Debug("automatic resend invitation is deactivated. Set a valid value to the organization's receiver_invitation_resend_interval_days to activate it.")
	}

	orgReceiverRegistrationMessageTemplate := organization.ReceiverRegistrationMessageTemplate
	if !strings.Contains(orgReceiverRegistrationMessageTemplate, "{{.RegistrationLink}}") {
		orgReceiverRegistrationMessageTemplate = fmt.Sprintf("%s {{.RegistrationLink}}", strings.TrimSpace(orgReceiverRegistrationMessageTemplate))
	}

	// Execute the template early so we avoid hitting the database to query the other info
	msgTemplate, err := template.New("").Parse(orgReceiverRegistrationMessageTemplate)
	if err != nil {
		return fmt.Errorf("parsing organization receiver registration message template: %w", err)
	}

	wallets, err := s.Models.Wallets.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("getting all wallets: %w", err)
	}

	walletsMap := make(map[string]data.Wallet, len(wallets))
	for _, wallet := range wallets {
		walletsMap[wallet.ID] = wallet
	}

	receiverWallets, err := s.resolveReceiverWalletsPendingRegistration(ctx, receiverWalletInvitationData)
	if err != nil {
		return fmt.Errorf("resolving receiver wallets pending registration: %w", err)
	}

	receiverWalletsAsset, err := s.Models.Assets.GetAssetsPerReceiverWallet(ctx, receiverWallets...)
	if err != nil {
		return fmt.Errorf("getting all assets: %w", err)
	}

	msgsToInsert := []*data.MessageInsert{}
	receiverWalletIDs := []string{}
	// TODO: improve this code adding go routines
	for _, rwa := range receiverWalletsAsset {
		if !s.shouldSendInvitation(ctx, organization, &rwa) {
			continue
		}

		wallet := walletsMap[rwa.WalletID]

		wdl := WalletDeepLink{
			DeepLink:         wallet.DeepLinkSchema,
			OrganizationName: organization.Name,
			AssetCode:        rwa.Asset.Code,
			AssetIssuer:      rwa.Asset.Issuer,
			TenantBaseURL:    *currentTenant.BaseURL,
		}

		registrationLink, err := wdl.GetSignedRegistrationLink(s.sep10SigningPrivateKey)
		if err != nil {
			log.Ctx(ctx).Errorf(
				"error getting signed registration link to receiver wallet ID %s for wallet ID %s and asset ID %s: %s",
				rwa.ReceiverWallet.ID, wallet.ID, rwa.Asset.ID, err.Error(),
			)
			continue
		}

		shortCode, err := s.Models.URLShortener.CreateShortURL(ctx, registrationLink)
		if err != nil {
			log.Ctx(ctx).Errorf(
				"shortening registration link to receiver wallet ID %s and asset ID %s: %v",
				rwa.ReceiverWallet.ID, rwa.Asset.ID, err,
			)
			continue
		}

		shortenedRegistrationLink, err := url.JoinPath(*currentTenant.BaseURL, "r", shortCode)
		if err != nil {
			log.Ctx(ctx).Errorf(
				"building shortened registration link for shortCode %s: %v",
				shortCode, err,
			)
			continue
		}

		disbursementReceiverRegistrationMessageTemplate := rwa.DisbursementReceiverRegistrationMsgTemplate
		if disbursementReceiverRegistrationMessageTemplate != nil && *disbursementReceiverRegistrationMessageTemplate != "" {
			if !strings.Contains(*disbursementReceiverRegistrationMessageTemplate, "{{.RegistrationLink}}") {
				*disbursementReceiverRegistrationMessageTemplate = fmt.Sprintf("%s {{.RegistrationLink}}", strings.TrimSpace(*disbursementReceiverRegistrationMessageTemplate))
			}

			msgTemplate, err = template.New("").Parse(*disbursementReceiverRegistrationMessageTemplate)
			if err != nil {
				return fmt.Errorf("parsing disbursement receiver registration message template: %w", err)
			}
		}

		content := new(strings.Builder)
		err = msgTemplate.Execute(content, struct {
			OrganizationName string
			RegistrationLink template.HTML
		}{
			OrganizationName: organization.Name,
			RegistrationLink: template.HTML(shortenedRegistrationLink),
		})
		if err != nil {
			return fmt.Errorf("executing registration message template: %w", err)
		}

		msg := message.Message{Body: content.String()}
		if rwa.ReceiverWallet.Receiver.PhoneNumber != "" {
			msg.ToPhoneNumber = rwa.ReceiverWallet.Receiver.PhoneNumber
		}
		if rwa.ReceiverWallet.Receiver.Email != "" {
			msg.ToEmail = rwa.ReceiverWallet.Receiver.Email
			msg.Title = "You have a payment waiting for you from " + organization.Name
		}

		msgToInsert := &data.MessageInsert{
			AssetID:          &rwa.Asset.ID,
			ReceiverID:       rwa.ReceiverWallet.Receiver.ID,
			WalletID:         wallet.ID,
			ReceiverWalletID: &rwa.ReceiverWallet.ID,
			TextEncrypted:    msg.Body,
			TitleEncrypted:   msg.Title,
		}

		if messengerType, sendErr := s.messageDispatcher.SendMessage(ctx, msg, organization.MessageChannelPriority); sendErr != nil {
			errMsg := fmt.Sprintf(
				"error sending message to receiver ID %s for receiver wallet ID %s using messenger type %s",
				rwa.ReceiverWallet.Receiver.ID, rwa.ReceiverWallet.ID, messengerType,
			)
			// call crash tracker client to log and report error
			s.crashTrackerClient.LogAndReportErrors(ctx, sendErr, errMsg)
			msgToInsert.Status = data.FailureMessageStatus
			msgToInsert.Type = messengerType
		} else {
			msgToInsert.Status = data.SuccessMessageStatus
			msgToInsert.Type = messengerType
		}

		msgsToInsert = append(msgsToInsert, msgToInsert)

		// We don't want to update the `invitation_sent_at` for receiver wallets for which we've already sent the invitation message
		// because there's no way to calculate how many times we've resent the invitation message since
		// the first invitation if we update it.
		if rwa.ReceiverWallet.InvitationSentAt == nil && msgToInsert.Status == data.SuccessMessageStatus {
			receiverWalletIDs = append(receiverWalletIDs, rwa.ReceiverWallet.ID)
		}
	}

	return db.RunInTransaction(ctx, s.Models.DBConnectionPool, nil, func(dbTx db.DBTransaction) error {
		if _, err := s.Models.ReceiverWallet.UpdateInvitationSentAt(ctx, dbTx, receiverWalletIDs...); err != nil {
			return fmt.Errorf("updating receiver wallets' invitation sent at: %w", err)
		}

		if err := s.Models.Message.BulkInsert(ctx, dbTx, msgsToInsert); err != nil {
			return fmt.Errorf("inserting messages in the database: %w", err)
		}

		return nil
	})
}

// resolveReceiverWalletsPendingRegistration returns the receiver wallets pending registration based on the receiverWalletInvitationData.
// If the receiverWalletInvitationData is empty, it will return all receiver wallets pending registration.
func (s SendReceiverWalletInviteService) resolveReceiverWalletsPendingRegistration(ctx context.Context, receiverWalletInvitationData []schemas.EventReceiverWalletInvitationData) ([]*data.ReceiverWallet, error) {
	var err error
	var receiverWallets []*data.ReceiverWallet
	if len(receiverWalletInvitationData) == 0 {
		receiverWallets, err = s.Models.ReceiverWallet.GetAllPendingRegistrations(ctx, s.Models.DBConnectionPool)
		if err != nil {
			return nil, fmt.Errorf("getting all receiver wallets pending registration: %w", err)
		}
	} else {
		receiverWalletIDsPendingRegistration := make([]string, 0, len(receiverWalletInvitationData))
		for _, receiverWallet := range receiverWalletInvitationData {
			receiverWalletIDsPendingRegistration = append(receiverWalletIDsPendingRegistration, receiverWallet.ReceiverWalletID)
		}
		receiverWallets, err = s.Models.ReceiverWallet.GetAllPendingRegistrationByReceiverWalletIDs(ctx, s.Models.DBConnectionPool, receiverWalletIDsPendingRegistration)
		if err != nil {
			return nil, fmt.Errorf("getting receiver wallets pending registration by rw ids %v: %w", receiverWalletIDsPendingRegistration, err)
		}
	}
	return receiverWallets, err
}

// shouldSendInvitation returns true if we should send the invitation to the receiver. It will be used to either
// send the invitation for the first time, or to resend it automatically according to the organization's Resend
// Interval and the maximum number of resend attempts.
func (s SendReceiverWalletInviteService) shouldSendInvitation(ctx context.Context, organization *data.Organization, rwa *data.ReceiverWalletAsset) bool {
	receiver := rwa.ReceiverWallet.Receiver

	// We've never sent an Invitation message
	if rwa.ReceiverWallet.InvitationSentAt == nil {
		return true
	}

	// If organization's Receiver Invitation Resend Interval is nil and we've sent the invitation message to the receiver, we won't resend it.
	if organization.ReceiverInvitationResendIntervalDays == nil && rwa.ReceiverWallet.InvitationSentAt != nil {
		log.Ctx(ctx).Debugf(
			"the invitation message was not automatically resent to the receiver %s because the organization's Receiver Invitation Resend Interval is nil",
			receiver.ID)
		return false
	}

	// The organizations defined an interval to automatically resend the receiver invitation message.
	if organization.ReceiverInvitationResendIntervalDays != nil {
		// Check if the receiver wallet reached the maximum number of resend attempts.
		if rwa.ReceiverWallet.ReceiverWalletStats.TotalInvitationResentAttempts >= s.maxInvitationResendAttempts {
			log.Ctx(ctx).Debugf(
				"the invitation message was not resent to the receiver because the maximum number of message resend attempts has been reached: Receiver ID %s - Wallet ID %s - Total Invitation resent %d - Maximum attempts %d",
				receiver.ID,
				rwa.WalletID,
				rwa.ReceiverWallet.ReceiverWalletStats.TotalInvitationResentAttempts,
				s.maxInvitationResendAttempts,
			)
			return false
		}

		// Check if it's in the period to resend it.
		resendPeriod := time.Now().
			AddDate(0, 0, -int(*organization.ReceiverInvitationResendIntervalDays*(rwa.ReceiverWallet.ReceiverWalletStats.TotalInvitationResentAttempts+1)))
		if !rwa.ReceiverWallet.InvitationSentAt.Before(resendPeriod) {
			log.Ctx(ctx).Debugf(
				"the invitation message was not automatically resent to the receiver because the receiver is not in the resend period: Receiver ID %s - Wallet ID %s - Last Invitation Sent At %s - Receiver Invitation Resend Interval %d day(s)",
				receiver.ID,
				rwa.WalletID,
				rwa.ReceiverWallet.InvitationSentAt.Format(time.RFC1123),
				*organization.ReceiverInvitationResendIntervalDays,
			)
			return false
		}
	}

	return true
}

func NewSendReceiverWalletInviteService(models *data.Models, messageDispatcher message.MessageDispatcherInterface, sep10SigningPrivateKey string, maxInvitationResendAttempts int64, crashTrackerClient crashtracker.CrashTrackerClient) (*SendReceiverWalletInviteService, error) {
	s := &SendReceiverWalletInviteService{
		messageDispatcher:           messageDispatcher,
		Models:                      models,
		maxInvitationResendAttempts: maxInvitationResendAttempts,
		sep10SigningPrivateKey:      sep10SigningPrivateKey,
		crashTrackerClient:          crashTrackerClient,
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
	// OrganizationName is the name of the organization that is sending the invitation.
	OrganizationName string
	// AssetCode is the code of the Stellar asset that the receiver will be able to receive.
	AssetCode string
	// AssetIssuer is the issuer of the Stellar asset that the receiver will be able to receive.
	AssetIssuer string
	// TenantBaseURL is the base URL for the tenant that the receiver wallet belongs to.
	TenantBaseURL string
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
	if wdl.TenantBaseURL == "" {
		return "", fmt.Errorf("base URL for tenant can't be empty")
	}

	tenantBaseURL, err := utils.GetURLWithScheme(wdl.TenantBaseURL)
	if err != nil {
		return "", fmt.Errorf("setting the protocol scheme: %w", err)
	}

	tenantURL, err := url.Parse(tenantBaseURL)
	if err != nil {
		return "", fmt.Errorf("error parsing TenantBaseURL %s: %w", tenantBaseURL, err)
	}

	return tenantURL.Hostname(), nil
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

	if wdl.TenantBaseURL == "" {
		return fmt.Errorf("tenant base URL can't be empty")
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
