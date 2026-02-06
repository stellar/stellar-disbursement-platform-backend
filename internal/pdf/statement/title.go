package statement

import (
	"fmt"
	"strings"

	"github.com/jung-kurt/gofpdf/v2"
)

func drawTitleSection(pdf *gofpdf.Fpdf, walletAccount string) {
	walletAddr := strings.TrimPrefix(walletAccount, "stellar:")

	pdf.SetFont("Inter", "", bodyFontSize)
	pdf.SetTextColor(noteColor[0], noteColor[1], noteColor[2])
	pdf.CellFormat(0, titleSectionLine1Height, "REPORT", "", 1, "L", false, 0, "")

	pdf.SetFont("Inter", "B", titleFontSize)
	pdf.SetTextColor(highlightColor[0], highlightColor[1], highlightColor[2])
	pdf.CellFormat(0, titleSectionLine2Height, "Wallet Statement", "", 1, "L", false, 0, "")

	yWalletLine := pdf.GetY()
	pdf.SetFont("Inter", "", organizationNameFontSize)
	pdf.SetTextColor(defaultCellColor[0], defaultCellColor[1], defaultCellColor[2])
	labelText := "Wallet Address: "
	labelWidth := pdf.GetStringWidth(labelText)
	xLabelStart := pdf.GetX()
	pdf.SetXY(xLabelStart, yWalletLine)
	pdf.CellFormat(labelWidth, titleSectionLine3Height, labelText, "", 0, "L", false, 0, "")
	xValueStart := xLabelStart + labelWidth + walletAddressLabelGap
	pdf.SetFont("GoogleSansCode", "U", organizationNameFontSize)
	pdf.SetTextColor(defaultCellColor[0], defaultCellColor[1], defaultCellColor[2])
	pdf.SetXY(xValueStart, yWalletLine)
	walletURL := fmt.Sprintf("%saccount/%s", stellarExpertBaseURL, walletAddr)
	walletAddrWidth := pdf.GetStringWidth(walletAddr)
	pdf.CellFormat(walletAddrWidth, titleSectionLine3Height, walletAddr, "", 0, "L", false, 0, walletURL)
	pdf.LinkString(xValueStart, yWalletLine, walletAddrWidth, titleSectionLine3Height, walletURL)
	pdf.SetFont("Inter", "", organizationNameFontSize)
	pdf.SetTextColor(defaultCellColor[0], defaultCellColor[1], defaultCellColor[2])
	pdf.Ln(titleSectionLine3Height)

	pdf.Ln(titleSectionBottomMargin)
	pdf.SetTextColor(0, 0, 0)
}
