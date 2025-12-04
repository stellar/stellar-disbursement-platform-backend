package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/stellar/go-stellar-sdk/support/log"
	"golang.org/x/crypto/bcrypt"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type ReceiverVerification struct {
	ReceiverID          string                  `json:"receiver_id" db:"receiver_id"`
	VerificationField   VerificationType        `json:"verification_field" db:"verification_field"`
	HashedValue         string                  `json:"hashed_value" db:"hashed_value"`
	Attempts            int                     `json:"attempts" db:"attempts"`
	CreatedAt           time.Time               `json:"created_at" db:"created_at"`
	ConfirmedByType     *ConfirmedByType        `json:"confirmed_by_type" db:"confirmed_by_type"`
	ConfirmedByID       *string                 `json:"confirmed_by_id" db:"confirmed_by_id"`
	UpdatedAt           time.Time               `json:"updated_at" db:"updated_at"`
	ConfirmedAt         *time.Time              `json:"confirmed_at" db:"confirmed_at"`
	FailedAt            *time.Time              `json:"failed_at" db:"failed_at"`
	VerificationChannel *message.MessageChannel `json:"verification_channel" db:"verification_channel"`
}

type ReceiverVerificationModel struct {
	dbConnectionPool db.DBConnectionPool
}

type ReceiverVerificationInsert struct {
	ReceiverID        string           `db:"receiver_id"`
	VerificationField VerificationType `db:"verification_field"`
	VerificationValue string           `db:"hashed_value"`
}

const MaxAttemptsAllowed = 15

func (rvi *ReceiverVerificationInsert) Validate() error {
	if strings.TrimSpace(rvi.ReceiverID) == "" {
		return fmt.Errorf("receiver id is required")
	}
	if rvi.VerificationField == "" {
		return fmt.Errorf("verification field is required")
	}
	if strings.TrimSpace(rvi.VerificationValue) == "" {
		return fmt.Errorf("verification value is required")
	}
	return nil
}

// GetByReceiverIDsAndVerificationField returns receiver verifications by receiver IDs and verification type.
func (m *ReceiverVerificationModel) GetByReceiverIDsAndVerificationField(ctx context.Context, sqlExec db.SQLExecuter, receiverIds []string, verificationField VerificationType) ([]*ReceiverVerification, error) {
	receiverVerifications := []*ReceiverVerification{}
	query := `
		SELECT
		    *
		FROM
		    receiver_verifications
		WHERE
		    receiver_id = ANY($1) AND
		    verification_field = $2
	`
	err := sqlExec.SelectContext(ctx, &receiverVerifications, query, pq.Array(receiverIds), verificationField)
	if err != nil {
		return nil, fmt.Errorf("error querying receiver verifications: %w", err)
	}
	return receiverVerifications, nil
}

// GetAllByReceiverID returns all receiver verifications by receiver id.
func (m *ReceiverVerificationModel) GetAllByReceiverID(ctx context.Context, sqlExec db.SQLExecuter, receiverID string) ([]ReceiverVerification, error) {
	receiverVerifications := []ReceiverVerification{}
	query := `
		SELECT 
		    *
		FROM 
		    receiver_verifications
		WHERE 
		    receiver_id = $1
	`
	err := sqlExec.SelectContext(ctx, &receiverVerifications, query, receiverID)
	if err != nil {
		return nil, fmt.Errorf("error querying receiver verifications: %w", err)
	}
	return receiverVerifications, nil
}

// GetLatestByContactInfo returns the latest updated receiver verification for a receiver associated with a phone number or email.
func (m *ReceiverVerificationModel) GetLatestByContactInfo(ctx context.Context, contactInfo string) (*ReceiverVerification, error) {
	query := `
		SELECT 
			rv.*
		FROM 
			receiver_verifications rv
			JOIN receivers r ON rv.receiver_id = r.id
		WHERE 
			r.phone_number = $1
			OR r.email = $1
		ORDER BY
			rv.updated_at DESC,
			rv.verification_field ASC
		LIMIT 1
	`

	receiverVerification := ReceiverVerification{}
	err := m.dbConnectionPool.GetContext(ctx, &receiverVerification, query, contactInfo)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		truncatedContactInfo := utils.TruncateString(contactInfo, 3)
		return nil, fmt.Errorf("fetching receiver verifications for contact info %s: %w", truncatedContactInfo, err)
	}

	return &receiverVerification, nil
}

// Insert inserts a new receiver verification
func (m *ReceiverVerificationModel) Insert(ctx context.Context, sqlExec db.SQLExecuter, verificationInsert ReceiverVerificationInsert) (string, error) {
	err := verificationInsert.Validate()
	if err != nil {
		return "", fmt.Errorf("error validating receiver verification insert: %w", err)
	}
	hashedValue, err := HashVerificationValue(verificationInsert.VerificationValue)
	if err != nil {
		return "", fmt.Errorf("error hashing verification value: %w", err)
	}

	query := `
		INSERT INTO receiver_verifications (
		    receiver_id, 
		    verification_field, 
		    hashed_value
		) VALUES ($1, $2, $3)
	`

	_, err = sqlExec.ExecContext(ctx, query, verificationInsert.ReceiverID, verificationInsert.VerificationField, hashedValue)
	if err != nil {
		return "", fmt.Errorf("error inserting receiver verification: %w", err)
	}

	return hashedValue, nil
}

