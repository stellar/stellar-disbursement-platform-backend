package statement

import (
	"bytes"
	"fmt"
	"time"

	"github.com/jung-kurt/gofpdf/v2"
	"github.com/shopspring/decimal"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

// BuildPDF generates a multi-page PDF from a StatementResult and returns the bytes.
func BuildPDF(result *services.StatementResult, fromDate, toDate time.Time, organizationName string, organizationLogo []byte) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")

	pdf.AddUTF8FontFromBytes("Inter", "", interRegularFont)
	pdf.AddUTF8FontFromBytes("Inter", "B", interBoldFont)
	pdf.AddUTF8FontFromBytes("Inter", "M", interMediumFont)
	pdf.AddUTF8FontFromBytes("Inter", "emi", interSemiBoldFont)
	pdf.AddUTF8FontFromBytes("GoogleSansCode", "", googleSansCodeRegularFont)

	pdf.SetMargins(marginLR, marginTop, marginLR)
	pdf.SetAutoPageBreak(true, marginBottom)
	setupFooter(pdf)
	pdf.AddPage()

	walletAccount := result.Summary.Account
	drawHeader(pdf, organizationName, organizationLogo, fromDate, toDate)
	drawTitleSection(pdf, walletAccount)
	drawHeaderSeparatorLine(pdf, headerSeparatorBottomMargin)

	drawSummaryTable(pdf, result)

	pageBottom := pageHeight - marginBottom
	for _, asset := range result.Summary.Assets {
		if len(asset.Transactions) == 0 {
			continue
		}
		pdf.SetFont("Inter", "B", sectionTitleSize)
		pdf.SetTextColor(sectionTitleColor[0], sectionTitleColor[1], sectionTitleColor[2])
		pdf.CellFormat(0, 8, "Transactions - "+asset.Code, "", 1, "L", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
		pdf.Ln(1)

		drawTxTableHeader(pdf)
		runningBalance, err := decimal.NewFromString(asset.BeginningBalance)
		if err != nil {
			runningBalance = decimal.Zero
		}
		transactions := asset.Transactions
		for i := range transactions {
			if pdf.GetY()+txDataRowHeight > pageBottom {
				pdf.AddPage()
				if pdf.PageNo() > 1 {
					drawMiniHeader(pdf, organizationName, organizationLogo, fromDate, toDate, walletAccount)
				}
				drawTxTableHeader(pdf)
			}
			runningBalance = drawTxRow(pdf, &transactions[i], asset.Code, runningBalance)
		}
		if pdf.GetY()+txDataRowHeight > pageBottom {
			pdf.AddPage()
			if pdf.PageNo() > 1 {
				drawMiniHeader(pdf, organizationName, organizationLogo, fromDate, toDate, walletAccount)
			}
			drawTxTableHeader(pdf)
		}
		drawTotalsRowForAsset(pdf, &asset)
		pdf.Ln(sectionBottomMargin)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("writing PDF: %w", err)
	}
	return buf.Bytes(), nil
}
