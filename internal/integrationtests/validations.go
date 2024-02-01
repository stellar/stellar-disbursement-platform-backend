package integrationtests

import (
	"context"
	"fmt"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
)

func validateExpectationsAfterProcessDisbursement(ctx context.Context, disbursementID string, models *data.Models, sqlExec db.SQLExecuter) error {
	disbursement, err := models.Disbursements.Get(ctx, sqlExec, disbursementID)
	if err != nil {
		return fmt.Errorf("error getting disbursement: %w", err)
	}

	if disbursement.Status != data.ReadyDisbursementStatus {
		return fmt.Errorf("invalid status for disbursement after process disbursement")
	}

	receivers, err := models.DisbursementReceivers.GetAll(ctx, sqlExec, &data.QueryParams{}, disbursementID)
	if err != nil {
		return fmt.Errorf("error getting receivers from disbursement: %w", err)
	}

	if len(receivers) <= 0 {
		return fmt.Errorf("error getting receivers from disbursement: receivers not found")
	}
	receiver := receivers[0]

	// TODO upgrade this function to validate multiples receiver wallets and payments.
	if receiver.ReceiverWallet.Status != data.DraftReceiversWalletStatus {
		return fmt.Errorf("invalid status for receiver_wallet after process disbursement")
	}

	if receiver.Payment.Status != data.DraftPaymentStatus {
		return fmt.Errorf("invalid status for payment after process disbursement")
	}

	return nil
}

func validateExpectationsAfterStartDisbursement(ctx context.Context, disbursementID string, models *data.Models, sqlExec db.SQLExecuter) error {
	disbursement, err := models.Disbursements.Get(ctx, sqlExec, disbursementID)
	if err != nil {
		return fmt.Errorf("error getting disbursement: %w", err)
	}

	if disbursement.Status != data.StartedDisbursementStatus {
		return fmt.Errorf("invalid status for disbursement after start disbursement")
	}

	receivers, err := models.DisbursementReceivers.GetAll(ctx, sqlExec, &data.QueryParams{}, disbursementID)
	if err != nil {
		return fmt.Errorf("error getting receivers from disbursement: %w", err)
	}
	if len(receivers) <= 0 {
		return fmt.Errorf("error getting receivers from disbursement: receivers not found")
	}

	receiver := receivers[0]

	// TODO upgrade this function to validate multiples receiver wallets and payments.
	if receiver.ReceiverWallet.Status != data.ReadyReceiversWalletStatus {
		return fmt.Errorf("invalid status for receiver_wallet after start disbursement")
	}

	if receiver.Payment.Status != data.ReadyPaymentStatus {
		return fmt.Errorf("invalid status for payment after start disbursement")
	}

	return nil
}

func validateExpectationsAfterReceiverRegistration(ctx context.Context, models *data.Models, stellarAccount, stellarMemo, clientDomain string) error {
	receiverWallet, err := models.ReceiverWallet.GetByStellarAccountAndMemo(ctx, stellarAccount, stellarMemo, clientDomain)
	if err != nil {
		return fmt.Errorf("error getting receiver wallet with stellar account: %w", err)
	}

	if receiverWallet.Status != data.RegisteredReceiversWalletStatus {
		return fmt.Errorf("invalid status for receiver_wallet after receiver registration")
	}

	return nil
}

func validateStellarTransaction(paymentHorizon *PaymentHorizon, receiverAccount, disbursedAssetCode, disbursedAssetIssuer, amount string) error {
	if !paymentHorizon.TransactionSuccessful {
		return fmt.Errorf("transaction was not successful on horizon network")
	}

	if paymentHorizon.ReceiverAccount != receiverAccount {
		return fmt.Errorf("transaction sent to wrong receiver account")
	}

	if paymentHorizon.Amount != amount {
		return fmt.Errorf("transaction with wrong amount")
	}

	if paymentHorizon.AssetCode != disbursedAssetCode || paymentHorizon.AssetIssuer != disbursedAssetIssuer {
		return fmt.Errorf("transaction with wrong disbursed asset")
	}

	return nil
}
