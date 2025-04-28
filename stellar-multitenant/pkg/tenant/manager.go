package tenant

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/router"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

var (
	ErrTenantDoesNotExist      = errors.New("tenant does not exist")
	ErrDuplicatedTenantName    = errors.New("duplicated tenant name")
	ErrEmptyTenantName         = errors.New("tenant name cannot be empty")
	ErrEmptyUpdateTenant       = errors.New("provide at least one field to be updated")
	ErrTenantNotFoundInContext = errors.New("tenant not found in context")
	ErrTooManyDefaultTenants   = errors.New("too many default tenants. Expected at most one default tenant")
)

type tenantContextKey struct{}

type ManagerInterface interface {
	GetDSNForTenant(ctx context.Context, tenantName string) (string, error)
	GetDSNForTenantByID(ctx context.Context, id string) (string, error)
	GetAllTenants(ctx context.Context, queryParams *QueryParams) ([]Tenant, error)
	GetTenant(ctx context.Context, queryParams *QueryParams) (*Tenant, error)
	GetTenantByID(ctx context.Context, id string) (*Tenant, error)
	GetTenantByName(ctx context.Context, name string) (*Tenant, error)
	GetTenantByIDOrName(ctx context.Context, arg string) (*Tenant, error)
	GetDefault(ctx context.Context) (*Tenant, error)
	SetDefault(ctx context.Context, sqlExec db.SQLExecuter, id string) (*Tenant, error)
	AddTenant(ctx context.Context, name string) (*Tenant, error)
	DeleteTenantByName(ctx context.Context, name string) error
	CreateTenantSchema(ctx context.Context, tenantName string) error
	DropTenantSchema(ctx context.Context, tenantName string) error
	UpdateTenantConfig(ctx context.Context, tu *TenantUpdate) (*Tenant, error)
	SoftDeleteTenantByID(ctx context.Context, tenantID string) (*Tenant, error)
	DeactivateTenantDistributionAccount(ctx context.Context, tenantID string) error
}

type Manager struct {
	db               db.DBConnectionPool
	singleTenantMode bool
}

func (m *Manager) GetDSNForTenant(ctx context.Context, tenantName string) (string, error) {
	dataSourceName, err := m.db.DSN(ctx)
	if err != nil {
		return "", fmt.Errorf("getting database DSN: %w", err)
	}

	return router.GetDSNForTenant(dataSourceName, tenantName)
}

func (m *Manager) GetDSNForTenantByID(ctx context.Context, id string) (string, error) {
	t, err := m.GetTenantByID(ctx, id)
	if err != nil {
		return "", fmt.Errorf("getting tenant for ID %s: %w", id, err)
	}

	return m.GetDSNForTenant(ctx, t.Name)
}

var selectQuery = `
	SELECT 
		*
	FROM
		tenants t
`

// GetAllTenants returns all tenants in the database.
func (m *Manager) GetAllTenants(ctx context.Context, queryParams *QueryParams) ([]Tenant, error) {
	if queryParams == nil {
		queryParams = &QueryParams{
			Filters:   excludeInactiveTenantsFilters(),
			SortBy:    data.SortFieldName,
			SortOrder: data.SortOrderASC,
		}
	}

	tnts := []Tenant{}
	query, params := m.newManagerQuery(selectQuery, queryParams)
	err := m.db.SelectContext(ctx, &tnts, query, params...)
	if err != nil {
		return nil, fmt.Errorf("getting all tenants: %w", err)
	}

	return tnts, nil
}

// GetTenant is a generic method that fetches a tenant based on queryParams.
func (m *Manager) GetTenant(ctx context.Context, queryParams *QueryParams) (*Tenant, error) {
	var t Tenant
	q, params := m.newManagerQuery(selectQuery, queryParams)
	if err := m.db.GetContext(ctx, &t, q, params...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTenantDoesNotExist
		}
		return nil, fmt.Errorf("getting tenant: %w", err)
	}
	return &t, nil
}

