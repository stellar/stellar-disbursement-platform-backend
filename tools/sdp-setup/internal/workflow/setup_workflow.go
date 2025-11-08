package workflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/tools/sdp-setup/internal/accounts"
	"github.com/stellar/stellar-disbursement-platform-backend/tools/sdp-setup/internal/config"
	"github.com/stellar/stellar-disbursement-platform-backend/tools/sdp-setup/internal/docker"
	"github.com/stellar/stellar-disbursement-platform-backend/tools/sdp-setup/internal/tenant"
	"github.com/stellar/stellar-disbursement-platform-backend/tools/sdp-setup/internal/ui"
)

// SetupPhase represents the different phases of the setup process
type SetupPhase int

const (
	PhaseConfigSelection SetupPhase = iota
	PhaseSetupNaming
	PhaseEnvResolution
	PhaseNetworkSelection
	PhaseTenantModeSelection
	PhaseAccountSetup
	PhaseConfigGeneration
	PhaseLaunch
)

// SetupError represents errors that occur during setup
type SetupError struct {
	Phase   SetupPhase
	Message string
	Cause   error
}

func (e *SetupError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("setup failed at phase %d: %s: %v", e.Phase, e.Message, e.Cause)
	}
	return fmt.Sprintf("setup failed at phase %d: %s", e.Phase, e.Message)
}

func (e *SetupError) Unwrap() error {
	return e.Cause
}

// Options holds all configuration options for the setup process
type Options struct {
	SetupName string // optional name for this setup (affects env file and project)
}

// Execute runs the complete setup workflow
func Execute(ctx context.Context) error {
	// 1. Print Welcome Message
	printWelcome()

	// 2. Handle launching an existing configuration
	if handled, err := handleExistingConfigurationSelection(ctx); err != nil {
		return &SetupError{PhaseConfigSelection, "failed to select configuration", err}
	} else if handled {
		return nil
	}

	// 3. Proceed with new configuration setup
	return handleNewConfigurationSetup(ctx)
}

func handleNewConfigurationSetup(ctx context.Context) error {
	// Phase 1: Setup Naming
	setupName := promptSetupName()

	// Phase 2: Environment Path Resolution
	envPath, err := resolveEnvPath(setupName)
	if err != nil {
		return &SetupError{PhaseEnvResolution, "failed to resolve env path", err}
	}

	// Phase 3: Network Selection
	network := selectNetwork()

	// Phase 4: Tenant Mode Selection
	var singleTenantMode bool
	if network.IsPubnet() {
		fmt.Println("\n⚠️  Pubnet (mainnet) selected: only single-tenant mode is supported for pubnet setups.")
		fmt.Println("   For multi-tenant setups, please directly edit the .env file after setup.")
		singleTenantMode = true
	} else {
		singleTenantMode = selectTenantMode()
	}

	// Phase 5: Distribution and SEP10 Account Setup
	accounts := setupDistributionAndSEP10Accounts(network)

	// Phase 6: Configuration Generation
	cfg := config.NewConfig(config.ConfigOpts{
		EnvPath:          envPath,
		SetupName:        setupName,
		Network:          network,
		SingleTenantMode: singleTenantMode,
		Accounts:         accounts,
	})
	if err = config.Write(cfg, envPath); err != nil {
		return &SetupError{PhaseConfigGeneration, "failed to generate configuration", err}
	}

	// Phase 7: Success Message
	printSuccess(cfg)

	// Phase 8: Launch
	if err = handleLaunch(ctx, setupName, cfg); err != nil {
		return &SetupError{PhaseLaunch, "failed to launch", err}
	}

	return nil
}

// printWelcome displays the welcome message
func printWelcome() {
	fmt.Println("🪄  SDP Setup Wizard")
	fmt.Println("SDP Setup wizard manages your run configurations for the Stellar Disbursement Platform")
	fmt.Println()
}

// handleExistingConfigurationSelection handles the selection of existing configurations in interactive mode.
func handleExistingConfigurationSelection(ctx context.Context) (bool, error) {
	choice, err := chooseRunConfiguration()
	if err != nil {
		return false, fmt.Errorf("choosing run configuration: %w", err)
	}

	if choice.CreateNew {
		return false, nil // Continue with new configuration setup
	}

	// Use existing configuration
	printExistingConfigSummary(choice.SetupName, choice)

	cfg, err := config.Load(choice.EnvPath)
	if err != nil {
		return false, fmt.Errorf("reading existing config: %w", err)
	}

	return true, handleLaunch(ctx, choice.SetupName, cfg)
}

