package utils

import (
	"errors"
	"fmt"
	"time"
)

// MAX_RETRY_VALUE defines the max retry value. We need this to avoid memory overflow.
const MAX_RETRY_VALUE = 32

var (
	ErrInvalidBackoffRetryValue = errors.New("invalid backoff retry value")
	ErrMaxRetryValueOverflow    = errors.New("max retry value overflow")
)

// CalculateExponentialBackoffDuration returns exponential value based on the retries in time.Duration.
//
//	CalculateExponentialBackoffDuration(1) -> time.Duration(2)
//	CalculateExponentialBackoffDuration(2) -> time.Duration(4)
//	CalculateExponentialBackoffDuration(3) -> time.Duration(8)
func CalculateExponentialBackoffDuration(retry int) (time.Duration, error) {
	if retry < 0 {
		return 0, ErrInvalidBackoffRetryValue
	}

	if retry > MAX_RETRY_VALUE {
		return 0, ErrMaxRetryValueOverflow
	}

	return time.Duration(1 << retry), nil
}

// ExponentialBackoffInSeconds returns the duration in seconds based on the number of retries.
func ExponentialBackoffInSeconds(retry int) (time.Duration, error) {
	backoff, err := CalculateExponentialBackoffDuration(retry)
	if err != nil {
		return 0, fmt.Errorf("calculating exponential backoff duration: %w", err)
	}

	return time.Second * backoff, nil
}
