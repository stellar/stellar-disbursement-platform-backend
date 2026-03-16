package validators

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_StatementQueryValidator_ValidateAndGetStatementParams(t *testing.T) {
	t.Run("valid params", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/statements?asset_code=XLM&from_date=2026-01-01&to_date=2026-01-31", nil)
		require.NoError(t, err)
		v := NewStatementQueryValidator()
		params := v.ValidateAndGetStatementParams(req)
		assert.False(t, v.HasErrors())
		assert.Equal(t, "XLM", params.AssetCode)
		assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), params.FromDate)
		assert.Equal(t, time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC), params.ToDate)
	})

	t.Run("missing asset_code is valid (all assets)", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/statements?from_date=2026-01-01&to_date=2026-01-31", nil)
		require.NoError(t, err)
		v := NewStatementQueryValidator()
		params := v.ValidateAndGetStatementParams(req)
		assert.False(t, v.HasErrors())
		assert.Equal(t, "", params.AssetCode)
		assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), params.FromDate)
		assert.Equal(t, time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC), params.ToDate)
	})

	t.Run("missing from_date", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/statements?asset_code=XLM&to_date=2026-01-31", nil)
		require.NoError(t, err)
		v := NewStatementQueryValidator()
		_ = v.ValidateAndGetStatementParams(req)
		assert.True(t, v.HasErrors())
		assert.Equal(t, "from_date is required", v.Errors["from_date"])
	})

	t.Run("missing to_date", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/statements?asset_code=XLM&from_date=2026-01-01", nil)
		require.NoError(t, err)
		v := NewStatementQueryValidator()
		_ = v.ValidateAndGetStatementParams(req)
		assert.True(t, v.HasErrors())
		assert.Equal(t, "to_date is required", v.Errors["to_date"])
	})

	t.Run("invalid from_date format", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/statements?asset_code=XLM&from_date=2026-13-01&to_date=2026-01-31", nil)
		require.NoError(t, err)
		v := NewStatementQueryValidator()
		_ = v.ValidateAndGetStatementParams(req)
		assert.True(t, v.HasErrors())
		assert.Equal(t, "invalid date format. valid format is 'YYYY-MM-DD'", v.Errors["from_date"])
	})

	t.Run("invalid to_date format", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/statements?asset_code=XLM&from_date=2026-01-01&to_date=not-a-date", nil)
		require.NoError(t, err)
		v := NewStatementQueryValidator()
		_ = v.ValidateAndGetStatementParams(req)
		assert.True(t, v.HasErrors())
		assert.Equal(t, "invalid date format. valid format is 'YYYY-MM-DD'", v.Errors["to_date"])
	})

	t.Run("from_date after to_date", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/statements?asset_code=XLM&from_date=2026-01-31&to_date=2026-01-01", nil)
		require.NoError(t, err)
		v := NewStatementQueryValidator()
		_ = v.ValidateAndGetStatementParams(req)
		assert.True(t, v.HasErrors())
		assert.Equal(t, "from_date must be before or equal to to_date", v.Errors["from_date"])
	})

	t.Run("from_date equals to_date is valid", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/statements?asset_code=USD&from_date=2026-01-15&to_date=2026-01-15", nil)
		require.NoError(t, err)
		v := NewStatementQueryValidator()
		params := v.ValidateAndGetStatementParams(req)
		assert.False(t, v.HasErrors())
		assert.Equal(t, "USD", params.AssetCode)
		assert.Equal(t, params.FromDate, params.ToDate)
	})
}
