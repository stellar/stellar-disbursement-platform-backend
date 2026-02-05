package statement

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"
	"time"

	"github.com/jung-kurt/gofpdf/v2"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// drawHeader draws the header: left column (logo + organization name), right column (generated date, statement period).
func drawHeader(pdf *gofpdf.Fpdf, organizationName string, organizationLogo []byte, fromDate, toDate time.Time) {
	xLeft := pdf.GetX()
	yStart := pdf.GetY()
	contentWidth := tableWidth
	halfWidth := contentWidth / 2
	rightColX := xLeft + halfWidth

	yLeftBottom := yStart

	if len(organizationLogo) > 0 {
		imgName, imgInfo := registerLogoImage(pdf, organizationLogo)
		if imgName != "" && imgInfo != nil {
			const logoMaxHeight = 10.0
			imgW, imgH := imgInfo.Width(), imgInfo.Height()
			if imgH > logoMaxHeight {
				imgW = imgW * (logoMaxHeight / imgH)
				imgH = logoMaxHeight
			}
			pdf.ImageOptions(imgName, xLeft+logoOffsetX, yStart, imgW, imgH, false, gofpdf.ImageOptions{}, 0, "")
			yLeftBottom = yStart + imgH + headerLogoToOrgNameGap
		}
	}
	if organizationName != "" {
		pdf.SetFont("Inter", "B", organizationNameFontSize)
		pdf.SetTextColor(highlightColor[0], highlightColor[1], highlightColor[2])
		pdf.SetXY(xLeft, yLeftBottom)
		pdf.CellFormat(halfWidth, headerLeftColLineHeight, strings.ToUpper(organizationName), "", 0, "L", false, 0, "")
		yLeftBottom += headerLeftColLineHeight
	}

	pdf.SetFont("Inter", "", bodyFontSize)
	pdf.SetTextColor(defaultCellColor[0], defaultCellColor[1], defaultCellColor[2])
	genStr := fmt.Sprintf("Generated on %s", time.Now().UTC().Format("2006-01-02 15:04 UTC"))
	pdf.SetXY(rightColX, yStart)
	pdf.CellFormat(halfWidth, headerRightColLineHeight, genStr, "", 0, "R", false, 0, "")
	pdf.SetXY(rightColX, yStart+headerRightColLineHeight)
	pdf.SetFont("Inter", "emi", bodyFontSize)
	pdf.SetTextColor(highlightColor[0], highlightColor[1], highlightColor[2])
	pdf.CellFormat(halfWidth, headerRightColLineHeight, "Statement Period:", "", 0, "R", false, 0, "")
	periodStr := fmt.Sprintf("%s to %s", fromDate.Format("2006-01-02"), toDate.Format("2006-01-02"))
	pdf.SetXY(rightColX, yStart+2*headerRightColLineHeight)
	pdf.SetFont("Inter", "B", dateRangeFontSize)
	pdf.SetTextColor(highlightColor[0], highlightColor[1], highlightColor[2])
	pdf.CellFormat(halfWidth, headerRightColLineHeight, periodStr, "", 0, "R", false, 0, "")
	yRightBottom := yStart + 3*headerRightColLineHeight

	if yRightBottom > yLeftBottom {
		yLeftBottom = yRightBottom
	}
	pdf.SetXY(xLeft, yLeftBottom)
	pdf.Ln(headerBottomMargin)
	pdf.SetTextColor(0, 0, 0)
}

