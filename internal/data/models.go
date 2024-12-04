package data

import (
	"errors"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

var (
	ErrRecordNotFound          = errors.New("record not found")
	ErrRecordAlreadyExists     = errors.New("record already exists")
	ErrMismatchNumRowsAffected = errors.New("mismatch number of rows affected")
	ErrMissingInput            = errors.New("missing input")
)

type Models struct {
	Disbursements            *DisbursementModel
	Wallets                  *WalletModel
	Assets                   *AssetModel
	Organizations            *OrganizationModel
	Payment                  *PaymentModel
	Receiver                 *ReceiverModel
	DisbursementInstructions *DisbursementInstructionModel
	ReceiverVerification     *ReceiverVerificationModel
	ReceiverWallet           *ReceiverWalletModel
	DisbursementReceivers    *DisbursementReceiverModel
	Message                  *MessageModel
	CircleTransferRequests   *CircleTransferRequestModel
	CircleRecipient          *CircleRecipientModel
	DBConnectionPool         db.DBConnectionPool
}

func NewModels(dbConnectionPool db.DBConnectionPool) (*Models, error) {
	if dbConnectionPool == nil {
		return nil, errors.New("dbConnectionPool is required for NewModels")
	}
	return &Models{
		Disbursements:            &DisbursementModel{dbConnectionPool: dbConnectionPool},
		Wallets:                  &WalletModel{dbConnectionPool: dbConnectionPool},
		Assets:                   &AssetModel{dbConnectionPool: dbConnectionPool},
		Organizations:            &OrganizationModel{dbConnectionPool: dbConnectionPool},
		Payment:                  &PaymentModel{dbConnectionPool: dbConnectionPool},
		Receiver:                 &ReceiverModel{},
		DisbursementInstructions: NewDisbursementInstructionModel(dbConnectionPool),
		ReceiverVerification:     &ReceiverVerificationModel{dbConnectionPool: dbConnectionPool},
		ReceiverWallet:           &ReceiverWalletModel{dbConnectionPool: dbConnectionPool},
		DisbursementReceivers:    &DisbursementReceiverModel{dbConnectionPool: dbConnectionPool},
		Message:                  &MessageModel{dbConnectionPool: dbConnectionPool},
		CircleTransferRequests:   &CircleTransferRequestModel{dbConnectionPool: dbConnectionPool},
		CircleRecipient:          &CircleRecipientModel{dbConnectionPool: dbConnectionPool},
		DBConnectionPool:         dbConnectionPool,
	}, nil
}
