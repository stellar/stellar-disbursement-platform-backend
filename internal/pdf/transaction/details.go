package transaction

import (
	"github.com/jung-kurt/gofpdf/v2"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

const (
	walletAddressBreakChars = 38
	memoMaxChars            = 77
)

// Enrichment holds fields not on the payment entity (sender name, sender account address, fee charged),
// Stellar Expert base URL for wallet links, and optional disbursement Created by / Approved by.
type Enrichment struct {
	SenderName                      string
	SenderAccountName               string
	SenderAccountAddress            string
	FeeCharged                      string
	MemoText                        string
	StellarExpertBaseURL            string
	DisbursementCreatedByUserName   string
	DisbursementCreatedByTimestamp  string
	DisbursementApprovedByUserName  string
	DisbursementApprovedByTimestamp string
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// truncateWithEllipsis returns s truncated to maxChars runes with "..." appended if truncated.

// splitWalletAddressLines splits an address into lines of at most walletAddressBreakChars characters.
func splitWalletAddressLines(addr string) []string {
	if addr == "" {
		return nil
	}
	var lines []string
	for len(addr) > walletAddressBreakChars {
		lines = append(lines, addr[:walletAddressBreakChars])
		addr = addr[walletAddressBreakChars:]
	}
	if len(addr) > 0 {
		lines = append(lines, addr)
	}
	return lines
}

func walletAddressValueLines(addr string) int {
	if addr == "" || addr == "—" {
		return 1
	}
	lines := splitWalletAddressLines(addr)
	if len(lines) == 0 {
		return 1
	}
	return len(lines)
}

// detailRow is a single label + value pair. isWalletAddress is true when value should be drawn as Google Sans Code underlined link.
type detailRow struct {
	label           string
	value           string
	isWalletAddress bool
}

// detailRows lays out the two columns of the Transaction Details section. Circle rows appear only
// for payments that went through Circle.
func detailRows(payment *data.Payment, enrichment *Enrichment) (left, right []detailRow) {
	var senderName, senderAccountName, senderAccountAddr, feeCharged string
	if enrichment != nil {
		senderName = enrichment.SenderName
		senderAccountName = enrichment.SenderAccountName
		senderAccountAddr = enrichment.SenderAccountAddress
		feeCharged = enrichment.FeeCharged
	}

	left = []detailRow{
		{"Sender Name", orDash(senderName), false},
		{"Distribution Account", orDash(senderAccountName), false},
		{"Sender Account Address", orDash(senderAccountAddr), true},
	}
	if memoValue := memoForDisplay(payment, enrichment); memoValue != "" {
		left = append(left, detailRow{"MEMO (Text)", orDash(utils.TruncateToMaxLength(memoValue, memoMaxChars)), false})
	}
	left = append(left, detailRow{"Fee Charged", orDash(feeCharged), false})
	if payment.CircleTransactionID != nil {
		circleType := ""
		if payment.CircleTransactionType != nil {
			circleType = string(*payment.CircleTransactionType)
		}
		left = append(left,
			detailRow{"Circle Transaction ID", orDash(*payment.CircleTransactionID), false},
			detailRow{"Circle Transaction Type", orDash(circleType), false},
		)
	}

	right = []detailRow{
		{"Recipient Org ID", orDash(recipientOrgID(payment)), false},
		{"Recipient Wallet Address", orDash(recipientWalletAddress(payment)), true},
		{"Wallet Provider", orDash(walletProvider(payment)), false},
	}
	return left, right
}

// drawDetailsTable draws the Transaction Details section.
func drawDetailsTable(pdf *gofpdf.Fpdf, payment *data.Payment, enrichment *Enrichment, bottomMargin float64) {
	pdf.SetFont("Inter", "B", sectionTitleSize)
	pdf.SetTextColor(sectionTitleColor[0], sectionTitleColor[1], sectionTitleColor[2])
	pdf.CellFormat(0, 8, "Transaction Details", "", 1, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(1)

	halfWidth := tableWidth / 2
	labelWidth := 55.0
	baseURL := ""
	if enrichment != nil {
		baseURL = enrichment.StellarExpertBaseURL
	}
	leftRows, rightRows := detailRows(payment, enrichment)

	yStart := pdf.GetY()
	xLeft := pdf.GetX()
	xRight := xLeft + halfWidth + 2.0
	valueLines := func(row detailRow) int {
		if row.isWalletAddress && row.value != "" && row.value != "—" {
			return walletAddressValueLines(row.value)
		}
		return 1
	}
	rowHeight := func(row detailRow) float64 {
		return detailLabelLineHeight + float64(valueLines(row))*detailValueLineHeight + detailRowGap
	}
	maxRows := len(leftRows)
	if len(rightRows) > maxRows {
		maxRows = len(rightRows)
	}
	rowHeights := make([]float64, maxRows)
	for i := 0; i < maxRows; i++ {
		h := detailRowGap + detailLabelLineHeight
		if i < len(leftRows) {
			hL := rowHeight(leftRows[i])
			if hL > h {
				h = hL
			}
		}
		if i < len(rightRows) {
			hR := rowHeight(rightRows[i])
			if hR > h {
				h = hR
			}
		}
		rowHeights[i] = h
	}
	yPositions := make([]float64, maxRows+1)
	yPositions[0] = yStart
	for i := 0; i < maxRows; i++ {
		yPositions[i+1] = yPositions[i] + rowHeights[i]
	}

	drawDetailRow := func(x, y float64, row detailRow) {
		pdf.SetFont("Inter", "", bodyFontSize)
		pdf.SetTextColor(detailLabelColor[0], detailLabelColor[1], detailLabelColor[2])
		pdf.SetXY(x, y)
		pdf.CellFormat(labelWidth, detailLabelLineHeight, row.label+":", "", 0, "L", false, 0, "")
		valueY := y + detailLabelLineHeight
		pdf.SetFont("Inter", "B", bodyFontSize)
		pdf.SetTextColor(highlightColor[0], highlightColor[1], highlightColor[2])
		pdf.SetXY(x, valueY)
		if row.isWalletAddress && row.value != "—" && baseURL != "" {
			addr := row.value
			url := baseURL + "account/" + addr
			lines := splitWalletAddressLines(addr)
			if len(lines) == 0 {
				lines = []string{addr}
			}
			pdf.SetFont("GoogleSansCode", "U", bodyFontSize)
			for i, line := range lines {
				lineY := valueY + float64(i)*detailValueLineHeight
				pdf.SetXY(x, lineY)
				lineW := pdf.GetStringWidth(line)
				pdf.CellFormat(lineW, detailValueLineHeight, line, "", 0, "L", false, 0, url)
				pdf.LinkString(x, lineY, lineW, detailValueLineHeight, url)
			}
		} else {
			pdf.CellFormat(halfWidth-2.0, detailValueLineHeight, row.value, "", 0, "L", false, 0, "")
		}
	}

	for i, row := range leftRows {
		drawDetailRow(xLeft, yPositions[i], row)
	}
	for i, row := range rightRows {
		drawDetailRow(xRight, yPositions[i], row)
	}

	leftBottom := yPositions[len(leftRows)]
	pdf.SetXY(xLeft, leftBottom)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Inter", "", bodyFontSize)
	pdf.Ln(bottomMargin)
}

func enrichmentValue(e *Enrichment, ok bool, v string) string {
	if !ok || e == nil {
		return ""
	}
	return v
}

func memoForDisplay(p *data.Payment, e *Enrichment) string {
	if e != nil && e.MemoText != "" {
		return e.MemoText
	}
	return memoDisplay(p)
}

func memoDisplay(p *data.Payment) string {
	if p.ReceiverWallet != nil && p.ReceiverWallet.StellarMemo != "" {
		return p.ReceiverWallet.StellarMemo
	}
	return ""
}

func walletProvider(p *data.Payment) string {
	if p.ReceiverWallet != nil && p.ReceiverWallet.Wallet.Name != "" {
		return p.ReceiverWallet.Wallet.Name
	}
	return ""
}

func recipientOrgID(p *data.Payment) string {
	if p.ReceiverWallet != nil && p.ReceiverWallet.Receiver.ExternalID != "" {
		return p.ReceiverWallet.Receiver.ExternalID
	}
	return ""
}

func recipientWalletAddress(p *data.Payment) string {
	if p.ReceiverWallet != nil {
		return p.ReceiverWallet.StellarAddress
	}
	return ""
}
