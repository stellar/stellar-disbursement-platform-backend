package utils

import (
	"database/sql"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func Test_SQLNullString(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want sql.NullString
	}{
		{
			name: "empty string",
			arg:  "",
			want: sql.NullString{String: "", Valid: false},
		},
		{
			name: "non-empty string",
			arg:  "hello",
			want: sql.NullString{String: "hello", Valid: true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SQLNullString(tc.arg)
			assert.Equal(t, tc.want, got)
		})
	}
}

func Test_SQLNullTime(t *testing.T) {
	wantTime := time.Date(2000, 1, 2, 3, 4, 5, 6, time.UTC)
	tests := []struct {
		name string
		arg  time.Time
		want pq.NullTime
	}{
		{
			name: "zero time",
			arg:  time.Time{},
			want: pq.NullTime{Time: time.Time{}, Valid: false},
		},
		{
			name: "non-zero time",
			arg:  wantTime,
			want: pq.NullTime{Time: wantTime, Valid: true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SQLNullTime(tc.arg)
			assert.Equal(t, tc.want, got)
		})
	}
}

func Test_SQLNullNumeric(t *testing.T) {
	tests := []struct {
		name string
		arg  decimal.Decimal
		want sql.NullString
	}{
		{
			name: "zero value",
			arg:  decimal.Zero,
			want: sql.NullString{String: "0", Valid: false},
		},
		{
			name: "positive value",
			arg:  decimal.NewFromFloat(123.45),
			want: sql.NullString{String: "123.45", Valid: true},
		},
		{
			name: "negative value",
			arg:  decimal.NewFromFloat(-99.99),
			want: sql.NullString{String: "-99.99", Valid: true},
		},
		{
			name: "integer value",
			arg:  decimal.NewFromInt(100),
			want: sql.NullString{String: "100", Valid: true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SQLNullNumeric(tc.arg)
			assert.Equal(t, tc.want, got)
		})
	}
}
