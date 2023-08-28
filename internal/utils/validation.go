package utils

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/asaskevich/govalidator"
	"github.com/nyaruka/phonenumbers"
)

var (
	// RxPhone is a regex used to validate phone number, according with the E.164 standard https://en.wikipedia.org/wiki/E.164
	rxPhone                   = regexp.MustCompile(`^\+[1-9]{1}[0-9]{9,14}$`)
	rxOTP                     = regexp.MustCompile(`^\d{6}$`)
	ErrInvalidE164PhoneNumber = fmt.Errorf("the provided phone number is not a valid E.164 number")
)

// https://github.com/firebase/firebase-admin-go/blob/cef91acd46f2fc5d0b3408d8154a0005db5bdb0b/auth/user_mgt.go#L449-L457
func ValidatePhoneNumber(phoneNumberStr string) error {
	if phoneNumberStr == "" {
		return fmt.Errorf("phone number cannot be empty")
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
// It's free to use under the [MIT Licence](https://opensource.org/licenses/MIT)
var rxEmail = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+\\/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

func ValidateEmail(email string) error {
	if email == "" {
		return fmt.Errorf("email cannot be empty")
	}

	if !rxEmail.MatchString(email) {
		return fmt.Errorf("the provided email is not valid")
	}

	return nil
}

// ValidateDNS will validate the given string as a DNS name
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
