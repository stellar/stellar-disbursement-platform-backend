package serve

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	supporthttp "github.com/stellar/go-stellar-sdk/support/http"
	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/bridge"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sepauth"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httphandler"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/publicfiles"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/sep24frontend"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/wallet"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	authUtils "github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

const ServiceID = "serve"

type HTTPServerInterface interface {
	Run(conf supporthttp.Config)
}

type HTTPServer struct{}

func (h *HTTPServer) Run(conf supporthttp.Config) {
	supporthttp.Run(conf)
}

type ServeOptions struct {
	Environment                    string
	GitCommit                      string
	Port                           int
	Version                        string
	InstanceName                   string
	MonitorService                 monitor.MonitorServiceInterface
	MtnDBConnectionPool            db.DBConnectionPool
	AdminDBConnectionPool          db.DBConnectionPool
	TSSDBConnectionPool            db.DBConnectionPool
	EC256PrivateKey                string
	Models                         *data.Models
	CorsAllowedOrigins             []string
	authManager                    auth.AuthManager
	EmailMessengerClient           message.MessengerClient
	MessageDispatcher              message.MessageDispatcherInterface
	SEP24JWTSecret                 string
	sep24JWTManager                *sepauth.JWTManager
	BaseURL                        string
	ResetTokenExpirationHours      int
	NetworkPassphrase              string
	NetworkType                    utils.NetworkType
	SubmitterEngine                engine.SubmitterEngine
	Sep10SigningPublicKey          string
	Sep10SigningPrivateKey         string
	Sep10ClientAttributionRequired bool
	Sep10Service                   services.SEP10Service
	Sep45Service                   services.SEP45Service
	EnableEmbeddedWallets          bool
	EmbeddedWalletsWasmHash        string
	EnableSep45                    bool
	Sep45ContractID                string
	RPCConfig                      stellar.RPCOptions
	CrashTrackerClient             crashtracker.CrashTrackerClient
	ReCAPTCHASiteKey               string
	ReCAPTCHASiteSecretKey         string
	CAPTCHAType                    validators.CAPTCHAType
	ReCAPTCHAV3MinScore            float64
	DisableMFA                     bool
	DisableReCAPTCHA               bool
	PasswordValidator              *authUtils.PasswordValidator

	tenantManager               tenant.ManagerInterface
	DistributionAccountService  services.DistributionAccountServiceInterface
	DistAccEncryptionPassphrase string

	MaxInvitationResendAttempts int
	SingleTenantMode            bool
	CircleService               circle.ServiceInterface
	CircleAPIType               circle.APIType
	BridgeService               bridge.ServiceInterface

	EmbeddedWalletService     services.EmbeddedWalletServiceInterface
	WebAuthnSessionTTLSeconds int
	WebAuthnService           wallet.WebAuthnServiceInterface
	walletJWTManager          wallet.WalletJWTManager
}