// printExistingConfigSummary prints information about the selected existing configuration
func printExistingConfigSummary(setupName string, choice ConfigurationChoice) {
	fmt.Println("✅ Found existing configuration")
	fmt.Printf("📁 Using .env: %s\n", choice.EnvPath)
	fmt.Printf("🧩 Setup name: %s\n", setupName)
	fmt.Printf("🐳 Docker project: %s\n", config.ComposeProjectName(setupName))
	fmt.Println()
}

// promptSetupName prompts the user to optionally name the setup and sets the name in options.
func promptSetupName() string {
	fmt.Println("You can optionally name this setup (e.g., 'testnet', 'mainnet', 'testnet2').")
	fmt.Println("Press Enter to skip and use defaults.")

	name := strings.TrimSpace(ui.Input("Setup name (optional)", validateSetupName))
	if name == "" {
		fmt.Println("Using default setup (no name).")
		name = "default"
	}
	return name
}

// validateSetupName validates that the setup name contains only lowercase letters,
// numbers, hyphens, and underscores.
func validateSetupName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil // optional
	}

	for _, r := range name {
		if !isValidSetupNameChar(r) {
			return fmt.Errorf("only lowercase letters, numbers, '-', and '_' allowed")
		}
	}
	return nil
}

// isValidSetupNameChar returns true if the rune is valid for a setup name.
func isValidSetupNameChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
}

// resolveEnvPath determines the environment file path and handles existing files
func resolveEnvPath(setupName string) (string, error) {
	envPath, err := config.Resolve(setupName)
	if err != nil {
		return "", fmt.Errorf("resolving env path: %w", err)
	}

	handleExistingEnvFile(envPath)
	return envPath, nil
}

// selectNetwork handles network selection with validation and confirmation
func selectNetwork() utils.NetworkType {
	// Present available options
	networkOptions := []string{"testnet", "pubnet (mainnet)"}

	selected := ui.Select("Select network", networkOptions)

	// Parse the selected option back to NetworkType
	var selectedType utils.NetworkType
	if strings.Contains(selected, string(utils.PubnetNetworkType)) {
		selectedType = utils.PubnetNetworkType
	} else {
		selectedType = utils.NetworkType(selected)
	}

	// Confirm pubnet selection due to real funds risk
	if selectedType.IsPubnet() {
		if !ui.Confirm("Continue with pubnet (mainnet) setup") {
			fmt.Println("Setup cancelled.")
			os.Exit(0)
		}
	}

	return selectedType
}

// selectTenantMode handles tenant mode selection
func selectTenantMode() bool {
	fmt.Println()
	fmt.Println("The SDP can be configured for single-tenant or multi-tenant mode:")
	fmt.Println("• Single-tenant: One organization per deployment (simpler setup)")
	fmt.Println("• Multi-tenant: Multiple organizations per deployment (enterprise setup)")

	modeOptions := []string{"single-tenant", "multi-tenant"}
	selected := ui.Select("Select tenant mode", modeOptions)

	isSingleTenant := selected == "single-tenant"

	if isSingleTenant {
		fmt.Println("✅ Single-tenant mode selected")
		fmt.Println("   This will create one default tenant organization.")
	} else {
		fmt.Println("✅ Multi-tenant mode selected")
		fmt.Printf("   This will enable multiple tenant organizations %v\n", tenant.TestnetTenants)
	}

	return isSingleTenant
}

// printSuccess displays the success message with configuration details
func printSuccess(cfg config.Config) {
	fmt.Println()
	fmt.Println("✅ Setup complete!")
	fmt.Printf("📁 .env file created at: %s\n", cfg.EnvFilePath)
	fmt.Printf("🧩 Setup name: %s\n", cfg.SetupName)
	fmt.Printf("🐳 Docker project: %s\n", cfg.DockerProject)
	fmt.Println()
}

// handleExistingEnvFile manages behavior when an .env file already exists
func handleExistingEnvFile(envPath string) {
	if _, err := os.Stat(envPath); err != nil {
		return // file doesn't exist, proceed
	}

	if !ui.Confirm("⚠️  .env file already exists. Overwrite") {
		fmt.Println("Setup cancelled.")
		os.Exit(0)
	}
}

// ConfigurationChoice represents the user's selection when choosing a run configuration
type ConfigurationChoice struct {
	EnvPath   string // Path to the selected .env file (empty if creating new)
	SetupName string // Name of the setup (e.g., "testnet", "mainnet1", empty for default)
	CreateNew bool   // true if user wants to create new configuration, false if using existing
}

