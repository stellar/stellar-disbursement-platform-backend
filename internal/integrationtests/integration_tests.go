package integrationtests

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/google/uuid"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/support/log"
	"golang.org/x/crypto/bcrypt"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/router"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httphandler"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

const paymentProcessTimeSeconds = 30

type IntegrationTestsInterface interface {
	StartIntegrationTests(ctx context.Context, opts IntegrationTestsOpts) error
	CreateTestData(ctx context.Context, opts IntegrationTestsOpts) error
	StartEmbeddedWalletIntegrationTests(ctx context.Context, opts IntegrationTestsOpts) error
}

type IntegrationTestsOpts struct {
	DatabaseDSN                string
	TenantName                 string
	UserEmail                  string
	UserPassword               string
	RegistrationContactType    data.RegistrationContactType
	DistributionAccountType    string
	DisbursedAssetCode         string
	DisbursetAssetIssuer       string
	WalletName                 string
	WalletHomepage             string
	WalletDeepLink             string
	WalletSEP10Domain          string
	DisbursementName           string
	DisbursementCSVFilePath    string
	DisbursementCSVFileName    string
	ReceiverAccountPublicKey   string
	ReceiverAccountPrivateKey  string
	ReceiverAccountStellarMemo string
	Sep10SigningPublicKey      string
	RecaptchaSiteKey           string
	ServerAPIBaseURL           string
	AdminServerBaseURL         string
	AdminServerAccountID       string
	AdminServerAPIKey          string
	CircleUSDCWalletID         string
	CircleAPIKey               string
	HorizonURL                 string
	NetworkPassphrase          string
	SingleTenantMode           bool
	// Embedded wallet options for contract account testing
	EnableEmbeddedWallets   bool
	EmbeddedWalletsWasmHash string
	RPCUrl                  string
}

type IntegrationTestsService struct {
	models                *data.Models
	adminDBConnectionPool db.DBConnectionPool
	mtnDBConnectionPool   db.DBConnectionPool
	tssDBConnectionPool   db.DBConnectionPool
	tenantManager         *tenant.Manager
	serverAPI             ServerAPIIntegrationTestsInterface
	adminAPI              AdminAPIIntegrationTestsInterface
	horizonClient         horizonclient.ClientInterface
	sdpSepServices        SDPSepServicesIntegrationTestsInterface
	// Embedded wallet service for contract account testing
	embeddedWalletService services.EmbeddedWalletServiceInterface
}

// NewIntegrationTestsService is a function that create a new IntegrationTestsService instance.
func NewIntegrationTestsService(opts IntegrationTestsOpts) (*IntegrationTestsService, error) {
	adminDSN, err := router.GetDSNForAdmin(opts.DatabaseDSN)
	if err != nil {
		return nil, fmt.Errorf("getting admin database DSN: %w", err)
	}
	adminDBConnectionPool, err := db.OpenDBConnectionPool(adminDSN)
	if err != nil {
		return nil, fmt.Errorf("connecting to the database: %w", err)
	}
	tm := tenant.NewManager(tenant.WithDatabase(adminDBConnectionPool))
	tr := tenant.NewMultiTenantDataSourceRouter(tm)
	mtnDBConnectionPool, err := db.NewConnectionPoolWithRouter(tr)
	if err != nil {
		return nil, fmt.Errorf("getting multi-tenant database connection pool: %w", err)
	}
	models, err := data.NewModels(mtnDBConnectionPool)
	if err != nil {
		return nil, fmt.Errorf("creating models for integration tests: %w", err)
	}
	tssDSN, err := router.GetDSNForTSS(opts.DatabaseDSN)
	if err != nil {
		return nil, fmt.Errorf("getting TSS database DSN: %w", err)
	}
	tssDBConnectionPool, err := db.OpenDBConnectionPool(tssDSN)
	if err != nil {
		return nil, fmt.Errorf("connecting to the tss database: %w", err)
	}
	it := &IntegrationTestsService{
		models:                models,
		adminDBConnectionPool: adminDBConnectionPool,
		mtnDBConnectionPool:   mtnDBConnectionPool,
		tssDBConnectionPool:   tssDBConnectionPool,
		tenantManager:         tm,
	}

	// initialize admin api integration tests service
	it.adminAPI = &AdminAPIIntegrationTests{
		HTTPClient:      httpclient.DefaultClient(),
		AdminAPIBaseURL: opts.AdminServerBaseURL,
		AccountID:       opts.AdminServerAccountID,
		APIKey:          opts.AdminServerAPIKey,
	}

	return it, nil
}

