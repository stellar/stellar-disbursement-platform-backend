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
	"github.com/shopspring/decimal"
	"github.com/stellar/go-stellar-sdk/amount"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/keypair"
	protocol "github.com/stellar/go-stellar-sdk/protocols/rpc"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"
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
	DisbursedAssetIssuer       string
	WalletName                 string
	WalletHomepage             string
	WalletDeepLink             string
	WalletSEP10Domain          string
	WalletIsEmbedded           bool
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
	RPCUrl                     string
	NetworkPassphrase          string
	SingleTenantMode           bool
	EmbeddedWalletsWasmHash    string
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
	rpcClient             stellar.RPCClient
	networkPassphrase     string
	sdpSepServices        SDPSepServicesIntegrationTestsInterface
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

	rpcClient, err := dependencyinjection.NewRPCClient(ctx, stellar.RPCOptions{RPCUrl: opts.RPCUrl})
	if err != nil {
		log.Ctx(ctx).Warnf("Failed to initialize RPC client for embedded wallets: %v", err)
		return
	}
	it.rpcClient = rpcClient
	it.networkPassphrase = opts.NetworkPassphrase
	embeddedWalletService, err := services.NewEmbeddedWalletService(it.models, opts.EmbeddedWalletsWasmHash, rpcClient)
	if err != nil {
		log.Ctx(ctx).Warnf("Failed to initialize embedded wallet service: %v", err)
		return
	}
	it.embeddedWalletService = embeddedWalletService
	log.Ctx(ctx).Info("Embedded wallet service initialized successfully")
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

	asset, err := it.models.Assets.GetByCodeAndIssuer(ctx, opts.DisbursedAssetCode, opts.DisbursedAssetIssuer)
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
		wallet, err := it.models.Wallets.GetByWalletName(ctx, opts.WalletName)
		if err != nil {
			return nil, fmt.Errorf("getting test wallet: %w", err)
		}
		walletID = wallet.ID

		// Only set verification field for non-embedded wallets
		if !wallet.Embedded {
			verificationField = data.VerificationTypeDateOfBirth
		}
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
	err = validateStellarTransaction(hPayment, intendedPaymentDestination, opts.DisbursedAssetCode, opts.DisbursedAssetIssuer, receivers[0].Payment.Amount)
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
	_, err = it.models.Assets.GetOrCreate(ctx, opts.DisbursedAssetCode, opts.DisbursedAssetIssuer)
	if err != nil {
		return fmt.Errorf("getting or creating test asset: %w", err)
	}

	_, err = it.models.Wallets.GetOrCreate(ctx, opts.WalletName, opts.WalletHomepage, opts.WalletDeepLink, opts.WalletSEP10Domain, opts.WalletIsEmbedded)
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

	// Phase 1: Admin creates and starts disbursement
	log.Ctx(ctx).Info("=== Phase 1: Admin creates and starts disbursement ===")
	log.Ctx(ctx).Info("Login user to get server API auth token")
	authToken, err := it.serverAPI.Login(ctx)
	if err != nil {
		return fmt.Errorf("trying to login in server API: %w", err)
	}

	asset, err := it.models.Assets.GetByCodeAndIssuer(ctx, opts.DisbursedAssetCode, opts.DisbursedAssetIssuer)
	if err != nil {
		return fmt.Errorf("getting test asset: %w", err)
	}

	// Create embedded wallet disbursement
	disbursement, receiverWalletID, err := it.createEmbeddedWalletDisbursement(ctx, opts, authToken, asset)
	if err != nil {
		return fmt.Errorf("creating embedded wallet disbursement: %w", err)
	}
	log.Ctx(ctx).Infof("✅ Disbursement created and started: %s", disbursement.ID)

	// Phase 2: System sends invitation → retrieve embedded wallet token
	log.Ctx(ctx).Info("=== Phase 2: System sent invitation - retrieving embedded wallet token ===")
	embeddedWallet, err := it.waitForEmbeddedWalletToken(ctx, receiverWalletID)
	if err != nil {
		return fmt.Errorf("waiting for embedded wallet token: %w", err)
	}
	log.Ctx(ctx).Infof("✅ Retrieved embedded wallet token: %s", embeddedWallet.Token)

	// Phase 3: Simulate receiver creating embedded wallet
	log.Ctx(ctx).Info("=== Phase 3: Simulating receiver creating embedded wallet ===")
	if err = it.createEmbeddedWallet(ctx, embeddedWallet.Token); err != nil {
		return fmt.Errorf("simulating receiver registration: %w", err)
	}
	log.Ctx(ctx).Info("✅ Receiver registration API called - waiting for contract deployment")

	// Phase 4: Wait for contract deployment
	log.Ctx(ctx).Info("=== Phase 4: Waiting for contract deployment ===")
	contractAddress, err := it.waitForContractDeployment(ctx, embeddedWallet.Token)
	if err != nil {
		return fmt.Errorf("waiting for contract deployment: %w", err)
	}
	log.Ctx(ctx).Infof("✅ Contract deployed at: %s", contractAddress)

	// Phase 5: Complete verification if required (OTP + DOB)
	log.Ctx(ctx).Info("=== Phase 5: Skipping OTP and PII verification (RequiresVerification=false) ===")

	// Phase 6: Verify receiver wallet was registered
	log.Ctx(ctx).Info("=== Phase 6: Verifying receiver wallet registration ===")
	if err = it.verifyReceiverWalletRegistration(ctx, receiverWalletID, contractAddress); err != nil {
		return fmt.Errorf("verifying receiver wallet registration: %w", err)
	}
	log.Ctx(ctx).Info("✅ Receiver wallet registered with contract address")

	// Phase 7: Verify disbursement reaches contract
	log.Ctx(ctx).Info("=== Phase 7: Verifying disbursement to contract ===")
	if err = it.ensureContractTransactionCompletion(ctx, disbursement); err != nil {
		return fmt.Errorf("ensuring contract transaction completion: %w", err)
	}

	log.Ctx(ctx).Info("🎉🎉🎉 Successfully finished embedded wallet integration tests! Payment delivered to contract! 🎉🎉🎉")
	return nil
}

