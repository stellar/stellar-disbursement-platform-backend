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

	t.Run("generates codes of the exact requested length from the base36 alphabet", func(t *testing.T) {
		for _, length := range []int{1, shortCodeLength, 32} {
			code := generator.Generate(length)

			assert.Len(t, code, length)
			for _, char := range code {
				assert.Contains(t, shortCodeAlphabet, string(char), "character %q is not in the short code alphabet", char)
			}
		}
	})

	t.Run("returns empty string for non-positive lengths", func(t *testing.T) {
		assert.Empty(t, generator.Generate(0))
		assert.Empty(t, generator.Generate(-1))
	})

	t.Run("generates unique codes", func(t *testing.T) {
		codes := make(map[string]bool)
		for i := 0; i < 100; i++ {
			code := generator.Generate(shortCodeLength)
			assert.False(t, codes[code], "Duplicate code generated: %s", code)
			codes[code] = true
		}
		assert.Len(t, codes, 100)
	})

	t.Run("uses every character of the alphabet", func(t *testing.T) {
		// 7200 characters: the chance any of the 36 symbols never appears is (35/36)^7200 ≈ 1e-88.
		seen := make(map[rune]bool)
		for i := 0; i < 720; i++ {
			for _, char := range generator.Generate(shortCodeLength) {
				seen[char] = true
			}
		}
		assert.Len(t, seen, len(shortCodeAlphabet))
	})
}