// SetupDependencies uses the serve options to setup the dependencies for the server.
func (opts *ServeOptions) SetupDependencies() error {
	// Setup crash tracker:
	// Call crash tracker FlushEvents to flush buffered events before the server terminates
	defer opts.CrashTrackerClient.FlushEvents(2 * time.Second)
	// Call crash tracker Recover for recover from unhandled panics
	defer opts.CrashTrackerClient.Recover()
	// Set crash tracker LogAndReportErrors as DefaultReportErrorFunc
	httperror.SetDefaultReportErrorFunc(opts.CrashTrackerClient.LogAndReportErrors)

	// Setup Multi-Tenant Database when enabled
	opts.tenantManager = tenant.NewManager(
		tenant.WithDatabase(opts.AdminDBConnectionPool),
		tenant.WithSingleTenantMode(opts.SingleTenantMode),
	)

	var err error
	opts.Models, err = data.NewModels(opts.MtnDBConnectionPool)
	if err != nil {
		return fmt.Errorf("error creating models for Serve: %w", err)
	}

	// Setup Stellar Auth JWT manager
	opts.authManager, err = createAuthManager(
		opts.MtnDBConnectionPool, opts.EC256PrivateKey, opts.ResetTokenExpirationHours,
	)
	if err != nil {
		return fmt.Errorf("error creating Stellar Auth manager: %w", err)
	}

	// Setup Wallet JWT Manager for passkey authentication
	opts.walletJWTManager, err = wallet.NewWalletJWTManager(opts.EC256PrivateKey)
	if err != nil {
		return fmt.Errorf("error creating wallet JWT manager: %w", err)
	}

	// Setup SEP24 JWT manager
	sep24JWTManager, err := sepauth.NewJWTManager(opts.SEP24JWTSecret, 300000)
	if err != nil {
		return fmt.Errorf("error creating SEP-24 JWT manager: %w", err)
	}
	opts.sep24JWTManager = sep24JWTManager

	opts.PasswordValidator, err = authUtils.GetPasswordValidatorInstance()
	if err != nil {
		return fmt.Errorf("error initializing password validator: %w", err)
	}

	// Determine allow retry based on network passphrase
	allowHTTPRetry := opts.NetworkPassphrase != network.PublicNetworkPassphrase

	sep10NonceStore, err := services.NewNonceStore(opts.MtnDBConnectionPool, services.DefaultSEP10NonceExpiration)
	if err != nil {
		return fmt.Errorf("initializing SEP 10 nonce store: %w", err)
	}
	sep10Service, err := services.NewSEP10Service(
		sep24JWTManager,
		opts.NetworkPassphrase,
		opts.Sep10SigningPrivateKey,
		opts.BaseURL,
		allowHTTPRetry,
		opts.SubmitterEngine.HorizonClient,
		opts.Sep10ClientAttributionRequired,
		sep10NonceStore,
	)
	if err != nil {
		return fmt.Errorf("initializing SEP 10 Service: %w", err)
	}

	opts.Sep10Service = sep10Service

	if opts.EnableSep45 {
		sep45NonceStore, err := services.NewNonceStore(opts.MtnDBConnectionPool, services.DefaultSEP45NonceExpiration)
		if err != nil {
			return fmt.Errorf("initializing SEP 45 nonce store: %w", err)
		}
		rpcClient, rpcErr := dependencyinjection.NewRPCClient(context.Background(), opts.RPCConfig)
		if rpcErr != nil {
			return fmt.Errorf("initializing RPC client: %w", rpcErr)
		}

		signingKP, kpErr := keypair.ParseFull(opts.Sep10SigningPrivateKey)
		if kpErr != nil {
			return fmt.Errorf("parsing SEP-45 signing key: %w", kpErr)
		}

		sep45Service, sep45Err := services.NewSEP45Service(services.SEP45ServiceOptions{
			RPCClient:               rpcClient,
			TOMLClient:              nil,
			JWTManager:              sep24JWTManager,
			NetworkPassphrase:       opts.NetworkPassphrase,
			WebAuthVerifyContractID: opts.Sep45ContractID,
			ServerSigningKeypair:    signingKP,
			BaseURL:                 opts.BaseURL,
			AllowHTTPRetry:          allowHTTPRetry,
			NonceStore:              sep45NonceStore,
		})
		if sep45Err != nil {
			return fmt.Errorf("initializing SEP 45 Service: %w", sep45Err)
		}

		opts.Sep45Service = sep45Service
	}

	return nil
}

// ValidateSecurity validates the MFA and ReCAPTCHA security options.
func (opts *ServeOptions) ValidateSecurity() error {
	if opts.NetworkPassphrase == network.PublicNetworkPassphrase {
		if opts.DisableMFA {
			return fmt.Errorf("MFA cannot be disabled in pubnet")
		} else if opts.DisableReCAPTCHA {
			log.Warnf("reCAPTCHA is disabled in pubnet. This might reduce security!")
		}
	}

	if opts.DisableMFA {
		log.Warnf("MFA is disabled in network '%s'", opts.NetworkPassphrase)
	}
	if opts.DisableReCAPTCHA {
		log.Warnf("reCAPTCHA is disabled in network '%s'", opts.NetworkPassphrase)
	}

	return nil
}

// ValidateRPC validates the RPC options.
func (opts *ServeOptions) ValidateRPC() error {
	if opts.RPCConfig.RPCUrl == "" && (opts.RPCConfig.RPCRequestAuthHeaderKey != "" || opts.RPCConfig.RPCRequestAuthHeaderValue != "") {
		return fmt.Errorf("RPC URL must be set when RPC request header key or value is set")
	}

	if opts.RPCConfig.RPCRequestAuthHeaderKey != "" && opts.RPCConfig.RPCRequestAuthHeaderValue == "" {
		return fmt.Errorf("RPC request header value must be set when RPC request header key is set")
	}

	if opts.RPCConfig.RPCRequestAuthHeaderKey == "" && opts.RPCConfig.RPCRequestAuthHeaderValue != "" {
		return fmt.Errorf("RPC request header key must be set when RPC request header value is set")
	}

	// RPC-dependent feature validation
	hasRPCFeatures := opts.EnableEmbeddedWallets || opts.EnableSep45
	if hasRPCFeatures && opts.RPCConfig.RPCUrl == "" {
		return fmt.Errorf("RPC URL must be set when RPC-dependent features are enabled")
	}

	// Embedded wallet feature validation
	if opts.EnableEmbeddedWallets && opts.EmbeddedWalletsWasmHash == "" {
		return fmt.Errorf("embedded wallets WASM hash must be set when embedded wallets are enabled")
	}

	// SEP-45 feature validation
	if opts.EnableSep45 && opts.Sep45ContractID == "" {
		return fmt.Errorf("SEP-45 contract ID must be set when SEP-45 is enabled")
	}

	return nil
}

