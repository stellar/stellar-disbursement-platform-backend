package httphandler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/pdf/transaction"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
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

func (m *mockReportsService) GetStatement(ctx context.Context, account *schema.TransactionAccount, assetCode string, fromDate, toDate time.Time) (*services.StatementResult, error) {
	args := m.Called(ctx, account, assetCode, fromDate, toDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*services.StatementResult), args.Error(1)
}

func TestReportsHandlerGetStatementExport(t *testing.T) {
	stellarAccount := schema.TransactionAccount{
		Address: "GDNRRK5EXMZ4STV7UTO3CW4LSVNY5KYWTM3J7BM5SQNA7KE2RYX55IYV",
		Type:    schema.DistributionAccountStellarEnv,
	}
	successResult := &services.StatementResult{
		Summary: services.StatementSummary{
			Account: "stellar:GDNRRK5EXMZ4STV7UTO3CW4LSVNY5KYWTM3J7BM5SQNA7KE2RYX55IYV",
			Assets: []services.StatementAssetSummary{
				{
					Code:             "XLM",
					BeginningBalance: emptyBalance,
					TotalCredits:     emptyBalance,
					TotalDebits:      emptyBalance,
					EndingBalance:    "9.7998900",
					Transactions:     []services.StatementTransaction{},
				},
			},
		},
	}

	testCases := []struct {
		name               string
		query              string
		prepareMocks       func(*mockReportsService, *sigMocks.MockDistributionAccountResolver)
		expectedStatus     int
		expectedContains   string
		expectPDFOnSuccess bool
	}{
		{
			name:  "returns 200 and PDF when asset_code is omitted (all assets)",
			query: "?from_date=2026-01-01&to_date=2026-01-31",
			prepareMocks: func(mSvc *mockReportsService, mResolver *sigMocks.MockDistributionAccountResolver) {
				mResolver.On("DistributionAccountFromContext", mock.Anything).
					Return(stellarAccount, nil).
					Once()
				mSvc.On("GetStatement", mock.Anything, mock.MatchedBy(func(a *schema.TransactionAccount) bool {
					return a != nil && a.Address == stellarAccount.Address && a.Type == stellarAccount.Type
				}), "",
					time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)).
					Return(successResult, nil).
					Once()
			},
			expectedStatus:     http.StatusOK,
			expectedContains:   "",
			expectPDFOnSuccess: true,
		},
		{
			name:             "returns 400 when from_date is missing",
			query:            "?asset_code=XLM&to_date=2026-01-31",
			prepareMocks:     func(_ *mockReportsService, _ *sigMocks.MockDistributionAccountResolver) {},
			expectedStatus:   http.StatusBadRequest,
			expectedContains: "from_date",
		},
		{
			name:             "returns 400 when to_date is missing",
			query:            "?asset_code=XLM&from_date=2026-01-01",
			prepareMocks:     func(_ *mockReportsService, _ *sigMocks.MockDistributionAccountResolver) {},
			expectedStatus:   http.StatusBadRequest,
			expectedContains: "to_date",
		},
		{
			name:  "returns 500 when distribution account resolver fails",
			query: testQueryParams,
			prepareMocks: func(_ *mockReportsService, mResolver *sigMocks.MockDistributionAccountResolver) {
				mResolver.On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{}, errors.New("resolver error")).
					Once()
			},
			expectedStatus:   http.StatusInternalServerError,
			expectedContains: "Cannot retrieve distribution account",
		},
		{
			name:  "returns 400 when account is not Stellar",
			query: testQueryParams,
			prepareMocks: func(_ *mockReportsService, mResolver *sigMocks.MockDistributionAccountResolver) {
				mResolver.On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{Type: schema.DistributionAccountCircleDBVault}, nil).
					Once()
			},
			expectedStatus:   http.StatusBadRequest,
			expectedContains: "only supported for Stellar",
		},
		{
			name:  "returns 404 when asset not found for account",
			query: testQueryParams,
			prepareMocks: func(mSvc *mockReportsService, mResolver *sigMocks.MockDistributionAccountResolver) {
				mResolver.On("DistributionAccountFromContext", mock.Anything).
					Return(stellarAccount, nil).
					Once()
				mSvc.On("GetStatement", mock.Anything, mock.MatchedBy(func(a *schema.TransactionAccount) bool {
					return a != nil && a.Address == stellarAccount.Address && a.Type == stellarAccount.Type
				}), "XLM",
					time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)).
					Return(nil, services.ErrStatementAssetNotFound).
					Once()
			},
			expectedStatus:   http.StatusNotFound,
			expectedContains: "asset not found",
		},
		{
			name:  "returns 500 when reports service fails with unexpected error",
			query: testQueryParams,
			prepareMocks: func(mSvc *mockReportsService, mResolver *sigMocks.MockDistributionAccountResolver) {
				mResolver.On("DistributionAccountFromContext", mock.Anything).
					Return(stellarAccount, nil).
					Once()
				mSvc.On("GetStatement", mock.Anything, mock.MatchedBy(func(a *schema.TransactionAccount) bool {
					return a != nil && a.Address == stellarAccount.Address && a.Type == stellarAccount.Type
				}), "XLM",
					time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)).
					Return(nil, errors.New("horizon error")).
					Once()
			},
			expectedStatus:   http.StatusInternalServerError,
			expectedContains: "Cannot retrieve statement",
		},
		{
			name:  "returns 200 and PDF on success",
			query: testQueryParams,
			prepareMocks: func(mSvc *mockReportsService, mResolver *sigMocks.MockDistributionAccountResolver) {
				mResolver.On("DistributionAccountFromContext", mock.Anything).
					Return(stellarAccount, nil).
					Once()
				mSvc.On("GetStatement", mock.Anything, mock.MatchedBy(func(a *schema.TransactionAccount) bool {
					return a != nil && a.Address == stellarAccount.Address && a.Type == stellarAccount.Type
				}), "XLM",
					time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)).
					Return(successResult, nil).
					Once()
			},
			expectedStatus:     http.StatusOK,
			expectedContains:   "",
			expectPDFOnSuccess: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mSvc := &mockReportsService{}
			mResolver := sigMocks.NewMockDistributionAccountResolver(t)
			tc.prepareMocks(mSvc, mResolver)

			h := ReportsHandler{
				DistributionAccountResolver: mResolver,
				ReportsService:              mSvc,
				StatementQueryValidator:     validators.NewStatementQueryValidator(),
			}

			rr := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/reports/statement"+tc.query, nil)
			require.NoError(t, err)
			http.HandlerFunc(h.GetStatementExport).ServeHTTP(rr, req)
			resp := rr.Result()
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
			if tc.expectedContains != "" {
				assert.Contains(t, string(body), tc.expectedContains)
			}
			if tc.expectPDFOnSuccess && tc.expectedStatus == http.StatusOK {
				assert.Equal(t, "application/pdf", resp.Header.Get("Content-Type"))
				assert.Contains(t, resp.Header.Get("Content-Disposition"), "attachment")
				assert.NotEmpty(t, body)
			}
		})
	}
}

