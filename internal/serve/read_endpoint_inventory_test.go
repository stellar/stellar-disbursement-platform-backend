package serve

import (
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readEndpointInventory is the executable form of the committed leak-vector matrix
// the cardinality classification: every authenticated GET route is classified.
// ADDING A READ ENDPOINT WITHOUT CLASSIFYING IT FAILS CI — this is the acceptance gate
// ("new endpoints cannot ship without a leak-vector inventory entry").
var readEndpointInventory = map[string]string{
	// MEMBERSHIP-FILTERED: rows/objects filter to the caller's wallets; individual reads 404
	// outside scope; pagination totals are post-filter.
	"GET /disbursements/":                    "membership-filtered",
	"GET /disbursements/{id}":                "membership-filtered",
	"GET /disbursements/{id}/receivers":      "membership-filtered",
	"GET /disbursements/{id}/instructions":   "membership-filtered",
	"GET /payments/":                         "membership-filtered",
	"GET /payments/{id}":                     "membership-filtered",
	"GET /receivers/":                        "membership-filtered",
	"GET /receivers/{id}":                    "membership-filtered",
	"GET /statistics/":                       "membership-filtered",
	"GET /statistics/{id}":                   "membership-filtered",
	"GET /exports/disbursements":             "membership-filtered",
	"GET /exports/payments":                  "membership-filtered",
	"GET /exports/receivers":                 "membership-filtered",
	"GET /distribution-wallets/{id}/balance": "membership-filtered",

	// TENANT-SCOPED: identical cross-wallet data for every qualifying tenant role.
	"GET /receivers/verification-types":          "tenant-scoped",
	"GET /registration-contact-types":            "tenant-scoped",
	"GET /assets/":                               "tenant-scoped",
	"GET /wallets/":                              "tenant-scoped",
	"GET /organization/":                         "tenant-scoped",
	"GET /organization/logo":                     "tenant-scoped",
	"GET /bridge-integration/":                   "tenant-scoped",
	"GET /profile/":                              "tenant-scoped",
	"GET /users/":                                "tenant-scoped",
	"GET /users/roles":                           "tenant-scoped",
	"GET /api-keys/":                             "tenant-scoped",
	"GET /api-keys/{id}":                         "tenant-scoped",
	"GET /balances":                              "tenant-scoped",
	"GET /distribution-wallets/":                 "membership-filtered", // members list their wallets; Owners all
	"GET /distribution-wallets/balance":          "membership-filtered", // scope-summed aggregate (Owner = tenant-wide)
	"GET /distribution-wallets/{id}/":            "tenant-scoped",       // Owner-only admin view
	"GET /distribution-wallets/{id}/memberships": "tenant-scoped",       // Owner-only admin/audit view

	// OUT OF SCOPE: recipient/SEP/embedded-wallet flows (PRD non-goal) and infra.
	"GET /health":                                       "out-of-scope",
	"GET /.well-known/stellar.toml":                     "out-of-scope",
	"GET /sep10/auth":                                   "out-of-scope",
	"GET /sep45/auth":                                   "out-of-scope",
	"GET /sep24/info":                                   "out-of-scope",
	"GET /sep24/transaction":                            "out-of-scope",
	"GET /sep24/transactions":                           "out-of-scope",
	"GET /embedded-wallets/{credentialID}":              "out-of-scope",
	"GET /embedded-wallets/profile":                     "out-of-scope",
	"GET /embedded-wallets/sponsored-transactions/{id}": "out-of-scope",
	"GET /app-config":                                   "out-of-scope",
	"GET /sep24-interactive-deposit/info":               "out-of-scope",
	"GET /sponsored-transactions/{id}/":                 "out-of-scope",
	"GET /r/{code}":                                     "out-of-scope",
	"GET /metrics":                                      "out-of-scope",
}

// Test_ReadEndpointInventory_CIGate walks the live route tree: every GET route must have an
// inventory entry. A new, unclassified read endpoint fails this test (and therefore CI).
func Test_ReadEndpointInventory_CIGate(t *testing.T) {
	dbConnectionPool := getConnectionPool(t)
	serveOptions := getServeOptionsForTests(t, dbConnectionPool)
	mux := handleHTTP(serveOptions)

	var unclassified []string
	walked := 0
	err := chi.Walk(mux, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method != http.MethodGet {
			return nil
		}
		// Normalize chi's wildcard suffixes for static file servers.
		if strings.HasSuffix(route, "*") {
			return nil
		}
		walked++
		key := method + " " + route
		if _, ok := readEndpointInventory[key]; !ok {
			unclassified = append(unclassified, key)
		}
		return nil
	})
	require.NoError(t, err)
	require.NotZero(t, walked, "route walk must visit GET routes")

	assert.Empty(t, unclassified,
		"UNCLASSIFIED READ ENDPOINTS — add each to the leak-vector inventory "+
			"the cardinality classification and readEndpointInventory before shipping: %v",
		unclassified)
}
