package tenant

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

var defaultTenants = []string{"redcorp", "bluecorp", "pinkcorp"}

// Service handles tenant and user management operations
type Service struct {
	baseURL   string
	adminUser string
	adminKey  string
	workDir   string
	dbURL     string
}

type ServiceOpts struct {
	BaseURL     string
	AdminUser   string
	AdminKey    string
	WorkDir     string
	DatabaseURL string
}

// NewService creates a new tenant service
func NewService(opts ServiceOpts) *Service {
	return &Service{
		baseURL:   opts.BaseURL,
		adminUser: opts.AdminUser,
		adminKey:  opts.AdminKey,
		workDir:   opts.WorkDir,
		dbURL:     opts.DatabaseURL,
	}
}

// InitializeDefaultTenants creates the default tenants if they don't exist
func (s *Service) InitializeDefaultTenants() error {
	existingTenants, err := s.FetchTenants()
	if err != nil {
		return fmt.Errorf("failed to fetch existing tenants: %w", err)
	}

	existingNames := s.buildTenantNameSet(existingTenants)

	for _, tenantName := range defaultTenants {
		if existingNames[tenantName] {
			fmt.Printf("🔵Tenant %s already exists. Skipping.\n", tenantName)
			continue
		}

		// 1. Create tenant
		if err = s.createTenant(tenantName); err != nil {
			return fmt.Errorf("failed to create tenant %s: %w", tenantName, err)
		}
		fmt.Printf("✅Tenant %s created.\n", tenantName)

		// 2. Add user for tenant
		var tenant schema.Tenant
		tenant, err = s.FetchTenant(tenantName)
		if err != nil {
			fmt.Printf("⚠️  Fetching tenant %s failed: %v\n", tenantName, err)
			continue
		}
		if err = s.addUserForTenant(tenant); err != nil {
			fmt.Printf("⚠️  Adding user for tenant %s failed: %v\n", tenant.Name, err)
			continue
		}
	}
	return nil
}

// addUser create default user for tenant
//func (s *Service) addDefaultUser(env map[string]string, string tenantName) error {
//	fmt.Println("====> initialize test users (host CLI)")
//
//
//	dbName := env["DATABASE_NAME"]
//	if dbName == "" {
//		dbName = "sdp_mtn"
//	}
//	dbURL := fmt.Sprintf("postgres://postgres@localhost:5432/%s?sslmode=disable", dbName)
//
//	for _, tenant := range tenants {
//
//		fmt.Printf("✅ Added user for tenant %s\n", tenant.Name)
//	}
//	return nil
//}

