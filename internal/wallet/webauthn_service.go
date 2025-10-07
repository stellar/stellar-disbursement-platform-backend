package wallet

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/veraison/go-cose"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

var (
	ErrEmptyToken          = errors.New("token cannot be empty")
	ErrInvalidToken        = errors.New("token is invalid")
	ErrWalletAlreadyExists = errors.New("wallet already exists for this token")
	ErrWalletNotReady      = errors.New("wallet not ready for authentication")
	ErrSessionNotFound     = errors.New("session not found or expired")
	ErrSessionTypeMismatch = errors.New("session type mismatch")
)

// WebAuthnServiceInterface defines the interface for WebAuthn passkey operations.
type WebAuthnServiceInterface interface {
	StartPasskeyRegistration(ctx context.Context, token string) (*protocol.CredentialCreation, error)
	FinishPasskeyRegistration(ctx context.Context, token string, request *http.Request) (*webauthn.Credential, error)
	StartPasskeyAuthentication(ctx context.Context) (*protocol.CredentialAssertion, error)
	FinishPasskeyAuthentication(ctx context.Context, request *http.Request) (*data.EmbeddedWallet, error)
}

var _ WebAuthnServiceInterface = (*WebAuthnService)(nil)

const (
	// DefaultSessionTTL is the default time-to-live for WebAuthn sessions.
	DefaultSessionTTL = 5 * time.Minute
)

// SessionType represents the type of WebAuthn session.
type SessionType string

const (
	SessionTypeRegistration   SessionType = "registration"
	SessionTypeAuthentication SessionType = "authentication"
)

// WebAuthnService handles WebAuthn passkey operations for embedded wallets.
type WebAuthnService struct {
	sdpModels    *data.Models
	webAuthn     *webauthn.WebAuthn
	sessionCache SessionCacheInterface
	sessionTTL   time.Duration
}

// NewWebAuthnService creates a new WebAuthnService.
func NewWebAuthnService(models *data.Models, webAuthn *webauthn.WebAuthn, sessionCache SessionCacheInterface) (*WebAuthnService, error) {
	if models == nil {
		return nil, fmt.Errorf("models cannot be nil")
	}
	if webAuthn == nil {
		return nil, fmt.Errorf("webAuthn cannot be nil")
	}
	if sessionCache == nil {
		return nil, fmt.Errorf("sessionCache cannot be nil")
	}

	return &WebAuthnService{
		sdpModels:    models,
		webAuthn:     webAuthn,
		sessionCache: sessionCache,
		sessionTTL:   DefaultSessionTTL,
	}, nil
}

type newUser struct {
	token string
}

func (u *newUser) WebAuthnID() []byte {
	return []byte(u.token)
}

func (u *newUser) WebAuthnName() string {
	return u.token
}

func (u *newUser) WebAuthnDisplayName() string {
	return "SDP Wallet User"
}

func (u *newUser) WebAuthnCredentials() []webauthn.Credential {
	return []webauthn.Credential{}
}

var _ webauthn.User = (*newUser)(nil)

// StartPasskeyRegistration initiates the WebAuthn passkey registration process.
func (w *WebAuthnService) StartPasskeyRegistration(ctx context.Context, token string) (*protocol.CredentialCreation, error) {
	if token == "" {
		return nil, ErrEmptyToken
	}

	embeddedWallet, err := w.sdpModels.EmbeddedWallets.GetByToken(ctx, w.sdpModels.DBConnectionPool, token)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("getting embedded wallet by token: %w", err)
	}

	if embeddedWallet.WalletStatus != data.PendingWalletStatus {
		return nil, ErrWalletAlreadyExists
	}

	user := &newUser{
		token: token,
	}

	opts := []webauthn.RegistrationOption{
		// Prefer resident keys (passkeys stored on device) for passwordless authentication
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementPreferred),
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			// Platform: Use device's built-in authenticator (Touch ID, Face ID, etc.)
			AuthenticatorAttachment: protocol.Platform,
			// Prefer storing credentials on the authenticator for discoverable login
			ResidentKey: protocol.ResidentKeyRequirementPreferred,
			// Required: User must verify identity (biometric or PIN)
			UserVerification: protocol.VerificationRequired,
		}),
		// Request credential properties extension to confirm resident key support
		webauthn.WithExtensions(map[string]any{"credProps": true}),
	}

	// BeginRegistration creates the challenge and parameters for the client:
	// - Generates a cryptographic challenge (random bytes) to prevent replay attacks
	// - Returns credential creation options (algorithms, attestation preferences, etc.)
	// - Client will use these to create a new credential with the authenticator
	creation, session, err := w.webAuthn.BeginRegistration(user, opts...)
	if err != nil {
		return nil, fmt.Errorf("beginning WebAuthn registration: %w", err)
	}

	if err := w.sessionCache.Store(session.Challenge, SessionTypeRegistration, *session, w.sessionTTL); err != nil {
		return nil, fmt.Errorf("storing session: %w", err)
	}

	return creation, nil
}

