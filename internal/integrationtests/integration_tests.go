package integrationtests

import (
	"context"
	"database/sql"
	"errors"
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
	ServerApiBaseURL           string
	AdminServerBaseURL         string
	AdminServerAccountId       string
	AdminServerApiKey          string
	CircleUSDCWalletID         string
	CircleAPIKey               string
	HorizonURL                 string
	NetworkPassphrase          string
	EnableEmbeddedWallets      bool
	EmbeddedWalletCredentialID string
	EmbeddedWalletPublicKey    string
}

type IntegrationTestsService struct {
	models                *data.Models
	adminDbConnectionPool db.DBConnectionPool
	mtnDbConnectionPool   db.DBConnectionPool
	tssDbConnectionPool   db.DBConnectionPool
	tenantManager         *tenant.Manager
	serverAPI             ServerApiIntegrationTestsInterface
	adminAPI              AdminApiIntegrationTestsInterface
	anchorPlatform        AnchorPlatformIntegrationTestsInterface
	horizonClient         horizonclient.ClientInterface
}

// NewIntegrationTestsService is a function that create a new IntegrationTestsService instance.
func NewIntegrationTestsService(opts IntegrationTestsOpts) (*IntegrationTestsService, error) {
	adminDSN, err := router.GetDSNForAdmin(opts.DatabaseDSN)
	if err != nil {
		return nil, fmt.Errorf("getting admin database DSN: %w", err)
	}
	adminDbConnectionPool, err := db.OpenDBConnectionPool(adminDSN)
	if err != nil {
		return nil, fmt.Errorf("connecting to the database: %w", err)
	}
	tm := tenant.NewManager(tenant.WithDatabase(adminDbConnectionPool))
	tr := tenant.NewMultiTenantDataSourceRouter(tm)
	mtnDbConnectionPool, err := db.NewConnectionPoolWithRouter(tr)
	if err != nil {
		return nil, fmt.Errorf("getting multi-tenant database connection pool: %w", err)
	}
	models, err := data.NewModels(mtnDbConnectionPool)
	if err != nil {
		return nil, fmt.Errorf("creating models for integration tests: %w", err)
	}
	tssDSN, err := router.GetDSNForTSS(opts.DatabaseDSN)
	if err != nil {
		return nil, fmt.Errorf("getting TSS database DSN: %w", err)
	}
	tssDbConnectionPool, err := db.OpenDBConnectionPool(tssDSN)
	if err != nil {
		return nil, fmt.Errorf("connecting to the tss database: %w", err)
	}
	it := &IntegrationTestsService{
		models:                models,
		adminDbConnectionPool: adminDbConnectionPool,
		mtnDbConnectionPool:   mtnDbConnectionPool,
		tssDbConnectionPool:   tssDbConnectionPool,
		tenantManager:         tm,
	}

	// initialize admin api integration tests service
	it.adminAPI = &AdminApiIntegrationTests{
		HttpClient:      httpclient.DefaultClient(),
		AdminApiBaseURL: opts.AdminServerBaseURL,
		AccountId:       opts.AdminServerAccountId,
		ApiKey:          opts.AdminServerApiKey,
	}

	return it, nil
}

