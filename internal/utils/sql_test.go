package utils

import (
	"database/sql"
	"testing"
	"time"

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
		want sql.NullTime
	}{
		{
			name: "zero time",
			arg:  time.Time{},
			want: sql.NullTime{Time: time.Time{}, Valid: false},
		},
		{
			name: "non-zero time",
			arg:  wantTime,
			want: sql.NullTime{Time: wantTime, Valid: true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SQLNullTime(tc.arg)
			assert.Equal(t, tc.want, got)
		})
	}
}
