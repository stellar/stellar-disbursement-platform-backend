package httphandler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/pdf/transaction"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

const (
	testQueryParams = "?asset_code=XLM&from_date=2026-01-01&to_date=2026-01-31"
	emptyBalance    = "0.0000000"
)

type mockReportsService struct {
	mock.Mock
}

func (m *mockReportsService) GetStatement(ctx context.Context, account *schema.TransactionAccount, walletID, assetCode string, fromDate, toDate time.Time) (*services.StatementResult, error) {
	args := m.Called(ctx, account, walletID, assetCode, fromDate, toDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*services.StatementResult), args.Error(1)
}

func TestReportsHandlerGetStatementExport(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()
	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	// Three DB_VAULT accounts: the default (provisioned), a second live one, and an archived one.
	// A fourth is still PENDING, so it has no address to read.
	addrA, addrB, addrOld := keypair.MustRandom().Address(), keypair.MustRandom().Address(), keypair.MustRandom().Address()
	walletA := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)
	_, err = dbConnectionPool.ExecContext(ctx, `UPDATE distribution_wallets SET distribution_account_address = $1 WHERE id = $2`, addrA, walletA.ID)
	require.NoError(t, err)
	insertWallet := func(name, addr, status string) string {
		var id string
		require.NoError(t, dbConnectionPool.GetContext(ctx, &id, `
			INSERT INTO distribution_wallets (name, distribution_account_type, distribution_account_address, status, archived_at)
			VALUES ($1, 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT', NULLIF($2, ''), $3::text, CASE WHEN $3::text = 'ARCHIVED' THEN NOW() END)
			RETURNING id`, name, addr, status))
		return id
	}
	walletBID := insertWallet("Field office B", addrB, "ACTIVE")
	walletOldID := insertWallet("Old account", addrOld, "ARCHIVED")
	walletPendingID := insertWallet("Not yet funded", "", "PENDING")

	memberA := &auth.User{ID: "stmt-member-a", Email: "a@stmt.test", Roles: []string{string(data.FinancialControllerUserRole)}}
	owner := &auth.User{ID: "stmt-owner", Email: "o@stmt.test", IsOwner: true, Roles: []string{string(data.OwnerUserRole)}}
	_, err = models.WalletMemberships.Insert(ctx, dbConnectionPool, memberA.ID, walletA.ID, data.FinancialControllerUserRole, nil)
	require.NoError(t, err)
	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.On("GetUserByID", mock.Anything, memberA.ID).Return(memberA, nil)
	authManagerMock.On("GetUserByID", mock.Anything, owner.ID).Return(owner, nil)

	const query = "/reports/statement?from_date=2026-01-01&to_date=2026-01-31"
	statementFor := func(addr string) *services.StatementResult {
		return &services.StatementResult{Summary: services.StatementSummary{
			Account: "stellar:" + addr,
			Assets:  []services.StatementAssetSummary{{Code: "XLM", BeginningBalance: emptyBalance, TotalCredits: emptyBalance, TotalDebits: emptyBalance, EndingBalance: emptyBalance}},
		}}
	}
	accountWith := func(addr string) interface{} {
		return mock.MatchedBy(func(a *schema.TransactionAccount) bool { return a.Address == addr })
	}

	type call struct {
		userID, walletHeader, query string
	}
	do := func(svc *mockReportsService, c call) *httptest.ResponseRecorder {
		h := ReportsHandler{ReportsService: svc, Models: models, DBConnectionPool: dbConnectionPool, AuthManager: authManagerMock}
		r := chi.NewRouter()
		r.Get("/reports/statement", h.GetStatementExport)
		req := httptest.NewRequest(http.MethodGet, c.query, nil).WithContext(sdpcontext.SetUserIDInContext(ctx, c.userID))
		if c.walletHeader != "" {
			req.Header.Set(XWalletIDHeader, c.walletHeader)
		}
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	t.Run("validates the date range before touching any account", func(t *testing.T) {
		for _, tc := range []struct{ query, missing string }{
			{"/reports/statement?to_date=2026-01-31", "from_date"},
			{"/reports/statement?from_date=2026-01-01", "to_date"},
		} {
			rr := do(&mockReportsService{}, call{userID: owner.ID, query: tc.query})
			assert.Equal(t, http.StatusBadRequest, rr.Code)
			assert.Contains(t, rr.Body.String(), tc.missing)
		}
	})

	t.Run("owner: statement for the account named by X-Wallet-Id", func(t *testing.T) {
		svc := &mockReportsService{}
		svc.On("GetStatement", mock.Anything, accountWith(addrB), walletBID, "", mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time")).
			Return(statementFor(addrB), nil).Once()

		rr := do(svc, call{userID: owner.ID, walletHeader: walletBID, query: query})
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.Equal(t, "application/pdf", rr.Header().Get("Content-Type"))
		assert.Equal(t, "attachment; filename=statement_field-office-b_20260101-20260131.pdf", rr.Header().Get("Content-Disposition"))
		svc.AssertExpectations(t)
	})

	t.Run("owner: several visible accounts and no header is a 400, never a silent default", func(t *testing.T) {
		rr := do(&mockReportsService{}, call{userID: owner.ID, query: query})
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "X-Wallet-Id")
	})

	t.Run("owner: archived accounts stay reportable and are marked as such", func(t *testing.T) {
		svc := &mockReportsService{}
		result := statementFor(addrOld)
		svc.On("GetStatement", mock.Anything, accountWith(addrOld), walletOldID, "", mock.Anything, mock.Anything).Return(result, nil).Once()

		rr := do(svc, call{userID: owner.ID, walletHeader: walletOldID, query: query})
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.Equal(t, "Old account (archived)", result.Summary.AccountName)
	})

	t.Run("owner: a pending account has nothing to read yet", func(t *testing.T) {
		rr := do(&mockReportsService{}, call{userID: owner.ID, walletHeader: walletPendingID, query: query})
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "not been provisioned")
	})

	t.Run("owner: unknown account id is a 404", func(t *testing.T) {
		rr := do(&mockReportsService{}, call{userID: owner.ID, walletHeader: "no-such-wallet", query: query})
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("member: only accounts they hold a membership on, 404 elsewhere", func(t *testing.T) {
		rr := do(&mockReportsService{}, call{userID: memberA.ID, walletHeader: walletBID, query: query})
		assert.Equal(t, http.StatusNotFound, rr.Code)

		svc := &mockReportsService{}
		svc.On("GetStatement", mock.Anything, accountWith(addrA), walletA.ID, "", mock.Anything, mock.Anything).Return(statementFor(addrA), nil).Once()
		rr = do(svc, call{userID: memberA.ID, walletHeader: walletA.ID, query: query})
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	})

	t.Run("member: a single visible account needs no header", func(t *testing.T) {
		svc := &mockReportsService{}
		svc.On("GetStatement", mock.Anything, accountWith(addrA), walletA.ID, "", mock.Anything, mock.Anything).Return(statementFor(addrA), nil).Once()
		rr := do(svc, call{userID: memberA.ID, query: query})
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		svc.AssertExpectations(t)
	})

	t.Run("service errors map to 404 for a missing asset and 500 otherwise", func(t *testing.T) {
		for _, tc := range []struct {
			err    error
			status int
		}{
			{services.ErrStatementAssetNotFound, http.StatusNotFound},
			{errors.New("horizon down"), http.StatusInternalServerError},
		} {
			svc := &mockReportsService{}
			svc.On("GetStatement", mock.Anything, mock.Anything, walletBID, "", mock.Anything, mock.Anything).Return(nil, tc.err).Once()
			rr := do(svc, call{userID: owner.ID, walletHeader: walletBID, query: query})
			assert.Equal(t, tc.status, rr.Code)
		}
	})
}

func Test_statementAccount(t *testing.T) {
	addr := keypair.MustRandom().Address()
	testCases := []struct {
		name        string
		wallet      data.DistributionWallet
		expectedErr string
	}{
		{"shared host account", data.DistributionWallet{AccountType: schema.DistributionAccountStellarEnv, Address: &addr}, errStatementNotForSharedAccount},
		{"circle account", data.DistributionWallet{AccountType: schema.DistributionAccountCircleDBVault}, errStatementOnlySupportedForStellar},
		{"pending account", data.DistributionWallet{AccountType: schema.DistributionAccountStellarDBVault}, errStatementAccountNotProvisioned},
		{"provisioned DB vault account", data.DistributionWallet{AccountType: schema.DistributionAccountStellarDBVault, Address: &addr, AccountStatus: schema.AccountStatusActive}, ""},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			account, httpErr := statementAccount(&tc.wallet)
			if tc.expectedErr != "" {
				require.NotNil(t, httpErr)
				assert.Equal(t, http.StatusBadRequest, httpErr.StatusCode)
				assert.Equal(t, tc.expectedErr, httpErr.Message)
				return
			}
			require.Nil(t, httpErr)
			assert.Equal(t, schema.TransactionAccount{Address: addr, Type: schema.DistributionAccountStellarDBVault, Status: schema.AccountStatusActive}, account)
		})
	}
}

