package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/stellar/go/support/log"
	"golang.org/x/crypto/bcrypt"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
)

type ReceiverVerification struct {
	ReceiverID        string            `db:"receiver_id"`
	VerificationField VerificationField `db:"verification_field"`
	HashedValue       string            `db:"hashed_value"`
	Attempts          int               `db:"attempts"`
	CreatedAt         time.Time         `db:"created_at"`
	UpdatedAt         time.Time         `db:"updated_at"`
	ConfirmedAt       *time.Time        `db:"confirmed_at"`
	FailedAt          *time.Time        `db:"failed_at"`
}

type ReceiverVerificationModel struct {
	dbConnectionPool db.DBConnectionPool
}

type ReceiverVerificationInsert struct {
	ReceiverID        string            `db:"receiver_id"`
	VerificationField VerificationField `db:"verification_field"`
	VerificationValue string            `db:"hashed_value"`
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
func (m *ReceiverVerificationModel) GetByReceiverIDsAndVerificationField(ctx context.Context, sqlExec db.SQLExecuter, receiverIds []string, verificationField VerificationField) ([]*ReceiverVerification, error) {
	receiverVerifications := []*ReceiverVerification{}
	query := `
		SELECT 
		    receiver_id, 
		    verification_field, 
		    hashed_value,
		    attempts,
		    created_at,
		    updated_at,
		    confirmed_at,
		    failed_at
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

// GetAllByReceiverId returns all receiver verifications by receiver id.
func (m *ReceiverVerificationModel) GetAllByReceiverId(ctx context.Context, sqlExec db.SQLExecuter, receiverId string) ([]ReceiverVerification, error) {
	receiverVerifications := []ReceiverVerification{}
	query := `
		SELECT 
		    *
		FROM 
		    receiver_verifications
		WHERE 
		    receiver_id = $1
	`
	err := sqlExec.SelectContext(ctx, &receiverVerifications, query, receiverId)
	if err != nil {
		return nil, fmt.Errorf("error querying receiver verifications: %w", err)
	}
	return receiverVerifications, nil
}

// GetLatestByPhoneNumber returns the latest updated receiver verification for some receiver that is associated with a phone number.
func (m *ReceiverVerificationModel) GetLatestByPhoneNumber(ctx context.Context, phoneNumber string) (*ReceiverVerification, error) {
	receiverVerification := ReceiverVerification{}
	query := `
		SELECT 
		    rv.*
		FROM 
		    receiver_verifications rv
		JOIN receivers r ON rv.receiver_id = r.id
		WHERE 
		    r.phone_number = $1
		ORDER BY
		    rv.updated_at DESC
		LIMIT 1
	`

	err := m.dbConnectionPool.GetContext(ctx, &receiverVerification, query, phoneNumber)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("fetching receiver verifications for phone number %s: %w", phoneNumber, err)
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
	verificationField VerificationField,
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

// UpdateVerificationValue updates the hashed value of a receiver verification.
func (m *ReceiverVerificationModel) UpdateReceiverVerification(ctx context.Context, receiverVerification ReceiverVerification, sqlExec db.SQLExecuter) error {
	query := `
		UPDATE 
			receiver_verifications
		SET 
			attempts = $1,
			confirmed_at = $2,
			failed_at = $3
		WHERE 
			receiver_id = $4 AND verification_field = $5
	`

	_, err := sqlExec.ExecContext(ctx,
		query,
		receiverVerification.Attempts,
		receiverVerification.ConfirmedAt,
		receiverVerification.FailedAt,
		receiverVerification.ReceiverID,
		receiverVerification.VerificationField,
	)
	if err != nil {
		return fmt.Errorf("error updating receiver verification: %w", err)
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
