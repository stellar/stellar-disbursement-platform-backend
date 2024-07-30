package paymentdispatchers

import (
	"context"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

type PaymentDispatcherInterface interface {
	DispatchPayments(ctx context.Context, sdpDBTx db.DBTransaction, tenantID string, paymentsToDispatch []*data.Payment) error
	SupportedPlatform() schema.Platform
}
