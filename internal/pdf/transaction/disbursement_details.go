package transaction

import (
	"github.com/jung-kurt/gofpdf/v2"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

func drawDisbursementDetailRowAt(pdf *gofpdf.Fpdf, x, y float64, label, value string) {
	pdf.SetFont("Inter", "", bodyFontSize)
	pdf.SetTextColor(detailLabelColor[0], detailLabelColor[1], detailLabelColor[2])
	pdf.SetXY(x, y)
	pdf.CellFormat(0, detailLabelLineHeight, label+":", "", 0, "L", false, 0, "")
	pdf.SetFont("Inter", "B", bodyFontSize)
	pdf.SetTextColor(highlightColor[0], highlightColor[1], highlightColor[2])
	pdf.SetXY(x, y+detailLabelLineHeight)
	pdf.CellFormat(0, detailValueLineHeight, value, "", 0, "L", false, 0, "")
}

func drawDisbursementDetailCreatedApprovedAt(pdf *gofpdf.Fpdf, x, y float64, label, userName, timestamp string) {
	pdf.SetFont("Inter", "", bodyFontSize)
	pdf.SetTextColor(detailLabelColor[0], detailLabelColor[1], detailLabelColor[2])
	pdf.SetXY(x, y)
	pdf.CellFormat(0, detailLabelLineHeight, label+":", "", 0, "L", false, 0, "")
	pdf.SetFont("Inter", "B", bodyFontSize)
	pdf.SetTextColor(highlightColor[0], highlightColor[1], highlightColor[2])
	pdf.SetXY(x, y+detailLabelLineHeight)
	if userName == "" {
		userName = "—"
	}
	pdf.CellFormat(0, detailValueLineHeight, userName, "", 0, "L", false, 0, "")
	pdf.SetFont("Inter", "", bodyFontSize)
	pdf.SetTextColor(detailLabelColor[0], detailLabelColor[1], detailLabelColor[2])
	pdf.SetXY(x, y+detailLabelLineHeight+detailValueLineHeight)
	if timestamp == "" {
		timestamp = "—"
	}
	pdf.CellFormat(0, detailValueLineHeight, timestamp, "", 0, "L", false, 0, "")
}

// drawDisbursementDetailsSection draws the "Disbursement Details" section when payment is a disbursement.
func drawDisbursementDetailsSection(pdf *gofpdf.Fpdf, payment *data.Payment, enrichment *Enrichment) {
	if payment.Disbursement == nil {
		return
	}
	d := payment.Disbursement

	pdf.SetFont("Inter", "B", sectionTitleSize)
	pdf.SetTextColor(sectionTitleColor[0], sectionTitleColor[1], sectionTitleColor[2])
	pdf.CellFormat(0, 8, "Disbursement Details", "", 1, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(1)

	halfWidth := tableWidth / 2
	yStart := pdf.GetY()
	xLeft := pdf.GetX()
	xRight := xLeft + halfWidth + 2.0

	// Block height: label + value + gap (for Disbursement ID / Name row)
	block1Height := detailLabelLineHeight + detailValueLineHeight + detailRowGap
	// Block height: label + name + date + gap (for Created by / Approved by)
	block2Height := detailLabelLineHeight + detailValueLineHeight*2 + detailRowGap

	// Column 1: Disbursement ID, Created by
	drawDisbursementDetailRowAt(pdf, xLeft, yStart, "Disbursement ID", d.ID)
	yCreated := yStart + block1Height
	var createdByName, createdByTs, approvedByName, approvedByTs string
	if enrichment != nil {
		createdByName = enrichment.DisbursementCreatedByUserName
		createdByTs = enrichment.DisbursementCreatedByTimestamp
		approvedByName = enrichment.DisbursementApprovedByUserName
		approvedByTs = enrichment.DisbursementApprovedByTimestamp
	}
	drawDisbursementDetailCreatedApprovedAt(pdf, xLeft, yCreated, "Created by", createdByName, createdByTs)

	// Column 2: Disbursement Name, Approved by
	drawDisbursementDetailRowAt(pdf, xRight, yStart, "Disbursement Name", d.Name)
	drawDisbursementDetailCreatedApprovedAt(pdf, xRight, yCreated, "Approved by", approvedByName, approvedByTs)

	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Inter", "", bodyFontSize)
	pdf.SetXY(xLeft, yStart+block1Height+block2Height)
	pdf.Ln(sectionBottomMargin)
}
