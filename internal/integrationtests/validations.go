package integrationtests

import (
	"context"
	"fmt"

	"github.com/stellar/go-stellar-sdk/protocols/horizon/operations"
	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
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

	for _, receiver := range receivers {
		// Validate receiver_wallet status
		expectedStatusByRegistrationContactType := map[data.RegistrationContactType]data.ReceiversWalletStatus{
			data.RegistrationContactTypePhone:                 data.DraftReceiversWalletStatus,
			data.RegistrationContactTypeEmail:                 data.DraftReceiversWalletStatus,
			data.RegistrationContactTypePhoneAndWalletAddress: data.RegisteredReceiversWalletStatus,
			data.RegistrationContactTypeEmailAndWalletAddress: data.RegisteredReceiversWalletStatus,
		}
		if expectedStatus := expectedStatusByRegistrationContactType[disbursement.RegistrationContactType]; expectedStatus != receiver.ReceiverWallet.Status {
			return fmt.Errorf("receiver_wallet should be in %s status for registrationContactType %s", expectedStatus, disbursement.RegistrationContactType)
		}

		// Validate payment status
		if receiver.Payment.Status != data.DraftPaymentStatus {
			return fmt.Errorf("invalid status for payment after process disbursement")
		}
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

	for _, receiver := range receivers {
		// Validate receiver_wallet status
		expectedStatusByRegistrationContactType := map[data.RegistrationContactType]data.ReceiversWalletStatus{
			data.RegistrationContactTypePhone:                 data.ReadyReceiversWalletStatus,
			data.RegistrationContactTypeEmail:                 data.ReadyReceiversWalletStatus,
			data.RegistrationContactTypePhoneAndWalletAddress: data.RegisteredReceiversWalletStatus,
			data.RegistrationContactTypeEmailAndWalletAddress: data.RegisteredReceiversWalletStatus,
		}
		if expectedStatus := expectedStatusByRegistrationContactType[disbursement.RegistrationContactType]; expectedStatus != receiver.ReceiverWallet.Status {
			return fmt.Errorf("receiver_wallet should be in %s status for registrationContactType %s", expectedStatus, disbursement.RegistrationContactType)
		}

		// Validate payment status
		if receiver.Payment.Status != data.ReadyPaymentStatus {
			return fmt.Errorf("invalid status for payment after start disbursement")
		}
	}

	return nil
}

func validateExpectationsAfterReceiverRegistration(ctx context.Context, models *data.Models, stellarAccount, stellarMemo, clientDomain string) error {
	receiverWallet, err := models.ReceiverWallet.GetByStellarAccountAndMemo(ctx, stellarAccount, clientDomain, &stellarMemo)
	if err != nil {
		return fmt.Errorf("error getting receiver wallet with stellar account: %w", err)
	}

	if receiverWallet.Status != data.RegisteredReceiversWalletStatus {
		return fmt.Errorf("invalid status for receiver_wallet after receiver registration")
	}

	return nil
}

func validateStellarTransaction(hPayment *operations.Payment, receiverAccount, disbursedAssetCode, disbursedAssetIssuer, amount string) error {
	if !hPayment.TransactionSuccessful {
		return fmt.Errorf("transaction was not successful on horizon network")
	}

	if hPayment.To != receiverAccount {
		return fmt.Errorf("transaction sent to wrong receiver account")
	}

	if hPayment.Amount != amount {
		return fmt.Errorf("transaction with wrong amount")
	}

	dataAsset := data.Asset{
		Code:   disbursedAssetCode,
		Issuer: disbursedAssetIssuer,
	}
	if !dataAsset.EqualsHorizonAsset(hPayment.Asset) {
		log.Errorf("disbursed.asset: %s:%s", disbursedAssetCode, disbursedAssetIssuer)
		log.Errorf("hAsset: %+v", hPayment.Asset)
		return fmt.Errorf("transaction with wrong disbursed asset")
	}

	return nil
}

// validateContractStellarTransaction validates a payment to a contract address (C-address).
// Contract payments show up as InvokeHostFunction operations on Horizon, not Payment operations.
// This function validates that the transaction was successful.
// Note: Unlike regular payments, InvokeHostFunction operations don't expose the recipient address
// or amount directly in the operation fields - the transfer happens inside the contract execution.
func validateContractStellarTransaction(ihf *operations.InvokeHostFunction) error {
	if !ihf.TransactionSuccessful {
		return fmt.Errorf("contract transaction was not successful on horizon network")
	}

	// For contract payments, we can verify:
	// 1. The transaction was successful
	// 2. The operation type is InvokeHostFunction (already verified by getInvokeHostFunctionOnHorizon)
	// 3. The function type is HostFunctionTypeInvokeContract (SAC token transfer)
	if ihf.Function != "HostFunctionTypeHostFunctionTypeInvokeContract" {
		log.Warnf("unexpected host function type: %s (expected HostFunctionTypeHostFunctionTypeInvokeContract)", ihf.Function)
	}

	log.Infof("Contract transaction validated successfully: tx_hash=%s, function=%s", ihf.TransactionHash, ihf.Function)
	return nil
}
