package shared

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"
	"time"

	"github.com/jung-kurt/gofpdf/v2"
)

const logoMaxHeight = 10.0

// HeaderLayout holds dimensions and colors for the PDF header.
type HeaderLayout struct {
	MmPerPage                    float64
	MarginLR                     float64
	TableWidth                   float64
	HeaderLeftColLineHeight      float64
	HeaderRightColLineHeight     float64
	HeaderBottomMargin           float64
	TitleSectionLine1Height      float64
	TitleSectionLine2Height     float64
	TitleSectionLine3Height     float64
	TitleSectionBottomMargin    float64
	MiniHeaderBottomMargin      float64
	BodyFontSize                 float64
	OrganizationNameFontSize     float64
	DateRangeFontSize            float64
	TitleFontSize                float64
	HeaderLogoToOrgNameGap       float64
	LogoOffsetX                  float64
	WalletAddressLabelGap        float64
	DefaultCellColor             []int
	HighlightColor               []int
	NoteColor                    []int
	DefaultBorderColor           []int
	HeaderSeparatorLineWidth     float64
}

// StatementPeriod holds the date range for the right column (statement PDF only).
type StatementPeriod struct {
	From, To time.Time
}

// TitleSection holds the title block below the top header.
type TitleSection struct {
	Title      string
	TitleLabel string
	TitleValue string
	TitleURL   string
}

// HeaderParams holds data and mode for the header.
type HeaderParams struct {
	OrganizationName    string
	OrganizationLogo    []byte
	StatementPeriod     *StatementPeriod
	TitleSection        *TitleSection
	WalletAccount       string
	WalletAccountDisplay string // optional truncated display for mini header; if empty use WalletAccount
	StellarExpertBaseURL string
}

