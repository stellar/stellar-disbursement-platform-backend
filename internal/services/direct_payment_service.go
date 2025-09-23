package services

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// CreateDirectPaymentRequest represents service-level request for creating direct payment
type CreateDirectPaymentRequest struct {
	Amount            string            `json:"amount"`
	Asset             AssetReference    `json:"asset" validate:"required"`
	Receiver          ReceiverReference `json:"receiver" validate:"required"`
	Wallet            WalletReference   `json:"wallet" validate:"required"`
	ExternalPaymentID *string           `json:"external_payment_id,omitempty"`
}

type TrustlineNotFoundError struct {
	Asset               data.Asset
	DistributionAccount string
}

func (e TrustlineNotFoundError) Error() string {
	return fmt.Sprintf("distribution account %s does not have a trustline for asset %s:%s",
		e.DistributionAccount, e.Asset.Code, e.Asset.Issuer)
}

type AccountNotFoundError struct {
	Address string
}

func (e AccountNotFoundError) Error() string {
	return fmt.Sprintf("distribution account %s not found on the Stellar network", e.Address)
}

type CircleAccountNotActivatedError struct {
	AccountType string
	Status      string
}

func (e CircleAccountNotActivatedError) Error() string {
	return fmt.Sprintf("Circle distribution account is in %s state, please complete the %s activation process",
		e.Status, e.AccountType)
}

type CircleAssetNotSupportedError struct {
	Asset data.Asset
}

func (e CircleAssetNotSupportedError) Error() string {
	return fmt.Sprintf("asset %s is not supported by Circle for this distribution account", e.Asset.Code)
}

type WalletNotEnabledError struct {
	WalletName string
}

func (e WalletNotEnabledError) Error() string {
	return fmt.Sprintf("wallet '%s' is not enabled for payments", e.WalletName)
}

type ReceiverWalletNotFoundError struct {
	ReceiverID string
	WalletID   string
}

func (e ReceiverWalletNotFoundError) Error() string {
	return fmt.Sprintf("no receiver wallet: receiver=%s wallet=%s", e.ReceiverID, e.WalletID)
}

type ReceiverWalletNotReadyForPaymentError struct {
	CurrentStatus data.ReceiversWalletStatus
}

func (e ReceiverWalletNotReadyForPaymentError) Error() string {
	return fmt.Sprintf("receiver wallet is not ready for payment, current status is %s", e.CurrentStatus)
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
		"insufficient balance for direct payment: requested %.6f %s, but only %.6f available (%.6f in pending payments). Need %.6f more %s",
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
	SubmitterEngine            engine.SubmitterEngine
}

