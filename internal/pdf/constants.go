package pdf

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
	txCellFontSize           = 8.0
	txSmallFontSize          = 6.7
	organizationNameFontSize = 9.2
	dateRangeFontSize        = 10.0
	titleFontSize            = 23.4
)

// Line heights
const (
	summaryHeaderLineHeight  = 4.0
	headerLeftColLineHeight  = 6.0
	headerRightColLineHeight = 5.0
	footerLineHeight         = 4.0
	titleSectionLine1Height  = 6.0
	titleSectionLine2Height  = 9.0
	titleSectionLine3Height  = 6.0
	dateLineHeight           = 4.0
	counterpartyLineHeight   = 4.0
	txIDLineHeight           = 4.0
	txHeaderLineHeight       = 4.0
	amountLineHeight         = 4.0
	currencyLineHeight       = 4.0
)

// Table row heights
const (
	summaryHeaderRowHeight = 15.5
	summaryDataRowHeight   = 14
	txHeaderRowHeight      = 15.5
	txDataRowHeight        = 16
)

// Table cell spacing
const (
	cellPaddingX    = 2.115
	counterpartyGap = 1.0
)

// Section margins
const (
	sectionBottomMargin = 12.0
)

// Header section spacing
const (
	headerBottomMargin              = 5.0
	miniHeaderBottomMargin          = 5.0
	headerSeparatorBottomMargin     = 6.0
	miniHeaderSeparatorBottomMargin = 3.0
	headerSeparatorLineWidth        = 0.26
	headerLogoToOrgNameGap          = 3.0
	logoOffsetX                     = 1.0
	walletAddressLabelGap           = 0.3
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

// Other constants
const (
	walletIDLabel             = "Wallet ID"
	maxCounterpartyTextLength = 35
)

// tableWidth is full content width between left and right margins (both = marginLR).
const tableWidth = mmPerPage - 2*marginLR

// summaryColWidths: Wallet Address | Beginning Balance | Total Credits | Total Debits | Ending Balance (sum = tableWidth)
var summaryColWidths = []float64{44, 34, 34, 34, 34}

// txColWidths: Date | Transaction ID | Counterparty | Debits | Credits | Balance (sum = tableWidth)
var txColWidths = []float64{16, 52, 49, 21, 21, 21}

// Text colors (RGB)
var headerAndTotalsColor = []int{54, 65, 83}   // #364153
var defaultCellColor      = []int{74, 85, 101} // #4A5565
var noteColor             = []int{106, 114, 130} // #6A7282
var activeColor           = []int{20, 71, 230}   // #1447E6
var highlightColor        = []int{16, 24, 40}   // #101828
var sectionTitleColor     = []int{30, 41, 57}   // #1E2939

// Border and background colors (RGB)
var headerBorderColor   = []int{203, 207, 215} // #CBCFD7
var defaultBorderColor = []int{240, 241, 243}  // #F0F1F3
var totalsRowBgColor   = []int{249, 250, 251}   // #F9FAFB
