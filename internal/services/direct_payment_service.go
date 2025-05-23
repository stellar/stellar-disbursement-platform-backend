package services

import (
	"context"
	"fmt"
	"strconv"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// DirectPaymentRequest represents service-level request for creating direct payment
type DirectPaymentRequest struct {
	AssetRef          AssetReference
	ReceiverRef       ReceiverReference
	WalletRef         WalletReference
	Amount            string
	ExternalPaymentID *string
}

type DirectPaymentService struct {
	Models                     *data.Models
	DBConnectionPool           db.DBConnectionPool
	EventProducer              events.Producer
	DistributionAccountService DistributionAccountServiceInterface
	Resolvers                  *ResolverFactory
}

// NewDirectPaymentService creates a new DirectPaymentService with resolvers
func NewDirectPaymentService(models *data.Models, dbConnectionPool db.DBConnectionPool) *DirectPaymentService {
	return &DirectPaymentService{
		Models:           models,
		DBConnectionPool: dbConnectionPool,
		Resolvers:        NewResolverFactory(models),
	}
}

// ResolvedPaymentComponents holds the resolved components for creating a direct payment
type ResolvedPaymentComponents struct {
	Asset          *data.Asset
	Receiver       *data.Receiver
	ReceiverWallet *data.ReceiverWallet
}

// CreateDirectPayment creates a new direct payment
func (s *DirectPaymentService) CreateDirectPayment(
	ctx context.Context,
	req *DirectPaymentRequest,
	user *auth.User,
	distributionAccount *schema.TransactionAccount,
) (*data.Payment, error) {
	return db.RunInTransactionWithResult(ctx, s.DBConnectionPool, nil, func(tx db.DBTransaction) (*data.Payment, error) {
		// 1. Resolve all components
		components, err := s.resolvePaymentComponents(ctx, tx, req)
		if err != nil {
			return nil, fmt.Errorf("resolving payment components: %w", err)
		}

		if err := s.validateBalanceForPayment(ctx, distributionAccount, components.Asset, req.Amount); err != nil {
			return nil, err
		}

		// 2. Create the direct payment using data model
		directPaymentInsert := data.DirectPaymentInsert{
			ReceiverID:        components.Receiver.ID,
			Amount:            req.Amount,
			AssetID:           components.Asset.ID,
			ReceiverWalletID:  components.ReceiverWallet.ID,
			ExternalPaymentID: req.ExternalPaymentID,
		}

		paymentID, err := s.Models.DirectPayment.Insert(ctx, tx, directPaymentInsert)
		if err != nil {
			return nil, fmt.Errorf("inserting direct payment: %w", err)
		}

		// 3. Get the created payment with all relations
		payment, err := s.Models.Payment.Get(ctx, paymentID, tx)
		if err != nil {
			return nil, fmt.Errorf("getting created payment: %w", err)
		}

		return payment, nil
	})
}

// resolvePaymentComponents resolves asset, receiver, and receiver wallet from the request
func (s *DirectPaymentService) resolvePaymentComponents(ctx context.Context, sqlExec db.SQLExecuter, req *DirectPaymentRequest) (*ResolvedPaymentComponents, error) {
	asset, err := s.Resolvers.Asset().Resolve(ctx, sqlExec, req.AssetRef)
	if err != nil {
		return nil, fmt.Errorf("resolving asset: %w", err)
	}

	receiver, err := s.Resolvers.Receiver().Resolve(ctx, sqlExec, req.ReceiverRef)
	if err != nil {
		return nil, fmt.Errorf("resolving receiver: %w", err)
	}

	wallet, err := s.Resolvers.Wallet().Resolve(ctx, sqlExec, req.WalletRef)
	if err != nil {
		return nil, fmt.Errorf("resolving wallet: %w", err)
	}

	receiverWallet, err := s.getReceiverWallet(ctx, sqlExec, receiver.ID, wallet.ID, req.WalletRef)
	if err != nil {
		return nil, fmt.Errorf("getting or creating receiver wallet: %w", err)
	}

	return &ResolvedPaymentComponents{
		Asset:          asset,
		Receiver:       receiver,
		ReceiverWallet: receiverWallet,
	}, nil
}

func (s *DirectPaymentService) getReceiverWallet(ctx context.Context, sqlExec db.SQLExecuter, receiverID, walletID string, walletRef WalletReference) (*data.ReceiverWallet, error) {
	existingReceiverWallets, err := s.Models.ReceiverWallet.GetByReceiverIDsAndWalletID(ctx, sqlExec, []string{receiverID}, walletID)
	if err != nil {
		return nil, fmt.Errorf("checking existing receiver wallets: %w", err)
	}

	if len(existingReceiverWallets) > 0 {
		rw := existingReceiverWallets[0]

		if walletRef.ID != nil {
			// Wallet by ID rules
			switch rw.Status {
			case data.RegisteredReceiversWalletStatus:
				// Issue payment directly to that wallet
				return rw, nil
			case data.ReadyReceiversWalletStatus:
				// Send a new invitation (for now, just proceed with payment)
				return rw, nil
			default:
				return nil, fmt.Errorf("receiver wallet in unexpected status: %s", rw.Status)
			}
		} else if walletRef.Address != nil {
			// Wallet by address rules (UserManagedWallet)
			// TODO: Check if it has the same address as requested
			return rw, nil
		}

		return rw, nil
	}

	// Create new receiver wallet if it doesn't exist
	receiverWalletInsert := data.ReceiverWalletInsert{
		ReceiverID: receiverID,
		WalletID:   walletID,
	}

	newReceiverWalletID, err := s.Models.ReceiverWallet.Insert(ctx, sqlExec, receiverWalletInsert)
	if err != nil {
		return nil, fmt.Errorf("creating receiver wallet: %w", err)
	}

	// Get the created receiver wallet
	receiverWallet, err := s.Models.ReceiverWallet.GetByID(ctx, sqlExec, newReceiverWalletID)
	if err != nil {
		return nil, fmt.Errorf("getting created receiver wallet: %w", err)
	}

	return receiverWallet, nil
}

func (s *DirectPaymentService) validateBalanceForPayment(
	ctx context.Context,
	distributionAccount *schema.TransactionAccount,
	asset *data.Asset,
	amount string,
) error {
	availableBalance, err := s.DistributionAccountService.GetBalance(ctx, distributionAccount, *asset)
	if err != nil {
		return fmt.Errorf("getting balance: %w", err)
	}

	totalPendingAmount, err := strconv.ParseFloat(amount, 64)
	if err != nil {
		return fmt.Errorf("parsing amount: %w", err)
	}

	if (availableBalance - totalPendingAmount) < 0 {
		return InsufficientBalanceError{
			DistributionAddress: distributionAccount.ID(),
			DisbursementID:      "direct-payment",
			DisbursementAsset:   *asset,
			AvailableBalance:    availableBalance,
			DisbursementAmount:  totalPendingAmount,
			TotalPendingAmount:  totalPendingAmount,
		}
	}

	return nil
}
