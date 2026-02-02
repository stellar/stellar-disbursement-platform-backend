package pdf

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/jung-kurt/gofpdf/v2"
	"github.com/shopspring/decimal"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

//go:embed assets/fonts/Inter_24pt-Regular.ttf
var interRegularFont []byte

//go:embed assets/fonts/Inter_24pt-Bold.ttf
var interBoldFont []byte

const (
	mmPerPage       = 210.0
	pageHeight      = 297.0
	marginLR        = 15.0
	marginTop       = 15.0
	marginBottom    = 20.0
	headerFontSize  = 14.0
	bodyFontSize    = 9.0
	tableHeaderSize = 10.0
	sectionTitleSize = 12.0
)

const multiCurrencyNote = "Note: If your wallet holds multiple currencies, please download separate statements for each currency."

// summaryColWidths: Wallet Address | Beginning Balance | Total Credits | Total Debits | Ending Balance
var summaryColWidths = []float64{35, 32, 32, 32, 32}

// txColWidths: Date | Operation ID | Recipient/Sender | Debits | Credits | Balance
var txColWidths = []float64{22, 28, 55, 28, 28, 28}

var blueColor = []int{0, 51, 153}

// Border and background colors (RGB)
var headerBorderColor = []int{209, 213, 220}   // #D1D5DC
var rowBorderColor = []int{229, 231, 235}     // #E5E7EB
var totalsRowBgColor = []int{249, 250, 251}   // #F9FAFB

// Row heights: content + vertical padding (12px ≈ 4.23mm, 20px ≈ 7.06mm)
const summaryHeaderRowHeight = 12.0  // ~12px padding top+bottom
const summaryDataRowHeight = 20.0   // ~20px padding top+bottom
const txHeaderRowHeight = 12.0      // ~12px padding top+bottom
const txDataRowHeight = 23.0         // two-line content + ~20px padding

