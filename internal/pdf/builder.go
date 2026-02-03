package pdf

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
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

// useMockTransactions enables mock transaction data for PDF testing. Set to false for production.
const useMockTransactions = true

// Page dimensions and margins
const (
	mmPerPage    = 210.0
	pageHeight   = 297.0
	marginLR     = 15.0
	marginTop    = 15.0
	marginBottom = 25.0
)

// Font sizes
const (
	bodyFontSize          = 8.7
	tableHeaderSize       = 8.7
	sectionTitleSize      = 12.0
	txCellFontSize        = 8.7
	txSmallFontSize       = 7.5
	organizationNameFontSize = 9.2
	titleFontSize         = 23.4
)

// Table row heights
const (
	summaryHeaderRowHeight = 16.5
	summaryHeaderLineHeight = 5.0
	summaryDataRowHeight    = 14.5
	txHeaderRowHeight       = 16.5
	txDataRowHeight         = 14.5
)

// Table cell spacing
const (
	cellPaddingH    = 2.115
	counterpartyGap = 1.5
)

// Section margins
const (
	summarySectionBottomMargin = 15.0
)

// Header section spacing
const (
	headerBottomMargin       = 5.0
	headerSeparatorLineWidth = 0.26
	headerLogoToOrgNameGap   = 3.0
	headerLeftColLineHeight  = 6.0
	headerRightColLineHeight = 5.0
	logoOffsetX              = 1.0
)

// Title section spacing
const (
	titleSectionBottomMargin = 5.0
	titleSectionLine1Height  = 6.0
	titleSectionLine2Height  = 9.0
	titleSectionLine3Height  = 6.0
)

// Footer section spacing
const (
	footerLineHeight          = 4.0
	footerMarginTop           = 1.5
	footerContentGap          = 3.5
	footerDisclaimerToPageGap = 2.0
)

// Other constants
const (
	walletIDLabel             = "Wallet ID"
	maxCounterpartyTextLength = 32
)

// tableWidth is full content width between left and right margins (both = marginLR).
const tableWidth = mmPerPage - 2*marginLR

// summaryColWidths: Wallet Address | Beginning Balance | Total Credits | Total Debits | Ending Balance (sum = tableWidth)
var summaryColWidths = []float64{52, 32, 32, 32, 32}

// txColWidths: Date | Transaction ID | Counterparty | Debits | Credits | Balance (sum = tableWidth)
var txColWidths = []float64{23, 29, 44, 28, 28, 28}

// Text colors (RGB)
var headerAndTotalsColor = []int{54, 65, 83}   // #364153
var defaultCellColor      = []int{74, 85, 101}  // #4A5565
var noteColor = []int{106, 114, 130} // #6A7282
var activeColor    = []int{20, 71, 230}  // #1447E6
var summaryValueColor    = []int{16, 24, 40}   // #101828
var sectionTitleColor    = []int{30, 41, 57}   // #1E2939

// Border and background colors (RGB)
var headerBorderColor = []int{209, 213, 220}   // #D1D5DC
var defaultBorderColor = []int{229, 231, 232}     // #E5E7EB
var totalsRowBgColor = []int{249, 250, 251}   // #F9FAFB


