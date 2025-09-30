package bridge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

// Service provides business logic for Bridge integration operations.
type Service struct {
	client                      ClientInterface
	baseURL                     string
	apiKey                      string
	models                      *data.Models
	distributionAccountResolver signing.DistributionAccountResolver
	distributionAccountService  services.DistributionAccountServiceInterface
	networkType                 utils.NetworkType
}

// BridgeIntegrationInfo represents the composite information about Bridge integration.
type BridgeIntegrationInfo struct {
	Status                  data.BridgeIntegrationStatus `json:"status"`
	CustomerID              *string                      `json:"customer_id,omitempty"`
	KYCLinkInfo             *KYCLinkInfo                 `json:"kyc_status,omitempty"`
	VirtualAccountDetails   *VirtualAccountInfo          `json:"virtual_account,omitempty"`
	OptedInBy               *string                      `json:"opted_in_by,omitempty"`
	OptedInAt               *time.Time                   `json:"opted_in_at,omitempty"`
	VirtualAccountCreatedBy *string                      `json:"virtual_account_created_by,omitempty"`
	VirtualAccountCreatedAt *time.Time                   `json:"virtual_account_created_at,omitempty"`
}

// ServiceInterface defines the interface for Bridge integration business operations.
//
//go:generate mockery --name=ServiceInterface --case=underscore --structname=MockService --output=. --filename=service_mock.go --inpackage
type ServiceInterface interface {
	OptInToBridge(ctx context.Context, opts OptInOptions) (*BridgeIntegrationInfo, error)
	GetBridgeIntegration(ctx context.Context) (*BridgeIntegrationInfo, error)
	CreateVirtualAccount(ctx context.Context, userID, distributionAccountAddress string) (*BridgeIntegrationInfo, error)
	OptInForExistingCustomer(ctx context.Context, customerID, userID string) (*BridgeIntegrationInfo, error)
}

var _ ServiceInterface = (*Service)(nil)

// ServiceOptions contains configuration options for the Bridge service.
type ServiceOptions struct {
	BaseURL                     string
	APIKey                      string
	Models                      *data.Models
	DistributionAccountResolver signing.DistributionAccountResolver
	DistributionAccountService  services.DistributionAccountServiceInterface
	NetworkType                 utils.NetworkType
}

// Validate validates the Bridge service options.
func (o ServiceOptions) Validate() error {
	if o.BaseURL == "" {
		return fmt.Errorf("baseURL is required")
	}
	if o.APIKey == "" {
		return fmt.Errorf("apiKey is required")
	}
	if o.Models == nil {
		return fmt.Errorf("models is required")
	}
	if o.DistributionAccountResolver == nil {
		return fmt.Errorf("distributionAccountResolver is required")
	}
	if o.DistributionAccountService == nil {
		return fmt.Errorf("distributionAccountService is required")
	}
	if err := o.NetworkType.Validate(); err != nil {
		return fmt.Errorf("validating NetworkType: %w", err)
	}
	return nil
}

// NewService creates a new Bridge service instance.
func NewService(opts ServiceOptions) (*Service, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("validating bridge.Service options: %w", err)
	}

	client, err := NewClient(ClientOptions{
		BaseURL: opts.BaseURL,
		APIKey:  opts.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("creating Bridge client: %w", err)
	}

	return &Service{
		client:                      client,
		baseURL:                     opts.BaseURL,
		apiKey:                      opts.APIKey,
		models:                      opts.Models,
		distributionAccountResolver: opts.DistributionAccountResolver,
		distributionAccountService:  opts.DistributionAccountService,
		networkType:                 opts.NetworkType,
	}, nil
}

var (
	ErrBridgeAlreadyOptedIn              = errors.New("organization already opted into Bridge integration")
	ErrBridgeNotOptedIn                  = errors.New("organization has not opted into Bridge integration")
	ErrBridgeVirtualAccountAlreadyExists = errors.New("virtual account already exists for this organization")
	ErrBridgeKYCNotApproved              = errors.New("KYC verification is not approved, cannot create virtual account")
	ErrBridgeTOSNotAccepted              = errors.New("terms of service not accepted, cannot create virtual account")
	ErrBridgeKYCRejected                 = errors.New("KYC verification was rejected, cannot create virtual account")
	ErrBridgeUSDCTrustlineRequired       = errors.New("distribution account must have a USDC trustline to opt into Bridge integration")
	ErrBridgeInvalidCustomerID           = errors.New("provided Bridge customer ID is not valid")
	ErrBridgeCustomerNotActive           = errors.New("provided Bridge customer is not active")
)

