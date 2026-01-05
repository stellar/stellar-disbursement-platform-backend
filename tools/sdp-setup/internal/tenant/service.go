package tenant

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/tools/sdp-setup/internal/config"
)

var TestnetTenants = []string{"redcorp", "bluecorp", "pinkcorp"}

// Service handles tenant and user management operations
type Service struct {
	cfg        config.Config
	httpClient *http.Client
}

// NewService creates a new tenant service
func NewService(cfg config.Config) *Service {
	return &Service{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// InitializeEnvironment handles tenant and user initialization with proper error handling
func (s *Service) InitializeEnvironment(ctx context.Context) error {
	fmt.Println("🔄 Waiting for services to be ready...")
	time.Sleep(10 * time.Second)

	var err error
	if s.cfg.SingleTenantMode {
		err = s.initializeSingleTenantEnvironment(ctx)
	} else {
		err = s.initializeMultiTenantEnvironment()
	}

	if err != nil {
		return fmt.Errorf("failed to initialize environment: %w", err)
	}

	s.printLoginHints()
	return nil
}

// initializeSingleTenantEnvironment sets up the default tenant and user
func (s *Service) initializeSingleTenantEnvironment(ctx context.Context) error {
	fmt.Println("🏢 Setting up single tenant environment...")

	fmt.Println("====> initialize default tenant using CLI")

	dbURL := fmt.Sprintf("postgres://postgres@localhost:5432/%s?sslmode=disable", s.cfg.DatabaseName)

	args := []string{
		"run", "..", "--log-level", "ERROR", "tenants", "ensure-default",
		"--database-url", dbURL,
		"--default-tenant-owner-email", "default@default.local",
		"--default-tenant-owner-first-name", "Default",
		"--default-tenant-owner-last-name", "Owner",
		"--distribution-public-key", s.cfg.DistributionPublic,
		"--distribution-seed", s.cfg.DistributionSeed,
		"--network-passphrase", s.cfg.NetworkPassphrase,
		"--horizon-url", s.cfg.HorizonURL,
		"--default-tenant-distribution-account-type", "DISTRIBUTION_ACCOUNT.STELLAR.ENV",
		"--distribution-account-encryption-passphrase", s.cfg.DistributionSeed,
		"--channel-account-encryption-passphrase", s.cfg.DistributionSeed,
		"--disable-mfa", s.cfg.DisableMFA,
	}

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = s.cfg.WorkDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ensuring default tenant: %w\nOutput: %s", err, string(out))
	}

	fmt.Printf("Default tenant ensured successfully\n")

	if err = s.addUserForSingleTenant(ctx, dbURL); err != nil {
		return fmt.Errorf("adding user for single tenant: %w", err)
	}

	fmt.Println("✅ Single tenant environment initialized successfully")
	return nil
}

// initializeMultiTenantEnvironment sets up multiple tenants and their users
func (s *Service) initializeMultiTenantEnvironment() error {
	fmt.Println("🏢 Setting up multi-tenant environment...")

	// First, ensure tenants are created
	if err := s.InitializeDefaultTenants(context.Background()); err != nil {
		return fmt.Errorf("failed to initialize tenants: %w", err)
	}
	fmt.Println("✅ Multi-tenant environment initialized successfully")
	return nil
}

// InitializeDefaultTenants creates the default tenants if they don't exist
func (s *Service) InitializeDefaultTenants(ctx context.Context) error {
	existingTenants, err := s.FetchTenants(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch existing tenants: %w", err)
	}

	existingNames := s.buildTenantNameSet(existingTenants)

	for _, tenantName := range TestnetTenants {
		if existingNames[tenantName] {
			fmt.Printf("🔵Tenant %s already exists. Skipping.\n", tenantName)
			continue
		}

		// 1. Create tenant
		if err = s.createTenant(ctx, tenantName); err != nil {
			return fmt.Errorf("failed to create tenant %s: %w", tenantName, err)
		}
		fmt.Printf("✅Tenant %s created.\n", tenantName)

		// 2. Add user for tenant
		var tenant schema.Tenant
		tenant, err = s.FetchTenant(ctx, tenantName)
		if err != nil {
			fmt.Printf("⚠️  Fetching tenant %s failed: %v\n", tenantName, err)
			continue
		}
		if err = s.addUserForTenant(ctx, tenant); err != nil {
			fmt.Printf("⚠️  Adding user for tenant %s failed: %v\n", tenant.Name, err)
			continue
		}
	}
	return nil
}

// FetchTenant retrieves an existing tenant by name
func (s *Service) FetchTenant(ctx context.Context, name string) (schema.Tenant, error) {
	if name == "" {
		return schema.Tenant{}, fmt.Errorf("tenant name cannot be empty")
	}
	req, err := s.buildAuthenticatedRequest(ctx, "GET", fmt.Sprintf("/tenants/%s", name), nil)
	if err != nil {
		return schema.Tenant{}, fmt.Errorf("creating GET request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return schema.Tenant{}, fmt.Errorf("fetching tenant: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close response body: %v\n", closeErr)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return schema.Tenant{}, fmt.Errorf("GET /tenants/:name response code = %d", resp.StatusCode)
	}

	var tenant schema.Tenant
	if err = json.NewDecoder(resp.Body).Decode(&tenant); err != nil {
		return schema.Tenant{}, fmt.Errorf("decoding GET /tenants/%s: %w", name, err)
	}
	return tenant, nil
}

// FetchTenants retrieves all existing tenants
func (s *Service) FetchTenants(ctx context.Context) ([]schema.Tenant, error) {
	req, err := s.buildAuthenticatedRequest(ctx, "GET", "/tenants", nil)
	if err != nil {
		return nil, fmt.Errorf("creating GET request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching tenants: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close response body: %v\n", closeErr)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET /tenants: http %d", resp.StatusCode)
	}

	var tenants []schema.Tenant
	if err := json.NewDecoder(resp.Body).Decode(&tenants); err != nil {
		return nil, fmt.Errorf("decoding tenants response: %w", err)
	}
	return tenants, nil
}

// addUserForTenant creates a test user for a specific tenant
func (s *Service) addUserForTenant(ctx context.Context, tenant schema.Tenant) error {
	email := fmt.Sprintf("owner@%s.local", strings.TrimSpace(tenant.Name))
	args := []string{
		"run", "..", "--log-level", "ERROR", "auth", "add-user", email, "john", "doe",
		"--password", "--owner", "--roles", "owner", "--tenant-id", tenant.ID, "--database-url", s.dbURL(),
	}

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = s.cfg.WorkDir
	cmd.Stdin = bytes.NewBufferString("Password123!\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(strings.ToLower(string(out)), "already exists") {
			fmt.Printf("🔵 User %s already exists for tenant %s. Skipping.\n", email, tenant.Name)
			return nil
		}
		return fmt.Errorf("command failed: %w\nOutput: %s", err, string(out))
	}

	return nil
}

// buildTenantNameSet creates a set of existing tenant names for quick lookup
func (s *Service) buildTenantNameSet(tenants []schema.Tenant) map[string]bool {
	existingNames := make(map[string]bool)
	for _, tenant := range tenants {
		existingNames[tenant.Name] = true
	}
	return existingNames
}

// createTenant creates a single tenant via API
func (s *Service) createTenant(ctx context.Context, tenantName string) error {
	tenantData, err := s.buildTenantPayload(tenantName)
	if err != nil {
		return fmt.Errorf("building tenant payload: %w", err)
	}

	req, err := s.buildAuthenticatedRequest(ctx, "POST", "/tenants", strings.NewReader(string(tenantData)))
	if err != nil {
		return fmt.Errorf("building create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending create request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close response body for tenant %s: %v\n", tenantName, closeErr)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("create tenant API returned status %d", resp.StatusCode)
	}

	return nil
}

// buildTenantPayload creates the JSON payload for tenant creation
func (s *Service) buildTenantPayload(tenantName string) ([]byte, error) {
	uiBaseURL := s.cfg.FrontendBaseURL(s.frontendHost(tenantName))
	payload := struct {
		Name                    string `json:"name"`
		OrganizationName        string `json:"organization_name"`
		BaseURL                 string `json:"base_url"`
		SDPUIBaseURL            string `json:"sdp_ui_base_url"`
		OwnerEmail              string `json:"owner_email"`
		OwnerFirstName          string `json:"owner_first_name"`
		OwnerLastName           string `json:"owner_last_name"`
		DistributionAccountType string `json:"distribution_account_type"`
	}{
		Name:                    tenantName,
		OrganizationName:        tenantName,
		BaseURL:                 fmt.Sprintf("http://%s.stellar.local:8000", tenantName),
		SDPUIBaseURL:            uiBaseURL,
		OwnerEmail:              fmt.Sprintf("init_owner@%s.local", tenantName),
		OwnerFirstName:          "jane",
		OwnerLastName:           "doe",
		DistributionAccountType: "DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT",
	}

	return json.Marshal(payload)
}

// frontendHost resolves the UI host for a tenant, keeping localhost in single-tenant setups.
func (s *Service) frontendHost(tenantName string) string {
	if s.cfg.SingleTenantMode && tenantName == "default" {
		return "localhost"
	}
	return fmt.Sprintf("%s.stellar.local", tenantName)
}

// buildAuthenticatedRequest creates an authenticated HTTP request
func (s *Service) buildAuthenticatedRequest(ctx context.Context, method, endpoint string, body *strings.Reader) (*http.Request, error) {
	url := s.cfg.AdminURL + endpoint
	var req *http.Request
	var err error

	if body != nil {
		req, err = http.NewRequestWithContext(ctx, method, url, body)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("creating HTTP request: %w", err)
	}

	auth := base64.StdEncoding.EncodeToString([]byte(s.cfg.AdminUser + ":" + s.cfg.AdminKey))
	req.Header.Set("Authorization", "Basic "+auth)

	return req, nil
}

// addUserForSingleTenant creates a test user for the default tenant
func (s *Service) addUserForSingleTenant(ctx context.Context, dbURL string) error {
	fmt.Println("====> adding owner user for single tenant")

	tenantID, err := s.getDefaultTenantID(ctx)
	if err != nil {
		return fmt.Errorf("getting default tenant ID: %w", err)
	}

	userArgs := []string{
		"run", "..", "--log-level", "ERROR", "auth", "add-user",
		"owner@default.local", "Default", "Owner",
		"--password", "--owner", "--roles", "owner",
		"--tenant-id", tenantID,
		"--database-url", dbURL,
	}

	cmd := exec.CommandContext(ctx, "go", userArgs...)
	cmd.Dir = s.cfg.WorkDir
	cmd.Stdin = bytes.NewBufferString("Password123!\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(strings.ToLower(string(out)), "already exists") {
			fmt.Printf("User owner@default.local already exists. Skipping.\n")
			return nil
		}
		return fmt.Errorf("adding user: %w\nOutput: %s", err, string(out))
	}

	fmt.Printf("✅ Added user owner@default.local with password Password123!\n")
	return nil
}