// BuildPDF generates a multi-page PDF from a StatementResult and returns the bytes.
func BuildPDF(result *services.StatementResult, fromDate, toDate time.Time, organizationName string, organizationLogo []byte) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")

	pdf.AddUTF8FontFromBytes("Inter", "", interRegularFont)
	pdf.AddUTF8FontFromBytes("Inter", "B", interBoldFont)
	pdf.AddUTF8FontFromBytes("Inter", "M", interMediumFont)
	pdf.AddUTF8FontFromBytes("Inter", "emi", interSemiBoldFont) // SemiBold: register as "emi" (no "S") to avoid gofpdf strikethrough

	pdf.SetMargins(marginLR, marginTop, marginLR)
	pdf.SetAutoPageBreak(true, marginBottom)
	pdf.SetFooterFunc(func() {
		// Position at top of footer area; add margin, then top border).
		pdf.SetY(pageHeight - marginBottom)
		pdf.Ln(footerMarginTop)
		pdf.SetDrawColor(headerBorderColor[0], headerBorderColor[1], headerBorderColor[2])
		pdf.SetLineWidth(headerSeparatorLineWidth)
		y := pdf.GetY()
		pdf.Line(marginLR, y, mmPerPage-marginLR, y)
		pdf.SetY(y + 1.0)
		pdf.Ln(footerContentGap)
		pdf.SetLineWidth(0.25)
		pdf.SetDrawColor(0, 0, 0)

		// Disclaimer and page count fit within marginBottom (same as marginTop).
		pdf.SetTextColor(noteColor[0], noteColor[1], noteColor[2])
		pdf.SetFont("Inter", "", bodyFontSize)
		pdf.CellFormat(0, footerLineHeight, "Disclaimer: This report is generated from SDP records. Blockchain confirmations reflect public ledger data.", "", 1, "L", false, 0, "")
		pdf.Ln(footerDisclaimerToPageGap)
		pdf.CellFormat(0, footerLineHeight, fmt.Sprintf("Page %d of %d", pdf.PageNo(), pdf.PageCount()), "", 0, "R", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	})
	pdf.AddPage()

	drawHeader(pdf, organizationName, organizationLogo, fromDate, toDate)
	drawTitleSection(pdf, result.Summary.Account)
	drawHeaderSeparatorLine(pdf)

	assetCode := result.Summary.Asset.Code

	// --- ACCOUNT SUMMARY SECTION ---
	pdf.SetFont("Inter", "B", sectionTitleSize)
	pdf.SetTextColor(sectionTitleColor[0], sectionTitleColor[1], sectionTitleColor[2])
	pdf.CellFormat(0, 8, "Account Summary", "", 1, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(1)

	// Summary table header
	pdf.SetFont("Inter", "emi", tableHeaderSize)
	pdf.SetTextColor(headerAndTotalsColor[0], headerAndTotalsColor[1], headerAndTotalsColor[2])
	pdf.SetDrawColor(headerBorderColor[0], headerBorderColor[1], headerBorderColor[2])
	pdf.SetLineWidth(0.53)
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
			lineHeight := summaryHeaderLineHeight
			blockHeight := lineHeight * float64(len(lines))
			lineY := ySummaryHeaderStart + (summaryHeaderRowHeight-blockHeight)/2
			for _, line := range lines {
				pdf.SetXY(xPos+cellPaddingH, lineY)
				pdf.CellFormat(textW, summaryHeaderLineHeight, line, "", 0, "R", false, 0, "")
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

	// Summary table data row: regular font weight for all cells except ending balance (SemiBold)
	pdf.SetFont("Inter", "", bodyFontSize)
	pdf.SetTextColor(defaultCellColor[0], defaultCellColor[1], defaultCellColor[2])
	walletAddr := result.Summary.Account
	if strings.HasPrefix(walletAddr, "stellar:") {
		walletAddr = walletAddr[8:]
	}
	walletAddr = utils.TruncateString(walletAddr, 5)

	pdf.SetDrawColor(defaultBorderColor[0], defaultBorderColor[1], defaultBorderColor[2])
	pdf.SetLineWidth(0.26)
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
		// Set font weight to SemiBold and color to activeColor for ending balance cell
		if i == 3 {
			pdf.SetFont("Inter", "emi", bodyFontSize)
			pdf.SetTextColor(activeColor[0], activeColor[1], activeColor[2])
		}
		pdf.SetXY(xPos+cellPaddingH, ySummaryData)
		pdf.CellFormat(textW, summaryDataRowHeight, text, "", 0, "R", false, 0, "")
		// Reset font weight to regular and color back to defaultCellColor after ending balance
		if i == 3 {
			pdf.SetFont("Inter", "", bodyFontSize)
			pdf.SetTextColor(defaultCellColor[0], defaultCellColor[1], defaultCellColor[2])
		}
		xPos += w
	}
	pdf.SetXY(xSummaryLeft, ySummaryData+summaryDataRowHeight)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetLineWidth(0.25)
	pdf.SetDrawColor(0, 0, 0)

	pdf.Ln(summarySectionBottomMargin)

	// --- TRANSACTIONS SECTION ---
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

	transactions := result.Transactions
	if useMockTransactions {
		transactions = generateMockTransactions(20)
	}
	for _, tx := range transactions {
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
	pdf.SetDrawColor(defaultBorderColor[0], defaultBorderColor[1], defaultBorderColor[2])
	pdf.SetLineWidth(0.53)
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

	// Counterparty: Recipient/sender wallet information
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
		if line2Label != "" && len(line2Label+" • "+line2Value) > maxCounterpartyTextLength {
			combined := line2Label + " • " + line2Value
			combined = utils.TruncateToMaxLength(combined, maxCounterpartyTextLength)
			// Try to preserve the structure, but truncate if needed
			parts := strings.Split(combined, " • ")
			if len(parts) >= 2 {
				line2Label = parts[0]
				line2Value = parts[1]
			} else {
				line2Value = combined
				line2Label = ""
			}
		} else if line2Value != "" && len(line2Value) > maxCounterpartyTextLength {
			line2Value = utils.TruncateToMaxLength(line2Value, maxCounterpartyTextLength)
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

	// Draw cells
	pdf.SetDrawColor(defaultBorderColor[0], defaultBorderColor[1], defaultBorderColor[2])
	pdf.SetLineWidth(0.26)
	pdf.SetTextColor(defaultCellColor[0], defaultCellColor[1], defaultCellColor[2])
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

	// Counterparty
	const counterpartyLineHeight = 4.0
	const bulletChar = "•" // Bullet character
	xCP := xStart + txColWidths[0] + txColWidths[1]
	cpW := txColWidths[2] - 2*cellPaddingH
	pdf.SetXY(xCP, yStart)
	pdf.CellFormat(txColWidths[2], txDataRowHeight, "", "B", 0, "L", false, 0, "")
	xTextStart := xCP + cellPaddingH
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
		counterpartyBlockHeight := 2 * counterpartyLineHeight
		counterpartyY := yStart + (txDataRowHeight-counterpartyBlockHeight)/2
		// Label
		pdf.SetFont("Inter", "", txSmallFontSize)
		pdf.SetTextColor(noteColor[0], noteColor[1], noteColor[2])
		pdf.SetXY(xTextStart, counterpartyY)
		pdf.CellFormat(fixedLabelWidth, 4, walletIDLabel, "", 0, "L", false, 0, "")
		// Bullet
		pdf.SetXY(fixedBulletX, counterpartyY)
		pdf.CellFormat(bulletWidth, 4, bulletChar, "", 0, "L", false, 0, "")
		// Wallet address
		pdf.SetFont("Inter", "M", txSmallFontSize)
		pdf.SetTextColor(summaryValueColor[0], summaryValueColor[1], summaryValueColor[2])
		pdf.SetXY(fixedValueX, counterpartyY)
		pdf.CellFormat(cpW-(fixedValueX-xTextStart), 4, walletAddr, "", 0, "L", false, 0, "")
		// Name
		pdf.SetFont("Inter", "", txSmallFontSize)
		pdf.SetTextColor(noteColor[0], noteColor[1], noteColor[2])

		if line2Label != "" {
			// Label
			pdf.SetXY(xTextStart, counterpartyY+counterpartyLineHeight)
			pdf.CellFormat(fixedLabelWidth, 4, line2Label, "", 0, "L", false, 0, "")
			// Bullet
			pdf.SetXY(fixedBulletX, counterpartyY+counterpartyLineHeight)
			pdf.CellFormat(bulletWidth, 4, bulletChar, "", 0, "L", false, 0, "")
			// Name
			pdf.SetXY(fixedValueX, counterpartyY+counterpartyLineHeight)
			pdf.CellFormat(cpW-(fixedValueX-xTextStart), 4, line2Value, "", 0, "L", false, 0, "")
		} else {
			// Just the name, but align it with the value position
			pdf.SetXY(fixedValueX, counterpartyY+counterpartyLineHeight)
			pdf.CellFormat(cpW-(fixedValueX-xTextStart), 4, line2Value, "", 0, "L", false, 0, "")
		}
	} else {
		// Label
		counterpartyY := yStart + (txDataRowHeight-counterpartyLineHeight)/2
		pdf.SetFont("Inter", "", txSmallFontSize)
		pdf.SetTextColor(noteColor[0], noteColor[1], noteColor[2])
		pdf.SetXY(xTextStart, counterpartyY)
		pdf.CellFormat(fixedLabelWidth, counterpartyLineHeight, walletIDLabel, "", 0, "L", false, 0, "")
		// Bullet
		pdf.SetXY(fixedBulletX, counterpartyY)
		pdf.CellFormat(bulletWidth, counterpartyLineHeight, bulletChar, "", 0, "L", false, 0, "")
		// Wallet address
		pdf.SetFont("Inter", "M", txSmallFontSize)
		pdf.SetTextColor(summaryValueColor[0], summaryValueColor[1], summaryValueColor[2])
		pdf.SetXY(fixedValueX, counterpartyY)
		pdf.CellFormat(cpW-(fixedValueX-xTextStart), counterpartyLineHeight, walletAddr, "", 0, "L", false, 0, "")
	}
	// Debits
	xDebits := xStart + txColWidths[0] + txColWidths[1] + txColWidths[2]
	if debitsAmount != "" {
		drawAmountWithCurrency(pdf, amountCellArgs{xDebits, yStart, txColWidths[3], txDataRowHeight, debitsAmount, assetCode, "B", false, amountCellOpts{amountColor: summaryValueColor}}, cellPaddingH)
	} else {
		pdf.SetXY(xDebits, yStart)
		pdf.CellFormat(txColWidths[3], txDataRowHeight, "", "B", 0, "R", false, 0, "")
	}
	// Credits
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
	pdf.SetLineWidth(0.53)
	xStart := pdf.GetX()
	yStart := pdf.GetY()

	pdf.SetFont("Inter", "emi", txCellFontSize)
	pdf.SetTextColor(headerAndTotalsColor[0], headerAndTotalsColor[1], headerAndTotalsColor[2])
	xRowStart := xStart
	// First three cells
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
	drawAmountWithCurrency(pdf, amountCellArgs{xBalance, yStart, txColWidths[5], txDataRowHeight, utils.FormatAmountTo2Decimals(result.Totals.Balance), assetCode, "T", true, amountCellOpts{forTotals: true, amountColor: activeColor}}, cellPaddingH)

	pdf.SetXY(xRowStart, yStart+txDataRowHeight)
	pdf.SetLineWidth(0.25)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Inter", "", bodyFontSize)
}

// amountCellOpts configures drawAmountWithCurrency
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

// drawAmountWithCurrency draws a single cell with fixed height
func drawAmountWithCurrency(pdf *gofpdf.Fpdf, a amountCellArgs, paddingH float64) {
	x, y, w, h := a.x, a.y, a.w, a.h
	opts := a.opts
	// Draw cell border
	pdf.SetXY(x, y)
	pdf.CellFormat(w, h, "", a.border, 0, "R", a.fill, 0, "")
	textW := w - 2*paddingH
	textX := x + paddingH
	// Amount + currency
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
	// Currency
	pdf.SetTextColor(noteColor[0], noteColor[1], noteColor[2])
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

// drawHeader draws the header: left column (logo + organization name), right column (generated date, statement period).
func drawHeader(pdf *gofpdf.Fpdf, organizationName string, organizationLogo []byte, fromDate, toDate time.Time) {
	xLeft := pdf.GetX()
	yStart := pdf.GetY()
	contentWidth := tableWidth
	halfWidth := contentWidth / 2
	rightColX := xLeft + halfWidth

	var yLeftBottom float64 = yStart

	// Left column
	if len(organizationLogo) > 0 {
		imgName, imgInfo := registerLogoImage(pdf, organizationLogo)
		if imgName != "" && imgInfo != nil {
			const logoMaxHeight = 10.0
			imgW, imgH := imgInfo.Width(), imgInfo.Height()
			if imgH > logoMaxHeight {
				imgW = imgW * (logoMaxHeight / imgH)
				imgH = logoMaxHeight
			}
			pdf.ImageOptions(imgName, xLeft+logoOffsetX, yStart, imgW, imgH, false, gofpdf.ImageOptions{}, 0, "")
			yLeftBottom = yStart + imgH + headerLogoToOrgNameGap
		}
	}
	if organizationName != "" {
		pdf.SetFont("Inter", "B", organizationNameFontSize)
		pdf.SetTextColor(summaryValueColor[0], summaryValueColor[1], summaryValueColor[2])
		pdf.SetXY(xLeft, yLeftBottom)
		pdf.CellFormat(halfWidth, headerLeftColLineHeight, strings.ToUpper(organizationName), "", 0, "L", false, 0, "")
		yLeftBottom += headerLeftColLineHeight
	}

	// Right column
	pdf.SetFont("Inter", "", bodyFontSize)
	pdf.SetTextColor(defaultCellColor[0], defaultCellColor[1], defaultCellColor[2])
	genStr := fmt.Sprintf("Generated on %s", time.Now().UTC().Format("2006-01-02 15:04 UTC"))
	pdf.SetXY(rightColX, yStart)
	pdf.CellFormat(halfWidth, headerRightColLineHeight, genStr, "", 0, "R", false, 0, "")
	pdf.SetXY(rightColX, yStart+headerRightColLineHeight)
	pdf.CellFormat(halfWidth, headerRightColLineHeight, "Statement Period:", "", 0, "R", false, 0, "")
	periodStr := fmt.Sprintf("%s to %s", fromDate.Format("2006-01-02"), toDate.Format("2006-01-02"))
	pdf.SetXY(rightColX, yStart+2*headerRightColLineHeight)
	pdf.CellFormat(halfWidth, headerRightColLineHeight, periodStr, "", 0, "R", false, 0, "")
	yRightBottom := yStart + 3*headerRightColLineHeight

	// Advance Y to the bottom of the taller column, then add margin
	if yRightBottom > yLeftBottom {
		yLeftBottom = yRightBottom
	}
	pdf.SetXY(xLeft, yLeftBottom)
	pdf.Ln(headerBottomMargin)
	pdf.SetTextColor(0, 0, 0)
}

// registerLogoImage registers logo bytes with the PDF
func registerLogoImage(pdf *gofpdf.Fpdf, logoBytes []byte) (string, *gofpdf.ImageInfoType) {
	_, format, err := image.Decode(bytes.NewReader(logoBytes))
	if err != nil {
		return "", nil
	}
	var imageType string
	switch format {
	case "jpeg", "jpg":
		imageType = "JPEG"
	case "png":
		imageType = "PNG"
	default:
		return "", nil
	}
	opts := gofpdf.ImageOptions{ImageType: imageType}
	info := pdf.RegisterImageOptionsReader("orglogo", opts, bytes.NewReader(logoBytes))
	if info == nil {
		return "", nil
	}
	return "orglogo", info
}

// drawHeaderSeparatorLine draws a horizontal line below the title section
func drawHeaderSeparatorLine(pdf *gofpdf.Fpdf) {
	pdf.SetDrawColor(headerBorderColor[0], headerBorderColor[1], headerBorderColor[2])
	pdf.SetLineWidth(headerSeparatorLineWidth)
	y := pdf.GetY()
	pdf.Line(marginLR, y, mmPerPage-marginLR, y)
	pdf.SetY(y + 1.0)
	pdf.Ln(titleSectionBottomMargin)
	pdf.SetLineWidth(0.25)
	pdf.SetDrawColor(0, 0, 0)
}

// drawTitleSection draws the title block
func drawTitleSection(pdf *gofpdf.Fpdf, walletAccount string) {
	walletAddr := walletAccount
	if strings.HasPrefix(walletAddr, "stellar:") {
		walletAddr = walletAddr[8:]
	}
	walletAddr = utils.TruncateString(walletAddr, 5)

	pdf.SetFont("Inter", "", bodyFontSize)
	pdf.SetTextColor(noteColor[0], noteColor[1], noteColor[2])
	pdf.CellFormat(0, titleSectionLine1Height, "REPORT", "", 1, "L", false, 0, "")

	pdf.SetFont("Inter", "B", titleFontSize)
	pdf.SetTextColor(summaryValueColor[0], summaryValueColor[1], summaryValueColor[2])
	pdf.CellFormat(0, titleSectionLine2Height, "Wallet Statement", "", 1, "L", false, 0, "")

	pdf.SetFont("Inter", "", organizationNameFontSize)
	pdf.SetTextColor(defaultCellColor[0], defaultCellColor[1], defaultCellColor[2])
	pdf.CellFormat(0, titleSectionLine3Height, "Wallet Address: "+walletAddr, "", 1, "L", false, 0, "")

	pdf.Ln(titleSectionBottomMargin)
	pdf.SetTextColor(0, 0, 0)
}

// breakHeaderWords inserts newlines between words
func breakHeaderWords(s string) string {
	return strings.ReplaceAll(s, " ", "\n")
}

// generateMockTransactions returns count copies of a template transaction with slight variations (for testing).
func generateMockTransactions(count int) []services.StatementTransaction {
	baseTime, _ := time.Parse(time.RFC3339, "2026-01-29T15:00:10Z")
	out := make([]services.StatementTransaction, 0, count)
	for i := 0; i < count; i++ {
		txType := "debit"
		if i%2 == 1 {
			txType = "credit"
		}
		createdAt := baseTime.Add(time.Duration(i) * time.Hour).UTC().Format(time.RFC3339)
		out = append(out, services.StatementTransaction{
			ID:                  "7cb4a68dc164ad69c6121086cf3aef0cec0d78634f60e1a1e23e4637b1f082e2",
			CreatedAt:           createdAt,
			Type:                txType,
			Amount:              "0.1000000",
			CounterpartyAddress: "GAHSWJ2ANIFE3ZEWM4EN7WKLC2F4OCLS2O4QQQJSADYHOXZDA3EZNJ2M",
			CounterpartyName:     "owner@bluecorp.local",
			WalletID:            "07815404-eb0d-4188-a362-38a90aae185c",
		})
	}
	return out
}
