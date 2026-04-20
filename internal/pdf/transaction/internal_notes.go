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
// The note section is anchored at the bottom and grows upward as more lines are added.
func drawInternalNotes(pdf *gofpdf.Fpdf, internalNotes string) {
	if internalNotes == "" {
		return
	}
	notes := strings.TrimSpace(internalNotes)
	if notes == "" {
		return
	}
	if len(notes) > internalNotesMaxLength {
		notes = notes[:internalNotesMaxLength]
	}
	notes = strings.TrimSpace(notes)
	if notes == "" {
		return
	}

	contentWidth := tableWidth
	textLeft := marginLR + internalNotesPadding
	textWidth := contentWidth - 2*internalNotesPadding

	pdf.SetFont("Inter", "", bodyFontSize)
	noteLines := splitNoteIntoLines(pdf, notes, textWidth)

	fixedPartHeight := internalNotesPadding*2 + internalNotesLineHeight*2 + internalNotesDisclaimerToValueGap
	userNoteHeight := float64(len(noteLines)) * internalNotesLineHeight
	boxHeight := fixedPartHeight + userNoteHeight
	boxBottom := pageHeight - marginBottom - internalNotesMarginBottom
	yStart := boxBottom - boxHeight

	pdf.SetFillColor(internalNotesBgColor[0], internalNotesBgColor[1], internalNotesBgColor[2])
	pdf.SetDrawColor(internalNotesBorderColor[0], internalNotesBorderColor[1], internalNotesBorderColor[2])
	pdf.SetLineWidth(0.2)
	pdf.RoundedRect(marginLR, yStart, contentWidth, boxHeight, internalNotesCornerRadius, "1234", "FD")

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
	pdf.SetFont("Inter", "", bodyFontSize)
	pdf.SetTextColor(internalNotesValueColor[0], internalNotesValueColor[1], internalNotesValueColor[2])
	for _, line := range noteLines {
		pdf.SetX(textLeft)
		pdf.CellFormat(textWidth, internalNotesLineHeight, line, "", 1, "L", false, 0, "")
	}

	pdf.SetY(yStart + boxHeight)
	pdf.SetX(marginLR)
	pdf.Ln(internalNotesMarginBottom)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Inter", "", bodyFontSize)
}

// splitNoteIntoLines splits the note into display lines, respecting explicit newlines and wrapping to textWidth.
func splitNoteIntoLines(pdf *gofpdf.Fpdf, notes string, textWidth float64) []string {
	paragraphs := strings.Split(notes, "\n")
	var lines []string
	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		wrapped := pdf.SplitText(para, textWidth)
		lines = append(lines, wrapped...)
	}
	return lines
}
