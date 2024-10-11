package validators

import (
	"context"

	"github.com/stretchr/testify/mock"
)

type ReCAPTCHAValidatorMock struct {
	mock.Mock
}

func (v *ReCAPTCHAValidatorMock) IsTokenValid(ctx context.Context, token string) (bool, error) {
	args := v.Called(ctx, token)
	return args.Bool(0), args.Error(1)
}

type testInterface interface {
	mock.TestingT
	Cleanup(func())
}

// NewReCAPTCHAValidatorMock creates a new instance of ReCAPTCHAValidatorMock. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewReCAPTCHAValidatorMock(t testInterface) *ReCAPTCHAValidatorMock {
	mock := &ReCAPTCHAValidatorMock{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
