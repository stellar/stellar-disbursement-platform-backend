package data

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data/mocks"
)

func Test_URLShortenerModel_GetOriginalURL(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	m := URLShortenerModel{dbConnectionPool: dbConnectionPool}

	existingCode := "exist123"
	originalURL := "https://stellar.org/original"
	CreateShortURLFixture(t, ctx, dbConnectionPool, existingCode, originalURL)

	testCases := []struct {
		name          string
		shortCode     string
		expectedURL   string
		expectedError error
		before        func(*testing.T)
	}{
		{
			name:          "🎉successfully retrieves existing URL",
			shortCode:     existingCode,
			expectedURL:   originalURL,
			expectedError: nil,
		},
		{
			name:          "returns ErrRecordNotFound for non-existent code",
			shortCode:     "does-not-exist",
			expectedError: ErrRecordNotFound,
		},
		{
			name:      "returns error on database failure",
			shortCode: existingCode,
			before: func(t *testing.T) {
				// Force a context cancellation
				ctxWithCancel, cancel := context.WithCancel(context.Background())
				cancel()
				_, err := m.GetOriginalURL(ctxWithCancel, existingCode)
				require.ErrorContains(t, err, "getting URL for code exist123")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.before != nil {
				tc.before(t)
				return
			}

			url, err := m.GetOriginalURL(ctx, tc.shortCode)
			if tc.expectedError != nil {
				require.ErrorIs(t, err, tc.expectedError)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedURL, url)
			}
		})
	}
}

func Test_URLShortenerModel_CreateShortURL(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	originalURL := "https://stellar.org/example"

	testCases := []struct {
		name           string
		setup          func(*testing.T, *mocks.CodeGeneratorMock)
		expectedErr    string
		validateResult func(*testing.T, string)
	}{
		{
			name: "🎉 successful short url creation",
			setup: func(t *testing.T, m *mocks.CodeGeneratorMock) {
				m.On("Generate", shortCodeLength).
					Return("abc123").
					Once()
			},
			validateResult: func(t *testing.T, code string) {
				var storedURL string
				err := dbConnectionPool.GetContext(
					ctx,
					&storedURL,
					"SELECT original_url FROM short_urls WHERE id = $1",
					"abc123",
				)
				require.NoError(t, err)
				require.Equal(t, originalURL, storedURL)
			},
		},
		{
			name: "handle collisions",
			setup: func(t *testing.T, m *mocks.CodeGeneratorMock) {
				m.On("Generate", shortCodeLength).
					Return("collide").
					Return("collide").
					Return("unique").
					Once()

				CreateShortURLFixture(t, ctx, dbConnectionPool, "collide", originalURL)
			},
			validateResult: func(t *testing.T, code string) {
				require.Equal(t, "unique", code)
			},
		},
		{
			name: "max attempts exceeded",
			setup: func(t *testing.T, m *mocks.CodeGeneratorMock) {
				m.On("Generate", shortCodeLength).
					Return("exceed").
					Times(maxCodeGenerationAttempts)

				CreateShortURLFixture(t, ctx, dbConnectionPool, "exceed", originalURL)
			},
			expectedErr: "generating unique code after 5 attempts",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			generatorMock := mocks.NewCodeGeneratorMock(t)

			model := &URLShortenerModel{
				dbConnectionPool: dbConnectionPool,
				codeGenerator:    generatorMock,
			}

			if tc.setup != nil {
				tc.setup(t, generatorMock)
			}

			code, err := model.CreateShortURL(ctx, originalURL)
			if tc.expectedErr != "" {
				require.ErrorContains(t, err, tc.expectedErr)
				return
			}

			require.NoError(t, err)
			if tc.validateResult != nil {
				tc.validateResult(t, code)
			}
		})
	}
}

func Test_URLShortenerModel_IncrementHits(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	m := URLShortenerModel{dbConnectionPool: dbConnectionPool}

	testCases := []struct {
		name          string
		shortCode     string
		setup         func(*testing.T)
		expectedHits  int64
		expectedError string
		before        func(*testing.T)
	}{
		{
			name:      "🎉 successfully increments hits for existing code",
			shortCode: "valid123",
			setup: func(t *testing.T) {
				CreateShortURLFixture(t, ctx, dbConnectionPool, "valid123", "https://stellar.org")
				for i := 0; i < 5; i++ {
					require.NoError(t, m.IncrementHits(ctx, "valid123"))
				}
			},
			expectedHits: 6,
		},
		{
			name:          "no error for non-existent code",
			shortCode:     "does-not-exist",
			expectedHits:  0,
			expectedError: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setup != nil {
				tc.setup(t)
			}

			if tc.before != nil {
				tc.before(t)
				return
			}

			err := m.IncrementHits(ctx, tc.shortCode)
			if tc.expectedError != "" {
				require.ErrorContains(t, err, tc.expectedError)
			} else {
				require.NoError(t, err)
			}

			if tc.expectedHits > 0 {
				var hits int64
				err := dbConnectionPool.GetContext(
					ctx,
					&hits,
					"SELECT hits FROM short_urls WHERE id = $1",
					tc.shortCode,
				)
				require.NoError(t, err)
				require.Equal(t, tc.expectedHits, hits)
			}
		})
	}
}
