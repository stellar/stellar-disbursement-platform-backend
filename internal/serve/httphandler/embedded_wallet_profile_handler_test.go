package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
)

func Test_EmbeddedWalletProfileHandler_GetProfile(t *testing.T) {
	contractAddress := "CCYU2FUIMK23K34U3SWCN2O2JVI6JBGUGQUILYK7GRPCIDABVVTCS7R4"

	t.Run("success when verification pending", func(t *testing.T) {
		t.Parallel()

		dbt := dbtest.Open(t)
		t.Cleanup(dbt.Close)

		pool, err := db.OpenDBConnectionPool(dbt.DSN)
		require.NoError(t, err)
		t.Cleanup(func() { pool.Close() })

		models, err := data.NewModels(pool)
		require.NoError(t, err)

		walletAsset := data.CreateAssetFixture(t, context.Background(), pool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
		wallet := data.CreateWalletFixture(t, context.Background(), pool, "embedded-wallet", "https://example.com", "embedded.example.com", "embedded://")
		data.CreateWalletAssets(t, context.Background(), pool, wallet.ID, []string{walletAsset.ID})
		_, err = pool.ExecContext(context.Background(), "UPDATE wallets SET embedded = true WHERE id = $1", wallet.ID)
		require.NoError(t, err)

		mockSvc := mocks.NewMockEmbeddedWalletService(t)
		handler := EmbeddedWalletProfileHandler{EmbeddedWalletService: mockSvc, Models: models}

		pendingAsset := &data.Asset{Code: "TEST"}

		mockSvc.On("IsVerificationPending", mock.Anything, contractAddress).
			Return(true, nil).Once()
		mockSvc.On("GetPendingDisbursementAsset", mock.Anything, contractAddress).
			Return(pendingAsset, nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/embedded-wallets/profile", nil)
		ctx := sdpcontext.SetWalletContractAddressInContext(req.Context(), contractAddress)
		req = req.WithContext(ctx)
		resp := httptest.NewRecorder()

		handler.GetProfile(resp, req)

		require.Equal(t, http.StatusOK, resp.Code)

		var body EmbeddedWalletProfileResponse
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
		assert.True(t, body.Verification.IsPending)
		assert.Equal(t, pendingAsset, body.Verification.PendingAsset)
		assert.Nil(t, body.Wallet)
		mockSvc.AssertNotCalled(t, "GetReceiverContact", mock.Anything, mock.Anything)
	})

	t.Run("unauthorized when contract address missing in context", func(t *testing.T) {
		t.Parallel()

		mockSvc := mocks.NewMockEmbeddedWalletService(t)
		handler := EmbeddedWalletProfileHandler{EmbeddedWalletService: mockSvc}

		req := httptest.NewRequest(http.MethodGet, "/embedded-wallets/profile", nil)
		resp := httptest.NewRecorder()

		handler.GetProfile(resp, req)

		assert.Equal(t, http.StatusUnauthorized, resp.Code)
		mockSvc.AssertNotCalled(t, "IsVerificationPending", mock.Anything, mock.Anything)
	})

	t.Run("internal error when pending asset retrieval fails", func(t *testing.T) {
		t.Parallel()

		mockSvc := mocks.NewMockEmbeddedWalletService(t)
		handler := EmbeddedWalletProfileHandler{EmbeddedWalletService: mockSvc}

		mockSvc.On("IsVerificationPending", mock.Anything, contractAddress).
			Return(true, nil).Once()
		pendingErr := errors.New("pending asset boom")
		mockSvc.On("GetPendingDisbursementAsset", mock.Anything, contractAddress).
			Return((*data.Asset)(nil), pendingErr).Once()

		req := httptest.NewRequest(http.MethodGet, "/embedded-wallets/profile", nil)
		ctx := sdpcontext.SetWalletContractAddressInContext(req.Context(), contractAddress)
		req = req.WithContext(ctx)
		resp := httptest.NewRecorder()

		handler.GetProfile(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
	})

	t.Run("unauthorized when pending asset contract invalid", func(t *testing.T) {
		t.Parallel()

		mockSvc := mocks.NewMockEmbeddedWalletService(t)
		handler := EmbeddedWalletProfileHandler{EmbeddedWalletService: mockSvc}

		mockSvc.On("IsVerificationPending", mock.Anything, contractAddress).
			Return(true, nil).Once()
		mockSvc.On("GetPendingDisbursementAsset", mock.Anything, contractAddress).
			Return((*data.Asset)(nil), services.ErrMissingContractAddress).Once()

		req := httptest.NewRequest(http.MethodGet, "/embedded-wallets/profile", nil)
		ctx := sdpcontext.SetWalletContractAddressInContext(req.Context(), contractAddress)
		req = req.WithContext(ctx)
		resp := httptest.NewRecorder()

		handler.GetProfile(resp, req)

		assert.Equal(t, http.StatusUnauthorized, resp.Code)
	})

	t.Run("success when receiver contact is available", func(t *testing.T) {
		t.Parallel()

		dbt := dbtest.Open(t)
		t.Cleanup(dbt.Close)

		pool, err := db.OpenDBConnectionPool(dbt.DSN)
		require.NoError(t, err)
		t.Cleanup(func() { pool.Close() })

		models, err := data.NewModels(pool)
		require.NoError(t, err)

		walletAsset := data.CreateAssetFixture(t, context.Background(), pool, "EURC", "GBBM37LEK4EQQM47SHKWWDS6EB4WIOVSKA3TVCXKSU4PTOJAS3I3XGX5")
		wallet := data.CreateWalletFixture(t, context.Background(), pool, "embedded-wallet", "https://example.com", "embedded.example.com", "embedded://")
		data.CreateWalletAssets(t, context.Background(), pool, wallet.ID, []string{walletAsset.ID})
		_, err = pool.ExecContext(context.Background(), "UPDATE wallets SET embedded = true WHERE id = $1", wallet.ID)
		require.NoError(t, err)

		mockSvc := mocks.NewMockEmbeddedWalletService(t)
		handler := EmbeddedWalletProfileHandler{EmbeddedWalletService: mockSvc, Models: models}

		mockSvc.On("IsVerificationPending", mock.Anything, contractAddress).
			Return(false, nil).Once()
		receiver := &data.Receiver{Email: "test@example.com"}
		mockSvc.On("GetReceiverContact", mock.Anything, contractAddress).
			Return(receiver, nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/embedded-wallets/profile", nil)
		ctx := sdpcontext.SetWalletContractAddressInContext(req.Context(), contractAddress)
		req = req.WithContext(ctx)
		resp := httptest.NewRecorder()

		handler.GetProfile(resp, req)

		require.Equal(t, http.StatusOK, resp.Code)

		var body EmbeddedWalletProfileResponse
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
		assert.False(t, body.Verification.IsPending)
		assert.Nil(t, body.Verification.PendingAsset)
		require.NotNil(t, body.Wallet)
		require.Len(t, body.Wallet.SupportedAssets, 1)
		assert.Equal(t, "EURC", body.Wallet.SupportedAssets[0].Code)
		require.NotNil(t, body.Wallet.ReceiverContact)
		assert.Equal(t, data.ReceiverContactTypeEmail, body.Wallet.ReceiverContact.Type)
		assert.Equal(t, "test@example.com", body.Wallet.ReceiverContact.Value)
		mockSvc.AssertNotCalled(t, "GetPendingDisbursementAsset", mock.Anything, mock.Anything)
	})

	t.Run("unauthorized when receiver contact lookup returns invalid data", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name string
			err  error
		}{
			{name: "missing contract address", err: services.ErrMissingContractAddress},
			{name: "record not found", err: data.ErrRecordNotFound},
		} {
			ttc := tc
			t.Run(ttc.name, func(t *testing.T) {
				t.Parallel()

				dbt := dbtest.Open(t)
				t.Cleanup(dbt.Close)

				pool, err := db.OpenDBConnectionPool(dbt.DSN)
				require.NoError(t, err)
				t.Cleanup(func() { pool.Close() })

				models, err := data.NewModels(pool)
				require.NoError(t, err)

				asset := data.CreateAssetFixture(t, context.Background(), pool, "USD", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
				wallet := data.CreateWalletFixture(t, context.Background(), pool, "embedded-wallet", "https://example.com", "embedded.example.com", "embedded://")
				data.CreateWalletAssets(t, context.Background(), pool, wallet.ID, []string{asset.ID})
				_, err = pool.ExecContext(context.Background(), "UPDATE wallets SET embedded = true WHERE id = $1", wallet.ID)
				require.NoError(t, err)

				mockSvc := mocks.NewMockEmbeddedWalletService(t)
				handler := EmbeddedWalletProfileHandler{EmbeddedWalletService: mockSvc, Models: models}

				mockSvc.On("IsVerificationPending", mock.Anything, contractAddress).
					Return(false, nil).Once()
				mockSvc.On("GetReceiverContact", mock.Anything, contractAddress).
					Return((*data.Receiver)(nil), ttc.err).Once()

				req := httptest.NewRequest(http.MethodGet, "/embedded-wallets/profile", nil)
				ctx := sdpcontext.SetWalletContractAddressInContext(req.Context(), contractAddress)
				req = req.WithContext(ctx)
				resp := httptest.NewRecorder()

				handler.GetProfile(resp, req)

				assert.Equal(t, http.StatusUnauthorized, resp.Code)
				mockSvc.AssertNotCalled(t, "GetPendingDisbursementAsset", mock.Anything, mock.Anything)
			})
		}
	})

	t.Run("internal error when receiver contact lookup fails", func(t *testing.T) {
		t.Parallel()

		dbt := dbtest.Open(t)
		t.Cleanup(dbt.Close)

		pool, err := db.OpenDBConnectionPool(dbt.DSN)
		require.NoError(t, err)
		t.Cleanup(func() { pool.Close() })

		models, err := data.NewModels(pool)
		require.NoError(t, err)

		asset := data.CreateAssetFixture(t, context.Background(), pool, "GBP", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
		wallet := data.CreateWalletFixture(t, context.Background(), pool, "embedded-wallet", "https://example.com", "embedded.example.com", "embedded://")
		data.CreateWalletAssets(t, context.Background(), pool, wallet.ID, []string{asset.ID})
		_, err = pool.ExecContext(context.Background(), "UPDATE wallets SET embedded = true WHERE id = $1", wallet.ID)
		require.NoError(t, err)

		mockSvc := mocks.NewMockEmbeddedWalletService(t)
		handler := EmbeddedWalletProfileHandler{EmbeddedWalletService: mockSvc, Models: models}

		mockSvc.On("IsVerificationPending", mock.Anything, contractAddress).
			Return(false, nil).Once()
		lookupErr := errors.New("contact boom")
		mockSvc.On("GetReceiverContact", mock.Anything, contractAddress).
			Return((*data.Receiver)(nil), lookupErr).Once()

		req := httptest.NewRequest(http.MethodGet, "/embedded-wallets/profile", nil)
		ctx := sdpcontext.SetWalletContractAddressInContext(req.Context(), contractAddress)
		req = req.WithContext(ctx)
		resp := httptest.NewRecorder()

		handler.GetProfile(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		mockSvc.AssertNotCalled(t, "GetPendingDisbursementAsset", mock.Anything, mock.Anything)
	})

	t.Run("internal error when embedded wallet not configured", func(t *testing.T) {
		t.Parallel()

		dbt := dbtest.Open(t)
		t.Cleanup(dbt.Close)

		pool, err := db.OpenDBConnectionPool(dbt.DSN)
		require.NoError(t, err)
		t.Cleanup(func() { pool.Close() })

		models, err := data.NewModels(pool)
		require.NoError(t, err)

		mockSvc := mocks.NewMockEmbeddedWalletService(t)
		handler := EmbeddedWalletProfileHandler{EmbeddedWalletService: mockSvc, Models: models}

		mockSvc.On("IsVerificationPending", mock.Anything, contractAddress).
			Return(false, nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/embedded-wallets/profile", nil)
		ctx := sdpcontext.SetWalletContractAddressInContext(req.Context(), contractAddress)
		req = req.WithContext(ctx)
		resp := httptest.NewRecorder()

		handler.GetProfile(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		mockSvc.AssertNotCalled(t, "GetPendingDisbursementAsset", mock.Anything, mock.Anything)
	})

	t.Run("internal error when multiple embedded wallets configured", func(t *testing.T) {
		t.Parallel()

		dbt := dbtest.Open(t)
		t.Cleanup(dbt.Close)

		pool, err := db.OpenDBConnectionPool(dbt.DSN)
		require.NoError(t, err)
		t.Cleanup(func() { pool.Close() })

		models, err := data.NewModels(pool)
		require.NoError(t, err)

		asset := data.CreateAssetFixture(t, context.Background(), pool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
		for i := 0; i < 2; i++ {
			name := fmt.Sprintf("embedded-wallet-multi-%d", i)
			homepage := fmt.Sprintf("https://example%d.com", i)
			sep10 := fmt.Sprintf("embedded%d.example.com", i)
			deepLink := fmt.Sprintf("embedded-%d://", i)
			var walletID string
			err := pool.GetContext(context.Background(), &walletID,
				`INSERT INTO wallets (name, homepage, sep_10_client_domain, deep_link_schema) VALUES ($1,$2,$3,$4) RETURNING id`,
				name, homepage, sep10, deepLink,
			)
			require.NoError(t, err)

			_, err = pool.ExecContext(context.Background(),
				"INSERT INTO wallets_assets (wallet_id, asset_id) VALUES ($1, $2)", walletID, asset.ID)
			require.NoError(t, err)

			_, err = pool.ExecContext(context.Background(), "UPDATE wallets SET embedded = true WHERE id = $1", walletID)
			require.NoError(t, err)
		}

		mockSvc := mocks.NewMockEmbeddedWalletService(t)
		handler := EmbeddedWalletProfileHandler{EmbeddedWalletService: mockSvc, Models: models}

		mockSvc.On("IsVerificationPending", mock.Anything, contractAddress).
			Return(false, nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/embedded-wallets/profile", nil)
		ctx := sdpcontext.SetWalletContractAddressInContext(req.Context(), contractAddress)
		req = req.WithContext(ctx)
		resp := httptest.NewRecorder()

		handler.GetProfile(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		mockSvc.AssertNotCalled(t, "GetPendingDisbursementAsset", mock.Anything, mock.Anything)
	})

	t.Run("unauthorized when wallet missing", func(t *testing.T) {
		t.Parallel()

		mockSvc := mocks.NewMockEmbeddedWalletService(t)
		handler := EmbeddedWalletProfileHandler{EmbeddedWalletService: mockSvc}

		mockSvc.On("IsVerificationPending", mock.Anything, contractAddress).
			Return(false, services.ErrMissingContractAddress).Once()

		req := httptest.NewRequest(http.MethodGet, "/embedded-wallets/profile", nil)
		ctx := sdpcontext.SetWalletContractAddressInContext(req.Context(), contractAddress)
		req = req.WithContext(ctx)
		resp := httptest.NewRecorder()

		handler.GetProfile(resp, req)

		assert.Equal(t, http.StatusUnauthorized, resp.Code)
	})

	t.Run("internal error when verification fails", func(t *testing.T) {
		t.Parallel()

		mockSvc := mocks.NewMockEmbeddedWalletService(t)
		handler := EmbeddedWalletProfileHandler{EmbeddedWalletService: mockSvc}

		wrappedErr := errors.New("boom")

		mockSvc.On("IsVerificationPending", mock.Anything, contractAddress).
			Return(false, wrappedErr).Once()

		req := httptest.NewRequest(http.MethodGet, "/embedded-wallets/profile", nil)
		ctx := sdpcontext.SetWalletContractAddressInContext(req.Context(), contractAddress)
		req = req.WithContext(ctx)
		resp := httptest.NewRecorder()

		handler.GetProfile(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
	})

	t.Run("internal error when receiver contact type unsupported", func(t *testing.T) {
		t.Parallel()

		dbt := dbtest.Open(t)
		t.Cleanup(dbt.Close)

		pool, err := db.OpenDBConnectionPool(dbt.DSN)
		require.NoError(t, err)
		t.Cleanup(func() { pool.Close() })

		models, err := data.NewModels(pool)
		require.NoError(t, err)

		asset := data.CreateAssetFixture(t, context.Background(), pool, "JPY", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
		wallet := data.CreateWalletFixture(t, context.Background(), pool, "embedded-wallet", "https://example.com", "embedded.example.com", "embedded://")
		data.CreateWalletAssets(t, context.Background(), pool, wallet.ID, []string{asset.ID})
		_, err = pool.ExecContext(context.Background(), "UPDATE wallets SET embedded = true WHERE id = $1", wallet.ID)
		require.NoError(t, err)

		mockSvc := mocks.NewMockEmbeddedWalletService(t)
		handler := EmbeddedWalletProfileHandler{EmbeddedWalletService: mockSvc, Models: models}

		mockSvc.On("IsVerificationPending", mock.Anything, contractAddress).
			Return(false, nil).Once()
		mockSvc.On("GetReceiverContact", mock.Anything, contractAddress).
			Return(&data.Receiver{}, nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/embedded-wallets/profile", nil)
		ctx := sdpcontext.SetWalletContractAddressInContext(req.Context(), contractAddress)
		req = req.WithContext(ctx)
		resp := httptest.NewRecorder()

		handler.GetProfile(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		mockSvc.AssertNotCalled(t, "GetPendingDisbursementAsset", mock.Anything, mock.Anything)
	})
}

func Test_renderWalletServiceError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("returns false when err nil", func(t *testing.T) {
		resp := httptest.NewRecorder()
		handled := renderWalletServiceError(ctx, resp, nil, "")
		assert.False(t, handled)
		assert.Equal(t, http.StatusOK, resp.Code)
	})

	for _, tc := range []struct {
		name         string
		err          error
		expectedCode int
	}{
		{name: "missing contract address", err: services.ErrMissingContractAddress, expectedCode: http.StatusUnauthorized},
		{name: "record not found", err: data.ErrRecordNotFound, expectedCode: http.StatusUnauthorized},
		{name: "internal error", err: errors.New("boom"), expectedCode: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := httptest.NewRecorder()
			handled := renderWalletServiceError(ctx, resp, tc.err, "internal failure")
			assert.True(t, handled)
			assert.Equal(t, tc.expectedCode, resp.Code)
		})
	}
}
