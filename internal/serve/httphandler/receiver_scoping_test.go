package httphandler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// Test_ReceiverScoping_Reads covers the read half of the rule: a receiver is visible from the
// account it was created under or from any account that has paid them, and hidden (404, no
// disclosure) otherwise. Owners see all.
func Test_ReceiverScoping_Reads(t *testing.T) {
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
		INSERT INTO distribution_wallets (name, distribution_account_type)
		VALUES ('recv-wallet-b', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))

	memberA := &auth.User{ID: "recv-member-a", Email: "a@recv.test", Roles: []string{string(data.BusinessUserRole)}}
	owner := &auth.User{ID: "recv-owner", Email: "o@recv.test", IsOwner: true}
	_, err = models.WalletMemberships.Insert(ctx, dbConnectionPool, memberA.ID, walletA.ID, data.BusinessUserRole, nil)
	require.NoError(t, err)

	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.On("GetUserByID", mock.Anything, memberA.ID).Return(memberA, nil)
	authManagerMock.On("GetUserByID", mock.Anything, owner.ID).Return(owner, nil)

	mkReceiverPaidFrom := func(name, walletID string) *data.Receiver {
		d := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name: name, SourceWalletID: walletID, Status: data.StartedDisbursementStatus,
		})
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{SourceWalletID: walletID})
		rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, d.Wallet.ID, data.ReadyReceiversWalletStatus)
		data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rw, Disbursement: d, Asset: *d.Asset, Amount: "3", Status: data.DraftPaymentStatus,
		})
		return receiver
	}
	receiverFromA := mkReceiverPaidFrom("recv-disb-a", walletA.ID)
	receiverFromB := mkReceiverPaidFrom("recv-disb-b", walletBID)

	handler := ReceiverHandler{Models: models, DBConnectionPool: dbConnectionPool, AuthManager: authManagerMock}
	r := chi.NewRouter()
	r.Get("/receivers", handler.GetReceivers)
	r.Get("/receivers/{id}", handler.GetReceiver)

	doAs := func(userID, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(sdpcontext.SetUserIDInContext(ctx, userID))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	t.Run("listing filters to receivers paid from the caller's wallets", func(t *testing.T) {
		rr := doAs(memberA.ID, "/receivers")
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), receiverFromA.ID)
		assert.NotContains(t, rr.Body.String(), receiverFromB.ID)

		rr = doAs(owner.ID, "/receivers")
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), receiverFromA.ID)
		assert.Contains(t, rr.Body.String(), receiverFromB.ID)
	})

	t.Run("individual receiver outside scope is 404 — no disclosure", func(t *testing.T) {
		outOfScope := doAs(memberA.ID, fmt.Sprintf("/receivers/%s", receiverFromB.ID))
		assert.Equal(t, http.StatusNotFound, outOfScope.Code)

		rr := doAs(memberA.ID, fmt.Sprintf("/receivers/%s", receiverFromA.ID))
		assert.Equal(t, http.StatusOK, rr.Code)

		rr = doAs(owner.ID, fmt.Sprintf("/receivers/%s", receiverFromB.ID))
		assert.Equal(t, http.StatusOK, rr.Code)

		// Visibility is resolved before the receiver is loaded, so a receiver that is real but out of
		// reach and one that never existed are both answered by the same check.
		absent := doAs(memberA.ID, fmt.Sprintf("/receivers/%s", receiverFromB.ID+"-nope"))
		assert.Equal(t, http.StatusNotFound, absent.Code)
	})

	// A receiver with no payments belongs to the account it was created under, so its creator can
	// find it immediately and nobody else can. This is what source_wallet_id buys: before it, an
	// unpaid receiver matched no account and had to be shown to the whole tenant.
	t.Run("an unpaid receiver is visible to its own account only", func(t *testing.T) {
		unpaidA := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{SourceWalletID: walletA.ID})
		unpaidB := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{SourceWalletID: walletBID})

		rr := doAs(memberA.ID, "/receivers")
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), unpaidA.ID, "the creator must find what they just created")
		assert.NotContains(t, rr.Body.String(), unpaidB.ID, "another account's unpaid receiver stays hidden")

		rr = doAs(memberA.ID, fmt.Sprintf("/receivers/%s", unpaidA.ID))
		assert.Equal(t, http.StatusOK, rr.Code)

		rr = doAs(memberA.ID, fmt.Sprintf("/receivers/%s", unpaidB.ID))
		assert.Equal(t, http.StatusNotFound, rr.Code)

		rr = doAs(owner.ID, fmt.Sprintf("/receivers/%s", unpaidB.ID))
		assert.Equal(t, http.StatusOK, rr.Code, "owners stay tenant-wide")
	})

	// Additive: paying a receiver created elsewhere brings them into your scope, and never removes
	// them from the creating account's.
	t.Run("a payment adds the paying account without removing the creating one", func(t *testing.T) {
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{SourceWalletID: walletBID})

		rr := doAs(memberA.ID, fmt.Sprintf("/receivers/%s", receiver.ID))
		require.Equal(t, http.StatusNotFound, rr.Code)

		d := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name: "recv-disb-additive", SourceWalletID: walletA.ID, Status: data.StartedDisbursementStatus,
		})
		rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, d.Wallet.ID, data.ReadyReceiversWalletStatus)
		data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rw, Disbursement: d, Asset: *d.Asset, Amount: "3", Status: data.DraftPaymentStatus,
		})

		rr = doAs(memberA.ID, fmt.Sprintf("/receivers/%s", receiver.ID))
		assert.Equal(t, http.StatusOK, rr.Code, "paying them brings them into wallet A's scope")
	})
}

