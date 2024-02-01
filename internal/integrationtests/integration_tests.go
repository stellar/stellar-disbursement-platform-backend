package integrationtests

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httphandler"
	tss "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

const paymentProcessTimeMinutes = 3

type IntegrationTestsInterface interface {
	StartIntegrationTests(ctx context.Context, opts IntegrationTestsOpts) error
	CreateTestData(ctx context.Context, opts IntegrationTestsOpts) error
}

type IntegrationTestsOpts struct {
	DatabaseDSN                string
	UserEmail                  string
	UserPassword               string
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
}

type IntegrationTestsService struct {
	models           *data.Models
	dbConnectionPool db.DBConnectionPool
	serverAPI        ServerApiIntegrationTestsInterface
	anchorPlatform   AnchorPlatformIntegrationTestsInterface
	horizonClient    horizonclient.ClientInterface
}

// NewIntegrationTestsService is a function that create a new IntegrationTestsService instance.
func NewIntegrationTestsService(opts IntegrationTestsOpts) (*IntegrationTestsService, error) {
	// initialize dbConnection and data.Models
	dbConnectionPool, err := db.OpenDBConnectionPool(opts.DatabaseDSN)
	if err != nil {
		return nil, fmt.Errorf("error connecting to the database: %w", err)
	}

	models, err := data.NewModels(dbConnectionPool)
	if err != nil {
		return nil, fmt.Errorf("error creating models for integration tests: %w", err)
	}

	return &IntegrationTestsService{
		models:           models,
		dbConnectionPool: dbConnectionPool,
	}, nil
}

func (it *IntegrationTestsService) initServices(ctx context.Context, opts IntegrationTestsOpts) {
	// initialize default testnet horizon client
	it.horizonClient = horizonclient.DefaultTestNetClient

	// initialize anchor platform integration tests service
	it.anchorPlatform = &AnchorPlatformIntegrationTests{
		HttpClient:                httpclient.DefaultClient(),
		AnchorPlatformBaseSepURL:  opts.AnchorPlatformBaseSepURL,
		ReceiverAccountPublicKey:  opts.ReceiverAccountPublicKey,
		ReceiverAccountPrivateKey: opts.ReceiverAccountPrivateKey,
		Sep10SigningPublicKey:     opts.Sep10SigningPublicKey,
		DisbursedAssetCode:        opts.DisbursedAssetCode,
	}

	// initialize server api integration tests service
	it.serverAPI = &ServerApiIntegrationTests{
		HttpClient:              httpclient.DefaultClient(),
		ServerApiBaseURL:        opts.ServerApiBaseURL,
		UserEmail:               opts.UserEmail,
		UserPassword:            opts.UserPassword,
		DisbursementCSVFilePath: opts.DisbursementCSVFilePath,
		DisbursementCSVFileName: opts.DisbursementCSVFileName,
	}
}

