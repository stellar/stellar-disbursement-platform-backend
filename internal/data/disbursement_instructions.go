package data

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/stellar/go/support/log"
	"golang.org/x/exp/maps"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

type DisbursementInstruction struct {
	Phone             string `csv:"phone"`
	Email             string `csv:"email"`
	ID                string `csv:"id"`
	Amount            string `csv:"amount"`
	VerificationValue string `csv:"verification"`
	ExternalPaymentId string `csv:"paymentID"`
	WalletAddress     string `csv:"walletAddress"`
	WalletAddressMemo string `csv:"walletAddressMemo"`
}

func (di *DisbursementInstruction) Contact() (string, error) {
	if di.Phone != "" && di.Email != "" {
		return "", errors.New("phone and email are both provided")
	}
	if di.Phone != "" {
		return di.Phone, nil
	}
	if di.Email != "" {
		return di.Email, nil
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
	ErrMaxInstructionsExceeded       = errors.New("maximum number of instructions exceeded")
	ErrReceiverVerificationMismatch  = errors.New("receiver verification mismatch")
	ErrReceiverWalletAddressMismatch = errors.New("receiver wallet address mismatch")
)

type DisbursementInstructionsOpts struct {
	UserID                  string
	Instructions            []*DisbursementInstruction
	Disbursement            *Disbursement
	DisbursementUpdate      *DisbursementUpdate
	MaxNumberOfInstructions int
}

// ProcessAll Processes all disbursement instructions and persists the data to the database.
//
//	|--- For each line in the instructions:
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
//	|    |    |--- [!ReceiverContactType.IncludesWalletAddress] Check if the receiver wallet exists.
//	|    |    |    |--- If the receiver wallet does not exist, create one.
//	|    |    |    |--- If the receiver wallet exists and it's not REGISTERED, retry the invitation SMS.
//	|    |    |--- [ReceiverContactType.IncludesWalletAddress] Register the supplied wallet address
//	|    |    |--- Delete all previously existing payments tied to this disbursement.
//	|    |    |--- Create all payments passed in the instructions.
func (di DisbursementInstructionModel) ProcessAll(ctx context.Context, dbTx db.DBTransaction, opts DisbursementInstructionsOpts) error {
	if len(opts.Instructions) > opts.MaxNumberOfInstructions {
		return ErrMaxInstructionsExceeded
	}

	// Step 1: Fetch all receivers by contact information (phone, email, etc.) and create missing ones
	registrationContactType := opts.Disbursement.RegistrationContactType
	receiversByIDMap, err := di.reconcileExistingReceiversWithInstructions(ctx, dbTx, opts.Instructions, registrationContactType.ReceiverContactType)
	if err != nil {
		return fmt.Errorf("processing receivers: %w", err)
	}

	// Step 2: Fetch all receiver wallets and create missing ones
	receiverIDToReceiverWalletIDMap, err := di.processReceiverWallets(ctx, dbTx, receiversByIDMap, opts.Disbursement)
	if err != nil {
		return fmt.Errorf("processing receiver wallets: %w", err)
	}

	// Step 3: Register supplied wallets or process receiver verifications based on the registration contact type
	if registrationContactType.IncludesWalletAddress {
		if err = di.registerSuppliedWallets(ctx, dbTx, opts.Instructions, receiversByIDMap, receiverIDToReceiverWalletIDMap); err != nil {
			if errors.Is(err, ErrDuplicateWalletAddress) {
				return err
			}
			return fmt.Errorf("registering supplied wallets: %w", err)
		}
	} else {
		err = di.processReceiverVerifications(ctx, dbTx, receiversByIDMap, opts.Instructions, opts.Disbursement, registrationContactType.ReceiverContactType)
		if err != nil {
			return fmt.Errorf("processing receiver verifications: %w", err)
		}
	}

	// Step 4: Delete all pre-existing draft payments tied to this disbursement for each receiver in one call
	if err = di.paymentModel.DeleteAllDraftForDisbursement(ctx, dbTx, opts.Disbursement.ID); err != nil {
		return fmt.Errorf("deleting draft payments: %w", err)
	}

	// Step 5: Create payments for all receivers
	if err = di.createPayments(ctx, dbTx, receiversByIDMap, receiverIDToReceiverWalletIDMap, opts.Instructions, opts.Disbursement); err != nil {
		return fmt.Errorf("creating payments: %w", err)
	}

	// Step 6: Persist Payment file to Disbursement
	if err = di.disbursementModel.Update(ctx, dbTx, opts.DisbursementUpdate); err != nil {
		return fmt.Errorf("persisting payment file: %w", err)
	}

	// Step 7: Update Disbursement Status
	if err = di.disbursementModel.UpdateStatus(ctx, dbTx, opts.UserID, opts.Disbursement.ID, ReadyDisbursementStatus); err != nil {
		return fmt.Errorf("updating status: %w", err)
	}

	return nil
}

// registerSuppliedWallets registers the supplied wallets for the given instructions.
func (di DisbursementInstructionModel) registerSuppliedWallets(ctx context.Context, dbTx db.DBTransaction, instructions []*DisbursementInstruction, receiversByIDMap map[string]*Receiver, receiverIDToReceiverWalletIDMap map[string]string) error {
	// Construct a map of receiverWalletID to receiverWallet
	receiverWalletsByIDMap, err := di.getReceiverWalletsByIDMap(ctx, dbTx, maps.Values(receiverIDToReceiverWalletIDMap))
	if err != nil {
		return fmt.Errorf("building receiver wallets lookup map: %w", err)
	}

	// Mark receiver wallets as registered
	for _, instruction := range instructions {
		receiver := findReceiverByInstruction(receiversByIDMap, instruction)
		if receiver == nil {
			return fmt.Errorf("receiver not found for instruction with ID %s", instruction.ID)
		}
		receiverWalletID, exists := receiverIDToReceiverWalletIDMap[receiver.ID]
		if !exists {
			return fmt.Errorf("receiver wallet not found for receiver with ID %s", receiver.ID)
		}
		receiverWallet := receiverWalletsByIDMap[receiverWalletID]

		if receiverWallet.StellarAddress != "" && receiverWallet.StellarAddress != instruction.WalletAddress {
			return fmt.Errorf("%w: receiver wallet address mismatch for receiver with ID %s", ErrReceiverWalletAddressMismatch, receiver.ID)
		}

		if slices.Contains([]ReceiversWalletStatus{RegisteredReceiversWalletStatus, FlaggedReceiversWalletStatus}, receiverWallet.Status) {
			log.Ctx(ctx).Infof("receiver wallet with ID %s is %s, skipping registration", receiverWallet.ID, receiverWallet.Status)
			continue
		}

		receiverWalletUpdate := ReceiverWalletUpdate{
			Status:         RegisteredReceiversWalletStatus,
			StellarAddress: instruction.WalletAddress,
		}
		if instruction.WalletAddressMemo != "" {
			_, memoType, err := schema.ParseMemo(instruction.WalletAddressMemo)
			if err != nil {
				return fmt.Errorf("parsing memo %s: %w", instruction.WalletAddressMemo, err)
			}
			receiverWalletUpdate.StellarMemo = &instruction.WalletAddressMemo
			receiverWalletUpdate.StellarMemoType = &memoType
		}
		if updateErr := di.receiverWalletModel.Update(ctx, receiverWalletID, receiverWalletUpdate, dbTx); updateErr != nil {
			if errors.Is(updateErr, ErrDuplicateWalletAddress) {
				return fmt.Errorf("wallet address %s is already registered to another receiver: %w", instruction.WalletAddress, ErrDuplicateWalletAddress)
			}
			return fmt.Errorf("marking receiver wallet as registered: %w", updateErr)
		}
	}
	return nil
}

func (di DisbursementInstructionModel) getReceiverWalletsByIDMap(ctx context.Context, dbTx db.DBTransaction, receiverWalletIDs []string) (map[string]ReceiverWallet, error) {
	receiverWallets, err := di.receiverWalletModel.GetByIDs(ctx, dbTx, receiverWalletIDs...)
	if err != nil {
		return nil, fmt.Errorf("fetching receiver wallets: %w", err)
	}
	receiverWalletsByIDMap := make(map[string]ReceiverWallet, len(receiverWallets))
	for _, receiverWallet := range receiverWallets {
		receiverWalletsByIDMap[receiverWallet.ID] = receiverWallet
	}
	return receiverWalletsByIDMap, nil
}

// reconcileExistingReceiversWithInstructions fetches all existing receivers by their contact information and creates missing ones.
func (di DisbursementInstructionModel) reconcileExistingReceiversWithInstructions(ctx context.Context, dbTx db.DBTransaction, instructions []*DisbursementInstruction, contactType ReceiverContactType) (map[string]*Receiver, error) {
	// Step 1: Fetch existing receivers
	contacts := make([]string, 0, len(instructions))
	for _, instruction := range instructions {
		contact, err := instruction.Contact()
		if err != nil {
			return nil, fmt.Errorf("resolving contact information for instruction with ID %s: %w", instruction.ID, err)
		}
		contacts = append(contacts, contact)
	}

	existingReceivers, err := di.receiverModel.GetByContacts(ctx, dbTx, contacts...)
	if err != nil {
		return nil, fmt.Errorf("fetching receivers by contacts: %w", err)
	}

	// Step 2: Create maps for quick lookup
	existingReceiversByContactMap := make(map[string]*Receiver)
	for _, receiver := range existingReceivers {
		contact := receiver.ContactByType(contactType)
		if contact == "" {
			return nil, fmt.Errorf("receiver with ID %s has no contact information for contact type %s", receiver.ID, contactType)
		}
		existingReceiversByContactMap[contact] = receiver
	}

	// Step 3: Create missing receivers from instructions
	for _, instruction := range instructions {
		if createErr := di.createReceiverFromInstructionIfNeeded(ctx, dbTx, instruction, existingReceiversByContactMap); createErr != nil {
			return nil, fmt.Errorf("creating receiver from instruction: %w", createErr)
		}
	}

	// Step 4: Fetch all receivers again
	receivers, err := di.receiverModel.GetByContacts(ctx, dbTx, contacts...)
	if err != nil {
		return nil, fmt.Errorf("fetching receivers by contact information: %w", err)
	}

	if len(receivers) != len(instructions) {
		return nil, fmt.Errorf("receiver count mismatch after processing instructions")
	}

	receiversByIDMap := make(map[string]*Receiver)
	for _, receiver := range receivers {
		receiversByIDMap[receiver.ID] = receiver
	}

	return receiversByIDMap, nil
}

// createReceiverFromInstructionIfNeeded create a new receiver if it doesn't exist for the given instruction.
func (di DisbursementInstructionModel) createReceiverFromInstructionIfNeeded(ctx context.Context, dbTx db.DBTransaction, instruction *DisbursementInstruction, existingReceiversByContactMap map[string]*Receiver) error {
	contact, err := instruction.Contact()
	if err != nil {
		return fmt.Errorf("resolving contact information for instruction with ID %s: %w", instruction.ID, err)
	}

	_, exists := existingReceiversByContactMap[contact]
	if !exists {
		var receiverInsert ReceiverInsert
		if instruction.Phone != "" {
			receiverInsert.PhoneNumber = &instruction.Phone
		}
		if instruction.Email != "" {
			receiverInsert.Email = &instruction.Email
		}
		if instruction.ID != "" {
			receiverInsert.ExternalId = &instruction.ID
		}
		_, insertErr := di.receiverModel.Insert(ctx, dbTx, receiverInsert)
		if insertErr != nil {
			return fmt.Errorf("inserting receiver: %w", insertErr)
		}
	}

	return nil
}

func (di DisbursementInstructionModel) processReceiverVerifications(ctx context.Context, dbTx db.DBTransaction, receiversByIDMap map[string]*Receiver, instructions []*DisbursementInstruction, disbursement *Disbursement, contactType ReceiverContactType) error {
	receiverIDs := maps.Keys(receiversByIDMap)

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
		contact := receiver.ContactByType(contactType)
		if contact == "" {
			return fmt.Errorf("receiver with ID %s has no contact information for contact type %s", receiver.ID, contactType)
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

func (di DisbursementInstructionModel) processReceiverWallets(ctx context.Context, dbTx db.DBTransaction, receiversByIDMap map[string]*Receiver, disbursement *Disbursement) (map[string]string, error) {
	receiverIDs := maps.Keys(receiversByIDMap)

	receiverWallets, err := di.receiverWalletModel.GetByReceiverIDsAndWalletID(ctx, dbTx, receiverIDs, disbursement.Wallet.ID)
	if err != nil {
		return nil, fmt.Errorf("fetching receiver wallets: %w", err)
	}
	receiverIDToReceiverWalletIDMap := make(map[string]string)
	for _, receiverWallet := range receiverWallets {
		receiverIDToReceiverWalletIDMap[receiverWallet.Receiver.ID] = receiverWallet.ID
	}

	for receiverID := range receiversByIDMap {
		receiverWalletID, exists := receiverIDToReceiverWalletIDMap[receiverID]
		if !exists {
			receiverWalletInsert := ReceiverWalletInsert{
				ReceiverID: receiverID,
				WalletID:   disbursement.Wallet.ID,
			}
			rwID, insertErr := di.receiverWalletModel.Insert(ctx, dbTx, receiverWalletInsert)
			if insertErr != nil {
				return nil, fmt.Errorf("inserting receiver wallet for receiver id %s: %w", receiverID, insertErr)
			}
			receiverIDToReceiverWalletIDMap[receiverID] = rwID
		} else {
			_, retryErr := di.receiverWalletModel.RetryInvitationMessage(ctx, dbTx, receiverWalletID)
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
			ReceiverID:       receiver.ID,
			DisbursementID:   &disbursement.ID,
			Amount:           instruction.Amount,
			AssetID:          disbursement.Asset.ID,
			ReceiverWalletID: receiverIDToReceiverWalletIDMap[receiver.ID],
			PaymentType:      PaymentTypeDisbursement,
		}
		if instruction.ExternalPaymentId != "" {
			payment.ExternalPaymentID = &instruction.ExternalPaymentId
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
		if contact == receiver.PhoneNumber || contact == receiver.Email {
			return receiver
		}
	}
	return nil
}
