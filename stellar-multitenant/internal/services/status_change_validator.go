package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

var (
	ErrCannotRetrieveTenantByID  = errors.New("cannot retrieve tenant by id")
	ErrCannotRetrievePayments    = errors.New("cannot retrieve payments for tenant")
	ErrCannotDeactivateTenant    = errors.New("cannot deactivate tenant")
	ErrCannotActivateTenant      = errors.New("cannot activate tenant for tenant that is not deactivated")
	ErrCannotPerformStatusUpdate = errors.New("cannot perform update on tenant to requested status")
)

func ValidateStatus(ctx context.Context, manager tenant.ManagerInterface, models *data.Models, tenantID string, reqStatus tenant.TenantStatus) error {
	tnt, err := manager.GetTenant(ctx,
		&tenant.QueryParams{Filters: map[tenant.FilterKey]interface{}{
			tenant.FilterKeyID: tenantID,
		},
		})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCannotRetrieveTenantByID, err)
	}

	// if attempting to deactivate tenant, need to check for a few conditions such as
	// 1. whether tenant is already deactivated
	// 2. whether there are any payments not in a terminal state
	if reqStatus == tenant.DeactivatedTenantStatus {
		if tnt.Status == tenant.DeactivatedTenantStatus {
			log.Ctx(ctx).Warnf("tenant %s is already deactivated", tenantID)
		} else {
			indeterminatePaymentsCount, getPaymentCountErr := models.Payment.Count(ctx, &data.QueryParams{
				Filters: map[data.FilterKey]interface{}{
					data.FilterKeyStatus: data.PaymentNonTerminalStatuses(),
				},
			}, models.DBConnectionPool)
			if getPaymentCountErr != nil {
				return fmt.Errorf("%w: %w", ErrCannotRetrievePayments, getPaymentCountErr)
			}

			if indeterminatePaymentsCount != 0 {
				return ErrCannotDeactivateTenant
			}
		}
	} else if reqStatus == tenant.ActivatedTenantStatus {
		if tnt.Status == tenant.ActivatedTenantStatus {
			log.Ctx(ctx).Warnf("tenant %s is already activated", tenantID)
		} else if tnt.Status != tenant.DeactivatedTenantStatus {
			return ErrCannotActivateTenant
		}
	} else {
		return ErrCannotPerformStatusUpdate
	}

	return nil
}
