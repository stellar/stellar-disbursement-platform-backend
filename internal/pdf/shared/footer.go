package shared

import (
	"fmt"

	"github.com/jung-kurt/gofpdf/v2"
)

// FooterConfig holds layout values for the PDF footer.
type FooterConfig struct {
	PageHeight                float64
	MarginBottom              float64
	MarginLR                  float64
	MmPerPage                 float64
	FooterMarginTop           float64
	FooterContentGap          float64
	FooterDisclaimerToPageGap float64
	FooterLineHeight          float64
	BodyFontSize              float64
	NoteColor                 []int
	DefaultBorderColor        []int
	HeaderSeparatorLineWidth  float64
}

// SetupFooter sets the PDF footer function.
func SetupFooter(pdf *gofpdf.Fpdf, cfg FooterConfig) {
	pdf.SetFooterFunc(func() {
		pdf.SetY(cfg.PageHeight - cfg.MarginBottom)
		pdf.Ln(cfg.FooterMarginTop)
		pdf.SetDrawColor(cfg.DefaultBorderColor[0], cfg.DefaultBorderColor[1], cfg.DefaultBorderColor[2])
		pdf.SetLineWidth(cfg.HeaderSeparatorLineWidth)
		y := pdf.GetY()
		pdf.Line(cfg.MarginLR, y, cfg.MmPerPage-cfg.MarginLR, y)
		pdf.SetY(y + 1.0)
		pdf.Ln(cfg.FooterContentGap)
		pdf.SetLineWidth(0.25)
		pdf.SetDrawColor(0, 0, 0)

		yDisclaimerLine := pdf.GetY()
		disclaimerLabel := "Disclaimer: "
		disclaimerTextLine1 := "This report is generated from publicly available blockchain ledger data and records"
		disclaimerTextLine2 := "maintained by the Stellar Disbursement Platform."
		pdf.SetFont("Inter", "B", cfg.BodyFontSize)
		pdf.SetTextColor(cfg.NoteColor[0], cfg.NoteColor[1], cfg.NoteColor[2])
		labelWidth := pdf.GetStringWidth(disclaimerLabel)
		xLabelStart := pdf.GetX()
		pdf.SetXY(xLabelStart, yDisclaimerLine)
		pdf.CellFormat(labelWidth, cfg.FooterLineHeight, disclaimerLabel, "", 0, "L", false, 0, "")
		xTextStart := xLabelStart + labelWidth
		pdf.SetFont("Inter", "", cfg.BodyFontSize)
		pdf.SetXY(xTextStart, yDisclaimerLine)
		pdf.CellFormat(0, cfg.FooterLineHeight, disclaimerTextLine1, "", 1, "L", false, 0, "")
		pdf.SetXY(xTextStart, pdf.GetY())
		pdf.CellFormat(0, cfg.FooterLineHeight, disclaimerTextLine2, "", 1, "L", false, 0, "")
		pdf.Ln(cfg.FooterDisclaimerToPageGap * 0.5)
		pdf.CellFormat(0, cfg.FooterLineHeight, fmt.Sprintf("Page %d of %d", pdf.PageNo(), pdf.PageCount()), "", 0, "R", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	})
}
