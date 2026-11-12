package wallet

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/veraison/go-cose"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func newTestSessionCache(t *testing.T, dbConnectionPool db.DBConnectionPool) SessionCacheInterface {
	t.Helper()
	cache, err := NewSessionCache(dbConnectionPool, 5*time.Minute)
	require.NoError(t, err)
	return cache
}

const (
	authenticatorFlagUP        byte = 0x01 // user present only: fails the required user verification
	authenticatorFlagsUPUV     byte = 0x05 // user present, user verified: a device-bound passkey
	authenticatorFlagsUPUVBEBS byte = 0x1D // + backup eligible, backed up: a synced passkey
	authenticatorFlagAT        byte = 0x40 // attested credential data present (registration only)
)

// softwarePasskey signs WebAuthn assertions with a P-256 key, standing in for a browser authenticator.
type softwarePasskey struct {
	key          *ecdsa.PrivateKey
	credentialID string
}

func newSoftwarePasskey(t *testing.T) *softwarePasskey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	rawID := make([]byte, 16)
	_, err = rand.Read(rawID)
	require.NoError(t, err)
	return &softwarePasskey{key: key, credentialID: base64.RawURLEncoding.EncodeToString(rawID)}
}

func (p *softwarePasskey) publicKeyHex(t *testing.T) string {
	t.Helper()
	ecdhKey, err := p.key.PublicKey.ECDH()
	require.NoError(t, err)
	return hex.EncodeToString(ecdhKey.Bytes())
}

// attestationRequest builds the registration/finish body for a challenge: a "none" attestation carrying this
// passkey's credential ID and public key, as the browser would for origin http://localhost:3000 (RP ID "localhost").
func (p *softwarePasskey) attestationRequest(t *testing.T, challenge string, flags byte) *http.Request {
	t.Helper()
	clientDataJSON := fmt.Appendf(nil, `{"type":"webauthn.create","challenge":%q,"origin":"http://localhost:3000","crossOrigin":false}`, challenge)
	rawID, err := base64.RawURLEncoding.DecodeString(p.credentialID)
	require.NoError(t, err)
	pubKey, err := hex.DecodeString(p.publicKeyHex(t))
	require.NoError(t, err)
	coseKey, err := cose.NewKeyEC2(cose.AlgorithmES256, pubKey[1:33], pubKey[33:65], nil)
	require.NoError(t, err)
	coseKeyBytes, err := coseKey.MarshalCBOR()
	require.NoError(t, err)

	rpIDHash := sha256.Sum256([]byte("localhost"))
	authenticatorData := slices.Concat(rpIDHash[:], []byte{flags | authenticatorFlagAT, 0, 0, 0, 0},
		make([]byte, 16), binary.BigEndian.AppendUint16(nil, uint16(len(rawID))), rawID, coseKeyBytes)
	// CBOR map {"fmt": "none", "attStmt": {}, "authData": <bytes>}, hand-encoded.
	attestationObject := slices.Concat([]byte{0xa3, 0x63}, []byte("fmt"), []byte{0x64}, []byte("none"),
		[]byte{0x67}, []byte("attStmt"), []byte{0xa0}, []byte{0x68}, []byte("authData"),
		[]byte{0x58, byte(len(authenticatorData))}, authenticatorData)

	b64 := base64.RawURLEncoding.EncodeToString
	body := fmt.Sprintf(`{"id":%q,"rawId":%q,"type":"public-key","response":{"clientDataJSON":%q,"attestationObject":%q}}`,
		p.credentialID, p.credentialID, b64(clientDataJSON), b64(attestationObject))
	return &http.Request{Body: io.NopCloser(strings.NewReader(body))}
}