// Test_ReceiverScoping_Writes covers the write half of the rule. Reads were membership-filtered
// while writes were not, so a wallet-scoped user could rewrite the contact details of a receiver
// they could not see.
func Test_ReceiverScoping_Writes(t *testing.T) {
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
		INSERT INTO distribution_wallets (name, distribution_account_type)
		VALUES ('write-wallet-b', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))

	memberA := &auth.User{ID: "write-member-a", Email: "a@write.test", Roles: []string{string(data.BusinessUserRole)}}
	_, err = models.WalletMemberships.Insert(ctx, dbConnectionPool, memberA.ID, walletA.ID, data.BusinessUserRole, nil)
	require.NoError(t, err)

	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.On("GetUserByID", mock.Anything, memberA.ID).Return(memberA, nil).Maybe()

	receiverA := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{SourceWalletID: walletA.ID})
	receiverB := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{SourceWalletID: walletBID})

	updateHandler := UpdateReceiverHandler{Models: models, DBConnectionPool: dbConnectionPool, AuthManager: authManagerMock}
	rwHandler := ReceiverWalletsHandler{Models: models, AuthManager: authManagerMock}
	r := chi.NewRouter()
	r.Patch("/receivers/{id}", updateHandler.UpdateReceiver)
	r.Patch("/receivers/wallets/{receiver_wallet_id}/status", rwHandler.PatchReceiverWalletStatus)
	r.Patch("/receivers/wallets/{receiver_wallet_id}", rwHandler.RetryInvitation)
	r.Patch("/receivers/{receiver_id}/wallets/{receiver_wallet_id}", rwHandler.PatchReceiverWallet)

	patchAs := func(userID, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPatch, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(sdpcontext.SetUserIDInContext(ctx, userID))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	t.Run("editing a receiver outside the scope is refused and changes nothing", func(t *testing.T) {
		rr := patchAs(memberA.ID, fmt.Sprintf("/receivers/%s", receiverB.ID), `{"email":"attacker@evil.test"}`)
		require.Equal(t, http.StatusNotFound, rr.Code)

		reloaded, getErr := models.Receiver.Get(ctx, dbConnectionPool, receiverB.ID)
		require.NoError(t, getErr)
		assert.Equal(t, receiverB.Email, reloaded.Email, "the contact must survive the refused write")
	})

	t.Run("editing a receiver inside the scope still works", func(t *testing.T) {
		rr := patchAs(memberA.ID, fmt.Sprintf("/receivers/%s", receiverA.ID), `{"email":"legit@write.test"}`)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

		reloaded, getErr := models.Receiver.Get(ctx, dbConnectionPool, receiverA.ID)
		require.NoError(t, getErr)
		assert.Equal(t, "legit@write.test", reloaded.Email)
	})

	// The gate must not become an existence oracle: a receiver wallet that is real but out of reach
	// has to be indistinguishable from one that was never there.
	t.Run("out of scope and absent are indistinguishable", func(t *testing.T) {
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "write-wallet", "https://w.com", "w.com", "w://")
		rwB := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverB.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

		outOfScope := patchAs(memberA.ID, fmt.Sprintf("/receivers/wallets/%s/status", rwB.ID), `{"status":"READY"}`)
		absent := patchAs(memberA.ID, "/receivers/wallets/8b9f0e1c-0000-4000-8000-000000000000/status", `{"status":"READY"}`)

		assert.Equal(t, http.StatusNotFound, outOfScope.Code)
		assert.Equal(t, absent.Code, outOfScope.Code)
		assert.JSONEq(t, absent.Body.String(), outOfScope.Body.String())
	})

	// RetryInvitation mutates and returns in one statement, so the gate has to run before it. If it
	// ran after, the invitation would already have been re-sent by the time we answered 404.
	t.Run("retrying another account's invitation is refused before it re-sends", func(t *testing.T) {
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "retry-wallet", "https://r.com", "r.com", "r://")
		rwB := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverB.ID, wallet.ID, data.ReadyReceiversWalletStatus)
		_, execErr := dbConnectionPool.ExecContext(ctx,
			`UPDATE receiver_wallets SET invitation_sent_at = NOW() WHERE id = $1`, rwB.ID)
		require.NoError(t, execErr)

		rr := patchAs(memberA.ID, fmt.Sprintf("/receivers/wallets/%s", rwB.ID), "")
		require.Equal(t, http.StatusNotFound, rr.Code)

		var invitationSentAt *time.Time
		require.NoError(t, dbConnectionPool.GetContext(ctx, &invitationSentAt,
			`SELECT invitation_sent_at FROM receiver_wallets WHERE id = $1`, rwB.ID))
		assert.NotNil(t, invitationSentAt, "the invitation must not have been reset by a refused call")
	})

	t.Run("patching another account's receiver wallet is refused", func(t *testing.T) {
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "patch-wallet", "https://p.com", "p.com", "p://")
		data.MakeWalletUserManaged(t, ctx, dbConnectionPool, wallet.ID)
		rwB := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverB.ID, wallet.ID, data.DraftReceiversWalletStatus)

		rr := patchAs(memberA.ID,
			fmt.Sprintf("/receivers/%s/wallets/%s", receiverB.ID, rwB.ID),
			`{"stellar_address":"GDQP2KPQGKIHYJGXNUIYOMHARUARCA7DJT5FO2FFOOKY3B2WSQHG4W37"}`)
		assert.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
	})
}

