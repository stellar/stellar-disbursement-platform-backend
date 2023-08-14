package auth

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const (
	minPasswordLength = 8
	maxPasswordLength = 16
)

var ErrPasswordTooShort = errors.New("password should have at least 8 characters")

// PasswordEncrypter is a interface that defines the methods to encrypt passwords and compare a password with its stored hash.
// This interface is used by `DefaultAuthenticator` as the type of `passwordEncrypter` attribute.
type PasswordEncrypter interface {
	// Encrypt encrypts the `password` and return a hash.
	Encrypt(ctx context.Context, password string) (string, error)

	// ComparePassword compares the `encryptedPassword` with the plain `password` to verify if it's correct.
	ComparePassword(ctx context.Context, encryptedPassword, password string) (bool, error)
}

// DefaultPasswordEncrypter defines the default way of encrypting passwords and comparing passwords with its stored hash.
// It uses `bcrypt` library to handle with the encryption and comparison.
type DefaultPasswordEncrypter struct{}

func (e *DefaultPasswordEncrypter) Encrypt(ctx context.Context, password string) (string, error) {
	// Assumes that a password can't have less than 8 characters.
	if len(password) < minPasswordLength {
		return "", ErrPasswordTooShort
	}

	encryptedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("encrypting password: %w", err)
	}

	return string(encryptedPassword), nil
}

func (e *DefaultPasswordEncrypter) ComparePassword(ctx context.Context, encryptedPassword, password string) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(encryptedPassword), []byte(password))
	if err != nil && !errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return false, fmt.Errorf("comparing encrypted password and password: %w", err)
	}
	return err == nil, nil
}

func NewDefaultPasswordEncrypter() *DefaultPasswordEncrypter {
	return &DefaultPasswordEncrypter{}
}

var _ PasswordEncrypter = (*DefaultPasswordEncrypter)(nil)
