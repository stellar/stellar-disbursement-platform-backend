package paymentengines

import (
	"context"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

type PaymentEngineInterface interface {
	SubmitPayments(ctx context.Context, sdpDBTx db.DBTransaction, tenantID string, paymentsToSubmit []*data.Payment) error
}