func (it *IntegrationTestsService) initServices(ctx context.Context, opts IntegrationTestsOpts) {
	// initialize default testnet horizon client
	it.horizonClient = &horizonclient.Client{
		HorizonURL: opts.HorizonURL,
		HTTP:       httpclient.DefaultClient(),
	}

	it.sdpSepServices = &SDPSepServicesIntegrationTests{
		HTTPClient:                httpclient.DefaultClient(),
		SDPBaseURL:                opts.ServerAPIBaseURL,
		TenantName:                opts.TenantName,
		ReceiverAccountPublicKey:  opts.ReceiverAccountPublicKey,
		ReceiverAccountPrivateKey: opts.ReceiverAccountPrivateKey,
		Sep10SigningPublicKey:     opts.Sep10SigningPublicKey,
		DisbursedAssetCode:        opts.DisbursedAssetCode,
		NetworkPassphrase:         opts.NetworkPassphrase,
		SingleTenantMode:          opts.SingleTenantMode,
	}

	// initialize server api integration tests service
	it.serverAPI = &ServerAPIIntegrationTests{
		HTTPClient:              httpclient.DefaultClient(),
		TenantName:              opts.TenantName,
		ServerAPIBaseURL:        opts.ServerAPIBaseURL,
		UserEmail:               opts.UserEmail,
		UserPassword:            opts.UserPassword,
		DisbursementCSVFilePath: opts.DisbursementCSVFilePath,
		DisbursementCSVFileName: opts.DisbursementCSVFileName,
	}

	// initialize embedded wallet service if enabled
	if opts.EnableEmbeddedWallets && opts.RPCUrl != "" && opts.EmbeddedWalletsWasmHash != "" {
		rpcClient, err := dependencyinjection.NewRPCClient(ctx, stellar.RPCOptions{RPCUrl: opts.RPCUrl})
		if err != nil {
			log.Ctx(ctx).Warnf("Failed to initialize RPC client for embedded wallets: %v", err)
			return
		}
		embeddedWalletService, err := services.NewEmbeddedWalletService(it.models, opts.EmbeddedWalletsWasmHash, rpcClient)
		if err != nil {
			log.Ctx(ctx).Warnf("Failed to initialize embedded wallet service: %v", err)
			return
		}
		it.embeddedWalletService = embeddedWalletService
		log.Ctx(ctx).Info("Embedded wallet service initialized successfully")
	}
}

func (it *IntegrationTestsService) StartIntegrationTests(ctx context.Context, opts IntegrationTestsOpts) error {
	log.Ctx(ctx).Info("Starting integration tests......")
	it.initServices(context.Background(), opts)

	log.Ctx(ctx).Infof("Resolving tenant %s from database and adding it to context", opts.TenantName)
	t, err := it.tenantManager.GetTenantByName(ctx, opts.TenantName)
	if err != nil {
		return fmt.Errorf("getting tenant %s from database: %w", opts.TenantName, err)
	}
	ctx = sdpcontext.SetTenantInContext(ctx, t)

	log.Ctx(ctx).Info("Login user to get server API auth token")
	authToken, err := it.serverAPI.Login(ctx)
	if err != nil {
		return fmt.Errorf("trying to login in server API: %w", err)
	}
	log.Ctx(ctx).Infof("User logged in with server API auth token %q", authToken)

	asset, err := it.models.Assets.GetByCodeAndIssuer(ctx, opts.DisbursedAssetCode, opts.DisbursetAssetIssuer)
	if err != nil {
		return fmt.Errorf("getting test asset: %w", err)
	}

	disbursement, err := it.createAndValidateDisbursement(ctx, opts, authToken, asset)
	if err != nil {
		return fmt.Errorf("creating and validating disbursement: %w", err)
	}

	if err = it.registerReceiverWalletIfNeeded(ctx, opts, disbursement); err != nil {
		return fmt.Errorf("registering receiver wallet if needed: %w", err)
	}

	if err = it.ensureTransactionCompletion(ctx, opts, disbursement); err != nil {
		return err
	}

	log.Ctx(ctx).Info("🎉🎉🎉 Successfully finished integration tests! The disbursement was delivered to the recipient! 🎉🎉🎉")

	return nil
}