func Serve(opts ServeOptions, httpServer HTTPServerInterface) error {
	if err := opts.ValidateSecurity(); err != nil {
		return fmt.Errorf("validating security options: %w", err)
	}

	if err := opts.ValidateRPC(); err != nil {
		return fmt.Errorf("validating RPC options: %w", err)
	}

	if err := opts.SetupDependencies(); err != nil {
		return fmt.Errorf("starting dependencies: %w", err)
	}

	// Start the server
	listenAddr := fmt.Sprintf(":%d", opts.Port)
	serverConfig := supporthttp.Config{
		ListenAddr:          listenAddr,
		Handler:             handleHTTP(opts),
		TCPKeepAlive:        time.Minute * 3,
		ShutdownGracePeriod: time.Second * 50,
		ReadTimeout:         time.Second * 5,
		WriteTimeout:        time.Second * 35,
		IdleTimeout:         time.Minute * 2,
		OnStarting: func() {
			log.Info("Starting SDP (Stellar Disbursement Platform) Server")
			log.Infof("Listening on %s", listenAddr)
		},
		OnStopping: func() {
			log.Info("Closing the SDP Server database connection pool")
			err := db.CloseConnectionPoolIfNeeded(context.Background(), opts.MtnDBConnectionPool)
			if err != nil {
				log.Errorf("error closing database connection: %v", err)
			}

			log.Info("Stopping SDP (Stellar Disbursement Platform) Server")
		},
	}
	httpServer.Run(serverConfig)
	return nil
}

const (
	rateLimitPer20Seconds = 40
	rateLimitWindow       = 20 * time.Second
)