func (m *Manager) GetTenantByID(ctx context.Context, id string) (*Tenant, error) {
	queryParams := &QueryParams{
		Filters: excludeInactiveTenantsFilters(),
	}
	queryParams.Filters[FilterKeyID] = id

	return m.GetTenant(ctx, queryParams)
}

func (m *Manager) GetTenantByName(ctx context.Context, name string) (*Tenant, error) {
	queryParams := &QueryParams{
		Filters: excludeInactiveTenantsFilters(),
	}
	queryParams.Filters[FilterKeyName] = name

	return m.GetTenant(ctx, queryParams)
}

// GetTenantByIDOrName returns the tenant with a given id or name.
func (m *Manager) GetTenantByIDOrName(ctx context.Context, arg string) (*Tenant, error) {
	queryParams := &QueryParams{
		Filters: excludeInactiveTenantsFilters(),
	}
	queryParams.Filters[FilterKeyNameOrID] = arg

	return m.GetTenant(ctx, queryParams)
}

// GetDefault returns the tenant where is_default is true.
// In single tenant mode, if no default exists and there's only one tenant, it returns that tenant.
func (m *Manager) GetDefault(ctx context.Context) (*Tenant, error) {
	queryParams := &QueryParams{
		Filters: excludeInactiveTenantsFilters(),
	}

	tnts := []Tenant{}
	query, params := m.newManagerQuery(selectQuery, queryParams)
	err := m.db.SelectContext(ctx, &tnts, query, params...)
	if err != nil {
		return nil, fmt.Errorf("getting tenants: %w", err)
	}

	if len(tnts) == 0 {
		return nil, ErrTenantDoesNotExist
	}

	if len(tnts) == 1 && m.singleTenantMode {
		return &tnts[0], nil
	}

	var defaultTenant *Tenant
	defaultCount := 0

	for i, tnt := range tnts {
		if tnt.IsDefault {
			defaultCount++
			if defaultCount > 1 {
				return nil, ErrTooManyDefaultTenants
			}
			defaultTenant = &tnts[i]
		}
	}

	if defaultTenant != nil {
		return defaultTenant, nil
	}

	return nil, ErrTenantDoesNotExist
}

// SetDefault sets the is_default = true for the given tenant id.
func (m *Manager) SetDefault(ctx context.Context, sqlExec db.SQLExecuter, id string) (*Tenant, error) {
	const q = `
		WITH remove_old_default_tenant AS (
			UPDATE tenants SET is_default = false WHERE is_default = true
		)
		UPDATE tenants SET is_default = true WHERE id = $1 AND status != 'TENANT_DEACTIVATED' RETURNING *
	`

	var tnt Tenant
	err := sqlExec.GetContext(ctx, &tnt, q, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTenantDoesNotExist
		}
		return nil, fmt.Errorf("setting tenant id %s as default: %w", id, err)
	}

	return &tnt, nil
}

func (m *Manager) AddTenant(ctx context.Context, name string) (*Tenant, error) {
	if name == "" {
		return nil, ErrEmptyTenantName
	}

	const q = "INSERT INTO tenants (name) VALUES ($1) RETURNING *"
	var t Tenant
	if err := m.db.GetContext(ctx, &t, q, name); err != nil {
		if pqError, ok := err.(*pq.Error); ok && pqError.Constraint == "idx_unique_name" {
			return nil, ErrDuplicatedTenantName
		}
		return nil, fmt.Errorf("inserting tenant %s: %w", name, err)
	}
	return &t, nil
}

func (m *Manager) DeleteTenantByName(ctx context.Context, name string) error {
	if name == "" {
		return ErrEmptyTenantName
	}

	q := "DELETE FROM tenants WHERE name = $1"
	_, err := m.db.ExecContext(ctx, q, name)
	if err != nil {
		return fmt.Errorf("deleting tenant %s: %w", name, err)
	}
	return nil
}

