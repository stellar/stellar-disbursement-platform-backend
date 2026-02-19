package serve

import (
	"context"
	"fmt"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	supporthttp "github.com/stellar/go-stellar-sdk/support/http"
	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	coreSvc "github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/internal/httphandler"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/internal/provisioning"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type HTTPServerInterface interface {
	Run(conf supporthttp.Config)
}

type HTTPServer struct{}

func (h *HTTPServer) Run(conf supporthttp.Config) {
	supporthttp.Run(conf)
}

type ServeOptions struct {
	AdminDBConnectionPool                   db.DBConnectionPool
	CrashTrackerClient                      crashtracker.CrashTrackerClient
	EmailMessengerClient                    message.MessengerClient
	Environment                             string
	GitCommit                               string
	Models                                  *data.Models
	MTNDBConnectionPool                     db.DBConnectionPool
	NetworkPassphrase                       string
	networkType                             utils.NetworkType
	Port                                    int
	SubmitterEngine                         engine.SubmitterEngine
	DistributionAccountService              coreSvc.DistributionAccountServiceInterface
	TenantAccountNativeAssetBootstrapAmount int
	tenantManager                           tenant.ManagerInterface
	tenantProvisioningManager               *provisioning.Manager
	Version                                 string
	AdminAccount                            string
	AdminAPIKey                             string
	SingleTenantMode                        bool
	BaseURL                                 string
	SDPUIBaseURL                            string
	DisableMFA                              bool
	DisableReCAPTCHA                        bool
}

// SetupDependencies uses the serve options to setup the dependencies for the server.
func (opts *ServeOptions) SetupDependencies() error {
	var err error
	opts.tenantManager = tenant.NewManager(
		tenant.WithDatabase(opts.AdminDBConnectionPool),
		tenant.WithSingleTenantMode(opts.SingleTenantMode),
	)
	opts.tenantProvisioningManager, err = provisioning.NewManager(provisioning.ManagerOptions{
		DBConnectionPool:           opts.AdminDBConnectionPool,
		TenantManager:              opts.tenantManager,
		SubmitterEngine:            opts.SubmitterEngine,
		NativeAssetBootstrapAmount: opts.TenantAccountNativeAssetBootstrapAmount,
	})
	if err != nil {
		return fmt.Errorf("creating provisioning manager: %w", err)
	}

	opts.networkType, err = utils.GetNetworkTypeFromNetworkPassphrase(opts.NetworkPassphrase)
	if err != nil {
		return fmt.Errorf("parsing network type: %w", err)
	}

	opts.Models, err = data.NewModels(opts.MTNDBConnectionPool)
	if err != nil {
		return fmt.Errorf("creating models: %w", err)
	}

	return nil
}

func StartServe(opts ServeOptions, httpServer HTTPServerInterface) error {
	if err := opts.SetupDependencies(); err != nil {
		return fmt.Errorf("starting dependencies: %w", err)
	}

	// Start the server
	listenAddr := fmt.Sprintf(":%d", opts.Port)
	serverConfig := supporthttp.Config{
		ListenAddr:          listenAddr,
		Handler:             handleHTTP(&opts),
		TCPKeepAlive:        time.Minute * 3,
		ShutdownGracePeriod: time.Second * 50,
		ReadTimeout:         time.Second * 5,
		WriteTimeout:        time.Second * 50,
		IdleTimeout:         time.Minute * 2,
		OnStarting: func() {
			log.Info("Starting Tenant Server")
			log.Infof("Listening on %s", listenAddr)
		},
		OnStopping: func() {
			log.Info("Closing the Tenant Server database connection pool")
			err := db.CloseConnectionPoolIfNeeded(context.Background(), opts.AdminDBConnectionPool)
			if err != nil {
				log.Errorf("error closing database connection: %v", err)
			}

			log.Info("Stopping Tenant Server")
		},
	}
	httpServer.Run(serverConfig)
	return nil
}

func handleHTTP(opts *ServeOptions) *chi.Mux {
	mux := chi.NewMux()

	mux.Use(chimiddleware.RequestID)
	mux.Use(chimiddleware.RealIP)
	mux.Use(supporthttp.LoggingMiddleware)
	mux.Use(middleware.RecoverHandler)
	mux.Use(middleware.MaxBodySize(middleware.DefaultMaxRequestBodySize))

	mux.Get("/health", httphandler.HealthHandler{
		GitCommit: opts.GitCommit,
		Version:   opts.Version,
	}.ServeHTTP)

	// Authenticated Routes
	mux.Group(func(r chi.Router) {
		r.Use(middleware.BasicAuthMiddleware(opts.AdminAccount, opts.AdminAPIKey))

		r.Route("/tenants", func(r chi.Router) {
			tenantsHandler := httphandler.TenantsHandler{
				Manager:                     opts.tenantManager,
				ProvisioningManager:         opts.tenantProvisioningManager,
				NetworkType:                 opts.networkType,
				AdminDBConnectionPool:       opts.AdminDBConnectionPool,
				SingleTenantMode:            opts.SingleTenantMode,
				Models:                      opts.Models,
				DistributionAccountResolver: opts.SubmitterEngine.DistributionAccountResolver,
				MessengerClient:             opts.EmailMessengerClient,
				CrashTrackerClient:          opts.CrashTrackerClient,
				DistributionAccountService:  opts.DistributionAccountService,
				BaseURL:                     opts.BaseURL,
				SDPUIBaseURL:                opts.SDPUIBaseURL,
				DisableMFA:                  opts.DisableMFA,
				DisableReCAPTCHA:            opts.DisableReCAPTCHA,
			}
			r.Get("/", tenantsHandler.GetAll)
			r.Post("/", tenantsHandler.Post)
			r.Get("/{arg}", tenantsHandler.GetByIDOrName)
			r.Delete("/{id}", tenantsHandler.Delete)
			r.Patch("/{id}", tenantsHandler.Patch)
			r.Post("/default-tenant", tenantsHandler.SetDefault)
		})
	})

	return mux
}
