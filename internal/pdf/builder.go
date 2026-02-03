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

//go:embed assets/fonts/Inter_24pt-Medium.ttf
var interMediumFont []byte

//go:embed assets/fonts/Inter_24pt-SemiBold.ttf
var interSemiBoldFont []byte

const (
	mmPerPage         = 210.0
	pageHeight        = 297.0
	marginLR          = 15.0
	marginTop         = 15.0
	marginBottom      = 20.0
	headerFontSize    = 14.0
	bodyFontSize      = 9.0
	tableHeaderSize   = 9.0
	sectionTitleSize  = 12.0
	txCellFontSize    = 9.0
	txSmallFontSize   = 7.5
	walletIDLabel     = "Wallet ID"
)

// tableWidth is full content width between left and right margins (both = marginLR).
const tableWidth = mmPerPage - 2*marginLR // 180mm for A4

// summaryColWidths: Wallet Address | Beginning Balance | Total Credits | Total Debits | Ending Balance (sum = tableWidth)
var summaryColWidths = []float64{52, 32, 32, 32, 32}

// txColWidths: Date | Transaction ID | Counterparty | Debits | Credits | Balance (sum = tableWidth)
var txColWidths = []float64{23, 29, 44, 28, 28, 28}

// Text colors (RGB)
var headerAndTotalsColor = []int{54, 65, 83}   // #364153 — header cells and totals row text
var normalCellColor      = []int{74, 85, 101}  // #4A5565 — normal data cells
var currencyColor        = []int{106, 114, 130} // #6A7282 — all currencies
var totalBalanceColor    = []int{20, 71, 230}  // #1447E6 — ending balance and total balance
var summaryValueColor    = []int{16, 24, 40}   // #101828 — wallet address, summary row values, debits/credits amounts
var sectionTitleColor    = []int{30, 41, 57}   // #1E2939 — section titles

// Border and background colors (RGB)
var headerBorderColor = []int{209, 213, 220}   // #D1D5DC
var rowBorderColor = []int{229, 231, 235}     // #E5E7EB
var totalsRowBgColor = []int{249, 250, 251}   // #F9FAFB

const summaryHeaderRowHeight = 17.0
const summaryDataRowHeight   = 15.0
const txHeaderRowHeight      = 17.0
const txDataRowHeight        = 15.0

const summarySectionBottomMargin = 17.0
const cellPaddingH = 2.115
const counterpartyGap = 1.5

