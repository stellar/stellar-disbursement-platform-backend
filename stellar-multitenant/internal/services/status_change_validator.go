package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

var (
	ErrCannotRetrieveTenantByID                 = errors.New("cannot retrieve tenant by id")
	ErrCannotRetrievePayments                   = errors.New("cannot retrieve payments for tenant")
	ErrCannotDeactivateDefaultTenant            = errors.New("cannot deactivate default tenant")
	ErrCannotDeactivateTenantWithActivePayments = errors.New("cannot deactivate tenant with active payments")
	ErrCannotActivateTenant                     = errors.New("cannot activate tenant for tenant that is not deactivated")
	ErrCannotPerformStatusUpdate                = errors.New("cannot perform update on tenant to requested status")
)

func ValidateStatus(ctx context.Context, manager tenant.ManagerInterface, models *data.Models, tenantID string, reqStatus schema.TenantStatus) error {
	tnt, err := manager.GetTenant(ctx,
		&tenant.QueryParams{
			Filters: map[tenant.FilterKey]interface{}{
				tenant.FilterKeyID: tenantID,
			},
		})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCannotRetrieveTenantByID, err)
	}
	ctx = tenant.SaveTenantInContext(ctx, tnt)

	// if attempting to deactivate tenant, need to check for a few conditions such as
	// 1. whether tenant is already deactivated
	// 2. whether there are any payments still active
	if reqStatus == schema.DeactivatedTenantStatus {
		if tnt.Status == schema.DeactivatedTenantStatus {
			log.Ctx(ctx).Warnf("tenant %s is already deactivated", tenantID)
		} else {
			if tnt.IsDefault {
				return ErrCannotDeactivateDefaultTenant
			}

			activePaymentsCount, getPaymentCountErr := models.Payment.Count(ctx, &data.QueryParams{
				Filters: map[data.FilterKey]interface{}{
					data.FilterKeyStatus: data.PaymentActiveStatuses(),
				},
			}, models.DBConnectionPool)
			if getPaymentCountErr != nil {
				return fmt.Errorf("%w: %w", ErrCannotRetrievePayments, getPaymentCountErr)
			}

			if activePaymentsCount != 0 {
				return ErrCannotDeactivateTenantWithActivePayments
			}
		}
	} else if reqStatus == schema.ActivatedTenantStatus {
		if tnt.Status == schema.ActivatedTenantStatus {
			log.Ctx(ctx).Warnf("tenant %s is already activated", tenantID)
		} else if tnt.Status != schema.DeactivatedTenantStatus {
			return ErrCannotActivateTenant
		}
	} else {
		return ErrCannotPerformStatusUpdate
	}

	return nil
}
