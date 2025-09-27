package mocks

import (
	"net/http"
	"net/url"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stretchr/testify/mock"

	httpclient "github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
)

type HTTPClientMock struct {
	mock.Mock
}

type testInterface interface {
	mock.TestingT
	Cleanup(func())
}

// NewHTTPClientMock creates a new instance of HTTPClientMock. It also registers a testing interface on the mock and a
// cleanup function to assert the mocks expectations.
func NewHTTPClientMock(t testInterface) *HTTPClientMock {
	m := &HTTPClientMock{}
	m.Mock.Test(t)

	t.Cleanup(func() { m.AssertExpectations(t) })

	return m
}

func (h *HTTPClientMock) Do(req *http.Request) (*http.Response, error) {
	args := h.Called(req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*http.Response), args.Error(1)
}

func (h *HTTPClientMock) Get(url string) (*http.Response, error) {
	args := h.Called(url)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*http.Response), args.Error(1)
}

func (h *HTTPClientMock) PostForm(url string, data url.Values) (*http.Response, error) {
	args := h.Called(url, data)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*http.Response), args.Error(1)
}

var (
	_ httpclient.HTTPClientInterface = (*HTTPClientMock)(nil)
	_ horizonclient.HTTP             = (*HTTPClientMock)(nil)
)