// assertionRequest builds the authentication/finish body for a challenge, as the browser would for origin
// http://localhost:3000 (RP ID "localhost").
func (p *softwarePasskey) assertionRequest(t *testing.T, challenge, userHandle string, flags byte) *http.Request {
	t.Helper()
	clientDataJSON := fmt.Appendf(nil, `{"type":"webauthn.get","challenge":%q,"origin":"http://localhost:3000","crossOrigin":false}`, challenge)
	rpIDHash := sha256.Sum256([]byte("localhost"))
	authenticatorData := slices.Concat(rpIDHash[:], []byte{flags, 0, 0, 0, 0})
	clientDataHash := sha256.Sum256(clientDataJSON)
	digest := sha256.Sum256(slices.Concat(authenticatorData, clientDataHash[:]))
	signature, err := ecdsa.SignASN1(rand.Reader, p.key, digest[:])
	require.NoError(t, err)

	b64 := base64.RawURLEncoding.EncodeToString
	body := fmt.Sprintf(`{"id":%q,"rawId":%q,"type":"public-key","response":{"clientDataJSON":%q,"authenticatorData":%q,"signature":%q,"userHandle":%q}}`,
		p.credentialID, p.credentialID, b64(clientDataJSON), b64(authenticatorData), b64(signature), b64([]byte(userHandle)))
	return &http.Request{Body: io.NopCloser(strings.NewReader(body))}
}

func TestBuildPasskeyDisplayName(t *testing.T) {
	sdpUIBaseURL := "http://localhost:3000"
	testTenant := schema.Tenant{ID: "tenant-id", Name: "Test Tenant", SDPUIBaseURL: &sdpUIBaseURL}
	ctxWithTenant := sdpcontext.SetTenantInContext(context.Background(), &testTenant)
	createdAt := time.Date(2025, 12, 17, 18, 5, 30, 0, time.UTC)

	tests := []struct {
		name           string
		ctx            context.Context
		createdAt      *time.Time
		expectedName   string
		expectedErr    error
		expectedErrMsg string
	}{
		{
			name:         "returns display name with tenant and timestamp",
			ctx:          ctxWithTenant,
			createdAt:    &createdAt,
			expectedName: "Test Tenant Wallet - 2025-12-17 18:05 UTC",
		},
		{
			name:        "returns error when tenant is missing from context",
			ctx:         context.Background(),
			createdAt:   &createdAt,
			expectedErr: sdpcontext.ErrTenantNotFoundInContext,
		},
		{
			name:           "returns error when createdAt is nil",
			ctx:            ctxWithTenant,
			expectedErrMsg: "wallet creation time is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			displayName, err := buildPasskeyDisplayName(tt.ctx, tt.createdAt)
			if tt.expectedErr != nil {
				assert.Empty(t, displayName)
				assert.ErrorIs(t, err, tt.expectedErr)
				return
			}
			if tt.expectedErrMsg != "" {
				assert.Empty(t, displayName)
				assert.EqualError(t, err, tt.expectedErrMsg)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedName, displayName)
		})
	}
}

func Test_NewWebAuthnService(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	sessionCache := newTestSessionCache(t, dbConnectionPool)

	t.Run("return an error if models is nil", func(t *testing.T) {
		service, err := NewWebAuthnService(nil, sessionCache)
		assert.Nil(t, service)
		assert.EqualError(t, err, "models cannot be nil")
	})

	t.Run("return an error if sessionCache is nil", func(t *testing.T) {
		service, err := NewWebAuthnService(models, nil)
		assert.Nil(t, service)
		assert.EqualError(t, err, "sessionCache cannot be nil")
	})

	t.Run("🎉 successfully creates a new WebAuthnService instance", func(t *testing.T) {
		service, err := NewWebAuthnService(models, sessionCache)
		require.NoError(t, err)
		require.NotNil(t, service)

		assert.Equal(t, models, service.sdpModels)
		assert.Equal(t, sessionCache, service.sessionCache)
	})
}

