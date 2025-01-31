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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httphandler"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/publicfiles"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
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
	EnableScheduler                 bool
	tenantManager                   tenant.ManagerInterface
	DistributionAccountService      services.DistributionAccountServiceInterface
	DistAccEncryptionPassphrase     string
	EventProducer                   events.Producer
	MaxInvitationResendAttempts     int
	SingleTenantMode                bool
	CircleService                   circle.ServiceInterface
	CircleAPIType                   circle.APIType
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
	opts.tenantManager = tenant.NewManager(tenant.WithDatabase(opts.AdminDBConnectionPool))

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
			return fmt.Errorf("reCAPTCHA cannot be disabled in pubnet")
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

func Serve(opts ServeOptions, httpServer HTTPServerInterface) error {
	if err := opts.ValidateSecurity(); err != nil {
		return fmt.Errorf("validating security options: %w", err)
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
		r.Use(middleware.AuthenticateMiddleware(authManager, o.tenantManager))
		r.Use(middleware.EnsureTenantMiddleware)

		r.With(middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...)).Route("/statistics", func(r chi.Router) {
			statisticsHandler := httphandler.StatisticsHandler{DBConnectionPool: o.MtnDBConnectionPool}
			r.Get("/", statisticsHandler.GetStatistics)
			r.Get("/{id}", statisticsHandler.GetStatisticsByDisbursement)
		})

		r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole)).Route("/users", func(r chi.Router) {
			userHandler := httphandler.UserHandler{
				AuthManager:        authManager,
				CrashTrackerClient: o.CrashTrackerClient,
				MessengerClient:    o.EmailMessengerClient,
				Models:             o.Models,
			}

			r.Get("/", userHandler.GetAllUsers)
			r.Post("/", userHandler.CreateUser)
			r.Get("/roles", httphandler.ListRolesHandler{}.GetRoles)
			r.Patch("/roles", userHandler.UpdateUserRoles)
			r.Patch("/activation", userHandler.UserActivation)
		})
		r.Post("/refresh-token", httphandler.RefreshTokenHandler{AuthManager: authManager}.PostRefreshToken)

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
			r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole)).
				Post("/", handler.PostDisbursement)

			r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole)).
				Delete("/{id}", handler.DeleteDisbursement)

			r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole)).
				Post("/{id}/instructions", handler.PostDisbursementInstructions)

			r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole)).
				Get("/{id}/instructions", handler.GetDisbursementInstructions)

			r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole, data.BusinessUserRole)).
				Get("/", handler.GetDisbursements)

			r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole, data.BusinessUserRole)).
				Get("/{id}", handler.GetDisbursement)

			r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole, data.BusinessUserRole)).
				Get("/{id}/receivers", handler.GetDisbursementReceivers)

			r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole)).
				Patch("/{id}/status", handler.PatchDisbursementStatus)
		})

		r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole, data.BusinessUserRole)).Route("/payments", func(r chi.Router) {
			paymentsHandler := httphandler.PaymentsHandler{
				Models:                      o.Models,
				DBConnectionPool:            o.MtnDBConnectionPool,
				AuthManager:                 o.authManager,
				EventProducer:               o.EventProducer,
				CrashTrackerClient:          o.CrashTrackerClient,
				DistributionAccountResolver: o.SubmitterEngine.DistributionAccountResolver,
			}
			r.Get("/", paymentsHandler.GetPayments)
			r.Get("/{id}", paymentsHandler.GetPayment)
			r.Patch("/retry", paymentsHandler.RetryPayments)
			r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole)).
				Patch("/{id}/status", paymentsHandler.PatchPaymentStatus)
		})

		r.Route("/receivers", func(r chi.Router) {
			receiversHandler := httphandler.ReceiverHandler{Models: o.Models, DBConnectionPool: o.MtnDBConnectionPool}
			r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole, data.BusinessUserRole)).
				Get("/", receiversHandler.GetReceivers)
			r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole, data.BusinessUserRole)).
				Get("/{id}", receiversHandler.GetReceiver)

			r.With(middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...)).
				Get("/verification-types", receiversHandler.GetReceiverVerificationTypes)

			updateReceiverHandler := httphandler.UpdateReceiverHandler{
				Models:           o.Models,
				DBConnectionPool: o.MtnDBConnectionPool,
				AuthManager:      authManager,
			}
			r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole)).
				Patch("/{id}", updateReceiverHandler.UpdateReceiver)

			receiverWalletHandler := httphandler.ReceiverWalletsHandler{
				Models:             o.Models,
				CrashTrackerClient: o.CrashTrackerClient,
				EventProducer:      o.EventProducer,
			}
			r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole)).
				Patch("/wallets/{receiver_wallet_id}", receiverWalletHandler.RetryInvitation)
		})

		r.
			With(middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...)).
			Get("/registration-contact-types", httphandler.RegistrationContactTypesHandler{}.Get)

		r.Route("/assets", func(r chi.Router) {
			assetsHandler := httphandler.AssetsHandler{
				Models:          o.Models,
				SubmitterEngine: o.SubmitterEngine,
			}

			r.With(middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...)).
				Get("/", assetsHandler.GetAssets)

			r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole, data.DeveloperUserRole)).
				Post("/", assetsHandler.CreateAsset)

			r.Route("/{id}", func(r chi.Router) {
				r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole, data.DeveloperUserRole)).Delete("/", assetsHandler.DeleteAsset)
			})
		})

		r.With(middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...)).Route("/wallets", func(r chi.Router) {
			walletsHandler := httphandler.WalletsHandler{
				Models:      o.Models,
				NetworkType: o.NetworkType,
			}
			r.Get("/", walletsHandler.GetWallets)
			r.With(middleware.AnyRoleMiddleware(authManager, data.DeveloperUserRole)).
				Post("/", walletsHandler.PostWallets)
			r.With(middleware.AnyRoleMiddleware(authManager, data.DeveloperUserRole)).
				Delete("/{id}", walletsHandler.DeleteWallet)
			r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole)).
				Patch("/{id}", walletsHandler.PatchWallets)
		})

		profileHandler := httphandler.ProfileHandler{
			Models:                      o.Models,
			AuthManager:                 authManager,
			MaxMemoryAllocation:         httphandler.DefaultMaxMemoryAllocation,
			BaseURL:                     o.BaseURL,
			DistributionAccountResolver: o.SubmitterEngine.DistributionAccountResolver,
			PasswordValidator:           o.PasswordValidator,
			PublicFilesFS:               publicfiles.PublicFiles,
			NetworkType:                 o.NetworkType,
		}
		r.Route("/profile", func(r chi.Router) {
			r.With(middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...)).
				Get("/", profileHandler.GetProfile)

			r.With(middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...)).
				Patch("/", profileHandler.PatchUserProfile)

			r.With(middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...)).
				Patch("/reset-password", profileHandler.PatchUserPassword)
		})

		r.Route("/organization", func(r chi.Router) {
			r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole)).
				Patch("/", profileHandler.PatchOrganizationProfile)

			r.With(middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...)).
				Get("/", profileHandler.GetOrganizationInfo)

			r.With(middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...)).
				Get("/logo", profileHandler.GetOrganizationLogo)

			r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole)).
				Patch("/circle-config", httphandler.CircleConfigHandler{
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

		balancesHandler := httphandler.BalancesHandler{
			DistributionAccountResolver: o.SubmitterEngine.DistributionAccountResolver,
			CircleService:               o.CircleService,
			NetworkType:                 o.NetworkType,
		}
		r.Get("/balances", balancesHandler.Get)

		exportHandler := httphandler.ExportHandler{
			Models: o.Models,
		}
		r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole)).
			Route("/exports", func(r chi.Router) {
				r.Get("/disbursements", exportHandler.ExportDisbursements)
				r.Get("/payments", exportHandler.ExportPayments)
				r.Get("/receivers", exportHandler.ExportReceivers)
			})
	})

	reCAPTCHAValidator := validators.NewGoogleReCAPTCHAValidator(o.ReCAPTCHASiteSecretKey, httpclient.DefaultClient())

	// Public routes that are tenant aware (they need to know the tenant ID)
	mux.Group(func(r chi.Router) {
		r.Use(middleware.EnsureTenantMiddleware)

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
			InstanceName:                o.InstanceName,
		}.ServeHTTP)

		r.Route("/wallet-registration", func(r chi.Router) {
			sep24QueryTokenAuthenticationMiddleware := anchorplatform.SEP24QueryTokenAuthenticateMiddleware(o.sep24JWTManager, o.NetworkPassphrase, o.tenantManager, o.SingleTenantMode)
			r.With(sep24QueryTokenAuthenticationMiddleware).Get("/start", httphandler.ReceiverRegistrationHandler{
				Models:              o.Models,
				ReceiverWalletModel: o.Models.ReceiverWallet,
				ReCAPTCHASiteKey:    o.ReCAPTCHASiteKey,
			}.ServeHTTP) // This loads the SEP-24 PII registration webpage.

			sep24HeaderTokenAuthenticationMiddleware := anchorplatform.SEP24HeaderTokenAuthenticateMiddleware(o.sep24JWTManager, o.NetworkPassphrase, o.tenantManager, o.SingleTenantMode)
			r.With(sep24HeaderTokenAuthenticationMiddleware).Post("/otp", httphandler.ReceiverSendOTPHandler{
				Models:             o.Models,
				MessageDispatcher:  o.MessageDispatcher,
				ReCAPTCHAValidator: reCAPTCHAValidator,
			}.ServeHTTP)
			r.With(sep24HeaderTokenAuthenticationMiddleware).Post("/verification", httphandler.VerifyReceiverRegistrationHandler{
				AnchorPlatformAPIService:    o.AnchorPlatformAPIService,
				Models:                      o.Models,
				ReCAPTCHAValidator:          reCAPTCHAValidator,
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
