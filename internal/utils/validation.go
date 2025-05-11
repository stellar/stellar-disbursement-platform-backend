package utils

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/nyaruka/phonenumbers"
	"golang.org/x/net/html"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
)

var (
	// RxPhone is a regex used to validate phone number, according with the E.164 standard https://en.wikipedia.org/wiki/E.164
	rxPhone                   = regexp.MustCompile(`^\+[1-9]{1}[0-9]{9,14}$`)
	rxOTP                     = regexp.MustCompile(`^\d{6}$`)
	ErrInvalidE164PhoneNumber = fmt.Errorf("the provided phone number is not a valid E.164 number")
	ErrEmptyPhoneNumber       = fmt.Errorf("phone number cannot be empty")
	ErrEmptyEmail             = fmt.Errorf("email field is required")
)

const (
	VerificationFieldPinMinLength = 4
	VerificationFieldPinMaxLength = 8

	VerificationFieldMaxIdLength = 50
)

// https://github.com/firebase/firebase-admin-go/blob/cef91acd46f2fc5d0b3408d8154a0005db5bdb0b/auth/user_mgt.go#L449-L457
func ValidatePhoneNumber(phoneNumberStr string) error {
	if phoneNumberStr == "" {
		return ErrEmptyPhoneNumber
	}

	if !rxPhone.MatchString(phoneNumberStr) {
		return ErrInvalidE164PhoneNumber
	}

	parsedNumber, err := phonenumbers.Parse(phoneNumberStr, "")
	if err != nil || !phonenumbers.IsValidNumber(parsedNumber) {
		// Parsing error, not a valid phone number
		return ErrInvalidE164PhoneNumber
	}

	return nil
}

func ValidateAmount(amount string) error {
	if amount == "" {
		return fmt.Errorf("amount cannot be empty")
	}

	value, err := strconv.ParseFloat(amount, 64)
	if err != nil {
		return fmt.Errorf("the provided amount is not a valid number")
	}

	if value <= 0 {
		return fmt.Errorf("the provided amount must be greater than zero")
	}

	return nil
}

// RxEmail is a regex used to validate e-mail addresses, according with the reference https://www.alexedwards.net/blog/validation-snippets-for-go#email-validation.
// It's free to use under the [MIT Licence](https://opensource.org/licenses/MIT).
var rxEmail = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+\\/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

func ValidateEmail(email string) error {
	if email == "" {
		return ErrEmptyEmail
	}

	if !rxEmail.MatchString(email) {
		return fmt.Errorf("the email address provided is not valid")
	}

	return nil
}

// ValidateStringLength will validate the given string to ensure it is not empty and does not exceed the maximum length.
func ValidateStringLength(field, fieldName string, maxLength int) error {
	if strings.TrimSpace(field) == "" {
		return fmt.Errorf("%s field is required", fieldName)
	}

	if len(field) > maxLength {
		return fmt.Errorf("%s cannot exceed %d characters", fieldName, maxLength)
	}

	return nil
}

// ValidateDNS will validate the given string as a DNS name.
func ValidateDNS(domain string) error {
	isDNS := govalidator.IsDNSName(domain)
	if !isDNS {
		return fmt.Errorf("%q is not a valid DNS name", domain)
	}

	return nil
}

func ValidateOTP(otp string) error {
	if otp == "" {
		return fmt.Errorf("otp cannot be empty")
	}

	if !rxOTP.MatchString(otp) {
		return fmt.Errorf("the provided OTP is not a valid 6 digits value")
	}

	return nil
}

// ValidateDateOfBirthVerification will validate the date of birth field for receiver verification.
func ValidateDateOfBirthVerification(dob string) (string, error) {
	// make sure date of birth is not empty
	if dob == "" {
		return httperror.Extra_0, fmt.Errorf("date of birth cannot be empty")
	}
	// make sure date of birth with format 2006-01-02
	dateOfBrith, err := time.Parse("2006-01-02", dob)
	if err != nil {
		return httperror.Extra_2, fmt.Errorf("invalid date of birth format. Correct format: 1990-01-30")
	}

	// check if date of birth is in the past
	if dateOfBrith.After(time.Now()) {
		return httperror.Extra_4, fmt.Errorf("date of birth cannot be in the future")
	}

	return "", nil
}

// ValidateYearMonthVerification will validate the year/month field for receiver verification.
func ValidateYearMonthVerification(yearMonth string) (string, error) {
	// make sure year/month is not empty
	if yearMonth == "" {
		return httperror.Extra_0, fmt.Errorf("year/month cannot be empty")
	}

	// make sure year/month with format 2006-01
	ym, err := time.Parse("2006-01", yearMonth)
	if err != nil {
		return httperror.Extra_3, fmt.Errorf("invalid year/month format. Correct format: 1990-12")
	}

	// check if year/month is in the past
	if ym.After(time.Now()) {
		return httperror.Extra_4, fmt.Errorf("year/month cannot be in the future")
	}

	return "", nil
}

// ValidatePinVerification will validate the pin field for receiver verification.
func ValidatePinVerification(pin string) (string, error) {
	if len(pin) < VerificationFieldPinMinLength || len(pin) > VerificationFieldPinMaxLength {
		return httperror.Extra_5, fmt.Errorf("invalid pin length. Cannot have less than %d or more than %d characters in pin", VerificationFieldPinMinLength, VerificationFieldPinMaxLength)
	}

	return "", nil
}

// ValidateNationalIDVerification will validate the national id field for receiver verification.
func ValidateNationalIDVerification(nationalID string) (string, error) {
	if nationalID == "" {
		return httperror.Extra_0, fmt.Errorf("national id cannot be empty")
	}

	if len(nationalID) > VerificationFieldMaxIdLength {
		return httperror.Extra_6, fmt.Errorf("invalid national id. Cannot have more than %d characters in national id", VerificationFieldMaxIdLength)
	}

	return "", nil
}

// ValidatePathIsNotTraversal will validate the given path to ensure it does not contain path traversal.
func ValidatePathIsNotTraversal(p string) error {
	if pathTraversalPattern.MatchString(p) {
		return errors.New("path cannot contain path traversal")
	}

	return nil
}

var pathTraversalPattern = regexp.MustCompile(`(^|[\\/])\.\.([\\/]|$)`)

// ValidateURLScheme checks if a URL is valid and if it has a valid scheme.
func ValidateURLScheme(link string, scheme ...string) error {
	// Use govalidator to check if it's a valid URL
	if !govalidator.IsURL(link) {
		return errors.New("invalid URL format")
	}

	parsedURL, err := url.ParseRequestURI(link)
	if err != nil {
		return errors.New("invalid URL format")
	}

	// Check if the scheme is valid
	if len(scheme) > 0 {
		if !slices.Contains(scheme, parsedURL.Scheme) {
			return fmt.Errorf("invalid URL scheme is not part of %v", scheme)
		}
	}

	return nil
}

// ValidateNoHTML returns an error if the input contains any of the following HTML-related characters: [<, >, &, ', "],
// either in encoded or decoded form.
func ValidateNoHTML(input string) error {
	if escapedStr := html.EscapeString(input); escapedStr != input {
		return errors.New(`input contains one or more of the following HTML-related charactetes [<, >, &, ', "]`)
	}

	if unescapedStr := html.UnescapeString(input); unescapedStr != input {
		return errors.New("input contains HTML entities")
	}

	return nil
}
