package transaction

import (
	"github.com/jung-kurt/gofpdf/v2"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func drawSummaryTable(pdf *gofpdf.Fpdf, payment *data.Payment) {
	pdf.SetFont("Inter", "B", sectionTitleSize)
	pdf.SetTextColor(sectionTitleColor[0], sectionTitleColor[1], sectionTitleColor[2])
	pdf.CellFormat(0, 8, "Transaction Summary", "", 1, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(1)

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
		{"Amount", "L"},
		{"Currency", "R"},
		{"Status", "R"},
		{"Payment ID", "R"},
		{"Status Update At", "R"},
	}
	for i, h := range summaryHeaders {
		w := summaryColWidths[i]
		pdf.SetXY(xPos, ySummaryHeaderStart)
		pdf.CellFormat(w, summaryHeaderRowHeight, "", "B", 0, "L", false, 0, "")
		textW := w - 2*cellPaddingX
		pdf.SetXY(xPos+cellPaddingX, ySummaryHeaderStart)
		pdf.CellFormat(textW, summaryHeaderRowHeight, h.text, "", 0, h.align, false, 0, "")
		xPos += w
	}
	pdf.SetXY(xSummaryLeft, ySummaryHeaderStart+summaryHeaderRowHeight)
	pdf.SetLineWidth(0.25)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Inter", "B", bodyFontSize)

	pdf.SetDrawColor(defaultBorderColor[0], defaultBorderColor[1], defaultBorderColor[2])
	pdf.SetLineWidth(0.26)
	xPos = xSummaryLeft
	for _, w := range summaryColWidths {
		pdf.SetXY(xPos, pdf.GetY())
		pdf.CellFormat(w, summaryDataRowHeight, "", "B", 0, "L", false, 0, "")
		xPos += w
	}
	ySummaryData := pdf.GetY()

	amountStr := utils.FormatAmountTo2Decimals(payment.Amount)
	currencyCode := payment.Asset.Code
	statusStr := string(payment.Status)
	externalPaymentID := payment.ExternalPaymentID
	updatedAtStr := payment.UpdatedAt.UTC().Format("2006-01-02 15:04:05 UTC")

	pdf.SetXY(xSummaryLeft+cellPaddingX, ySummaryData)
	pdf.SetTextColor(highlightColor[0], highlightColor[1], highlightColor[2])
	pdf.CellFormat(summaryColWidths[0]-2*cellPaddingX, summaryDataRowHeight, amountStr, "", 0, "L", false, 0, "")

	pdf.SetXY(xSummaryLeft+summaryColWidths[0]+cellPaddingX, ySummaryData)
	pdf.CellFormat(summaryColWidths[1]-2*cellPaddingX, summaryDataRowHeight, currencyCode, "", 0, "R", false, 0, "")

	pdf.SetXY(xSummaryLeft+summaryColWidths[0]+summaryColWidths[1]+cellPaddingX, ySummaryData)
	if statusStr == "SUCCESS" {
		pdf.SetTextColor(successGreen[0], successGreen[1], successGreen[2])
	}
	pdf.CellFormat(summaryColWidths[2]-2*cellPaddingX, summaryDataRowHeight, statusStr, "", 0, "R", false, 0, "")
	pdf.SetTextColor(highlightColor[0], highlightColor[1], highlightColor[2])

	pdf.SetXY(xSummaryLeft+summaryColWidths[0]+summaryColWidths[1]+summaryColWidths[2]+cellPaddingX, ySummaryData)
	pdf.CellFormat(summaryColWidths[3]-2*cellPaddingX, summaryDataRowHeight, externalPaymentID, "", 0, "R", false, 0, "")

	pdf.SetXY(xSummaryLeft+summaryColWidths[0]+summaryColWidths[1]+summaryColWidths[2]+summaryColWidths[3]+cellPaddingX, ySummaryData)
	pdf.CellFormat(summaryColWidths[4]-2*cellPaddingX, summaryDataRowHeight, updatedAtStr, "", 0, "R", false, 0, "")

	pdf.SetTextColor(0, 0, 0)
	pdf.SetLineWidth(0.25)
	pdf.SetDrawColor(0, 0, 0)
	pdf.Ln(summaryDataRowHeight)
	pdf.Ln(sectionBottomMargin)
}
