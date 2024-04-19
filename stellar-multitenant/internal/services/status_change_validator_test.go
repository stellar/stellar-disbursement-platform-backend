package services

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_ValidateStatus(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	tenantManagerMock := tenant.TenantManagerMock{}

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	tntID := "tenantID"

	testCases := []struct {
		name             string
		mockTntManagerFn func(tntManagerMock *tenant.TenantManagerMock)
		createFixtures   func()
		deleteFixtures   func()
		reqStatus        tenant.TenantStatus
		expectedErr      error
	}{
		{
			name: "cannot retrieve tenant by id",
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock) {
				tntManagerMock.On("GetTenant", ctx, &tenant.QueryParams{
					Filters: map[tenant.FilterKey]interface{}{
						tenant.FilterKeyID: tntID,
					},
				}).Return(nil, errors.New("foobar")).Once()
			},
			reqStatus:   tenant.DeactivatedTenantStatus,
			expectedErr: fmt.Errorf("%w: %w", ErrCannotRetrieveTenantByID, errors.New("foobar")),
		},
		{
			name: "number of active payments is not 0",
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock) {
				tntManagerMock.On("GetTenant", ctx, &tenant.QueryParams{
					Filters: map[tenant.FilterKey]interface{}{
						tenant.FilterKeyID: tntID,
					},
				}).Return(&tenant.Tenant{ID: tntID, Status: tenant.ActivatedTenantStatus}, nil).Once()
			},
			createFixtures: func() {
				country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
				wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
				asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
				disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
					Country: country,
					Wallet:  wallet,
					Status:  data.ReadyDisbursementStatus,
					Asset:   asset,
				})
				receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
				rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.DraftReceiversWalletStatus)
				_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
					Amount:         "50",
					Status:         data.ReadyPaymentStatus,
					Disbursement:   disbursement,
					Asset:          *asset,
					ReceiverWallet: rw,
				})
			},
			deleteFixtures: func() {
				data.DeleteAllFixtures(t, ctx, dbConnectionPool)
			},
			reqStatus:   tenant.DeactivatedTenantStatus,
			expectedErr: ErrCannotDeactivateTenant,
		},
		{
			name: "tenant is already deactivated",
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock) {
				tntManagerMock.On("GetTenant", ctx, &tenant.QueryParams{
					Filters: map[tenant.FilterKey]interface{}{
						tenant.FilterKeyID: tntID,
					},
				}).Return(&tenant.Tenant{ID: tntID, Status: tenant.DeactivatedTenantStatus}, nil).Once()
			},
			reqStatus:   tenant.DeactivatedTenantStatus,
			expectedErr: nil,
		},
		{
			name: "tenant is already activated",
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock) {
				tntManagerMock.On("GetTenant", ctx, &tenant.QueryParams{
					Filters: map[tenant.FilterKey]interface{}{
						tenant.FilterKeyID: tntID,
					},
				}).Return(&tenant.Tenant{ID: tntID, Status: tenant.ActivatedTenantStatus}, nil).Once()
			},
			reqStatus:   tenant.ActivatedTenantStatus,
			expectedErr: nil,
		},
		{
			name: "cannot activate tenant that isn't deactivated",
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock) {
				tntManagerMock.On("GetTenant", ctx, &tenant.QueryParams{
					Filters: map[tenant.FilterKey]interface{}{
						tenant.FilterKeyID: tntID,
					},
				}).Return(&tenant.Tenant{ID: tntID, Status: tenant.CreatedTenantStatus}, nil).Once()
			},
			reqStatus:   tenant.ActivatedTenantStatus,
			expectedErr: ErrCannotActivateTenant,
		},
		{
			name: "cannot perform update on tenant to requested status",
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock) {
				tntManagerMock.On("GetTenant", ctx, &tenant.QueryParams{
					Filters: map[tenant.FilterKey]interface{}{
						tenant.FilterKeyID: tntID,
					},
				}).Return(&tenant.Tenant{ID: tntID, Status: tenant.DeactivatedTenantStatus}, nil).Once()
			},
			reqStatus:   tenant.CreatedTenantStatus,
			expectedErr: ErrCannotPerformStatusUpdate,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockTntManagerFn(&tenantManagerMock)
			if tc.createFixtures != nil {
				tc.createFixtures()
			}

			validateStatusErr := ValidateStatus(ctx, &tenantManagerMock, models, tntID, tc.reqStatus)
			if tc.expectedErr != nil {
				require.Error(t, validateStatusErr)
				assert.ErrorContains(t, validateStatusErr, tc.expectedErr.Error())
			} else {
				require.NoError(t, validateStatusErr)
			}
			if tc.deleteFixtures != nil {
				tc.deleteFixtures()
			}
		})

		tenantManagerMock.AssertExpectations(t)
	}
}
