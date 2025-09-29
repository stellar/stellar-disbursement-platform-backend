package data

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
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
		name                string
		shortCode           string
		expectedURL         string
		expectedErrContains string
		setup               func(*testing.T)
	}{
		{
			name:                "🎉successfully retrieves existing URL",
			shortCode:           existingCode,
			expectedURL:         originalURL,
			expectedErrContains: "",
		},
		{
			name:                "returns ErrRecordNotFound for non-existent code",
			shortCode:           "does-not-exist",
			expectedErrContains: ErrRecordNotFound.Error(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setup != nil {
				tc.setup(t)
			}

			url, err := m.GetOriginalURL(ctx, tc.shortCode)
			if tc.expectedErrContains != "" {
				assert.ErrorContains(t, err, tc.expectedErrContains)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedURL, url)
			}
		})
	}
}

func Test_URLShortenerModel_GetOrCreateShortCode(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	testCases := []struct {
		name           string
		setup          func(*testing.T, *mocks.CodeGeneratorMock, string)
		expectedErr    string
		validateResult func(*testing.T, string, string)
	}{
		{
			name: "🎉 creates new code for new URL",
			setup: func(t *testing.T, m *mocks.CodeGeneratorMock, originalURL string) {
				m.On("Generate", shortCodeLength).
					Return("abc123").
					Once()
			},
			validateResult: func(t *testing.T, originalURL, code string) {
				var actualURL string
				err := dbConnectionPool.GetContext(
					ctx,
					&actualURL,
					"SELECT original_url FROM short_urls WHERE id = $1",
					"abc123",
				)
				require.NoError(t, err)
				require.Equal(t, originalURL, actualURL)
			},
		},
		{
			name: "🎉 returns existing code for duplicate URL",
			setup: func(t *testing.T, m *mocks.CodeGeneratorMock, originalURL string) {
				CreateShortURLFixture(t, ctx, dbConnectionPool, "existing", originalURL)
			},
			validateResult: func(t *testing.T, originalURL, code string) {
				assert.Equal(t, "existing", code)
			},
		},
		{
			name: "handle collisions for new URL",
			setup: func(t *testing.T, m *mocks.CodeGeneratorMock, originalURL string) {
				m.On("Generate", shortCodeLength).
					Return("collide").
					Return("collide").
					Return("unique").
					Once()
			},
			validateResult: func(t *testing.T, originalURL, code string) {
				assert.Equal(t, "unique", code)

				var actualURL string
				err := dbConnectionPool.GetContext(
					ctx,
					&actualURL,
					"SELECT original_url FROM short_urls WHERE id = $1",
					"unique",
				)
				require.NoError(t, err)
				assert.Equal(t, originalURL, actualURL)
			},
		},
		{
			name: "max attempts exceeded",
			setup: func(t *testing.T, m *mocks.CodeGeneratorMock, originalURL string) {
				m.On("Generate", shortCodeLength).
					Return("exceed").
					Times(maxCodeGenerationAttempts)

				CreateShortURLFixture(t, ctx, dbConnectionPool, "exceed", "https://stellar.org/other")
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

			originalURL := "https://stellar.org/" + t.Name()
			if tc.setup != nil {
				tc.setup(t, generatorMock, originalURL)
			}

			code, err := model.GetOrCreateShortCode(ctx, originalURL)
			if tc.expectedErr != "" {
				assert.ErrorContains(t, err, tc.expectedErr)
				return
			}

			assert.NoError(t, err)
			if tc.validateResult != nil {
				tc.validateResult(t, originalURL, code)
			}
		})
	}
}

func Test_RandomCodeGenerator_Generate(t *testing.T) {
	generator := &RandomCodeGenerator{}

	testCases := []struct {
		name           string
		length         int
		expectedLength int
	}{
		{
			name:           "generates code of length 5",
			length:         5,
			expectedLength: 5,
		},
		{
			name:           "generates code of length 8",
			length:         8,
			expectedLength: 8,
		},
		{
			name:           "generates code of length 9 (includes first hyphen)",
			length:         9,
			expectedLength: 8,
		},
		{
			name:           "generates code of length 14 (includes two hyphens)",
			length:         14,
			expectedLength: 12,
		},
		{
			name:           "generates code of length 1",
			length:         1,
			expectedLength: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			code := generator.Generate(tc.length)
			
			assert.Equal(t, tc.expectedLength, len(code))
			assert.NotContains(t, code, "-")
			
			for _, char := range code {
				assert.True(t, (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f'),
					"Character '%c' is not a valid UUID character", char)
			}
		})
	}

	t.Run("generates unique codes", func(t *testing.T) {
		codes := make(map[string]bool)
		generator := &RandomCodeGenerator{}
		
		for i := 0; i < 100; i++ {
			code := generator.Generate(8)
			assert.False(t, codes[code], "Duplicate code generated: %s", code)
			codes[code] = true
		}
		
		assert.Equal(t, 100, len(codes))
	})

	t.Run("handles small lengths", func(t *testing.T) {
		generator := &RandomCodeGenerator{}
		
		code := generator.Generate(0)
		assert.Equal(t, 0, len(code))
		
		code = generator.Generate(32)
		assert.LessOrEqual(t, len(code), 32)
		assert.NotContains(t, code, "-")
	})
}
