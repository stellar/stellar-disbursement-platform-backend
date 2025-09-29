package paymentdispatchers

import (
	"context"
	"fmt"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type MemoResolverInterface interface {
	GetMemo(ctx context.Context, receiverWallet data.ReceiverWallet) (schema.Memo, error)
}

type MemoResolver struct {
	Organizations *data.OrganizationModel
}

func (m *MemoResolver) GetMemo(ctx context.Context, receiverWallet data.ReceiverWallet) (schema.Memo, error) {
	if receiverWallet.StellarMemo != "" {
		memoValue := receiverWallet.StellarMemo
		memoType := receiverWallet.StellarMemoType
		if memoType == "" {
			memoType = schema.MemoTypeID
		}
		return schema.Memo{Value: memoValue, Type: memoType}, nil
	}

	org, err := m.Organizations.Get(ctx)
	if err != nil {
		return schema.Memo{}, fmt.Errorf("getting organization: %w", err)
	}

	if !org.IsMemoTracingEnabled {
		return schema.Memo{}, nil
	}

	tenantMemo, err := tenant.GenerateMemoForTenant(ctx)
	if err != nil {
		return schema.Memo{}, fmt.Errorf("generating tenant memo: %w", err)
	}
	return tenantMemo, nil
}

var _ MemoResolverInterface = (*MemoResolver)(nil)