func TestReportsHandlerGetPaymentExport(t *testing.T) {
	ctx := context.Background()
	dbPool := testutils.GetDBConnectionPool(t)
	models, err := data.NewModels(dbPool)
	require.NoError(t, err)

	// Two accounts; memberA may read only the first. Payments inherit their disbursement's account.
	walletA := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbPool)
	addrA := keypair.MustRandom().Address()
	_, err = dbPool.ExecContext(ctx, `UPDATE distribution_wallets SET distribution_account_address = $1 WHERE id = $2`, addrA, walletA.ID)
	require.NoError(t, err)
	var walletBID string
	require.NoError(t, dbPool.GetContext(ctx, &walletBID, `
		INSERT INTO distribution_wallets (name, distribution_account_type)
		VALUES ('Field office B', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))

	memberA := &auth.User{ID: "notice-member-a", Email: "a@notice.test", Roles: []string{string(data.BusinessUserRole)}}
	owner := &auth.User{ID: "notice-owner", Email: "o@notice.test", IsOwner: true, Roles: []string{string(data.OwnerUserRole)}}
	_, err = models.WalletMemberships.Insert(ctx, dbPool, memberA.ID, walletA.ID, data.BusinessUserRole, nil)
	require.NoError(t, err)
	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.On("GetUserByID", mock.Anything, memberA.ID).Return(memberA, nil)
	authManagerMock.On("GetUserByID", mock.Anything, owner.ID).Return(owner, nil)

	receiver := data.CreateReceiverFixture(t, ctx, dbPool, &data.Receiver{ExternalID: "RCV-77"})
	wallet := data.CreateWalletFixture(t, ctx, dbPool, "w", "https://w.com", "w.com", "w://")
	rw := data.CreateReceiverWalletFixture(t, ctx, dbPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)
	paymentFrom := func(walletID, name string) *data.Payment {
		d := data.CreateDisbursementFixture(t, ctx, dbPool, models.Disbursements, &data.Disbursement{Name: name, SourceWalletID: walletID})
		return data.CreatePaymentFixture(t, ctx, dbPool, models.Payment, &data.Payment{
			ReceiverWallet: rw, Disbursement: d, Asset: *d.Asset, Amount: "100.0000000", Status: data.DraftPaymentStatus,
		})
	}
	paymentA := paymentFrom(walletA.ID, "notice-disb-a")
	paymentB := paymentFrom(walletBID, "notice-disb-b")

	h := ReportsHandler{Models: models, DBConnectionPool: dbPool, AuthManager: authManagerMock}
	r := chi.NewRouter()
	r.Get("/reports/payment/{id}", h.GetPaymentExport)
	doAs := func(userID, paymentID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/reports/payment/"+paymentID, nil).WithContext(sdpcontext.SetUserIDInContext(ctx, userID))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	t.Run("unknown payment is a 404", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, doAs(owner.ID, "nonexistent-id").Code)
	})

	t.Run("a payment outside the caller's accounts is a 404, inside it a PDF", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, doAs(memberA.ID, paymentB.ID).Code)

		rr := doAs(memberA.ID, paymentA.ID)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.Equal(t, "application/pdf", rr.Header().Get("Content-Type"))
		assert.Contains(t, rr.Header().Get("Content-Disposition"), "transaction_notice_"+paymentA.ID)
		assert.NotEmpty(t, rr.Body.Bytes())

		assert.Equal(t, http.StatusOK, doAs(owner.ID, paymentB.ID).Code)
	})

	t.Run("a Circle payment still renders", func(t *testing.T) {
		data.CreateCircleTransferRequestFixture(t, ctx, dbPool, data.CircleTransferRequest{
			PaymentID: paymentA.ID, CirclePayoutID: utils.Ptr("payout-1"), Status: utils.Ptr(data.CircleTransferStatusSuccess),
		})
		assert.Equal(t, http.StatusOK, doAs(owner.ID, paymentA.ID).Code)
	})
}

func Test_populateDisbursementCreatedApprovedBy(t *testing.T) {
	ctx := context.Background()
	draftTime := time.Date(2026, 1, 9, 10, 0, 0, 0, time.UTC)
	readyTime := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	startedTime := time.Date(2026, 1, 10, 14, 30, 0, 0, time.UTC)

	userA := "user-draft-id"
	userB := "user-ready-id"
	userC := "user-started-id"

	history := data.DisbursementStatusHistory{
		{UserID: userA, Status: data.DraftDisbursementStatus, Timestamp: draftTime},
		{UserID: userB, Status: data.ReadyDisbursementStatus, Timestamp: readyTime},
		{UserID: userC, Status: data.StartedDisbursementStatus, Timestamp: startedTime},
	}

	authManagerMock := auth.NewAuthManagerMock(t)
	authManagerMock.
		On("GetUsersByID", ctx, mock.MatchedBy(func(ids []string) bool {
			return len(ids) == 2 && (ids[0] == userA || ids[1] == userA) && (ids[0] == userC || ids[1] == userC)
		}), false).
		Return([]*auth.User{
			{ID: userA, FirstName: "Alice", LastName: "Creator", Email: "alice@test.com"},
			{ID: userC, FirstName: "Bob", LastName: "Starter", Email: "bob@test.com"},
		}, nil).
		Once()

	enrichment := &transaction.Enrichment{}
	populateDisbursementCreatedApprovedBy(ctx, authManagerMock, history, enrichment)

	assert.Equal(t, "Alice Creator", enrichment.DisbursementCreatedByUserName)
	assert.Equal(t, "Jan 9, 2026 · 10:00:00 UTC", enrichment.DisbursementCreatedByTimestamp)
	assert.Equal(t, "Bob Starter", enrichment.DisbursementApprovedByUserName)
	assert.Equal(t, "Jan 10, 2026 · 14:30:00 UTC", enrichment.DisbursementApprovedByTimestamp)
}

func Test_operatedBy(t *testing.T) {
	ctx := context.Background()
	assert.Empty(t, operatedBy(ctx), "no tenant in context")

	withURL := func(u *string) context.Context {
		return sdpcontext.SetTenantInContext(ctx, &schema.Tenant{ID: "t1", Name: "redcorp", SDPUIBaseURL: u})
	}
	assert.Empty(t, operatedBy(withURL(nil)), "tenant without a dashboard URL")
	assert.Equal(t, "redcorp.stellar.local:3000", operatedBy(withURL(utils.Ptr("https://redcorp.stellar.local:3000/some/path"))))
}