func handleHTTP(o ServeOptions) *chi.Mux {
	mux := chi.NewMux()

	// Middleware
	mux.Use(middleware.CorsMiddleware(o.CorsAllowedOrigins))
	// Rate limits requests made with the pair <IP, endpoint>.
	mux.Use(httprate.Limit(
		rateLimitPer20Seconds,
		rateLimitWindow,
		httprate.WithKeyFuncs(httprate.KeyByIP, httprate.KeyByEndpoint),
	))
	mux.Use(chimiddleware.RequestID)
	mux.Use(middleware.ResolveTenantFromRequestMiddleware(o.tenantManager, o.SingleTenantMode))
	mux.Use(middleware.LoggingMiddleware)
	mux.Use(middleware.RecoverHandler)
	mux.Use(middleware.MetricsRequestHandler(o.MonitorService))
	mux.Use(middleware.CSPMiddleware())
	mux.Use(chimiddleware.CleanPath)
	mux.Use(chimiddleware.Compress(5))

	// Create a route along /static that will serve contents from the ./public_files folder.
	staticFileServer(mux, publicfiles.PublicFiles)

	// Authenticated Routes
	authManager := o.authManager
	mux.Group(func(r chi.Router) {
		r.Use(middleware.APIKeyOrJWTAuthenticate(o.Models.APIKeys, middleware.AuthenticateMiddleware(authManager, o.tenantManager)))
		r.Use(middleware.EnsureTenantMiddleware)

		// API Key management endpoints
		r.With(middleware.RequirePermission(
			data.WriteAll,
			middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.DeveloperUserRole),
		)).Route("/api-keys", func(r chi.Router) {
			apiKeyHandler := httphandler.APIKeyHandler{
				Models: o.Models,
			}
			r.Get("/{id}", apiKeyHandler.GetAPIKeyByID)
			r.Get("/", apiKeyHandler.GetAllAPIKeys)
			r.Post("/", apiKeyHandler.CreateAPIKey)
			r.Patch("/{id}", apiKeyHandler.UpdateKey)
			r.Delete("/{id}", apiKeyHandler.DeleteAPIKey)
		})

		// Statistics endpoints
		r.With(middleware.RequirePermission(
			data.ReadStatistics,
			middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...),
		)).Route("/statistics", func(r chi.Router) {
			h := httphandler.StatisticsHandler{DBConnectionPool: o.MtnDBConnectionPool}
			r.Get("/", h.GetStatistics)
			r.Get("/{id}", h.GetStatisticsByDisbursement)
		})

		// User management endpoints
		r.Route("/users", func(r chi.Router) {
			userHandler := httphandler.UserHandler{
				AuthManager:        authManager,
				CrashTrackerClient: o.CrashTrackerClient,
				MessengerClient:    o.EmailMessengerClient,
				Models:             o.Models,
			}

			r.With(middleware.RequirePermission(
				data.ReadUsers,
				middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole),
			)).Group(func(r chi.Router) {
				r.Get("/", userHandler.GetAllUsers)
				r.Get("/roles", httphandler.ListRolesHandler{}.GetRoles)
			})

			r.With(middleware.RequirePermission(
				data.WriteUsers,
				middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole),
			)).Group(func(r chi.Router) {
				r.Post("/", userHandler.CreateUser)
				r.Patch("/roles", userHandler.UpdateUserRoles)
				r.Patch("/activation", userHandler.UserActivation)
			})
		})
		r.With(middleware.RequirePermission(
			data.ReadAll,
			middleware.AnyRoleMiddleware(authManager),
		)).Post("/refresh-token", httphandler.RefreshTokenHandler{AuthManager: authManager}.PostRefreshToken)

		// Disbursement endpoints
		r.Route("/disbursements", func(r chi.Router) {
			handler := httphandler.DisbursementHandler{
				Models:                      o.Models,
				AuthManager:                 authManager,
				MonitorService:              o.MonitorService,
				DistributionAccountResolver: o.SubmitterEngine.DistributionAccountResolver,
				DisbursementManagementService: &services.DisbursementManagementService{
					Models:                     o.Models,
					AuthManager:                authManager,
					CrashTrackerClient:         o.CrashTrackerClient,
					DistributionAccountService: o.DistributionAccountService,
				},
			}

			// Group all READ operations
			r.With(middleware.RequirePermission(
				data.ReadDisbursements,
				middleware.AnyRoleMiddleware(authManager, data.GetBusinessOperationRoles()...),
			)).Group(func(r chi.Router) {
				r.Get("/", handler.GetDisbursements)
				r.Get("/{id}", handler.GetDisbursement)
				r.Get("/{id}/receivers", handler.GetDisbursementReceivers)
				r.Get("/{id}/instructions", handler.GetDisbursementInstructions)
			})

			// Group CREATE/EDIT operations (accessible to initiators)
			r.With(middleware.RequirePermission(
				data.WriteDisbursements,
				middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole, data.InitiatorUserRole),
			)).Group(func(r chi.Router) {
				r.Post("/", handler.PostDisbursement)
				r.Delete("/{id}", handler.DeleteDisbursement)
				r.Post("/{id}/instructions", handler.PostDisbursementInstructions)
			})

			// Group STATUS operations (accessible to approvers)
			r.With(middleware.RequirePermission(
				data.WriteDisbursements,
				middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole, data.ApproverUserRole),
			)).Group(func(r chi.Router) {
				r.Patch("/{id}/status", handler.PatchDisbursementStatus)
			})
		})

		// Payment endpoints
		r.Route("/payments", func(r chi.Router) {
			paymentsHandler := httphandler.PaymentsHandler{
				Models:                      o.Models,
				DBConnectionPool:            o.MtnDBConnectionPool,
				AuthManager:                 o.authManager,
				CrashTrackerClient:          o.CrashTrackerClient,
				DistributionAccountResolver: o.SubmitterEngine.DistributionAccountResolver,
				DirectPaymentService: services.NewDirectPaymentService(
					o.Models,
					o.DistributionAccountService,
					o.SubmitterEngine,
				),
			}

			// Read operations
			r.With(middleware.RequirePermission(
				data.ReadPayments,
				middleware.AnyRoleMiddleware(authManager, data.GetBusinessOperationRoles()...),
			)).Group(func(r chi.Router) {
				r.Get("/", paymentsHandler.GetPayments)
				r.Get("/{id}", paymentsHandler.GetPayment)
			})

			// Write operations with different role permissions
			r.With(middleware.RequirePermission(
				data.WritePayments,
				middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole, data.BusinessUserRole),
			)).Group(func(r chi.Router) {
				r.Post("/", paymentsHandler.PostDirectPayment)
				r.Patch("/retry", paymentsHandler.RetryPayments)
			})

			r.With(middleware.RequirePermission(
				data.WritePayments,
				middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole),
			)).Patch("/{id}/status", paymentsHandler.PatchPaymentStatus)
		})

		// Receiver endpoints
		r.Route("/receivers", func(r chi.Router) {
			receiversHandler := httphandler.ReceiverHandler{Models: o.Models, DBConnectionPool: o.MtnDBConnectionPool}

			// Read operations
			r.With(middleware.RequirePermission(
				data.ReadReceivers,
				middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...),
			)).Get("/verification-types", receiversHandler.GetReceiverVerificationTypes)

			r.With(middleware.RequirePermission(
				data.ReadReceivers,
				middleware.AnyRoleMiddleware(authManager, data.GetBusinessOperationRoles()...),
			)).Group(func(r chi.Router) {
				r.Get("/", receiversHandler.GetReceivers)
				r.Get("/{id}", receiversHandler.GetReceiver)
			})

			// Write operations
			updateReceiverHandler := httphandler.UpdateReceiverHandler{
				Models:           o.Models,
				DBConnectionPool: o.MtnDBConnectionPool,
				AuthManager:      authManager,
			}
			receiverWalletHandler := httphandler.ReceiverWalletsHandler{
				Models:             o.Models,
				CrashTrackerClient: o.CrashTrackerClient,
			}

			r.With(middleware.RequirePermission(
				data.WriteReceivers,
				middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole, data.ApproverUserRole, data.InitiatorUserRole),
			)).Group(func(r chi.Router) {
				r.Post("/", receiversHandler.CreateReceiver)
				r.Patch("/{id}", updateReceiverHandler.UpdateReceiver)
				r.Patch("/{receiver_id}/wallets/{receiver_wallet_id}", receiverWalletHandler.PatchReceiverWallet)
				r.Patch("/wallets/{receiver_wallet_id}", receiverWalletHandler.RetryInvitation)
				r.Patch("/wallets/{receiver_wallet_id}/status", receiverWalletHandler.PatchReceiverWalletStatus)
			})
		})

		r.With(middleware.RequirePermission(
			data.ReadAll,
			middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...),
		)).Get("/registration-contact-types", httphandler.RegistrationContactTypesHandler{}.Get)

		r.Route("/assets", func(r chi.Router) {
			assetsHandler := httphandler.AssetsHandler{
				Models:                     o.Models,
				SubmitterEngine:            o.SubmitterEngine,
				DistributionAccountService: o.DistributionAccountService,
			}

			// Read operations
			r.With(middleware.RequirePermission(
				data.ReadAll,
				middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...),
			)).Get("/", assetsHandler.GetAssets)

			// Write operations
			r.With(middleware.RequirePermission(
				data.WriteAll,
				middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole, data.DeveloperUserRole),
			)).Group(func(r chi.Router) {
				r.Post("/", assetsHandler.CreateAsset)
				r.Delete("/{id}", assetsHandler.DeleteAsset)
			})
		})

		r.Route("/wallets", func(r chi.Router) {
			walletsHandler := httphandler.WalletsHandler{
				Models:                o.Models,
				NetworkType:           o.NetworkType,
				WalletAssetResolver:   services.NewWalletAssetResolver(o.Models.Assets),
				EnableEmbeddedWallets: o.EnableEmbeddedWallets,
			}

			// Read operations
			r.With(middleware.RequirePermission(
				data.ReadWallets,
				middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...),
			)).Get("/", walletsHandler.GetWallets)

			// Write operations
			r.With(middleware.RequirePermission(
				data.WriteWallets,
				middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.DeveloperUserRole),
			)).Group(func(r chi.Router) {
				r.Post("/", walletsHandler.PostWallets)
				r.Delete("/{id}", walletsHandler.DeleteWallet)
			})

			r.With(middleware.RequirePermission(
				data.WriteWallets,
				middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.DeveloperUserRole),
			)).Patch("/{id}", walletsHandler.PatchWallets)
		})

		profileHandler := httphandler.ProfileHandler{
			Models:                      o.Models,
			AuthManager:                 authManager,
			MaxMemoryAllocation:         httphandler.DefaultMaxMemoryAllocation,
			DistributionAccountResolver: o.SubmitterEngine.DistributionAccountResolver,
			PasswordValidator:           o.PasswordValidator,
			NetworkType:                 o.NetworkType,
		}
		r.Route("/profile", func(r chi.Router) {
			// Read operations
			r.With(middleware.RequirePermission(
				data.ReadAll,
				middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...),
			)).Get("/", profileHandler.GetProfile)

			// Write operations
			r.With(middleware.RequirePermission(
				data.WriteAll,
				middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...),
			)).Group(func(r chi.Router) {
				r.Patch("/", profileHandler.PatchUserProfile)
				r.Patch("/reset-password", profileHandler.PatchUserPassword)
			})
		})

		r.Route("/organization", func(r chi.Router) {
			// Read operations
			r.With(middleware.RequirePermission(
				data.ReadOrganization,
				middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...),
			)).Get("/", profileHandler.GetOrganizationInfo)

			// Write operations with different role permissions
			r.With(middleware.RequirePermission(
				data.WriteOrganization,
				middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole),
			)).Patch("/", profileHandler.PatchOrganizationProfile)

			r.With(middleware.RequirePermission(
				data.WriteOrganization,
				middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole),
			)).Patch("/circle-config", httphandler.CircleConfigHandler{
				NetworkType:                 o.NetworkType,
				CircleFactory:               circle.NewClient,
				TenantManager:               o.tenantManager,
				Encrypter:                   &utils.DefaultPrivateKeyEncrypter{},
				EncryptionPassphrase:        o.DistAccEncryptionPassphrase,
				CircleClientConfigModel:     circle.NewClientConfigModel(o.MtnDBConnectionPool),
				DistributionAccountResolver: o.SubmitterEngine.DistributionAccountResolver,
				MonitorService:              o.MonitorService,
			}.Patch)
		})

		// Bridge integration endpoints
		if o.BridgeService != nil {
			r.Route("/bridge-integration", func(r chi.Router) {
				bridgeIntegrationHandler := httphandler.BridgeIntegrationHandler{
					BridgeService:               o.BridgeService,
					AuthManager:                 authManager,
					Models:                      o.Models,
					DistributionAccountResolver: o.SubmitterEngine.DistributionAccountResolver,
				}

				r.With(middleware.RequirePermission(
					data.ReadOrganization,
					middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...),
				)).Get("/", bridgeIntegrationHandler.Get)

				r.With(middleware.RequirePermission(
					data.WriteOrganization,
					middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole),
				)).Patch("/", bridgeIntegrationHandler.Patch)
			})
		}

		r.With(middleware.RequirePermission(
			data.ReadAll,
			middleware.AnyRoleMiddleware(authManager),
		)).Get("/balances", httphandler.BalancesHandler{
			DistributionAccountResolver: o.SubmitterEngine.DistributionAccountResolver,
			CircleService:               o.CircleService,
			NetworkType:                 o.NetworkType,
		}.Get)

		exportHandler := httphandler.ExportHandler{
			Models: o.Models,
		}
		r.With(middleware.RequirePermission(
			data.ReadExports,
			middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole, data.ApproverUserRole, data.InitiatorUserRole),
		)).Route("/exports", func(r chi.Router) {
			r.Get("/disbursements", exportHandler.ExportDisbursements)
			r.Get("/payments", exportHandler.ExportPayments)
			r.Get("/receivers", exportHandler.ExportReceivers)
		})
	})

	captchaFactory := validators.NewCAPTCHAValidatorFactory()
	reCAPTCHAValidator, err := captchaFactory.CreateValidator(o.CAPTCHAType, o.ReCAPTCHASiteSecretKey, o.ReCAPTCHAV3MinScore)
	if err != nil {
		log.Errorf("Error creating CAPTCHA validator: %v. Falling back to reCAPTCHA v2.", err)
		reCAPTCHAValidator = validators.NewGoogleReCAPTCHAValidator(o.ReCAPTCHASiteSecretKey, httpclient.DefaultClient())
	}

	// Public routes that are tenant aware (they need to know the tenant ID)
	mux.Group(func(r chi.Router) {
		r.Use(middleware.EnsureTenantMiddleware)

		r.Get("/app-config", httphandler.AppConfigHandler{
			Models:            o.Models,
			CAPTCHAType:       o.CAPTCHAType,
			ReCAPTCHASiteKey:  o.ReCAPTCHASiteKey,
			ReCAPTCHADisabled: o.DisableReCAPTCHA,
		}.ServeHTTP)

		r.Get("/organization/logo", httphandler.OrganizationLogoHandler{
			Models:        o.Models,
			PublicFilesFS: publicfiles.PublicFiles,
		}.GetOrganizationLogo)

		r.Post("/login", httphandler.LoginHandler{
			AuthManager:        authManager,
			ReCAPTCHAValidator: reCAPTCHAValidator,
			MessengerClient:    o.EmailMessengerClient,
			Models:             o.Models,
			ReCAPTCHADisabled:  o.DisableReCAPTCHA,
			MFADisabled:        o.DisableMFA,
		}.ServeHTTP)
		r.Post("/mfa", httphandler.MFAHandler{
			AuthManager:        authManager,
			ReCAPTCHAValidator: reCAPTCHAValidator,
			Models:             o.Models,
			ReCAPTCHADisabled:  o.DisableReCAPTCHA,
		}.ServeHTTP)
		r.Post("/forgot-password", httphandler.ForgotPasswordHandler{
			AuthManager:        authManager,
			MessengerClient:    o.EmailMessengerClient,
			Models:             o.Models,
			ReCAPTCHAValidator: reCAPTCHAValidator,
			ReCAPTCHADisabled:  o.DisableReCAPTCHA,
		}.ServeHTTP)
		r.Post("/reset-password", httphandler.ResetPasswordHandler{
			AuthManager:       authManager,
			PasswordValidator: o.PasswordValidator,
		}.ServeHTTP)

		r.Get("/r/{code}", httphandler.URLShortenerHandler{Models: o.Models}.HandleRedirect)

		// Embedded wallet routes (only if feature is enabled)
		if o.EnableEmbeddedWallets && o.EmbeddedWalletService != nil {
			mux.Group(func(r chi.Router) {
				walletCreationHandler := httphandler.WalletCreationHandler{
					EmbeddedWalletService: o.EmbeddedWalletService,
				}
				embeddedWalletProfileHandler := httphandler.EmbeddedWalletProfileHandler{
					EmbeddedWalletService: o.EmbeddedWalletService,
					Models:                o.Models,
				}
				sponsoredTransactionHandler := httphandler.SponsoredTransactionHandler{
					EmbeddedWalletService: o.EmbeddedWalletService,
					Models:                o.Models,
					NetworkPassphrase:     o.NetworkPassphrase,
				}
				passkeyHandler := &httphandler.PasskeyHandler{
					WebAuthnService:       o.WebAuthnService,
					WalletJWTManager:      o.walletJWTManager,
					EmbeddedWalletService: o.EmbeddedWalletService,
				}

				r.Route("/embedded-wallets", func(r chi.Router) {
					// Wallet creation routes
					r.Post("/", walletCreationHandler.CreateWallet)
					r.Get("/{credentialID}", walletCreationHandler.GetWallet)

					// Authenticated wallet routes
					r.With(middleware.WalletAuthMiddleware(o.walletJWTManager)).Group(func(r chi.Router) {
						// Profile routes
						r.Get("/profile", embeddedWalletProfileHandler.GetProfile)

						// Sponsored transactions
						r.Route("/sponsored-transactions", func(r chi.Router) {
							r.Post("/", sponsoredTransactionHandler.CreateSponsoredTransaction)
							r.Get("/{id}", sponsoredTransactionHandler.GetSponsoredTransaction)
						})
					})

					// Passkey registration + authentication routes
					if passkeyHandler != nil {
						r.Route("/passkey", func(r chi.Router) {
							r.Route("/registration", func(r chi.Router) {
								r.Post("/start", passkeyHandler.StartPasskeyRegistration)
								r.Post("/finish", passkeyHandler.FinishPasskeyRegistration)
							})
							r.Route("/authentication", func(r chi.Router) {
								r.Post("/start", passkeyHandler.StartPasskeyAuthentication)
								r.Post("/finish", passkeyHandler.FinishPasskeyAuthentication)
								r.Post("/refresh", passkeyHandler.RefreshToken)
							})
						})
					}
				})
			})
		}

		// RPC endpoints for wallet and dashboard (only if RPC URL is set)
		if o.RPCConfig.RPCUrl != "" {
			rpcProxyHandler := httphandler.RPCProxyHandler{
				RPCUrl:             o.RPCConfig.RPCUrl,
				RPCAuthHeaderKey:   o.RPCConfig.RPCRequestAuthHeaderKey,
				RPCAuthHeaderValue: o.RPCConfig.RPCRequestAuthHeaderValue,
			}
			r.With(middleware.WalletAuthMiddleware(o.walletJWTManager)).
				Post("/rpc/wallet", rpcProxyHandler.ServeHTTP)
			r.With(middleware.AuthenticateMiddleware(o.authManager, o.tenantManager)).
				Post("/rpc/user", rpcProxyHandler.ServeHTTP)
		}
	})

	// SEP-1, SEP-10, SEP-24 and miscellaneous endpoints that are tenant-unaware
	mux.Group(func(r chi.Router) {
		r.Get("/health", httphandler.HealthHandler{
			ReleaseID:        o.GitCommit,
			ServiceID:        ServiceID,
			Version:          o.Version,
			DBConnectionPool: o.AdminDBConnectionPool,
		}.ServeHTTP)

		// SEP 1 TOML file endpoint
		r.Get("/.well-known/stellar.toml", httphandler.StellarTomlHandler{
			DistributionAccountResolver: o.SubmitterEngine.DistributionAccountResolver,
			NetworkPassphrase:           o.NetworkPassphrase,
			Models:                      o.Models,
			Sep10SigningPublicKey:       o.Sep10SigningPublicKey,
			Sep45ContractID:             o.Sep45ContractID,
			InstanceName:                o.InstanceName,
			BaseURL:                     o.BaseURL,
		}.ServeHTTP)

		// SEP-10 endpoints
		r.Route("/sep10", func(r chi.Router) {
			sep10Handler := httphandler.SEP10Handler{
				SEP10Service: o.Sep10Service,
			}

			r.Get("/auth", sep10Handler.GetChallenge)
			r.Post("/auth", sep10Handler.PostChallenge)
		})

		// SEP-45 endpoints
		if o.EnableSep45 && o.Sep45Service != nil {
			r.Route("/sep45", func(r chi.Router) {
				sep45Handler := httphandler.SEP45Handler{
					SEP45Service: o.Sep45Service,
				}

				r.Get("/auth", sep45Handler.GetChallenge)
				r.Post("/auth", sep45Handler.PostChallenge)
			})
		}
		// SEP-24 endpoints
		r.Route("/sep24", func(r chi.Router) {
			sep24Handler := httphandler.SEP24Handler{
				Models:             o.Models,
				SEP24JWTManager:    o.sep24JWTManager,
				InteractiveBaseURL: o.BaseURL,
			}
			r.Get("/info", sep24Handler.GetInfo)
			// Protect transaction lookup with SEP-10 or SEP-45 auth to ensure only authorized clients can access details
			r.With(sepauth.WebAuthHeaderTokenAuthenticateMiddleware(o.sep24JWTManager)).Get("/transaction", sep24Handler.GetTransaction)

			// For initiating interactive deposit, allow either the new middleware (preferred) or legacy header path inside handler
			r.With(sepauth.WebAuthHeaderTokenAuthenticateMiddleware(o.sep24JWTManager)).Post("/transactions/deposit/interactive", sep24Handler.PostDepositInteractive)
		})

		sep24QueryTokenAuthenticationMiddleware := sepauth.SEP24QueryTokenAuthenticateMiddleware(o.sep24JWTManager, o.NetworkPassphrase, o.tenantManager, o.SingleTenantMode)
		r.With(sep24QueryTokenAuthenticationMiddleware).Get("/wallet-registration/*", httphandler.SEP24InteractiveDepositHandler{
			App:      sep24frontend.App,
			BasePath: "app/dist",
		}.ServeApp)

		sep24HeaderTokenAuthenticationMiddleware := sepauth.SEP24HeaderTokenAuthenticateMiddleware(o.sep24JWTManager, o.NetworkPassphrase, o.tenantManager, o.SingleTenantMode)
		r.With(sep24HeaderTokenAuthenticationMiddleware).Route("/sep24-interactive-deposit", func(r chi.Router) {
			r.Get("/info", httphandler.ReceiverRegistrationHandler{
				Models:              o.Models,
				ReceiverWalletModel: o.Models.ReceiverWallet,
				ReCAPTCHASiteKey:    o.ReCAPTCHASiteKey,
				ReCAPTCHADisabled:   o.DisableReCAPTCHA,
				CAPTCHAType:         o.CAPTCHAType,
			}.ServeHTTP)

			r.Post("/otp", httphandler.ReceiverSendOTPHandler{
				Models:             o.Models,
				MessageDispatcher:  o.MessageDispatcher,
				ReCAPTCHAValidator: reCAPTCHAValidator,
				ReCAPTCHADisabled:  o.DisableReCAPTCHA,
			}.ServeHTTP)
			r.Post("/verification", httphandler.VerifyReceiverRegistrationHandler{
				Models:                      o.Models,
				ReCAPTCHAValidator:          reCAPTCHAValidator,
				ReCAPTCHADisabled:           o.DisableReCAPTCHA,
				NetworkPassphrase:           o.NetworkPassphrase,
				CrashTrackerClient:          o.CrashTrackerClient,
				DistributionAccountResolver: o.SubmitterEngine.DistributionAccountResolver,
			}.VerifyReceiverRegistration)
		})

		// This will be used for test purposes and will only be available when IsPubnet is false:
		r.With(middleware.EnsureTenantMiddleware).Delete("/contact-info/{contact_info}", httphandler.DeleteContactInfoHandler{
			Models:            o.Models,
			NetworkPassphrase: o.NetworkPassphrase,
		}.ServeHTTP)
		// END SEP-24 endpoints
	})

	return mux
}

