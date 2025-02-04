package data

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

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
	Hits        int64     `json:"hits"`
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
	var url string
	query := `SELECT original_url FROM short_urls WHERE id = $1`
	err := u.dbConnectionPool.GetContext(ctx, &url, query, shortCode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrRecordNotFound
		}
		return "", fmt.Errorf("getting URL for code %s: %w", shortCode, err)
	}
	return url, nil
}

func (u *URLShortenerModel) CreateShortURL(ctx context.Context, url string) (string, error) {
	var code string
	var attempts int

	for attempts < maxCodeGenerationAttempts {
		code = u.codeGenerator.Generate(shortCodeLength)
		err := u.insertURL(ctx, code, url)

		if err == nil {
			return code, nil
		}

		if !isDuplicateError(err) {
			return "", fmt.Errorf("creating short URL: %w", err)
		}

		attempts++
	}

	return "", fmt.Errorf("generating unique code after %d attempts", maxCodeGenerationAttempts)
}

func (u *URLShortenerModel) IncrementHits(ctx context.Context, code string) error {
	query := `UPDATE short_urls SET hits = hits + 1 WHERE id = $1`
	_, err := u.dbConnectionPool.ExecContext(ctx, query, code)
	return err
}

func (u *URLShortenerModel) insertURL(ctx context.Context, code, url string) error {
	query := `
		INSERT INTO short_urls (id, original_url)
		VALUES ($1, $2)
	`
	_, err := u.dbConnectionPool.ExecContext(ctx, query, code, url)
	return err
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
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate random bytes: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)[:length]
}