// createAndValidateDisbursement is a function that creates a disbursement and validates it.
func (it *IntegrationTestsService) createAndValidateDisbursement(ctx context.Context, opts IntegrationTestsOpts, authToken *ServerAPIAuthToken, asset *data.Asset) (*data.Disbursement, error) {
	var (
		verificationField data.VerificationType
		walletID          string
	)
	if !opts.RegistrationContactType.IncludesWalletAddress {
		log.Ctx(ctx).Infof("Getting test wallet in database...")
		verificationField = data.VerificationTypeDateOfBirth
		wallet, err := it.models.Wallets.GetByWalletName(ctx, opts.WalletName)
		if err != nil {
			return nil, fmt.Errorf("getting test wallet: %w", err)
		}
		walletID = wallet.ID
	}

	log.Ctx(ctx).Info("Creating disbursement using server API...")
	disbursement, err := it.serverAPI.CreateDisbursement(ctx, authToken, &httphandler.PostDisbursementRequest{
		Name:                    opts.DisbursementName,
		WalletID:                walletID,
		AssetID:                 asset.ID,
		VerificationField:       verificationField,
		RegistrationContactType: opts.RegistrationContactType,
	})
	if err != nil {
		return nil, fmt.Errorf("creating disbursement: %w", err)
	}

	log.Ctx(ctx).Info("Processing disbursement CSV file using server API...")
	if err = it.serverAPI.ProcessDisbursement(ctx, authToken, disbursement.ID); err != nil {
		return nil, fmt.Errorf("processing disbursement: %w", err)
	}

	log.Ctx(ctx).Info("Validating disbursement data after processing the disbursement file...")
	if err = validateExpectationsAfterProcessDisbursement(ctx, disbursement.ID, it.models, it.mtnDBConnectionPool); err != nil {
		return nil, fmt.Errorf("validating data after process disbursement: %w", err)
	}

	if !opts.RegistrationContactType.IncludesWalletAddress {
		log.Ctx(ctx).Info("Creating receiver wallets for SEP-24 registration flow...")
		if err = it.createReceiverWalletsForSEP24(ctx, disbursement.ID, walletID); err != nil {
			return nil, fmt.Errorf("creating receiver wallets for SEP-24: %w", err)
		}
	}

	log.Ctx(ctx).Info("Starting disbursement using server API...")
	if err = it.serverAPI.StartDisbursement(ctx, authToken, disbursement.ID, &httphandler.PatchDisbursementStatusRequest{Status: "STARTED"}); err != nil {
		return nil, fmt.Errorf("starting disbursement: %w", err)
	}

	log.Ctx(ctx).Info("Validating disbursement data after starting disbursement using server API...")
	if err = validateExpectationsAfterStartDisbursement(ctx, disbursement.ID, it.models, it.mtnDBConnectionPool); err != nil {
		return nil, fmt.Errorf("validating data after process disbursement: %w", err)
	}
	return disbursement, nil
}

func (it *IntegrationTestsService) registerReceiverWalletIfNeeded(ctx context.Context, opts IntegrationTestsOpts, disbursement *data.Disbursement) error {
	if disbursement.RegistrationContactType.IncludesWalletAddress {
		log.Ctx(ctx).Infof("⏭ Skipping SEP-24 flow because registrationContactType=%q", disbursement.RegistrationContactType)
		return nil
	}

	return it.registerWithInternalSEP(ctx, opts, disbursement)
}