// FetchTenant retrieves an existing tenant by name
func (s *Service) FetchTenant(name string) (schema.Tenant, error) {
	if name == "" {
		return schema.Tenant{}, fmt.Errorf("tenant name cannot be empty")
	}
	req, err := s.buildAuthenticatedRequest("GET", fmt.Sprintf("/tenants/%s", name), nil)
	if err != nil {
		return schema.Tenant{}, fmt.Errorf("creating GET request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
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
func (s *Service) FetchTenants() ([]schema.Tenant, error) {
	req, err := s.buildAuthenticatedRequest("GET", "/tenants", nil)
	if err != nil {
		return nil, fmt.Errorf("creating GET request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
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
func (s *Service) addUserForTenant(tenant schema.Tenant) error {
	email := fmt.Sprintf("owner@%s.local", strings.TrimSpace(tenant.Name))
	args := []string{
		"run", "..", "--log-level", "ERROR", "auth", "add-user", email, "john", "doe",
		"--password", "--owner", "--roles", "owner", "--tenant-id", tenant.ID, "--database-url", s.dbURL,
	}

	cmd := exec.Command("go", args...)
	cmd.Dir = s.workDir
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
func (s *Service) createTenant(tenantName string) error {
	tenantData := s.buildTenantPayload(tenantName)

	req, err := s.buildAuthenticatedRequest("POST", "/tenants", strings.NewReader(tenantData))
	if err != nil {
		return fmt.Errorf("building create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
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
func (s *Service) buildTenantPayload(tenantName string) string {
	return fmt.Sprintf(`{
		"name": %q,
		"organization_name": %q,
		"base_url": %q,
		"sdp_ui_base_url": %q,
		"owner_email": %q,
		"owner_first_name": "jane",
		"owner_last_name": "doe",
		"distribution_account_type": "DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT"
	}`,
		tenantName,
		tenantName,
		fmt.Sprintf("http://%s.stellar.local:8000", tenantName),
		fmt.Sprintf("http://%s.stellar.local:3000", tenantName),
		fmt.Sprintf("init_owner@%s.local", tenantName),
	)
}

// buildAuthenticatedRequest creates an authenticated HTTP request
func (s *Service) buildAuthenticatedRequest(method, endpoint string, body *strings.Reader) (*http.Request, error) {
	url := s.baseURL + endpoint
	var req *http.Request
	var err error

	if body != nil {
		req, err = http.NewRequest(method, url, body)
	} else {
		req, err = http.NewRequest(method, url, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("creating HTTP request: %w", err)
	}

	auth := base64.StdEncoding.EncodeToString([]byte(s.adminUser + ":" + s.adminKey))
	req.Header.Set("Authorization", "Basic "+auth)

	return req, nil
}

// InitializeSingleTenant sets up the default tenant for single-tenant mode
func (s *Service) InitializeSingleTenant(env map[string]string) error {
	fmt.Println("====> initialize default tenant using CLI")

	dbName := env["DATABASE_NAME"]
	if dbName == "" {
		dbName = "sdp_mtn"
	}
	dbURL := fmt.Sprintf("postgres://postgres@localhost:5432/%s?sslmode=disable", dbName)

	distributionPublicKey := env["DISTRIBUTION_PUBLIC_KEY"]
	if distributionPublicKey == "" {
		return fmt.Errorf("DISTRIBUTION_PUBLIC_KEY not found in env file")
	}

	distributionSeed := env["DISTRIBUTION_SEED"]
	if distributionSeed == "" {
		return fmt.Errorf("DISTRIBUTION_SEED not found in env file")
	}

	networkPassphrase := env["NETWORK_PASSPHRASE"]
	if networkPassphrase == "" {
		networkPassphrase = "Test SDF Network ; September 2015"
	}

	horizonURL := env["HORIZON_URL"]
	if horizonURL == "" {
		horizonURL = "https://horizon-testnet.stellar.org"
	}

	args := []string{
		"run", "..", "--log-level", "ERROR", "tenants", "ensure-default",
		"--database-url", dbURL,
		"--default-tenant-owner-email", "default@default.local",
		"--default-tenant-owner-first-name", "Default",
		"--default-tenant-owner-last-name", "Owner",
		"--distribution-public-key", distributionPublicKey,
		"--distribution-seed", distributionSeed,
		"--network-passphrase", networkPassphrase,
		"--horizon-url", horizonURL,
		"--default-tenant-distribution-account-type", "DISTRIBUTION_ACCOUNT.STELLAR.ENV",
		"--distribution-account-encryption-passphrase", distributionSeed,
		"--channel-account-encryption-passphrase", distributionSeed,
	}

	cmd := exec.Command("go", args...)
	cmd.Dir = s.workDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ensuring default tenant: %w\nOutput: %s", err, string(out))
	}

	fmt.Printf("Default tenant ensured successfully\n")

	if err := s.addUserForSingleTenant(dbURL); err != nil {
		return fmt.Errorf("adding user for single tenant: %w", err)
	}

	return nil
}

// addUserForSingleTenant creates a test user for the default tenant
func (s *Service) addUserForSingleTenant(dbURL string) error {
	fmt.Println("====> adding owner user for single tenant")

	tenantID, err := s.getDefaultTenantID()
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

	cmd := exec.Command("go", userArgs...)
	cmd.Dir = s.workDir
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
func (s *Service) getDefaultTenantID() (string, error) {
	tenants, err := s.FetchTenants()
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