// BuildPDF generates a multi-page PDF from a StatementResult and returns the bytes.
func BuildPDF(result *services.StatementResult, fromDate, toDate time.Time) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	
	// Register Inter fonts
	pdf.AddUTF8FontFromBytes("Inter", "", interRegularFont)
	pdf.AddUTF8FontFromBytes("Inter", "B", interBoldFont)
	
	pdf.SetMargins(marginLR, marginTop, marginLR)
	pdf.SetAutoPageBreak(true, marginBottom)
	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)
		pdf.SetFont("Inter", "", 8)
		pdf.CellFormat(0, 6, fmt.Sprintf("Generated on %s", time.Now().UTC().Format(time.RFC3339)), "", 0, "C", false, 0, "")
	})
	pdf.AddPage()

	assetCode := result.Summary.Asset.Code

	// --- Account Summary ---
	pdf.SetFont("Inter", "B", sectionTitleSize)
	pdf.CellFormat(0, 8, "Account Summary", "", 1, "L", false, 0, "")
	pdf.Ln(1)

	// Summary table header: no background, right-align, bottom border 2px #D1D5DC, 12px vertical padding, break by words
	pdf.SetFont("Inter", "B", bodyFontSize)
	pdf.SetDrawColor(headerBorderColor[0], headerBorderColor[1], headerBorderColor[2])
	pdf.SetLineWidth(0.53) // ~2px
	ySummaryHeaderStart := pdf.GetY()
	xPos := pdf.GetX()
	maxY := ySummaryHeaderStart
	for i, h := range []string{"Wallet Address", "Beginning Balance", "Total Credits", "Total Debits", "Ending Balance"} {
		pdf.SetXY(xPos, ySummaryHeaderStart)
		pdf.MultiCell(summaryColWidths[i], 4, breakHeaderWords(h), "B", "R", false)
		if pdf.GetY() > maxY {
			maxY = pdf.GetY()
		}
		xPos += summaryColWidths[i]
	}
	nextY := ySummaryHeaderStart + summaryHeaderRowHeight
	if maxY > nextY {
		nextY = maxY
	}
	pdf.SetY(nextY)
	pdf.SetLineWidth(0.25)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetFont("Inter", "", bodyFontSize)

	// Summary table data row: border bottom 1px #E5E7EB, 20px vertical padding, amounts to 2 decimals
	walletAddr := result.Summary.Account
	if strings.HasPrefix(walletAddr, "stellar:") {
		walletAddr = walletAddr[8:]
	}
	walletAddr = utils.TruncateString(walletAddr, 5)

	pdf.SetDrawColor(rowBorderColor[0], rowBorderColor[1], rowBorderColor[2])
	pdf.SetLineWidth(0.26) // ~1px
	pdf.CellFormat(summaryColWidths[0], summaryDataRowHeight, walletAddr, "B", 0, "L", false, 0, "")
	pdf.CellFormat(summaryColWidths[1], summaryDataRowHeight, utils.FormatAmountTo2Decimals(result.Summary.BeginningBalance)+" "+assetCode, "B", 0, "R", false, 0, "")
	pdf.CellFormat(summaryColWidths[2], summaryDataRowHeight, utils.FormatAmountTo2Decimals(result.Summary.TotalCredits)+" "+assetCode, "B", 0, "R", false, 0, "")
	pdf.CellFormat(summaryColWidths[3], summaryDataRowHeight, utils.FormatAmountTo2Decimals(result.Summary.TotalDebits)+" "+assetCode, "B", 0, "R", false, 0, "")
	pdf.SetTextColor(blueColor[0], blueColor[1], blueColor[2])
	pdf.CellFormat(summaryColWidths[4], summaryDataRowHeight, utils.FormatAmountTo2Decimals(result.Summary.EndingBalance)+" "+assetCode, "B", 1, "R", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.SetLineWidth(0.25)
	pdf.SetDrawColor(0, 0, 0)

	pdf.Ln(2)
	pdf.SetFont("Inter", "", bodyFontSize)
	pdf.MultiCell(0, 5, multiCurrencyNote, "", "L", false)
	pdf.Ln(4)

	// --- Transactions ---
	pdf.SetFont("Inter", "B", sectionTitleSize)
	pdf.CellFormat(0, 8, "Transactions", "", 1, "L", false, 0, "")
	pdf.Ln(1)

	pageBottom := pageHeight - marginBottom
	drawTxTableHeader(pdf)

	// Running balance: parse beginning balance once
	runningBalance, err := decimal.NewFromString(result.Summary.BeginningBalance)
	if err != nil {
		runningBalance = decimal.Zero
	}

	for _, tx := range result.Transactions {
		if pdf.GetY()+txDataRowHeight > pageBottom {
			pdf.AddPage()
			drawTxTableHeader(pdf)
		}
		runningBalance = drawTxRow(pdf, &tx, assetCode, runningBalance)
	}

	// Totals row
	if pdf.GetY()+txDataRowHeight > pageBottom {
		pdf.AddPage()
		drawTxTableHeader(pdf)
	}
	drawTotalsRow(pdf, result, assetCode)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("writing PDF: %w", err)
	}
	return buf.Bytes(), nil
}

func drawTxTableHeader(pdf *gofpdf.Fpdf) {
	pdf.SetFont("Inter", "B", tableHeaderSize)
	pdf.SetDrawColor(headerBorderColor[0], headerBorderColor[1], headerBorderColor[2])
	pdf.SetLineWidth(0.53) // ~2px
	yStart := pdf.GetY()
	xPos := pdf.GetX()
	maxY := yStart
	for i, h := range []string{"Date", "Operation ID", "Recipient/Sender", "Debits", "Credits", "Balance"} {
		pdf.SetXY(xPos, yStart)
		pdf.MultiCell(txColWidths[i], 4, breakHeaderWords(h), "B", "R", false)
		if pdf.GetY() > maxY {
			maxY = pdf.GetY()
		}
		xPos += txColWidths[i]
	}
	nextY := yStart + txHeaderRowHeight
	if maxY > nextY {
		nextY = maxY
	}
	pdf.SetY(nextY)
	pdf.SetLineWidth(0.25)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetFont("Inter", "", bodyFontSize)
}

