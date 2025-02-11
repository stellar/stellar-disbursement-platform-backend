package paymentdispatchers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

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
		return schema.Memo{
			Value: receiverWallet.StellarMemo,
			Type:  schema.MemoTypeID,
		}, nil
	}

	org, err := m.Organizations.Get(ctx)
	if err != nil {
		return schema.Memo{}, fmt.Errorf("getting organization: %w", err)
	}

	if !org.IsMemoTracingEnabled {
		return schema.Memo{}, nil
	}

	tnt, err := tenant.GetTenantFromContext(ctx)
	if err != nil {
		return schema.Memo{}, fmt.Errorf("getting tenant: %w", err)
	}

	baseURLMemo := GenerateHashFromBaseURL(*tnt.BaseURL)
	return schema.Memo{
		Value: baseURLMemo,
		Type:  schema.MemoTypeText,
	}, nil
}

var _ MemoResolverInterface = (*MemoResolver)(nil)

// GenerateHashFromBaseURL generates a hash from the base URL and returns the first 12 hex chars (6 bytes) prefixed with
// "sdp-". This is used to create a unique memo for each tenant that fits within the Stellar memo limit of 28 bytes for
// a `MEMO_TEXT`.
func GenerateHashFromBaseURL(baseURL string) string {
	// Trim any whitespace
	baseURL = strings.TrimSpace(baseURL)
	u, err := url.Parse(baseURL)
	if err == nil {
		// Remove trailing slash if it's the only path component
		u.Path = strings.TrimRight(u.Path, "/")
		baseURL = u.String()
	}

	hash := sha256.Sum256([]byte(baseURL))
	return "sdp-" + hex.EncodeToString(hash[:])[:12]
}