func (it *IntegrationTestsService) registerWithInternalSEP(ctx context.Context, opts IntegrationTestsOpts, disbursement *data.Disbursement) error {
	log.Ctx(ctx).Info("Starting SEP-10 authentication with SDP internal service")

	// Step 1: Get SEP-10 challenge
	challenge, err := it.sdpSepServices.GetSEP10Challenge(ctx)
	if err != nil {
		return fmt.Errorf("getting SEP-10 challenge: %w", err)
	}
	log.Ctx(ctx).Info("Received SEP-10 challenge from SDP")

	// Step 2: Sign challenge
	signedChallenge, err := it.sdpSepServices.SignSEP10Challenge(challenge)
	if err != nil {
		return fmt.Errorf("signing SEP-10 challenge: %w", err)
	}
	log.Ctx(ctx).Info("Signed SEP-10 challenge")

	// Step 3: Validate and get JWT token
	sep10Token, err := it.sdpSepServices.ValidateSEP10Challenge(ctx, signedChallenge)
	if err != nil {
		return fmt.Errorf("validating SEP-10 challenge: %w", err)
	}
	log.Ctx(ctx).Info("Received SEP-10 JWT token")

	// Step 4: Initiate SEP-24 deposit
	depositResp, err := it.sdpSepServices.InitiateSEP24Deposit(ctx, sep10Token)
	if err != nil {
		return fmt.Errorf("initiating SEP-24 deposit: %w", err)
	}
	log.Ctx(ctx).Infof("Initiated SEP-24 deposit, transaction ID: %s", depositResp.TransactionID)

	// Step 5: Check transaction status (should be incomplete initially)
	txStatus, err := it.sdpSepServices.GetSEP24Transaction(ctx, sep10Token, depositResp.TransactionID)
	if err != nil {
		return fmt.Errorf("checking transaction status: %w", err)
	}
	log.Ctx(ctx).Infof("Transaction status: %s", txStatus.Transaction.Status)

	// Step 6: Read disbursement instructions
	disbursementInstructions, err := readDisbursementCSV(opts.DisbursementCSVFilePath, opts.DisbursementCSVFileName)
	if err != nil {
		return fmt.Errorf("reading disbursement CSV: %w", err)
	}

	// Step 7: Complete registration
	registrationData := &ReceiverRegistrationRequest{
		OTP:               data.TestnetAlwaysValidOTP,
		PhoneNumber:       disbursementInstructions[0].Phone,
		Email:             disbursementInstructions[0].Email,
		VerificationValue: disbursementInstructions[0].VerificationValue,
		VerificationField: string(disbursement.VerificationField),
		ReCAPTCHAToken:    opts.RecaptchaSiteKey,
	}

	if err = it.sdpSepServices.CompleteReceiverRegistration(ctx, depositResp.Token, registrationData); err != nil {
		return fmt.Errorf("completing receiver registration: %w", err)
	}
	log.Ctx(ctx).Info("Completed receiver registration")

	// Step 8: Verify transaction is completed
	txStatus, err = it.sdpSepServices.GetSEP24Transaction(ctx, sep10Token, depositResp.TransactionID)
	if err != nil {
		return fmt.Errorf("checking final transaction status: %w", err)
	}

	if txStatus.Transaction.Status != "completed" {
		return fmt.Errorf("transaction not completed, status: %s", txStatus.Transaction.Status)
	}

	// Step 9: Validate registration in database
	if err = validateExpectationsAfterReceiverRegistration(ctx, it.models, opts.ReceiverAccountPublicKey, opts.ReceiverAccountStellarMemo, opts.WalletSEP10Domain); err != nil {
		return fmt.Errorf("validating receiver after registration: %w", err)
	}

	log.Ctx(ctx).Info("✅ SEP-24 registration completed successfully")
	return nil
}

// ensureTransactionCompletion is a function that ensures the transaction completion by waiting for the payment to be
// processed by TSS or Circle, and then ensuring the transaction is present on the Stellar network.
func (it *IntegrationTestsService) ensureTransactionCompletion(ctx context.Context, opts IntegrationTestsOpts, disbursement *data.Disbursement) error {
	log.Ctx(ctx).Info("Waiting for payment to be processed...")
	var receivers []*data.DisbursementReceiver
	var payment *data.Payment

	time.Sleep(paymentProcessTimeSeconds * time.Second) // wait for payment to be processed by TSS or Circle
	err := retry.Do(
		func() error {
			var innerErr error
			receivers, innerErr = it.models.DisbursementReceivers.GetAll(ctx, it.mtnDBConnectionPool, &data.QueryParams{}, disbursement.ID)
			if innerErr != nil {
				return fmt.Errorf("getting receivers: %w", innerErr)
			}

			payment = receivers[0].Payment
			if payment.Status != data.SuccessPaymentStatus || payment.StellarTransactionID == "" {
				return fmt.Errorf("payment was not processed successfully by TSS or Circle within the expected time = %+v", payment)
			}

			return nil
		},
		retry.Context(ctx),
		retry.Attempts(6),
		retry.Delay(20*time.Second),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			log.Ctx(ctx).Infof("🔄 Retry attempt #%d: %v", n+1, err)
		}),
	)
	// Max elapsed time is `paymentProcessTimeSeconds + 20 * (6-1)` = 130 seconds
	if err != nil {
		return fmt.Errorf("waiting for payment to be processed: %w", err)
	}

	log.Ctx(ctx).Infof("Validating transaction %s is on the Stellar network...", payment.StellarTransactionID)
	hPayment, getPaymentErr := getTransactionOnHorizon(it.horizonClient, payment.StellarTransactionID)
	if getPaymentErr != nil {
		return fmt.Errorf("getting transaction on Stellar network: %w", getPaymentErr)
	}

	intendedPaymentDestination := opts.ReceiverAccountPublicKey
	if disbursement.RegistrationContactType.IncludesWalletAddress {
		var disbursementInstructions []*data.DisbursementInstruction
		disbursementInstructions, err = readDisbursementCSV(opts.DisbursementCSVFilePath, opts.DisbursementCSVFileName)
		if err != nil {
			return fmt.Errorf("reading disbursement CSV in ensureTransactionCompletion: %w", err)
		}
		intendedPaymentDestination = disbursementInstructions[0].WalletAddress
	}
	err = validateStellarTransaction(hPayment, intendedPaymentDestination, opts.DisbursedAssetCode, opts.DisbursetAssetIssuer, receivers[0].Payment.Amount)
	if err != nil {
		return fmt.Errorf("validating stellar transaction: %w", err)
	}
	log.Ctx(ctx).Info("Transaction validated")

	return nil
}