type OptInOptions struct {
	UserID      string
	FullName    string
	Email       string
	RedirectURL string
	KYCType     CustomerType
}

func (opts OptInOptions) Validate() error {
	if opts.UserID == "" {
		return fmt.Errorf("userID is required to opt into Bridge integration")
	}
	if opts.FullName == "" {
		return fmt.Errorf("fullName is required to opt into Bridge integration")
	}
	if opts.Email == "" {
		return fmt.Errorf("email is required to opt into Bridge integration")
	}
	if opts.RedirectURL == "" {
		return fmt.Errorf("redirectURL is required to opt into Bridge integration")
	}
	if opts.KYCType != CustomerTypeIndividual && opts.KYCType != CustomerTypeBusiness {
		return fmt.Errorf("CustomerType must be either 'individual' or 'business'")
	}
	return nil
}

// OptInToBridge creates a KYC link and opts the tenant into Bridge integration.
func (s *Service) OptInToBridge(ctx context.Context, opts OptInOptions) (*BridgeIntegrationInfo, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("validating opt-in options: %w", err)
	}

	// 1. Validate USDC trustline exists
	if err := s.validateUSDCTrustline(ctx); err != nil {
		return nil, fmt.Errorf("validating USDC trustline: %w", err)
	}

	// 2. Check if organization already opted in
	existing, err := s.models.BridgeIntegration.Get(ctx)
	if err != nil && !errors.Is(err, data.ErrRecordNotFound) {
		return nil, fmt.Errorf("checking existing Bridge integration: %w", err)
	}
	if existing != nil {
		return nil, ErrBridgeAlreadyOptedIn
	}

	// 3. Create KYC link via Bridge API for organization onboarding
	request := KYCLinkRequest{
		FullName:    opts.FullName,
		Email:       opts.Email,
		Type:        opts.KYCType,
		RedirectURI: opts.RedirectURL,
	}

	kycLinkInfo, err := s.client.PostKYCLink(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("creating KYC link via Bridge API: %w", err)
	}

	// 4. Persist the Bridge integration IDs in the database
	integration, err := s.models.BridgeIntegration.Insert(ctx, data.BridgeIntegrationInsert{
		KYCLinkID:  utils.StringPtr(kycLinkInfo.ID),
		CustomerID: kycLinkInfo.CustomerID,
		OptedInBy:  opts.UserID,
	})
	if err != nil {
		return nil, fmt.Errorf("storing Bridge integration in database: %w", err)
	}

	return &BridgeIntegrationInfo{
		Status:     integration.Status,
		CustomerID: integration.CustomerID,
		OptedInBy:  integration.OptedInBy,
		OptedInAt:  integration.OptedInAt,
		// Live data from Bridge API response
		KYCLinkInfo: kycLinkInfo,
	}, nil
}

