package statement

import (
	"fmt"

	"github.com/jung-kurt/gofpdf/v2"
)

// setupFooter sets the PDF footer function (disclaimer and page count).
func setupFooter(pdf *gofpdf.Fpdf) {
	pdf.SetFooterFunc(func() {
		pdf.SetY(pageHeight - marginBottom)
		pdf.Ln(footerMarginTop)
		pdf.SetDrawColor(defaultBorderColor[0], defaultBorderColor[1], defaultBorderColor[2])
		pdf.SetLineWidth(headerSeparatorLineWidth)
		y := pdf.GetY()
		pdf.Line(marginLR, y, mmPerPage-marginLR, y)
		pdf.SetY(y + 1.0)
		pdf.Ln(footerContentGap)
		pdf.SetLineWidth(0.25)
		pdf.SetDrawColor(0, 0, 0)

		yDisclaimerLine := pdf.GetY()
		disclaimerLabel := "Disclaimer:"
		disclaimerText := " This report is generated from SDP records. Blockchain confirmations reflect public ledger data."
		pdf.SetFont("Inter", "B", bodyFontSize)
		pdf.SetTextColor(noteColor[0], noteColor[1], noteColor[2])
		labelWidth := pdf.GetStringWidth(disclaimerLabel)
		xLabelStart := pdf.GetX()
		pdf.SetXY(xLabelStart, yDisclaimerLine)
		pdf.CellFormat(labelWidth, footerLineHeight, disclaimerLabel, "", 0, "L", false, 0, "")
		xTextStart := xLabelStart + labelWidth
		pdf.SetFont("Inter", "", bodyFontSize)
		pdf.SetXY(xTextStart, yDisclaimerLine)
		pdf.CellFormat(0, footerLineHeight, disclaimerText, "", 1, "L", false, 0, "")
		pdf.Ln(footerDisclaimerToPageGap)
		pdf.CellFormat(0, footerLineHeight, fmt.Sprintf("Page %d of %d", pdf.PageNo(), pdf.PageCount()), "", 0, "R", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	})
}
