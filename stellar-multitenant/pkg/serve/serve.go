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
	"github.com/stellar/stellar-disbursement-platform-backend/db/router"
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
	tssDBConnectionPool       db.DBConnectionPool
	EmailMessengerClient      message.MessengerClient
	Environment               string
	GitCommit                 string
	NetworkPassphrase         string
	networkType               utils.NetworkType
	Port                      int
	SignatureServiceOptions   signing.SignatureServiceOptions
	tenantManager             *tenant.Manager
	tenantProvisioningManager *provisioning.Manager
	Version                   string
	AdminAccount              string
	AdminApiKey               string
}

// SetupDependencies uses the serve options to setup the dependencies for the server.
func (opts *ServeOptions) SetupDependencies() error {
	// Setup Database:
	dbConnectionPool, err := db.OpenDBConnectionPool(opts.DatabaseDSN)
	if err != nil {
		return fmt.Errorf("connecting to the database: %w", err)
	}

	opts.dbConnectionPool = dbConnectionPool

	// We need to use a dbConnectionPool that resolves to the tss namespace for the distribution account signature client.
	tssDBConnectionPool, err := router.GetDBForTSSSchema(opts.DatabaseDSN)
	if err != nil {
		return fmt.Errorf("getting TSS DBConnectionPool: %w", err)
	}
	opts.tssDBConnectionPool = tssDBConnectionPool

	opts.tenantManager = tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
	distAccSigClient, err := signing.NewSignatureClient(opts.SignatureServiceOptions.DistributionSignerType, signing.SignatureClientOptions{
		NetworkPassphrase:           opts.NetworkPassphrase,
		DistributionPrivateKey:      opts.SignatureServiceOptions.DistributionPrivateKey,
		DistAccEncryptionPassphrase: opts.SignatureServiceOptions.DistAccEncryptionPassphrase,
		DBConnectionPool:            tssDBConnectionPool,
	})
	if err != nil {
		return fmt.Errorf("creating a new distribution account signature client: %w", err)
	}

	opts.tenantProvisioningManager = provisioning.NewManager(
		provisioning.WithDatabase(opts.dbConnectionPool),
		provisioning.WithTenantManager(opts.tenantManager),
		provisioning.WithMessengerClient(opts.EmailMessengerClient),
		provisioning.WithDistributionAccountSignatureClient(distAccSigClient),
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
			ctx := context.Background()
			err := db.CloseConnectionPoolIfNeeded(ctx, opts.dbConnectionPool)
			if err != nil {
				log.Errorf("error closing database connection: %v", err)
			}

			err = db.CloseConnectionPoolIfNeeded(ctx, opts.tssDBConnectionPool)
			if err != nil {
				log.Errorf("error closing tss database connection: %v", err)
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

	// Authenticated Routes
	mux.Group(func(r chi.Router) {
		r.Use(middleware.BasicAuthMiddleware(opts.AdminAccount, opts.AdminApiKey))

		r.Route("/tenants", func(r chi.Router) {
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
	})

	return mux
}