// FinishPasskeyRegistration completes the WebAuthn passkey registration process, creating a credential that can be used for wallet creation and authentication.
func (w *WebAuthnService) FinishPasskeyRegistration(ctx context.Context, token string, request *http.Request) (*webauthn.Credential, error) {
	if token == "" {
		return nil, ErrEmptyToken
	}

	embeddedWallet, err := w.sdpModels.EmbeddedWallets.GetByToken(ctx, w.sdpModels.DBConnectionPool, token)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("getting embedded wallet by token: %w", err)
	}

	if embeddedWallet.WalletStatus != data.PendingWalletStatus {
		return nil, ErrWalletAlreadyExists
	}

	parsedResponse, err := protocol.ParseCredentialCreationResponseBody(request.Body)
	if err != nil {
		return nil, fmt.Errorf("parsing credential creation response: %w", err)
	}

	session, err := w.sessionCache.Get(parsedResponse.Response.CollectedClientData.Challenge, SessionTypeRegistration)
	if err != nil {
		return nil, fmt.Errorf("retrieving session: %w", err)
	}

	user := &newUser{
		token: token,
	}

	// CreateCredential verifies and stores the new credential:
	// - Validates the client's response matches the challenge we sent
	// - Verifies the attestation signature
	// - Extracts and returns the public key and credential ID to use for wallet creation and authentication
	credential, err := w.webAuthn.CreateCredential(user, session, parsedResponse)
	if err != nil {
		return nil, fmt.Errorf("creating credential: %w", err)
	}

	w.sessionCache.Delete(parsedResponse.Response.CollectedClientData.Challenge)

	return credential, nil
}

type existingUser struct {
	credentialID string
	publicKey    string
	wallet       *data.EmbeddedWallet
}

func (u *existingUser) WebAuthnID() []byte {
	return []byte(u.wallet.Token)
}

func (u *existingUser) WebAuthnName() string {
	return u.wallet.Token
}

func (u *existingUser) WebAuthnDisplayName() string {
	return "SDP Wallet User"
}

func (u *existingUser) WebAuthnCredentials() []webauthn.Credential {
	credID, err := base64.RawURLEncoding.DecodeString(u.credentialID)
	if err != nil {
		return []webauthn.Credential{}
	}

	uncompressedPubKey, err := hex.DecodeString(u.publicKey)
	if err != nil {
		return []webauthn.Credential{}
	}

	coseKey, err := uncompressedECToCOSE(uncompressedPubKey)
	if err != nil {
		return []webauthn.Credential{}
	}

	return []webauthn.Credential{
		{
			ID:        credID,
			PublicKey: coseKey,
		},
	}
}

var _ webauthn.User = (*existingUser)(nil)

