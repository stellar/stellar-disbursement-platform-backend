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

var (
	//go:embed common_passwords.txt.gz
	passwordsBinary []byte
	// singlePasswordValidator is a singleton instance of PasswordValidator that we will use to ensure
	// that we do not load multiple copies of the passwords set into memory if one already exists.
	singlePasswordValidator *PasswordValidator
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

type PasswordValidator struct {
	commonPasswordsList map[string]bool
}

// GetPasswordValidatorInstance (1) retrieves the reference for a global PasswordValidator instance if it already
// exists or (2) creates a new one, assigns it to the aforementioned global reference, and returns it.
func GetPasswordValidatorInstance() (*PasswordValidator, error) {
	if singlePasswordValidator != nil {
		return singlePasswordValidator, nil
	}

	pwValidator := PasswordValidator{}
	reader := bytes.NewReader(passwordsBinary)

	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return &pwValidator, fmt.Errorf("error creating gzip reader: %w", err)
	}
	defer gzipReader.Close()

	contents, err := io.ReadAll(gzipReader)
	if err != nil {
		return &pwValidator, fmt.Errorf("error reading contents: %w", err)
	}

	passwordsList := strings.Split(string(contents), "\n")
	commonPasswordsList := make(map[string]bool)
	for _, password := range passwordsList {
		cleanedPassword := strings.TrimSpace(strings.ToLower(password))
		commonPasswordsList[cleanedPassword] = true
	}
	pwValidator.commonPasswordsList = commonPasswordsList

	singlePasswordValidator = &pwValidator
	return &pwValidator, nil
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
	_, found := pv.commonPasswordsList[strings.ToLower(input)]
	return found
}