func (it *IntegrationTestsService) initServices(_ context.Context, opts IntegrationTestsOpts) {
	// initialize default testnet horizon client
	it.horizonClient = &horizonclient.Client{
		HorizonURL: opts.HorizonURL,
		HTTP:       httpclient.DefaultClient(),
	}

	// initialize anchor platform integration tests service
	it.anchorPlatform = &AnchorPlatformIntegrationTests{
		HttpClient:                httpclient.DefaultClient(),
		TenantName:                opts.TenantName,
		AnchorPlatformBaseSepURL:  opts.AnchorPlatformBaseSepURL,
		ReceiverAccountPublicKey:  opts.ReceiverAccountPublicKey,
		ReceiverAccountPrivateKey: opts.ReceiverAccountPrivateKey,
		Sep10SigningPublicKey:     opts.Sep10SigningPublicKey,
		DisbursedAssetCode:        opts.DisbursedAssetCode,
	}

	// initialize server api integration tests service
	it.serverAPI = &ServerApiIntegrationTests{
		HttpClient:              httpclient.DefaultClient(),
		TenantName:              opts.TenantName,
		ServerApiBaseURL:        opts.ServerApiBaseURL,
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

	if err = it.createEmbeddedWalletIfNeeded(ctx, opts, authToken); err != nil {
		return fmt.Errorf("creating embedded wallet if needed: %w", err)
	}

	if err = it.ensureTransactionCompletion(ctx, opts, disbursement); err != nil {
		return err
	}

	log.Ctx(ctx).Info("🎉🎉🎉 Successfully finished integration tests! The disbursement was delivered to the recipient! 🎉🎉🎉")

	return nil
}

// createAndValidateDisbursement is a function that creates a disbursement and validates it.
func (it *IntegrationTestsService) createAndValidateDisbursement(ctx context.Context, opts IntegrationTestsOpts, authToken *ServerApiAuthToken, asset *data.Asset) (*data.Disbursement, error) {
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
	if err = validateExpectationsAfterProcessDisbursement(ctx, disbursement.ID, it.models, it.mtnDbConnectionPool); err != nil {
		return nil, fmt.Errorf("validating data after process disbursement: %w", err)
	}

	log.Ctx(ctx).Info("Starting disbursement using server API...")
	if err = it.serverAPI.StartDisbursement(ctx, authToken, disbursement.ID, &httphandler.PatchDisbursementStatusRequest{Status: "STARTED"}); err != nil {
		return nil, fmt.Errorf("starting disbursement: %w", err)
	}

	log.Ctx(ctx).Info("Validating disbursement data after starting disbursement using server API...")
	if err = validateExpectationsAfterStartDisbursement(ctx, disbursement.ID, it.models, it.mtnDbConnectionPool); err != nil {
		return nil, fmt.Errorf("validating data after process disbursement: %w", err)
	}
	return disbursement, nil
}

// registerReceiverWalletIfNeeded is a function that registers the receiver wallet through the SEP-24 flow if needed,
// i.e. if the registration contact type does not include the wallet address.
func (it *IntegrationTestsService) registerReceiverWalletIfNeeded(ctx context.Context, opts IntegrationTestsOpts, disbursement *data.Disbursement) error {
	if disbursement.RegistrationContactType.IncludesWalletAddress {
		log.Ctx(ctx).Infof("⏭ Skipping SEP-24 flow because registrationContactType=%q", disbursement.RegistrationContactType)
		return nil
	}

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
	err = it.serverAPI.ReceiverRegistration(ctx, authSEP24Token, &data.ReceiverRegistrationRequest{
		OTP:               data.TestnetAlwaysValidOTP,
		PhoneNumber:       disbursementInstructions[0].Phone,
		Email:             disbursementInstructions[0].Email,
		VerificationValue: disbursementInstructions[0].VerificationValue,
		VerificationField: disbursement.VerificationField,
		ReCAPTCHAToken:    opts.RecaptchaSiteKey,
	})
	if err != nil {
		return fmt.Errorf("registring receiver: %w", err)
	}

	log.Ctx(ctx).Info("Validating receiver data after completing registration")
	err = validateExpectationsAfterReceiverRegistration(ctx, it.models, opts.ReceiverAccountPublicKey, opts.ReceiverAccountStellarMemo, opts.WalletSEP10Domain)
	if err != nil {
		return fmt.Errorf("validating receiver after registration: %w", err)
	}

	return nil
}

func (it *IntegrationTestsService) createEmbeddedWalletIfNeeded(ctx context.Context, opts IntegrationTestsOpts, authToken *ServerApiAuthToken) error {
	if !opts.EnableEmbeddedWallets {
		log.Ctx(ctx).Info("⏭ Skipping embedded wallet creation flow")
		return nil
	}

	if opts.EmbeddedWalletCredentialID == "" || opts.EmbeddedWalletPublicKey == "" {
		return fmt.Errorf("embedded wallet credential id and public key must be provided when embedded wallets are enabled")
	}

	log.Ctx(ctx).Info("Checking for existing embedded wallet with provided credential ID")
	existingWallet, err := it.models.EmbeddedWallets.GetByCredentialID(ctx, it.mtnDbConnectionPool, opts.EmbeddedWalletCredentialID)
	if err == nil {
		if existingWallet.WalletStatus == data.SuccessWalletStatus && existingWallet.ContractAddress != "" {
			log.Ctx(ctx).Infof("Embedded wallet already exists with credential ID %s and contract address %s", opts.EmbeddedWalletCredentialID, existingWallet.ContractAddress)
			return nil
		}
	} else if !errors.Is(err, data.ErrRecordNotFound) {
		return fmt.Errorf("checking existing embedded wallet: %w", err)
	}

	log.Ctx(ctx).Info("Waiting for embedded wallet invitation token to be generated")
	invitation, err := it.waitForEmbeddedWalletInvitation(ctx)
	if err != nil {
		return fmt.Errorf("waiting for embedded wallet invitation: %w", err)
	}
	log.Ctx(ctx).Infof("Embedded wallet invitation ready with token %s", invitation.Token)

	req := &CreateEmbeddedWalletRequest{
		Token:        invitation.Token,
		PublicKey:    opts.EmbeddedWalletPublicKey,
		CredentialID: opts.EmbeddedWalletCredentialID,
	}

	log.Ctx(ctx).Info("Triggering embedded wallet creation through server API")
	if err = it.serverAPI.CreateEmbeddedWallet(ctx, authToken, req); err != nil {
		return fmt.Errorf("creating embedded wallet: %w", err)
	}

	log.Ctx(ctx).Info("Waiting for embedded wallet contract deployment to complete")
	wallet, err := it.waitForEmbeddedWalletReady(ctx, opts.EmbeddedWalletCredentialID)
	if err != nil {
		return fmt.Errorf("waiting for embedded wallet readiness: %w", err)
	}

	if wallet.ContractAddress == "" {
		return fmt.Errorf("embedded wallet contract address is empty for credential id %s", opts.EmbeddedWalletCredentialID)
	}
	log.Ctx(ctx).Infof("Embedded wallet ready with contract address %s", wallet.ContractAddress)

	return nil
}

func (it *IntegrationTestsService) enableEmbeddedWalletForWallet(ctx context.Context, walletID string) error {
	const query = `
		UPDATE wallets
		SET embedded = TRUE,
		    deep_link_schema = 'SELF',
		    updated_at = NOW()
		WHERE id = $1
	`

	if _, err := it.mtnDbConnectionPool.ExecContext(ctx, query, walletID); err != nil {
		return fmt.Errorf("updating wallet %s to embedded: %w", walletID, err)
	}

	return nil
}

func (it *IntegrationTestsService) waitForEmbeddedWalletInvitation(ctx context.Context) (*data.EmbeddedWallet, error) {
	var wallet data.EmbeddedWallet
	query := fmt.Sprintf(`
		SELECT %s
		FROM embedded_wallets
		WHERE wallet_status = $1 AND credential_id IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`, data.EmbeddedWalletColumnNames("", ""))

	err := retry.Do(
		func() error {
			err := it.mtnDbConnectionPool.GetContext(ctx, &wallet, query, data.PendingWalletStatus)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return fmt.Errorf("embedded wallet invitation not available yet")
				}
				return err
			}
			return nil
		},
		retry.Context(ctx),
		retry.Attempts(18),
		retry.Delay(10*time.Second),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			log.Ctx(ctx).Infof("Waiting for embedded wallet invitation (attempt #%d): %v", n+1, err)
		}),
	)
	if err != nil {
		return nil, err
	}

	return &wallet, nil
}