// UpdateVerificationValue updates the hashed value of a receiver verification.
func (m *ReceiverVerificationModel) UpdateVerificationValue(ctx context.Context,
	sqlExec db.SQLExecuter,
	receiverID string,
	verificationField VerificationType,
	verificationValue string,
) error {
	log.Ctx(ctx).Infof("Calling UpdateVerificationValue for receiver %s and verification field %s", receiverID, verificationField)
	hashedValue, err := HashVerificationValue(verificationValue)
	if err != nil {
		return fmt.Errorf("error hashing verification value: %w", err)
	}

	query := `
		UPDATE receiver_verifications
		SET hashed_value = $1
		WHERE receiver_id = $2 AND verification_field = $3
	`

	_, err = sqlExec.ExecContext(ctx, query, hashedValue, receiverID, verificationField)
	if err != nil {
		return fmt.Errorf("error updating receiver verification: %w", err)
	}

	return nil
}

// UpsertVerificationValue creates or updates the receiver's verification. Even if the verification exists and is
// already confirmed by the receiver, it will be updated.
func (m *ReceiverVerificationModel) UpsertVerificationValue(ctx context.Context, sqlExec db.SQLExecuter, userID, receiverID string, verificationField VerificationType, verificationValue string) error {
	log.Ctx(ctx).Infof("Calling UpsertVerificationValue for receiver %s and verification field %s", receiverID, verificationField)
	hashedValue, err := HashVerificationValue(verificationValue)
	if err != nil {
		return fmt.Errorf("hashing verification value: %w", err)
	}

	query := `
		INSERT INTO receiver_verifications
			(receiver_id, verification_field, hashed_value)
		VALUES
			($1, $2, $3)
		ON CONFLICT (receiver_id, verification_field)
		DO UPDATE SET
			hashed_value = EXCLUDED.hashed_value,
			-- Resetting the attempts to 0 on upsert. 
			attempts = 0,
			-- If the verification is already confirmed, the USER is updating it:
			confirmed_by_type = CASE
				WHEN receiver_verifications.confirmed_at IS NOT NULL THEN 'USER'
				ELSE receiver_verifications.confirmed_by_type
			END,
			confirmed_by_id = CASE
				WHEN receiver_verifications.confirmed_at IS NOT NULL THEN $4
				ELSE receiver_verifications.confirmed_by_id
			END
	`

	_, err = sqlExec.ExecContext(ctx, query, receiverID, verificationField, hashedValue, userID)
	if err != nil {
		return fmt.Errorf("upserting receiver verification: %w", err)
	}

	return nil
}

type ReceiverVerificationUpdate struct {
	ReceiverID          string                 `db:"receiver_id"`
	VerificationField   VerificationType       `db:"verification_field"`
	VerificationChannel message.MessageChannel `db:"verification_channel"`
	Attempts            *int                   `db:"attempts"`
	ConfirmedAt         *time.Time             `db:"confirmed_at"`
	ConfirmedByType     ConfirmedByType        `db:"confirmed_by_type"`
	ConfirmedByID       string                 `db:"confirmed_by_id"`
	FailedAt            *time.Time             `db:"failed_at"`
}

type ConfirmedByType string

const (
	ConfirmedByTypeReceiver ConfirmedByType = "RECEIVER"
	ConfirmedByTypeUser     ConfirmedByType = "USER"
)

func (rvu ReceiverVerificationUpdate) Validate() error {
	if strings.TrimSpace(rvu.ReceiverID) == "" {
		return fmt.Errorf("receiver id is required")
	}
	if rvu.VerificationField == "" {
		return fmt.Errorf("verification field is required")
	}
	if rvu.VerificationChannel == "" {
		return fmt.Errorf("verification channel is required")
	}
	return nil
}

// UpdateReceiverVerification updates the attempts, confirmed_at, and failed_at values of a receiver verification.
func (m *ReceiverVerificationModel) UpdateReceiverVerification(ctx context.Context, update ReceiverVerificationUpdate, sqlExec db.SQLExecuter) error {
	if err := update.Validate(); err != nil {
		return fmt.Errorf("validating receiver verification update: %w", err)
	}

	fields := []string{}
	args := []interface{}{}

	if update.Attempts != nil {
		fields = append(fields, "attempts = ?")
		args = append(args, update.Attempts)
	}

	if update.ConfirmedAt != nil {
		fields = append(fields, "confirmed_at = ?")
		args = append(args, update.ConfirmedAt)
	}

	if update.ConfirmedByID != "" {
		fields = append(fields, "confirmed_by_id = ?")
		args = append(args, update.ConfirmedByID)
	}

	if update.ConfirmedByType != "" {
		fields = append(fields, "confirmed_by_type = ?")
		args = append(args, update.ConfirmedByType)
	}

	if update.FailedAt != nil {
		fields = append(fields, "failed_at = ?")
		args = append(args, update.FailedAt)
	}

	query := `
		UPDATE 
			receiver_verifications
		SET 
			%s,
			verification_channel = ?
		WHERE
			receiver_id = ? AND
			verification_field = ?
	`

	args = append(args, update.VerificationChannel, update.ReceiverID, update.VerificationField)
	query = sqlExec.Rebind(fmt.Sprintf(query, strings.Join(fields, ", ")))
	_, err := sqlExec.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("updating receiver verification: %w", err)
	}

	return nil
}

// ExceededAttempts check if the number of attempts exceeded the max value.
func (*ReceiverVerificationModel) ExceededAttempts(attempts int) bool {
	return attempts >= MaxAttemptsAllowed
}

func HashVerificationValue(verificationValue string) (string, error) {
	hashedValue, err := bcrypt.GenerateFromPassword([]byte(verificationValue), bcrypt.MinCost)
	if err != nil {
		return "", fmt.Errorf("error hashing verification value: %w", err)
	}
	return string(hashedValue), nil
}

func CompareVerificationValue(hashedValue, verificationValue string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedValue), []byte(verificationValue))
	if err != nil {
		return false
	}
	return err == nil
}
