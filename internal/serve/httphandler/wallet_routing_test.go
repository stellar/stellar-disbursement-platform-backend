package httphandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// Test_W3_SourceWalletRouting proves the routing rule at the resolver level:
// explicit X-Wallet-Id honored after entitlement; 400 when omitted on multi-wallet tenants;
// single-wallet tenants legitimately omit it; 403 without wallet-existence disclosure;
// archived wallets rejected with 400 only after entitlement.
func Test_W3_SourceWalletRouting(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	walletA := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)

	owner := &auth.User{ID: "w3-owner", Email: "o@w3.test", IsOwner: true}
	initiatorOnA := &auth.User{ID: "w3-initiator-a", Email: "a@w3.test", Roles: []string{string(data.InitiatorUserRole)}}
	_, err = models.WalletMemberships.Insert(ctx, dbConnectionPool, initiatorOnA.ID, walletA.ID, data.InitiatorUserRole, nil)
	require.NoError(t, err)

	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.On("GetUserByID", mock.Anything, owner.ID).Return(owner, nil)
	authManagerMock.On("GetUserByID", mock.Anything, initiatorOnA.ID).Return(initiatorOnA, nil)

	resolve := func(userID, headerWalletID string) (*data.DistributionWallet, int, string) {
		req := httptest.NewRequest(http.MethodPost, "/disbursements", strings.NewReader("{}"))
		if headerWalletID != "" {
			req.Header.Set(XWalletIDHeader, headerWalletID)
		}
		reqCtx := sdpcontext.SetUserIDInContext(ctx, userID)
		wallet, httpErr := resolveSourceWalletForWrite(reqCtx, req, authManagerMock, models,
			data.FinancialControllerUserRole, data.InitiatorUserRole)
		if httpErr != nil {
			return nil, httpErr.StatusCode, httpErr.Message
		}
		return wallet, http.StatusOK, ""
	}

	t.Run("single-wallet tenant legitimately omits the header (pre-opt-in default semantics)", func(t *testing.T) {
		wallet, code, _ := resolve(initiatorOnA.ID, "")
		require.Equal(t, http.StatusOK, code)
		assert.Equal(t, walletA.ID, wallet.ID)
	})

	t.Run("explicit header is honored", func(t *testing.T) {
		wallet, code, _ := resolve(initiatorOnA.ID, walletA.ID)
		require.Equal(t, http.StatusOK, code)
		assert.Equal(t, walletA.ID, wallet.ID)
	})

	// Opt in: a second wallet appears.
	var walletBID string
	require.NoError(t, dbConnectionPool.GetContext(ctx, &walletBID, `
		INSERT INTO distribution_wallets (name, distribution_account_type)
		VALUES ('w3-wallet-b', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))

	t.Run("multi-wallet tenant: omitted header → 400 (no silent routing)", func(t *testing.T) {
		_, code, msg := resolve(initiatorOnA.ID, "")
		assert.Equal(t, http.StatusBadRequest, code)
		assert.Contains(t, msg, "X-Wallet-Id")

		_, code, _ = resolve(owner.ID, "")
		assert.Equal(t, http.StatusBadRequest, code, "owners must be explicit too — no auto-fallback misroutes funds")
	})

	t.Run("unentitled wallet → 403 without existence disclosure", func(t *testing.T) {
		_, code, msg := resolve(initiatorOnA.ID, walletBID)
		assert.Equal(t, http.StatusForbidden, code)
		assert.NotContains(t, msg, walletBID)
		assert.NotContains(t, msg, "w3-wallet-b")

		// Unknown wallet id gives the IDENTICAL 403 to a non-owner.
		_, codeUnknown, msgUnknown := resolve(initiatorOnA.ID, "22222222-2222-2222-2222-222222222222")
		assert.Equal(t, http.StatusForbidden, codeUnknown)
		assert.Equal(t, msg, msgUnknown, "unknown and unentitled must be indistinguishable")
	})

	t.Run("owner gets honest 404 for unknown wallet", func(t *testing.T) {
		_, code, _ := resolve(owner.ID, "22222222-2222-2222-2222-222222222222")
		assert.Equal(t, http.StatusNotFound, code)
	})

	t.Run("archived wallet → 400, only after entitlement", func(t *testing.T) {
		// Archive wallet B (A stays active; B is not default).
		_, aErr := models.DistributionWallets.Archive(ctx, dbConnectionPool, walletBID)
		require.NoError(t, aErr)

		// Entitled (owner) → 400 archived.
		_, code, msg := resolve(owner.ID, walletBID)
		assert.Equal(t, http.StatusBadRequest, code)
		assert.Contains(t, msg, "archived")

		// Unentitled member still sees 403, not the archived detail.
		_, code, msg = resolve(initiatorOnA.ID, walletBID)
		assert.Equal(t, http.StatusForbidden, code)
		assert.NotContains(t, msg, "archived")
	})

	t.Run("payments inherit and direct payments require the wallet at the DB layer", func(t *testing.T) {
		disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name: "w3-routing-disb", SourceWalletID: walletA.ID,
		})
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, disbursement.Wallet.ID, data.ReadyReceiversWalletStatus)

		// Disbursement payment with NO explicit wallet: the DB derives it from the parent.
		payment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: receiverWallet, Disbursement: disbursement, Asset: *disbursement.Asset,
			Amount: "5", Status: data.DraftPaymentStatus,
		})
		got, gErr := models.Payment.Get(ctx, payment.ID, dbConnectionPool)
		require.NoError(t, gErr)
		assert.Equal(t, walletA.ID, got.SourceWalletID, "disbursement payments inherit their parent's wallet")

		// Direct payment without a wallet is rejected by the DB.
		_, iErr := dbConnectionPool.ExecContext(ctx, `
			INSERT INTO payments (amount, asset_id, receiver_id, receiver_wallet_id, type)
			VALUES ('5', $1, $2, $3, 'DIRECT')`, disbursement.Asset.ID, receiver.ID, receiverWallet.ID)
		require.Error(t, iErr)
		assert.ErrorContains(t, iErr, "explicitly")

		// Immutability: changing source_wallet_id after creation is rejected on both tables.
		_, uErr := dbConnectionPool.ExecContext(ctx,
			`UPDATE payments SET source_wallet_id = $1 WHERE id = $2`, walletBID, payment.ID)
		require.Error(t, uErr)
		assert.ErrorContains(t, uErr, "immutable")
		_, uErr = dbConnectionPool.ExecContext(ctx,
			`UPDATE disbursements SET source_wallet_id = $1 WHERE id = $2`, walletBID, disbursement.ID)
		require.Error(t, uErr)
		assert.ErrorContains(t, uErr, "immutable")
	})
}