func (m *Manager) SoftDeleteTenantByID(ctx context.Context, tenantID string) (*Tenant, error) {
	updateQuery := `
		UPDATE tenants t
		SET
			%s
	`

	q := fmt.Sprintf(updateQuery, "deleted_at = NOW()")
	queryParams := &QueryParams{
		Filters: getDeactivatedTenantsFilters(),
	}
	queryParams.Filters[FilterKeyID] = tenantID

	var t Tenant
	query, params := m.newManagerQuery(q, queryParams)
	query += " RETURNING *"

	err := m.db.GetContext(ctx, &t, query, params...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTenantDoesNotExist
		}
		return nil, fmt.Errorf("soft deleting tenant %s: %w", tenantID, err)
	}

	return &t, nil
}

// DeactivateTenantDistributionAccount sets a distribution account of status ACTIVE to PENDING_USER_ACTIVATION for the given tenant id,
// and is only used in the case where the distribution account is of type CircleDBVault.
func (m *Manager) DeactivateTenantDistributionAccount(ctx context.Context, tenantID string) error {
	q := `
		UPDATE tenants t
		SET
			distribution_account_status = 'PENDING_USER_ACTIVATION'
		WHERE id = $1
		AND distribution_account_type = 'DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT'
	`

	if _, err := m.db.ExecContext(ctx, q, tenantID); err != nil {
		return fmt.Errorf("deactivating distribution account for tenant %s: %w", tenantID, err)
	}

	return nil
}

func (m *Manager) CreateTenantSchema(ctx context.Context, tenantName string) error {
	schemaName := fmt.Sprintf("sdp_%s", tenantName)
	_, err := m.db.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", pq.QuoteIdentifier(schemaName)))
	if err != nil {
		return fmt.Errorf("creating schema for tenant %s: %w", schemaName, err)
	}

	return nil
}

func (m *Manager) DropTenantSchema(ctx context.Context, tenantName string) error {
	schemaName := fmt.Sprintf("sdp_%s", tenantName)
	_, err := m.db.ExecContext(ctx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", pq.QuoteIdentifier(schemaName)))
	if err != nil {
		return fmt.Errorf("dropping schema for tenant %s: %w", schemaName, err)
	}

	return nil
}

func (m *Manager) UpdateTenantConfig(ctx context.Context, tu *TenantUpdate) (*Tenant, error) {
	if tu == nil {
		return nil, fmt.Errorf("tenant update cannot be nil")
	}

	if err := tu.Validate(); err != nil {
		return nil, err
	}

	q := `
		UPDATE tenants
		SET
			%s
		WHERE
			id = ?
		RETURNING *
	`

	fields := make([]string, 0)
	args := make([]interface{}, 0)

	if tu.BaseURL != nil {
		fields = append(fields, "base_url = ?")
		args = append(args, *tu.BaseURL)
	}

	if tu.SDPUIBaseURL != nil {
		fields = append(fields, "sdp_ui_base_url = ?")
		args = append(args, *tu.SDPUIBaseURL)
	}

	if tu.Status != nil {
		fields = append(fields, "status = ?")
		args = append(args, *tu.Status)
	}

	fields, args = m.updateDistributionAccountFields(ctx, tu, fields, args)

	args = append(args, tu.ID)
	q = fmt.Sprintf(q, strings.Join(fields, ",\n"))
	q = m.db.Rebind(q)

	var t Tenant
	if err := m.db.GetContext(ctx, &t, q, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTenantDoesNotExist
		}
		return nil, fmt.Errorf("updating tenant ID %s: %w", tu.ID, err)
	}

	return &t, nil
}

