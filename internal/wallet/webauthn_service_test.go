package wallet

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/veraison/go-cose"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

func Test_NewWebAuthnService(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	webAuthnConfig := &webauthn.Config{
		RPDisplayName: "Test RP",
		RPID:          "localhost",
		RPOrigins:     []string{"http://localhost"},
	}
	webAuthn, err := webauthn.New(webAuthnConfig)
	require.NoError(t, err)

	sessionCache := NewInMemorySessionCache(5*time.Minute, 10*time.Minute)

	t.Run("return an error if models is nil", func(t *testing.T) {
		service, err := NewWebAuthnService(nil, webAuthn, sessionCache)
		assert.Nil(t, service)
		assert.EqualError(t, err, "models cannot be nil")
	})

	t.Run("return an error if webAuthn is nil", func(t *testing.T) {
		service, err := NewWebAuthnService(models, nil, sessionCache)
		assert.Nil(t, service)
		assert.EqualError(t, err, "webAuthn cannot be nil")
	})

	t.Run("return an error if sessionCache is nil", func(t *testing.T) {
		service, err := NewWebAuthnService(models, webAuthn, nil)
		assert.Nil(t, service)
		assert.EqualError(t, err, "sessionCache cannot be nil")
	})

	t.Run("🎉 successfully creates a new WebAuthnService instance", func(t *testing.T) {
		service, err := NewWebAuthnService(models, webAuthn, sessionCache)
		require.NoError(t, err)
		require.NotNil(t, service)

		assert.Equal(t, models, service.sdpModels)
		assert.Equal(t, webAuthn, service.webAuthn)
		assert.Equal(t, sessionCache, service.sessionCache)
		assert.Equal(t, DefaultSessionTTL, service.sessionTTL)
	})
}

func Test_WebAuthnService_StartPasskeyRegistration(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	webAuthnConfig := &webauthn.Config{
		RPDisplayName: "Test RP",
		RPID:          "localhost",
		RPOrigins:     []string{"http://localhost"},
	}
	webAuthn, err := webauthn.New(webAuthnConfig)
	require.NoError(t, err)

	t.Run("returns ErrInvalidToken if token does not exist", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		sessionCache := NewInMemorySessionCache(5*time.Minute, 10*time.Minute)
		service, err := NewWebAuthnService(models, webAuthn, sessionCache)
		require.NoError(t, err)

		nonExistentToken := "non-existent-token"

		creation, err := service.StartPasskeyRegistration(ctx, nonExistentToken)
		assert.Nil(t, creation)
		assert.ErrorIs(t, err, ErrInvalidToken)
	})

	t.Run("returns ErrWalletAlreadyExists if wallet status is not pending", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		sessionCache := NewInMemorySessionCache(5*time.Minute, 10*time.Minute)
		service, err := NewWebAuthnService(models, webAuthn, sessionCache)
		require.NoError(t, err)

		wallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "", data.SuccessWalletStatus)

		creation, err := service.StartPasskeyRegistration(ctx, wallet.Token)
		assert.Nil(t, creation)
		assert.ErrorIs(t, err, ErrWalletAlreadyExists)
	})

	t.Run("🎉 successfully starts passkey registration and stores session", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		sessionCache := NewInMemorySessionCache(5*time.Minute, 10*time.Minute)
		service, err := NewWebAuthnService(models, webAuthn, sessionCache)
		require.NoError(t, err)

		wallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "", data.PendingWalletStatus)

		creation, err := service.StartPasskeyRegistration(ctx, wallet.Token)
		require.NoError(t, err)
		require.NotNil(t, creation)

		assert.NotNil(t, creation.Response)
		assert.NotEmpty(t, creation.Response.Challenge)
		assert.Equal(t, "localhost", creation.Response.RelyingParty.ID)
		assert.Equal(t, "Test RP", creation.Response.RelyingParty.Name)
		assert.Equal(t, protocol.URLEncodedBase64(wallet.Token), creation.Response.User.ID)
		assert.Equal(t, wallet.Token, creation.Response.User.Name)
		assert.Equal(t, "SDP Wallet User", creation.Response.User.DisplayName)

		assert.Equal(t, protocol.ResidentKeyRequirementPreferred, creation.Response.AuthenticatorSelection.ResidentKey)
		assert.Equal(t, protocol.Platform, creation.Response.AuthenticatorSelection.AuthenticatorAttachment)
		assert.Equal(t, protocol.VerificationRequired, creation.Response.AuthenticatorSelection.UserVerification)

		assert.Equal(t, 1, sessionCache.cache.ItemCount())
	})
}