func Test_WebAuthnService_StartPasskeyRegistration(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	sdpUIBaseURL := "http://localhost:3000"
	testTenant := schema.Tenant{ID: "tenant-id", Name: "Test Tenant", SDPUIBaseURL: &sdpUIBaseURL}
	ctx := sdpcontext.SetTenantInContext(context.Background(), &testTenant)
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	t.Run("returns ErrInvalidToken if token does not exist", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		sessionCache := newTestSessionCache(t, dbConnectionPool)
		service, err := NewWebAuthnService(models, sessionCache)
		require.NoError(t, err)

		nonExistentToken := "non-existent-token"

		creation, err := service.StartPasskeyRegistration(ctx, nonExistentToken)
		assert.Nil(t, creation)
		assert.ErrorIs(t, err, ErrInvalidToken)
	})

	t.Run("returns ErrWalletAlreadyExists if wallet status is not pending", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		sessionCache := newTestSessionCache(t, dbConnectionPool)
		service, err := NewWebAuthnService(models, sessionCache)
		require.NoError(t, err)

		wallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "", data.SuccessWalletStatus)

		creation, err := service.StartPasskeyRegistration(ctx, wallet.Token)
		assert.Nil(t, creation)
		assert.ErrorIs(t, err, ErrWalletAlreadyExists)
	})

	t.Run("🎉 successfully starts passkey registration and stores session", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		sessionCache := newTestSessionCache(t, dbConnectionPool)
		service, err := NewWebAuthnService(models, sessionCache)
		require.NoError(t, err)

		wallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "", data.PendingWalletStatus)

		creation, err := service.StartPasskeyRegistration(ctx, wallet.Token)
		require.NoError(t, err)
		require.NotNil(t, creation)

		assert.NotNil(t, creation.Response)
		assert.NotEmpty(t, creation.Response.Challenge)
		assert.Equal(t, "localhost", creation.Response.RelyingParty.ID)
		assert.Equal(t, RPDisplayName, creation.Response.RelyingParty.Name)
		assert.Equal(t, protocol.URLEncodedBase64(wallet.Token), creation.Response.User.ID)
		expectedDisplayName, err := buildPasskeyDisplayName(ctx, wallet.CreatedAt)
		require.NoError(t, err)
		assert.Equal(t, expectedDisplayName, creation.Response.User.Name)
		assert.Equal(t, expectedDisplayName, creation.Response.User.DisplayName)

		assert.Equal(t, protocol.ResidentKeyRequirementPreferred, creation.Response.AuthenticatorSelection.ResidentKey)
		assert.Equal(t, protocol.Platform, creation.Response.AuthenticatorSelection.AuthenticatorAttachment)
		assert.Equal(t, protocol.VerificationRequired, creation.Response.AuthenticatorSelection.UserVerification)

		_, err = sessionCache.Get(ctx, creation.Response.Challenge.String(), SessionTypeRegistration)
		require.NoError(t, err)
	})
}

