package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

const (
	maxCodeGenerationAttempts = 5
	shortCodeLength           = 6
)

type ShortURL struct {
	ID          string    `json:"id"`
	OriginalURL string    `json:"original_url"`
	CreatedAt   time.Time `json:"created_at"`
}

type URLShortenerModel struct {
	dbConnectionPool db.DBConnectionPool
	codeGenerator    CodeGenerator
}

func NewURLShortenerModel(db db.DBConnectionPool) *URLShortenerModel {
	return &URLShortenerModel{
		dbConnectionPool: db,
		codeGenerator:    &RandomCodeGenerator{},
	}
}

func (u *URLShortenerModel) GetOriginalURL(ctx context.Context, shortCode string) (string, error) {
	var originalURL string
	query := `SELECT original_url FROM short_urls WHERE id = $1`
	err := u.dbConnectionPool.GetContext(ctx, &originalURL, query, shortCode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrRecordNotFound
		}
		return "", fmt.Errorf("getting URL for code %s: %w", shortCode, err)
	}
	return originalURL, nil
}

func (u *URLShortenerModel) GetOrCreateShortCode(ctx context.Context, originalURL string) (string, error) {
	// Attempt to generate a unique short code.
	for attempts := 0; attempts < maxCodeGenerationAttempts; attempts++ {
		result, err := db.RunInTransactionWithResult(ctx, u.dbConnectionPool, nil, func(dbTx db.DBTransaction) (string, error) {
			// Check if there is already a short code for this original URL.
			var code string
			query := `SELECT id FROM short_urls WHERE original_url = $1`
			err := dbTx.GetContext(ctx, &code, query, originalURL)
			if err == nil {
				return code, nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return "", fmt.Errorf("checking for existing URL: %w", err)
			}

			// Generate a new short code.
			code = u.codeGenerator.Generate(shortCodeLength)

			// Insert the new short code.
			query = `
				INSERT INTO short_urls (id, original_url)
				VALUES ($1, $2)
			`
			if _, err = dbTx.ExecContext(ctx, query, code, originalURL); err != nil {
				return "", fmt.Errorf("inserting new URL: %w", err)
			}
			return code, nil
		})

		switch {
		case err == nil:
			return result, nil
		case isDuplicateError(err): // Retry if the short code already exists.
			continue
		default:
			return "", fmt.Errorf("getting or creating short code: %w", err)
		}
	}
	return "", fmt.Errorf("generating unique code after %d attempts", maxCodeGenerationAttempts)
}

// isDuplicateError checks if the error is a PostgreSQL unique violation
func isDuplicateError(err error) bool {
	var pqErr *pq.Error
	return err != nil && errors.As(err, &pqErr) && pqErr.Code == "23505"
}

//go:generate mockery --name CodeGenerator  --case=underscore --structname=CodeGeneratorMock --filename=code_generator.go
type CodeGenerator interface {
	Generate(length int) string
}

type RandomCodeGenerator struct{}

func (g *RandomCodeGenerator) Generate(length int) string {
	genUUID := uuid.New().String()
	return strings.ReplaceAll(genUUID[:length], "-", "")
}
