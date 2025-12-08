package httphandler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
)

func TestEmbeddedWalletProfileHandler_GetProfile(t *testing.T) {
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