// createReceiverWalletsForSEP24 creates receiver wallet records for each receiver in the disbursement
// to enable SEP-24 registration flow
func (it *IntegrationTestsService) createReceiverWalletsForSEP24(ctx context.Context, disbursementID, walletID string) error {
	// Get all receivers from the disbursement
	receivers, err := it.models.DisbursementReceivers.GetAll(ctx, it.mtnDBConnectionPool, &data.QueryParams{}, disbursementID)
	if err != nil {
		return fmt.Errorf("getting receivers from disbursement: %w", err)
	}

	// Create receiver wallet for each receiver
	for _, receiver := range receivers {
		// Check if receiver wallet already exists
		existingWallets, err := it.models.ReceiverWallet.GetWithReceiverIDs(ctx, it.mtnDBConnectionPool, data.ReceiverIDs{receiver.ID})
		if err != nil {
			return fmt.Errorf("checking existing receiver wallets for receiver %s: %w", receiver.ID, err)
		}

		// If no receiver wallet exists for this receiver and wallet, create one
		walletExists := false
		for _, existingWallet := range existingWallets {
			if existingWallet.Wallet.ID == walletID {
				walletExists = true
				break
			}
		}

		if !walletExists {
			log.Ctx(ctx).Infof("Creating receiver wallet for receiver %s with wallet %s", receiver.ID, walletID)
			_, err = it.models.ReceiverWallet.GetOrInsertReceiverWallet(ctx, it.mtnDBConnectionPool, data.ReceiverWalletInsert{
				ReceiverID: receiver.ID,
				WalletID:   walletID,
			})
			if err != nil {
				return fmt.Errorf("creating receiver wallet for receiver %s: %w", receiver.ID, err)
			}
		}
	}

	log.Ctx(ctx).Infof("Successfully created receiver wallets for %d receivers", len(receivers))
	return nil
}

func (it *IntegrationTestsService) CreateTestData(ctx context.Context, opts IntegrationTestsOpts) error {
	// 1. Create new tenant and add owner user
	distributionAccType := schema.AccountType(opts.DistributionAccountType)

	// Use 3-part domain with tenant name for proper tenant extraction in SEP-24
	baseURL := fmt.Sprintf("http://%s.stellar.local:8000", opts.TenantName)

	t, err := it.adminAPI.CreateTenant(ctx, CreateTenantRequest{
		Name:                    opts.TenantName,
		OwnerEmail:              opts.UserEmail,
		OwnerFirstName:          "John",
		OwnerLastName:           "Doe",
		OrganizationName:        "Integration Tests Organization",
		DistributionAccountType: distributionAccType,
		BaseURL:                 baseURL,
		SDPUIBaseURL:            "http://localhost:3000",
	})
	if err != nil {
		return fmt.Errorf("creating tenant: %w", err)
	}

	ctx = sdpcontext.SetTenantInContext(ctx, t)

	// 2. Reset password for the user
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(opts.UserPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashing owner user password: %w", err)
	}
	query := `UPDATE auth_users SET encrypted_password = $1 WHERE email = $2`
	_, err = it.mtnDBConnectionPool.ExecContext(ctx, query, hashedPassword, opts.UserEmail)
	if err != nil {
		return fmt.Errorf("updating owner user password: %w", err)
	}

	// 3. Create test asset and wallet
	_, err = it.models.Assets.GetOrCreate(ctx, opts.DisbursedAssetCode, opts.DisbursetAssetIssuer)
	if err != nil {
		return fmt.Errorf("getting or creating test asset: %w", err)
	}

	_, err = it.models.Wallets.GetOrCreate(ctx, opts.WalletName, opts.WalletHomepage, opts.WalletDeepLink, opts.WalletSEP10Domain)
	if err != nil {
		return fmt.Errorf("getting or creating test wallet: %w", err)
	}

	// 4. Provision Circle distribution account if needed
	if distributionAccType.IsCircle() {
		// 4.1. Create Circle configuration by calling endpoint
		it.initServices(ctx, opts)
		authToken, loginErr := it.serverAPI.Login(ctx)
		if loginErr != nil {
			return fmt.Errorf("trying to login in server API: %w", loginErr)
		}

		err = it.serverAPI.ConfigureCircleAccess(ctx,
			authToken,
			&httphandler.PatchCircleConfigRequest{
				WalletID: &opts.CircleUSDCWalletID,
				APIKey:   &opts.CircleAPIKey,
			})
		if err != nil {
			return fmt.Errorf("configuring Circle access: %w", err)
		}
	}

	return nil
}

