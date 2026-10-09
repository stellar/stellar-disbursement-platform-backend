package transaction

import (
	"strings"
	"testing"

	"github.com/jung-kurt/gofpdf/v2"
	"github.com/stretchr/testify/assert"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/pdf/shared"
)

func TestSplitNoteIntoLines(t *testing.T) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddUTF8FontFromBytes("Inter", "", shared.InterRegularFont)
	pdf.SetFont("Inter", "", bodyFontSize)

	testCases := map[string]struct {
		notes string
		want  []string
	}{
		"newlines start new lines":        {"line one\nline two", []string{"line one", "line two"}},
		"blank lines collapse":            {"line one\n\n\nline two", []string{"line one", "line two"}},
		"windows line endings":            {"line one\r\nline two", []string{"line one", "line two"}},
		"line of only dropped characters": {"line one\n🚀\x01\nline two", []string{"line one", "line two"}},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, splitNoteIntoLines(pdf, tc.notes, 180))
		})
	}

	t.Run("long line wraps", func(t *testing.T) {
		assert.Greater(t, len(splitNoteIntoLines(pdf, strings.Repeat("word ", 60), 50)), 1)
	})
}