func Test_WebAuthnService_FinishPasskeyRegistration(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	sdpUIBaseURL := "http://localhost:3000"
	testTenant := schema.Tenant{ID: "tenant-id", Name: "Test Tenant", SDPUIBaseURL: &sdpUIBaseURL}
	ctx := sdpcontext.SetTenantInContext(context.Background(), &testTenant)
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	t.Run("returns ErrInvalidToken if token does not exist", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		sessionCache := newTestSessionCache(t, dbConnectionPool)
		service, err := NewWebAuthnService(models, sessionCache)
		require.NoError(t, err)

		nonExistentToken := "non-existent-token"
		req := &http.Request{}

		credential, err := service.FinishPasskeyRegistration(ctx, nonExistentToken, req)
		assert.Nil(t, credential)
		assert.ErrorIs(t, err, ErrInvalidToken)
	})

	t.Run("returns ErrWalletAlreadyExists if wallet status is not pending", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		sessionCache := newTestSessionCache(t, dbConnectionPool)
		service, err := NewWebAuthnService(models, sessionCache)
		require.NoError(t, err)

		wallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "", data.SuccessWalletStatus)

		req := &http.Request{}

		credential, err := service.FinishPasskeyRegistration(ctx, wallet.Token, req)
		assert.Nil(t, credential)
		assert.ErrorIs(t, err, ErrWalletAlreadyExists)
	})

	t.Run("returns error if WebAuthn verification fails", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		sessionCache := newTestSessionCache(t, dbConnectionPool)
		service, err := NewWebAuthnService(models, sessionCache)
		require.NoError(t, err)

		wallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "", data.PendingWalletStatus)

		_, err = service.StartPasskeyRegistration(ctx, wallet.Token)
		require.NoError(t, err)

		req := &http.Request{
			Body: io.NopCloser(bytes.NewBufferString(`{"invalid": "data"}`)),
		}

		credential, err := service.FinishPasskeyRegistration(ctx, wallet.Token, req)
		assert.Nil(t, credential)
		assert.ErrorIs(t, err, ErrRegistrationVerificationFailed)
		assert.Contains(t, err.Error(), "parsing credential creation response")
	})

	service, serviceErr := NewWebAuthnService(models, newTestSessionCache(t, dbConnectionPool))
	require.NoError(t, serviceErr)

	startChallenge := func(t *testing.T, token string) string {
		t.Helper()
		creation, err := service.StartPasskeyRegistration(ctx, token)
		require.NoError(t, err)
		return creation.Response.Challenge.String()
	}

	t.Run("registers a passkey that is not backup-eligible", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		wallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "", data.PendingWalletStatus)
		passkey := newSoftwarePasskey(t)

		req := passkey.attestationRequest(t, startChallenge(t, wallet.Token), authenticatorFlagsUPUV)
		credential, err := service.FinishPasskeyRegistration(ctx, wallet.Token, req)
		require.NoError(t, err)
		assert.Equal(t, passkey.credentialID, base64.RawURLEncoding.EncodeToString(credential.ID))
		assert.False(t, credential.Flags.BackupEligible)
	})

	t.Run("rejects an attestation without user verification", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		wallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "", data.PendingWalletStatus)
		passkey := newSoftwarePasskey(t)

		req := passkey.attestationRequest(t, startChallenge(t, wallet.Token), authenticatorFlagUP)
		credential, err := service.FinishPasskeyRegistration(ctx, wallet.Token, req)
		assert.Nil(t, credential)
		assert.ErrorIs(t, err, ErrRegistrationVerificationFailed)
	})
}

func Test_WebAuthnService_StartPasskeyAuthentication(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	sdpUIBaseURL := "http://localhost:3000"
	testTenant := schema.Tenant{ID: "tenant-id", Name: "Test Tenant", SDPUIBaseURL: &sdpUIBaseURL}
	ctx := sdpcontext.SetTenantInContext(context.Background(), &testTenant)
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	t.Run("🎉 successfully starts discoverable passkey authentication and stores session in cache", func(t *testing.T) {
		sessionCache := newTestSessionCache(t, dbConnectionPool)
		service, err := NewWebAuthnService(models, sessionCache)
		require.NoError(t, err)

		assertion, err := service.StartPasskeyAuthentication(ctx)
		require.NoError(t, err)
		require.NotNil(t, assertion)

		assert.NotNil(t, assertion.Response)
		assert.NotEmpty(t, assertion.Response.Challenge)
		assert.Equal(t, protocol.VerificationRequired, assertion.Response.UserVerification)

		_, err = sessionCache.Get(ctx, assertion.Response.Challenge.String(), SessionTypeAuthentication)
		require.NoError(t, err)
	})
}