// DrawHeader draws the full header: logo + left column (org name; if WalletAccount set, add wallet address line);
// right column ("Generated on"; if StatementPeriod set, add "Statement Period" + dates);
// if TitleSection set, draw REPORT + Title + label/value (underlined+link when TitleURL set); does not draw separator.
func DrawHeader(pdf *gofpdf.Fpdf, layout *HeaderLayout, params *HeaderParams) {
	xLeft := pdf.GetX()
	yStart := pdf.GetY()
	contentWidth := layout.TableWidth
	halfWidth := contentWidth / 2
	rightColX := xLeft + halfWidth

	yLeftBottom := yStart
	var logoWidth float64

	if len(params.OrganizationLogo) > 0 {
		imgName, imgInfo := registerLogoImage(pdf, params.OrganizationLogo)
		if imgName != "" && imgInfo != nil {
			imgW, imgH := imgInfo.Width(), imgInfo.Height()
			if imgH > logoMaxHeight {
				imgW = imgW * (logoMaxHeight / imgH)
				imgH = logoMaxHeight
			}
			pdf.ImageOptions(imgName, xLeft+layout.LogoOffsetX, yStart, imgW, imgH, false, gofpdf.ImageOptions{}, 0, "")
			logoWidth = imgW + layout.LogoOffsetX
			yLeftBottom = yStart + imgH + layout.HeaderLogoToOrgNameGap
		}
	}

	if params.WalletAccount != "" {
		// Mini header: left column has logo, then org name + "Wallet Address: " + link
		col2X := xLeft + logoWidth + layout.HeaderLogoToOrgNameGap
		col2Width := halfWidth - logoWidth - layout.HeaderLogoToOrgNameGap
		if col2Width > 0 {
			walletAddr := strings.TrimPrefix(params.WalletAccount, "stellar:")
			displayAddr := params.WalletAccountDisplay
			if displayAddr == "" {
				displayAddr = walletAddr
			}
			walletURL := fmt.Sprintf("%saccount/%s", params.StellarExpertBaseURL, walletAddr)

			if params.OrganizationName != "" {
				pdf.SetFont("Inter", "B", layout.OrganizationNameFontSize)
				pdf.SetTextColor(layout.HighlightColor[0], layout.HighlightColor[1], layout.HighlightColor[2])
				pdf.SetXY(col2X, yStart)
				pdf.CellFormat(col2Width, layout.HeaderLeftColLineHeight, strings.ToUpper(params.OrganizationName), "", 0, "L", false, 0, "")
			}
			yLine2 := yStart + layout.HeaderLeftColLineHeight
			pdf.SetFont("Inter", "", layout.OrganizationNameFontSize)
			pdf.SetTextColor(layout.NoteColor[0], layout.NoteColor[1], layout.NoteColor[2])
			labelText := "Wallet Address: "
			labelWidth := pdf.GetStringWidth(labelText)
			pdf.SetXY(col2X, yLine2)
			pdf.CellFormat(labelWidth, layout.TitleSectionLine3Height, labelText, "", 0, "L", false, 0, "")
			xValueStart := col2X + labelWidth + layout.WalletAddressLabelGap
			pdf.SetFont("GoogleSansCode", "U", layout.OrganizationNameFontSize)
			pdf.SetTextColor(layout.DefaultCellColor[0], layout.DefaultCellColor[1], layout.DefaultCellColor[2])
			displayWidth := pdf.GetStringWidth(displayAddr)
			pdf.SetXY(xValueStart, yLine2)
			pdf.CellFormat(displayWidth, layout.TitleSectionLine3Height, displayAddr, "", 0, "L", false, 0, walletURL)
			pdf.LinkString(xValueStart, yLine2, displayWidth, layout.TitleSectionLine3Height, walletURL)
			textBlockBottom := yLine2 + layout.TitleSectionLine3Height
			if textBlockBottom > yLeftBottom {
				yLeftBottom = textBlockBottom
			}
		}
	} else {
		// Standard left column: logo + org name only
		if params.OrganizationName != "" {
			pdf.SetFont("Inter", "B", layout.OrganizationNameFontSize)
			pdf.SetTextColor(layout.HighlightColor[0], layout.HighlightColor[1], layout.HighlightColor[2])
			pdf.SetXY(xLeft, yLeftBottom)
			pdf.CellFormat(halfWidth, layout.HeaderLeftColLineHeight, strings.ToUpper(params.OrganizationName), "", 0, "L", false, 0, "")
			yLeftBottom += layout.HeaderLeftColLineHeight
		}
	}

	// Right column: "Generated on"
	pdf.SetFont("Inter", "", layout.BodyFontSize)
	pdf.SetTextColor(layout.DefaultCellColor[0], layout.DefaultCellColor[1], layout.DefaultCellColor[2])
	genStr := fmt.Sprintf("Generated on %s", time.Now().UTC().Format("2006-01-02 15:04 UTC"))
	pdf.SetXY(rightColX, yStart)
	pdf.CellFormat(halfWidth, layout.HeaderRightColLineHeight, genStr, "", 0, "R", false, 0, "")

	yRightBottom := yStart + layout.HeaderRightColLineHeight
	if params.StatementPeriod != nil {
		pdf.SetXY(rightColX, yStart+layout.HeaderRightColLineHeight)
		pdf.SetFont("Inter", "emi", layout.BodyFontSize)
		pdf.SetTextColor(layout.HighlightColor[0], layout.HighlightColor[1], layout.HighlightColor[2])
		pdf.CellFormat(halfWidth, layout.HeaderRightColLineHeight, "Statement Period:", "", 0, "R", false, 0, "")
		periodStr := fmt.Sprintf("%s to %s", params.StatementPeriod.From.Format("2006-01-02"), params.StatementPeriod.To.Format("2006-01-02"))
		pdf.SetXY(rightColX, yStart+2*layout.HeaderRightColLineHeight)
		pdf.SetFont("Inter", "B", layout.DateRangeFontSize)
		pdf.SetTextColor(layout.HighlightColor[0], layout.HighlightColor[1], layout.HighlightColor[2])
		pdf.CellFormat(halfWidth, layout.HeaderRightColLineHeight, periodStr, "", 0, "R", false, 0, "")
		yRightBottom = yStart + 3*layout.HeaderRightColLineHeight
	}

	if yRightBottom > yLeftBottom {
		yLeftBottom = yRightBottom
	}
	pdf.SetXY(xLeft, yLeftBottom)

	headerBottom := layout.HeaderBottomMargin
	if params.WalletAccount != "" {
		headerBottom = layout.MiniHeaderBottomMargin
	}
	pdf.Ln(headerBottom)
	pdf.SetTextColor(0, 0, 0)

	// Title section: REPORT, title, label + value (with optional underlined link)
	if params.TitleSection != nil {
		ts := params.TitleSection
		pdf.SetFont("Inter", "", layout.BodyFontSize)
		pdf.SetTextColor(layout.NoteColor[0], layout.NoteColor[1], layout.NoteColor[2])
		pdf.CellFormat(0, layout.TitleSectionLine1Height, "REPORT", "", 1, "L", false, 0, "")

		pdf.SetFont("Inter", "B", layout.TitleFontSize)
		pdf.SetTextColor(layout.HighlightColor[0], layout.HighlightColor[1], layout.HighlightColor[2])
		pdf.CellFormat(0, layout.TitleSectionLine2Height, ts.Title, "", 1, "L", false, 0, "")

		yTitleLine := pdf.GetY()
		pdf.SetFont("Inter", "", layout.OrganizationNameFontSize)
		pdf.SetTextColor(layout.DefaultCellColor[0], layout.DefaultCellColor[1], layout.DefaultCellColor[2])
		labelWidth := pdf.GetStringWidth(ts.TitleLabel)
		xLabelStart := pdf.GetX()
		pdf.SetXY(xLabelStart, yTitleLine)
		pdf.CellFormat(labelWidth, layout.TitleSectionLine3Height, ts.TitleLabel, "", 0, "L", false, 0, "")
		xValueStart := xLabelStart + labelWidth + layout.WalletAddressLabelGap
		if ts.TitleURL != "" {
			pdf.SetFont("GoogleSansCode", "U", layout.OrganizationNameFontSize)
			valueWidth := pdf.GetStringWidth(ts.TitleValue)
			pdf.SetXY(xValueStart, yTitleLine)
			pdf.CellFormat(valueWidth, layout.TitleSectionLine3Height, ts.TitleValue, "", 0, "L", false, 0, ts.TitleURL)
			pdf.LinkString(xValueStart, yTitleLine, valueWidth, layout.TitleSectionLine3Height, ts.TitleURL)
		} else {
			pdf.SetFont("GoogleSansCode", "", layout.OrganizationNameFontSize)
			pdf.SetXY(xValueStart, yTitleLine)
			pdf.CellFormat(0, layout.TitleSectionLine3Height, ts.TitleValue, "", 0, "L", false, 0, "")
		}
		pdf.SetFont("Inter", "", layout.OrganizationNameFontSize)
		pdf.SetTextColor(layout.DefaultCellColor[0], layout.DefaultCellColor[1], layout.DefaultCellColor[2])
		pdf.Ln(layout.TitleSectionLine3Height)
		pdf.Ln(layout.TitleSectionBottomMargin)
		pdf.SetTextColor(0, 0, 0)
	}
}

// DrawHeaderSeparatorLine draws the horizontal line and advances by bottomMargin.
func DrawHeaderSeparatorLine(pdf *gofpdf.Fpdf, layout *HeaderLayout, bottomMargin float64) {
	pdf.SetDrawColor(layout.DefaultBorderColor[0], layout.DefaultBorderColor[1], layout.DefaultBorderColor[2])
	pdf.SetLineWidth(layout.HeaderSeparatorLineWidth)
	y := pdf.GetY()
	pdf.Line(layout.MarginLR, y, layout.MmPerPage-layout.MarginLR, y)
	pdf.SetY(y)
	pdf.Ln(bottomMargin)
	pdf.SetLineWidth(0.25)
	pdf.SetDrawColor(0, 0, 0)
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