// Test_ReceiverScoping_Stats pins the payment counters to the caller's accounts. The counters and
// the payments list on the receiver details page are two different queries, so without this they
// disagree: the totals count every account's payments while the list below shows only one.
func Test_ReceiverScoping_Stats(t *testing.T) {
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
		INSERT INTO distribution_wallets (name, distribution_account_type)
		VALUES ('stats-scope-b', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))

	memberA := &auth.User{ID: "stats-member-a", Email: "a@stats.test", Roles: []string{string(data.BusinessUserRole)}}
	owner := &auth.User{ID: "stats-owner", Email: "o@stats.test", IsOwner: true}
	_, err = models.WalletMemberships.Insert(ctx, dbConnectionPool, memberA.ID, walletA.ID, data.BusinessUserRole, nil)
	require.NoError(t, err)

	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.On("GetUserByID", mock.Anything, memberA.ID).Return(memberA, nil).Maybe()
	authManagerMock.On("GetUserByID", mock.Anything, owner.ID).Return(owner, nil).Maybe()

	// One receiver paid once from A and twice from B: three payments in total, one of which A can see.
	// The receiver-wallet pair is unique, so both disbursements share the one row.
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{SourceWalletID: walletA.ID})
	appWallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "stats-app-wallet", "https://s.com", "s.com", "s://")
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, appWallet.ID, data.ReadyReceiversWalletStatus)

	payFrom := func(name, walletID string, count int) {
		d := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name: name, SourceWalletID: walletID, Status: data.StartedDisbursementStatus, Wallet: appWallet,
		})
		for i := 0; i < count; i++ {
			data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
				ReceiverWallet: receiverWallet, Disbursement: d, Asset: *d.Asset, Amount: "3", Status: data.DraftPaymentStatus,
			})
		}
	}
	payFrom("stats-disb-a", walletA.ID, 1)
	payFrom("stats-disb-b", walletBID, 2)

	handler := ReceiverHandler{Models: models, DBConnectionPool: dbConnectionPool, AuthManager: authManagerMock}
	r := chi.NewRouter()
	r.Get("/receivers/{id}", handler.GetReceiver)

	totalPaymentsFor := func(userID, headerWalletID string) string {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/receivers/%s", receiver.ID), nil)
		req = req.WithContext(sdpcontext.SetUserIDInContext(ctx, userID))
		if headerWalletID != "" {
			req.Header.Set(XWalletIDHeader, headerWalletID)
		}
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

		var body struct {
			TotalPayments string `json:"total_payments"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
		return body.TotalPayments
	}

	assert.Equal(t, "1", totalPaymentsFor(memberA.ID, ""), "a scoped caller counts only their own account's payments")
	assert.Equal(t, "3", totalPaymentsFor(owner.ID, ""), "no account selected stays tenant-wide")

	// The counters follow the account picker, which reaches the API only as a header. Scoping them by
	// membership instead looks right for a member of one account and does nothing at all for an Owner.
	assert.Equal(t, "1", totalPaymentsFor(owner.ID, walletA.ID), "an owner on a selected account counts only that account")
	assert.Equal(t, "2", totalPaymentsFor(owner.ID, walletBID), "and switching accounts switches the counters")

	// Selecting an account the receiver has no payments from is a real state: the page still opens,
	// it just has nothing to count.
	var walletCID string
	require.NoError(t, dbConnectionPool.GetContext(ctx, &walletCID, `
		INSERT INTO distribution_wallets (name, distribution_account_type)
		VALUES ('stats-scope-c', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))
	assert.Equal(t, "0", totalPaymentsFor(owner.ID, walletCID), "an account that never paid them counts zero")
}