// ========================================
// EMBEDDED WALLET (CONTRACT ACCOUNT) INTEGRATION TESTS
// ========================================

// StartEmbeddedWalletIntegrationTests runs the full E2E test flow:
// 1. Create embedded wallet (contract account) via API
// 2. Wait for contract deployment
// 3. Create disbursement to the contract address
// 4. Verify payment reaches the contract via InvokeHostFunction
func (it *IntegrationTestsService) StartEmbeddedWalletIntegrationTests(ctx context.Context, opts IntegrationTestsOpts) error {
	log.Ctx(ctx).Info("Starting embedded wallet (contract account) integration tests...")
	it.initServices(ctx, opts)

	if it.embeddedWalletService == nil {
		return fmt.Errorf("embedded wallet service is not initialized - ensure EnableEmbeddedWallets=true, RPCUrl, and EmbeddedWalletsWasmHash are set")
	}

	log.Ctx(ctx).Infof("Resolving tenant %s from database and adding it to context", opts.TenantName)
	t, err := it.tenantManager.GetTenantByName(ctx, opts.TenantName)
	if err != nil {
		return fmt.Errorf("getting tenant %s from database: %w", opts.TenantName, err)
	}
	ctx = sdpcontext.SetTenantInContext(ctx, t)

	// Phase 1: Create embedded wallet and get contract address
	log.Ctx(ctx).Info("=== Phase 1: Creating embedded wallet (contract account) ===")
	contractAddress, err := it.createEmbeddedWallet(ctx)
	if err != nil {
		return fmt.Errorf("creating embedded wallet: %w", err)
	}
	log.Ctx(ctx).Infof("✅ Embedded wallet created with contract address: %s", contractAddress)

	// Phase 2: Run disbursement to contract address
	log.Ctx(ctx).Info("=== Phase 2: Creating disbursement to contract address ===")
	log.Ctx(ctx).Info("Login user to get server API auth token")
	authToken, err := it.serverAPI.Login(ctx)
	if err != nil {
		return fmt.Errorf("trying to login in server API: %w", err)
	}

	asset, err := it.models.Assets.GetByCodeAndIssuer(ctx, opts.DisbursedAssetCode, opts.DisbursetAssetIssuer)
	if err != nil {
		return fmt.Errorf("getting test asset: %w", err)
	}

	// Create disbursement with contract address
	disbursement, err := it.createContractDisbursement(ctx, opts, authToken, asset, contractAddress)
	if err != nil {
		return fmt.Errorf("creating contract disbursement: %w", err)
	}

	// Phase 3: Verify payment reaches contract
	log.Ctx(ctx).Info("=== Phase 3: Verifying payment to contract ===")
	if err = it.ensureContractTransactionCompletion(ctx, disbursement); err != nil {
		return fmt.Errorf("ensuring contract transaction completion: %w", err)
	}

	log.Ctx(ctx).Info("🎉🎉🎉 Successfully finished embedded wallet integration tests! Payment delivered to contract! 🎉🎉🎉")
	return nil
}

