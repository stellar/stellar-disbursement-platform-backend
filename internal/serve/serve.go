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
	"github.com/stellar/go/network"
	supporthttp "github.com/stellar/go/support/http"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
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
	Environment                     string
	GitCommit                       string
	Port                            int
	Version                         string
	InstanceName                    string
	MonitorService                  monitor.MonitorServiceInterface
	MtnDBConnectionPool             db.DBConnectionPool
	AdminDBConnectionPool           db.DBConnectionPool
	TSSDBConnectionPool             db.DBConnectionPool
	EC256PrivateKey                 string
	Models                          *data.Models
	CorsAllowedOrigins              []string
	authManager                     auth.AuthManager
	EmailMessengerClient            message.MessengerClient
	MessageDispatcher               message.MessageDispatcherInterface
	SEP24JWTSecret                  string
	sep24JWTManager                 *anchorplatform.JWTManager
	BaseURL                         string
	ResetTokenExpirationHours       int
	NetworkPassphrase               string
	NetworkType                     utils.NetworkType
	SubmitterEngine                 engine.SubmitterEngine
	Sep10SigningPublicKey           string
	Sep10SigningPrivateKey          string
	EnableEmbeddedWallets           bool
	EmbeddedWalletsWasmHash         string
	EnableSep45                     bool
	Sep45ContractId                 string
	RpcConfig                       stellar.RPCOptions
	AnchorPlatformBaseSepURL        string
	AnchorPlatformBasePlatformURL   string
	AnchorPlatformOutgoingJWTSecret string
	AnchorPlatformAPIService        anchorplatform.AnchorPlatformAPIServiceInterface
	CrashTrackerClient              crashtracker.CrashTrackerClient
	ReCAPTCHASiteKey                string
	ReCAPTCHASiteSecretKey          string
	DisableMFA                      bool
	DisableReCAPTCHA                bool
	PasswordValidator               *authUtils.PasswordValidator
	EnableScheduler                 bool // Deprecated: Use EventBrokerType=SCHEDULER instead.
	tenantManager                   tenant.ManagerInterface
	DistributionAccountService      services.DistributionAccountServiceInterface
	DistAccEncryptionPassphrase     string
	EventProducer                   events.Producer
	MaxInvitationResendAttempts     int
	SingleTenantMode                bool
	CircleService                   circle.ServiceInterface
	CircleAPIType                   circle.APIType
	EmbeddedWalletService           services.EmbeddedWalletServiceInterface
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

	// Setup Embedded Wallet Service (only if enabled)
	if opts.EnableEmbeddedWallets {
		opts.EmbeddedWalletService, err = dependencyinjection.NewEmbeddedWalletService(context.Background(), services.EmbeddedWalletServiceOptions{
			MTNDBConnectionPool: opts.MtnDBConnectionPool,
			TSSDBConnectionPool: opts.TSSDBConnectionPool,
			WasmHash:            opts.EmbeddedWalletsWasmHash,
		})
		if err != nil {
			return fmt.Errorf("error creating embedded wallet service: %w", err)
		}
	}

	// Setup Stellar Auth JWT manager
	opts.authManager, err = createAuthManager(
		opts.MtnDBConnectionPool, opts.EC256PrivateKey, opts.ResetTokenExpirationHours,
	)
	if err != nil {
		return fmt.Errorf("error creating Stellar Auth manager: %w", err)
	}

	// Setup Anchor Platform SEP24 JWT manager
	sep24JWTManager, err := anchorplatform.NewJWTManager(opts.SEP24JWTSecret, 15000)
	if err != nil {
		return fmt.Errorf("error creating SEP-24 JWT manager: %w", err)
	}
	opts.sep24JWTManager = sep24JWTManager

	opts.PasswordValidator, err = authUtils.GetPasswordValidatorInstance()
	if err != nil {
		return fmt.Errorf("error initializing password validator: %w", err)
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

// ValidateRpc validates the RPC options.
func (opts *ServeOptions) ValidateRpc() error {
	if opts.RpcConfig.RPCUrl == "" && (opts.RpcConfig.RPCRequestAuthHeaderKey != "" || opts.RpcConfig.RPCRequestAuthHeaderValue != "") {
		return fmt.Errorf("RPC URL must be set when RPC request header key or value is set")
	}

	if opts.RpcConfig.RPCRequestAuthHeaderKey != "" && opts.RpcConfig.RPCRequestAuthHeaderValue == "" {
		return fmt.Errorf("RPC request header value must be set when RPC request header key is set")
	}

	if opts.RpcConfig.RPCRequestAuthHeaderKey == "" && opts.RpcConfig.RPCRequestAuthHeaderValue != "" {
		return fmt.Errorf("RPC request header key must be set when RPC request header value is set")
	}

	// RPC-dependent feature validation
	hasRpcFeatures := opts.EnableEmbeddedWallets || opts.EnableSep45
	if hasRpcFeatures && opts.RpcConfig.RPCUrl == "" {
		return fmt.Errorf("RPC URL must be set when RPC-dependent features are enabled")
	}

	// Embedded wallet feature validation
	if opts.EnableEmbeddedWallets && opts.EmbeddedWalletsWasmHash == "" {
		return fmt.Errorf("embedded wallets WASM hash must be set when embedded wallets are enabled")
	}

	// SEP-45 feature validation
	if opts.EnableSep45 && opts.Sep45ContractId == "" {
		return fmt.Errorf("SEP-45 contract ID must be set when SEP-45 is enabled")
	}

	return nil
}

func Serve(opts ServeOptions, httpServer HTTPServerInterface) error {
	if err := opts.ValidateSecurity(); err != nil {
		return fmt.Errorf("validating security options: %w", err)
	}

	if err := opts.ValidateRpc(); err != nil {
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

		r.With(middleware.RequirePermission(
			data.WriteAll,
			middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.DeveloperUserRole),
		)).Route("/api-keys", func(r chi.Router) {
			apiKeyHandler := httphandler.APIKeyHandler{
				Models: o.Models,
			}
			r.Get("/api-keys/{id}", apiKeyHandler.GetApiKeyByID)
			r.Get("/", apiKeyHandler.GetAllApiKeys)
			r.Post("/", apiKeyHandler.CreateAPIKey)
			r.Patch("/api-keys/{id}", apiKeyHandler.UpdateKey)
			r.Delete("/{id}", apiKeyHandler.DeleteApiKey)
		})

		r.With(middleware.RequirePermission(
			data.ReadStatistics,
			middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...),
		)).Route("/statistics", func(r chi.Router) {
			h := httphandler.StatisticsHandler{DBConnectionPool: o.MtnDBConnectionPool}
			r.Get("/", h.GetStatistics)
			r.Get("/{id}", h.GetStatisticsByDisbursement)
		})

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

		r.Route("/disbursements", func(r chi.Router) {
			handler := httphandler.DisbursementHandler{
				Models:                      o.Models,
				AuthManager:                 authManager,
				MonitorService:              o.MonitorService,
				DistributionAccountResolver: o.SubmitterEngine.DistributionAccountResolver,
				DisbursementManagementService: &services.DisbursementManagementService{
					Models:                     o.Models,
					AuthManager:                authManager,
					EventProducer:              o.EventProducer,
					CrashTrackerClient:         o.CrashTrackerClient,
					DistributionAccountService: o.DistributionAccountService,
				},
			}

			// Group all READ operations
			r.With(middleware.RequirePermission(
				data.ReadDisbursements,
				middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole, data.BusinessUserRole),
			)).Group(func(r chi.Router) {
				r.Get("/", handler.GetDisbursements)
				r.Get("/{id}", handler.GetDisbursement)
				r.Get("/{id}/receivers", handler.GetDisbursementReceivers)
				r.Get("/{id}/instructions", handler.GetDisbursementInstructions)
			})

			// Group all WRITE operations
			r.With(middleware.RequirePermission(
				data.WriteDisbursements,
				middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole),
			)).Group(func(r chi.Router) {
				r.Post("/", handler.PostDisbursement)
				r.Delete("/{id}", handler.DeleteDisbursement)
				r.Post("/{id}/instructions", handler.PostDisbursementInstructions)
				r.Patch("/{id}/status", handler.PatchDisbursementStatus)
			})
		})

		r.Route("/payments", func(r chi.Router) {
			paymentsHandler := httphandler.PaymentsHandler{
				Models:                      o.Models,
				DBConnectionPool:            o.MtnDBConnectionPool,
				AuthManager:                 o.authManager,
				EventProducer:               o.EventProducer,
				CrashTrackerClient:          o.CrashTrackerClient,
				DistributionAccountResolver: o.SubmitterEngine.DistributionAccountResolver,
			}

			// Read operations
			r.With(middleware.RequirePermission(
				data.ReadPayments,
				middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole, data.BusinessUserRole),
			)).Group(func(r chi.Router) {
				r.Get("/", paymentsHandler.GetPayments)
				r.Get("/{id}", paymentsHandler.GetPayment)
			})

			// Write operations with different role permissions
			r.With(middleware.RequirePermission(
				data.WritePayments,
				middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole, data.BusinessUserRole),
			)).Patch("/retry", paymentsHandler.RetryPayments)

			r.With(middleware.RequirePermission(
				data.WritePayments,
				middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole),
			)).Patch("/{id}/status", paymentsHandler.PatchPaymentStatus)
		})

		r.Route("/receivers", func(r chi.Router) {
			receiversHandler := httphandler.ReceiverHandler{Models: o.Models, DBConnectionPool: o.MtnDBConnectionPool}

			// Read operations
			r.With(middleware.RequirePermission(
				data.ReadReceivers,
				middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...),
			)).Get("/verification-types", receiversHandler.GetReceiverVerificationTypes)

			r.With(middleware.RequirePermission(
				data.ReadReceivers,
				middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole, data.BusinessUserRole),
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
				EventProducer:      o.EventProducer,
			}

			r.With(middleware.RequirePermission(
				data.WriteReceivers,
				middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole),
			)).Group(func(r chi.Router) {
				r.Post("/", receiversHandler.CreateReceiver)
				r.Patch("/{id}", updateReceiverHandler.UpdateReceiver)
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
				Models:          o.Models,
				SubmitterEngine: o.SubmitterEngine,
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
				Models:        o.Models,
				NetworkType:   o.NetworkType,
				AssetResolver: services.NewAssetResolver(o.Models.Assets),
			}

			// Read operations
			r.With(middleware.RequirePermission(
				data.ReadWallets,
				middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...),
			)).Get("/", walletsHandler.GetWallets)

			// Write operations
			r.With(middleware.RequirePermission(
				data.WriteWallets,
				middleware.AnyRoleMiddleware(authManager, data.DeveloperUserRole),
			)).Group(func(r chi.Router) {
				r.Post("/", walletsHandler.PostWallets)
				r.Delete("/{id}", walletsHandler.DeleteWallet)
			})

			r.With(middleware.RequirePermission(
				data.WriteWallets,
				middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole),
			)).Patch("/{id}", walletsHandler.PatchWallets)
		})

		// Embedded wallet routes (only if feature is enabled)
		if o.EnableEmbeddedWallets && o.EmbeddedWalletService != nil {
			r.With(middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...)).Route("/embedded-wallets", func(r chi.Router) {
				walletCreationHandler := httphandler.WalletCreationHandler{
					EmbeddedWalletService: o.EmbeddedWalletService,
				}
				r.Post("/", walletCreationHandler.CreateWallet)
				r.Get("/status", walletCreationHandler.GetWallet)
			})
		}

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
			middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole),
		)).Route("/exports", func(r chi.Router) {
			r.Get("/disbursements", exportHandler.ExportDisbursements)
			r.Get("/payments", exportHandler.ExportPayments)
			r.Get("/receivers", exportHandler.ExportReceivers)
		})
	})

	reCAPTCHAValidator := validators.NewGoogleReCAPTCHAValidator(o.ReCAPTCHASiteSecretKey, httpclient.DefaultClient())

	// Public routes that are tenant aware (they need to know the tenant ID)
	mux.Group(func(r chi.Router) {
		r.Use(middleware.EnsureTenantMiddleware)

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
	})

	// SEP-24 and miscellaneous endpoints that are tenant-unaware
	mux.Group(func(r chi.Router) {
		r.Get("/health", httphandler.HealthHandler{
			ReleaseID:        o.GitCommit,
			ServiceID:        ServiceID,
			Version:          o.Version,
			DBConnectionPool: o.AdminDBConnectionPool,
			Producer:         o.EventProducer,
		}.ServeHTTP)

		// START SEP-24 endpoints
		r.Get("/.well-known/stellar.toml", httphandler.StellarTomlHandler{
			AnchorPlatformBaseSepURL:    o.AnchorPlatformBaseSepURL,
			DistributionAccountResolver: o.SubmitterEngine.DistributionAccountResolver,
			NetworkPassphrase:           o.NetworkPassphrase,
			Models:                      o.Models,
			Sep10SigningPublicKey:       o.Sep10SigningPublicKey,
			Sep45ContractId:             o.Sep45ContractId,
			InstanceName:                o.InstanceName,
		}.ServeHTTP)

		sep24QueryTokenAuthenticationMiddleware := anchorplatform.SEP24QueryTokenAuthenticateMiddleware(o.sep24JWTManager, o.NetworkPassphrase, o.tenantManager, o.SingleTenantMode)
		r.With(sep24QueryTokenAuthenticationMiddleware).Get("/wallet-registration/*", httphandler.SEP24InteractiveDepositHandler{
			App:      sep24frontend.App,
			BasePath: "app/dist",
		}.ServeApp)

		sep24HeaderTokenAuthenticationMiddleware := anchorplatform.SEP24HeaderTokenAuthenticateMiddleware(o.sep24JWTManager, o.NetworkPassphrase, o.tenantManager, o.SingleTenantMode)
		r.With(sep24HeaderTokenAuthenticationMiddleware).Route("/sep24-interactive-deposit", func(r chi.Router) {
			r.Get("/info", httphandler.ReceiverRegistrationHandler{
				Models:              o.Models,
				ReceiverWalletModel: o.Models.ReceiverWallet,
				ReCAPTCHASiteKey:    o.ReCAPTCHASiteKey,
				ReCAPTCHADisabled:   o.DisableReCAPTCHA,
			}.ServeHTTP)

			r.Post("/otp", httphandler.ReceiverSendOTPHandler{
				Models:             o.Models,
				MessageDispatcher:  o.MessageDispatcher,
				ReCAPTCHAValidator: reCAPTCHAValidator,
				ReCAPTCHADisabled:  o.DisableReCAPTCHA,
			}.ServeHTTP)
			r.Post("/verification", httphandler.VerifyReceiverRegistrationHandler{
				AnchorPlatformAPIService:    o.AnchorPlatformAPIService,
				Models:                      o.Models,
				ReCAPTCHAValidator:          reCAPTCHAValidator,
				ReCAPTCHADisabled:           o.DisableReCAPTCHA,
				NetworkPassphrase:           o.NetworkPassphrase,
				EventProducer:               o.EventProducer,
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
