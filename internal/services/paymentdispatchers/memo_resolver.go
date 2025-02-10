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

func GetMemoAndType(receiverWallet data.ReceiverWallet) (string, schema.MemoType) {
	if receiverWallet.StellarMemo != "" {
		return receiverWallet.StellarMemo, schema.MemoTypeID
	}

	return "", ""
	// return "foo-bar", schema.MemoTypeText
}

//go:generate mockery --name=MemoResolverInterface --case=underscore --structname=MockMemoResolver --filename=memo_resolver_mock.go --inpackage
type MemoResolverInterface interface {
	GetMemo(ctx context.Context, receiverWallet data.ReceiverWallet) (*schema.Memo, error)
}

type MemoResolver struct {
	Organizations *data.OrganizationModel
}

func (m *MemoResolver) GetMemo(ctx context.Context, receiverWallet data.ReceiverWallet) (*schema.Memo, error) {
	if receiverWallet.StellarMemo != "" {
		return &schema.Memo{
			Value: receiverWallet.StellarMemo,
			Type:  schema.MemoTypeID,
		}, nil
	}

	org, err := m.Organizations.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting organization: %w", err)
	}

	if !org.IsTenantMemoEnabled {
		return nil, nil
	}

	tnt, err := tenant.GetTenantFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting tenant: %w", err)
	}

	baseURLMemo := generateHashFromBaseURL(*tnt.BaseURL)
	return &schema.Memo{
		Value: baseURLMemo,
		Type:  schema.MemoTypeText,
	}, nil
}

var _ MemoResolverInterface = (*MemoResolver)(nil)

// generateHashFromBaseURL generates a hash from the base URL and returns the first 12 hex chars (6 bytes) prefixed with
// "sdp-". This is used to create a unique memo for each tenant that fits within the Stellar memo limit of 28 bytes for
// a `MEMO_TEXT`.
func generateHashFromBaseURL(baseURL string) string {
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
