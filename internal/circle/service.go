package circle

import (
	"context"
	"fmt"

	"github.com/stellar/go/strkey"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type Service struct {
	ClientFactory        ClientFactory
	ClientConfigModel    ClientConfigModelInterface
	NetworkType          utils.NetworkType
	EncryptionPassphrase string
}

const StellarChainCode = "XLM"

// ServiceInterface defines the interface for Circle related SDP operations.
//
//go:generate mockery --name=ServiceInterface --case=underscore --structname=MockService --output=. --filename=service_mock.go --inpackage
type ServiceInterface interface {
	ClientInterface
	SendPayment(ctx context.Context, paymentRequest PaymentRequest) (*Transfer, error)
}

var _ ServiceInterface = (*Service)(nil)

type ServiceOptions struct {
	ClientFactory        ClientFactory
	ClientConfigModel    ClientConfigModelInterface
	NetworkType          utils.NetworkType
	EncryptionPassphrase string
}

func (o ServiceOptions) Validate() error {
	if o.ClientFactory == nil {
		return fmt.Errorf("ClientFactory is required")
	}

	if o.ClientConfigModel == nil {
		return fmt.Errorf("ClientConfigModel is required")
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
	}, nil
}

func (s *Service) SendPayment(ctx context.Context, paymentRequest PaymentRequest) (*Transfer, error) {
	if err := paymentRequest.Validate(); err != nil {
		return nil, fmt.Errorf("validating payment request: %w", err)
	}

	circleAssetCode, err := paymentRequest.getCircleAssetCode()
	if err != nil {
		return nil, fmt.Errorf("getting Circle asset code: %w", err)
	}

	return s.PostTransfer(ctx, TransferRequest{
		IdempotencyKey: paymentRequest.IdempotencyKey,
		Amount: Money{
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

func (s *Service) getClient(ctx context.Context) (ClientInterface, error) {
	apiKey, err := s.ClientConfigModel.GetDecryptedAPIKey(ctx, s.EncryptionPassphrase)
	if err != nil {
		return nil, fmt.Errorf("retrieving decrypted Circle API key: %w", err)
	}
	return s.ClientFactory(s.NetworkType, apiKey), nil
}

func (s *Service) Ping(ctx context.Context) (bool, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return false, fmt.Errorf("cannot get Circle client: %w", err)
	}
	return client.Ping(ctx)
}

func (s *Service) PostTransfer(ctx context.Context, transferRequest TransferRequest) (*Transfer, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get Circle client: %w", err)
	}
	return client.PostTransfer(ctx, transferRequest)
}

func (s *Service) GetTransferByID(ctx context.Context, transferID string) (*Transfer, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get Circle client: %w", err)
	}
	return client.GetTransferByID(ctx, transferID)
}

func (s *Service) GetWalletByID(ctx context.Context, walletID string) (*Wallet, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get Circle client: %w", err)
	}
	return client.GetWalletByID(ctx, walletID)
}