// Test_ReceiverScoping_APIKey covers the API key branch of the scope resolution, which is separate
// code from the JWT branch: it answers from the key's own stored scope and never loads a user, so a
// key outlives its creator. The AuthManager mock below has no expectations on purpose — if the key
// path ever falls back to resolving a user, these tests panic rather than quietly passing.
func Test_ReceiverScoping_APIKey(t *testing.T) {
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
		INSERT INTO distribution_wallets (name, distribution_account_type)
		VALUES ('apikey-wallet-b', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))

	receiverA := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{SourceWalletID: walletA.ID})
	receiverB := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{SourceWalletID: walletBID})

	authManagerMock := &auth.AuthManagerMock{}
	readHandler := ReceiverHandler{Models: models, DBConnectionPool: dbConnectionPool, AuthManager: authManagerMock}
	writeHandler := UpdateReceiverHandler{Models: models, DBConnectionPool: dbConnectionPool, AuthManager: authManagerMock}
	r := chi.NewRouter()
	r.Get("/receivers/{id}", readHandler.GetReceiver)
	r.Patch("/receivers/{id}", writeHandler.UpdateReceiver)

	// Mirrors api_keys_middleware: the key carries the scope, and its creator's id rides along as the
	// author of any write. The scope must come from the key, never from that user.
	asKey := func(method, path, body string, walletIDs []string) *httptest.ResponseRecorder {
		key := &data.APIKey{
			ID: "apikey-1", Name: "scoped key",
			DistributionWalletIDs: walletIDs,
			CreatedBy:             "apikey-creator",
		}
		reqCtx := sdpcontext.SetAPIKeyInContext(ctx, key)
		reqCtx = sdpcontext.SetUserIDInContext(reqCtx, key.CreatedBy)

		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(reqCtx)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	scopedToA := []string{walletA.ID}

	t.Run("a key reads only the receivers its own scope reaches", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, asKey(http.MethodGet, fmt.Sprintf("/receivers/%s", receiverA.ID), "", scopedToA).Code)
		assert.Equal(t, http.StatusNotFound, asKey(http.MethodGet, fmt.Sprintf("/receivers/%s", receiverB.ID), "", scopedToA).Code)
	})

	t.Run("a key writes only to the receivers its own scope reaches", func(t *testing.T) {
		rr := asKey(http.MethodPatch, fmt.Sprintf("/receivers/%s", receiverB.ID), `{"email":"apikey@evil.test"}`, scopedToA)
		require.Equal(t, http.StatusNotFound, rr.Code)

		reloaded, getErr := models.Receiver.Get(ctx, dbConnectionPool, receiverB.ID)
		require.NoError(t, getErr)
		assert.Equal(t, receiverB.Email, reloaded.Email)

		assert.Equal(t, http.StatusOK,
			asKey(http.MethodPatch, fmt.Sprintf("/receivers/%s", receiverA.ID), `{"email":"apikey-ok@test.local"}`, scopedToA).Code)
	})

	// WalletScope() returns an empty slice rather than nil for a key with no accounts, so a key never
	// inherits the Owner's tenant-wide view the way a nil scope would.
	t.Run("a key naming no accounts reaches nothing", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, asKey(http.MethodGet, fmt.Sprintf("/receivers/%s", receiverA.ID), "", nil).Code)
		assert.Equal(t, http.StatusNotFound, asKey(http.MethodGet, fmt.Sprintf("/receivers/%s", receiverB.ID), "", nil).Code)
	})
}
