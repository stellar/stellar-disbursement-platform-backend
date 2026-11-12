package seputil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient/mocks"
)

const testSigningKey = "GBDYDBJKQBJK4GY4V7FAONSFF2IBJSKNTBYJ65F5KCGBY2BIGPGGLJOH"

func Test_SigningKeyFromStellarToml(t *testing.T) {
	testCases := []struct {
		name    string
		body    string
		wantKey string
	}{
		{"basic string", `SIGNING_KEY = "` + testSigningKey + `"` + "\n", testSigningKey},
		{"literal string", `SIGNING_KEY = '` + testSigningKey + `'` + "\n", testSigningKey},
		{"multi-line basic string, one line", `SIGNING_KEY = """` + testSigningKey + `"""` + "\n", testSigningKey},
		{"multi-line basic string, own lines", "SIGNING_KEY = \"\"\"\n" + testSigningKey + "\n\"\"\"\n", testSigningKey},
		{"multi-line basic string, CRLF", "SIGNING_KEY = \"\"\"\r\n" + testSigningKey + "\r\n\"\"\"\r\n", testSigningKey},
		{"multi-line literal string, one line", `SIGNING_KEY = '''` + testSigningKey + `'''` + "\n", testSigningKey},
		{"multi-line literal string, own lines", "SIGNING_KEY = '''\n" + testSigningKey + "\n'''\n", testSigningKey},
		{"double-quoted key", `"SIGNING_KEY" = "` + testSigningKey + `"` + "\n", testSigningKey},
		{"single-quoted key, double-quoted value", `'SIGNING_KEY' = "` + testSigningKey + `"` + "\n", testSigningKey},
		{"double-quoted key, single-quoted value", `"SIGNING_KEY" = '` + testSigningKey + `'` + "\n", testSigningKey},
		{"no spaces", `SIGNING_KEY="` + testSigningKey + `"`, testSigningKey},
		{"trailing comment", `SIGNING_KEY = "` + testSigningKey + `" # key` + "\n", testSigningKey},
		{"indented", "  \tSIGNING_KEY = \"" + testSigningKey + "\"\n", testSigningKey},
		{"BOM and CRLF", "\xef\xbb\xbfVERSION = \"2.0.0\"\r\nSIGNING_KEY = \"" + testSigningKey + "\"\r\n", testSigningKey},
		{"among other top-level keys", "VERSION = \"2.0.0\"\nACCOUNTS = [\n  \"" + testSigningKey + "\",\n]\nSIGNING_KEY = \"" + testSigningKey + "\"\n\n[DOCUMENTATION]\nORG_NAME = \"x\"\n", testSigningKey},
		{"first occurrence wins", `SIGNING_KEY = "` + testSigningKey + "\"\nSIGNING_KEY = \"GOTHER\"\n", testSigningKey},
		{"empty body", "", ""},
		{"missing", "VERSION = \"2.0.0\"\n", ""},
		{"empty value", `SIGNING_KEY = ""`, ""},
		{"unquoted value", "SIGNING_KEY = " + testSigningKey, ""},
		{"mismatched value quotes", `SIGNING_KEY = "` + testSigningKey + `'` + "\n", ""},
		{"trailing text after value", `SIGNING_KEY = "` + testSigningKey + `" junk` + "\n", ""},
		{"escape sequence in value", `SIGNING_KEY = "\` + `u0047` + testSigningKey[1:] + `"` + "\n", ""},
		{"multi-line value split across lines", "SIGNING_KEY = \"\"\"" + testSigningKey[:20] + "\n" + testSigningKey[20:] + "\"\"\"\n", ""},
		{"multi-line value with indented closer", "SIGNING_KEY = \"\"\"\n" + testSigningKey + "\n   \"\"\"\n", ""},
		{"multi-line value with line-ending backslash", "SIGNING_KEY = \"\"\"" + testSigningKey + " \\\n   \"\"\"\n", ""},
		{"only after a table header", "[DOCUMENTATION]\nSIGNING_KEY = \"" + testSigningKey + "\"\n", ""},
		{"inside a multi-line string under a table", "[DOCUMENTATION]\nORG_DESCRIPTION = \"\"\"\nSIGNING_KEY = \"" + testSigningKey + "\"\n\"\"\"\n", ""},
		{"commented out", "# SIGNING_KEY = \"" + testSigningKey + "\"\n", ""},
		{"different key", "SIGNING_KEY_OLD = \"" + testSigningKey + "\"\n", ""},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotKey, ok := SigningKeyFromStellarToml([]byte(tc.body))
			assert.Equal(t, tc.wantKey, gotKey)
			assert.Equal(t, tc.wantKey != "", ok)
		})
	}
}

