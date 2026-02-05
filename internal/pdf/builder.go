package pdf

import (
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/pdf/statement"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

// BuildPDF generates a multi-page PDF from a StatementResult and returns the bytes.
func BuildPDF(result *services.StatementResult, fromDate, toDate time.Time, organizationName string, organizationLogo []byte) ([]byte, error) {
	return statement.BuildPDF(result, fromDate, toDate, organizationName, organizationLogo)
}