// createEmbeddedWallet creates an embedded wallet and waits for the contract to be deployed.
// Returns the contract address (C-address) once deployment is complete.
func (it *IntegrationTestsService) createEmbeddedWallet(ctx context.Context) (string, error) {
	// Step 1: Create invitation token using the service directly
	// (No HTTP endpoint for this - it's created internally when sending invitations)
	log.Ctx(ctx).Info("Creating invitation token...")
	token, err := it.embeddedWalletService.CreateInvitationToken(ctx)
	if err != nil {
		return "", fmt.Errorf("creating invitation token: %w", err)
	}
	log.Ctx(ctx).Infof("Created invitation token: %s", token)

	// Step 2: Generate P256 key pair and credential ID for wallet registration
	publicKeyHex, err := generateP256PublicKeyHex()
	if err != nil {
		return "", fmt.Errorf("generating P256 public key: %w", err)
	}
	credentialID := generateCredentialID()
	log.Ctx(ctx).Infof("Generated credential ID: %s", credentialID)

	// Step 3: Register embedded wallet via HTTP API
	log.Ctx(ctx).Info("Registering embedded wallet via API...")
	_, err = it.serverAPI.RegisterEmbeddedWallet(ctx, &RegisterEmbeddedWalletRequest{
		Token:        token,
		PublicKey:    publicKeyHex,
		CredentialID: credentialID,
	})
	if err != nil {
		return "", fmt.Errorf("registering embedded wallet: %w", err)
	}
	log.Ctx(ctx).Info("Embedded wallet registered, waiting for contract deployment...")

	// Step 4: Poll until contract is deployed (status = SUCCESS)
	var contractAddress string
	err = retry.Do(
		func() error {
			wallet, innerErr := it.serverAPI.GetEmbeddedWallet(ctx, credentialID)
			if innerErr != nil {
				return fmt.Errorf("getting embedded wallet: %w", innerErr)
			}

			log.Ctx(ctx).Infof("Wallet status: %s, contract_address: %s", wallet.Status, wallet.ContractAddress)

			if wallet.Status == data.FailedWalletStatus {
				return retry.Unrecoverable(fmt.Errorf("wallet deployment failed"))
			}

			if wallet.Status != data.SuccessWalletStatus || wallet.ContractAddress == "" {
				return fmt.Errorf("wallet not ready yet, status=%s", wallet.Status)
			}

			if !strkey.IsValidContractAddress(wallet.ContractAddress) {
				return retry.Unrecoverable(fmt.Errorf("invalid contract address: %s", wallet.ContractAddress))
			}

			contractAddress = wallet.ContractAddress
			return nil
		},
		retry.Context(ctx),
		retry.Attempts(30),          // 30 attempts
		retry.Delay(10*time.Second), // 10 seconds between attempts
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			log.Ctx(ctx).Infof("🔄 Polling wallet status, attempt #%d: %v", n+1, err)
		}),
	)
	if err != nil {
		return "", fmt.Errorf("waiting for contract deployment: %w", err)
	}

	return contractAddress, nil
}

// createContractDisbursement creates a disbursement targeting the given contract address.
func (it *IntegrationTestsService) createContractDisbursement(
	ctx context.Context,
	opts IntegrationTestsOpts,
	authToken *ServerAPIAuthToken,
	asset *data.Asset,
	contractAddress string,
) (*data.Disbursement, error) {
	// Create disbursement with phone_and_wallet_address registration type
	log.Ctx(ctx).Info("Creating disbursement for contract address...")
	disbursement, err := it.serverAPI.CreateDisbursement(ctx, authToken, &httphandler.PostDisbursementRequest{
		Name:                    opts.DisbursementName + "-contract-" + time.Now().Format("20060102150405"),
		AssetID:                 asset.ID,
		RegistrationContactType: data.RegistrationContactTypePhoneAndWalletAddress,
	})
	if err != nil {
		return nil, fmt.Errorf("creating disbursement: %w", err)
	}
	log.Ctx(ctx).Infof("Created disbursement: %s", disbursement.ID)

	// Process disbursement with contract address CSV
	// We need to create a temporary CSV with the actual contract address
	log.Ctx(ctx).Info("Processing disbursement with contract address...")
	if err = it.processContractDisbursement(ctx, authToken, disbursement.ID, contractAddress); err != nil {
		return nil, fmt.Errorf("processing contract disbursement: %w", err)
	}

	log.Ctx(ctx).Info("Validating disbursement data after processing...")
	if err = validateExpectationsAfterProcessDisbursement(ctx, disbursement.ID, it.models, it.mtnDBConnectionPool); err != nil {
		return nil, fmt.Errorf("validating data after process disbursement: %w", err)
	}

	log.Ctx(ctx).Info("Starting disbursement...")
	if err = it.serverAPI.StartDisbursement(ctx, authToken, disbursement.ID, &httphandler.PatchDisbursementStatusRequest{Status: "STARTED"}); err != nil {
		return nil, fmt.Errorf("starting disbursement: %w", err)
	}

	log.Ctx(ctx).Info("Validating disbursement data after starting...")
	if err = validateExpectationsAfterStartDisbursement(ctx, disbursement.ID, it.models, it.mtnDBConnectionPool); err != nil {
		return nil, fmt.Errorf("validating data after start disbursement: %w", err)
	}

	return disbursement, nil
}

