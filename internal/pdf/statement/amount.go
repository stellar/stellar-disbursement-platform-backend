package statement

import (
	"github.com/jung-kurt/gofpdf/v2"
)

type amountCellOpts struct {
	forTotals   bool
	amountColor []int
}

type amountCellArgs struct {
	x, y, w, h   float64
	amountStr    string
	currencyCode string
	border       string
	fill         bool
	opts         amountCellOpts
}

func drawAmountWithCurrency(pdf *gofpdf.Fpdf, a amountCellArgs, paddingH float64) {
	x, y, w, h := a.x, a.y, a.w, a.h
	opts := a.opts
	pdf.SetXY(x, y)
	pdf.CellFormat(w, h, "", a.border, 0, "R", a.fill, 0, "")
	textW := w - 2*paddingH
	textX := x + paddingH
	amountCurrencyBlockHeight := amountLineHeight + currencyLineHeight
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
	pdf.CellFormat(textW, amountLineHeight, a.amountStr, "", 0, "R", false, 0, "")
	if opts.amountColor != nil {
		pdf.SetTextColor(0, 0, 0)
	}
	pdf.SetTextColor(noteColor[0], noteColor[1], noteColor[2])
	if opts.forTotals {
		pdf.SetFont("Inter", "emi", txSmallFontSize)
	} else {
		pdf.SetFont("Inter", "", txSmallFontSize)
	}
	pdf.SetXY(textX, amountY+amountLineHeight)
	pdf.CellFormat(textW, currencyLineHeight, a.currencyCode, "", 0, "R", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Inter", "", bodyFontSize)
}
