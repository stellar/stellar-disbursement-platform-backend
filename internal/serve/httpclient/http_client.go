package httpclient

import (
	"net/http"
	"net/url"
	"time"

	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
)

type HTTPClientInterface interface {
	Do(*http.Request) (*http.Response, error)
	Get(url string) (resp *http.Response, err error)
	PostForm(url string, data url.Values) (resp *http.Response, err error)
}

const TimeoutClientInSeconds = 40

// DefaultClient returns a default HTTP client with a timeout.
func DefaultClient() HTTPClientInterface {
	return &http.Client{Timeout: TimeoutClientInSeconds * time.Second}
}

var (
	_ HTTPClientInterface = DefaultClient()
	_ horizonclient.HTTP  = DefaultClient()
)
