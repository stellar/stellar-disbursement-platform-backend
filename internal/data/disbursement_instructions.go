package data

import (
	"context"
	"errors"
	"fmt"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
)

type DisbursementInstruction struct {
	Phone             string  `csv:"phone"`
	ID                string  `csv:"id"`
	Amount            string  `csv:"amount"`
	VerificationValue string  `csv:"verification"`
	ExternalPaymentId *string `csv:"paymentID"`
}

type DisbursementInstructionModel struct {
	dbConnectionPool          db.DBConnectionPool
	receiverVerificationModel *ReceiverVerificationModel
	receiverWalletModel       *ReceiverWalletModel
	receiverModel             *ReceiverModel
	paymentModel              *PaymentModel
	disbursementModel         *DisbursementModel
}

const MaxInstructionsPerDisbursement = 10000 // TODO: update this number with load testing results [SDP-524]

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

// ProcessAll Processes all disbursement instructions and persists the data to the database.
//
//	|--- For each phone number in the instructions:
//	|    |--- Check if a receiver exists.
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
//	|    |    |--- Delete all payments tied to this disbursement.
//	|    |    |--- Create all payments passed in the instructions.
func (di DisbursementInstructionModel) ProcessAll(ctx context.Context, userID string, instructions []*DisbursementInstruction, disbursement *Disbursement, update *DisbursementUpdate, maxNumberOfInstructions int, eventProducer events.Producer) error {
	if len(instructions) > maxNumberOfInstructions {
		return ErrMaxInstructionsExceeded
	}

	// We need all the following logic to be executed in one transaction.
	return db.RunInTransaction(ctx, di.dbConnectionPool, nil, func(dbTx db.DBTransaction) error {
		// Step 1: Fetch all receivers by phone number and create missing ones
		phoneNumbers := make([]string, 0, len(instructions))
		for _, instruction := range instructions {
			phoneNumbers = append(phoneNumbers, instruction.Phone)
		}

		existingReceivers, err := di.receiverModel.GetByPhoneNumbers(ctx, dbTx, phoneNumbers)
		if err != nil {
			return fmt.Errorf("error fetching receivers by phone number: %w", err)
		}

		receiverMap := make(map[string]*Receiver)
		for _, receiver := range existingReceivers {
			receiverMap[receiver.PhoneNumber] = receiver
		}

		instructionMap := make(map[string]*DisbursementInstruction)
		for _, instruction := range instructions {
			instructionMap[instruction.Phone] = instruction
		}

		for _, instruction := range instructions {
			_, exists := receiverMap[instruction.Phone]
			if !exists {
				receiverInsert := ReceiverInsert{
					PhoneNumber: instruction.Phone,
					ExternalId:  &instruction.ID,
				}
				receiver, insertErr := di.receiverModel.Insert(ctx, dbTx, receiverInsert)
				if insertErr != nil {
					return fmt.Errorf("error inserting receiver: %w", insertErr)
				}
				receiverMap[instruction.Phone] = receiver
			}
		}

		// Step 2: Fetch all receiver verifications and create missing ones.
		receiverIDs := make([]string, 0, len(receiverMap))
		for _, receiver := range receiverMap {
			receiverIDs = append(receiverIDs, receiver.ID)
		}
		verifications, err := di.receiverVerificationModel.GetByReceiverIDsAndVerificationField(ctx, dbTx, receiverIDs, disbursement.VerificationField)
		if err != nil {
			return fmt.Errorf("error fetching receiver verifications: %w", err)
		}

		verificationMap := make(map[string]*ReceiverVerification)
		for _, verification := range verifications {
			verificationMap[verification.ReceiverID] = verification
		}

		for _, receiver := range receiverMap {
			verification, verificationExists := verificationMap[receiver.ID]
			instruction := instructionMap[receiver.PhoneNumber]
			if !verificationExists {
				verificationInsert := ReceiverVerificationInsert{
					ReceiverID:        receiver.ID,
					VerificationValue: instruction.VerificationValue,
					VerificationField: disbursement.VerificationField,
				}
				hashedVerification, insertError := di.receiverVerificationModel.Insert(ctx, dbTx, verificationInsert)
				if insertError != nil {
					return fmt.Errorf("error inserting receiver verification: %w", insertError)
				}
				verificationMap[receiver.ID] = &ReceiverVerification{
					ReceiverID:        verificationInsert.ReceiverID,
					VerificationField: verificationInsert.VerificationField,
					HashedValue:       hashedVerification,
				}

			} else {
				if verified := CompareVerificationValue(verification.HashedValue, instruction.VerificationValue); !verified {
					if verification.ConfirmedAt != nil {
						return fmt.Errorf("%w: receiver verification for %s doesn't match", ErrReceiverVerificationMismatch, receiver.PhoneNumber)
					}
					err = di.receiverVerificationModel.UpdateVerificationValue(ctx, dbTx, verification.ReceiverID, verification.VerificationField, instruction.VerificationValue)
					if err != nil {
						return fmt.Errorf("error updating receiver verification for disbursement id %s: %w", disbursement.ID, err)
					}
				}
			}
		}

		// Step 3: Fetch all receiver wallets and create missing ones
		receiverWallets, err := di.receiverWalletModel.GetByReceiverIDsAndWalletID(ctx, dbTx, receiverIDs, disbursement.Wallet.ID)
		if err != nil {
			return fmt.Errorf("error fetching receiver wallets: %w", err)
		}
		receiverIDToReceiverWalletIDMap := make(map[string]string)
		for _, receiverWallet := range receiverWallets {
			receiverIDToReceiverWalletIDMap[receiverWallet.Receiver.ID] = receiverWallet.ID
		}

		eventData := make([]schemas.EventReceiverWalletSMSInvitationData, 0, len(receiverIDs))
		for _, receiverId := range receiverIDs {
			receiverWalletId, exists := receiverIDToReceiverWalletIDMap[receiverId]
			if !exists {
				receiverWalletInsert := ReceiverWalletInsert{
					ReceiverID: receiverId,
					WalletID:   disbursement.Wallet.ID,
				}
				rwID, insertErr := di.receiverWalletModel.Insert(ctx, dbTx, receiverWalletInsert)
				if insertErr != nil {
					return fmt.Errorf("error inserting receiver wallet for receiver id %s: %w", receiverId, insertErr)
				}
				receiverIDToReceiverWalletIDMap[receiverId] = rwID
				eventData = append(eventData, schemas.EventReceiverWalletSMSInvitationData{ReceiverWalletID: rwID})
			} else {
				rw, retryErr := di.receiverWalletModel.RetryInvitationSMS(ctx, dbTx, receiverWalletId)
				if retryErr != nil {
					if !errors.Is(retryErr, ErrRecordNotFound) {
						return fmt.Errorf("error retrying invitation: %w", retryErr)
					}
				}
				if rw != nil && rw.Status == ReadyReceiversWalletStatus {
					eventData = append(eventData, schemas.EventReceiverWalletSMSInvitationData{ReceiverWalletID: rw.ID})
				}
			}
		}

		// Step 4: Delete all payments tied to this disbursement for each receiver in one call
		if err = di.paymentModel.DeleteAllForDisbursement(ctx, dbTx, disbursement.ID); err != nil {
			return fmt.Errorf("error deleting payments: %w", err)
		}

		// Step 5: Create payments for all receivers
		payments := make([]PaymentInsert, 0, len(instructions))
		for _, instruction := range instructions {
			receiver := receiverMap[instruction.Phone]
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
		if err = di.paymentModel.InsertAll(ctx, dbTx, payments); err != nil {
			return fmt.Errorf("error inserting payments: %w", err)
		}

		// Step 6: Persist Payment file to Disbursement
		if err = di.disbursementModel.Update(ctx, update); err != nil {
			return fmt.Errorf("error persisting payment file: %w", err)
		}

		// Step 7: Update Disbursement Status
		if err = di.disbursementModel.UpdateStatus(ctx, dbTx, userID, disbursement.ID, ReadyDisbursementStatus); err != nil {
			return fmt.Errorf("error updating status: %w", err)
		}

		// Step 8: Produce event to send invitation message to the receivers
		msg, err := events.NewMessage(ctx, events.ReceiverWalletNewInvitationTopic, disbursement.ID, events.BatchReceiverWalletSMSInvitationType, eventData)
		if err != nil {
			return fmt.Errorf("creating event producer message: %w", err)
		}

		if eventProducer != nil {
			err = eventProducer.WriteMessages(ctx, *msg)
			if err != nil {
				return fmt.Errorf("publishing message %s on event producer: %w", msg.String(), err)
			}
		} else {
			log.Ctx(ctx).Errorf("event producer is nil, could not publish message %s", msg.String())
		}

		return nil
	})
}
