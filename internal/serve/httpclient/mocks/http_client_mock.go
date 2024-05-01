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

func (h *HttpClientMock) Do(req *http.Request) (*http.Response, error) {
	args := h.Called(req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*http.Response), args.Error(1)
}

func (h *HttpClientMock) Get(url string) (*http.Response, error) {
	args := h.Called(url)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*http.Response), args.Error(1)
}

func (h *HttpClientMock) PostForm(url string, data url.Values) (*http.Response, error) {
	args := h.Called(url, data)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*http.Response), args.Error(1)
}

var (
	_ httpclient.HttpClientInterface = (*HttpClientMock)(nil)
	_ horizonclient.HTTP             = (*HttpClientMock)(nil)
)
