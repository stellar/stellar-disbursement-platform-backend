package statement

import (
	"fmt"
	"strings"
	"time"

	"github.com/jung-kurt/gofpdf/v2"
	"github.com/shopspring/decimal"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func drawTxTableHeader(pdf *gofpdf.Fpdf) {
	pdf.SetFont("Inter", "emi", tableHeaderSize)
	pdf.SetTextColor(headerAndTotalsColor[0], headerAndTotalsColor[1], headerAndTotalsColor[2])
	pdf.SetDrawColor(headerBorderColor[0], headerBorderColor[1], headerBorderColor[2])
	pdf.SetLineWidth(0.4)
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
		textW := w - 2*cellPaddingX
		if h.align == "L" {
			pdf.SetXY(xPos+cellPaddingX, yStart)
			pdf.CellFormat(textW, txHeaderRowHeight, h.text, "", 0, h.align, false, 0, "")
		} else {
			lines := strings.Split(breakHeaderWords(h.text), "\n")
			blockHeight := txHeaderLineHeight * float64(len(lines))
			lineY := yStart + (txHeaderRowHeight-blockHeight)/2
			for _, line := range lines {
				pdf.SetXY(xPos+cellPaddingX, lineY)
				pdf.CellFormat(textW, txHeaderLineHeight, line, "", 0, "R", false, 0, "")
				lineY += txHeaderLineHeight
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

func drawTxRow(pdf *gofpdf.Fpdf, tx *services.StatementTransaction, assetCode string, runningBalance decimal.Decimal) decimal.Decimal {
	var dateLine1, dateLine2 string
	dateStr := tx.UpdatedAt
	if dateStr == "" {
		dateStr = tx.CreatedAt
	}
	parsedTime, err := time.Parse(time.RFC3339, dateStr)
	if err == nil {
		utcTime := parsedTime.UTC()
		dateLine1 = utcTime.Format("2006-01-02")
		dateLine2 = utcTime.Format("15:04 UTC")
	} else {
		if len(dateStr) >= 10 {
			dateLine1 = dateStr[:10]
		} else {
			dateLine1 = dateStr
		}
		dateLine2 = ""
	}
	opID := tx.ID

	walletAddr := utils.TruncateString(tx.CounterpartyAddress, 5)
	name := tx.CounterpartyName
	hasName := name != ""
	var line2Label, line2Value string
	if hasName {
		if tx.Type == "credit" {
			line2Value = name
		} else {
			line2Label = "Recipient"
			line2Value = name
		}
		if line2Label != "" && len(line2Label+" • "+line2Value) > maxCounterpartyTextLength {
			combined := line2Label + " • " + line2Value
			combined = utils.TruncateToMaxLength(combined, maxCounterpartyTextLength)
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
	amount, err := decimal.NewFromString(tx.Amount)
	if err != nil {
		// If amount parsing fails, use zero as fallback
		amount = decimal.Zero
	}
	amountStr := utils.FormatAmountTo2Decimals(tx.Amount)
	if tx.Type == "debit" {
		debitsAmount = amountStr
		runningBalance = runningBalance.Sub(amount)
	} else {
		creditsAmount = amountStr
		runningBalance = runningBalance.Add(amount)
	}
	balanceAmountStr := utils.FormatDecimal(runningBalance)

	pdf.SetDrawColor(defaultBorderColor[0], defaultBorderColor[1], defaultBorderColor[2])
	pdf.SetLineWidth(0.26)
	pdf.SetTextColor(defaultCellColor[0], defaultCellColor[1], defaultCellColor[2])
	xStart := pdf.GetX()
	yStart := pdf.GetY()

	pdf.SetFont("Inter", "", txSmallFontSize)
	pdf.SetXY(xStart, yStart)
	pdf.CellFormat(txColWidths[0], txDataRowHeight, "", "B", 0, "L", false, 0, "")
	dateCellWidth := txColWidths[0] - 2*cellPaddingX
	dateBlockHeight := dateLineHeight * 2
	dateY := yStart + (txDataRowHeight-dateBlockHeight)/2
	pdf.SetXY(xStart+cellPaddingX, dateY)
	pdf.CellFormat(dateCellWidth, dateLineHeight, dateLine1, "", 0, "L", false, 0, "")
	if dateLine2 != "" {
		pdf.SetXY(xStart+cellPaddingX, dateY+dateLineHeight)
		pdf.CellFormat(dateCellWidth, dateLineHeight, dateLine2, "", 0, "L", false, 0, "")
	}

	xID := xStart + txColWidths[0]
	pdf.SetXY(xID, yStart)
	pdf.CellFormat(txColWidths[1], txDataRowHeight, "", "B", 0, "L", false, 0, "")
	cellWidth := txColWidths[1] - 2*cellPaddingX
	txURL := fmt.Sprintf("%stx/%s", stellarExpertTestnetBaseURL, tx.ID)
	pdf.SetFont("GoogleSansCode", "U", txSmallFontSize)
	pdf.SetTextColor(defaultCellColor[0], defaultCellColor[1], defaultCellColor[2])
	idLines := pdf.SplitText(opID, cellWidth)
	idBlockHeight := float64(len(idLines)) * txIDLineHeight
	var extBlockHeight float64
	if tx.ExternalPaymentID != "" {
		extBlockHeight = txIDLineHeight
	}
	totalBlockHeight := idBlockHeight + extBlockHeight
	blockY := yStart + (txDataRowHeight-totalBlockHeight)/2
	pdf.SetXY(xID+cellPaddingX, blockY)
	pdf.MultiCell(cellWidth, txIDLineHeight, opID, "", "L", false)
	pdf.LinkString(xID+cellPaddingX, blockY, cellWidth, idBlockHeight, txURL)
	if tx.ExternalPaymentID != "" {
		pdf.SetFont("Inter", "", txSmallFontSize)
		pdf.SetXY(xID+cellPaddingX, blockY+idBlockHeight)
		pdf.CellFormat(cellWidth, txIDLineHeight, tx.ExternalPaymentID, "", 0, "L", false, 0, "")
	}
	pdf.SetY(yStart)
	pdf.SetTextColor(defaultCellColor[0], defaultCellColor[1], defaultCellColor[2])

	xCP := xStart + txColWidths[0] + txColWidths[1]
	cpW := txColWidths[2] - 2*cellPaddingX
	pdf.SetXY(xCP, yStart)
	pdf.CellFormat(txColWidths[2], txDataRowHeight, "", "B", 0, "L", false, 0, "")
	xTextStart := xCP + cellPaddingX

	counterpartyBlockHeight := counterpartyLineHeight * 2
	counterpartyY := yStart + (txDataRowHeight-counterpartyBlockHeight)/2

	pdf.SetFont("Inter", "", txSmallFontSize)

	if tx.Type == "credit" {
		pdf.SetTextColor(noteColor[0], noteColor[1], noteColor[2])
		line1Text := "Sender Wallet Address •"
		pdf.SetXY(xTextStart, counterpartyY)
		pdf.CellFormat(cpW, counterpartyLineHeight, line1Text, "", 0, "L", false, 0, "")

		pdf.SetFont("GoogleSansCode", "U", txSmallFontSize)
		pdf.SetTextColor(defaultCellColor[0], defaultCellColor[1], defaultCellColor[2])
		pdf.SetXY(xTextStart, counterpartyY+counterpartyLineHeight)
		walletURL := fmt.Sprintf("%saccount/%s", stellarExpertTestnetBaseURL, tx.CounterpartyAddress)
		pdf.CellFormat(cpW, counterpartyLineHeight, walletAddr, "", 0, "L", false, 0, walletURL)
		pdf.LinkString(xTextStart, counterpartyY+counterpartyLineHeight, cpW, counterpartyLineHeight, walletURL)
	} else {
		if hasName {
			line1Label := "Wallet Address •"
			pdf.SetFont("Inter", "", txSmallFontSize)
			pdf.SetTextColor(noteColor[0], noteColor[1], noteColor[2])
			labelWidth := pdf.GetStringWidth(line1Label)
			xLabelStart := xTextStart
			pdf.SetXY(xLabelStart, counterpartyY)
			pdf.CellFormat(labelWidth, counterpartyLineHeight, line1Label, "", 0, "L", false, 0, "")

			xWalletStart := xLabelStart + labelWidth + counterpartyGap
			pdf.SetFont("GoogleSansCode", "U", txSmallFontSize)
			pdf.SetTextColor(defaultCellColor[0], defaultCellColor[1], defaultCellColor[2])
			walletURL := fmt.Sprintf("%saccount/%s", stellarExpertTestnetBaseURL, tx.CounterpartyAddress)
			walletWidth := cpW - (xWalletStart - xTextStart)
			pdf.SetXY(xWalletStart, counterpartyY)
			pdf.CellFormat(walletWidth, counterpartyLineHeight, walletAddr, "", 0, "L", false, 0, walletURL)
			pdf.LinkString(xWalletStart, counterpartyY, walletWidth, counterpartyLineHeight, walletURL)

			pdf.SetFont("Inter", "", txSmallFontSize)
			pdf.SetTextColor(noteColor[0], noteColor[1], noteColor[2])
			var line2Text string
			if line2Label != "" {
				line2Text = line2Label + " • " + line2Value
			} else {
				line2Text = line2Value
			}
			pdf.SetXY(xTextStart, counterpartyY+counterpartyLineHeight)
			pdf.CellFormat(cpW, counterpartyLineHeight, line2Text, "", 0, "L", false, 0, "")
		} else {
			singleLineY := yStart + (txDataRowHeight-counterpartyLineHeight)/2
			line1Label := "Wallet Address • "
			pdf.SetFont("Inter", "", txSmallFontSize)
			pdf.SetTextColor(noteColor[0], noteColor[1], noteColor[2])
			labelWidth := pdf.GetStringWidth(line1Label)
			xLabelStart := xTextStart
			pdf.SetXY(xLabelStart, singleLineY)
			pdf.CellFormat(labelWidth, counterpartyLineHeight, line1Label, "", 0, "L", false, 0, "")

			xWalletStart := xLabelStart + labelWidth
			pdf.SetFont("GoogleSansCode", "U", txSmallFontSize)
			pdf.SetTextColor(defaultCellColor[0], defaultCellColor[1], defaultCellColor[2])
			walletURL := fmt.Sprintf("%saccount/%s", stellarExpertTestnetBaseURL, tx.CounterpartyAddress)
			walletWidth := cpW - (xWalletStart - xTextStart)
			pdf.SetXY(xWalletStart, singleLineY)
			pdf.CellFormat(walletWidth, counterpartyLineHeight, walletAddr, "", 0, "L", false, 0, walletURL)
			pdf.LinkString(xWalletStart, singleLineY, walletWidth, counterpartyLineHeight, walletURL)
		}
	}
	xDebits := xStart + txColWidths[0] + txColWidths[1] + txColWidths[2]
	if debitsAmount != "" {
		drawAmountWithCurrency(pdf, amountCellArgs{xDebits, yStart, txColWidths[3], txDataRowHeight, debitsAmount, assetCode, "B", false, amountCellOpts{amountColor: highlightColor}})
	} else {
		pdf.SetXY(xDebits, yStart)
		pdf.CellFormat(txColWidths[3], txDataRowHeight, "", "B", 0, "R", false, 0, "")
	}
	xCredits := xDebits + txColWidths[3]
	if creditsAmount != "" {
		drawAmountWithCurrency(pdf, amountCellArgs{xCredits, yStart, txColWidths[4], txDataRowHeight, creditsAmount, assetCode, "B", false, amountCellOpts{amountColor: highlightColor}})
	} else {
		pdf.SetXY(xCredits, yStart)
		pdf.CellFormat(txColWidths[4], txDataRowHeight, "", "B", 0, "R", false, 0, "")
	}
	xBalance := xCredits + txColWidths[4]
	drawAmountWithCurrency(pdf, amountCellArgs{xBalance, yStart, txColWidths[5], txDataRowHeight, balanceAmountStr, assetCode, "B", false, amountCellOpts{}})

	pdf.SetXY(xStart, yStart+txDataRowHeight)
	pdf.SetLineWidth(0.25)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Inter", "", bodyFontSize)

	return runningBalance
}

func drawTotalsRowForAsset(pdf *gofpdf.Fpdf, asset *services.StatementAssetSummary) {
	pdf.SetFillColor(totalsRowBgColor[0], totalsRowBgColor[1], totalsRowBgColor[2])
	pdf.SetDrawColor(headerBorderColor[0], headerBorderColor[1], headerBorderColor[2])
	pdf.SetLineWidth(0.53)
	xStart := pdf.GetX()
	yStart := pdf.GetY()

	pdf.SetFont("Inter", "emi", txCellFontSize)
	pdf.SetTextColor(headerAndTotalsColor[0], headerAndTotalsColor[1], headerAndTotalsColor[2])
	xRowStart := xStart
	for i := 0; i < 3; i++ {
		w := txColWidths[i]
		pdf.SetXY(xStart, yStart)
		pdf.CellFormat(w, txDataRowHeight, "", "T", 0, "L", true, 0, "")
		if i == 2 {
			textW := w - 2*cellPaddingX
			pdf.SetXY(xStart+cellPaddingX, yStart)
			pdf.CellFormat(textW, txDataRowHeight, "Totals:", "", 0, "R", false, 0, "")
		}
		xStart += w
	}

	xDebits := xStart
	xCredits := xDebits + txColWidths[3]
	xBalance := xCredits + txColWidths[4]

	drawAmountWithCurrency(pdf, amountCellArgs{xDebits, yStart, txColWidths[3], txDataRowHeight, utils.FormatAmountTo2Decimals(asset.TotalDebits), asset.Code, "T", true, amountCellOpts{forTotals: true}})
	drawAmountWithCurrency(pdf, amountCellArgs{xCredits, yStart, txColWidths[4], txDataRowHeight, utils.FormatAmountTo2Decimals(asset.TotalCredits), asset.Code, "T", true, amountCellOpts{forTotals: true}})
	drawAmountWithCurrency(pdf, amountCellArgs{xBalance, yStart, txColWidths[5], txDataRowHeight, utils.FormatAmountTo2Decimals(asset.EndingBalance), asset.Code, "T", true, amountCellOpts{forTotals: true, amountColor: activeColor}})

	pdf.SetXY(xRowStart, yStart+txDataRowHeight)
	pdf.SetLineWidth(0.25)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Inter", "", bodyFontSize)
}
