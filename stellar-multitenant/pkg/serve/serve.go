package serve

import (
	"context"
	"fmt"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	supporthttp "github.com/stellar/go/support/http"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/internal/httphandler"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/internal/provisioning"
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
	DatabaseDSN               string
	dbConnectionPool          db.DBConnectionPool
	EmailMessengerClient      message.MessengerClient
	Environment               string
	GitCommit                 string
	HorizonURL                string
	NetworkPassphrase         string
	networkType               utils.NetworkType
	Port                      int
	SignatureServiceOptions   signing.SignatureServiceOptions
	tenantManager             *tenant.Manager
	tenantProvisioningManager *provisioning.Manager
	Version                   string
}

// SetupDependencies uses the serve options to setup the dependencies for the server.
func (opts *ServeOptions) SetupDependencies() error {
	// Setup Database:
	dbConnectionPool, err := db.OpenDBConnectionPool(opts.DatabaseDSN)
	if err != nil {
		return fmt.Errorf("connecting to the database: %w", err)
	}

	opts.dbConnectionPool = dbConnectionPool

	opts.tenantManager = tenant.NewManager(tenant.WithDatabase(dbConnectionPool))

	signSvc, err := dependencyinjection.NewSignatureService(context.Background(), opts.SignatureServiceOptions)
	if err != nil {
		return fmt.Errorf("creating signature service: %w", err)
	}

	opts.tenantProvisioningManager = provisioning.NewManager(
		provisioning.WithDatabase(dbConnectionPool),
		provisioning.WithTenantManager(opts.tenantManager),
		provisioning.WithMessengerClient(opts.EmailMessengerClient),
		provisioning.WithSignatureService(signSvc),
	)

	opts.networkType, err = utils.GetNetworkTypeFromNetworkPassphrase(opts.NetworkPassphrase)
	if err != nil {
		return fmt.Errorf("parsing network type: %w", err)
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
		WriteTimeout:        time.Second * 35,
		IdleTimeout:         time.Minute * 2,
		OnStarting: func() {
			log.Info("Starting Tenant Server")
			log.Infof("Listening on %s", listenAddr)
		},
		OnStopping: func() {
			log.Info("Closing the Tenant Server database connection pool")
			err := db.CloseConnectionPoolIfNeeded(context.Background(), opts.dbConnectionPool)
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

	mux.Get("/health", httphandler.HealthHandler{
		GitCommit: opts.GitCommit,
		Version:   opts.Version,
	}.ServeHTTP)

	mux.Route("/tenants", func(r chi.Router) {
		tenantsHandler := httphandler.TenantsHandler{
			Manager:             opts.tenantManager,
			ProvisioningManager: opts.tenantProvisioningManager,
			NetworkType:         opts.networkType,
		}
		r.Get("/", tenantsHandler.GetAll)
		r.Post("/", tenantsHandler.Post)
		r.Get("/{arg}", tenantsHandler.GetByIDOrName)
		r.Patch("/{id}", tenantsHandler.Patch)
	})

	return mux
}
