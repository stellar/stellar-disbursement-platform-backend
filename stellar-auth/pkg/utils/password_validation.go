package utils

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	passwordMinLength = 8
	lowercasePattern  = `[a-z]`
	uppercasePattern  = `[A-Z]`
	digitsPattern     = `[0-9]`
	symbolsPattern    = `[!@#$%^&*]`
)

// ValidatePassword returns an error if the password does not meet the requirements.
func ValidatePassword(input string) error {
	if len(input) < passwordMinLength {
		return fmt.Errorf("password must have at least %d characters", passwordMinLength)
	}

	matchingPatterns := map[string]string{
		lowercasePattern: "lowercase letter",
		uppercasePattern: "uppercase letter",
		digitsPattern:    "digit",
		symbolsPattern:   "symbol",
	}

	const prefixErrStr = "password must contain at least one: "
	errorStr := prefixErrStr

	for pattern, patternErr := range matchingPatterns {
		matched, err := regexp.MatchString(pattern, input)
		if err != nil {
			return fmt.Errorf("error matching pattern %s", pattern)
		}
		if !matched {
			errorStr += patternErr + ", "
		}
	}

	if errorStr != prefixErrStr {
		return fmt.Errorf(strings.Trim(errorStr, ", "))
	} else { // even if password meets the above requirements, we still have to check for invalid characters
		matchInvalidCharacters := fmt.Sprintf("^(.*%s.*%s.*%s.*%s.*)$", lowercasePattern, uppercasePattern, digitsPattern, symbolsPattern)
		match, err := regexp.MatchString(matchInvalidCharacters, input)
		if err != nil {
			return fmt.Errorf("cannot match password to invalid characters regex: %w", err)
		}
		if !match {
			return fmt.Errorf("password contains invalid characters")
		}
	}

	return nil
}
