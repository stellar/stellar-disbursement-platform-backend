package services

import (
	"context"
	"reflect"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNewDirectPaymentService(t *testing.T) {
	type args struct {
		models           *data.Models
		dbConnectionPool db.DBConnectionPool
	}
	tests := []struct {
		name string
		args args
		want *DirectPaymentService
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewDirectPaymentService(tt.args.models, tt.args.dbConnectionPool); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewDirectPaymentService() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDirectPaymentService_CreateDirectPayment(t *testing.T) {
	type fields struct {
		Models                     *data.Models
		DBConnectionPool           db.DBConnectionPool
		EventProducer              events.Producer
		DistributionAccountService DistributionAccountServiceInterface
		Resolvers                  *ResolverFactory
	}
	type args struct {
		ctx                 context.Context
		req                 CreateDirectPaymentRequest
		user                *auth.User
		distributionAccount *schema.TransactionAccount
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *data.Payment
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &DirectPaymentService{
				Models:                     tt.fields.Models,
				DBConnectionPool:           tt.fields.DBConnectionPool,
				EventProducer:              tt.fields.EventProducer,
				DistributionAccountService: tt.fields.DistributionAccountService,
				Resolvers:                  tt.fields.Resolvers,
			}
			got, err := s.CreateDirectPayment(tt.args.ctx, tt.args.req, tt.args.user, tt.args.distributionAccount)
			if (err != nil) != tt.wantErr {
				t.Errorf("DirectPaymentService.CreateDirectPayment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DirectPaymentService.CreateDirectPayment() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDirectPaymentService_validateAssetWalletCompatibility(t *testing.T) {
	type fields struct {
		Models                     *data.Models
		DBConnectionPool           db.DBConnectionPool
		EventProducer              events.Producer
		DistributionAccountService DistributionAccountServiceInterface
		Resolvers                  *ResolverFactory
	}
	type args struct {
		ctx    context.Context
		asset  *data.Asset
		wallet *data.Wallet
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &DirectPaymentService{
				Models:                     tt.fields.Models,
				DBConnectionPool:           tt.fields.DBConnectionPool,
				EventProducer:              tt.fields.EventProducer,
				DistributionAccountService: tt.fields.DistributionAccountService,
				Resolvers:                  tt.fields.Resolvers,
			}
			if err := s.validateAssetWalletCompatibility(tt.args.ctx, tt.args.asset, tt.args.wallet); (err != nil) != tt.wantErr {
				t.Errorf("DirectPaymentService.validateAssetWalletCompatibility() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDirectPaymentService_getOrCreateReceiverWallet(t *testing.T) {
	type fields struct {
		Models                     *data.Models
		DBConnectionPool           db.DBConnectionPool
		EventProducer              events.Producer
		DistributionAccountService DistributionAccountServiceInterface
		Resolvers                  *ResolverFactory
	}
	type args struct {
		ctx           context.Context
		dbTx          db.DBTransaction
		receiverID    string
		walletID      string
		walletAddress *string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *data.ReceiverWallet
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &DirectPaymentService{
				Models:                     tt.fields.Models,
				DBConnectionPool:           tt.fields.DBConnectionPool,
				EventProducer:              tt.fields.EventProducer,
				DistributionAccountService: tt.fields.DistributionAccountService,
				Resolvers:                  tt.fields.Resolvers,
			}
			got, err := s.getOrCreateReceiverWallet(tt.args.ctx, tt.args.dbTx, tt.args.receiverID, tt.args.walletID, tt.args.walletAddress)
			if (err != nil) != tt.wantErr {
				t.Errorf("DirectPaymentService.getOrCreateReceiverWallet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DirectPaymentService.getOrCreateReceiverWallet() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDirectPaymentService_validateBalance(t *testing.T) {
	type fields struct {
		Models                     *data.Models
		DBConnectionPool           db.DBConnectionPool
		EventProducer              events.Producer
		DistributionAccountService DistributionAccountServiceInterface
		Resolvers                  *ResolverFactory
	}
	type args struct {
		ctx                 context.Context
		dbTx                db.DBTransaction
		distributionAccount *schema.TransactionAccount
		asset               *data.Asset
		amount              string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &DirectPaymentService{
				Models:                     tt.fields.Models,
				DBConnectionPool:           tt.fields.DBConnectionPool,
				EventProducer:              tt.fields.EventProducer,
				DistributionAccountService: tt.fields.DistributionAccountService,
				Resolvers:                  tt.fields.Resolvers,
			}
			if err := s.validateBalance(tt.args.ctx, tt.args.dbTx, tt.args.distributionAccount, tt.args.asset, tt.args.amount); (err != nil) != tt.wantErr {
				t.Errorf("DirectPaymentService.validateBalance() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDirectPaymentService_calculatePendingAmountForAsset(t *testing.T) {
	type fields struct {
		Models                     *data.Models
		DBConnectionPool           db.DBConnectionPool
		EventProducer              events.Producer
		DistributionAccountService DistributionAccountServiceInterface
		Resolvers                  *ResolverFactory
	}
	type args struct {
		ctx         context.Context
		dbTx        db.DBTransaction
		targetAsset data.Asset
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    float64
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &DirectPaymentService{
				Models:                     tt.fields.Models,
				DBConnectionPool:           tt.fields.DBConnectionPool,
				EventProducer:              tt.fields.EventProducer,
				DistributionAccountService: tt.fields.DistributionAccountService,
				Resolvers:                  tt.fields.Resolvers,
			}
			got, err := s.calculatePendingAmountForAsset(tt.args.ctx, tt.args.dbTx, tt.args.targetAsset)
			if (err != nil) != tt.wantErr {
				t.Errorf("DirectPaymentService.calculatePendingAmountForAsset() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("DirectPaymentService.calculatePendingAmountForAsset() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInsufficientBalanceForDirectPaymentError_Error(t *testing.T) {
	type fields struct {
		Asset              data.Asset
		RequestedAmount    float64
		AvailableBalance   float64
		TotalPendingAmount float64
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := InsufficientBalanceForDirectPaymentError{
				Asset:              tt.fields.Asset,
				RequestedAmount:    tt.fields.RequestedAmount,
				AvailableBalance:   tt.fields.AvailableBalance,
				TotalPendingAmount: tt.fields.TotalPendingAmount,
			}
			if got := e.Error(); got != tt.want {
				t.Errorf("InsufficientBalanceForDirectPaymentError.Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWalletNotEnabledError_Error(t *testing.T) {
	type fields struct {
		WalletName string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := WalletNotEnabledError{
				WalletName: tt.fields.WalletName,
			}
			if got := e.Error(); got != tt.want {
				t.Errorf("WalletNotEnabledError.Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAssetNotSupportedByWalletError_Error(t *testing.T) {
	type fields struct {
		AssetCode  string
		WalletName string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := AssetNotSupportedByWalletError{
				AssetCode:  tt.fields.AssetCode,
				WalletName: tt.fields.WalletName,
			}
			if got := e.Error(); got != tt.want {
				t.Errorf("AssetNotSupportedByWalletError.Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDirectPaymentService_CreateDirectPayment_Success(t *testing.T) {
	dbConnectionPool := getDBConnectionPool(t)

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	// Create test data
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "XLM", "")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Test Wallet", "https://testdomain.com", "testdomain.com", "test://")

	// Associate asset with wallet
	_, err = dbConnectionPool.ExecContext(ctx,
		"INSERT INTO wallets_assets (wallet_id, asset_id) VALUES ($1, $2)",
		wallet.ID, asset.ID)
	require.NoError(t, err)

	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
		Email: "horus@warmaster.terra",
	})

	user := &auth.User{ID: "user-vespasian", Email: "vespasian@emperor.terra"}
	distributionAccount := &schema.TransactionAccount{
		Type: schema.DistributionAccountStellarDBVault,
	}

	service := NewDirectPaymentService(models, dbConnectionPool)

	// Mock the distribution account service for balance validation
	mockDistService := &mocks.MockDistributionAccountService{}
	mockDistService.On("GetBalance", mock.Anything, distributionAccount, *asset).Return(float64(1000), nil)
	service.DistributionAccountService = mockDistService

	// Test case 1: Direct payment with asset by ID and receiver by ID
	req := CreateDirectPaymentRequest{
		Amount: "50.00",
		Asset: AssetReference{
			ID: &asset.ID,
		},
		Receiver: ReceiverReference{
			ID: &receiver.ID,
		},
		Wallet: WalletReference{
			ID: &wallet.ID,
		},
		ExternalPaymentID: stringPtr("PAY_HORUS_001"),
	}

	payment, err := service.CreateDirectPayment(ctx, req, user, distributionAccount)
	require.NoError(t, err)
	assert.NotNil(t, payment)
	assert.Equal(t, "50.00", payment.Amount)
	assert.Equal(t, data.PaymentTypeDirect, payment.PaymentType)
	assert.Equal(t, data.DraftPaymentStatus, payment.Status)
	assert.Equal(t, "PAY_HORUS_001", payment.ExternalPaymentID)
	assert.Nil(t, payment.Disbursement)
	assert.Equal(t, asset.ID, payment.Asset.ID)

	mockDistService.AssertExpectations(t)
}

func getDBConnectionPool(t *testing.T) db.DBConnectionPool {
	t.Helper()
	dbt := dbtest.Open(t)
	t.Cleanup(func() { dbt.Close() })

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	t.Cleanup(func() { dbConnectionPool.Close() })
	return dbConnectionPool
}

func stringPtr(s string) *string {
	return &s
}
