package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_CalculateExponentialBackoffDuration(t *testing.T) {
	testCases := []struct {
		name         string
		retry        int
		wantDuration time.Duration
		err          error
	}{
		{
			name:         "zero retries",
			retry:        0,
			wantDuration: time.Duration(1),
		},
		{
			name:  "negative numbers",
			retry: -1,
			err:   ErrInvalidBackoffRetryValue,
		},
		{
			name:         "returns the correct duration",
			retry:        2,
			wantDuration: time.Duration(4),
		},
		{
			name:         "returns the correct duration when is the max value",
			retry:        32,
			wantDuration: time.Duration(4294967296),
		},
		{
			name:  "returns error when retry value is greater than the max",
			retry: 50,
			err:   ErrMaxRetryValueOverflow,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			backoff, err := CalculateExponentialBackoffDuration(tc.retry)
			if err != nil {
				assert.ErrorIs(t, err, tc.err)
			} else {
				assert.Equal(t, tc.wantDuration, backoff)
			}
		})
	}
}

func Test_ExponentialBackoffInSeconds(t *testing.T) {
	testCases := []struct {
		name         string
		retry        int
		wantDuration time.Duration
		err          error
	}{
		{
			name:         "zero retries",
			retry:        0,
			wantDuration: time.Second * 1,
		},
		{
			name:  "negative numbers",
			retry: -1,
			err:   ErrInvalidBackoffRetryValue,
		},
		{
			name:         "returns the correct duration",
			retry:        2,
			wantDuration: time.Second * 4,
		},
		{
			name:         "returns the correct duration when is the max value",
			retry:        32,
			wantDuration: time.Second * 4294967296,
		},
		{
			name:  "returns error when retry value is greater than the max",
			retry: 50,
			err:   ErrMaxRetryValueOverflow,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			backoff, err := ExponentialBackoffInSeconds(tc.retry)
			if err != nil {
				assert.ErrorIs(t, err, tc.err)
			} else {
				assert.Equal(t, tc.wantDuration, backoff)
			}
		})
	}
}
