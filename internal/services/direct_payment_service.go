package services

import (
	"context"
	"fmt"
	"strconv"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// DirectPaymentRequest represents service-level request for creating direct payment
type CreateDirectPaymentRequest struct {
	Amount            string            `json:"amount" validate:"required"`
	Asset             AssetReference    `json:"asset" validate:"required"`
	Receiver          ReceiverReference `json:"receiver" validate:"required"`
	Wallet            WalletReference   `json:"wallet" validate:"required"`
	ExternalPaymentID *string           `json:"external_payment_id,omitempty"`
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

func (s *DirectPaymentService) CreateDirectPayment(
	ctx context.Context,
	req CreateDirectPaymentRequest,
	user *auth.User,
	distributionAccount *schema.TransactionAccount,
) (*data.Payment, error) {
	var payment *data.Payment

	opts := db.TransactionOptions{
		DBConnectionPool: s.Models.DBConnectionPool,
		AtomicFunctionWithPostCommit: func(dbTx db.DBTransaction) (postCommitFn db.PostCommitFunction, err error) {
			// 1. Resolve entities
			asset, err := s.Resolvers.Asset().Resolve(ctx, dbTx, req.Asset)
			if err != nil {
				return nil, fmt.Errorf("resolving asset: %w", err)
			}

			receiver, err := s.Resolvers.Receiver().Resolve(ctx, dbTx, req.Receiver)
			if err != nil {
				return nil, fmt.Errorf("resolving receiver: %w", err)
			}

			wallet, err := s.Resolvers.Wallet().Resolve(ctx, dbTx, req.Wallet)
			if err != nil {
				return nil, fmt.Errorf("resolving wallet: %w", err)
			}

			// 2. Validate wallet is enabled
			if !wallet.Enabled {
				return nil, fmt.Errorf("wallet %s is not enabled", wallet.Name)
			}

			// 3. Get or create receiver wallet
			receiverWallet, err := s.getOrCreateReceiverWallet(ctx, dbTx, receiver.ID, wallet.ID, req.Wallet.Address)
			if err != nil {
				return nil, fmt.Errorf("getting or creating receiver wallet: %w", err)
			}

			// 4. Validate balance
			err = s.validateBalance(ctx, dbTx, distributionAccount, asset, req.Amount)
			if err != nil {
				return nil, err
			}

			// 5. Create payment
			paymentInsert := data.PaymentInsert{
				ReceiverID:        receiver.ID,
				Amount:            req.Amount,
				AssetID:           asset.ID,
				ReceiverWalletID:  receiverWallet.ID,
				ExternalPaymentID: req.ExternalPaymentID,
			}

			paymentID, err := s.Models.Payment.CreateDirectPayment(ctx, dbTx, paymentInsert)
			if err != nil {
				return nil, fmt.Errorf("creating payment: %w", err)
			}

			// 6. Update payment status based on receiver wallet status
			if receiverWallet.Status == data.RegisteredReceiversWalletStatus {
				err = s.Models.Payment.UpdateStatus(ctx, dbTx, paymentID, data.ReadyPaymentStatus, nil, "")
				if err != nil {
					return nil, fmt.Errorf("updating payment status to ready: %w", err)
				}
			}

			// 7. Get the created payment
			payment, err = s.Models.Payment.Get(ctx, paymentID, dbTx)
			if err != nil {
				return nil, fmt.Errorf("getting created payment: %w", err)
			}

			// 8. Prepare post-commit events
			msgs := make([]*events.Message, 0)

			// Send invitation if receiver wallet needs registration
			if receiverWallet.Status == data.ReadyReceiversWalletStatus {
				eventData := []schemas.EventReceiverWalletInvitationData{
					{ReceiverWalletID: receiverWallet.ID},
				}

				inviteMsg, err := events.NewMessage(ctx, events.ReceiverWalletNewInvitationTopic,
					paymentID, events.BatchReceiverWalletInvitationType, eventData)
				if err != nil {
					return nil, fmt.Errorf("creating invitation message: %w", err)
				}
				msgs = append(msgs, inviteMsg)
			}

			// Send payment for processing if ready
			if payment.Status == data.ReadyPaymentStatus {
				paymentMsg, err := events.NewPaymentReadyToPayMessage(ctx,
					distributionAccount.Type.Platform(), paymentID, events.PaymentReadyToPayDirectPayment)
				if err != nil {
					return nil, fmt.Errorf("creating payment message: %w", err)
				}

				paymentData := schemas.EventPaymentsReadyToPayData{
					TenantID: paymentMsg.TenantID,
					Payments: []schemas.PaymentReadyToPay{{ID: payment.ID}},
				}
				paymentMsg.Data = paymentData
				msgs = append(msgs, paymentMsg)
			}

			if len(msgs) > 0 {
				postCommitFn = func() error {
					return events.ProduceEvents(ctx, s.EventProducer, msgs...)
				}
			}

			return postCommitFn, nil
		},
	}

	err := db.RunInTransactionWithPostCommit(ctx, &opts)
	if err != nil {
		return nil, err
	}

	return payment, nil
}

func (s *DirectPaymentService) getOrCreateReceiverWallet(
	ctx context.Context,
	dbTx db.DBTransaction,
	receiverID, walletID string,
	walletAddress *string,
) (*data.ReceiverWallet, error) {
	// Check if receiver wallet exists
	receiverWallets, err := s.Models.ReceiverWallet.GetByReceiverIDsAndWalletID(
		ctx, dbTx, []string{receiverID}, walletID)
	if err != nil {
		return nil, err
	}

	if len(receiverWallets) > 0 {
		return receiverWallets[0], nil
	}

	// Create new receiver wallet
	newID, err := s.Models.ReceiverWallet.Insert(ctx, dbTx, data.ReceiverWalletInsert{
		ReceiverID: receiverID,
		WalletID:   walletID,
	})
	if err != nil {
		return nil, err
	}

	// If wallet address provided (user-managed wallet), update it
	if walletAddress != nil && *walletAddress != "" {
		err = s.Models.ReceiverWallet.Update(ctx, newID, data.ReceiverWalletUpdate{
			Status:         data.RegisteredReceiversWalletStatus,
			StellarAddress: *walletAddress,
		}, dbTx)
		if err != nil {
			return nil, err
		}
	}

	return s.Models.ReceiverWallet.GetByID(ctx, dbTx, newID)
}

func (s *DirectPaymentService) validateBalance(
	ctx context.Context,
	dbTx db.DBTransaction,
	distributionAccount *schema.TransactionAccount,
	asset *data.Asset,
	amount string,
) error {
	amountFloat, err := strconv.ParseFloat(amount, 64)
	if err != nil {
		return fmt.Errorf("parsing amount: %w", err)
	}

	availableBalance, err := s.DistributionAccountService.GetBalance(ctx, distributionAccount, *asset)
	if err != nil {
		return fmt.Errorf("getting balance: %w", err)
	}

	// Calculate pending payments for this asset
	totalPending := 0.0
	pendingPayments, err := s.Models.Payment.GetAll(ctx, &data.QueryParams{
		Filters: map[data.FilterKey]interface{}{
			data.FilterKeyStatus: data.PaymentInProgressStatuses(),
		},
	}, dbTx, data.QueryTypeSelectAll)
	if err != nil {
		return fmt.Errorf("getting pending payments: %w", err)
	}

	for _, p := range pendingPayments {
		if p.Asset.Equals(*asset) {
			pAmount, _ := strconv.ParseFloat(p.Amount, 64)
			totalPending += pAmount
		}
	}

	if availableBalance < (amountFloat + totalPending) {
		return InsufficientBalanceForDirectPaymentError{
			Asset:              *asset,
			RequestedAmount:    amountFloat,
			AvailableBalance:   availableBalance,
			TotalPendingAmount: totalPending,
		}
	}

	return nil
}

// InsufficientBalanceForDirectPaymentError represents insufficient balance for direct payment
type InsufficientBalanceForDirectPaymentError struct {
	Asset              data.Asset
	RequestedAmount    float64
	AvailableBalance   float64
	TotalPendingAmount float64
}

func (e InsufficientBalanceForDirectPaymentError) Error() string {
	shortfall := (e.RequestedAmount + e.TotalPendingAmount) - e.AvailableBalance
	return fmt.Sprintf(
		"insufficient balance for direct payment: requested %.2f %s, but only %.2f available (%.2f in pending payments). Need %.2f more %s",
		e.RequestedAmount,
		e.Asset.Code,
		e.AvailableBalance,
		e.TotalPendingAmount,
		shortfall,
		e.Asset.Code,
	)
}