// Shapes that make a TOML parser quadratic in memory or time must be rejected quickly and without allocating.
func Test_SigningKeyFromStellarToml_hostileShapes(t *testing.T) {
	deep := func(n int) string { return "a" + strings.Repeat(".a", n) }
	shapes := map[string]string{
		"deep table header":      "[" + deep(51_000) + "]\n",
		"deep array table":       "[[" + deep(51_000) + "]]\n",
		"deep dotted key":        deep(51_000) + " = 1\n",
		"nested inline tables":   "x = " + strings.Repeat("{a=", 25_000) + "1" + strings.Repeat("}", 25_000) + "\n",
		"nested arrays":          "x = " + strings.Repeat("[", 50_000) + strings.Repeat("]", 50_000) + "\n",
		"long header, many keys": "[" + strings.Repeat("a", 51_200) + "]\n" + strings.Repeat("k=1\n", 12_000),
		"many siblings":          strings.Repeat("k=1\n", 25_000),
	}
	for name, body := range shapes {
		t.Run(name, func(t *testing.T) {
			require.LessOrEqual(t, len(body), stellarTomlMaxSize+1024)
			start := time.Now()
			_, ok := SigningKeyFromStellarToml([]byte(body))
			assert.False(t, ok)
			assert.Less(t, time.Since(start), time.Second)
		})
	}
}

