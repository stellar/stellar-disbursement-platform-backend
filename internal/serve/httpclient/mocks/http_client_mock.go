package mocks

import (
	"net/http"
	"net/url"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stretchr/testify/mock"

	httpclient "github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
)

type HttpClientMock struct {
	mock.Mock
}

type testInterface interface {
	mock.TestingT
	Cleanup(func())
}

// NewHttpClientMock creates a new instance of HttpClientMock. It also registers a testing interface on the mock and a
// cleanup function to assert the mocks expectations.
func NewHttpClientMock(t testInterface) *HttpClientMock {
	m := &HttpClientMock{}
	m.Mock.Test(t)

	t.Cleanup(func() { m.AssertExpectations(t) })

	return m
}

func (h *HttpClientMock) Do(req *http.Request) (*http.Response, error) {
	args := h.Called(req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	if fn, ok := args.Get(0).(func(*http.Request) *http.Response); ok {
		return fn(req), args.Error(1)
	}
	return args.Get(0).(*http.Response), args.Error(1)
}

func (h *HttpClientMock) Get(url string) (*http.Response, error) {
	args := h.Called(url)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	if fn, ok := args.Get(0).(func(string) *http.Response); ok {
		return fn(url), args.Error(1)
	}
	return args.Get(0).(*http.Response), args.Error(1)
}

func (h *HttpClientMock) PostForm(urlStr string, data url.Values) (*http.Response, error) {
	args := h.Called(urlStr, data)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	if fn, ok := args.Get(0).(func(string, url.Values) *http.Response); ok {
		return fn(urlStr, data), args.Error(1)
	}
	return args.Get(0).(*http.Response), args.Error(1)
}

var (
	_ httpclient.HttpClientInterface = (*HttpClientMock)(nil)
	_ horizonclient.HTTP             = (*HttpClientMock)(nil)
)