// getDefaultTenantID gets the default tenant ID using the admin API
func (s *Service) getDefaultTenantID(ctx context.Context) (string, error) {
	tenants, err := s.FetchTenants(ctx)
	if err != nil {
		return "", fmt.Errorf("fetching tenants: %w", err)
	}

	for _, tenant := range tenants {
		if tenant.Name == "default" {
			return tenant.ID, nil
		}
	}

	if len(tenants) > 0 {
		return tenants[0].ID, nil
	}

	return "", fmt.Errorf("no tenants found")
}

func (s *Service) printLoginHints() {
	fmt.Println("\n🎉🎉🎉🎉 SUCCESS! 🎉🎉🎉🎉")

	if s.cfg.SingleTenantMode {
		fmt.Println("Single tenant mode - Login URL:")
		fmt.Printf("🔗Default tenant: %s\n  username: owner@default.local  password: Password123!\n", s.cfg.FrontendBaseURL("localhost"))
	} else {
		fmt.Println("Multi-tenant mode - Login URLs for each tenant:")
		for _, t := range TestnetTenants {
			host := fmt.Sprintf("%s.stellar.local", t)
			fmt.Printf("🔗Tenant %s: %s\n  username: owner@%s.local  password: Password123!\n", t, s.cfg.FrontendBaseURL(host), t)
		}
	}
}

func (s *Service) dbURL() string {
	return fmt.Sprintf("postgres://postgres@localhost:5432/%s?sslmode=disable", s.cfg.DatabaseName)
}