// processContractDisbursement processes the disbursement with a CSV containing the contract address.
// This is similar to ProcessDisbursement but uses the contract address instead of reading from file.
func (it *IntegrationTestsService) processContractDisbursement(
	ctx context.Context,
	authToken *ServerAPIAuthToken,
	disbursementID string,
	contractAddress string,
) error {
	// Get the disbursement from the database
	disbursement, err := it.models.Disbursements.Get(ctx, it.mtnDBConnectionPool, disbursementID)
	if err != nil {
		return fmt.Errorf("getting disbursement: %w", err)
	}

	// Generate a unique phone number to avoid conflicts with previous test runs
	// Format: +1202555XXXX where XXXX is random
	phoneNumber := fmt.Sprintf("+1202555%04d", time.Now().UnixNano()%10000)

	// Create the instruction with the contract address
	instruction := &data.DisbursementInstruction{
		Phone:         phoneNumber,
		ID:            "1",
		Amount:        "0.1",
		WalletAddress: contractAddress,
	}

	log.Ctx(ctx).Infof("Using phone number %s for contract disbursement", phoneNumber)

	// Use the disbursement instructions model to process within a transaction
	return db.RunInTransaction(ctx, it.mtnDBConnectionPool, nil, func(dbTx db.DBTransaction) error {
		di := data.NewDisbursementInstructionModel(it.mtnDBConnectionPool)
		opts := data.DisbursementInstructionsOpts{
			UserID:       authToken.Token, // Using token as user ID for integration tests
			Instructions: []*data.DisbursementInstruction{instruction},
			Disbursement: disbursement,
			DisbursementUpdate: &data.DisbursementUpdate{
				ID:       disbursementID,
				FileName: "contract_disbursement.csv",
				FileContent: []byte(fmt.Sprintf(
					"phone,id,amount,walletAddress\n%s,1,0.1,%s", phoneNumber, contractAddress,
				)),
			},
			MaxNumberOfInstructions: 1000,
		}
		if processErr := di.ProcessAll(ctx, dbTx, opts); processErr != nil {
			return fmt.Errorf("processing disbursement instructions: %w", processErr)
		}
		return nil
	})
}

// ensureContractTransactionCompletion waits for the payment to complete and validates it on Horizon.
// Contract payments show up as InvokeHostFunction operations, not regular Payment operations.
func (it *IntegrationTestsService) ensureContractTransactionCompletion(
	ctx context.Context,
	disbursement *data.Disbursement,
) error {
	log.Ctx(ctx).Info("Waiting for payment to contract to be processed...")
	var payment *data.Payment

	time.Sleep(paymentProcessTimeSeconds * time.Second)
	err := retry.Do(
		func() error {
			receivers, innerErr := it.models.DisbursementReceivers.GetAll(ctx, it.mtnDBConnectionPool, &data.QueryParams{}, disbursement.ID)
			if innerErr != nil {
				return fmt.Errorf("getting receivers: %w", innerErr)
			}

			if len(receivers) == 0 {
				return fmt.Errorf("no receivers found for disbursement")
			}

			payment = receivers[0].Payment
			if payment.Status != data.SuccessPaymentStatus || payment.StellarTransactionID == "" {
				return fmt.Errorf("payment not processed yet, status=%s, txID=%s", payment.Status, payment.StellarTransactionID)
			}

			return nil
		},
		retry.Context(ctx),
		retry.Attempts(6),
		retry.Delay(20*time.Second),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			log.Ctx(ctx).Infof("🔄 Retry attempt #%d: %v", n+1, err)
		}),
	)
	if err != nil {
		return fmt.Errorf("waiting for payment to be processed: %w", err)
	}

	log.Ctx(ctx).Infof("Payment successful! Validating InvokeHostFunction on Horizon for tx: %s", payment.StellarTransactionID)

	// For contract payments, we expect InvokeHostFunction instead of Payment operation
	ihf, err := getInvokeHostFunctionOnHorizon(it.horizonClient, payment.StellarTransactionID)
	if err != nil {
		return fmt.Errorf("getting InvokeHostFunction on Horizon: %w", err)
	}

	if err = validateContractStellarTransaction(ihf); err != nil {
		return fmt.Errorf("validating contract transaction: %w", err)
	}

	log.Ctx(ctx).Infof("✅ Contract payment validated! Transaction hash: %s", ihf.TransactionHash)
	return nil
}

// generateP256PublicKeyHex generates a random P256 key pair and returns the public key
// as a hex-encoded uncompressed point (65 bytes, starting with 0x04).
func generateP256PublicKeyHex() (string, error) {
	privateKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generating P256 private key: %w", err)
	}
	publicKeyBytes := privateKey.PublicKey().Bytes()
	return hex.EncodeToString(publicKeyBytes), nil
}

// generateCredentialID generates a random credential ID for WebAuthn registration.
func generateCredentialID() string {
	return uuid.New().String()
}
