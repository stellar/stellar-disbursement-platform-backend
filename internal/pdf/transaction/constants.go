package transaction

import (
	"os"
	"strings"
)

// Page dimensions and margins
const (
	mmPerPage    = 210.0
	pageHeight   = 297.0
	marginLR     = 15.0
	marginTop    = 15.0
	marginBottom = 25.0
)

// Font sizes
const (
	bodyFontSize             = 8.0
	tableHeaderSize          = 8.0
	sectionTitleSize         = 12.0
	organizationNameFontSize = 9.2
	dateRangeFontSize        = 10.0
	titleFontSize            = 23.4
)

// Line heights
const (
	headerLeftColLineHeight  = 6.0
	headerRightColLineHeight = 5.0
	footerLineHeight         = 4.0
	titleSectionLine1Height  = 6.0
	titleSectionLine2Height  = 9.0
	titleSectionLine3Height  = 6.0
	detailLabelLineHeight    = 4.0
	detailValueLineHeight    = 4.0
)

// Table row heights
const (
	summaryHeaderRowHeight = 15.5
	summaryDataRowHeight   = 14
)

// Table cell spacing
const (
	cellPaddingX = 2.115
)

// Section margins
const (
	sectionBottomMargin = 4.0
)

// Row gap between label+value rows
const (
	detailRowGap = 3.0
)

// Header section spacing
const (
	headerBottomMargin          = 5.0
	headerSeparatorBottomMargin = 6.0
	headerSeparatorLineWidth    = 0.26
	headerLogoToOrgNameGap      = 3.0
	logoOffsetX                 = 1.0
	walletAddressLabelGap       = 0.3
)

// Title section spacing
const (
	titleSectionBottomMargin = 5.0
)

// Footer section spacing
const (
	footerMarginTop           = 1.5
	footerContentGap          = 3.5
	footerDisclaimerToPageGap = 2.0
)

// Internal notes
const (
	internalNotesMaxLength     = 100
	internalNotesCornerRadius  = 1.5
	internalNotesMarginBottom  = 3.0
	internalNotesSmallFontSize = 7.0
)

// Stellar Expert Explorer URL
// Gets the base URL from STELLAR_EXPERT_URL environment variable; defaults to testnet if not set.
const stellarExpertTestnetDefault = "https://stellar.expert/explorer/testnet/"

var stellarExpertBaseURL = func() string {
	url := os.Getenv("STELLAR_EXPERT_URL")
	if url == "" {
		return stellarExpertTestnetDefault
	}
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	return url
}()

// GetStellarExpertBaseURL returns the base URL for Stellar Expert (used by the handler to set Enrichment).
func GetStellarExpertBaseURL() string {
	return stellarExpertBaseURL
}

const tableWidth = mmPerPage - 2*marginLR

// Transaction Summary columns: Amount, Currency, Status, Payment ID, Status Update At
var summaryColWidths = []float64{36, 28, 32, 40, 44}

var (
	headerAndTotalsColor = []int{54, 65, 83}
	defaultCellColor     = []int{74, 85, 101}
	noteColor            = []int{106, 114, 130}
	detailLabelColor     = []int{106, 114, 130}
	highlightColor       = []int{16, 24, 40}
	sectionTitleColor    = []int{30, 41, 57}
	successGreen         = []int{34, 139, 34}
)

var (
	headerBorderColor        = []int{203, 207, 215}
	defaultBorderColor       = []int{240, 241, 243}
	internalNotesTitleColor  = []int{74, 85, 101}
	internalNotesValueColor  = []int{54, 65, 83}
	internalNotesBgColor     = []int{249, 250, 251}
	internalNotesBorderColor = []int{209, 213, 220}
)
