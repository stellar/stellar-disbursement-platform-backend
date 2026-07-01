package httphandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// Test_ReadEndpointTaxonomy_Conformance is the executable form of
// the read-endpoint taxonomy: every classified read endpoint must behave per its
// bucket. MEMBERSHIP-FILTERED endpoints return different data to a single-wallet member vs an
// Owner; TENANT-SCOPED endpoints return identical cross-wallet data to both, gated by tenant
// role alone. A bucket violation fails this suite.
func Test_ReadEndpointTaxonomy_Conformance(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	// Fixtures: two wallets, member of A only, owner; one disbursement per wallet.
	walletA := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)
	var walletBID string
	require.NoError(t, dbConnectionPool.GetContext(ctx, &walletBID, `
		INSERT INTO distribution_wallets (name, distribution_account_type)
		VALUES ('taxonomy-wallet-b', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))

	memberA := &auth.User{ID: "tax-member-a", Email: "a@tax.test", Roles: []string{string(data.BusinessUserRole)}}
	owner := &auth.User{ID: "tax-owner", Email: "o@tax.test", IsOwner: true}
	_, err = models.WalletMemberships.Insert(ctx, dbConnectionPool, memberA.ID, walletA.ID, data.BusinessUserRole, nil)
	require.NoError(t, err)

	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.On("GetUserByID", mock.Anything, memberA.ID).Return(memberA, nil)
	authManagerMock.On("GetUserByID", mock.Anything, owner.ID).Return(owner, nil)
	authManagerMock.On("GetUsersByID", mock.Anything, mock.Anything, mock.Anything).Return([]*auth.User{}, nil).Maybe()

	data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name: "tax-disb-a", SourceWalletID: walletA.ID,
	})
	data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name: "tax-disb-b", SourceWalletID: walletBID,
	})

	disbursementHandler := DisbursementHandler{
		Models:      models,
		AuthManager: authManagerMock,
		DisbursementManagementService: &services.DisbursementManagementService{
			Models: models, AuthManager: authManagerMock,
		},
	}
	walletsHandler := WalletsHandler{Models: models, WalletAssetResolver: services.NewWalletAssetResolver(models.Assets)}
	assetsHandler := AssetsHandler{Models: models}

	serveAs := func(userID string, h http.HandlerFunc, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(sdpcontext.SetUserIDInContext(req.Context(), userID))
		rr := httptest.NewRecorder()
		h(rr, req)
		return rr
	}

	// ===== Bucket 1: MEMBERSHIP-FILTERED — member and owner MUST see different data =====
	t.Run("membership-filtered: GET /disbursements differs by membership", func(t *testing.T) {
		memberBody := serveAs(memberA.ID, disbursementHandler.GetDisbursements, "/disbursements").Body.String()
		ownerBody := serveAs(owner.ID, disbursementHandler.GetDisbursements, "/disbursements").Body.String()

		assert.Contains(t, memberBody, "tax-disb-a")
		assert.NotContains(t, memberBody, "tax-disb-b", "BUCKET VIOLATION: membership-filtered endpoint leaked another wallet's rows")
		assert.Contains(t, ownerBody, "tax-disb-a")
		assert.Contains(t, ownerBody, "tax-disb-b")
	})

	// ===== Bucket 2: TENANT-SCOPED — member and owner MUST see identical data =====
	t.Run("tenant-scoped: GET /assets identical for member and owner", func(t *testing.T) {
		memberBody := serveAs(memberA.ID, assetsHandler.GetAssets, "/assets").Body.String()
		ownerBody := serveAs(owner.ID, assetsHandler.GetAssets, "/assets").Body.String()
		require.NotEmpty(t, memberBody)
		assert.JSONEq(t, ownerBody, memberBody,
			"BUCKET VIOLATION: tenant-scoped endpoint returned membership-dependent data")
	})

	t.Run("tenant-scoped: GET /wallets (recipient providers) identical for member and owner", func(t *testing.T) {
		memberBody := serveAs(memberA.ID, walletsHandler.GetWallets, "/wallets").Body.String()
		ownerBody := serveAs(owner.ID, walletsHandler.GetWallets, "/wallets").Body.String()
		require.NotEmpty(t, memberBody)
		assert.JSONEq(t, ownerBody, memberBody,
			"BUCKET VIOLATION: tenant-scoped endpoint returned membership-dependent data")
	})
}
