package transaction

import (
	"bytes"
	"fmt"

	"github.com/jung-kurt/gofpdf/v2"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/pdf/shared"
)

// BuildPDF generates a single-page Transaction Notice PDF from a payment and returns the bytes.
// enrichment can be nil; internalNotes is optional (grey area omitted when empty).
func BuildPDF(payment *data.Payment, organizationName string, organizationLogo []byte, enrichment *Enrichment, internalNotes *string) ([]byte, error) {
	pdfDoc := gofpdf.New("P", "mm", "A4", "")

	pdfDoc.AddUTF8FontFromBytes("Inter", "", shared.InterRegularFont)
	pdfDoc.AddUTF8FontFromBytes("Inter", "B", shared.InterBoldFont)
	pdfDoc.AddUTF8FontFromBytes("Inter", "M", shared.InterMediumFont)
	pdfDoc.AddUTF8FontFromBytes("Inter", "emi", shared.InterSemiBoldFont)
	pdfDoc.AddUTF8FontFromBytes("Inter", "I", shared.InterItalicFont)
	pdfDoc.AddUTF8FontFromBytes("GoogleSansCode", "", shared.GoogleSansCodeRegularFont)
	pdfDoc.AddUTF8FontFromBytes("GoogleSansCode", "U", shared.GoogleSansCodeRegularFont)

	pdfDoc.SetMargins(marginLR, marginTop, marginLR)
	pdfDoc.SetAutoPageBreak(true, marginBottom)

	footerConfig := shared.FooterConfig{
		PageHeight:               pageHeight,
		MarginBottom:             marginBottom,
		MarginLR:                 marginLR,
		MmPerPage:                mmPerPage,
		FooterMarginTop:          footerMarginTop,
		FooterContentGap:         footerContentGap,
		FooterDisclaimerToPageGap: footerDisclaimerToPageGap,
		FooterLineHeight:         footerLineHeight,
		BodyFontSize:             bodyFontSize,
		NoteColor:                noteColor,
		DefaultBorderColor:       defaultBorderColor,
		HeaderSeparatorLineWidth: headerSeparatorLineWidth,
	}
	shared.SetupFooter(pdfDoc, footerConfig)
	pdfDoc.AddPage()

	headerLayout := transactionHeaderLayout()
	titleURL := ""
	if stellarExpertBaseURL != "" && payment.StellarTransactionID != "" {
		titleURL = stellarExpertBaseURL + "tx/" + payment.StellarTransactionID
	}
	shared.DrawHeader(pdfDoc, headerLayout, &shared.HeaderParams{
		OrganizationName:     organizationName,
		OrganizationLogo:     organizationLogo,
		StatementPeriod:      nil,
		TitleSection:         &shared.TitleSection{
			Title:      "Transaction Notice",
			TitleLabel: "Stellar Transaction ID: ",
			TitleValue: payment.StellarTransactionID,
			TitleURL:   titleURL,
		},
		WalletAccount:        "",
		StellarExpertBaseURL: stellarExpertBaseURL,
	})
	shared.DrawHeaderSeparatorLine(pdfDoc, headerLayout, headerSeparatorBottomMargin)

	drawSummaryTable(pdfDoc, payment)
	drawDetailsTable(pdfDoc, payment, enrichment)
	if payment.Type == data.PaymentTypeDisbursement && payment.Disbursement != nil {
		drawDisbursementDetailsSection(pdfDoc, payment, enrichment)
	}

	notesStr := ""
	if internalNotes != nil && *internalNotes != "" {
		notesStr = *internalNotes
	}
	drawInternalNotes(pdfDoc, notesStr)

	var buf bytes.Buffer
	if err := pdfDoc.Output(&buf); err != nil {
		return nil, fmt.Errorf("writing PDF: %w", err)
	}
	return buf.Bytes(), nil
}

func transactionHeaderLayout() *shared.HeaderLayout {
	return &shared.HeaderLayout{
		MmPerPage:                 mmPerPage,
		MarginLR:                  marginLR,
		TableWidth:                tableWidth,
		HeaderLeftColLineHeight:   headerLeftColLineHeight,
		HeaderRightColLineHeight:  headerRightColLineHeight,
		HeaderBottomMargin:        headerBottomMargin,
		TitleSectionLine1Height:   titleSectionLine1Height,
		TitleSectionLine2Height:   titleSectionLine2Height,
		TitleSectionLine3Height:   titleSectionLine3Height,
		TitleSectionBottomMargin:  titleSectionBottomMargin,
		MiniHeaderBottomMargin:    headerBottomMargin,
		BodyFontSize:              bodyFontSize,
		OrganizationNameFontSize:  organizationNameFontSize,
		DateRangeFontSize:         dateRangeFontSize,
		TitleFontSize:             titleFontSize,
		HeaderLogoToOrgNameGap:    headerLogoToOrgNameGap,
		LogoOffsetX:               logoOffsetX,
		WalletAddressLabelGap:     walletAddressLabelGap,
		DefaultCellColor:          defaultCellColor,
		HighlightColor:            highlightColor,
		NoteColor:                 noteColor,
		DefaultBorderColor:        defaultBorderColor,
		HeaderSeparatorLineWidth:  headerSeparatorLineWidth,
	}
}