func (*Manager) updateDistributionAccountFields(ctx context.Context, tu *TenantUpdate, fields []string, args []interface{}) (outFields []string, outArgs []interface{}) {
	if tu.DistributionAccountAddress != "" {
		fields = append(fields, "distribution_account_address = ?")
		args = append(args, tu.DistributionAccountAddress)

		log.Ctx(ctx).Warnf("distribution account address for tenant id %s updated to %s", tu.ID, tu.DistributionAccountAddress)
	}

	if tu.DistributionAccountType != "" {
		fields = append(fields, "distribution_account_type = ?")
		args = append(args, tu.DistributionAccountType)

		log.Ctx(ctx).Warnf("distribution account type for tenant id %s updated to %s", tu.ID, tu.DistributionAccountType)
	}

	if tu.DistributionAccountStatus != "" {
		fields = append(fields, "distribution_account_status = ?")
		args = append(args, tu.DistributionAccountStatus)

		log.Ctx(ctx).Warnf("distribution account status for tenant id %s updated to %s", tu.ID, tu.DistributionAccountStatus)
	}

	return fields, args
}

// GetTenantFromContext retrieves the tenant information from the context.
func GetTenantFromContext(ctx context.Context) (*Tenant, error) {
	currentTenant, ok := ctx.Value(tenantContextKey{}).(*Tenant)
	if !ok {
		return nil, ErrTenantNotFoundInContext
	}
	return currentTenant, nil
}

// SaveTenantInContext stores the tenant information in the context.
func SaveTenantInContext(ctx context.Context, t *Tenant) context.Context {
	return context.WithValue(ctx, tenantContextKey{}, t)
}

func (m *Manager) newManagerQuery(baseQuery string, queryParams *QueryParams) (string, []any) {
	qb := data.NewQueryBuilder(baseQuery)
	if queryParams.Filters[FilterKeyNameOrID] != nil {
		param := queryParams.Filters[FilterKeyNameOrID]
		qb.AddCondition("(t.name = ? OR t.id = ?)", param, param)
	}
	if queryParams.Filters[FilterKeyName] != nil {
		qb.AddCondition("t.name = ?", queryParams.Filters[FilterKeyName])
	}
	if queryParams.Filters[FilterKeyID] != nil {
		qb.AddCondition("t.id = ?", queryParams.Filters[FilterKeyID])
	}

	if queryParams.Filters[FilterKeyIsDefault] != nil {
		qb.AddCondition("t.is_default = ?", queryParams.Filters[FilterKeyIsDefault])
	}

	if queryParams.Filters[FilterKeyStatus] != nil {
		if statusSlice, ok := queryParams.Filters[FilterKeyStatus].([]TenantStatus); ok && len(statusSlice) > 0 {
			qb.AddCondition("t.status = ANY(?)", pq.Array(statusSlice))
		} else {
			qb.AddCondition("t.status = ?", queryParams.Filters[FilterKeyStatus])
		}
	} else if queryParams.Filters[FilterKeyOutStatus] != nil {
		if statusSlice, ok := queryParams.Filters[FilterKeyOutStatus].([]TenantStatus); ok && len(statusSlice) > 0 {
			qb.AddCondition("NOT (t.status = ANY(?))", pq.Array(statusSlice))
		} else {
			qb.AddCondition("t.status != ?", queryParams.Filters[FilterKeyOutStatus])
		}
	}

	if queryParams.Filters[FilterKeyDeleted] != nil {
		if queryParams.Filters[FilterKeyDeleted] == true {
			qb.AddCondition("t.deleted_at IS NULL")
		} else {
			qb.AddCondition("t.deleted_at IS NOT NULL")
		}
	}

	if queryParams.SortBy != "" && queryParams.SortOrder != "" {
		qb.AddSorting(queryParams.SortBy, queryParams.SortOrder, "t")
	}

	query, params := qb.Build()
	return m.db.Rebind(query), params
}

type Option func(m *Manager)

func NewManager(opts ...Option) *Manager {
	m := Manager{}
	for _, opt := range opts {
		opt(&m)
	}
	return &m
}

func WithDatabase(dbConnectionPool db.DBConnectionPool) Option {
	return func(m *Manager) {
		m.db = dbConnectionPool
	}
}

func WithSingleTenantMode(singleTenantMode bool) Option {
	return func(m *Manager) {
		m.singleTenantMode = singleTenantMode
	}
}

var _ ManagerInterface = (*Manager)(nil)