func TestReportsHandlerGetPaymentExport(t *testing.T) {
	ctx := context.Background()
	dbPool := testutils.GetDBConnectionPool(t)
	models, err := data.NewModels(dbPool)
	require.NoError(t, err)
	mResolver := sigMocks.NewMockDistributionAccountResolver(t)

	t.Run("returns 404 when payment not found", func(t *testing.T) {
		h := ReportsHandler{
			Models:                      models,
			DBConnectionPool:            dbPool,
			DistributionAccountResolver: mResolver,
		}

		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/reports/payment/nonexistent-id", nil)
		require.NoError(t, err)
		r := chi.NewRouter()
		r.Get("/reports/payment/{id}", h.GetPaymentExport)
		r.ServeHTTP(rr, req)
		resp := rr.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("returns 200 and PDF when payment exists", func(t *testing.T) {
		// Create minimal payment fixture so BuildPDF can run
		receiver := data.CreateReceiverFixture(t, ctx, dbPool, &data.Receiver{})
		wallet := data.CreateWalletFixture(t, ctx, dbPool, "w", "https://w.com", "w.com", "w://")
		rw := data.CreateReceiverWalletFixture(t, ctx, dbPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)
		rw.Receiver = *receiver
		disbursement := data.CreateDisbursementFixture(t, ctx, dbPool, models.Disbursements, &data.Disbursement{})
		payment := data.CreatePaymentFixture(t, ctx, dbPool, models.Payment, &data.Payment{
			ReceiverWallet: rw,
			Disbursement:   disbursement,
			Asset:          *disbursement.Asset,
			Amount:         "100.0000000",
			Status:         data.DraftPaymentStatus,
		})

		mResolver.On("DistributionAccountFromContext", mock.Anything).
			Return(schema.TransactionAccount{}, nil).
			Maybe()

		h := ReportsHandler{
			Models:                      models,
			DBConnectionPool:            dbPool,
			DistributionAccountResolver: mResolver,
			HorizonClient:               nil,
			AuthManager:                 auth.NewAuthManagerMock(t),
		}

		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/reports/payment/"+payment.ID, nil)
		require.NoError(t, err)
		r := chi.NewRouter()
		r.Get("/reports/payment/{id}", h.GetPaymentExport)
		r.ServeHTTP(rr, req)
		resp := rr.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/pdf", resp.Header.Get("Content-Type"))
		assert.Contains(t, resp.Header.Get("Content-Disposition"), "transaction_notice_")
		assert.NotEmpty(t, body)
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
