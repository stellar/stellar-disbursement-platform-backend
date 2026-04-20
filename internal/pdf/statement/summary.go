package statement

import (
	"strings"

	"github.com/jung-kurt/gofpdf/v2"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/pdf/shared"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func drawSummaryTable(pdf *gofpdf.Fpdf, result *services.StatementResult) {
	pdf.SetFont("Inter", "B", sectionTitleSize)
	pdf.SetTextColor(sectionTitleColor[0], sectionTitleColor[1], sectionTitleColor[2])
	pdf.CellFormat(0, 8, "Account Summary", "", 1, "L", false, 0, "")
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
		{"Currency", "L"},
		{"Beginning Balance", "R"},
		{"Total Credits", "R"},
		{"Total Debits", "R"},
		{"Ending Balance", "R"},
	}
	for i, h := range summaryHeaders {
		w := summaryColWidths[i]
		pdf.SetXY(xPos, ySummaryHeaderStart)
		pdf.CellFormat(w, summaryHeaderRowHeight, "", "B", 0, "L", false, 0, "")
		textW := w - 2*cellPaddingX
		if i == 0 {
			pdf.SetXY(xPos+cellPaddingX, ySummaryHeaderStart)
			pdf.CellFormat(textW, summaryHeaderRowHeight, h.text, "", 0, h.align, false, 0, "")
		} else {
			lines := strings.Split(shared.BreakHeaderWords(h.text), "\n")
			lineHeight := summaryHeaderLineHeight
			blockHeight := lineHeight * float64(len(lines))
			lineY := ySummaryHeaderStart + (summaryHeaderRowHeight-blockHeight)/2
			for _, line := range lines {
				pdf.SetXY(xPos+cellPaddingX, lineY)
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

	pdf.SetDrawColor(defaultBorderColor[0], defaultBorderColor[1], defaultBorderColor[2])
	pdf.SetLineWidth(0.26)
	for _, asset := range result.Summary.Assets {
		xPos = xSummaryLeft
		for _, w := range summaryColWidths {
			pdf.SetXY(xPos, pdf.GetY())
			pdf.CellFormat(w, summaryDataRowHeight, "", "B", 0, "L", false, 0, "")
			xPos += w
		}
		ySummaryData := pdf.GetY()
		textW0 := summaryColWidths[0] - 2*cellPaddingX
		pdf.SetXY(xSummaryLeft+cellPaddingX, ySummaryData)
		pdf.SetFont("Inter", "B", bodyFontSize)
		pdf.SetTextColor(highlightColor[0], highlightColor[1], highlightColor[2])
		pdf.CellFormat(textW0, summaryDataRowHeight, asset.Code, "", 0, "L", false, 0, "")
		xPos = xSummaryLeft + summaryColWidths[0]
		for i, w := range summaryColWidths[1:] {
			textW := w - 2*cellPaddingX
			var text string
			switch i {
			case 0:
				text = utils.FormatAmountTo2Decimals(asset.BeginningBalance)
			case 1:
				text = utils.FormatAmountTo2Decimals(asset.TotalCredits)
			case 2:
				text = utils.FormatAmountTo2Decimals(asset.TotalDebits)
			case 3:
				text = utils.FormatAmountTo2Decimals(asset.EndingBalance)
			}
			if i < 3 {
				pdf.SetFont("Inter", "", bodyFontSize)
				pdf.SetTextColor(highlightColor[0], highlightColor[1], highlightColor[2])
			} else {
				pdf.SetFont("Inter", "emi", bodyFontSize)
				pdf.SetTextColor(activeColor[0], activeColor[1], activeColor[2])
			}
			pdf.SetXY(xPos+cellPaddingX, ySummaryData)
			pdf.CellFormat(textW, summaryDataRowHeight, text, "", 0, "R", false, 0, "")
			xPos += w
		}
		pdf.SetFont("Inter", "", bodyFontSize)
		pdf.SetTextColor(defaultCellColor[0], defaultCellColor[1], defaultCellColor[2])
		pdf.SetXY(xSummaryLeft, ySummaryData+summaryDataRowHeight)
	}
	pdf.SetTextColor(0, 0, 0)
	pdf.SetLineWidth(0.25)
	pdf.SetDrawColor(0, 0, 0)
	pdf.Ln(sectionBottomMargin)
}
