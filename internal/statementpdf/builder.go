package statementpdf

import (
	"bytes"
	"fmt"
	"time"

	"github.com/jung-kurt/gofpdf/v2"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

const (
	mmPerPage       = 210.0
	pageHeight      = 297.0
	marginLR        = 15.0
	marginTop       = 15.0
	marginBottom    = 20.0
	rowHeight       = 6.0
	headerFontSize  = 14.0
	bodyFontSize    = 9.0
	tableHeaderSize = 10.0
)

// column widths (mm) for table: Date | Type | Amount | Counterparty | Wallet ID
var colWidths = []float64{38, 18, 28, 75, 26}

// BuildPDF generates a multi-page PDF from a StatementResult and returns the bytes.
func BuildPDF(result *services.StatementResult, fromDate, toDate time.Time) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(marginLR, marginTop, marginLR)
	pdf.SetAutoPageBreak(true, marginBottom)
	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)
		pdf.SetFont("Helvetica", "", 8)
		pdf.CellFormat(0, 6, fmt.Sprintf("Generated on %s", time.Now().UTC().Format(time.RFC3339)), "", 0, "C", false, 0, "")
	})
	pdf.AddPage()

	// Title and period
	pdf.SetFont("Helvetica", "B", 16)
	pdf.CellFormat(0, 10, "Account Statement", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", bodyFontSize)
	periodStr := fmt.Sprintf("Period: %s to %s", fromDate.Format("2006-01-02"), toDate.Format("2006-01-02"))
	pdf.CellFormat(0, 6, periodStr, "", 1, "L", false, 0, "")
	pdf.Ln(2)

	// Summary block
	pdf.SetFont("Helvetica", "B", bodyFontSize)
	pdf.CellFormat(0, 6, "Summary", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", bodyFontSize)
	pdf.CellFormat(0, 5, "Account: "+result.Summary.Account, "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 5, "Asset: "+result.Summary.Asset.Code, "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 5, "Beginning balance: "+result.Summary.BeginningBalance, "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 5, "Total credits: "+result.Summary.TotalCredits, "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 5, "Total debits: "+result.Summary.TotalDebits, "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 5, "Ending balance: "+result.Summary.EndingBalance, "", 1, "L", false, 0, "")
	pdf.Ln(4)

	// Table header
	drawTableHeader(pdf)
	pageBottom := pageHeight - marginBottom

	for _, tx := range result.Transactions {
		// Check if we need a new page before this row
		if pdf.GetY()+rowHeight > pageBottom {
			pdf.AddPage()
			drawTableHeader(pdf)
		}
		drawTableRow(pdf, &tx)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("writing PDF: %w", err)
	}
	return buf.Bytes(), nil
}

func drawTableHeader(pdf *gofpdf.Fpdf) {
	pdf.SetFont("Helvetica", "B", tableHeaderSize)
	pdf.SetFillColor(230, 230, 230)
	headers := []string{"Date", "Type", "Amount", "Counterparty", "Wallet ID"}
	for i, h := range headers {
		pdf.CellFormat(colWidths[i], rowHeight+1, h, "1", 0, "L", true, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetFont("Helvetica", "", bodyFontSize)
	pdf.SetFillColor(255, 255, 255)
}

func drawTableRow(pdf *gofpdf.Fpdf, tx *services.StatementTransaction) {
	// Format date for display (shorten if needed)
	dateStr := tx.CreatedAt
	if len(dateStr) >= 19 {
		dateStr = dateStr[:10] + " " + dateStr[11:19]
	}
	counterparty := tx.CounterpartyName
	if counterparty == "" {
		counterparty = truncate(tx.CounterpartyAddress, 32)
	} else {
		counterparty = truncate(counterparty, 32)
	}
	walletID := truncate(tx.WalletID, 12)

	pdf.CellFormat(colWidths[0], rowHeight, dateStr, "1", 0, "L", false, 0, "")
	pdf.CellFormat(colWidths[1], rowHeight, tx.Type, "1", 0, "L", false, 0, "")
	pdf.CellFormat(colWidths[2], rowHeight, tx.Amount, "1", 0, "R", false, 0, "")
	pdf.CellFormat(colWidths[3], rowHeight, counterparty, "1", 0, "L", false, 0, "")
	pdf.CellFormat(colWidths[4], rowHeight, walletID, "1", 1, "L", false, 0, "")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
