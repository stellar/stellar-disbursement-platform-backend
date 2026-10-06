package transaction

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func TestOrDash(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string returns em dash", "", "—"},
		{"non-empty returns as-is", "value", "value"},
		{"whitespace returns as-is", "  ", "  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := orDash(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestSplitWalletAddressLines(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		expected []string
	}{
		{"empty returns nil", "", nil},
		{"short address single line", "GABC123", []string{"GABC123"}},
		{"exactly break chars", longAddr(38), []string{longAddr(38)}},
		{"one char over break", longAddr(39), []string{longAddr(38), "x"}},
		{"multiple lines", longAddr(38*2 + 10), []string{longAddr(38), longAddr(38), longAddr(10)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitWalletAddressLines(tt.addr)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func longAddr(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}

func TestWalletAddressValueLines(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		expected int
	}{
		{"empty returns 1", "", 1},
		{"em dash returns 1", "—", 1},
		{"short address returns 1", "GABC", 1},
		{"over break chars returns 2", longAddr(39), 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := walletAddressValueLines(tt.addr)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestEnrichmentValue(t *testing.T) {
	e := &Enrichment{SenderName: "Alice", FeeCharged: "0.01 XLM"}

	t.Run("ok false returns empty", func(t *testing.T) {
		got := enrichmentValue(nil, false, "x")
		assert.Empty(t, got)
	})

	t.Run("e nil returns empty", func(t *testing.T) {
		got := enrichmentValue(nil, true, "x")
		assert.Empty(t, got)
	})

	t.Run("ok true and e set returns value", func(t *testing.T) {
		got := enrichmentValue(e, true, e.SenderName)
		assert.Equal(t, "Alice", got)
		got = enrichmentValue(e, true, e.FeeCharged)
		assert.Equal(t, "0.01 XLM", got)
	})
}

func TestMemoForDisplay(t *testing.T) {
	const testDBMemo = "db-memo"
	withMemo := &data.Payment{ReceiverWallet: &data.ReceiverWallet{StellarMemo: testDBMemo}}
	noMemo := &data.Payment{}
	testCases := []struct {
		name       string
		payment    *data.Payment
		enrichment *Enrichment
		expected   string
	}{
		{"horizon memo wins", withMemo, &Enrichment{MemoText: "horizon-memo"}, "horizon-memo"},
		{"falls back to the receiver wallet memo", withMemo, &Enrichment{}, testDBMemo},
		{"no enrichment at all", withMemo, nil, testDBMemo},
		{"horizon memo without a wallet memo", noMemo, &Enrichment{MemoText: "from-horizon"}, "from-horizon"},
		{"nothing to show", noMemo, nil, ""},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, memoForDisplay(tc.payment, tc.enrichment))
		})
	}
}

func TestPaymentAccessors(t *testing.T) {
	// Every accessor tolerates a payment without a receiver wallet and otherwise returns its field.
	rw := &data.ReceiverWallet{StellarAddress: "GADDR123", Receiver: data.Receiver{ExternalID: "org-123"}, Wallet: data.Wallet{Name: "Vibrant"}}
	for name, tc := range map[string]struct {
		accessor func(*data.Payment) string
		expected string
	}{
		"walletProvider":         {walletProvider, "Vibrant"},
		"recipientOrgID":         {recipientOrgID, "org-123"},
		"recipientWalletAddress": {recipientWalletAddress, "GADDR123"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Empty(t, tc.accessor(&data.Payment{}))
			assert.Equal(t, tc.expected, tc.accessor(&data.Payment{ReceiverWallet: rw}))
		})
	}
}

func TestDetailRows(t *testing.T) {
	labels := func(rows []detailRow) []string {
		out := make([]string, 0, len(rows))
		for _, r := range rows {
			out = append(out, r.label)
		}
		return out
	}
	enrichment := &Enrichment{SenderName: "redcorp", SenderAccountName: "Main account", SenderWalletAddress: "GABC"}

	t.Run("stellar payment: no Circle rows", func(t *testing.T) {
		left, right := detailRows(&data.Payment{}, enrichment)
		assert.Equal(t, []string{"Sender Name", "Distribution Account", "Sender Wallet Address", "Fee Charged"}, labels(left))
		assert.Equal(t, "Main account", left[1].value)
		assert.Equal(t, []string{"Recipient Org ID", "Recipient Wallet Address", "Wallet Provider"}, labels(right))
	})

	t.Run("circle payment: id and type rows", func(t *testing.T) {
		payoutType := data.CircleTransactionTypePayout
		p := &data.Payment{CircleTransactionID: utils.Ptr("payout-1"), CircleTransactionType: &payoutType}
		left, _ := detailRows(p, enrichment)
		require.Len(t, left, 6)
		assert.Equal(t, detailRow{"Circle Transaction ID", "payout-1", false}, left[4])
		assert.Equal(t, detailRow{"Circle Transaction Type", "PAYOUT", false}, left[5])
	})
}
