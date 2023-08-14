package services

import (
	"context"
	"fmt"
	"html/template"
	"net/url"
	"path"
	"strings"

	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type SendReceiverWalletInviteService struct {
	messengerClient          message.MessengerClient
	models                   *data.Models
	anchorPlatformBaseSepURL string
	minDaysBetweenRetries    int
	maxRetries               int
	sep10SigningPrivateKey   string
	crashTrackerClient       crashtracker.CrashTrackerClient
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

	// Execute the template early so we avoid hitting the database to query the other info
	msgTemplate, err := template.New("").Parse(organization.SMSRegistrationMessageTemplate)
	if err != nil {
		return fmt.Errorf("error parsing SMS registration message template: %w", err)
	}

	wallets, err := s.models.Wallets.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("error getting all wallets: %w", err)
	}

	walletsMap := make(map[string]data.Wallet, len(wallets))
	for _, wallet := range wallets {
		walletsMap[wallet.ID] = wallet
	}

	receiverWallets, err := s.models.ReceiverWallet.GetAllPendingRegistration(ctx, s.minDaysBetweenRetries, s.maxRetries)
	if err != nil {
		return fmt.Errorf("error getting receiver wallets pending registration: %w", err)
	}

	receiverWalletsAsset, err := s.models.Assets.GetAssetsPerReceiverWallet(ctx, receiverWallets...)
	if err != nil {
		return fmt.Errorf("error getting all assets: %w", err)
	}

	msgsToInsert := []*data.MessageInsert{}
	// TODO: improve this code adding go routines
	for _, rwa := range receiverWalletsAsset {
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

		receiverWalletID := rwa.ReceiverWallet.ID
		messageType := s.messengerClient.MessengerType()
		msgToInsert := &data.MessageInsert{
			Type:             messageType,
			AssetID:          nil,
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
	}

	if err := s.models.Message.BulkInsert(ctx, msgsToInsert); err != nil {
		return fmt.Errorf("error inserting messages in the database: %w", err)
	}

	return nil
}

func NewSendReceiverWalletInviteService(models *data.Models, messengerClient message.MessengerClient, anchorPlatformBaseSepURL, sep10SigningPrivateKey string, minDaysBetweenRetries, maxRetries int, crashTrackerClient crashtracker.CrashTrackerClient) (*SendReceiverWalletInviteService, error) {
	s := &SendReceiverWalletInviteService{
		messengerClient:          messengerClient,
		models:                   models,
		anchorPlatformBaseSepURL: anchorPlatformBaseSepURL,
		minDaysBetweenRetries:    minDaysBetweenRetries,
		maxRetries:               maxRetries,
		sep10SigningPrivateKey:   sep10SigningPrivateKey,
		crashTrackerClient:       crashTrackerClient,
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
	if strings.ToUpper(wdl.AssetCode) != "XLM" {
		if wdl.AssetIssuer == "" {
			return fmt.Errorf("asset issuer can't be empty unless the asset code is XLM")
		}

		if !strkey.IsValidEd25519PublicKey(wdl.AssetIssuer) {
			return fmt.Errorf("asset issuer is not a valid Ed25519 public key %s", wdl.AssetIssuer)
		}

		return nil
	}

	// XLM:
	if wdl.AssetIssuer != "" {
		return fmt.Errorf("asset issuer should be empty for XLM, but is %s", wdl.AssetIssuer)
	}

	return nil
}

// GetUnsignedRegistrationLink creates a deep link for the wallet registration using the format below:
// <deep_link></route>?<domain>&<name>&<asset>.
func (wdl WalletDeepLink) GetUnsignedRegistrationLink() (string, error) {
	if err := wdl.validate(); err != nil {
		return "", fmt.Errorf("validating WalletDeepLink: %w", err)
	}

	assetName := wdl.AssetCode
	if wdl.AssetIssuer != "" {
		assetName += "-" + wdl.AssetIssuer
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
	q.Add("asset", assetName)

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
