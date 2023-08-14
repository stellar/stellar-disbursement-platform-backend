package validators

import (
	"context"
	"net/http"

	"github.com/stretchr/testify/mock"
)

type ReCAPTCHAValidatorMock struct {
	mock.Mock
}

func (v *ReCAPTCHAValidatorMock) IsTokenValid(ctx context.Context, token string) (bool, error) {
	args := v.Called(ctx, token)
	return args.Bool(0), args.Error(1)
}

type httpClientMock struct {
	mockDo func(req *http.Request) (*http.Response, error)
}

func (c *httpClientMock) Do(req *http.Request) (*http.Response, error) {
	return c.mockDo(req)
}
