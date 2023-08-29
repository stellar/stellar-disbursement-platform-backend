package utils

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	passwordMinLength = 12
	passwordMaxLength = 36
)

// ValidatePasswordError is an error type that contains the failed validations specified under a map.
type ValidatePasswordError struct {
	Err                  error
	FailedValidationsMap map[string]string
}

func (e *ValidatePasswordError) Error() string {
	var failedValidations []string
	for key, value := range e.FailedValidationsMap {
		failedValidations = append(failedValidations, fmt.Sprintf("%s: %s", key, value))
	}

	return fmt.Sprintf("password validation failed for the following reason(s) [%s]", strings.Join(failedValidations, "; "))
}

func (e *ValidatePasswordError) Unwrap() error {
	return e.Err
}

func (e *ValidatePasswordError) FailedValidations() map[string]string {
	return e.FailedValidationsMap
}

// ValidatePassword returns an error if the password does not meet the requirements.
func ValidatePassword(input string) error {
	var (
		hasLength          bool
		hasLower           bool
		hasUpper           bool
		hasDigit           bool
		hasSpecial         bool
		invalidCharacteres []string
	)

	if len(input) >= passwordMinLength && len(input) <= passwordMaxLength {
		hasLength = true
	}

	for _, c := range input {
		switch {
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case unicode.IsDigit(c):
			hasDigit = true
		case unicode.IsPunct(c) || unicode.IsSymbol(c):
			hasSpecial = true
		default:
			invalidCharacteres = append(invalidCharacteres, string(c))
		}
	}

	failedValidations := map[string]string{}
	if !hasLength {
		failedValidations["length"] = fmt.Sprintf("password length must be between %d and %d characters", passwordMinLength, passwordMaxLength)
	}
	if !hasLower {
		failedValidations["lowercase"] = "password must contain at least one lowercase letter"
	}
	if !hasUpper {
		failedValidations["uppercase"] = "password must contain at least one uppercase letter"
	}
	if !hasDigit {
		failedValidations["digit"] = "password must contain at least one numberical digit"
	}
	if !hasSpecial {
		failedValidations["special character"] = "password must contain at least one special character"
	}
	if len(invalidCharacteres) > 0 {
		failedValidations["invalid character"] = fmt.Sprintf("password cannot contain any invalid characters ('%s')", strings.Join(invalidCharacteres, "', '"))
	}

	if len(failedValidations) == 0 {
		return nil
	}

	return &ValidatePasswordError{FailedValidationsMap: failedValidations}
}
