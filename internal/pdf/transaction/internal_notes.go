package transaction

import (
	"strings"

	"github.com/jung-kurt/gofpdf/v2"
)

const (
	internalNotesTitle      = "Internal Reference Notes (User-entered)"
	internalNotesDisclaimer = "This note is manually entered by the user for internal reference only. It is not part of the official transaction record."
	internalNotesPadding    = 4.0
	internalNotesLineHeight = 4.5
	internalNotesItalicSize = 7.5
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

	yStart := pdf.GetY()
	pdf.SetFillColor(greyAreaBgColor[0], greyAreaBgColor[1], greyAreaBgColor[2])
	pdf.SetDrawColor(defaultBorderColor[0], defaultBorderColor[1], defaultBorderColor[2])
	pdf.SetLineWidth(0.2)

	contentWidth := tableWidth
	boxHeight := internalNotesPadding*2 + internalNotesLineHeight*3 + 2.0 // title + disclaimer + user text + gaps
	pdf.Rect(marginLR, yStart, contentWidth, boxHeight, "FD")
	pdf.SetY(yStart + internalNotesPadding)
	pdf.SetX(marginLR + internalNotesPadding)

	pdf.SetFont("Inter", "I", sectionTitleSize-1)
	pdf.SetTextColor(sectionTitleColor[0], sectionTitleColor[1], sectionTitleColor[2])
	pdf.CellFormat(contentWidth-2*internalNotesPadding, internalNotesLineHeight, internalNotesTitle, "", 1, "L", false, 0, "")
	pdf.SetFont("Inter", "I", internalNotesItalicSize)
	pdf.SetTextColor(noteColor[0], noteColor[1], noteColor[2])
	pdf.CellFormat(contentWidth-2*internalNotesPadding, internalNotesLineHeight, internalNotesDisclaimer, "", 1, "L", false, 0, "")
	pdf.SetFont("Inter", "I", bodyFontSize)
	pdf.SetTextColor(0, 0, 0)
	pdf.CellFormat(contentWidth-2*internalNotesPadding, internalNotesLineHeight, notes, "", 1, "L", false, 0, "")

	pdf.SetY(yStart + boxHeight)
	pdf.SetX(marginLR)
	pdf.Ln(internalNotesPadding)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Inter", "", bodyFontSize)
}
