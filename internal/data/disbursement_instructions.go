package data

import (
	"context"
	"errors"
	"fmt"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type DisbursementInstruction struct {
	Phone             *string `csv:"phone"`
	Email             *string `csv:"email"`
	ID                string  `csv:"id"`
	Amount            string  `csv:"amount"`
	VerificationValue string  `csv:"verification"`
	ExternalPaymentId *string `csv:"paymentID"`
}

func NewDisbursementInstruction(id, amount, verificationValue string) *DisbursementInstruction {
	return &DisbursementInstruction{
		ID:                id,
		Amount:            amount,
		VerificationValue: verificationValue,
	}
}

func (di *DisbursementInstruction) WithPhone(phone string) *DisbursementInstruction {
	di.Phone = utils.StringPtr(phone)
	return di
}

func (di *DisbursementInstruction) WithEmail(email string) *DisbursementInstruction {
	di.Email = utils.StringPtr(email)
	return di
}

func (di *DisbursementInstruction) WithExternalPaymentID(externalPaymentID string) *DisbursementInstruction {
	di.ExternalPaymentId = utils.StringPtr(externalPaymentID)
	return di
}

func (di *DisbursementInstruction) Contact() (string, error) {
	if di.Phone != nil && di.Email != nil {
		return "", errors.New("phone and email are both provided")
	}
	if di.Phone != nil {
		return *di.Phone, nil
	}
	if di.Email != nil {
		return *di.Email, nil
	}
	return "", errors.New("phone and email are empty")
}

type DisbursementInstructionModel struct {
	dbConnectionPool          db.DBConnectionPool
	receiverVerificationModel *ReceiverVerificationModel
	receiverWalletModel       *ReceiverWalletModel
	receiverModel             *ReceiverModel
	paymentModel              *PaymentModel
	disbursementModel         *DisbursementModel
}

const MaxInstructionsPerDisbursement = 10000

// NewDisbursementInstructionModel creates a new DisbursementInstructionModel.
func NewDisbursementInstructionModel(dbConnectionPool db.DBConnectionPool) *DisbursementInstructionModel {
	return &DisbursementInstructionModel{
		dbConnectionPool:          dbConnectionPool,
		receiverVerificationModel: &ReceiverVerificationModel{},
		receiverWalletModel:       &ReceiverWalletModel{dbConnectionPool: dbConnectionPool},
		receiverModel:             &ReceiverModel{},
		paymentModel:              &PaymentModel{dbConnectionPool: dbConnectionPool},
		disbursementModel:         &DisbursementModel{dbConnectionPool: dbConnectionPool},
	}
}

var (
	ErrMaxInstructionsExceeded      = errors.New("maximum number of instructions exceeded")
	ErrReceiverVerificationMismatch = errors.New("receiver verification mismatch")
)

type DisbursementInstructionsOpts struct {
	UserID                  string
	Instructions            []*DisbursementInstruction
	ReceiverContactType     ReceiverContactType
	Disbursement            *Disbursement
	DisbursementUpdate      *DisbursementUpdate
	MaxNumberOfInstructions int
}

// ProcessAll Processes all disbursement instructions and persists the data to the database.
//
//	|--- For line in the instructions:
//	|    |--- Check if a receiver exists by their contact information (phone, email).
//	|    |    |--- If a receiver does not exist, create one.
//	|    |--- For each receiver:
//	|    |    |--- Check if the receiver verification exists.
//	|    |    |    |--- If the receiver verification does not exist, create one.
//	|    |    |    |--- If the receiver verification exists:
//	|    |    |    |    |--- Check if the verification value matches.
//	|    |    |    |    |    |--- If the verification value does not match and the verification is confirmed, return an error.
//	|    |    |    |    |    |--- If the verification value does not match and the verification is not confirmed, update the verification value.
//	|    |    |    |    |    |--- If the verification value matches, continue.
//	|    |    |--- Check if the receiver wallet exists.
//	|    |    |    |--- If the receiver wallet does not exist, create one.
//	|    |    |    |--- If the receiver wallet exists and it's not REGISTERED, retry the invitation SMS.
//	|    |    |--- Delete all previously existing payments tied to this disbursement.
//	|    |    |--- Create all payments passed in the instructions.
func (di DisbursementInstructionModel) ProcessAll(ctx context.Context, opts DisbursementInstructionsOpts) error {
	if len(opts.Instructions) > opts.MaxNumberOfInstructions {
		return ErrMaxInstructionsExceeded
	}

	// We need all the following logic to be executed in one transaction.
	return db.RunInTransaction(ctx, di.dbConnectionPool, nil, func(dbTx db.DBTransaction) error {
		// Step 1: Fetch all receivers by phone number and create missing ones
		receiverMap, err := di.processReceivers(ctx, dbTx, opts.Instructions, opts.ReceiverContactType)
		if err != nil {
			return fmt.Errorf("processing receivers: %w", err)
		}

		// Step 2: Fetch all receiver verifications and create missing ones.
		err = di.processReceiverVerifications(ctx, dbTx, receiverMap, opts.Instructions, opts.Disbursement, opts.ReceiverContactType)
		if err != nil {
			return fmt.Errorf("processing receiver verifications: %w", err)
		}

		// Step 3: Fetch all receiver wallets and create missing ones
		receiverIDToReceiverWalletIDMap, err := di.processReceiverWallets(ctx, dbTx, receiverMap, opts.Disbursement)
		if err != nil {
			return fmt.Errorf("processing receiver wallets: %w", err)
		}

		// Step 4: Delete all payments tied to this disbursement for each receiver in one call
		if err = di.paymentModel.DeleteAllForDisbursement(ctx, dbTx, opts.Disbursement.ID); err != nil {
			return fmt.Errorf("deleting payments: %w", err)
		}

		// Step 5: Create payments for all receivers
		if err = di.createPayments(ctx, dbTx, receiverMap, receiverIDToReceiverWalletIDMap, opts.Instructions, opts.Disbursement); err != nil {
			return fmt.Errorf("creating payments: %w", err)
		}

		// Step 6: Persist Payment file to Disbursement
		if err = di.disbursementModel.Update(ctx, opts.DisbursementUpdate); err != nil {
			return fmt.Errorf("persisting payment file: %w", err)
		}

		// Step 7: Update Disbursement Status
		if err = di.disbursementModel.UpdateStatus(ctx, dbTx, opts.UserID, opts.Disbursement.ID, ReadyDisbursementStatus); err != nil {
			return fmt.Errorf("updating status: %w", err)
		}

		return nil
	})
}

func (di DisbursementInstructionModel) processReceivers(ctx context.Context, dbTx db.DBTransaction, instructions []*DisbursementInstruction, contactType ReceiverContactType) (map[string]*Receiver, error) {
	// Step 1: Fetch existing receivers
	contacts := make([]string, 0, len(instructions))
	for _, instruction := range instructions {
		contact, err := instruction.Contact()
		if err != nil {
			return nil, fmt.Errorf("resolving contact information for instruction with ID %s: %w", instruction.ID, err)
		}
		contacts = append(contacts, contact)
	}

	existingReceivers, err := di.receiverModel.GetByContacts(ctx, dbTx, contacts)
	if err != nil {
		return nil, fmt.Errorf("fetching receivers by contacts: %w", err)
	}

	// Step 2: Create maps for quick lookup
	existingReceiversMap := make(map[string]*Receiver)
	for _, receiver := range existingReceivers {
		contact, contactErr := receiver.ContactByType(contactType)
		if contactErr != nil {
			return nil, fmt.Errorf("receiver with ID %s has no contact information: %w", receiver.ID, contactErr)
		}
		existingReceiversMap[contact] = receiver
	}

	// Step 3: Process each instruction
	for _, instruction := range instructions {
		processErr := di.processInstruction(ctx, dbTx, instruction, existingReceiversMap)
		if processErr != nil {
			return nil, fmt.Errorf("processing instruction: %w", processErr)
		}
	}

	// Step 4: Fetch all receivers again
	receivers, err := di.receiverModel.GetByContacts(ctx, dbTx, contacts)
	if err != nil {
		return nil, fmt.Errorf("fetching receivers by contact information: %w", err)
	}

	if len(receivers) != len(instructions) {
		return nil, fmt.Errorf("receiver count mismatch after processing instructions")
	}

	receiversMap := make(map[string]*Receiver)
	for _, receiver := range receivers {
		receiversMap[receiver.ID] = receiver
	}

	return receiversMap, nil
}

func (di DisbursementInstructionModel) processInstruction(ctx context.Context, dbTx db.DBTransaction, instruction *DisbursementInstruction, receiverMap map[string]*Receiver) error {
	contact, err := instruction.Contact()
	if err != nil {
		return fmt.Errorf("resolving contact information for instruction with ID %s: %w", instruction.ID, err)
	}

	_, exists := receiverMap[contact]
	if !exists {
		receiverInsert := ReceiverInsert{
			PhoneNumber: instruction.Phone,
			ExternalId:  &instruction.ID,
		}
		_, insertErr := di.receiverModel.Insert(ctx, dbTx, receiverInsert)
		if insertErr != nil {
			return fmt.Errorf("inserting receiver: %w", insertErr)
		}
	}

	return nil
}

func (di DisbursementInstructionModel) processReceiverVerifications(ctx context.Context, dbTx db.DBTransaction, receiversByIDMap map[string]*Receiver, instructions []*DisbursementInstruction, disbursement *Disbursement, contactType ReceiverContactType) error {
	receiverIDs := make([]string, 0, len(receiversByIDMap))
	for id := range receiversByIDMap {
		receiverIDs = append(receiverIDs, id)
	}

	verifications, err := di.receiverVerificationModel.GetByReceiverIDsAndVerificationField(ctx, dbTx, receiverIDs, disbursement.VerificationField)
	if err != nil {
		return fmt.Errorf("fetching receiver verifications: %w", err)
	}

	verificationByReceiverIDMap := make(map[string]*ReceiverVerification)
	for _, verification := range verifications {
		verificationByReceiverIDMap[verification.ReceiverID] = verification
	}

	instructionsByContactMap := make(map[string]*DisbursementInstruction)
	for _, instruction := range instructions {
		contact, err := instruction.Contact()
		if err != nil {
			return fmt.Errorf("resolving contact information for instruction with ID %s: %w", instruction.ID, err)
		}
		instructionsByContactMap[contact] = instruction
	}

	for _, receiver := range receiversByIDMap {
		contact, err := receiver.ContactByType(contactType)
		if err != nil {
			return fmt.Errorf("receiver with ID %s has no contact information for contact type %s: %w", receiver.ID, contactType, err)
		}
		instruction := instructionsByContactMap[contact]
		if instruction == nil {
			return fmt.Errorf("instruction not found for receiver with ID %s", receiver.ID)
		}
		verification, exists := verificationByReceiverIDMap[receiver.ID]

		if !exists {
			verificationInsert := ReceiverVerificationInsert{
				ReceiverID:        receiver.ID,
				VerificationValue: instruction.VerificationValue,
				VerificationField: disbursement.VerificationField,
			}
			_, insertErr := di.receiverVerificationModel.Insert(ctx, dbTx, verificationInsert)
			if insertErr != nil {
				return fmt.Errorf("error inserting receiver verification: %w", insertErr)
			}
		} else if !CompareVerificationValue(verification.HashedValue, instruction.VerificationValue) {
			if verification.ConfirmedAt != nil {
				return fmt.Errorf("%w: receiver verification for %s doesn't match. Check instruction with ID %s", ErrReceiverVerificationMismatch, contact, instruction.ID)
			}
			updateErr := di.receiverVerificationModel.UpdateVerificationValue(ctx, dbTx, verification.ReceiverID, verification.VerificationField, instruction.VerificationValue)
			if updateErr != nil {
				return fmt.Errorf("error updating receiver verification for disbursement id %s: %w", disbursement.ID, updateErr)
			}
		}
	}

	return nil
}

func (di DisbursementInstructionModel) processReceiverWallets(ctx context.Context, dbTx db.DBTransaction, receiverMap map[string]*Receiver, disbursement *Disbursement) (map[string]string, error) {
	receiverIDs := make([]string, 0, len(receiverMap))
	for id := range receiverMap {
		receiverIDs = append(receiverIDs, id)
	}

	receiverWallets, err := di.receiverWalletModel.GetByReceiverIDsAndWalletID(ctx, dbTx, receiverIDs, disbursement.Wallet.ID)
	if err != nil {
		return nil, fmt.Errorf("fetching receiver wallets: %w", err)
	}
	receiverIDToReceiverWalletIDMap := make(map[string]string)
	for _, receiverWallet := range receiverWallets {
		receiverIDToReceiverWalletIDMap[receiverWallet.Receiver.ID] = receiverWallet.ID
	}

	for receiverId := range receiverMap {
		receiverWalletId, exists := receiverIDToReceiverWalletIDMap[receiverId]
		if !exists {
			receiverWalletInsert := ReceiverWalletInsert{
				ReceiverID: receiverId,
				WalletID:   disbursement.Wallet.ID,
			}
			rwID, insertErr := di.receiverWalletModel.Insert(ctx, dbTx, receiverWalletInsert)
			if insertErr != nil {
				return nil, fmt.Errorf("inserting receiver wallet for receiver id %s: %w", receiverId, insertErr)
			}
			receiverIDToReceiverWalletIDMap[receiverId] = rwID
		} else {
			_, retryErr := di.receiverWalletModel.RetryInvitationSMS(ctx, dbTx, receiverWalletId)
			if retryErr != nil {
				if !errors.Is(retryErr, ErrRecordNotFound) {
					return nil, fmt.Errorf("retrying invitation: %w", retryErr)
				}
			}
		}
	}

	return receiverIDToReceiverWalletIDMap, nil
}

func (di DisbursementInstructionModel) createPayments(ctx context.Context, dbTx db.DBTransaction, receiverMap map[string]*Receiver, receiverIDToReceiverWalletIDMap map[string]string, instructions []*DisbursementInstruction, disbursement *Disbursement) error {
	payments := make([]PaymentInsert, 0, len(instructions))

	for _, instruction := range instructions {
		receiver := findReceiverByInstruction(receiverMap, instruction)
		if receiver == nil {
			return fmt.Errorf("receiver not found for instruction with ID %s", instruction.ID)
		}
		payment := PaymentInsert{
			ReceiverID:        receiver.ID,
			DisbursementID:    disbursement.ID,
			Amount:            instruction.Amount,
			AssetID:           disbursement.Asset.ID,
			ReceiverWalletID:  receiverIDToReceiverWalletIDMap[receiver.ID],
			ExternalPaymentID: instruction.ExternalPaymentId,
		}
		payments = append(payments, payment)
	}

	if err := di.paymentModel.InsertAll(ctx, dbTx, payments); err != nil {
		return fmt.Errorf("inserting payments: %w", err)
	}

	return nil
}

func findReceiverByInstruction(receiverMap map[string]*Receiver, instruction *DisbursementInstruction) *Receiver {
	contact, err := instruction.Contact()
	if err != nil {
		return nil
	}

	for _, receiver := range receiverMap {
		if utils.StringPtrEq(receiver.Email, contact) || utils.StringPtrEq(receiver.PhoneNumber, contact) {
			return receiver
		}
	}
	return nil
}