// drawMiniHeader draws a compact header for pages 2+
func drawMiniHeader(pdf *gofpdf.Fpdf, organizationName string, organizationLogo []byte, fromDate, toDate time.Time, walletAccount string) {
	xLeft := pdf.GetX()
	yStart := pdf.GetY()
	contentWidth := tableWidth
	halfWidth := contentWidth / 2
	rightColX := xLeft + halfWidth

	yLeftBottom := yStart
	var logoWidth float64

	if len(organizationLogo) > 0 {
		imgName, imgInfo := registerLogoImage(pdf, organizationLogo)
		if imgName != "" && imgInfo != nil {
			const logoMaxHeight = 10.0
			imgW, imgH := imgInfo.Width(), imgInfo.Height()
			if imgH > logoMaxHeight {
				imgW = imgW * (logoMaxHeight / imgH)
				imgH = logoMaxHeight
			}
			pdf.ImageOptions(imgName, xLeft+logoOffsetX, yStart, imgW, imgH, false, gofpdf.ImageOptions{}, 0, "")
			logoWidth = imgW + logoOffsetX
			yLeftBottom = yStart + imgH + headerLogoToOrgNameGap
		}
	}

	col2X := xLeft + logoWidth + headerLogoToOrgNameGap
	col2Width := halfWidth - logoWidth - headerLogoToOrgNameGap
	if col2Width > 0 {
		walletAddr := strings.TrimPrefix(walletAccount, "stellar:")
		truncatedAddr := utils.TruncateString(walletAddr, 5)

		if organizationName != "" {
			pdf.SetFont("Inter", "B", organizationNameFontSize)
			pdf.SetTextColor(highlightColor[0], highlightColor[1], highlightColor[2])
			pdf.SetXY(col2X, yStart)
			pdf.CellFormat(col2Width, headerLeftColLineHeight, strings.ToUpper(organizationName), "", 0, "L", false, 0, "")
		}
		yLine2 := yStart + headerLeftColLineHeight
		pdf.SetFont("Inter", "", organizationNameFontSize)
		pdf.SetTextColor(noteColor[0], noteColor[1], noteColor[2])
		labelText := "Wallet Address: "
		labelWidth := pdf.GetStringWidth(labelText)
		pdf.SetXY(col2X, yLine2)
		pdf.CellFormat(labelWidth, titleSectionLine3Height, labelText, "", 0, "L", false, 0, "")
		xValueStart := col2X + labelWidth + walletAddressLabelGap
		pdf.SetFont("GoogleSansCode", "U", organizationNameFontSize)
		pdf.SetTextColor(defaultCellColor[0], defaultCellColor[1], defaultCellColor[2])
		walletURL := fmt.Sprintf("%saccount/%s", stellarExpertTestnetBaseURL, walletAddr)
		truncatedWidth := pdf.GetStringWidth(truncatedAddr)
		pdf.SetXY(xValueStart, yLine2)
		pdf.CellFormat(truncatedWidth, titleSectionLine3Height, truncatedAddr, "", 0, "L", false, 0, walletURL)
		pdf.LinkString(xValueStart, yLine2, truncatedWidth, titleSectionLine3Height, walletURL)
		textBlockBottom := yLine2 + titleSectionLine3Height
		if textBlockBottom > yLeftBottom {
			yLeftBottom = textBlockBottom
		}
	}

	pdf.SetFont("Inter", "", bodyFontSize)
	pdf.SetTextColor(defaultCellColor[0], defaultCellColor[1], defaultCellColor[2])
	genStr := fmt.Sprintf("Generated on %s", time.Now().UTC().Format("2006-01-02 15:04 UTC"))
	pdf.SetXY(rightColX, yStart)
	pdf.CellFormat(halfWidth, headerRightColLineHeight, genStr, "", 0, "R", false, 0, "")
	pdf.SetXY(rightColX, yStart+headerRightColLineHeight)
	pdf.SetFont("Inter", "emi", bodyFontSize)
	pdf.SetTextColor(highlightColor[0], highlightColor[1], highlightColor[2])
	pdf.CellFormat(halfWidth, headerRightColLineHeight, "Statement Period:", "", 0, "R", false, 0, "")
	periodStr := fmt.Sprintf("%s to %s", fromDate.Format("2006-01-02"), toDate.Format("2006-01-02"))
	pdf.SetXY(rightColX, yStart+2*headerRightColLineHeight)
	pdf.SetFont("Inter", "B", dateRangeFontSize)
	pdf.SetTextColor(highlightColor[0], highlightColor[1], highlightColor[2])
	pdf.CellFormat(halfWidth, headerRightColLineHeight, periodStr, "", 0, "R", false, 0, "")
	yRightBottom := yStart + 3*headerRightColLineHeight

	if yRightBottom > yLeftBottom {
		yLeftBottom = yRightBottom
	}
	pdf.SetXY(xLeft, yLeftBottom)
	pdf.Ln(miniHeaderBottomMargin)
	pdf.SetTextColor(0, 0, 0)
	drawHeaderSeparatorLine(pdf, miniHeaderSeparatorBottomMargin)
}

func registerLogoImage(pdf *gofpdf.Fpdf, logoBytes []byte) (string, *gofpdf.ImageInfoType) {
	_, format, err := image.Decode(bytes.NewReader(logoBytes))
	if err != nil {
		return "", nil
	}
	var imageType string
	switch format {
	case "jpeg", "jpg":
		imageType = "JPEG"
	case "png":
		imageType = "PNG"
	default:
		return "", nil
	}
	opts := gofpdf.ImageOptions{ImageType: imageType}
	info := pdf.RegisterImageOptionsReader("orglogo", opts, bytes.NewReader(logoBytes))
	if info == nil {
		return "", nil
	}
	return "orglogo", info
}

func drawHeaderSeparatorLine(pdf *gofpdf.Fpdf, bottomMargin float64) {
	pdf.SetDrawColor(defaultBorderColor[0], defaultBorderColor[1], defaultBorderColor[2])
	pdf.SetLineWidth(headerSeparatorLineWidth)
	y := pdf.GetY()
	pdf.Line(marginLR, y, mmPerPage-marginLR, y)
	pdf.SetY(y)
	pdf.Ln(bottomMargin)
	pdf.SetLineWidth(0.25)
	pdf.SetDrawColor(0, 0, 0)
}
