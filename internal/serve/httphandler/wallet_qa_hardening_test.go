package httphandler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
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

// Test_WalletQA_AdversarialInputs hardens the wallet surfaces against hostile or malformed
// identifiers: SQL-injection-shaped, oversized, control-character, and unicode inputs must
// produce taxonomy-correct 4xx responses — never 5xx, never echoing the payload back, never
// disclosing existence.
func Test_WalletQA_AdversarialInputs(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	wallet := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)

	member := &auth.User{ID: "qa-member", Email: "m@qa.test", Roles: []string{string(data.BusinessUserRole)}}
	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.On("GetUserByID", mock.Anything, member.ID).Return(member, nil)
	_, err = models.WalletMemberships.Insert(ctx, dbConnectionPool, member.ID, wallet.ID, data.BusinessUserRole, nil)
	require.NoError(t, err)

	statsHandler := StatisticsHandler{DBConnectionPool: dbConnectionPool, Models: models, AuthManager: authManagerMock}
	walletsHandler := DistributionWalletsHandler{
		Service: &mockDistributionWalletService{getFn: func(_ context.Context, id string) (*data.DistributionWallet, error) {
			return models.DistributionWallets.Get(ctx, dbConnectionPool, id)
		}},
		AuthManager: authManagerMock,
		Models:      models,
	}
	r := chi.NewRouter()
	r.Get("/statistics", statsHandler.GetStatistics)
	r.Get("/distribution-wallets/{id}/balance", walletsHandler.GetDistributionWalletBalance)

	hostileIDs := []struct {
		name  string
		value string
	}{
		{"sql injection", `'; DROP TABLE distribution_wallets;--`},
		{"oversized", strings.Repeat("A", 5000)},
		{"control chars", "abc\x01\x02def"},
		{"unicode", "💸-кошелёк-ウォレット"},
		{"valid uuid, nonexistent", "be4b6e3e-58f9-4d65-bd6f-2e9c8b6e7a01"},
		{"whitespace", "   "},
	}

	for _, hostile := range hostileIDs {
		t.Run("X-Wallet-Id header: "+hostile.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/statistics", nil)
			req.Header.Set(XWalletIDHeader, hostile.value)
			req = req.WithContext(sdpcontext.SetUserIDInContext(ctx, member.ID))
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			assert.Less(t, rr.Code, 500, "hostile input must never 5xx")
			assert.Equal(t, http.StatusNotFound, rr.Code, "outside-scope header is 404 regardless of shape")
			if len(hostile.value) >= 8 { // skip the trivially-contained whitespace case
				assert.NotContains(t, rr.Body.String(), hostile.value, "payload must not be echoed")
			}
		})
	}

	for _, hostile := range hostileIDs {
		if strings.ContainsAny(hostile.value, "\x01\x02 ") {
			continue // not URL-path-representable without encoding; httptest.NewRequest would reject
		}
		t.Run("URL wallet id: "+hostile.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/distribution-wallets/"+strings.ReplaceAll(hostile.value, " ", "%20")+"/balance", nil)
			req = req.WithContext(sdpcontext.SetUserIDInContext(ctx, member.ID))
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			assert.Less(t, rr.Code, 500, "hostile input must never 5xx")
			assert.Equal(t, http.StatusNotFound, rr.Code)
		})
	}

	t.Run("injection attempt left the schema intact", func(t *testing.T) {
		var n int
		require.NoError(t, dbConnectionPool.GetContext(ctx, &n, `SELECT COUNT(*) FROM distribution_wallets`))
		assert.GreaterOrEqual(t, n, 1, "distribution_wallets must still exist and hold the fixture")
	})
}

// Test_WalletQA_PostFilterPaginationAtVolume seeds 3 wallets × 20 disbursements and pages a
// member through their wallet with a small page size: every page must contain only in-scope
// rows, totals must reflect the post-filter count, and the union of pages must be exactly the
// member's 20 — no leakage, no dupes, no gaps at page boundaries.
func Test_WalletQA_PostFilterPaginationAtVolume(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	walletA := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)
	mkWallet := func(name string) string {
		var id string
		require.NoError(t, dbConnectionPool.GetContext(ctx, &id, `
			INSERT INTO distribution_wallets (name, distribution_account_type)
			VALUES ($1, 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`, name))
		return id
	}
	walletB, walletC := mkWallet("qa-vol-b"), mkWallet("qa-vol-c")

	member := &auth.User{ID: "qa-vol-member", Email: "v@qa.test", Roles: []string{string(data.BusinessUserRole)}}
	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.On("GetUserByID", mock.Anything, member.ID).Return(member, nil)
	authManagerMock.On("GetUsersByID", mock.Anything, mock.Anything, mock.Anything).Return([]*auth.User{}, nil).Maybe()
	_, err = models.WalletMemberships.Insert(ctx, dbConnectionPool, member.ID, walletA.ID, data.BusinessUserRole, nil)
	require.NoError(t, err)

	wantIDs := map[string]bool{}
	for i := 0; i < 20; i++ {
		for _, w := range []string{walletA.ID, walletB, walletC} {
			d := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
				Name: fmt.Sprintf("qa-vol-%s-%d", w[:8], i), SourceWalletID: w,
			})
			if w == walletA.ID {
				wantIDs[d.ID] = true
			}
		}
	}
	require.Len(t, wantIDs, 20)

	handler := DisbursementHandler{
		Models:      models,
		AuthManager: authManagerMock,
		DisbursementManagementService: &services.DisbursementManagementService{
			Models: models, AuthManager: authManagerMock,
		},
	}
	r := chi.NewRouter()
	r.Get("/disbursements", handler.GetDisbursements)

	type page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Pagination struct {
			Pages int `json:"pages"`
			Total int `json:"total"`
		} `json:"pagination"`
	}

	gotIDs := map[string]bool{}
	pages := 0
	for pageNum := 1; pageNum <= 10; pageNum++ {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/disbursements?page=%d&page_limit=7", pageNum), nil)
		req = req.WithContext(sdpcontext.SetUserIDInContext(ctx, member.ID))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)

		var p page
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &p))
		assert.Equal(t, 20, p.Pagination.Total, "post-filter total must count only the member's rows")
		assert.Equal(t, 3, p.Pagination.Pages, "ceil(20/7)")
		if len(p.Data) == 0 {
			break
		}
		pages++
		for _, row := range p.Data {
			assert.True(t, wantIDs[row.ID], "row %s leaked from another wallet", row.ID)
			assert.False(t, gotIDs[row.ID], "row %s duplicated across pages", row.ID)
			gotIDs[row.ID] = true
		}
	}

	assert.Equal(t, 3, pages)
	assert.Len(t, gotIDs, 20, "union of pages must be exactly the member's rows — no gaps")
}
