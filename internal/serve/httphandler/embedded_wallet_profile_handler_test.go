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

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mockSvc := mocks.NewMockEmbeddedWalletService(t)
		handler := EmbeddedWalletProfileHandler{EmbeddedWalletService: mockSvc}

		asset := &data.Asset{Code: "USDC"}

		mockSvc.On("IsVerificationPending", mock.Anything, contractAddress).
			Return(true, nil).Once()
		mockSvc.On("GetPendingDisbursementAsset", mock.Anything, contractAddress).
			Return(asset, nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/embedded-wallets/profile", nil)
		ctx := sdpcontext.SetWalletContractAddressInContext(req.Context(), contractAddress)
		req = req.WithContext(ctx)
		resp := httptest.NewRecorder()

		handler.GetProfile(resp, req)

		require.Equal(t, http.StatusOK, resp.Code)

		var body EmbeddedWalletProfileResponse
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
		assert.True(t, body.IsVerificationPending)
		assert.Equal(t, asset, body.PendingAsset)
	})

	t.Run("unauthorized when wallet missing", func(t *testing.T) {
		t.Parallel()

		mockSvc := mocks.NewMockEmbeddedWalletService(t)
		handler := EmbeddedWalletProfileHandler{EmbeddedWalletService: mockSvc}

		mockSvc.On("IsVerificationPending", mock.Anything, contractAddress).
			Return(false, services.ErrInvalidContractAddress).Once()

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
}

func Test_EmbeddedWalletProfileHandler_GetAssets(t *testing.T) {
	t.Run("retrieve supported assets successfully", func(t *testing.T) {
		dbt := dbtest.Open(t)
		t.Cleanup(dbt.Close)

		pool, err := db.OpenDBConnectionPool(dbt.DSN)
		require.NoError(t, err)
		t.Cleanup(func() { pool.Close() })

		models, err := data.NewModels(pool)
		require.NoError(t, err)

		asset := data.CreateAssetFixture(t, context.Background(), pool, "TEST", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
		wallet := data.CreateWalletFixture(t, context.Background(), pool, "embedded-wallet", "https://example.com", "embedded.example.com", "embedded://")
		data.CreateWalletAssets(t, context.Background(), pool, wallet.ID, []string{asset.ID})
		_, err = pool.ExecContext(context.Background(), "UPDATE wallets SET embedded = true WHERE id = $1", wallet.ID)
		require.NoError(t, err)

		handler := EmbeddedWalletProfileHandler{Models: models}

		req := httptest.NewRequest(http.MethodGet, "/embedded-wallets/profile-assets", nil)
		resp := httptest.NewRecorder()

		handler.GetAssets(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		var body EmbeddedWalletAssetsResponse
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
		assert.Len(t, body.Assets, 1)
		assert.Equal(t, "TEST", body.Assets[0].Code)
		assert.Equal(t, "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV", body.Assets[0].Issuer)
	})

	t.Run("internal error when embedded wallet not configured", func(t *testing.T) {
		dbt := dbtest.Open(t)
		t.Cleanup(dbt.Close)

		pool, err := db.OpenDBConnectionPool(dbt.DSN)
		require.NoError(t, err)
		t.Cleanup(func() { pool.Close() })

		models, err := data.NewModels(pool)
		require.NoError(t, err)

		handler := EmbeddedWalletProfileHandler{Models: models}

		req := httptest.NewRequest(http.MethodGet, "/embedded-wallets/profile-assets", nil)
		resp := httptest.NewRecorder()

		handler.GetAssets(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
	})

	t.Run("internal error when multiple embedded wallets configured", func(t *testing.T) {
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

		handler := EmbeddedWalletProfileHandler{Models: models}

		req := httptest.NewRequest(http.MethodGet, "/embedded-wallets/profile-assets", nil)
		resp := httptest.NewRecorder()

		handler.GetAssets(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
	})
}
