package utils

import "github.com/stretchr/testify/mock"

type PrivateKeyEncrypterMock struct {
	mock.Mock
}

func (pke *PrivateKeyEncrypterMock) Encrypt(message, passphrase string) (string, error) {
	args := pke.Called(message, passphrase)
	return args.String(0), args.Error(1)
}

func (pke *PrivateKeyEncrypterMock) Decrypt(message, passphrase string) (string, error) {
	args := pke.Called(message, passphrase)
	return args.String(0), args.Error(1)
}

// Making sure that PrivateKeyEncrypterMock implements PrivateKeyEncrypter
var _ PrivateKeyEncrypter = (*PrivateKeyEncrypterMock)(nil)

type testInterface interface {
	mock.TestingT
	Cleanup(func())
}

func NewPrivateKeyEncrypterMock(t testInterface) *PrivateKeyEncrypterMock {
	mock := &PrivateKeyEncrypterMock{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