// chooseRunConfiguration presents the user with existing SDP configurations to choose from.
// It scans the dev/ directory for .env files (e.g., .env, .env.testnet, .env.mainnet1)
// and allows the user to either select an existing configuration or create a new one.
//
// Returns:
// - If user selects existing config: EnvPath, SetupName populated, CreateNew=false
// - If user wants new config: EnvPath="", SetupName="", CreateNew=true
func chooseRunConfiguration() (ConfigurationChoice, error) {
	devDir, err := config.FindDevDir()
	if err != nil {
		return ConfigurationChoice{}, fmt.Errorf("finding dev directory: %w", err)
	}

	configs, err := config.ListConfigs(devDir)
	if err != nil {
		return ConfigurationChoice{}, fmt.Errorf("listing configs: %w", err)
	}

	// Build selection menu with existing configurations
	items := buildConfigurationMenu(configs)

	// Present user choice
	choice := ui.Select("Select an existing run configuration or create new", items)

	// Handle "Create new" selection
	if choice == "Create new configuration" || choice == "" {
		return ConfigurationChoice{CreateNew: true}, nil
	}

	// Find and return selected existing configuration
	return findSelectedConfiguration(choice, configs)
}

// buildConfigurationMenu creates the menu items for configuration selection
func buildConfigurationMenu(configs []config.ConfigRef) []string {
	items := []string{"Create new configuration"}
	for _, config := range configs {
		label := config.Name
		if label == "default" {
			label = "default (dev/.env)"
		}
		items = append(items, label)
	}
	return items
}

// findSelectedConfiguration finds the configuration matching the user's selection
func findSelectedConfiguration(choice string, configs []config.ConfigRef) (ConfigurationChoice, error) {
	selectedName := strings.TrimSuffix(strings.TrimSpace(choice), " (dev/.env)")

	for _, config := range configs {
		if isMatchingConfig(config, selectedName) {
			setupName := ""
			if config.Name != "default" {
				setupName = config.Name
			}
			return ConfigurationChoice{
				EnvPath:   config.Path,
				SetupName: setupName,
				CreateNew: false,
			}, nil
		}
	}

	// Fallback to create new if selection doesn't match (shouldn't happen)
	return ConfigurationChoice{CreateNew: true}, nil
}

// isMatchingConfig checks if a config matches the selected name
func isMatchingConfig(config config.ConfigRef, selectedName string) bool {
	return (config.Name == "default" && selectedName == "default") || config.Name == selectedName
}

// promptForDistributionAccount prompts the user to enter existing distributionAccount
func promptForDistributionAccount(network utils.NetworkType) accounts.Info {
	distributionSeed := ui.Input("Distribution Private Key", accounts.ValidateSecret)
	return accounts.Generate(network, distributionSeed)
}

// setupDistributionAndSEP10Accounts handles account setup in interactive mode
func setupDistributionAndSEP10Accounts(network utils.NetworkType) accounts.Info {
	availableChoices := []string{"Enter existing accounts"}
	if network.IsTestnet() {
		availableChoices = append([]string{"Generate new accounts"}, availableChoices...)
	}
	choice := ui.Select("Account setup", availableChoices)

	if choice == "Generate new accounts" {
		return accounts.Generate(network, "")
	}

	return promptForDistributionAccount(network)
}

// handleLaunch manages the launch process for the configured environment
func handleLaunch(ctx context.Context, setupName string, cfg config.Config) error {
	launchLabel := fmt.Sprintf("Launch local environment now (project=%s, setup=%s)", cfg.DockerProject, setupName)
	if !ui.ConfirmWithDefault(launchLabel, ui.ConfirmationDefaultYes) {
		fmt.Println("🐳 Next steps:")
		fmt.Println("   Run `make setup` again and choose to launch when prompted")
		fmt.Println()
		return nil
	}

	if err := docker.Preflight(); err != nil {
		fmt.Printf("❌ Preflight failed: %v\n", err)
		fmt.Println("   Fix the above issues and run `make setup` again to launch")
		return nil
	}

	dockerSvc, err := docker.NewService(cfg)
	if err != nil {
		return fmt.Errorf("failed to create docker service: %w", err)
	}

	if err = dockerSvc.StartDockerStack(ctx); err != nil {
		if errors.Is(err, docker.ErrUserDeclined) {
			return nil // User declined gracefully, don't treat as error
		}
		return fmt.Errorf("launching dev environment: %w", err)
	}

	// Ask user if they want to perform initialization
	if ui.Confirm("Initialize tenants and users") {
		tenantSvc := tenant.NewService(cfg)
		if err = tenantSvc.InitializeEnvironment(ctx); err != nil {
			fmt.Printf("⚠️  Environment initialization failed: %v\n", err)
			fmt.Println("💡 You can run the setup again later to retry initialization")
		}
	} else {
		fmt.Println("⏭️  Skipping tenant and user initialization")
	}

	return nil
}
