package integrationtests

import (
	"context"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/support/log"
	"golang.org/x/crypto/bcrypt"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/router"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httphandler"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

const paymentProcessTimeSeconds = 30

type IntegrationTestsInterface interface {
	StartIntegrationTests(ctx context.Context, opts IntegrationTestsOpts) error
	CreateTestData(ctx context.Context, opts IntegrationTestsOpts) error
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
	AnchorPlatformBaseSepURL   string
	ServerAPIBaseURL           string
	AdminServerBaseURL         string
	AdminServerAccountID       string
	AdminServerAPIKey          string
	CircleUSDCWalletID         string
	CircleAPIKey               string
	HorizonURL                 string
	NetworkPassphrase          string
	EnableAnchorPlatform       bool
	SingleTenantMode           bool
}

type IntegrationTestsService struct {
	models                *data.Models
	adminDBConnectionPool db.DBConnectionPool
	mtnDBConnectionPool   db.DBConnectionPool
	tssDBConnectionPool   db.DBConnectionPool
	tenantManager         *tenant.Manager
	serverAPI             ServerAPIIntegrationTestsInterface
	adminAPI              AdminAPIIntegrationTestsInterface
	anchorPlatform        AnchorPlatformIntegrationTestsInterface
	horizonClient         horizonclient.ClientInterface
	sdpSepServices        SDPSepServicesIntegrationTestsInterface
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

func (it *IntegrationTestsService) initServices(_ context.Context, opts IntegrationTestsOpts) {
	// initialize default testnet horizon client
	it.horizonClient = &horizonclient.Client{
		HorizonURL: opts.HorizonURL,
		HTTP:       httpclient.DefaultClient(),
	}

	if !opts.EnableAnchorPlatform {
		it.sdpSepServices = &SDPSepServicesIntegrationTests{
			HTTPClient:                httpclient.DefaultClient(),
			SDPBaseURL:                opts.ServerApiBaseURL,
			TenantName:                opts.TenantName,
			ReceiverAccountPublicKey:  opts.ReceiverAccountPublicKey,
			ReceiverAccountPrivateKey: opts.ReceiverAccountPrivateKey,
			Sep10SigningPublicKey:     opts.Sep10SigningPublicKey,
			DisbursedAssetCode:        opts.DisbursedAssetCode,
			NetworkPassphrase:         opts.NetworkPassphrase,
			SingleTenantMode:          opts.SingleTenantMode,
		}
	}

	// initialize anchor platform integration tests service
	it.anchorPlatform = &AnchorPlatformIntegrationTests{
		HTTPClient:                httpclient.DefaultClient(),
		TenantName:                opts.TenantName,
		AnchorPlatformBaseSepURL:  opts.AnchorPlatformBaseSepURL,
		ReceiverAccountPublicKey:  opts.ReceiverAccountPublicKey,
		ReceiverAccountPrivateKey: opts.ReceiverAccountPrivateKey,
		Sep10SigningPublicKey:     opts.Sep10SigningPublicKey,
		DisbursedAssetCode:        opts.DisbursedAssetCode,
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

	if opts.EnableAnchorPlatform {
		return it.registerWithAnchorPlatform(ctx, opts, disbursement)
	} else {
		return it.registerWithInternalSEP(ctx, opts, disbursement)
	}
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

// registerWithAnchorPlatform is a function that registers the receiver wallet through the SEP-24 flow if needed,
// i.e. if the registration contact type does not include the wallet address, used with Anchor Platform
func (it *IntegrationTestsService) registerWithAnchorPlatform(ctx context.Context, opts IntegrationTestsOpts, disbursement *data.Disbursement) error {
	log.Ctx(ctx).Info("Starting challenge transaction on anchor platform")
	challengeTx, err := it.anchorPlatform.StartChallengeTransaction()
	if err != nil {
		return fmt.Errorf("creating SEP10 challenge transaction: %w", err)
	}

	log.Ctx(ctx).Info("Signing challenge transaction with Sep10SigningKey")
	signedTx, err := it.anchorPlatform.SignChallengeTransaction(challengeTx)
	if err != nil {
		return fmt.Errorf("signing SEP10 challenge transaction: %w", err)
	}

	log.Ctx(ctx).Info("Sending challenge transaction to anchor platform")
	authSEP10Token, err := it.anchorPlatform.SendSignedChallengeTransaction(signedTx)
	if err != nil {
		return fmt.Errorf("sending SEP10 challenge transaction: %w", err)
	}

	log.Ctx(ctx).Info("Creating SEP24 deposit transaction on anchor platform")
	authSEP24Token, _, err := it.anchorPlatform.CreateSep24DepositTransaction(authSEP10Token)
	if err != nil {
		return fmt.Errorf("creating SEP24 deposit transaction: %w", err)
	}

	disbursementInstructions, err := readDisbursementCSV(opts.DisbursementCSVFilePath, opts.DisbursementCSVFileName)
	if err != nil {
		return fmt.Errorf("reading disbursement CSV: %w", err)
	}

	log.Ctx(ctx).Info("Completing receiver registration using server API")
	if err = it.serverAPI.ReceiverRegistration(ctx, authSEP24Token, &data.ReceiverRegistrationRequest{
		OTP:               data.TestnetAlwaysValidOTP,
		PhoneNumber:       disbursementInstructions[0].Phone,
		Email:             disbursementInstructions[0].Email,
		VerificationValue: disbursementInstructions[0].VerificationValue,
		VerificationField: disbursement.VerificationField,
		ReCAPTCHAToken:    opts.RecaptchaSiteKey,
	}); err != nil {
		return fmt.Errorf("registering receiver: %w", err)
	}

	log.Ctx(ctx).Info("Validating receiver data after completing registration")
	if err = validateExpectationsAfterReceiverRegistration(ctx, it.models, opts.ReceiverAccountPublicKey, opts.ReceiverAccountStellarMemo, opts.WalletSEP10Domain); err != nil {
		return fmt.Errorf("validating receiver after registration: %w", err)
	}

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
	receivers, err := it.models.DisbursementReceivers.GetAll(ctx, it.mtnDbConnectionPool, &data.QueryParams{}, disbursementID)
	if err != nil {
		return fmt.Errorf("getting receivers from disbursement: %w", err)
	}

	// Create receiver wallet for each receiver
	for _, receiver := range receivers {
		// Check if receiver wallet already exists
		existingWallets, err := it.models.ReceiverWallet.GetWithReceiverIDs(ctx, it.mtnDbConnectionPool, data.ReceiverIDs{receiver.ID})
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
			_, err = it.models.ReceiverWallet.GetOrInsertReceiverWallet(ctx, it.mtnDbConnectionPool, data.ReceiverWalletInsert{
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

	// Use appropriate base URL based on whether we're using Anchor Platform or Internal SEP
	// For Internal SEP, use 3-part domain with tenant name for proper tenant extraction in SEP-24
	// For Anchor Platform, use the standard localhost URL
	var baseURL string
	if opts.EnableAnchorPlatform {
		baseURL = "http://localhost:8000"
	} else {
		baseURL = fmt.Sprintf("http://%s.stellar.local:8000", opts.TenantName)
	}

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