// createEmbeddedWalletDisbursement creates a disbursement for embedded wallet flow.
// Returns the disbursement and the receiver_wallet_id that will be linked to the embedded wallet.
// This reuses createAndValidateDisbursement by setting the appropriate options.
func (it *IntegrationTestsService) createEmbeddedWalletDisbursement(
	ctx context.Context,
	opts IntegrationTestsOpts,
	authToken *ServerAPIAuthToken,
	asset *data.Asset,
) (*data.Disbursement, string, error) {
	embeddedOpts := opts
	embeddedOpts.WalletName = "Embedded Wallet"
	embeddedOpts.WalletIsEmbedded = true
	embeddedOpts.DisbursementName = opts.DisbursementName + "-embedded-" + time.Now().Format("20060102150405")

	disbursement, err := it.createAndValidateDisbursement(ctx, embeddedOpts, authToken, asset)
	if err != nil {
		return nil, "", fmt.Errorf("creating disbursement: %w", err)
	}

	// Get the receiverWalletID that was created when the disbursement was processed
	receivers, err := it.models.DisbursementReceivers.GetAll(ctx, it.mtnDBConnectionPool, &data.QueryParams{}, disbursement.ID)
	if err != nil {
		return nil, "", fmt.Errorf("getting receivers: %w", err)
	}
	if len(receivers) == 0 {
		return nil, "", fmt.Errorf("no receivers found for disbursement")
	}
	if receivers[0].ReceiverWallet == nil {
		return nil, "", fmt.Errorf("receiver wallet not found for receiver %s", receivers[0].ID)
	}
	receiverWalletID := receivers[0].ReceiverWallet.ID
	log.Ctx(ctx).Infof("Found receiver wallet: %s for receiver: %s", receiverWalletID, receivers[0].ID)

	return disbursement, receiverWalletID, nil
}