func (it *IntegrationTestsService) waitForEmbeddedWalletReady(ctx context.Context, credentialID string) (*data.EmbeddedWallet, error) {
	var wallet data.EmbeddedWallet
	query := fmt.Sprintf(`
		SELECT %s
		FROM embedded_wallets
		WHERE credential_id = $1
	`, data.EmbeddedWalletColumnNames("", ""))

	err := retry.Do(
		func() error {
			err := it.mtnDbConnectionPool.GetContext(ctx, &wallet, query, credentialID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return fmt.Errorf("embedded wallet with credential id %s not found yet", credentialID)
				}
				return err
			}

			if wallet.WalletStatus == data.FailedWalletStatus {
				return retry.Unrecoverable(fmt.Errorf("embedded wallet creation failed"))
			}

			if wallet.WalletStatus != data.SuccessWalletStatus || wallet.ContractAddress == "" {
				return fmt.Errorf("embedded wallet not ready yet (status=%s)", wallet.WalletStatus)
			}

			return nil
		},
		retry.Context(ctx),
		retry.Attempts(30),
		retry.Delay(10*time.Second),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			log.Ctx(ctx).Infof("Waiting for embedded wallet readiness (attempt #%d): %v", n+1, err)
		}),
	)
	if err != nil {
		return nil, err
	}

	return &wallet, nil
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
			receivers, innerErr = it.models.DisbursementReceivers.GetAll(ctx, it.mtnDbConnectionPool, &data.QueryParams{}, disbursement.ID)
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

func (it *IntegrationTestsService) CreateTestData(ctx context.Context, opts IntegrationTestsOpts) error {
	// 1. Create new tenant and add owner user
	distributionAccType := schema.AccountType(opts.DistributionAccountType)
	t, err := it.adminAPI.CreateTenant(ctx, CreateTenantRequest{
		Name:                    opts.TenantName,
		OwnerEmail:              opts.UserEmail,
		OwnerFirstName:          "John",
		OwnerLastName:           "Doe",
		OrganizationName:        "Integration Tests Organization",
		DistributionAccountType: distributionAccType,
		BaseURL:                 "http://localhost:8000",
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
	_, err = it.mtnDbConnectionPool.ExecContext(ctx, query, hashedPassword, opts.UserEmail)
	if err != nil {
		return fmt.Errorf("updating owner user password: %w", err)
	}

	// 3. Create test asset and wallet
	_, err = it.models.Assets.GetOrCreate(ctx, opts.DisbursedAssetCode, opts.DisbursetAssetIssuer)
	if err != nil {
		return fmt.Errorf("getting or creating test asset: %w", err)
	}

	wallet, err := it.models.Wallets.GetOrCreate(ctx, opts.WalletName, opts.WalletHomepage, opts.WalletDeepLink, opts.WalletSEP10Domain)
	if err != nil {
		return fmt.Errorf("getting or creating test wallet: %w", err)
	}

	if opts.EnableEmbeddedWallets {
		log.Ctx(ctx).Info("Enabling embedded wallet configuration for test wallet")
		if err = it.enableEmbeddedWalletForWallet(ctx, wallet.ID); err != nil {
			return fmt.Errorf("configuring embedded wallet: %w", err)
		}
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