// BuildPDF generates a multi-page PDF from a StatementResult and returns the bytes.
func BuildPDF(result *services.StatementResult, fromDate, toDate time.Time) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")

	pdf.AddUTF8FontFromBytes("Inter", "", interRegularFont)
	pdf.AddUTF8FontFromBytes("Inter", "B", interBoldFont)
	pdf.AddUTF8FontFromBytes("Inter", "M", interMediumFont)
	pdf.AddUTF8FontFromBytes("Inter", "emi", interSemiBoldFont) // SemiBold: register as "emi" (no "S") to avoid gofpdf strikethrough

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
	pdf.SetTextColor(sectionTitleColor[0], sectionTitleColor[1], sectionTitleColor[2])
	pdf.CellFormat(0, 8, "Account Summary", "", 1, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(1)

	// Summary table header: fixed height 65px, Wallet Address left (no break), others right, SemiBold, #364153
	// Header letter spacing -0.15px is not supported by gofpdf.
	pdf.SetFont("Inter", "emi", tableHeaderSize)
	pdf.SetTextColor(headerAndTotalsColor[0], headerAndTotalsColor[1], headerAndTotalsColor[2])
	pdf.SetDrawColor(headerBorderColor[0], headerBorderColor[1], headerBorderColor[2])
	pdf.SetLineWidth(0.53) // ~2px
	ySummaryHeaderStart := pdf.GetY()
	xPos := pdf.GetX()
	xSummaryLeft := xPos
	summaryHeaders := []struct {
		text  string
		align string
	}{
		{"Wallet Address", "L"},
		{"Beginning Balance", "R"},
		{"Total Credits", "R"},
		{"Total Debits", "R"},
		{"Ending Balance", "R"},
	}
	for i, h := range summaryHeaders {
		w := summaryColWidths[i]
		pdf.SetXY(xPos, ySummaryHeaderStart)
		pdf.CellFormat(w, summaryHeaderRowHeight, "", "B", 0, "L", false, 0, "")
		textW := w - 2*cellPaddingH
		if i == 0 {
			pdf.SetXY(xPos+cellPaddingH, ySummaryHeaderStart)
			pdf.CellFormat(textW, summaryHeaderRowHeight, h.text, "", 0, h.align, false, 0, "")
		} else {
			lines := strings.Split(breakHeaderWords(h.text), "\n")
			lineHeight := 4.0
			blockHeight := lineHeight * float64(len(lines))
			lineY := ySummaryHeaderStart + (summaryHeaderRowHeight-blockHeight)/2
			for _, line := range lines {
				pdf.SetXY(xPos+cellPaddingH, lineY)
				pdf.CellFormat(textW, 4, line, "", 0, "R", false, 0, "")
				lineY += lineHeight
			}
		}
		xPos += w
	}
	pdf.SetXY(xSummaryLeft, ySummaryHeaderStart+summaryHeaderRowHeight)
	pdf.SetLineWidth(0.25)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Inter", "", bodyFontSize)

	// Summary table data row: all values SemiBold #101828
	pdf.SetFont("Inter", "emi", bodyFontSize)
	pdf.SetTextColor(summaryValueColor[0], summaryValueColor[1], summaryValueColor[2])
	walletAddr := result.Summary.Account
	if strings.HasPrefix(walletAddr, "stellar:") {
		walletAddr = walletAddr[8:]
	}
	walletAddr = utils.TruncateString(walletAddr, 5)

	pdf.SetDrawColor(rowBorderColor[0], rowBorderColor[1], rowBorderColor[2])
	pdf.SetLineWidth(0.26) // ~1px
	xPos = xSummaryLeft
	for _, w := range summaryColWidths {
		pdf.SetXY(xPos, pdf.GetY())
		pdf.CellFormat(w, summaryDataRowHeight, "", "B", 0, "L", false, 0, "")
		xPos += w
	}
	pdf.SetXY(xSummaryLeft, pdf.GetY())
	ySummaryData := pdf.GetY()
	textW0 := summaryColWidths[0] - 2*cellPaddingH
	pdf.SetXY(xSummaryLeft+cellPaddingH, ySummaryData)
	pdf.CellFormat(textW0, summaryDataRowHeight, walletAddr, "", 0, "L", false, 0, "")
	xPos = xSummaryLeft + summaryColWidths[0]
	for i, w := range summaryColWidths[1:] {
		textW := w - 2*cellPaddingH
		var text string
		switch i {
		case 0:
			text = utils.FormatAmountTo2Decimals(result.Summary.BeginningBalance) + " " + assetCode
		case 1:
			text = utils.FormatAmountTo2Decimals(result.Summary.TotalCredits) + " " + assetCode
		case 2:
			text = utils.FormatAmountTo2Decimals(result.Summary.TotalDebits) + " " + assetCode
		case 3:
			text = utils.FormatAmountTo2Decimals(result.Summary.EndingBalance) + " " + assetCode
		}
		pdf.SetXY(xPos+cellPaddingH, ySummaryData)
		pdf.CellFormat(textW, summaryDataRowHeight, text, "", 0, "R", false, 0, "")
		xPos += w
	}
	pdf.SetXY(xSummaryLeft, ySummaryData+summaryDataRowHeight)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetLineWidth(0.25)
	pdf.SetDrawColor(0, 0, 0)

	pdf.Ln(summarySectionBottomMargin)

	// --- Transactions ---
	pdf.SetFont("Inter", "B", sectionTitleSize)
	pdf.SetTextColor(sectionTitleColor[0], sectionTitleColor[1], sectionTitleColor[2])
	pdf.CellFormat(0, 8, "Transactions", "", 1, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
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
	pdf.SetFont("Inter", "emi", tableHeaderSize)
	pdf.SetTextColor(headerAndTotalsColor[0], headerAndTotalsColor[1], headerAndTotalsColor[2])
	pdf.SetDrawColor(headerBorderColor[0], headerBorderColor[1], headerBorderColor[2])
	pdf.SetLineWidth(0.53) // ~2px
	yStart := pdf.GetY()
	xPos := pdf.GetX()
	txHeaders := []struct {
		text  string
		align string
	}{
		{"Date", "L"},
		{"Transaction ID", "L"},
		{"Counterparty", "L"},
		{"Debits", "R"},
		{"Credits", "R"},
		{"Balance", "R"},
	}
	for i, h := range txHeaders {
		w := txColWidths[i]
		pdf.SetXY(xPos, yStart)
		pdf.CellFormat(w, txHeaderRowHeight, "", "B", 0, "L", false, 0, "")
		textW := w - 2*cellPaddingH
		if h.align == "L" {
			pdf.SetXY(xPos+cellPaddingH, yStart)
			pdf.CellFormat(textW, txHeaderRowHeight, h.text, "", 0, h.align, false, 0, "")
		} else {
			lines := strings.Split(breakHeaderWords(h.text), "\n")
			lineHeight := 4.0
			blockHeight := lineHeight * float64(len(lines))
			lineY := yStart + (txHeaderRowHeight-blockHeight)/2
			for _, line := range lines {
				pdf.SetXY(xPos+cellPaddingH, lineY)
				pdf.CellFormat(textW, 4, line, "", 0, "R", false, 0, "")
				lineY += lineHeight
			}
		}
		xPos += w
	}
	pdf.SetY(yStart + txHeaderRowHeight)
	pdf.SetLineWidth(0.25)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Inter", "", bodyFontSize)
}

// drawTxRow draws one transaction row and returns the new running balance after this transaction.
func drawTxRow(pdf *gofpdf.Fpdf, tx *services.StatementTransaction, assetCode string, runningBalance decimal.Decimal) decimal.Decimal {
	dateStr := tx.CreatedAt
	if len(dateStr) >= 10 {
		dateStr = dateStr[:10]
	}
	opID := utils.TruncateToMaxLength(tx.ID, 16)

	// Recipient/Sender: line1 = "Wallet ID" + bullet + address (separate colors/weights), line2 = "Recipient" • name (or just name for credits, only if name exists)
	walletAddr := utils.TruncateString(tx.CounterpartyAddress, 5)
	name := tx.CounterpartyName
	hasName := name != ""
	var line2Label, line2Value string
	if hasName {
		if tx.Type == "credit" {
			// For credit rows, don't show "Sender" label
			line2Value = name
		} else {
			// For debit rows, show "Recipient • name"
			line2Label = "Recipient"
			line2Value = name
		}
		if line2Label != "" && len(line2Label+" • "+line2Value) > 40 {
			combined := line2Label + " • " + line2Value
			combined = utils.TruncateToMaxLength(combined, 40)
			// Try to preserve the structure, but truncate if needed
			parts := strings.Split(combined, " • ")
			if len(parts) >= 2 {
				line2Label = parts[0]
				line2Value = parts[1]
			} else {
				line2Value = combined
				line2Label = ""
			}
		} else if line2Value != "" && len(line2Value) > 40 {
			line2Value = utils.TruncateToMaxLength(line2Value, 40)
		}
	}

	var debitsAmount, creditsAmount string
	amount, _ := decimal.NewFromString(tx.Amount)
	amountStr := utils.FormatAmountTo2Decimals(tx.Amount)
	if tx.Type == "debit" {
		debitsAmount = amountStr
		runningBalance = runningBalance.Sub(amount)
	} else {
		creditsAmount = amountStr
		runningBalance = runningBalance.Add(amount)
	}
	balanceAmountStr := utils.FormatDecimal(runningBalance)

	// Draw cells: fixed height, no MultiCell, normal cell color #4A5565
	pdf.SetDrawColor(rowBorderColor[0], rowBorderColor[1], rowBorderColor[2])
	pdf.SetLineWidth(0.26) // ~1px
	pdf.SetTextColor(normalCellColor[0], normalCellColor[1], normalCellColor[2])
	xStart := pdf.GetX()
	yStart := pdf.GetY()

	// Date: border then text with padding
	pdf.SetFont("Inter", "", txCellFontSize)
	pdf.SetXY(xStart, yStart)
	pdf.CellFormat(txColWidths[0], txDataRowHeight, "", "B", 0, "L", false, 0, "")
	pdf.SetXY(xStart+cellPaddingH, yStart)
	pdf.CellFormat(txColWidths[0]-2*cellPaddingH, txDataRowHeight, dateStr, "", 0, "L", false, 0, "")
	// Transaction ID
	pdf.SetFont("Inter", "", txSmallFontSize)
	xID := xStart + txColWidths[0]
	pdf.SetXY(xID, yStart)
	pdf.CellFormat(txColWidths[1], txDataRowHeight, "", "B", 0, "L", false, 0, "")
	pdf.SetXY(xID+cellPaddingH, yStart)
	pdf.CellFormat(txColWidths[1]-2*cellPaddingH, txDataRowHeight, opID, "", 0, "L", false, 0, "")
	// Counterparty: border then one or two lines with padding
	const counterpartyLineHeight = 4.0
	const bulletChar = "•" // Bullet character
	xCP := xStart + txColWidths[0] + txColWidths[1]
	cpW := txColWidths[2] - 2*cellPaddingH
	pdf.SetXY(xCP, yStart)
	pdf.CellFormat(txColWidths[2], txDataRowHeight, "", "B", 0, "L", false, 0, "")
	xTextStart := xCP + cellPaddingH

	// Calculate fixed widths for vertical alignment
	pdf.SetFont("Inter", "", txSmallFontSize)
	walletIDWidth := pdf.GetStringWidth(walletIDLabel)
	recipientWidth := pdf.GetStringWidth("Recipient")
	fixedLabelWidth := walletIDWidth
	if recipientWidth > walletIDWidth {
		fixedLabelWidth = recipientWidth
	}
	bulletWidth := pdf.GetStringWidth(bulletChar)
	fixedBulletX := xTextStart + fixedLabelWidth + counterpartyGap
	fixedValueX := fixedBulletX + bulletWidth + counterpartyGap

	if hasName {
		// Two lines: Wallet ID • address, then Recipient • name or just name
		counterpartyBlockHeight := 2 * counterpartyLineHeight
		counterpartyY := yStart + (txDataRowHeight-counterpartyBlockHeight)/2
		// First line: "Wallet ID" (color #6A7282) + gap + bullet (color #6A7282) + gap + address (color #101828, Medium weight)
		pdf.SetFont("Inter", "", txSmallFontSize)
		pdf.SetTextColor(currencyColor[0], currencyColor[1], currencyColor[2])
		pdf.SetXY(xTextStart, counterpartyY)
		pdf.CellFormat(fixedLabelWidth, 4, walletIDLabel, "", 0, "L", false, 0, "")
		// Bullet at fixed position
		pdf.SetXY(fixedBulletX, counterpartyY)
		pdf.CellFormat(bulletWidth, 4, bulletChar, "", 0, "L", false, 0, "")
		// Wallet address (color #101828, Medium weight) at fixed position
		pdf.SetFont("Inter", "M", txSmallFontSize)
		pdf.SetTextColor(summaryValueColor[0], summaryValueColor[1], summaryValueColor[2])
		pdf.SetXY(fixedValueX, counterpartyY)
		pdf.CellFormat(cpW-(fixedValueX-xTextStart), 4, walletAddr, "", 0, "L", false, 0, "")
		// Second line: Recipient • name or just name (all color #6A7282)
		pdf.SetFont("Inter", "", txSmallFontSize)
		pdf.SetTextColor(currencyColor[0], currencyColor[1], currencyColor[2])
		if line2Label != "" {
			// Draw "Recipient" at fixed label width + gap + bullet at fixed position + gap + name at fixed value position
			pdf.SetXY(xTextStart, counterpartyY+counterpartyLineHeight)
			pdf.CellFormat(fixedLabelWidth, 4, line2Label, "", 0, "L", false, 0, "")
			// Bullet at fixed position
			pdf.SetXY(fixedBulletX, counterpartyY+counterpartyLineHeight)
			pdf.CellFormat(bulletWidth, 4, bulletChar, "", 0, "L", false, 0, "")
			// Name at fixed value position
			pdf.SetXY(fixedValueX, counterpartyY+counterpartyLineHeight)
			pdf.CellFormat(cpW-(fixedValueX-xTextStart), 4, line2Value, "", 0, "L", false, 0, "")
		} else {
			// Just the name, but align it with the value position
			pdf.SetXY(fixedValueX, counterpartyY+counterpartyLineHeight)
			pdf.CellFormat(cpW-(fixedValueX-xTextStart), 4, line2Value, "", 0, "L", false, 0, "")
		}
	} else {
		// Single line: "Wallet ID" (color #6A7282) + gap + bullet (color #6A7282) + gap + address (color #101828, Medium weight, centered vertically)
		counterpartyY := yStart + (txDataRowHeight-counterpartyLineHeight)/2
		pdf.SetFont("Inter", "", txSmallFontSize)
		pdf.SetTextColor(currencyColor[0], currencyColor[1], currencyColor[2])
		pdf.SetXY(xTextStart, counterpartyY)
		pdf.CellFormat(fixedLabelWidth, counterpartyLineHeight, walletIDLabel, "", 0, "L", false, 0, "")
		// Bullet at fixed position
		pdf.SetXY(fixedBulletX, counterpartyY)
		pdf.CellFormat(bulletWidth, counterpartyLineHeight, bulletChar, "", 0, "L", false, 0, "")
		// Wallet address (color #101828, Medium weight) at fixed position
		pdf.SetFont("Inter", "M", txSmallFontSize)
		pdf.SetTextColor(summaryValueColor[0], summaryValueColor[1], summaryValueColor[2])
		pdf.SetXY(fixedValueX, counterpartyY)
		pdf.CellFormat(cpW-(fixedValueX-xTextStart), counterpartyLineHeight, walletAddr, "", 0, "L", false, 0, "")
	}
	// Debits (amount color #101828, horizontal padding applied in drawAmountWithCurrency)
	xDebits := xStart + txColWidths[0] + txColWidths[1] + txColWidths[2]
	if debitsAmount != "" {
		drawAmountWithCurrency(pdf, amountCellArgs{xDebits, yStart, txColWidths[3], txDataRowHeight, debitsAmount, assetCode, "B", false, amountCellOpts{amountColor: summaryValueColor}}, cellPaddingH)
	} else {
		pdf.SetXY(xDebits, yStart)
		pdf.CellFormat(txColWidths[3], txDataRowHeight, "", "B", 0, "R", false, 0, "")
	}
	// Credits (amount color #101828)
	xCredits := xDebits + txColWidths[3]
	if creditsAmount != "" {
		drawAmountWithCurrency(pdf, amountCellArgs{xCredits, yStart, txColWidths[4], txDataRowHeight, creditsAmount, assetCode, "B", false, amountCellOpts{amountColor: summaryValueColor}}, cellPaddingH)
	} else {
		pdf.SetXY(xCredits, yStart)
		pdf.CellFormat(txColWidths[4], txDataRowHeight, "", "B", 0, "R", false, 0, "")
	}
	// Balance
	xBalance := xCredits + txColWidths[4]
	drawAmountWithCurrency(pdf, amountCellArgs{xBalance, yStart, txColWidths[5], txDataRowHeight, balanceAmountStr, assetCode, "B", false, amountCellOpts{}}, cellPaddingH)

	pdf.SetXY(xStart, yStart+txDataRowHeight)
	pdf.SetLineWidth(0.25)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Inter", "", bodyFontSize)

	return runningBalance
}

func drawTotalsRow(pdf *gofpdf.Fpdf, result *services.StatementResult, assetCode string) {
	pdf.SetFillColor(totalsRowBgColor[0], totalsRowBgColor[1], totalsRowBgColor[2])
	pdf.SetDrawColor(headerBorderColor[0], headerBorderColor[1], headerBorderColor[2])
	pdf.SetLineWidth(0.53) // ~2px top border
	xStart := pdf.GetX()
	yStart := pdf.GetY()

	pdf.SetFont("Inter", "emi", txCellFontSize)
	pdf.SetTextColor(headerAndTotalsColor[0], headerAndTotalsColor[1], headerAndTotalsColor[2])
	xRowStart := xStart
	// First three cells: border then text with padding (first two empty, third "Totals:")
	for i := 0; i < 3; i++ {
		w := txColWidths[i]
		pdf.SetXY(xStart, yStart)
		pdf.CellFormat(w, txDataRowHeight, "", "T", 0, "L", true, 0, "")
		if i == 2 {
			textW := w - 2*cellPaddingH
			pdf.SetXY(xStart+cellPaddingH, yStart)
			pdf.CellFormat(textW, txDataRowHeight, "Totals:", "", 0, "R", false, 0, "")
		}
		xStart += w
	}

	xDebits := xStart
	xCredits := xDebits + txColWidths[3]
	xBalance := xCredits + txColWidths[4]

	drawAmountWithCurrency(pdf, amountCellArgs{xDebits, yStart, txColWidths[3], txDataRowHeight, utils.FormatAmountTo2Decimals(result.Totals.TotalDebits), assetCode, "T", true, amountCellOpts{forTotals: true}}, cellPaddingH)
	drawAmountWithCurrency(pdf, amountCellArgs{xCredits, yStart, txColWidths[4], txDataRowHeight, utils.FormatAmountTo2Decimals(result.Totals.TotalCredits), assetCode, "T", true, amountCellOpts{forTotals: true}}, cellPaddingH)
	drawAmountWithCurrency(pdf, amountCellArgs{xBalance, yStart, txColWidths[5], txDataRowHeight, utils.FormatAmountTo2Decimals(result.Totals.Balance), assetCode, "T", true, amountCellOpts{forTotals: true, amountColor: totalBalanceColor}}, cellPaddingH)

	pdf.SetXY(xRowStart, yStart+txDataRowHeight)
	pdf.SetLineWidth(0.25)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Inter", "", bodyFontSize)
}

// amountCellOpts configures drawAmountWithCurrency (forTotals = Bold/SemiBold, amountColor = e.g. totalBalanceColor).
type amountCellOpts struct {
	forTotals   bool
	amountColor []int
}

// amountCellArgs holds arguments for drawAmountWithCurrency.
type amountCellArgs struct {
	x, y, w, h       float64
	amountStr        string
	currencyCode     string
	border           string
	fill             bool
	opts             amountCellOpts
}

// drawAmountWithCurrency draws a single cell with fixed height: border, then amount and currency with horizontal padding.
// amount on first line (Inter Medium or SemiBold for totals), currency on second (Regular/SemiBold 10px #6A7282).
func drawAmountWithCurrency(pdf *gofpdf.Fpdf, a amountCellArgs, paddingH float64) {
	x, y, w, h := a.x, a.y, a.w, a.h
	opts := a.opts
	// Draw cell border (empty content)
	pdf.SetXY(x, y)
	pdf.CellFormat(w, h, "", a.border, 0, "R", a.fill, 0, "")
	textW := w - 2*paddingH
	textX := x + paddingH
	// Two lines (amount + currency): center vertically in cell. Line heights 5 + 4 = 9mm.
	const amountCurrencyBlockHeight = 9.0
	amountY := y + (h-amountCurrencyBlockHeight)/2
	if opts.forTotals {
		pdf.SetFont("Inter", "emi", txCellFontSize)
	} else {
		pdf.SetFont("Inter", "M", txCellFontSize)
	}
	if opts.amountColor != nil {
		pdf.SetTextColor(opts.amountColor[0], opts.amountColor[1], opts.amountColor[2])
	}
	pdf.SetXY(textX, amountY)
	pdf.CellFormat(textW, 5, a.amountStr, "", 0, "R", false, 0, "")
	if opts.amountColor != nil {
		pdf.SetTextColor(0, 0, 0)
	}
	// Second line: currency (Regular 10px #6A7282 or SemiBold for totals)
	pdf.SetTextColor(currencyColor[0], currencyColor[1], currencyColor[2])
	if opts.forTotals {
		pdf.SetFont("Inter", "emi", txSmallFontSize)
	} else {
		pdf.SetFont("Inter", "", txSmallFontSize)
	}
	pdf.SetXY(textX, amountY+5)
	pdf.CellFormat(textW, 4, a.currencyCode, "", 0, "R", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Inter", "", bodyFontSize)
}

// breakHeaderWords inserts newlines between words so each word appears on its own line.
func breakHeaderWords(s string) string {
	return strings.ReplaceAll(s, " ", "\n")
}