// waitForEmbeddedWalletToken waits for the embedded wallet record to be created with a token
func (it *IntegrationTestsService) waitForEmbeddedWalletToken(ctx context.Context, receiverWalletID string) (*data.EmbeddedWallet, error) {
	var embeddedWallet *data.EmbeddedWallet

	err := retry.Do(
		func() error {
			statuses := []data.EmbeddedWalletStatus{
				data.PendingWalletStatus,
				data.ProcessingWalletStatus,
				data.SuccessWalletStatus,
			}
			wallet, innerErr := it.models.EmbeddedWallets.GetByReceiverWalletIDAndStatuses(ctx, it.mtnDBConnectionPool, receiverWalletID, statuses)
			if innerErr != nil {
				return fmt.Errorf("embedded wallet not yet created for receiver wallet %s: %w", receiverWalletID, innerErr)
			}

			if wallet.Token == "" {
				return fmt.Errorf("embedded wallet exists but token is empty")
			}

			embeddedWallet = wallet
			return nil
		},
		retry.Context(ctx),
		retry.Attempts(10),
		retry.Delay(2*time.Second),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			log.Ctx(ctx).Infof("🔄 Waiting for embedded wallet token, attempt #%d: %v", n+1, err)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("waiting for embedded wallet token: %w", err)
	}

	return embeddedWallet, nil
}

// createEmbeddedWallet simulates a receiver clicks the invitation link and creates embedded wallet.
func (it *IntegrationTestsService) createEmbeddedWallet(ctx context.Context, token string) error {
	publicKeyHex, err := generateP256PublicKeyHex()
	if err != nil {
		return fmt.Errorf("generating P256 public key: %w", err)
	}
	credentialID := generateCredentialID()
	log.Ctx(ctx).Infof("Generated credential ID: %s for token: %s", credentialID, token)

	_, err = it.serverAPI.CreateEmbeddedWallet(ctx, &httphandler.CreateWalletRequest{
		Token:        token,
		PublicKey:    publicKeyHex,
		CredentialID: credentialID,
	})
	if err != nil {
		return fmt.Errorf("creating embedded wallet: %w", err)
	}
	log.Ctx(ctx).Info("Embedded wallet created successfully")

	return nil
}

// waitForContractDeployment waits for the embedded wallet contract to be deployed.
func (it *IntegrationTestsService) waitForContractDeployment(ctx context.Context, token string) (string, error) {
	var contractAddress string

	err := retry.Do(
		func() error {
			wallet, innerErr := it.models.EmbeddedWallets.GetByToken(ctx, it.mtnDBConnectionPool, token)
			if innerErr != nil {
				return fmt.Errorf("getting embedded wallet: %w", innerErr)
			}

			log.Ctx(ctx).Infof("Wallet status: %s, contract_address: %s", wallet.WalletStatus, wallet.ContractAddress)

			if wallet.WalletStatus == data.FailedWalletStatus {
				return retry.Unrecoverable(fmt.Errorf("wallet deployment failed"))
			}

			if wallet.WalletStatus != data.SuccessWalletStatus || wallet.ContractAddress == "" {
				return fmt.Errorf("wallet not ready yet, status=%s", wallet.WalletStatus)
			}

			if !strkey.IsValidContractAddress(wallet.ContractAddress) {
				return retry.Unrecoverable(fmt.Errorf("invalid contract address: %s", wallet.ContractAddress))
			}

			contractAddress = wallet.ContractAddress
			return nil
		},
		retry.Context(ctx),
		retry.Attempts(30),
		retry.Delay(10*time.Second),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			log.Ctx(ctx).Infof("🔄 Polling contract deployment, attempt #%d: %v", n+1, err)
		}),
	)
	if err != nil {
		return "", fmt.Errorf("waiting for contract deployment: %w", err)
	}

	return contractAddress, nil
}

