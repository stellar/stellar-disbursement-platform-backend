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
