package utils

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

var ErrEmptyEmail = fmt.Errorf("email cannot be empty")

const (
	// Default charset to be used with StringWithCharset function
	DefaultCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	SpecialCharset = "!@#$%&*+-_"
	// Password charset adds special chars
	PasswordCharset = DefaultCharset + SpecialCharset
)

// Generates a random string with the charset infromed and the length
func StringWithCharset(length int, charset string) (string, error) {
	b := make([]byte, length)
	for i := range b {
		randomNumber, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", fmt.Errorf("error generating random number in StringWithCharset: %w", err)
		}
		b[i] = charset[randomNumber.Int64()]
	}
	return string(b), nil
}

// RxEmail is a regex used to validate e-mail addresses, according with the reference https://www.alexedwards.net/blog/validation-snippets-for-go#email-validation.
// It's free to use under the [MIT License](https://opensource.org/licenses/MIT)
var rxEmail = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+\\/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

func SanitizeAndValidateEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	return email, ValidateEmail(email)
}

func ValidateEmail(email string) error {
	if email == "" {
		return ErrEmptyEmail
	}

	if !rxEmail.MatchString(email) {
		return fmt.Errorf("the provided email %q is not valid", email)
	}

	return nil
}

func TruncateString(str string, borderSizeToKeep int) string {
	if len(str) <= 2*borderSizeToKeep {
		return str
	}
	return str[:borderSizeToKeep] + "..." + str[len(str)-borderSizeToKeep:]
}