func Test_WebAuthnService_FinishPasskeyRegistration(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	webAuthnConfig := &webauthn.Config{
		RPDisplayName: "Test RP",
		RPID:          "localhost",
		RPOrigins:     []string{"http://localhost"},
	}
	webAuthn, err := webauthn.New(webAuthnConfig)
	require.NoError(t, err)

	t.Run("returns ErrInvalidToken if token does not exist", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		sessionCache := NewInMemorySessionCache(5*time.Minute, 10*time.Minute)
		service, err := NewWebAuthnService(models, webAuthn, sessionCache)
		require.NoError(t, err)

		nonExistentToken := "non-existent-token"
		req := &http.Request{}

		credential, err := service.FinishPasskeyRegistration(ctx, nonExistentToken, req)
		assert.Nil(t, credential)
		assert.ErrorIs(t, err, ErrInvalidToken)
	})

	t.Run("returns ErrWalletAlreadyExists if wallet status is not pending", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		sessionCache := NewInMemorySessionCache(5*time.Minute, 10*time.Minute)
		service, err := NewWebAuthnService(models, webAuthn, sessionCache)
		require.NoError(t, err)

		wallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "", data.SuccessWalletStatus)
		req := &http.Request{}

		credential, err := service.FinishPasskeyRegistration(ctx, wallet.Token, req)
		assert.Nil(t, credential)
		assert.ErrorIs(t, err, ErrWalletAlreadyExists)
	})

	t.Run("returns error if WebAuthn verification fails", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		sessionCache := NewInMemorySessionCache(5*time.Minute, 10*time.Minute)
		service, err := NewWebAuthnService(models, webAuthn, sessionCache)
		require.NoError(t, err)

		wallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "", data.PendingWalletStatus)

		_, err = service.StartPasskeyRegistration(ctx, wallet.Token)
		require.NoError(t, err)

		req := &http.Request{
			Body: io.NopCloser(bytes.NewBufferString(`{"invalid": "data"}`)),
		}

		credential, err := service.FinishPasskeyRegistration(ctx, wallet.Token, req)
		assert.Nil(t, credential)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "parsing credential creation response")
	})
}

func Test_WebAuthnService_StartPasskeyAuthentication(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	webAuthnConfig := &webauthn.Config{
		RPDisplayName: "Test RP",
		RPID:          "localhost",
		RPOrigins:     []string{"http://localhost"},
	}
	webAuthn, err := webauthn.New(webAuthnConfig)
	require.NoError(t, err)

	t.Run("🎉 successfully starts discoverable passkey authentication and stores session in cache", func(t *testing.T) {
		sessionCache := NewInMemorySessionCache(5*time.Minute, 10*time.Minute)
		service, err := NewWebAuthnService(models, webAuthn, sessionCache)
		require.NoError(t, err)

		assertion, err := service.StartPasskeyAuthentication(ctx)
		require.NoError(t, err)
		require.NotNil(t, assertion)

		assert.NotNil(t, assertion.Response)
		assert.NotEmpty(t, assertion.Response.Challenge)
		assert.Equal(t, protocol.VerificationRequired, assertion.Response.UserVerification)

		assert.Equal(t, 1, sessionCache.cache.ItemCount())
	})
}

func Test_WebAuthnService_FinishPasskeyAuthentication(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	webAuthnConfig := &webauthn.Config{
		RPDisplayName: "Test RP",
		RPID:          "localhost",
		RPOrigins:     []string{"http://localhost"},
	}
	webAuthn, err := webauthn.New(webAuthnConfig)
	require.NoError(t, err)

	sessionCache := NewInMemorySessionCache(5*time.Minute, 10*time.Minute)
	service, err := NewWebAuthnService(models, webAuthn, sessionCache)
	require.NoError(t, err)

	t.Run("returns error if parsing request body fails", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		req := &http.Request{
			Body: io.NopCloser(bytes.NewBufferString(`invalid json`)),
		}

		retrievedWallet, err := service.FinishPasskeyAuthentication(ctx, req)
		assert.Nil(t, retrievedWallet)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "parsing credential request response")
	})
}

func Test_uncompressedECToCOSE(t *testing.T) {
	t.Run("returns error if key length is not 65 bytes", func(t *testing.T) {
		shortKey := make([]byte, 32)
		_, err := uncompressedECToCOSE(shortKey)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid uncompressed key length")
	})

	t.Run("returns error if key does not start with 0x04 prefix", func(t *testing.T) {
		invalidKey := make([]byte, 65)
		invalidKey[0] = 0x03
		_, err := uncompressedECToCOSE(invalidKey)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid uncompressed key format")
	})

	t.Run("🎉 successfully converts valid uncompressed EC key to COSE", func(t *testing.T) {
		validKey := make([]byte, 65)
		validKey[0] = 0x04
		for i := 1; i < 65; i++ {
			validKey[i] = byte(i)
		}

		coseBytes, err := uncompressedECToCOSE(validKey)
		require.NoError(t, err)
		assert.NotEmpty(t, coseBytes)

		var key cose.Key
		err = key.UnmarshalCBOR(coseBytes)
		require.NoError(t, err)

		assert.Equal(t, cose.KeyTypeEC2, key.Type)
		assert.Equal(t, cose.AlgorithmES256, key.Algorithm)
	})
}
