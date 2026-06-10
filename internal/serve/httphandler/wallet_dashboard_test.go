package httphandler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	svcMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// Test_W4_WalletAwareDashboards proves the W4 dashboard backend: statistics scope to the
// caller's wallets (header narrows; Owners aggregate tenant-wide), and the balance endpoints
// serve per-wallet (membership-scoped) and Owner-only aggregate views.
func Test_W4_WalletAwareDashboards(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	walletA := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)
	var walletBID string
	require.NoError(t, dbConnectionPool.GetContext(ctx, &walletBID, `
		INSERT INTO distribution_wallets (name, distribution_account_type, distribution_account_address)
		VALUES ('dash-wallet-b', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT', 'GDASHWALLETBADDRESSAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA') RETURNING id`))

	memberA := &auth.User{ID: "dash-member-a", Email: "a@dash.test", Roles: []string{string(data.BusinessUserRole)}}
	owner := &auth.User{ID: "dash-owner", Email: "o@dash.test", IsOwner: true}
	_, err = models.WalletMemberships.Insert(ctx, dbConnectionPool, memberA.ID, walletA.ID, data.BusinessUserRole, nil)
	require.NoError(t, err)

	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.On("GetUserByID", mock.Anything, memberA.ID).Return(memberA, nil)
	authManagerMock.On("GetUserByID", mock.Anything, owner.ID).Return(owner, nil)

	// One disbursement + one DRAFT payment per wallet (amount 10 from A, 25 from B).
	mkPayment := func(name, walletID, amount string) {
		d := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name: name, SourceWalletID: walletID, Status: data.StartedDisbursementStatus,
		})
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, d.Wallet.ID, data.ReadyReceiversWalletStatus)
		data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rw, Disbursement: d, Asset: *d.Asset, Amount: amount, Status: data.DraftPaymentStatus,
		})
	}
	mkPayment("dash-disb-a", walletA.ID, "10")
	mkPayment("dash-disb-b", walletBID, "25")

	statsHandler := StatisticsHandler{DBConnectionPool: dbConnectionPool, Models: models, AuthManager: authManagerMock}

	statsAs := func(userID, headerWalletID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/statistics", nil)
		if headerWalletID != "" {
			req.Header.Set(XWalletIDHeader, headerWalletID)
		}
		req = req.WithContext(sdpcontext.SetUserIDInContext(ctx, userID))
		rr := httptest.NewRecorder()
		statsHandler.GetStatistics(rr, req)
		return rr
	}

	t.Run("statistics: member sees only their wallets; owner sees tenant-wide", func(t *testing.T) {
		rr := statsAs(memberA.ID, "")
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"total_disbursements": 1`)

		rr = statsAs(owner.ID, "")
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"total_disbursements": 2`)
	})

	t.Run("statistics: header narrows; out-of-scope header → 404 no-disclosure", func(t *testing.T) {
		rr := statsAs(owner.ID, walletBID)
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"total_disbursements": 1`)

		rr = statsAs(memberA.ID, walletBID)
		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.NotContains(t, rr.Body.String(), "dash-wallet-b")
	})

	// ===== Balances =====
	// Give wallet A an on-chain address (the ensured default has none — pending path).
	_, err = models.DistributionWallets.UpdateAddress(ctx, dbConnectionPool, walletA.ID, "GDASHWALLETAADDRESSAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	require.NoError(t, err)

	mDistAccSvc := &svcMocks.MockDistributionAccountService{}
	usdc := data.Asset{Code: "USDC", Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"}
	mDistAccSvc.On("GetBalances", mock.Anything, mock.Anything).
		Return(map[data.Asset]decimal.Decimal{usdc: decimal.NewFromInt(40)}, nil)

	walletsHandler := DistributionWalletsHandler{
		Service:                    &mockDistributionWalletService{getFn: func(_ context.Context, id string) (*data.DistributionWallet, error) { return models.DistributionWallets.Get(ctx, dbConnectionPool, id) }, listFn: func(_ context.Context, includeArchived bool) ([]data.DistributionWallet, error) { return models.DistributionWallets.GetAll(ctx, dbConnectionPool, includeArchived) }},
		AuthManager:                authManagerMock,
		Models:                     models,
		DistributionAccountService: mDistAccSvc,
	}
	r := chi.NewRouter()
	r.Get("/distribution-wallets/balance", walletsHandler.GetDistributionWalletsTotalBalance)
	r.Get("/distribution-wallets/{id}/balance", walletsHandler.GetDistributionWalletBalance)

	balanceAs := func(userID, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(sdpcontext.SetUserIDInContext(ctx, userID))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	t.Run("per-wallet balance: member-scoped, 404 outside scope", func(t *testing.T) {
		rr := balanceAs(memberA.ID, fmt.Sprintf("/distribution-wallets/%s/balance", walletA.ID))
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"USDC:GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5": "40"`)

		rr = balanceAs(memberA.ID, fmt.Sprintf("/distribution-wallets/%s/balance", walletBID))
		assert.Equal(t, http.StatusNotFound, rr.Code, "no existence disclosure outside scope")
	})

	t.Run("aggregate balance: Owner-only, sums across wallets", func(t *testing.T) {
		rr := balanceAs(owner.ID, "/distribution-wallets/balance")
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"USDC:GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5": "80"`, "40 + 40 across two wallets")

		rr = balanceAs(memberA.ID, "/distribution-wallets/balance")
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})
}
