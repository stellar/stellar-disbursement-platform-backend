package serve

import (
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/network"
	supporthttp "github.com/stellar/go/support/http"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httphandler"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	publicfiles "github.com/stellar/stellar-disbursement-platform-backend/internal/serve/publicfiles"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	txnsubmitterutils "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
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
	MonitorService                  monitor.MonitorServiceInterface
	DatabaseDSN                     string
	dbConnectionPool                db.DBConnectionPool
	EC256PublicKey                  string
	EC256PrivateKey                 string
	Models                          *data.Models
	CorsAllowedOrigins              []string
	authManager                     auth.AuthManager
	EmailMessengerClient            message.MessengerClient
	SMSMessengerClient              message.MessengerClient
	SEP24JWTSecret                  string
	sep24JWTManager                 *anchorplatform.JWTManager
	BaseURL                         string
	UIBaseURL                       string
	ResetTokenExpirationHours       int
	NetworkPassphrase               string
	HorizonURL                      string
	horizonClient                   horizonclient.ClientInterface
	signatureService                engine.SignatureService
	Sep10SigningPublicKey           string
	Sep10SigningPrivateKey          string
	AnchorPlatformBaseSepURL        string
	AnchorPlatformBasePlatformURL   string
	AnchorPlatformOutgoingJWTSecret string
	AnchorPlatformAPIService        anchorplatform.AnchorPlatformAPIServiceInterface
	CrashTrackerClient              crashtracker.CrashTrackerClient
	DistributionPublicKey           string
	DistributionSeed                string
	ReCAPTCHASiteKey                string
	ReCAPTCHASiteSecretKey          string
	EnableMFA                       bool
	EnableReCAPTCHA                 bool
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

	// Setup Database:
	dbConnectionPool, err := db.OpenDBConnectionPoolWithMetrics(opts.DatabaseDSN, opts.MonitorService)
	if err != nil {
		return fmt.Errorf("error connecting to the database: %w", err)
	}
	opts.Models, err = data.NewModels(dbConnectionPool)
	if err != nil {
		return fmt.Errorf("error creating models for Serve: %w", err)
	}
	opts.dbConnectionPool = dbConnectionPool

	// Setup Stellar Auth JWT manager
	opts.authManager, err = createAuthManager(
		opts.dbConnectionPool, opts.EC256PublicKey, opts.EC256PrivateKey, opts.ResetTokenExpirationHours,
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

	// Setup Horizon Client
	opts.horizonClient = &horizonclient.Client{
		HorizonURL: opts.HorizonURL,
		HTTP:       httpclient.DefaultClient(),
	}

	// Setup Signature Service
	// TODO: improve the way we setup signature service
	opts.signatureService, err = engine.NewDefaultSignatureService(
		opts.NetworkPassphrase,
		dbConnectionPool,
		opts.DistributionSeed,
		store.NewChannelAccountModel(opts.dbConnectionPool),
		txnsubmitterutils.DefaultPrivateKeyEncrypter{},
		opts.DistributionSeed,
	)
	if err != nil {
		return fmt.Errorf("error creating signature service: %w", err)
	}

	return nil
}

func Serve(opts ServeOptions, httpServer HTTPServerInterface) error {
	err := opts.SetupDependencies()
	if err != nil {
		return fmt.Errorf("error starting dependencies: %w", err)
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
			log.Info("Closing the database connection...")
			err := opts.dbConnectionPool.Close()
			if err != nil {
				log.Errorf("error closing database connection: %s", err.Error())
			}

			log.Info("Stopping SDP (Stellar Disbursement Platform) Server")
		},
	}
	httpServer.Run(serverConfig)
	return nil
}

