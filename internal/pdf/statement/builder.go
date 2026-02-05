package statement

import (
	"bytes"
	"fmt"
	"time"

	"github.com/jung-kurt/gofpdf/v2"
	"github.com/shopspring/decimal"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/pdf/shared"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

// BuildPDF generates a multi-page PDF from a StatementResult and returns the bytes.
func BuildPDF(result *services.StatementResult, fromDate, toDate time.Time, organizationName string, organizationLogo []byte) ([]byte, error) {
	pdfDoc := gofpdf.New("P", "mm", "A4", "")

	pdfDoc.AddUTF8FontFromBytes("Inter", "", shared.InterRegularFont)
	pdfDoc.AddUTF8FontFromBytes("Inter", "B", shared.InterBoldFont)
	pdfDoc.AddUTF8FontFromBytes("Inter", "M", shared.InterMediumFont)
	pdfDoc.AddUTF8FontFromBytes("Inter", "emi", shared.InterSemiBoldFont)
	pdfDoc.AddUTF8FontFromBytes("GoogleSansCode", "", shared.GoogleSansCodeRegularFont)

	pdfDoc.SetMargins(marginLR, marginTop, marginLR)
	pdfDoc.SetAutoPageBreak(true, marginBottom)
	setupFooter(pdfDoc)
	pdfDoc.AddPage()

	walletAccount := result.Summary.Account
	drawHeader(pdfDoc, organizationName, organizationLogo, fromDate, toDate)
	drawTitleSection(pdfDoc, walletAccount)
	drawHeaderSeparatorLine(pdfDoc, headerSeparatorBottomMargin)

	drawSummaryTable(pdfDoc, result)

	pageBottom := pageHeight - marginBottom
	for _, asset := range result.Summary.Assets {
		if len(asset.Transactions) == 0 {
			continue
		}
		pdfDoc.SetFont("Inter", "B", sectionTitleSize)
		pdfDoc.SetTextColor(sectionTitleColor[0], sectionTitleColor[1], sectionTitleColor[2])
		pdfDoc.CellFormat(0, 8, "Transactions - "+asset.Code, "", 1, "L", false, 0, "")
		pdfDoc.SetTextColor(0, 0, 0)
		pdfDoc.Ln(1)

		drawTxTableHeader(pdfDoc)
		runningBalance, err := decimal.NewFromString(asset.BeginningBalance)
		if err != nil {
			runningBalance = decimal.Zero
		}
		transactions := asset.Transactions
		for i := range transactions {
			if pdfDoc.GetY()+txDataRowHeight > pageBottom {
				pdfDoc.AddPage()
				if pdfDoc.PageNo() > 1 {
					drawMiniHeader(pdfDoc, organizationName, organizationLogo, fromDate, toDate, walletAccount)
				}
				drawTxTableHeader(pdfDoc)
			}
			runningBalance = drawTxRow(pdfDoc, &transactions[i], asset.Code, runningBalance)
		}
		if pdfDoc.GetY()+txDataRowHeight > pageBottom {
			pdfDoc.AddPage()
			if pdfDoc.PageNo() > 1 {
				drawMiniHeader(pdfDoc, organizationName, organizationLogo, fromDate, toDate, walletAccount)
			}
			drawTxTableHeader(pdfDoc)
		}
		drawTotalsRowForAsset(pdfDoc, &asset)
		pdfDoc.Ln(sectionBottomMargin)
	}

	var buf bytes.Buffer
	if err := pdfDoc.Output(&buf); err != nil {
		return nil, fmt.Errorf("writing PDF: %w", err)
	}
	return buf.Bytes(), nil
}
