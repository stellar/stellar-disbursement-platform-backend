package transaction

import (
	"strings"

	"github.com/jung-kurt/gofpdf/v2"
)

const (
	internalNotesTitle                = "Internal Reference Notes (User-entered)"
	internalNotesDisclaimer           = "This note is manually entered by the user for internal reference only. It is not part of the official transaction record."
	internalNotesPadding              = 4.0
	internalNotesLineHeight           = 4.5
	internalNotesDisclaimerToValueGap = 2.0
)

// drawInternalNotes draws the grey area above the footer when internalNotes is non-empty.
func drawInternalNotes(pdf *gofpdf.Fpdf, internalNotes string) {
	if internalNotes == "" {
		return
	}
	notes := strings.TrimSpace(internalNotes)
	if notes == "" {
		return
	}
	// Single line: take first line, truncate to max length
	if idx := strings.IndexAny(notes, "\n\r"); idx >= 0 {
		notes = notes[:idx]
	}
	if len(notes) > internalNotesMaxLength {
		notes = notes[:internalNotesMaxLength]
	}
	notes = strings.TrimSpace(notes)
	if notes == "" {
		return
	}

	// Content-height box placed at bottom: bottom edge = page bottom minus footer margin minus 3mm
	boxHeight := internalNotesPadding*2 + internalNotesLineHeight*3 + 2.0 + internalNotesDisclaimerToValueGap // title + disclaimer + gap + user text + gaps
	boxBottom := pageHeight - marginBottom - internalNotesMarginBottom
	yStart := boxBottom - boxHeight

	pdf.SetFillColor(internalNotesBgColor[0], internalNotesBgColor[1], internalNotesBgColor[2])
	pdf.SetDrawColor(internalNotesBorderColor[0], internalNotesBorderColor[1], internalNotesBorderColor[2])
	pdf.SetLineWidth(0.2)

	contentWidth := tableWidth
	pdf.RoundedRect(marginLR, yStart, contentWidth, boxHeight, internalNotesCornerRadius, "1234", "FD")

	textLeft := marginLR + internalNotesPadding
	textWidth := contentWidth - 2*internalNotesPadding

	// Title
	pdf.SetY(yStart + internalNotesPadding)
	pdf.SetX(textLeft)
	pdf.SetFont("Inter", "emi", internalNotesSmallFontSize)
	pdf.SetTextColor(internalNotesTitleColor[0], internalNotesTitleColor[1], internalNotesTitleColor[2])
	pdf.CellFormat(textWidth, internalNotesLineHeight, internalNotesTitle, "", 1, "L", false, 0, "")
	// Disclaimer
	pdf.SetX(textLeft)
	pdf.SetFont("Inter", "I", internalNotesSmallFontSize)
	pdf.SetTextColor(noteColor[0], noteColor[1], noteColor[2])
	pdf.CellFormat(textWidth, internalNotesLineHeight, internalNotesDisclaimer, "", 1, "L", false, 0, "")
	pdf.SetY(pdf.GetY() + internalNotesDisclaimerToValueGap)
	// User note
	pdf.SetX(textLeft)
	pdf.SetFont("Inter", "", bodyFontSize)
	pdf.SetTextColor(internalNotesValueColor[0], internalNotesValueColor[1], internalNotesValueColor[2])
	pdf.CellFormat(textWidth, internalNotesLineHeight, notes, "", 1, "L", false, 0, "")

	pdf.SetY(yStart + boxHeight)
	pdf.SetX(marginLR)
	pdf.Ln(internalNotesMarginBottom)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Inter", "", bodyFontSize)
}