// GetBridgeIntegration retrieves the current Bridge integration status with live data.
func (s *Service) GetBridgeIntegration(ctx context.Context) (*BridgeIntegrationInfo, error) {
	// 1. Get basic integration info from database.
	integration, err := s.models.BridgeIntegration.Get(ctx)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			// No integration record exists - return NOT_OPTED_IN status
			return &BridgeIntegrationInfo{
				Status: data.BridgeIntegrationStatusNotOptedIn,
			}, nil
		}
		return nil, fmt.Errorf("getting Bridge integration: %w", err)
	}

	result := &BridgeIntegrationInfo{
		Status: integration.Status,
	}

	// 2. If we have a customer ID, fetch live data from Bridge API.
	//    This includes KYC link info if we have a KYC link ID.
	if integration.CustomerID != nil {
		result.CustomerID = integration.CustomerID
		result.OptedInBy = integration.OptedInBy
		result.OptedInAt = integration.OptedInAt

		customerInfo, custErr := s.client.GetCustomer(ctx, *integration.CustomerID)
		if custErr != nil {
			return nil, fmt.Errorf("getting customer info via Bridge API: %w", custErr)
		}
		if customerInfo.Status == CustomerStatusActive {
			result.KYCLinkInfo = &KYCLinkInfo{
				CustomerID: customerInfo.ID,
				Email:      customerInfo.Email,
				FullName:   fmt.Sprintf("%s %s", customerInfo.FirstName, customerInfo.LastName),
				Type:       customerInfo.Type,
				KYCStatus:  KYCStatusApproved,
				TOSStatus:  TOSStatusApproved,
			}
		} else if integration.KYCLinkID != nil {
			kycResponse, kycErr := s.client.GetKYCLink(ctx, *integration.KYCLinkID)
			if kycErr != nil {
				return nil, fmt.Errorf("getting KYC link via Bridge API: %w", kycErr)
			}
			result.KYCLinkInfo = kycResponse
		}
	}

	// 3. If we have a virtual account ID, fetch live data from Bridge API.
	if integration.CustomerID != nil && integration.VirtualAccountID != nil {
		result.VirtualAccountCreatedBy = integration.VirtualAccountCreatedBy
		result.VirtualAccountCreatedAt = integration.VirtualAccountCreatedAt

		vaResponse, vaErr := s.client.GetVirtualAccount(ctx, *integration.CustomerID, *integration.VirtualAccountID)
		if vaErr != nil {
			return nil, fmt.Errorf("getting virtual account via Bridge API: %w", vaErr)
		}
		result.VirtualAccountDetails = vaResponse
	}

	return result, nil
}

const (
	sourceCurrencyUSD       = "usd"
	destinationCurrencyUSDC = "usdc"
	destinationRailStellar  = "stellar"
)

// CreateVirtualAccount creates a virtual account for the Bridge integration.
func (s *Service) CreateVirtualAccount(ctx context.Context, userID, distributionAccountAddress string) (*BridgeIntegrationInfo, error) {
	// 1. Get existing integration
	integration, err := s.models.BridgeIntegration.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting Bridge integration: %w", err)
	}

	if integration.Status != data.BridgeIntegrationStatusOptedIn {
		return nil, ErrBridgeNotOptedIn
	}

	if integration.VirtualAccountID != nil {
		return nil, ErrBridgeVirtualAccountAlreadyExists
	}

	// 2. Validate KYC Link if exists
	if integration.KYCLinkID != nil {
		kycResp, kycErr := s.client.GetKYCLink(ctx, *integration.KYCLinkID)
		if kycErr != nil {
			return nil, fmt.Errorf("getting KYC link via Bridge API: %w", kycErr)
		}

		if kycResp.KYCStatus == KYCStatusRejected {
			return nil, s.recordRejectedStatus(ctx, kycResp.RejectionReasons)
		}
		if kycResp.KYCStatus != KYCStatusApproved {
			return nil, ErrBridgeKYCNotApproved
		}

		if kycResp.TOSStatus != TOSStatusApproved {
			return nil, ErrBridgeTOSNotAccepted
		}
	}

	// 3. Validate customer status
	if integration.CustomerID == nil {
		return nil, fmt.Errorf("bridge integration does not have a valid customer ID")
	}
	customerInfo, err := s.client.GetCustomer(ctx, *integration.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("getting customer info via Bridge API: %w", err)
	}
	if customerInfo.Status != CustomerStatusActive {
		return nil, ErrBridgeCustomerNotActive
	}

	memo, err := tenant.GenerateMemoForTenant(ctx)
	if err != nil {
		return nil, fmt.Errorf("generating memo for tenant: %w", err)
	}

	// 4. Create virtual account request
	vaRequest := VirtualAccountRequest{
		Source: VirtualAccountSource{
			Currency: sourceCurrencyUSD,
		},
		Destination: VirtualAccountDestination{
			PaymentRail:    destinationRailStellar,
			Currency:       destinationCurrencyUSDC,
			Address:        distributionAccountAddress,
			BlockchainMemo: memo.Value,
		},
	}

	vaResponse, err := s.client.PostVirtualAccount(ctx, *integration.CustomerID, vaRequest)
	if err != nil {
		return nil, fmt.Errorf("creating virtual account via Bridge API: %w", err)
	}

	// 5. Update integration with virtual account ID and status
	now := time.Now()
	update := data.BridgeIntegrationUpdate{
		VirtualAccountID:        &vaResponse.ID,
		Status:                  utils.Ptr(data.BridgeIntegrationStatusReadyForDeposit),
		VirtualAccountCreatedBy: &userID,
		VirtualAccountCreatedAt: &now,
	}

	updatedIntegration, err := s.models.BridgeIntegration.Update(ctx, update)
	if err != nil {
		return nil, fmt.Errorf("updating Bridge integration with virtual account: %w", err)
	}

	// 6. Return response with all the data
	result := &BridgeIntegrationInfo{
		Status:                  updatedIntegration.Status,
		CustomerID:              updatedIntegration.CustomerID,
		VirtualAccountCreatedAt: updatedIntegration.VirtualAccountCreatedAt,
		VirtualAccountCreatedBy: updatedIntegration.VirtualAccountCreatedBy,
		VirtualAccountDetails:   vaResponse,
	}

	return result, nil
}