func handleHTTP(o ServeOptions) *chi.Mux {
	mux := chi.NewMux()

	// Middleware
	mux.Use(middleware.CorsMiddleware(o.CorsAllowedOrigins))
	mux.Use(chimiddleware.RequestID)
	mux.Use(chimiddleware.RealIP)
	mux.Use(supporthttp.LoggingMiddleware)
	mux.Use(middleware.RecoverHandler)
	mux.Use(middleware.MetricsRequestHandler(o.MonitorService))
	mux.Use(middleware.CSPMiddleware())

	// Create a route along /static that will serve contents from the ./public_files folder.
	staticFileServer(mux, publicfiles.PublicFiles)

	// Authenticated Routes
	authManager := o.authManager
	mux.Group(func(r chi.Router) {
		r.Use(middleware.AuthenticateMiddleware(authManager))

		r.With(middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...)).Route("/statistics", func(r chi.Router) {
			statisticsHandler := httphandler.StatisticsHandler{DBConnectionPool: o.dbConnectionPool}
			r.Get("/", statisticsHandler.GetStatistics)
			r.Get("/{id}", statisticsHandler.GetStatisticsByDisbursement)
		})

		r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole)).Route("/users", func(r chi.Router) {
			userHandler := httphandler.UserHandler{
				AuthManager:     authManager,
				MessengerClient: o.EmailMessengerClient,
				UIBaseURL:       o.UIBaseURL,
				Models:          o.Models,
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
				Models:           o.Models,
				MonitorService:   o.MonitorService,
				DBConnectionPool: o.dbConnectionPool,
				AuthManager:      authManager,
			}
			r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole)).
				Post("/", handler.PostDisbursement)

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
			paymentsHandler := httphandler.PaymentsHandler{Models: o.Models, DBConnectionPool: o.dbConnectionPool, AuthManager: o.authManager}
			r.Get("/", paymentsHandler.GetPayments)
			r.Get("/{id}", paymentsHandler.GetPayment)
			r.Patch("/retry", paymentsHandler.RetryPayments)
		})

		r.Route("/receivers", func(r chi.Router) {
			receiversHandler := httphandler.ReceiverHandler{Models: o.Models, DBConnectionPool: o.dbConnectionPool}
			r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole, data.BusinessUserRole)).
				Get("/", receiversHandler.GetReceivers)
			r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole)).
				Get("/{id}", receiversHandler.GetReceiver)

			updateReceiverHandler := httphandler.UpdateReceiverHandler{Models: o.Models, DBConnectionPool: o.dbConnectionPool}
			r.With(middleware.AnyRoleMiddleware(authManager, data.OwnerUserRole, data.FinancialControllerUserRole)).
				Patch("/{id}", updateReceiverHandler.UpdateReceiver)
		})

		r.With(middleware.AnyRoleMiddleware(authManager, data.GetAllRoles()...)).Route("/countries", func(r chi.Router) {
			r.Get("/", httphandler.CountriesHandler{Models: o.Models}.GetCountries)
		})

		r.Route("/assets", func(r chi.Router) {
			assetsHandler := httphandler.AssetsHandler{
				Models:           o.Models,
				SignatureService: o.signatureService,
				HorizonClient:    o.horizonClient,
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
			walletsHandler := httphandler.WalletsHandler{Models: o.Models}
			r.Get("/", walletsHandler.GetWallets)
			r.With(middleware.AnyRoleMiddleware(authManager, data.DeveloperUserRole)).
				Post("/", walletsHandler.PostWallets)
		})

		profileHandler := httphandler.ProfileHandler{
			Models:                o.Models,
			AuthManager:           authManager,
			MaxMemoryAllocation:   httphandler.DefaultMaxMemoryAllocation,
			BaseURL:               o.BaseURL,
			DistributionPublicKey: o.DistributionPublicKey,
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
		})
	})

	// Even if the logo URL is under the public endpoints, it'll be authenticated. The `auth token` should be
	// added in the URL's query params. Example: https://...?token=mytoken
	mux.Get("/organization/logo", httphandler.ProfileHandler{Models: o.Models, PublicFilesFS: publicfiles.PublicFiles}.GetOrganizationLogo)

	mux.Get("/health", httphandler.HealthHandler{
		ReleaseID: o.GitCommit,
		ServiceID: ServiceID,
		Version:   o.Version,
	}.ServeHTTP)

	reCAPTCHAValidator := validators.NewGoogleReCAPTCHAValidator(o.ReCAPTCHASiteSecretKey, httpclient.DefaultClient())

	mux.Post("/login", httphandler.LoginHandler{
		AuthManager:        authManager,
		ReCAPTCHAValidator: reCAPTCHAValidator,
		MessengerClient:    o.EmailMessengerClient,
		Models:             o.Models,
		ReCAPTCHAEnabled:   o.EnableReCAPTCHA,
		MFAEnabled:         o.EnableMFA,
	}.ServeHTTP)
	mux.Post("/mfa", httphandler.MFAHandler{
		AuthManager:        authManager,
		ReCAPTCHAValidator: reCAPTCHAValidator,
		Models:             o.Models,
	}.ServeHTTP)
	mux.Post("/forgot-password", httphandler.ForgotPasswordHandler{
		AuthManager:        authManager,
		MessengerClient:    o.EmailMessengerClient,
		UIBaseURL:          o.UIBaseURL,
		Models:             o.Models,
		ReCAPTCHAValidator: reCAPTCHAValidator,
		ReCAPTCHAEnabled:   o.EnableReCAPTCHA,
	}.ServeHTTP)
	mux.Post("/reset-password", httphandler.ResetPasswordHandler{AuthManager: authManager}.ServeHTTP)

	// START SEP-24 endpoints
	mux.Get("/.well-known/stellar.toml", httphandler.StellarTomlHandler{
		AnchorPlatformBaseSepURL: o.AnchorPlatformBaseSepURL,
		DistributionPublicKey:    o.DistributionPublicKey,
		NetworkPassphrase:        o.NetworkPassphrase,
		Models:                   o.Models,
		Sep10SigningPublicKey:    o.Sep10SigningPublicKey,
	}.ServeHTTP)

	mux.Route("/wallet-registration", func(r chi.Router) {
		sep24QueryTokenAuthenticationMiddleware := anchorplatform.SEP24QueryTokenAuthenticateMiddleware(o.sep24JWTManager, o.NetworkPassphrase)
		r.With(sep24QueryTokenAuthenticationMiddleware).Get("/start", httphandler.ReceiverRegistrationHandler{ReceiverWalletModel: o.Models.ReceiverWallet, ReCAPTCHASiteKey: o.ReCAPTCHASiteKey}.ServeHTTP) // This loads the SEP-24 PII registration webpage.

		sep24HeaderTokenAuthenticationMiddleware := anchorplatform.SEP24HeaderTokenAuthenticateMiddleware(o.sep24JWTManager, o.NetworkPassphrase)
		r.With(sep24HeaderTokenAuthenticationMiddleware).Post("/otp", httphandler.ReceiverSendOTPHandler{Models: o.Models, SMSMessengerClient: o.SMSMessengerClient, ReCAPTCHAValidator: reCAPTCHAValidator}.ServeHTTP)
		r.With(sep24HeaderTokenAuthenticationMiddleware).Post("/verification", httphandler.VerifyReceiverRegistrationHandler{
			AnchorPlatformAPIService: o.AnchorPlatformAPIService,
			Models:                   o.Models,
			ReCAPTCHAValidator:       reCAPTCHAValidator,
			NetworkPassphrase:        o.NetworkPassphrase,
		}.VerifyReceiverRegistration)

		// This will be used for test purposes and will only be available when IsPubnet is false:
		if o.NetworkPassphrase == network.TestNetworkPassphrase {
			r.Delete("/phone-number/{phone_number}", httphandler.DeletePhoneNumberHandler{Models: o.Models, NetworkPassphrase: o.NetworkPassphrase}.ServeHTTP)
		}
	})
	// END SEP-24 endpoints

	return mux
}

