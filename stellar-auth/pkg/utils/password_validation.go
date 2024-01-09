package utils

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"strings"
	"unicode"
)

const (
	passwordMinLength = 12
	passwordMaxLength = 36
)

//go:embed common_passwords.txt.gz
var passwordsBinary []byte

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

type PasswordValidator struct {
	commonPasswordsList []string
}

func NewPasswordValidator() (PasswordValidator, error) {
	reader := bytes.NewReader(passwordsBinary)

	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return PasswordValidator{}, fmt.Errorf("error creating gzip reader: %w", err)
	}
	defer gzipReader.Close()

	contents, err := io.ReadAll(gzipReader)
	if err != nil {
		return PasswordValidator{}, fmt.Errorf("error reading contents: %w", err)
	}

	passwordsList := strings.Split(string(contents), "\n")
	return PasswordValidator{
		commonPasswordsList: passwordsList,
	}, nil
}

// ValidatePassword returns an error if the password does not meet the requirements.
func (pv *PasswordValidator) ValidatePassword(input string) error {
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
	if !hasLength {
		failedValidations["length"] = fmt.Sprintf("password length must be between %d and %d characters", passwordMinLength, passwordMaxLength)
	}

	if len(failedValidations) == 0 {
		if pv.determineIfCommonPassword(input) {
			failedValidations["common password"] = "password is determined to be too common"
		} else {
			return nil
		}
	}

	return &ValidatePasswordError{FailedValidationsMap: failedValidations}
}

func (pv *PasswordValidator) determineIfCommonPassword(input string) bool {
	input = strings.ToLower(input)
	for _, commonPassword := range pv.commonPasswordsList {
		if input == commonPassword {
			return true
		}
	}

	return false
}