// StartPasskeyAuthentication initiates the WebAuthn passkey authentication process.
func (w *WebAuthnService) StartPasskeyAuthentication(ctx context.Context) (*protocol.CredentialAssertion, error) {
	opts := []webauthn.LoginOption{
		webauthn.WithUserVerification(protocol.VerificationRequired),
	}

	// BeginDiscoverableMediatedLogin starts passwordless authentication where:
	// - "Discoverable": User doesn't need to enter username, authenticator presents available credentials
	// - "Mediated": Browser shows account picker UI for user to select which credential to use
	assertion, session, err := w.webAuthn.BeginDiscoverableMediatedLogin(protocol.MediationDefault, opts...)
	if err != nil {
		return nil, fmt.Errorf("beginning WebAuthn discoverable login: %w", err)
	}

	if err := w.sessionCache.Store(session.Challenge, SessionTypeAuthentication, *session, w.sessionTTL); err != nil {
		return nil, fmt.Errorf("storing session: %w", err)
	}

	return assertion, nil
}

// FinishPasskeyAuthentication completes the WebAuthn passkey authentication process, returning the authenticated embedded wallet.
func (w *WebAuthnService) FinishPasskeyAuthentication(ctx context.Context, request *http.Request) (*data.EmbeddedWallet, error) {
	parsedResponse, err := protocol.ParseCredentialRequestResponseBody(request.Body)
	if err != nil {
		return nil, fmt.Errorf("parsing credential request response: %w", err)
	}

	session, err := w.sessionCache.Get(parsedResponse.Response.CollectedClientData.Challenge, SessionTypeAuthentication)
	if err != nil {
		return nil, fmt.Errorf("retrieving session: %w", err)
	}

	// Handler to look up user by credential ID during authentication
	getUserForCredential := func(rawID, userHandle []byte) (webauthn.User, error) {
		credentialID := base64.RawURLEncoding.EncodeToString(rawID)
		embeddedWallet, getErr := w.sdpModels.EmbeddedWallets.GetByCredentialID(ctx, w.sdpModels.DBConnectionPool, credentialID)
		if getErr != nil {
			return nil, fmt.Errorf("getting embedded wallet by credential ID: %w", getErr)
		}

		if embeddedWallet.WalletStatus != data.SuccessWalletStatus {
			return nil, ErrWalletNotReady
		}

		return &existingUser{
			credentialID: embeddedWallet.CredentialID,
			publicKey:    embeddedWallet.PublicKey,
			wallet:       embeddedWallet,
		}, nil
	}

	// ValidatePasskeyLogin verifies the authentication response from the client:
	// 1. Calls getUserForCredential handler to look up the user and their public key
	// 2. Validates the cryptographic signature using the stored public key
	// 3. Returns the authenticated user if signature verification succeeds
	user, _, err := w.webAuthn.ValidatePasskeyLogin(getUserForCredential, session, parsedResponse)
	if err != nil {
		return nil, fmt.Errorf("validating WebAuthn passkey login: %w", err)
	}

	existingUserObj, ok := user.(*existingUser)
	if !ok {
		return nil, fmt.Errorf("unexpected user type: got %T, want *existingUser", user)
	}

	w.sessionCache.Delete(parsedResponse.Response.CollectedClientData.Challenge)

	return existingUserObj.wallet, nil
}

const (
	UncompressedKeyLength = 65
	ECPointXStart         = 1
	ECPointXEnd           = 33
	ECPointYEnd           = 65
)

// uncompressedECToCOSE converts an uncompressed EC public key to COSE format.
func uncompressedECToCOSE(uncompressed []byte) ([]byte, error) {
	if len(uncompressed) != UncompressedKeyLength {
		return nil, fmt.Errorf("invalid uncompressed key length (expected %d, got %d)", UncompressedKeyLength, len(uncompressed))
	}

	if uncompressed[0] != 0x04 {
		return nil, fmt.Errorf("invalid uncompressed key format (expected 0x04 prefix, got 0x%02x)", uncompressed[0])
	}

	x := uncompressed[ECPointXStart:ECPointXEnd]
	y := uncompressed[ECPointXEnd:ECPointYEnd]

	key, err := cose.NewKeyEC2(cose.AlgorithmES256, x, y, nil)
	if err != nil {
		return nil, fmt.Errorf("creating COSE key: %w", err)
	}

	coseBytes, err := key.MarshalCBOR()
	if err != nil {
		return nil, fmt.Errorf("marshaling COSE key: %w", err)
	}

	return coseBytes, nil
}