// verifyReceiverWalletRegistration verifies that the receiver wallet was registered.
func (it *IntegrationTestsService) verifyReceiverWalletRegistration(ctx context.Context, receiverWalletID, expectedContractAddress string) error {
	receiverWallet, err := it.models.ReceiverWallet.GetByID(ctx, it.mtnDBConnectionPool, receiverWalletID)
	if err != nil {
		return fmt.Errorf("getting receiver wallet: %w", err)
	}

	if receiverWallet.Status != data.RegisteredReceiversWalletStatus {
		return fmt.Errorf("receiver wallet not registered, status=%s", receiverWallet.Status)
	}

	if receiverWallet.StellarAddress != expectedContractAddress {
		return fmt.Errorf("receiver wallet stellar address mismatch: expected=%s, got=%s", expectedContractAddress, receiverWallet.StellarAddress)
	}

	log.Ctx(ctx).Infof("Receiver wallet %s registered with contract address %s", receiverWalletID, expectedContractAddress)
	return nil
}

// ensureContractTransactionCompletion waits for the payment to complete.
func (it *IntegrationTestsService) ensureContractTransactionCompletion(
	ctx context.Context,
	disbursement *data.Disbursement,
) error {
	log.Ctx(ctx).Info("Waiting for payment to contract to be processed...")
	var payment *data.Payment
	var receivers []*data.DisbursementReceiver

	time.Sleep(paymentProcessTimeSeconds * time.Second)
	err := retry.Do(
		func() error {
			var innerErr error
			receivers, innerErr = it.models.DisbursementReceivers.GetAll(ctx, it.mtnDBConnectionPool, &data.QueryParams{}, disbursement.ID)
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
		retry.Attempts(30),
		retry.Delay(10*time.Second),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			log.Ctx(ctx).Infof("🔄 Polling payment completion, attempt #%d: %v", n+1, err)
		}),
	)
	if err != nil {
		return fmt.Errorf("waiting for payment to be processed: %w", err)
	}

	log.Ctx(ctx).Infof("✅ Contract payment completed! Transaction ID: %s", payment.StellarTransactionID)

	// Verify on-chain balance
	if it.rpcClient == nil {
		log.Ctx(ctx).Warn("RPC client not available, skipping on-chain balance verification")
		return nil
	}

	if len(receivers) == 0 {
		return fmt.Errorf("no receivers found for balance verification")
	}
	if receivers[0].ReceiverWallet == nil {
		return fmt.Errorf("receiver wallet is nil")
	}
	receiverContractAddress := receivers[0].ReceiverWallet.StellarAddress
	if receiverContractAddress == "" {
		return fmt.Errorf("receiver wallet has no stellar address")
	}

	log.Ctx(ctx).Infof("Verifying on-chain balance for contract %s...", receiverContractAddress)

	actualBalance, err := it.getContractTokenBalance(ctx, receiverContractAddress, disbursement.Asset)
	if err != nil {
		return fmt.Errorf("getting contract token balance: %w", err)
	}

	expectedAmount, err := decimal.NewFromString(payment.Amount)
	if err != nil {
		return fmt.Errorf("parsing payment amount: %w", err)
	}

	if actualBalance.LessThan(expectedAmount) {
		return fmt.Errorf("on-chain balance %s is less than expected %s", actualBalance, expectedAmount)
	}

	log.Ctx(ctx).Infof("✅ On-chain balance verified: %s %s", actualBalance, disbursement.Asset.Code)
	return nil
}

