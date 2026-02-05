package pdf

import (
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/pdf/statement"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

// BuildPDF generates a multi-page PDF from a StatementResult and returns the bytes.
func BuildPDF(result *services.StatementResult, fromDate, toDate time.Time, organizationName string, organizationLogo []byte) ([]byte, error) {
	pdfBytes, err := statement.BuildPDF(result, fromDate, toDate, organizationName, organizationLogo)
	if err != nil {
		return nil, fmt.Errorf("building PDF statement: %w", err)
	}
	return pdfBytes, nil
}
