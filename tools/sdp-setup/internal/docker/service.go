package docker

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/tools/sdp-setup/internal/envfile"
	"github.com/stellar/stellar-disbursement-platform-backend/tools/sdp-setup/internal/tenant"
	"github.com/stellar/stellar-disbursement-platform-backend/tools/sdp-setup/internal/ui"
)

const defaultWorkDir = "dev"

// ErrUserDeclined is returned when the user chooses not to proceed with an operation
var ErrUserDeclined = errors.New("user declined to proceed")

// Service provides Docker Compose operations for SDP environment.
type Service struct {
	workDir   string
	config    Config
	tenantSvc *tenant.Service
	env       map[string]string
}

// NewService creates a new Docker service.
func NewService(config Config) (*Service, error) {
	finalConfig := mergeWithDefaults(config)

	// Load environment variables from the specified env file
	env, err := envfile.LoadDotEnv(config.EnvFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load env file %s: %w", config.EnvFile, err)
	}

	return &Service{
		workDir:   defaultWorkDir,
		config:    finalConfig,
		tenantSvc: tenant.NewService(finalConfig.AdminURL, finalConfig.AdminUser, finalConfig.AdminKey, defaultWorkDir),
		env:       env,
	}, nil
}

// mergeWithDefaults merges the provided config with defaults, preferring provided values
func mergeWithDefaults(config Config) Config {
	defaults := defaultConfig()

	if config.Project != "" {
		defaults.Project = config.Project
	}
	if config.AdminUser != "" {
		defaults.AdminUser = config.AdminUser
	}
	if config.AdminKey != "" {
		defaults.AdminKey = config.AdminKey
	}
	if config.AdminURL != "" {
		defaults.AdminURL = config.AdminURL
	}
	if config.EnvFile != "" {
		defaults.EnvFile = config.EnvFile
	}
	if config.Network != "" {
		defaults.Network = config.Network
	}

	return defaults
}

// Config holds the configuration for Docker operations.
type Config struct {
	EnvFile   string
	Project   string
	Network   string
	AdminUser string
	AdminKey  string
	AdminURL  string
}

// DefaultConfig returns default configuration.
func DefaultConfig() Config {
	return Config{
		Project:   "sdp",
		AdminUser: "SDP-admin",
		AdminKey:  "api_key_1234567890",
		AdminURL:  "http://localhost:8003",
	}
}

// defaultConfig is an internal alias for DefaultConfig
func defaultConfig() Config {
	return DefaultConfig()
}

// Preflight checks for required tools and running docker daemon.
func Preflight() error {
	for _, bin := range []string{"docker", "curl", "go"} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("required tool '%s' not found in PATH", bin)
		}
	}
	if err := exec.Command("docker", "ps").Run(); err != nil {
		return fmt.Errorf("docker not running: %w", err)
	}
	if err := exec.Command("docker", "compose", "version").Run(); err != nil {
		return fmt.Errorf("docker compose missing: %w", err)
	}
	return nil
}

// StartDockerStack starts the Docker stack and initializes tenants/users.
func (s *Service) StartDockerStack() error {
	if err := Preflight(); err != nil {
		return err
	}

	network := s.env["NETWORK_TYPE"]
	if network == "" {
		network = "testnet"
	}
	s.config.Network = network

	// Derive project name from env SETUP_NAME if default project is used
	if s.config.Project == "sdp" {
		if setup := strings.TrimSpace(s.env["SETUP_NAME"]); setup != "" {
			s.config.Project = fmt.Sprintf("sdp-%s", setup)
		}
	}

	fmt.Printf("Using network: %s (project=%s)\n", network, s.config.Project)

	// Prompt to stop other running SDP projects (prevents port conflicts)
	if err := s.ensureExclusiveProject(s.config.Project); err != nil {
		return err
	}

	if err := s.composeDown(); err != nil {
		return err
	}

	if err := s.composeUp(); err != nil {
		return err
	}

	// Ask user if they want to perform initialization
	if ui.Confirm("Initialize tenants and users") {
		if err := s.initializeEnvironment(); err != nil {
			fmt.Printf("⚠️  Environment initialization failed: %v\n", err)
			fmt.Println("💡 You can run the setup again later to retry initialization")
		}
	} else {
		fmt.Println("⏭️  Skipping tenant and user initialization")
	}

	s.printLoginHints(s.env["SINGLE_TENANT_MODE"] == "true")
	return nil
}

// initializeEnvironment handles tenant and user initialization with proper error handling
func (s *Service) initializeEnvironment() error {
	fmt.Println("🔄 Waiting for services to be ready...")
	time.Sleep(10 * time.Second)

	singleTenantMode := s.env["SINGLE_TENANT_MODE"] == "true"

	if singleTenantMode {
		return s.initializeSingleTenantEnvironment()
	}
	return s.initializeMultiTenantEnvironment()
}

// initializeSingleTenantEnvironment sets up the default tenant and user
func (s *Service) initializeSingleTenantEnvironment() error {
	fmt.Println("🏢 Setting up single tenant environment...")

	if err := s.tenantSvc.InitializeSingleTenant(s.env); err != nil {
		return fmt.Errorf("failed to initialize default tenant: %w", err)
	}

	fmt.Println("✅ Single tenant environment initialized successfully")
	return nil
}