// recordRejectedStatus updates the Bridge integration status to ERROR and returns an error with rejection reasons.
func (s *Service) recordRejectedStatus(ctx context.Context, rejectionReasons []string) error {
	errorMsg := fmt.Sprintf("KYC verification rejected: %v", rejectionReasons)
	update := data.BridgeIntegrationUpdate{
		Status:       utils.Ptr(data.BridgeIntegrationStatusError),
		ErrorMessage: &errorMsg,
	}
	if _, updateErr := s.models.BridgeIntegration.Update(ctx, update); updateErr != nil {
		return fmt.Errorf("updating Bridge integration with error status: %w", updateErr)
	}
	return fmt.Errorf("%w: %v", ErrBridgeKYCRejected, rejectionReasons)
}

// validateUSDCTrustline checks if the distribution account has a USDC trustline.
func (s *Service) validateUSDCTrustline(ctx context.Context) error {
	distributionAccount, err := s.distributionAccountResolver.DistributionAccountFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting distribution account from context: %w", err)
	}

	// Get the appropriate USDC asset based on network type
	var usdcAsset data.Asset
	if s.networkType.IsPubnet() {
		usdcAsset = assets.USDCAssetPubnet
	} else {
		usdcAsset = assets.USDCAssetTestnet
	}

	// Check if the distribution account has USDC balance (which implies trustline exists)
	if _, err = s.distributionAccountService.GetBalance(ctx, &distributionAccount, usdcAsset); err != nil {
		return fmt.Errorf("%w: %w", ErrBridgeUSDCTrustlineRequired, err)
	}

	return nil
}

// OptInForExistingCustomer directly opt in the tenant into Bridge integration with a provided customer ID.
func (s *Service) OptInForExistingCustomer(ctx context.Context, customerID, userID string) (*BridgeIntegrationInfo, error) {
	if strings.TrimSpace(customerID) == "" {
		return nil, fmt.Errorf("customer ID is required")
	}
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	// 1. Check if organization already opted in
	existing, err := s.models.BridgeIntegration.Get(ctx)
	if err != nil && !errors.Is(err, data.ErrRecordNotFound) {
		return nil, fmt.Errorf("checking existing Bridge integration: %w", err)
	}
	if existing != nil {
		return nil, ErrBridgeAlreadyOptedIn
	}

	// 2. Validate customer ID exists and is active via Bridge API
	customerInfo, err := s.client.GetCustomer(ctx, customerID)
	if err != nil {
		log.Ctx(ctx).Errorf("Error validating Bridge customer ID %s during onboarding: %v", customerID, err)
		return nil, fmt.Errorf("%w: %w", ErrBridgeInvalidCustomerID, err)
	}
	if customerInfo.Status != CustomerStatusActive {
		log.Ctx(ctx).Errorf("Bridge customer ID %s is not active, current status is %q", customerID, customerInfo.Status)
		return nil, ErrBridgeCustomerNotActive
	}

	// 3. Store Bridge integration with provided customer ID
	integration, err := s.models.BridgeIntegration.Insert(ctx, data.BridgeIntegrationInsert{
		CustomerID: customerID,
		OptedInBy:  userID,
	})
	if err != nil {
		return nil, fmt.Errorf("storing manual Bridge integration in database: %w", err)
	}

	return &BridgeIntegrationInfo{
		Status:     integration.Status,
		CustomerID: integration.CustomerID,
		OptedInBy:  integration.OptedInBy,
		OptedInAt:  integration.OptedInAt,
	}, nil
}
