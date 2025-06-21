package bridge

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// Service provides business logic for Bridge integration operations.
type Service struct {
	client  ClientInterface
	baseURL string
	apiKey  string
	models  *data.Models
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
	OptInToBridge(ctx context.Context, userID string, fullName, email string) (*BridgeIntegrationInfo, error)
	GetBridgeIntegration(ctx context.Context) (*BridgeIntegrationInfo, error)
	CreateVirtualAccount(ctx context.Context, userID, memo, distributionAccountAddress string) (*BridgeIntegrationInfo, error)
}

var _ ServiceInterface = (*Service)(nil)

// ServiceOptions contains configuration options for the Bridge service.
type ServiceOptions struct {
	BaseURL string
	APIKey  string
	Models  *data.Models
}

// Validate validates the Bridge service options.
func (o ServiceOptions) Validate() error {
	if o.BaseURL == "" {
		return fmt.Errorf("BaseURL is required")
	}
	if o.APIKey == "" {
		return fmt.Errorf("APIKey is required")
	}
	if o.Models == nil {
		return fmt.Errorf("Models is required")
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
		client:  client,
		baseURL: opts.BaseURL,
		apiKey:  opts.APIKey,
		models:  opts.Models,
	}, nil
}

var (
	ErrBridgeAlreadyOptedIn              = errors.New("organization already opted into Bridge integration")
	ErrBridgeNotOptedIn                  = errors.New("organization has not opted into Bridge integration")
	ErrBridgeVirtualAccountAlreadyExists = errors.New("virtual account already exists for this organization")
	ErrBridgeKYCNotApproved              = errors.New("KYC verification is not approved, cannot create virtual account")
	ErrBridgeTOSNotAccepted              = errors.New("terms of service not accepted, cannot create virtual account")
	ErrBridgeKYCRejected                 = errors.New("KYC verification was rejected, cannot create virtual account")
)

// OptInToBridge creates a KYC link and opts the tenant into Bridge integration.
func (s *Service) OptInToBridge(ctx context.Context, userID string, fullName, email string) (*BridgeIntegrationInfo, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID is required to opt into Bridge integration")
	}
	if fullName == "" {
		return nil, fmt.Errorf("fullName is required to opt into Bridge integration")
	}
	if email == "" {
		return nil, fmt.Errorf("email is required to opt into Bridge integration")
	}

	// 1. Check if organization already opted in
	existing, err := s.models.BridgeIntegration.Get(ctx)
	if err != nil && !errors.Is(err, data.ErrRecordNotFound) {
		return nil, fmt.Errorf("checking existing Bridge integration: %w", err)
	}
	if existing != nil {
		return nil, ErrBridgeAlreadyOptedIn
	}

	// 2. Create KYC link via Bridge API for organization onboarding
	request := KYCLinkRequest{
		FullName: fullName,
		Email:    email,
		Type:     KYCTypeBusiness, // Always business for tenant organizations
	}

	kycLinkInfo, err := s.client.PostKYCLink(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("creating KYC link via Bridge API: %w", err)
	}

	// 3. Persist the Bridge integration IDs in the database
	integration, err := s.models.BridgeIntegration.Insert(ctx, data.BridgeIntegrationInsert{
		KYCLinkID:  kycLinkInfo.ID,
		CustomerID: kycLinkInfo.CustomerID,
		OptedInBy:  userID,
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

	// 2. If we have a KYC link ID, fetch live status from Bridge API.
	if integration.CustomerID != nil && integration.KYCLinkID != nil {
		result.CustomerID = integration.CustomerID
		result.OptedInBy = integration.OptedInBy
		result.OptedInAt = integration.OptedInAt

		kycResponse, kycErr := s.client.GetKYCLink(ctx, *integration.KYCLinkID)
		if kycErr != nil {
			return nil, fmt.Errorf("getting KYC link via Bridge API: %w", kycErr)
		}
		result.KYCLinkInfo = kycResponse
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

// CreateVirtualAccount creates a virtual account for the Bridge integration.
func (s *Service) CreateVirtualAccount(ctx context.Context, userID, memo, distributionAccountAddress string) (*BridgeIntegrationInfo, error) {
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

	// 2. Validate customer status
	if integration.CustomerID == nil {
		return nil, fmt.Errorf("bridge integration does not have a valid customer ID")
	}
	if integration.KYCLinkID == nil {
		return nil, fmt.Errorf("bridge integration does not have a valid KYC link ID")
	}

	kycResp, err := s.client.GetKYCLink(ctx, *integration.KYCLinkID)
	if err != nil {
		return nil, fmt.Errorf("getting KYC link via Bridge API: %w", err)
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

	// 3. Create virtual account request
	vaRequest := VirtualAccountRequest{
		Source: VirtualAccountSource{
			Currency: "usd", // Always USD for source
		},
		Destination: VirtualAccountDestination{
			PaymentRail:    "stellar",
			Currency:       "usdc",
			Address:        distributionAccountAddress,
			BlockchainMemo: memo,
		},
	}

	vaResponse, err := s.client.PostVirtualAccount(ctx, *integration.CustomerID, vaRequest)
	if err != nil {
		return nil, fmt.Errorf("creating virtual account via Bridge API: %w", err)
	}

	// 4. Update integration with virtual account ID and status
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

	// 5. Return response with all the data
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