// initializeMultiTenantEnvironment sets up multiple tenants and their users
func (s *Service) initializeMultiTenantEnvironment() error {
	fmt.Println("🏢 Setting up multi-tenant environment...")

	// First, ensure tenants are created
	if err := s.tenantSvc.InitializeDefaultTenants(); err != nil {
		return fmt.Errorf("failed to initialize tenants: %w", err)
	}
	fmt.Println("✅ Tenants initialized")

	// Only proceed with user creation if tenant creation succeeded
	if err := s.tenantSvc.AddTestUsers(s.env); err != nil {
		return fmt.Errorf("failed to add test users: %w", err)
	}
	fmt.Println("✅ Test users added")

	fmt.Println("✅ Multi-tenant environment initialized successfully")
	return nil
}

// composeDown runs 'docker compose down' with the given config.
func (s *Service) composeDown() error {
	fmt.Println("====> docker compose down")
	args := []string{"compose", "-p", s.config.Project}
	if s.config.EnvFile != "" {
		if abs, err := filepath.Abs(s.config.EnvFile); err == nil {
			s.config.EnvFile = abs
		}
		args = append(args, "--env-file", s.config.EnvFile)
	}
	args = append(args, "down")
	cmd := exec.Command("docker", args...)
	cmd.Dir = s.workDir
	cmd.Env = append(os.Environ(), fmt.Sprintf("COMPOSE_PROJECT_NAME=%s", s.config.Project))
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running docker compose down: %w", err)
	}
	return nil
}

// composeUp runs 'docker compose up -d' with the given config.
func (s *Service) composeUp() error {
	fmt.Println("====> docker compose up -d --build")
	env := append(os.Environ(), "GIT_COMMIT=debug", fmt.Sprintf("COMPOSE_PROJECT_NAME=%s", s.config.Project))
	args := []string{"compose", "-p", s.config.Project}
	if s.config.EnvFile != "" {
		if abs, err := filepath.Abs(s.config.EnvFile); err == nil {
			s.config.EnvFile = abs
		}
		args = append(args, "--env-file", s.config.EnvFile)
	}
	args = append(args, "up", "-d")
	cmd := exec.Command("docker", args...)
	cmd.Dir = s.workDir
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running docker compose up: %w", err)
	}
	return nil
}

// ensureExclusiveProject checks for other running docker compose projects and prompts to stop them.
func (s *Service) ensureExclusiveProject(target string) error {
	others, err := s.listOtherActiveSDPProjects(target)
	if err != nil {
		return err
	}
	if len(others) == 0 {
		return nil
	}
	fmt.Printf("Detected other running SDP projects: %s\n", strings.Join(others, ", "))
	if !ui.Confirm("Stop these projects now") {
		fmt.Printf("⚠️  Cannot proceed with conflicting projects running.\n")
		fmt.Printf("   To launch manually later, stop conflicting projects first.\n")
		return ErrUserDeclined
	}
	// Stop each other project
	for _, p := range others {
		if err = s.composeStop(p, ""); err != nil {
			return fmt.Errorf("failed to stop project %s: %w", p, err)
		}
	}
	return nil
}

func (s *Service) composeStop(project, envFile string) error {
	fmt.Println("====> docker compose stop")
	args := []string{"compose", "-p", project}
	if envFile != "" {
		if abs, err := filepath.Abs(envFile); err == nil {
			envFile = abs
		}
		args = append(args, "--env-file", envFile)
	}
	args = append(args, "stop")
	cmd := exec.Command("docker", args...)
	cmd.Dir = s.workDir
	cmd.Env = append(os.Environ(), fmt.Sprintf("COMPOSE_PROJECT_NAME=%s", project))
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running docker compose stop: %w", err)
	}
	return nil
}

// listOtherActiveSDPProjects lists active docker compose projects with "sdp" prefix, excluding the specified project.
func (s *Service) listOtherActiveSDPProjects(exclude string) ([]string, error) {
	cmd := exec.Command("docker", "ps", "--format", "{{.ID}} {{.Label \"com.docker.compose.project\"}}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker ps failed: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	seen := map[string]bool{}
	var projects []string
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		proj := strings.TrimSpace(fields[1])
		if proj == "" || !strings.HasPrefix(proj, "sdp") || proj == exclude {
			continue
		}
		if !seen[proj] {
			seen[proj] = true
			projects = append(projects, proj)
		}
	}
	return projects, nil
}

func (s *Service) printLoginHints(singleTenantMode bool) {
	fmt.Println("\n🎉🎉🎉🎉 SUCCESS! 🎉🎉🎉🎉")

	if singleTenantMode {
		fmt.Println("Single tenant mode - Login URL:")
		fmt.Printf("🔗Default tenant: http://localhost:3000\n  username: owner@default.local  password: Password123!\n")
	} else {
		tenants := []string{"redcorp", "bluecorp", "pinkcorp"}
		fmt.Println("Multi-tenant mode - Login URLs for each tenant:")
		for _, t := range tenants {
			fmt.Printf("🔗Tenant %s: http://%s.stellar.local:3000\n  username: owner@%s.local  password: Password123!\n", t, t, t)
		}
	}
}