// createAuthManager builds the default AuthManager struct to be injected
// in all the authentication related routes.
func createAuthManager(dbConnectionPool db.DBConnectionPool, ec256PrivateKey string, resetTokenExpirationHours int) (auth.AuthManager, error) {
	if dbConnectionPool == nil {
		return nil, fmt.Errorf("db connection pool cannot be nil")
	}

	ec256PublicKey, err := utils.GetEC256PublicKeyFromPrivateKey(ec256PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("validating auth manager public key: %w", err)
	}

	if resetTokenExpirationHours < 1 {
		return nil, fmt.Errorf("reset token expiration hours must be greater than 0")
	}

	passwordEncrypter := auth.NewDefaultPasswordEncrypter()

	authManager := auth.NewAuthManager(
		auth.WithDefaultAuthenticatorOption(dbConnectionPool, passwordEncrypter, time.Hour*time.Duration(resetTokenExpirationHours)),
		auth.WithDefaultJWTManagerOption(ec256PublicKey, ec256PrivateKey),
		auth.WithDefaultRoleManagerOption(dbConnectionPool, data.OwnerUserRole.String()),
		auth.WithDefaultMFAManagerOption(dbConnectionPool),
	)

	return authManager, nil
}

// staticFileServer sets up a http.FileServer handler to serve
// static files from publicFiles embed FileSystem.
func staticFileServer(r chi.Router, fileSystem fs.FS) {
	r.Get("/static/*", func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")

		// Don't allow users to list directories
		if r.URL.Path[len(r.URL.Path)-1] == '/' {
			http.NotFound(w, r)
			return
		}

		fs := http.StripPrefix(pathPrefix, http.FileServer(http.FS(fileSystem)))
		fs.ServeHTTP(w, r)
	})
}
