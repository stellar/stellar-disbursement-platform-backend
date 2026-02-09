package statement

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/jung-kurt/gofpdf/v2"
	"github.com/shopspring/decimal"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/pdf/shared"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// BuildPDF generates a multi-page PDF from a StatementResult and returns the bytes.
func BuildPDF(result *services.StatementResult, fromDate, toDate time.Time, organizationName string, organizationLogo []byte) ([]byte, error) {
	pdfDoc := gofpdf.New("P", "mm", "A4", "")

	pdfDoc.AddUTF8FontFromBytes("Inter", "", shared.InterRegularFont)
	pdfDoc.AddUTF8FontFromBytes("Inter", "B", shared.InterBoldFont)
	pdfDoc.AddUTF8FontFromBytes("Inter", "M", shared.InterMediumFont)
	pdfDoc.AddUTF8FontFromBytes("Inter", "emi", shared.InterSemiBoldFont)
	pdfDoc.AddUTF8FontFromBytes("GoogleSansCode", "", shared.GoogleSansCodeRegularFont)
	pdfDoc.AddUTF8FontFromBytes("GoogleSansCode", "U", shared.GoogleSansCodeRegularFont)

	pdfDoc.SetMargins(marginLR, marginTop, marginLR)
	pdfDoc.SetAutoPageBreak(true, marginBottom)

	footerConfig := shared.FooterConfig{
		PageHeight:                pageHeight,
		MarginBottom:              marginBottom,
		MarginLR:                  marginLR,
		MmPerPage:                 mmPerPage,
		FooterMarginTop:           footerMarginTop,
		FooterContentGap:          footerContentGap,
		FooterDisclaimerToPageGap: footerDisclaimerToPageGap,
		FooterLineHeight:          footerLineHeight,
		BodyFontSize:              bodyFontSize,
		NoteColor:                 noteColor,
		DefaultBorderColor:        defaultBorderColor,
		HeaderSeparatorLineWidth:  headerSeparatorLineWidth,
	}
	shared.SetupFooter(pdfDoc, footerConfig)
	pdfDoc.AddPage()

	headerLayout := statementHeaderLayout()
	walletAccount := result.Summary.Account
	walletAddr := strings.TrimPrefix(walletAccount, "stellar:")
	walletURL := fmt.Sprintf("%saccount/%s", stellarExpertBaseURL, walletAddr)

	// First page header with title section (Wallet Statement + Wallet Address link)
	shared.DrawHeader(pdfDoc, headerLayout, &shared.HeaderParams{
		OrganizationName: organizationName,
		OrganizationLogo: organizationLogo,
		StatementPeriod:  &shared.StatementPeriod{From: fromDate, To: toDate},
		TitleSection: &shared.TitleSection{
			Title:      "Wallet Statement",
			TitleLabel: "Wallet Address: ",
			TitleValue: walletAddr,
			TitleURL:   walletURL,
		},
		WalletAccount:        "",
		StellarExpertBaseURL: stellarExpertBaseURL,
	})
	shared.DrawHeaderSeparatorLine(pdfDoc, headerLayout, headerSeparatorBottomMargin)

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
					shared.DrawHeader(pdfDoc, headerLayout, &shared.HeaderParams{
						OrganizationName:     organizationName,
						OrganizationLogo:     organizationLogo,
						StatementPeriod:      &shared.StatementPeriod{From: fromDate, To: toDate},
						TitleSection:         nil,
						WalletAccount:        walletAccount,
						WalletAccountDisplay: utils.TruncateString(walletAddr, 5),
						StellarExpertBaseURL: stellarExpertBaseURL,
					})
					shared.DrawHeaderSeparatorLine(pdfDoc, headerLayout, miniHeaderSeparatorBottomMargin)
				}
				drawTxTableHeader(pdfDoc)
			}
			runningBalance = drawTxRow(pdfDoc, &transactions[i], asset.Code, runningBalance)
		}
		if pdfDoc.GetY()+txDataRowHeight > pageBottom {
			pdfDoc.AddPage()
			if pdfDoc.PageNo() > 1 {
				shared.DrawHeader(pdfDoc, headerLayout, &shared.HeaderParams{
					OrganizationName:     organizationName,
					OrganizationLogo:     organizationLogo,
					StatementPeriod:      &shared.StatementPeriod{From: fromDate, To: toDate},
					TitleSection:         nil,
					WalletAccount:        walletAccount,
					WalletAccountDisplay: utils.TruncateString(walletAddr, 5),
					StellarExpertBaseURL: stellarExpertBaseURL,
				})
				shared.DrawHeaderSeparatorLine(pdfDoc, headerLayout, miniHeaderSeparatorBottomMargin)
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

func statementHeaderLayout() *shared.HeaderLayout {
	return &shared.HeaderLayout{
		MmPerPage:                mmPerPage,
		MarginLR:                 marginLR,
		TableWidth:               tableWidth,
		HeaderLeftColLineHeight:  headerLeftColLineHeight,
		HeaderRightColLineHeight: headerRightColLineHeight,
		HeaderBottomMargin:       headerBottomMargin,
		TitleSectionLine1Height:  titleSectionLine1Height,
		TitleSectionLine2Height:  titleSectionLine2Height,
		TitleSectionLine3Height:  titleSectionLine3Height,
		TitleSectionBottomMargin: titleSectionBottomMargin,
		MiniHeaderBottomMargin:   miniHeaderBottomMargin,
		BodyFontSize:             bodyFontSize,
		OrganizationNameFontSize: organizationNameFontSize,
		DateRangeFontSize:        dateRangeFontSize,
		TitleFontSize:            titleFontSize,
		HeaderLogoToOrgNameGap:   headerLogoToOrgNameGap,
		LogoOffsetX:              logoOffsetX,
		WalletAddressLabelGap:    walletAddressLabelGap,
		DefaultCellColor:         defaultCellColor,
		HighlightColor:           highlightColor,
		NoteColor:                noteColor,
		DefaultBorderColor:       defaultBorderColor,
		HeaderSeparatorLineWidth: headerSeparatorLineWidth,
	}
}