func NewDirectPaymentService(
	models *data.Models,
	eventProducer events.Producer,
	distributionAccount DistributionAccountServiceInterface,
	submitterEngine engine.SubmitterEngine,
) *DirectPaymentService {
	return &DirectPaymentService{
		Models:                     models,
		EventProducer:              eventProducer,
		DistributionAccountService: distributionAccount,
		Resolvers:                  NewResolverFactory(models),
		SubmitterEngine:            submitterEngine,
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

			// 4. Get and validate receiver wallet
			receiverWallet, err := s.getReceiverWallet(ctx, dbTx, receiver.ID, wallet.ID, req.Wallet.Address)
			if err != nil {
				return nil, fmt.Errorf("getting receiver wallet: %w", err)
			}
			if receiverWallet.Status != data.ReadyReceiversWalletStatus && receiverWallet.Status != data.RegisteredReceiversWalletStatus {
				return nil, ReceiverWalletNotReadyForPaymentError{CurrentStatus: receiverWallet.Status}
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

			paymentID, err := s.Models.Payment.CreateDirectPayment(ctx, dbTx, paymentInsert, user.ID)
			if err != nil {
				return nil, fmt.Errorf("creating payment: %w", err)
			}

			// 7. Get the created payment
			payment, err = s.Models.Payment.Get(ctx, paymentID, dbTx)
			if err != nil {
				return nil, fmt.Errorf("getting created payment: %w", err)
			}

			// 8. Prepare post-commit events (same as before)
			msgs := make([]*events.Message, 0)

			// Send payment for processing if ready
			if receiverWallet.Status == data.RegisteredReceiversWalletStatus {
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
		return nil, fmt.Errorf("creating direct payment: %w", err)
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

	// If receiver wallet exists, return it
	if len(receiverWallets) > 0 {
		receiverWallet := receiverWallets[0]

		if walletAddress != nil && *walletAddress != "" {
			if receiverWallet.StellarAddress != *walletAddress {
				return nil, fmt.Errorf("wallet address mismatch - receiver is registered with a different address for this wallet")
			}
		}

		return receiverWallet, nil
	}

	// No receiver wallet exists - check if this is a SEP-24 wallet and receiver has verifications
	wallet, err := s.Models.Wallets.Get(ctx, walletID)
	if err != nil {
		return nil, fmt.Errorf("getting wallet: %w", err)
	}

	if wallet.UserManaged {
		return nil, &ReceiverWalletNotFoundError{
			ReceiverID: receiverID,
			WalletID:   walletID,
		}
	}

	// Check if receiver has any verifications
	receiverVerifications, err := s.Models.ReceiverVerification.GetAllByReceiverId(ctx, dbTx, receiverID)
	if err != nil {
		return nil, fmt.Errorf("checking receiver verifications: %w", err)
	}

	if len(receiverVerifications) == 0 {
		return nil, &ReceiverWalletNotFoundError{
			ReceiverID: receiverID,
			WalletID:   walletID,
		}
	}

	rwInsert := data.ReceiverWalletInsert{
		ReceiverID: receiverID,
		WalletID:   walletID,
	}

	newReceiverWalletID, err := s.Models.ReceiverWallet.GetOrInsertReceiverWallet(ctx, dbTx, rwInsert)
	if err != nil {
		return nil, fmt.Errorf("creating receiver wallet: %w", err)
	}

	// Update the status to READY using the data package method
	rwUpdate := data.ReceiverWalletUpdate{
		Status: data.ReadyReceiversWalletStatus,
	}

	err = s.Models.ReceiverWallet.Update(ctx, newReceiverWalletID, rwUpdate, dbTx)
	if err != nil {
		return nil, fmt.Errorf("updating receiver wallet status to READY: %w", err)
	}

	createdReceiverWallet, err := s.Models.ReceiverWallet.GetByID(ctx, dbTx, newReceiverWalletID)
	if err != nil {
		return nil, fmt.Errorf("getting created receiver wallet: %w", err)
	}

	// Dispatch invitation event for the newly created wallet
	// This will trigger the invitation flow to get the receiver onboarded
	if s.EventProducer != nil {
		tenant, tenantErr := sdpcontext.GetTenantFromContext(ctx)
		if tenantErr != nil {
			log.Ctx(ctx).Errorf("failed to get tenant from context for invitation event: %v", tenantErr)
		} else {
			eventData := schemas.EventReceiverWalletInvitationData{
				ReceiverWalletID: newReceiverWalletID,
			}

			msg := events.Message{
				Topic:    events.ReceiverWalletNewInvitationTopic,
				Key:      newReceiverWalletID,
				TenantID: tenant.ID,
				Type:     events.BatchReceiverWalletInvitationType,
				Data:     eventData,
			}

			writeErr := s.EventProducer.WriteMessages(ctx, msg)
			if writeErr != nil {
				log.Ctx(ctx).Errorf("failed to dispatch receiver wallet invitation event: %v", writeErr)
			}
		}
	}

	return createdReceiverWallet, nil
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

	// Step 1a: Stellar account validation
	if distributionAccount.IsStellar() {
		var exists bool
		exists, err = s.checkTrustlineExists(distributionAccount, *asset)
		if err != nil {
			return fmt.Errorf("checking trustline existence: %w", err)
		}
		if !exists {
			return TrustlineNotFoundError{
				Asset:               *asset,
				DistributionAccount: distributionAccount.Address,
			}
		}
	}

	// Step 1b: Circle account validation
	if distributionAccount.IsCircle() {
		if err = s.validateCircleAccount(distributionAccount); err != nil {
			return err
		}
	}

	// Step 2: Get balance
	availableBalance, err := s.DistributionAccountService.GetBalance(ctx, distributionAccount, *asset)
	if err != nil {
		return fmt.Errorf("getting balance: %w", err)
	}

	// Step 3: Calculate pending amounts and validate sufficient balance
	totalPending, err := s.calculatePendingAmountForAsset(ctx, dbTx, *asset)
	if err != nil {
		return fmt.Errorf("calculating pending amounts: %w", err)
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

func (s *DirectPaymentService) checkTrustlineExists(
	account *schema.TransactionAccount,
	asset data.Asset,
) (bool, error) {
	if asset.IsNative() {
		return true, nil
	}

	client := s.SubmitterEngine.HorizonClient

	acc, err := client.AccountDetail(horizonclient.AccountRequest{
		AccountID: account.Address,
	})
	if err != nil {
		var horizonErr *horizonclient.Error
		if errors.As(err, &horizonErr) {
			if horizonErr.Response.StatusCode == 404 {
				return false, AccountNotFoundError{Address: account.Address}
			}
		}
		return false, fmt.Errorf("getting account details from Horizon: %w", err)
	}

	for _, balance := range acc.Balances {
		if balance.Asset.Type == validators.AssetTypeNative {
			continue
		}

		if balance.Asset.Code == asset.Code && balance.Asset.Issuer == asset.Issuer {
			return true, nil
		}
	}
	return false, nil
}

func (s *DirectPaymentService) validateCircleAccount(
	account *schema.TransactionAccount,
) error {
	if account.Status == schema.AccountStatusPendingUserActivation {
		return CircleAccountNotActivatedError{AccountType: string(account.Type), Status: string(account.Status)}
	}
	return nil
}
