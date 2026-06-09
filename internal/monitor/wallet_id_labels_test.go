package monitor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test_DisbursementLabels_WalletID asserts the W4 cardinality decision (accepted flag M1/M2):
// the business disbursements counter carries the DISTRIBUTION wallet UUID as `wallet_id`,
// coexisting with — and distinct from — the legacy `wallet` (recipient-provider name) label.
func Test_DisbursementLabels_WalletID(t *testing.T) {
	labels := DisbursementLabels{
		Asset:    "USDC",
		Wallet:   "Vibrant Assist",
		WalletID: "44444444-4444-4444-4444-444444444444",
		CommonLabels: CommonLabels{
			TenantName: "tenant-x",
		},
	}

	m := labels.ToMap()
	assert.Equal(t, "Vibrant Assist", m["wallet"], "recipient-provider label unchanged")
	assert.Equal(t, "44444444-4444-4444-4444-444444444444", m["wallet_id"], "distribution wallet label added")
	assert.Equal(t, "USDC", m["asset"])
	assert.Equal(t, "tenant-x", m["tenant_name"])
	assert.Len(t, m, 4)
}
