package wallet

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/veraison/go-cose"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
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
//
//go:generate mockery --name=WebAuthnServiceInterface --case=underscore --structname=MockWebAuthnService --filename=webauthn_service.go
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
	RPDisplayName     = "Stellar Disbursement Platform"
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
	sessionCache SessionCacheInterface
	sessionTTL   time.Duration
}

// NewWebAuthnService creates a new WebAuthnService.
func NewWebAuthnService(models *data.Models, sessionCache SessionCacheInterface) (*WebAuthnService, error) {
	if models == nil {
		return nil, fmt.Errorf("models cannot be nil")
	}
	if sessionCache == nil {
		return nil, fmt.Errorf("sessionCache cannot be nil")
	}

	return &WebAuthnService{
		sdpModels:    models,
		sessionCache: sessionCache,
		sessionTTL:   DefaultSessionTTL,
	}, nil
}

// createWebAuthn creates a WebAuthn instance configured with the tenant's origin and RPID.
func (w *WebAuthnService) createWebAuthn(ctx context.Context) (*webauthn.WebAuthn, error) {
	tenant, err := sdpcontext.GetTenantFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting tenant from context: %w", err)
	}

	if tenant.SDPUIBaseURL == nil || *tenant.SDPUIBaseURL == "" {
		return nil, fmt.Errorf("tenant SDPUIBaseURL is not configured")
	}

	origin := *tenant.SDPUIBaseURL
	rpID, err := extractRPIDFromOrigin(origin)
	if err != nil {
		return nil, fmt.Errorf("extracting RPID from origin: %w", err)
	}

	return webauthn.New(&webauthn.Config{
		RPDisplayName: RPDisplayName,
		RPID:          rpID,
		RPOrigins:     []string{origin},
	})
}

// extractRPIDFromOrigin extracts the Relying Party ID from a given origin URL.
func extractRPIDFromOrigin(origin string) (string, error) {
	parsedURL, err := url.Parse(origin)
	if err != nil {
		return "", fmt.Errorf("parsing origin URL: %w", err)
	}

	hostname := parsedURL.Hostname()
	if hostname == "" {
		return "", fmt.Errorf("empty hostname in origin URL")
	}

	return hostname, nil
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

	webAuthn, err := w.createWebAuthn(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating WebAuthn instance: %w", err)
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
	creation, session, err := webAuthn.BeginRegistration(user, opts...)
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

	webAuthn, err := w.createWebAuthn(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating WebAuthn instance: %w", err)
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
	credential, err := webAuthn.CreateCredential(user, session, parsedResponse)
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
			// We don't persist the backup state when we create the passkey
			// and by default the WebAuthn library sets both flags to false,
			// causing the authentication to fail.
			Flags: webauthn.CredentialFlags{
				BackupEligible: true,
				BackupState:    true,
			},
		},
	}
}

var _ webauthn.User = (*existingUser)(nil)

// StartPasskeyAuthentication initiates the WebAuthn passkey authentication process.
func (w *WebAuthnService) StartPasskeyAuthentication(ctx context.Context) (*protocol.CredentialAssertion, error) {
	webAuthn, err := w.createWebAuthn(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating WebAuthn instance: %w", err)
	}

	opts := []webauthn.LoginOption{
		webauthn.WithUserVerification(protocol.VerificationRequired),
	}

	// BeginDiscoverableMediatedLogin starts passwordless authentication where:
	// - "Discoverable": User doesn't need to enter username, authenticator presents available credentials
	// - "Mediated": Browser shows account picker UI for user to select which credential to use
	assertion, session, err := webAuthn.BeginDiscoverableMediatedLogin(protocol.MediationDefault, opts...)
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
	webAuthn, err := w.createWebAuthn(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating WebAuthn instance: %w", err)
	}

	parsedResponse, err := protocol.ParseCredentialRequestResponseBody(request.Body)
	if err != nil {
		return nil, fmt.Errorf("parsing credential request response: %w", err)
	}

	session, err := w.sessionCache.Get(parsedResponse.Response.CollectedClientData.Challenge, SessionTypeAuthentication)
	if err != nil {
		return nil, fmt.Errorf("retrieving session: %w", err)
	}

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
	user, _, err := webAuthn.ValidatePasskeyLogin(getUserForCredential, session, parsedResponse)
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

// COSEKeyToUncompressedHex converts a COSE-encoded public key to an uncompressed hex string.
func COSEKeyToUncompressedHex(coseBytes []byte) (string, error) {
	var rawKey map[int]interface{}
	if err := cbor.Unmarshal(coseBytes, &rawKey); err != nil {
		return "", fmt.Errorf("unmarshaling COSE key: %w", err)
	}

	xCoord, ok := rawKey[-2].([]byte)
	if !ok || len(xCoord) != 32 {
		return "", fmt.Errorf("invalid X coordinate in COSE key")
	}

	yCoord, ok := rawKey[-3].([]byte)
	if !ok || len(yCoord) != 32 {
		return "", fmt.Errorf("invalid Y coordinate in COSE key")
	}

	uncompressed := make([]byte, UncompressedKeyLength)
	uncompressed[0] = 0x04
	copy(uncompressed[ECPointXStart:ECPointXEnd], xCoord)
	copy(uncompressed[ECPointXEnd:ECPointYEnd], yCoord)

	return hex.EncodeToString(uncompressed), nil
}