func Test_WebAuthnService_FinishPasskeyAuthentication(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	sdpUIBaseURL := "http://localhost:3000"
	testTenant := schema.Tenant{ID: "tenant-id", Name: "Test Tenant", SDPUIBaseURL: &sdpUIBaseURL}
	ctx := sdpcontext.SetTenantInContext(context.Background(), &testTenant)
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	sessionCache := newTestSessionCache(t, dbConnectionPool)
	service, err := NewWebAuthnService(models, sessionCache)
	require.NoError(t, err)

	t.Run("returns error if parsing request body fails", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		req := &http.Request{
			Body: io.NopCloser(bytes.NewBufferString(`invalid json`)),
		}

		retrievedWallet, credential, err := service.FinishPasskeyAuthentication(ctx, req)
		assert.Nil(t, retrievedWallet)
		assert.Nil(t, credential)
		assert.ErrorIs(t, err, ErrAuthenticationVerificationFailed)
		assert.Contains(t, err.Error(), "parsing credential request response")
	})

	startChallenge := func(t *testing.T) string {
		t.Helper()
		assertion, err := service.StartPasskeyAuthentication(ctx)
		require.NoError(t, err)
		return assertion.Response.Challenge.String()
	}

	t.Run("logs in a passkey that is not backup-eligible", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		passkey := newSoftwarePasskey(t)
		wallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", passkey.credentialID, passkey.publicKeyHex(t), data.SuccessWalletStatus)

		req := passkey.assertionRequest(t, startChallenge(t), wallet.Token, authenticatorFlagsUPUV)
		retrievedWallet, credential, err := service.FinishPasskeyAuthentication(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, wallet.Token, retrievedWallet.Token)
		assert.False(t, credential.Flags.BackupEligible)
	})

	t.Run("logs in a backup-eligible passkey", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		passkey := newSoftwarePasskey(t)
		wallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", passkey.credentialID, passkey.publicKeyHex(t), data.SuccessWalletStatus)

		req := passkey.assertionRequest(t, startChallenge(t), wallet.Token, authenticatorFlagsUPUVBEBS)
		retrievedWallet, credential, err := service.FinishPasskeyAuthentication(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, wallet.Token, retrievedWallet.Token)
		assert.True(t, credential.Flags.BackupEligible)
	})

	t.Run("rejects a signature from another key", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		passkey := newSoftwarePasskey(t)
		wallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", passkey.credentialID, passkey.publicKeyHex(t), data.SuccessWalletStatus)
		impostor := &softwarePasskey{key: newSoftwarePasskey(t).key, credentialID: passkey.credentialID}

		req := impostor.assertionRequest(t, startChallenge(t), wallet.Token, authenticatorFlagsUPUV)
		retrievedWallet, credential, err := service.FinishPasskeyAuthentication(ctx, req)
		assert.Nil(t, retrievedWallet)
		assert.Nil(t, credential)
		assert.ErrorIs(t, err, ErrAuthenticationVerificationFailed)
	})

	t.Run("rejects an unknown credential ID", func(t *testing.T) {
		passkey := newSoftwarePasskey(t)

		req := passkey.assertionRequest(t, startChallenge(t), "unknown-wallet", authenticatorFlagsUPUV)
		retrievedWallet, credential, err := service.FinishPasskeyAuthentication(ctx, req)
		assert.Nil(t, retrievedWallet)
		assert.Nil(t, credential)
		assert.ErrorIs(t, err, ErrAuthenticationVerificationFailed)
	})

	t.Run("returns ErrWalletNotReady for a pending wallet", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		passkey := newSoftwarePasskey(t)
		wallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", passkey.credentialID, passkey.publicKeyHex(t), data.PendingWalletStatus)

		req := passkey.assertionRequest(t, startChallenge(t), wallet.Token, authenticatorFlagsUPUV)
		retrievedWallet, credential, err := service.FinishPasskeyAuthentication(ctx, req)
		assert.Nil(t, retrievedWallet)
		assert.Nil(t, credential)
		assert.ErrorIs(t, err, ErrWalletNotReady)
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

		hexKey, err := COSEKeyToUncompressedHex(coseBytes)
		require.NoError(t, err)
		assert.Equal(t, hex.EncodeToString(validKey), hexKey)
	})
}