// drawTxRow draws one transaction row and returns the new running balance after this transaction.
func drawTxRow(pdf *gofpdf.Fpdf, tx *services.StatementTransaction, assetCode string, runningBalance decimal.Decimal) decimal.Decimal {
	dateStr := tx.CreatedAt
	if len(dateStr) >= 10 {
		dateStr = dateStr[:10]
	}
	opID := utils.TruncateToMaxLength(tx.ID, 16)

	// Recipient/Sender: line1 = address, line2 = "Sender" or "Recipient" • name
	line1 := utils.TruncateString(tx.CounterpartyAddress, 5)
	role := "Recipient"
	if tx.Type == "credit" {
		role = "Sender"
	}
	name := tx.CounterpartyName
	if name == "" {
		name = "—"
	}
	line2 := role + " • " + name
	if len(line2) > 30 {
		line2 = utils.TruncateToMaxLength(line2, 30)
	}

	debitsStr := ""
	creditsStr := ""
	amount, _ := decimal.NewFromString(tx.Amount)
	amountStr := utils.FormatAmountTo2Decimals(tx.Amount)
	if tx.Type == "debit" {
		debitsStr = amountStr + " " + assetCode
		runningBalance = runningBalance.Sub(amount)
	} else {
		creditsStr = amountStr + " " + assetCode
		runningBalance = runningBalance.Add(amount)
	}
	balanceStr := utils.FormatDecimal(runningBalance) + " " + assetCode

	// Draw cells: border bottom 1px #E5E7EB, 20px vertical padding
	pdf.SetDrawColor(rowBorderColor[0], rowBorderColor[1], rowBorderColor[2])
	pdf.SetLineWidth(0.26) // ~1px
	xStart := pdf.GetX()
	yStart := pdf.GetY()

	pdf.CellFormat(txColWidths[0], txDataRowHeight, dateStr, "B", 0, "L", false, 0, "")
	pdf.CellFormat(txColWidths[1], txDataRowHeight, opID, "B", 0, "L", false, 0, "")

	// Recipient/Sender: use MultiCell so we get two lines; then advance Y to match row height and reset X for next column
	pdf.MultiCell(txColWidths[2], 4, line1+"\n"+line2, "B", "L", false)
	currY := pdf.GetY()
	pdf.SetXY(xStart+txColWidths[0]+txColWidths[1]+txColWidths[2], yStart)
	pdf.CellFormat(txColWidths[3], txDataRowHeight, debitsStr, "B", 0, "R", false, 0, "")
	pdf.CellFormat(txColWidths[4], txDataRowHeight, creditsStr, "B", 0, "R", false, 0, "")
	pdf.CellFormat(txColWidths[5], txDataRowHeight, balanceStr, "B", 1, "R", false, 0, "")
	if currY > yStart+txDataRowHeight {
		pdf.SetY(currY)
	} else {
		pdf.SetY(yStart + txDataRowHeight)
	}
	pdf.SetLineWidth(0.25)
	pdf.SetDrawColor(0, 0, 0)

	return runningBalance
}

func drawTotalsRow(pdf *gofpdf.Fpdf, result *services.StatementResult, assetCode string) {
	pdf.SetFont("Inter", "B", bodyFontSize)
	pdf.SetFillColor(totalsRowBgColor[0], totalsRowBgColor[1], totalsRowBgColor[2])
	pdf.SetDrawColor(headerBorderColor[0], headerBorderColor[1], headerBorderColor[2])
	pdf.SetLineWidth(0.53) // ~2px top border
	pdf.CellFormat(txColWidths[0], txDataRowHeight, "", "T", 0, "L", true, 0, "")
	pdf.CellFormat(txColWidths[1], txDataRowHeight, "", "T", 0, "L", true, 0, "")
	pdf.CellFormat(txColWidths[2], txDataRowHeight, "Totals:", "T", 0, "R", true, 0, "")
	pdf.CellFormat(txColWidths[3], txDataRowHeight, utils.FormatAmountTo2Decimals(result.Totals.TotalDebits)+" "+assetCode, "T", 0, "R", true, 0, "")
	pdf.CellFormat(txColWidths[4], txDataRowHeight, utils.FormatAmountTo2Decimals(result.Totals.TotalCredits)+" "+assetCode, "T", 0, "R", true, 0, "")
	pdf.SetTextColor(blueColor[0], blueColor[1], blueColor[2])
	pdf.CellFormat(txColWidths[5], txDataRowHeight, utils.FormatAmountTo2Decimals(result.Totals.Balance)+" "+assetCode, "T", 1, "R", true, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.SetLineWidth(0.25)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetFont("Inter", "", bodyFontSize)
}

// breakHeaderWords inserts newlines between words so each word appears on its own line.
func breakHeaderWords(s string) string {
	return strings.ReplaceAll(s, " ", "\n")
}