func Test_FetchClientDomainSigningKey(t *testing.T) {
	ctx := context.Background()
	tomlBody := "SIGNING_KEY = \"" + testSigningKey + "\"\n"
	newResponse := func(status int, body string) *http.Response {
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
	}
	requestTo := func(url string) any {
		return mock.MatchedBy(func(req *http.Request) bool { return req.URL.String() == url })
	}

	t.Run("returns the SIGNING_KEY over https", func(t *testing.T) {
		httpClient := mocks.NewHTTPClientMock(t)
		httpClient.On("Do", requestTo("https://wallet.example/.well-known/stellar.toml")).Return(newResponse(200, tomlBody), nil).Once()

		gotKey, err := FetchClientDomainSigningKey(ctx, httpClient, "wallet.example", false)
		require.NoError(t, err)
		assert.Equal(t, testSigningKey, gotKey)
	})

	t.Run("passes the context to the request", func(t *testing.T) {
		type ctxKey struct{}
		reqCtx := context.WithValue(ctx, ctxKey{}, "marker")
		httpClient := mocks.NewHTTPClientMock(t)
		httpClient.On("Do", mock.MatchedBy(func(req *http.Request) bool { return req.Context().Value(ctxKey{}) == "marker" })).
			Return(newResponse(200, tomlBody), nil).Once()

		_, err := FetchClientDomainSigningKey(reqCtx, httpClient, "wallet.example", false)
		require.NoError(t, err)
	})

	t.Run("retries over http when allowed", func(t *testing.T) {
		httpClient := mocks.NewHTTPClientMock(t)
		httpClient.On("Do", requestTo("https://wallet.example/.well-known/stellar.toml")).Return(nil, errors.New("tls handshake failed")).Once()
		httpClient.On("Do", requestTo("http://wallet.example/.well-known/stellar.toml")).Return(newResponse(200, tomlBody), nil).Once()

		gotKey, err := FetchClientDomainSigningKey(ctx, httpClient, "wallet.example", true)
		require.NoError(t, err)
		assert.Equal(t, testSigningKey, gotKey)
	})

	t.Run("does not retry over http when not allowed", func(t *testing.T) {
		httpClient := mocks.NewHTTPClientMock(t)
		httpClient.On("Do", requestTo("https://wallet.example/.well-known/stellar.toml")).Return(nil, errors.New("tls handshake failed")).Once()

		_, err := FetchClientDomainSigningKey(ctx, httpClient, "wallet.example", false)
		assert.EqualError(t, err, "fetching stellar.toml from wallet.example: http request errored: tls handshake failed")
	})

	t.Run("rejects a domain that is not a DNS name without fetching", func(t *testing.T) {
		for _, domain := range []string{"", "127.0.0.1", "wallet.example:8000", "user@wallet.example", "https://wallet.example"} {
			_, err := FetchClientDomainSigningKey(ctx, mocks.NewHTTPClientMock(t), domain, true)
			assert.ErrorContains(t, err, "invalid client_domain", domain)
		}
	})

	t.Run("rejects a non-2xx response", func(t *testing.T) {
		httpClient := mocks.NewHTTPClientMock(t)
		httpClient.On("Do", mock.Anything).Return(newResponse(404, "not found"), nil).Once()

		_, err := FetchClientDomainSigningKey(ctx, httpClient, "wallet.example", false)
		assert.EqualError(t, err, "fetching stellar.toml from wallet.example: http request failed with status code 404")
	})

	t.Run("rejects a body over the size limit", func(t *testing.T) {
		httpClient := mocks.NewHTTPClientMock(t)
		httpClient.On("Do", mock.Anything).Return(newResponse(200, tomlBody+strings.Repeat("# padding\n", 11_000)), nil).Once()

		_, err := FetchClientDomainSigningKey(ctx, httpClient, "wallet.example", false)
		assert.EqualError(t, err, fmt.Sprintf("fetching stellar.toml from wallet.example: stellar.toml response exceeds %d bytes limit", stellarTomlMaxSize))
	})

	t.Run("accepts a body at exactly the size limit", func(t *testing.T) {
		body := tomlBody + strings.Repeat("#", stellarTomlMaxSize-len(tomlBody))
		httpClient := mocks.NewHTTPClientMock(t)
		httpClient.On("Do", mock.Anything).Return(newResponse(200, body), nil).Once()

		gotKey, err := FetchClientDomainSigningKey(ctx, httpClient, "wallet.example", false)
		require.NoError(t, err)
		assert.Equal(t, testSigningKey, gotKey)
	})

	t.Run("rejects a body without SIGNING_KEY", func(t *testing.T) {
		httpClient := mocks.NewHTTPClientMock(t)
		httpClient.On("Do", mock.Anything).Return(newResponse(200, "[DOCUMENTATION]\nORG_NAME = \"x\"\n"), nil).Once()

		_, err := FetchClientDomainSigningKey(ctx, httpClient, "wallet.example", false)
		assert.EqualError(t, err, "SIGNING_KEY not present in client_domain stellar.toml")
	})

	t.Run("rejects a SIGNING_KEY that is not an account ID", func(t *testing.T) {
		httpClient := mocks.NewHTTPClientMock(t)
		httpClient.On("Do", mock.Anything).Return(newResponse(200, "SIGNING_KEY = \"not-a-key\"\n"), nil).Once()

		_, err := FetchClientDomainSigningKey(ctx, httpClient, "wallet.example", false)
		assert.EqualError(t, err, "SIGNING_KEY not-a-key is not a valid Stellar account ID")
	})

	t.Run("rejects a hostile document without parsing it", func(t *testing.T) {
		body := "[" + strings.Repeat("a", 51_200) + "]\n" + strings.Repeat("k=1\n", 12_000)
		httpClient := mocks.NewHTTPClientMock(t)
		httpClient.On("Do", mock.Anything).Return(newResponse(200, body), nil).Once()

		_, err := FetchClientDomainSigningKey(ctx, httpClient, "evil.example", false)
		assert.EqualError(t, err, "SIGNING_KEY not present in client_domain stellar.toml")
	})

	t.Run("accepts any valid account ID", func(t *testing.T) {
		kp := keypair.MustRandom()
		httpClient := mocks.NewHTTPClientMock(t)
		httpClient.On("Do", mock.Anything).Return(newResponse(200, "SIGNING_KEY = \""+kp.Address()+"\"\n"), nil).Once()

		gotKey, err := FetchClientDomainSigningKey(ctx, httpClient, "wallet.example", false)
		require.NoError(t, err)
		assert.Equal(t, kp.Address(), gotKey)
	})
}
