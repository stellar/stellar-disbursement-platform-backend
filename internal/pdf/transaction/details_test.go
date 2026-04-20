package transaction

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
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

func TestTruncateWithEllipsis(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		maxChars int
		expected string
	}{
		{"short string unchanged", "short", 10, "short"},
		{"exact length unchanged", "exact", 5, "exact"},
		{"truncate with ellipsis", "long string here", 10, "long st..."},
		{"maxChars 3 or less no ellipsis", "abc", 2, "ab"},
		{"maxChars 4 truncate to 1+ellipsis", "abcde", 4, "a..."},
		{"empty string", "", 5, ""},
		{"multi-byte runes not split", "ação中文字", 6, "açã..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateWithEllipsis(tt.s, tt.maxChars)
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

const msgNilReceiverWalletReturnsEmpty = "nil receiver wallet returns empty"

func TestMemoDisplay(t *testing.T) {
	t.Run(msgNilReceiverWalletReturnsEmpty, func(t *testing.T) {
		p := &data.Payment{}
		got := memoDisplay(p)
		assert.Empty(t, got)
	})

	t.Run("empty stellar memo returns empty", func(t *testing.T) {
		p := &data.Payment{ReceiverWallet: &data.ReceiverWallet{StellarMemo: ""}}
		got := memoDisplay(p)
		assert.Empty(t, got)
	})

	t.Run("stellar memo set returns memo", func(t *testing.T) {
		p := &data.Payment{ReceiverWallet: &data.ReceiverWallet{StellarMemo: "my-memo"}}
		got := memoDisplay(p)
		assert.Equal(t, "my-memo", got)
	})
}

const testDBMemo = "db-memo"

func TestMemoForDisplay(t *testing.T) {
	t.Run("enrichment nil uses payment memo", func(t *testing.T) {
		p := &data.Payment{ReceiverWallet: &data.ReceiverWallet{StellarMemo: testDBMemo}}
		got := memoForDisplay(p, nil)
		assert.Equal(t, testDBMemo, got)
	})

	t.Run("enrichment MemoText empty uses payment memo", func(t *testing.T) {
		p := &data.Payment{ReceiverWallet: &data.ReceiverWallet{StellarMemo: testDBMemo}}
		e := &Enrichment{MemoText: ""}
		got := memoForDisplay(p, e)
		assert.Equal(t, testDBMemo, got)
	})

	t.Run("enrichment MemoText set returns Horizon memo", func(t *testing.T) {
		p := &data.Payment{ReceiverWallet: &data.ReceiverWallet{StellarMemo: testDBMemo}}
		e := &Enrichment{MemoText: "horizon-memo"}
		got := memoForDisplay(p, e)
		assert.Equal(t, "horizon-memo", got)
	})

	t.Run("enrichment MemoText set and payment has no memo returns Horizon memo", func(t *testing.T) {
		p := &data.Payment{}
		e := &Enrichment{MemoText: "from-horizon"}
		got := memoForDisplay(p, e)
		assert.Equal(t, "from-horizon", got)
	})

	t.Run("enrichment nil and payment has no memo returns empty", func(t *testing.T) {
		p := &data.Payment{}
		got := memoForDisplay(p, nil)
		assert.Empty(t, got)
	})
}

func TestWalletProvider(t *testing.T) {
	t.Run(msgNilReceiverWalletReturnsEmpty, func(t *testing.T) {
		p := &data.Payment{}
		got := walletProvider(p)
		assert.Empty(t, got)
	})

	t.Run("wallet name empty returns empty", func(t *testing.T) {
		p := &data.Payment{ReceiverWallet: &data.ReceiverWallet{Wallet: data.Wallet{Name: ""}}}
		got := walletProvider(p)
		assert.Empty(t, got)
	})

	t.Run("wallet name set returns name", func(t *testing.T) {
		p := &data.Payment{ReceiverWallet: &data.ReceiverWallet{Wallet: data.Wallet{Name: "Vibrant"}}}
		got := walletProvider(p)
		assert.Equal(t, "Vibrant", got)
	})
}

func TestRecipientOrgID(t *testing.T) {
	t.Run(msgNilReceiverWalletReturnsEmpty, func(t *testing.T) {
		p := &data.Payment{}
		got := recipientOrgID(p)
		assert.Empty(t, got)
	})

	t.Run("receiver external_id set returns it", func(t *testing.T) {
		p := &data.Payment{ReceiverWallet: &data.ReceiverWallet{Receiver: data.Receiver{ExternalID: "org-123"}}}
		got := recipientOrgID(p)
		assert.Equal(t, "org-123", got)
	})
}

func TestRecipientWalletAddress(t *testing.T) {
	t.Run(msgNilReceiverWalletReturnsEmpty, func(t *testing.T) {
		p := &data.Payment{}
		got := recipientWalletAddress(p)
		assert.Empty(t, got)
	})

	t.Run("stellar address set returns it", func(t *testing.T) {
		p := &data.Payment{ReceiverWallet: &data.ReceiverWallet{StellarAddress: "GADDR123"}}
		got := recipientWalletAddress(p)
		assert.Equal(t, "GADDR123", got)
	})
}

func TestTransactionHeaderLayout(t *testing.T) {
	layout := transactionHeaderLayout()
	requireNotNilAndPositive := func(t *testing.T, name string, v float64) {
		t.Helper()
		assert.Greater(t, v, 0.0, "transactionHeaderLayout().%s should be positive", name)
	}
	requireNotNilAndPositive(t, "MmPerPage", layout.MmPerPage)
	requireNotNilAndPositive(t, "TableWidth", layout.TableWidth)
	requireNotNilAndPositive(t, "BodyFontSize", layout.BodyFontSize)
}
