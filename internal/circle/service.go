package circle

import (
	"context"
	"fmt"

	"github.com/stellar/go/strkey"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type Service struct {
	ClientFactory        ClientFactory
	ClientConfigModel    ClientConfigModelInterface
	NetworkType          utils.NetworkType
	EncryptionPassphrase string
	TenantManager        tenant.ManagerInterface
	MonitorService       monitor.MonitorServiceInterface
}

const StellarChainCode = "XLM"

// ServiceInterface defines the interface for Circle related SDP operations.
//
//go:generate mockery --name=ServiceInterface --case=underscore --structname=MockService --output=. --filename=service_mock.go --inpackage
type ServiceInterface interface {
	ClientInterface
	SendPayout(ctx context.Context, paymentRequest PaymentRequest) (*Payout, error)
	SendTransfer(ctx context.Context, paymentRequest PaymentRequest) (*Transfer, error)
}

var _ ServiceInterface = (*Service)(nil)

type ServiceOptions struct {
	ClientFactory        ClientFactory
	ClientConfigModel    ClientConfigModelInterface
	TenantManager        tenant.ManagerInterface
	NetworkType          utils.NetworkType
	EncryptionPassphrase string
	MonitorService       monitor.MonitorServiceInterface
}

func (o ServiceOptions) Validate() error {
	if o.ClientFactory == nil {
		return fmt.Errorf("ClientFactory is required")
	}

	if o.ClientConfigModel == nil {
		return fmt.Errorf("ClientConfigModel is required")
	}

	if o.TenantManager == nil {
		return fmt.Errorf("TenantManager is required")
	}

	if o.MonitorService == nil {
		return fmt.Errorf("MonitorService is required")
	}

	err := o.NetworkType.Validate()
	if err != nil {
		return fmt.Errorf("validating NetworkType: %w", err)
	}

	if !strkey.IsValidEd25519SecretSeed(o.EncryptionPassphrase) {
		return fmt.Errorf("EncryptionPassphrase is invalid")
	}

	return nil
}

func NewService(opts ServiceOptions) (*Service, error) {
	err := opts.Validate()
	if err != nil {
		return nil, fmt.Errorf("validating circle.Service options: %w", err)
	}

	return &Service{
		ClientFactory:        opts.ClientFactory,
		ClientConfigModel:    opts.ClientConfigModel,
		NetworkType:          opts.NetworkType,
		EncryptionPassphrase: opts.EncryptionPassphrase,
		TenantManager:        opts.TenantManager,
		MonitorService:       opts.MonitorService,
	}, nil
}

func (s *Service) SendTransfer(ctx context.Context, paymentRequest PaymentRequest) (*Transfer, error) {
	if paymentRequest.APIType != APITypeTransfers {
		return nil, fmt.Errorf("SendTransfer requires APITypeTransfers")
	}

	if err := paymentRequest.Validate(); err != nil {
		return nil, fmt.Errorf("validating payment request: %w", err)
	}

	circleAssetCode, err := paymentRequest.GetCircleAssetCode()
	if err != nil {
		return nil, fmt.Errorf("getting Circle asset code: %w", err)
	}

	return s.PostTransfer(ctx, TransferRequest{
		IdempotencyKey: paymentRequest.IdempotencyKey,
		Amount: Balance{
			Amount:   paymentRequest.Amount,
			Currency: circleAssetCode,
		},
		Source: TransferAccount{
			Type: TransferAccountTypeWallet,
			ID:   paymentRequest.SourceWalletID,
		},
		Destination: TransferAccount{
			Type:    TransferAccountTypeBlockchain,
			Chain:   StellarChainCode,
			Address: paymentRequest.DestinationStellarAddress,
		},
	})
}

func (s *Service) SendPayout(ctx context.Context, paymentRequest PaymentRequest) (*Payout, error) {
	if paymentRequest.APIType != APITypePayouts {
		return nil, fmt.Errorf("SendPayout requires APITypePayouts")
	}

	if err := paymentRequest.Validate(); err != nil {
		return nil, fmt.Errorf("validating payment request: %w", err)
	}

	circleAssetCode, err := paymentRequest.GetCircleAssetCode()
	if err != nil {
		return nil, fmt.Errorf("getting Circle asset code: %w", err)
	}

	return s.PostPayout(ctx, PayoutRequest{
		IdempotencyKey: paymentRequest.IdempotencyKey,
		Source: TransferAccount{
			Type: TransferAccountTypeWallet,
			ID:   paymentRequest.SourceWalletID,
		},
		Destination: TransferAccount{
			Type:  TransferAccountTypeAddressBook,
			Chain: StellarChainCode,
			ID:    paymentRequest.RecipientID,
		},
		Amount: Balance{
			Amount:   paymentRequest.Amount,
			Currency: circleAssetCode,
		},
		ToAmount: ToAmount{Currency: circleAssetCode},
	})
}

func (s *Service) getClientForTenantInContext(ctx context.Context) (ClientInterface, error) {
	apiKey, err := s.ClientConfigModel.GetDecryptedAPIKey(ctx, s.EncryptionPassphrase)
	if err != nil {
		return nil, fmt.Errorf("retrieving decrypted Circle API key: %w", err)
	}
	return s.ClientFactory(ClientOptions{
		APIKey:         apiKey,
		NetworkType:    s.NetworkType,
		TenantManager:  s.TenantManager,
		MonitorService: s.MonitorService,
	}), nil
}

func (s *Service) Ping(ctx context.Context) (bool, error) {
	client, err := s.getClientForTenantInContext(ctx)
	if err != nil {
		return false, fmt.Errorf("cannot get Circle client: %w", err)
	}
	return client.Ping(ctx)
}

func (s *Service) PostTransfer(ctx context.Context, transferRequest TransferRequest) (*Transfer, error) {
	client, err := s.getClientForTenantInContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get Circle client: %w", err)
	}
	return client.PostTransfer(ctx, transferRequest)
}

func (s *Service) GetTransferByID(ctx context.Context, transferID string) (*Transfer, error) {
	client, err := s.getClientForTenantInContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get Circle client: %w", err)
	}
	return client.GetTransferByID(ctx, transferID)
}

func (s *Service) PostRecipient(ctx context.Context, recipientRequest RecipientRequest) (*Recipient, error) {
	client, err := s.getClientForTenantInContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get Circle client: %w", err)
	}
	return client.PostRecipient(ctx, recipientRequest)
}

func (s *Service) GetRecipientByID(ctx context.Context, recipientID string) (*Recipient, error) {
	client, err := s.getClientForTenantInContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get Circle client: %w", err)
	}
	return client.GetRecipientByID(ctx, recipientID)
}

func (s *Service) PostPayout(ctx context.Context, payoutRequest PayoutRequest) (*Payout, error) {
	client, err := s.getClientForTenantInContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get Circle client: %w", err)
	}
	return client.PostPayout(ctx, payoutRequest)
}

func (s *Service) GetPayoutByID(ctx context.Context, payoutID string) (*Payout, error) {
	client, err := s.getClientForTenantInContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get Circle client: %w", err)
	}
	return client.GetPayoutByID(ctx, payoutID)
}

func (s *Service) GetBusinessBalances(ctx context.Context) (*Balances, error) {
	client, err := s.getClientForTenantInContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get Circle client: %w", err)
	}
	return client.GetBusinessBalances(ctx)
}

func (s *Service) GetAccountConfiguration(ctx context.Context) (*AccountConfiguration, error) {
	client, err := s.getClientForTenantInContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get Circle client: %w", err)
	}
	return client.GetAccountConfiguration(ctx)
}