func (it *IntegrationTestsService) StartIntegrationTests(ctx context.Context, opts IntegrationTestsOpts) error {
	log.Ctx(ctx).Info("Starting integration tests ......")

	it.initServices(ctx, opts)
	log.Ctx(ctx).Info("Login user to get server API auth token")
	authToken, err := it.serverAPI.Login(ctx)
	if err != nil {
		return fmt.Errorf("error trying to login in server API: %w", err)
	}
	log.Ctx(ctx).Info("User logged in")
	log.Ctx(ctx).Info(authToken)

	log.Ctx(ctx).Info("Getting test asset in database")
	asset, err := it.models.Assets.GetByCodeAndIssuer(ctx, opts.DisbursedAssetCode, opts.DisbursetAssetIssuer)
	if err != nil {
		return fmt.Errorf("error getting test asset: %w", err)
	}

	log.Ctx(ctx).Info("Getting test wallet in database")
	wallet, err := it.models.Wallets.GetByWalletName(ctx, opts.WalletName)
	if err != nil {
		return fmt.Errorf("error getting test wallet: %w", err)
	}

	log.Ctx(ctx).Info("Creating disbursement using server API")
	disbursement, err := it.serverAPI.CreateDisbursement(ctx, authToken, &httphandler.PostDisbursementRequest{
		Name:              opts.DisbursementName,
		CountryCode:       "USA",
		WalletID:          wallet.ID,
		AssetID:           asset.ID,
		VerificationField: data.VerificationFieldDateOfBirth,
	})
	if err != nil {
		return fmt.Errorf("error creating disbursement: %w", err)
	}
	log.Ctx(ctx).Info("Disbursement created")

	log.Ctx(ctx).Info("Processing disbursement CSV file using server API")
	err = it.serverAPI.ProcessDisbursement(ctx, authToken, disbursement.ID)
	if err != nil {
		return fmt.Errorf("error processing disbursement: %w", err)
	}
	log.Ctx(ctx).Info("CSV disbursement file processed")

	log.Ctx(ctx).Info("Validating disbursement data after processing the disbursement file")
	err = validateExpectationsAfterProcessDisbursement(ctx, disbursement.ID, it.models, it.dbConnectionPool)
	if err != nil {
		return fmt.Errorf("error validating data after process disbursement: %w", err)
	}
	log.Ctx(ctx).Info("Disbursement data validated")

	log.Ctx(ctx).Info("Starting disbursement using server API")
	err = it.serverAPI.StartDisbursement(ctx, authToken, disbursement.ID, &httphandler.PatchDisbursementStatusRequest{Status: "STARTED"})
	if err != nil {
		return fmt.Errorf("error starting disbursement: %w", err)
	}
	log.Ctx(ctx).Info("Disbursement started")

	log.Ctx(ctx).Info("Validating disbursement data after starting disbursement using server API")
	err = validateExpectationsAfterStartDisbursement(ctx, disbursement.ID, it.models, it.dbConnectionPool)
	if err != nil {
		return fmt.Errorf("error validating data after process disbursement: %w", err)
	}
	log.Ctx(ctx).Info("Disbursement data validated")

	log.Ctx(ctx).Info("Starting anchor platform integration ......")
	log.Ctx(ctx).Info("Starting challenge transaction on anchor platform")
	challengeTx, err := it.anchorPlatform.StartChallengeTransaction()
	if err != nil {
		return fmt.Errorf("error creating SEP10 challenge transaction: %w", err)
	}
	log.Ctx(ctx).Info("Challenge transaction created")

	log.Ctx(ctx).Info("Signing challenge transaction with Sep10SigningKey")
	signedTx, err := it.anchorPlatform.SignChallengeTransaction(challengeTx)
	if err != nil {
		return fmt.Errorf("error signing SEP10 challenge transaction: %w", err)
	}
	log.Ctx(ctx).Info("Challenge transaction signed")

	log.Ctx(ctx).Info("Sending challenge transaction to anchor platform")
	authSEP10Token, err := it.anchorPlatform.SendSignedChallengeTransaction(signedTx)
	if err != nil {
		return fmt.Errorf("error sending SEP10 challenge transaction: %w", err)
	}
	log.Ctx(ctx).Info("Received authSEP10Token")

	log.Ctx(ctx).Info("Creating SEP24 deposit transaction on anchor platform")
	authSEP24Token, _, err := it.anchorPlatform.CreateSep24DepositTransaction(authSEP10Token)
	if err != nil {
		return fmt.Errorf("error creating SEP24 deposit transaction: %w", err)
	}
	log.Ctx(ctx).Info("Received authSEP24Token")

	disbursementData, err := readDisbursementCSV(opts.DisbursementCSVFilePath, opts.DisbursementCSVFileName)
	if err != nil {
		return fmt.Errorf("error reading disbursement CSV: %w", err)
	}

	log.Ctx(ctx).Info("Completing receiver registration using server API")
	err = it.serverAPI.ReceiverRegistration(ctx, authSEP24Token, &data.ReceiverRegistrationRequest{
		OTP:               data.TestnetAlwaysValidOTP,
		PhoneNumber:       disbursementData[0].Phone,
		VerificationValue: disbursementData[0].VerificationValue,
		VerificationType:  disbursement.VerificationField,
		ReCAPTCHAToken:    opts.RecaptchaSiteKey,
	})
	if err != nil {
		return fmt.Errorf("error registring receiver: %w", err)
	}
	log.Ctx(ctx).Info("Receiver OTP obtained")

	log.Ctx(ctx).Info("Validating receiver data after completing registration")
	err = validateExpectationsAfterReceiverRegistration(ctx, it.models, opts.ReceiverAccountPublicKey, opts.ReceiverAccountStellarMemo, opts.WalletSEP10Domain)
	if err != nil {
		return fmt.Errorf("error validating receiver after registration: %w", err)
	}
	log.Ctx(ctx).Info("Receiver data validated")

	log.Ctx(ctx).Info("Waiting for payment to be processed by TSS")
	time.Sleep(paymentProcessTimeMinutes * time.Minute)

	log.Ctx(ctx).Info("Querying database to get disbursement receiver with payment data")
	receivers, err := it.models.DisbursementReceivers.GetAll(ctx, it.dbConnectionPool, &data.QueryParams{}, disbursement.ID)
	if err != nil {
		return fmt.Errorf("error getting receivers: %w", err)
	}

	payment := receivers[0].Payment
	q := `SELECT * FROM submitter_transactions WHERE external_id = $1`
	var tx tss.Transaction
	err = it.dbConnectionPool.GetContext(ctx, &tx, q, payment.ID)
	if err != nil {
		return fmt.Errorf("getting TSS transaction from database: %w", err)
	}
	log.Ctx(ctx).Infof("TSS transaction: %+v", tx)

	log.Ctx(ctx).Info("Getting payment from disbursement receiver")
	if payment.Status != data.SuccessPaymentStatus || payment.StellarTransactionID == "" {
		return fmt.Errorf("payment was not processed successfully by TSS: %+v", payment)
	}

	log.Ctx(ctx).Info("Payment was successfully updated by the TSS")
	log.Ctx(ctx).Info("Validating transaction on Horizon Network")
	ph, err := getTransactionOnHorizon(it.horizonClient, payment.StellarTransactionID)
	if err != nil {
		return fmt.Errorf("error getting transaction on horizon network: %w", err)
	}
	err = validateStellarTransaction(ph, opts.ReceiverAccountPublicKey, opts.DisbursedAssetCode, opts.DisbursetAssetIssuer, receivers[0].Payment.Amount)
	if err != nil {
		return fmt.Errorf("error validating stellar transaction: %w", err)
	}
	log.Ctx(ctx).Info("Transaction validated")

	log.Ctx(ctx).Info("🎉🎉🎉Finishing integration tests, the receiver was successfully funded 🎉🎉🎉")

	return nil
}

func (it *IntegrationTestsService) CreateTestData(ctx context.Context, opts IntegrationTestsOpts) error {
	_, err := it.models.Assets.GetOrCreate(ctx, opts.DisbursedAssetCode, opts.DisbursetAssetIssuer)
	if err != nil {
		return fmt.Errorf("error getting or creating test asset: %w", err)
	}

	_, err = it.models.Wallets.GetOrCreate(ctx, opts.WalletName, opts.WalletHomepage, opts.WalletDeepLink, opts.WalletSEP10Domain)
	if err != nil {
		return fmt.Errorf("error getting or creating test wallet: %w", err)
	}

	return nil
}
