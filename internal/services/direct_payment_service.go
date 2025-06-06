package services

import (
	"context"
	"fmt"
	"strconv"

	"github.com/stellar/go/support/log"

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

type WalletNotEnabledError struct {
	WalletName string
}

func (e WalletNotEnabledError) Error() string {
	return fmt.Sprintf("wallet '%s' is not enabled for payments", e.WalletName)
}

type AssetNotSupportedByWalletError struct {
	AssetCode  string
	WalletName string
}

func (e AssetNotSupportedByWalletError) Error() string {
	return fmt.Sprintf("asset '%s' is not supported by wallet '%s'", e.AssetCode, e.WalletName)
}

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

type DirectPaymentService struct {
	Models                     *data.Models
	EventProducer              events.Producer
	DistributionAccountService DistributionAccountServiceInterface
	Resolvers                  *ResolverFactory
}

// NewDirectPaymentService creates a new DirectPaymentService with resolvers
func NewDirectPaymentService(models *data.Models, eventProducer events.Producer, distributionAccount DistributionAccountServiceInterface) *DirectPaymentService {
	return &DirectPaymentService{
		Models:                     models,
		EventProducer:              eventProducer,
		DistributionAccountService: distributionAccount,
		Resolvers:                  NewResolverFactory(models),
	}
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
				return nil, err
			}

			receiver, err := s.Resolvers.Receiver().Resolve(ctx, dbTx, req.Receiver)
			if err != nil {
				return nil, err
			}

			wallet, err := s.Resolvers.Wallet().Resolve(ctx, dbTx, req.Wallet)
			if err != nil {
				return nil, err
			}

			// 2. Validate wallet is enabled
			if !wallet.Enabled {
				return nil, WalletNotEnabledError{WalletName: wallet.Name}
			}

			// 3. Validate asset is supported by wallet
			if err = s.validateAssetWalletCompatibility(ctx, asset, wallet); err != nil {
				return nil, err
			}

			// 4. Get receiver wallet
			receiverWallet, err := s.getReceiverWallet(ctx, dbTx, receiver.ID, wallet.ID, req.Wallet.Address)
			if err != nil {
				return nil, fmt.Errorf("getting receiver wallet: %w", err)
			}

			// 5. Validate balance
			if err = s.validateBalance(ctx, dbTx, distributionAccount, asset, req.Amount); err != nil {
				return nil, err
			}

			// 6. Create payment
			paymentInsert := data.PaymentInsert{
				ReceiverID:        receiver.ID,
				Amount:            req.Amount,
				AssetID:           asset.ID,
				ReceiverWalletID:  receiverWallet.ID,
				ExternalPaymentID: req.ExternalPaymentID,
				PaymentType:       data.PaymentTypeDirect,
			}

			paymentID, err := s.Models.Payment.CreateDirectPayment(ctx, dbTx, paymentInsert)
			if err != nil {
				return nil, fmt.Errorf("creating payment: %w", err)
			}

			// 7. Update payment status based on receiver wallet status
			if receiverWallet.Status == data.RegisteredReceiversWalletStatus {
				err = s.Models.Payment.UpdateStatus(ctx, dbTx, paymentID, data.ReadyPaymentStatus, nil, "")
				if err != nil {
					return nil, fmt.Errorf("updating payment status to ready: %w", err)
				}
			}

			// 8. Get the created payment
			payment, err = s.Models.Payment.Get(ctx, paymentID, dbTx)
			if err != nil {
				return nil, fmt.Errorf("getting created payment: %w", err)
			}

			// disbursment is an empty struct for the direct payments, set it to nil
			payment.Disbursement = nil

			// 9. Prepare post-commit events (same as before)
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

	if err := db.RunInTransactionWithPostCommit(ctx, &opts); err != nil {
		return nil, err
	}

	return payment, nil
}

func (s *DirectPaymentService) validateAssetWalletCompatibility(ctx context.Context, asset *data.Asset, wallet *data.Wallet) error {
	if wallet.UserManaged {
		return nil
	}

	walletAssets, err := s.Models.Wallets.GetAssets(ctx, wallet.ID)
	if err != nil {
		return fmt.Errorf("getting wallet assets: %w", err)
	}

	for _, walletAsset := range walletAssets {
		if walletAsset.Equals(*asset) {
			return nil
		}
	}

	return AssetNotSupportedByWalletError{
		AssetCode:  asset.Code,
		WalletName: wallet.Name,
	}
}

func (s *DirectPaymentService) getReceiverWallet(
	ctx context.Context,
	dbTx db.DBTransaction,
	receiverID, walletID string,
	walletAddress *string,
) (*data.ReceiverWallet, error) {
	// Check if receiver wallet exists
	receiverWallets, err := s.Models.ReceiverWallet.GetByReceiverIDsAndWalletID(
		ctx, dbTx, []string{receiverID}, walletID)
	if err != nil {
		return nil, fmt.Errorf("checking for existing receiver wallet: %w", err)
	}

	if len(receiverWallets) == 0 {
		return nil, fmt.Errorf("receiver wallet not found - receiver must be registered with this wallet to receive direct payments")
	}

	receiverWallet := receiverWallets[0]

	if walletAddress != nil && *walletAddress != "" {
		if receiverWallet.StellarAddress != *walletAddress {
			return nil, fmt.Errorf("wallet address mismatch - receiver is registered with a different address for this wallet")
		}
	}

	return receiverWallet, nil
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

	// Skip balance validation for user-managed wallets since they manage their own balance
	if distributionAccount.Type != schema.DistributionAccountStellarDBVault {
		return nil
	}

	// Get available balance for vault-managed accounts
	availableBalance, err := s.DistributionAccountService.GetBalance(ctx, distributionAccount, *asset)
	if err != nil {
		return fmt.Errorf("getting balance: %w", err)
	}

	// Calculate total pending amount for this specific asset
	totalPending, err := s.calculatePendingAmountForAsset(ctx, dbTx, *asset)
	if err != nil {
		return fmt.Errorf("calculating pending amounts: %w", err)
	}

	// Check if we have sufficient balance
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

func (s *DirectPaymentService) calculatePendingAmountForAsset(
	ctx context.Context,
	dbTx db.DBTransaction,
	targetAsset data.Asset,
) (float64, error) {
	pendingPayments, err := s.Models.Payment.GetAll(ctx, &data.QueryParams{
		Filters: map[data.FilterKey]any{
			data.FilterKeyStatus: data.PaymentInProgressStatuses(),
		},
	}, dbTx, data.QueryTypeSelectAll)
	if err != nil {
		return 0, fmt.Errorf("getting pending payments: %w", err)
	}

	totalPending := 0.0
	for _, payment := range pendingPayments {
		if payment.Asset.Equals(targetAsset) {
			amount, parseErr := strconv.ParseFloat(payment.Amount, 64)
			if parseErr != nil {
				log.Ctx(ctx).Warnf("Failed to parse payment amount %s for payment %s: %v",
					payment.Amount, payment.ID, parseErr)
				continue
			}
			totalPending += amount
		}
	}

	return totalPending, nil
}
