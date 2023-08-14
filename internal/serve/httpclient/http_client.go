package httpclient

import (
	"net/http"
	"net/url"
	"time"

	"github.com/stellar/go/clients/horizonclient"
)

type HttpClientInterface interface {
	Do(*http.Request) (*http.Response, error)
	Get(url string) (resp *http.Response, err error)
	PostForm(url string, data url.Values) (resp *http.Response, err error)
}

const TimeoutClientInSeconds = 30

// DefaultClient returns a default HTTP client with a timeout.
func DefaultClient() HttpClientInterface {
	return &http.Client{Timeout: TimeoutClientInSeconds * time.Second}
}

var (
	_ HttpClientInterface = DefaultClient()
	_ horizonclient.HTTP  = DefaultClient()
)