// getContractTokenBalance queries the SAC balance for a contract address by simulating a SEP-41 balance() call.
func (it *IntegrationTestsService) getContractTokenBalance(
	ctx context.Context,
	contractAddress string,
	asset *data.Asset,
) (decimal.Decimal, error) {
	// Get the SAC contract ID from the asset
	var txnAsset txnbuild.Asset
	if asset.IsNative() {
		txnAsset = txnbuild.NativeAsset{}
	} else {
		txnAsset = txnbuild.CreditAsset{
			Code:   asset.Code,
			Issuer: asset.Issuer,
		}
	}

	xdrAsset, err := txnAsset.ToXDR()
	if err != nil {
		return decimal.Zero, fmt.Errorf("converting asset to XDR: %w", err)
	}

	sacContractIDBytes, err := xdrAsset.ContractID(it.networkPassphrase)
	if err != nil {
		return decimal.Zero, fmt.Errorf("getting SAC contract ID: %w", err)
	}
	sacContractID := xdr.ContractId(sacContractIDBytes)

	// Parse the receiver contract address
	receiverContractIDBytes, err := strkey.Decode(strkey.VersionByteContract, contractAddress)
	if err != nil {
		return decimal.Zero, fmt.Errorf("decoding contract address: %w", err)
	}
	var receiverContractID xdr.ContractId
	copy(receiverContractID[:], receiverContractIDBytes)

	// Build the SEP-41 balance() invocation
	hostFunction := xdr.HostFunction{
		Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
		InvokeContract: &xdr.InvokeContractArgs{
			ContractAddress: xdr.ScAddress{
				Type:       xdr.ScAddressTypeScAddressTypeContract,
				ContractId: &sacContractID,
			},
			FunctionName: "balance",
			Args: xdr.ScVec{
				xdr.ScVal{
					Type: xdr.ScValTypeScvAddress,
					Address: &xdr.ScAddress{
						Type:       xdr.ScAddressTypeScAddressTypeContract,
						ContractId: &receiverContractID,
					},
				},
			},
		},
	}

	// Build transaction for simulation
	txParams := txnbuild.TransactionParams{
		SourceAccount: &txnbuild.SimpleAccount{
			AccountID: keypair.MustRandom().Address(),
			Sequence:  0,
		},
		BaseFee: int64(txnbuild.MinBaseFee),
		Preconditions: txnbuild.Preconditions{
			TimeBounds: txnbuild.NewTimeout(300),
		},
		Operations: []txnbuild.Operation{&txnbuild.InvokeHostFunction{
			HostFunction: hostFunction,
		}},
	}

	tx, err := txnbuild.NewTransaction(txParams)
	if err != nil {
		return decimal.Zero, fmt.Errorf("building simulation transaction: %w", err)
	}

	txEnvelope, err := tx.Base64()
	if err != nil {
		return decimal.Zero, fmt.Errorf("encoding simulation transaction: %w", err)
	}

	// Simulate the transaction
	result, simErr := it.rpcClient.SimulateTransaction(ctx, protocol.SimulateTransactionRequest{
		Transaction: txEnvelope,
	})
	if simErr != nil {
		return decimal.Zero, fmt.Errorf("simulating balance call: %w", simErr)
	}

	if len(result.Response.Results) == 0 {
		return decimal.Zero, fmt.Errorf("no results from balance simulation")
	}

	returnValueXDR := result.Response.Results[0].ReturnValueXDR
	if returnValueXDR == nil {
		return decimal.Zero, fmt.Errorf("no return value from balance simulation")
	}

	var returnValue xdr.ScVal
	err = xdr.SafeUnmarshalBase64(*returnValueXDR, &returnValue)
	if err != nil {
		return decimal.Zero, fmt.Errorf("unmarshaling return value: %w", err)
	}

	i128, ok := returnValue.GetI128()
	if !ok {
		return decimal.Zero, fmt.Errorf("return value is not i128")
	}

	balanceStr := amount.String128(i128)
	balance, err := decimal.NewFromString(balanceStr)
	if err != nil {
		return decimal.Zero, fmt.Errorf("parsing balance string: %w", err)
	}
	return balance, nil
}

// generateP256PublicKeyHex generates a random P256 key pair and returns the public key.
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