// createAuthManager builds the default AuthManager struct to be injected
// in all the authentication related routes.
func createAuthManager(dbConnectionPool db.DBConnectionPool, ec256PublicKey, ec256PrivateKey string, resetTokenExpirationHours int) (auth.AuthManager, error) {
	if dbConnectionPool == nil {
		return nil, fmt.Errorf("db connection pool cannot be nil")
	}

	err := utils.ValidateECDSAKeys(ec256PublicKey, ec256PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("validating auth manager keys: %w", err)
	}

	if resetTokenExpirationHours < 1 {
		return nil, fmt.Errorf("reset token expiration hours must be greater than 0")
	}

	passwordEncrypter := auth.NewDefaultPasswordEncrypter()

	authDBConnectionPool := auth.DBConnectionPoolFromSqlDB(dbConnectionPool.SqlDB(), dbConnectionPool.DriverName())
	authManager := auth.NewAuthManager(
		auth.WithDefaultAuthenticatorOption(authDBConnectionPool, passwordEncrypter, time.Hour*time.Duration(resetTokenExpirationHours)),
		auth.WithDefaultJWTManagerOption(ec256PublicKey, ec256PrivateKey),
		auth.WithDefaultRoleManagerOption(authDBConnectionPool, data.OwnerUserRole.String()),
		auth.WithDefaultMFAManagerOption(authDBConnectionPool),
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
