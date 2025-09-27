package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/tools/sdp-setup/internal/accounts"
	"github.com/stellar/stellar-disbursement-platform-backend/tools/sdp-setup/internal/docker"
	"github.com/stellar/stellar-disbursement-platform-backend/tools/sdp-setup/internal/envfile"
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

// Execute runs the setup command
func Execute() error {
	var opts Options
	root := newRootCommand(&opts)
	if err := root.Execute(); err != nil {
		return fmt.Errorf("executing command: %w", err)
	}
	return nil
}

func newRootCommand(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sdp-setup",
		Short: "SDP Setup wizard",
		Long:  "SDP Setup wizard manages your run configurations for the Stellar Disbursement Platform",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetup(opts)
		},
	}

	return cmd
}

// runSetup orchestrates the entire setup process
func runSetup(opts *Options) error {
	workflow := NewSetupWorkflow(opts)
	return workflow.Execute()
}

// Execute runs the complete setup workflow
func (sw *SetupWorkflow) Execute() error {
	sw.printWelcome()

	// Phase 1: Configuration Selection
	if handled, err := sw.handleExistingConfigurationSelection(); err != nil {
		return &SetupError{PhaseConfigSelection, "failed to select configuration", err}
	} else if handled {
		return nil // User selected existing config and launched
	}

	// Phase 2: Setup Naming
	sw.promptSetupName()

	// Phase 3: Environment Path Resolution
	envPath, err := sw.resolveEnvPath()
	if err != nil {
		return &SetupError{PhaseEnvResolution, "failed to resolve env path", err}
	}

	// Phase 4: Network Selection
	network := sw.selectNetwork()

	// Phase 5: Tenant Mode Selection
	singleTenantMode := sw.selectTenantMode()

	// Phase 6: Account Setup
	accounts := sw.setupAccounts(network)

	// Phase 7: Configuration Generation
	if err = sw.generateConfiguration(envPath, network, singleTenantMode, accounts); err != nil {
		return &SetupError{PhaseConfigGeneration, "failed to generate configuration", err}
	}

	sw.printSuccess(envPath)

	// Phase 8: Launch
	if err = sw.handleLaunch(envPath); err != nil {
		return &SetupError{PhaseLaunch, "failed to launch", err}
	}

	return nil
}

// printWelcome displays the welcome message
func (sw *SetupWorkflow) printWelcome() {
	fmt.Println("🪄  SDP Setup Wizard")
	fmt.Println("SDP Setup wizard manages your run configurations for the Stellar Disbursement Platform")
	fmt.Println()
}

// handleExistingConfigurationSelection handles the selection of existing configurations in interactive mode
func (sw *SetupWorkflow) handleExistingConfigurationSelection() (bool, error) {
	choice, err := chooseRunConfiguration()
	if err != nil {
		return false, err
	}

	if choice.CreateNew {
		return false, nil // Continue with new configuration setup
	}

	// Use existing configuration
	sw.opts.SetupName = choice.SetupName
	sw.printExistingConfigSummary(choice)

	return true, sw.handleLaunch(choice.EnvPath)
}

// printExistingConfigSummary prints information about the selected existing configuration
func (sw *SetupWorkflow) printExistingConfigSummary(choice ConfigurationChoice) {
	fmt.Println("✅ Found existing configuration")
	fmt.Printf("📁 Using .env: %s\n", choice.EnvPath)
	fmt.Printf("🧩 Setup name: %s\n", sw.opts.SetupName)
	fmt.Printf("🐳 Docker project: %s\n", envfile.ComposeProject("sdp", sw.opts.SetupName))
	fmt.Println()
}

// promptSetupName prompts the user to optionally name the setup and sets the name in options.
func (sw *SetupWorkflow) promptSetupName() {
	fmt.Println("You can optionally name this setup (e.g., 'testnet', 'mainnet', 'testnet2').")
	fmt.Println("Press Enter to skip and use defaults.")

	name := strings.TrimSpace(ui.Input("Setup name (optional)", validateSetupName))
	if name == "" {
		fmt.Println("Using default setup (no name).")
		name = "default"
	}
	sw.opts.SetupName = name
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
func (sw *SetupWorkflow) resolveEnvPath() (string, error) {
	envPath, err := envfile.Resolve(sw.opts.SetupName)
	if err != nil {
		return "", fmt.Errorf("resolving env path: %w", err)
	}

	handleExistingEnvFile(envPath)
	return envPath, nil
}

// selectNetwork handles network selection with validation and confirmation
func (sw *SetupWorkflow) selectNetwork() utils.NetworkType {
	// Present available options
	networkOptions := []string{"testnet", "pubnet (mainnet)"}

	selected := ui.Select("Select network", networkOptions)

	// Parse the selected option back to NetworkType
	var selectedType utils.NetworkType
	if strings.Contains(selected, "pubnet") {
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
func (sw *SetupWorkflow) selectTenantMode() bool {
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
		fmt.Println("   This will enable multiple tenant organizations (redcorp, bluecorp, pinkcorp).")
	}

	return isSingleTenant
}

// setupAccounts handles account setup based on the workflow options
func (sw *SetupWorkflow) setupAccounts(network utils.NetworkType) accounts.Info {
	return setupAccountsInteractive(network)
}

// generateConfiguration creates and writes the environment configuration
func (sw *SetupWorkflow) generateConfiguration(envPath string, network utils.NetworkType, singleTenantMode bool, acc accounts.Info) error {
	cfg := envfile.FromAccounts(network, acc)
	cfg.SetupName = sw.opts.SetupName
	cfg.SingleTenantMode = singleTenantMode

	if err := envfile.Write(cfg, envPath); err != nil {
		return fmt.Errorf("writing .env file: %w", err)
	}

	return nil
}

// printSuccess displays the success message with configuration details
func (sw *SetupWorkflow) printSuccess(envPath string) {
	fmt.Println()
	fmt.Println("✅ Setup complete!")
	fmt.Printf("📁 .env file created at: %s\n", envPath)
	fmt.Printf("🧩 Setup name: %s\n", sw.opts.SetupName)
	fmt.Printf("🐳 Docker project: %s\n", envfile.ComposeProject("sdp", sw.opts.SetupName))
	fmt.Printf("   Tip: dev commands can use --env-file %s\n", envPath)
	fmt.Println()
}

// handleLaunch manages the launch process
func (sw *SetupWorkflow) handleLaunch(envPath string) error {
	return handleLaunch(sw.opts, envPath)
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

// SetupWorkflow orchestrates the entire setup process
type SetupWorkflow struct {
	opts *Options
}

// NewSetupWorkflow creates a new setup workflow with the given options
func NewSetupWorkflow(opts *Options) *SetupWorkflow {
	return &SetupWorkflow{opts: opts}
}

// chooseRunConfiguration presents the user with existing SDP configurations to choose from.
// It scans the dev/ directory for .env files (e.g., .env, .env.testnet, .env.mainnet1)
// and allows the user to either select an existing configuration or create a new one.
//
// Returns:
// - If user selects existing config: EnvPath, SetupName populated, CreateNew=false
// - If user wants new config: EnvPath="", SetupName="", CreateNew=true
func chooseRunConfiguration() (ConfigurationChoice, error) {
	devDir, err := envfile.FindDevDir()
	if err != nil {
		return ConfigurationChoice{}, fmt.Errorf("finding dev directory: %w", err)
	}

	configs, err := envfile.ListConfigs(devDir)
	if err != nil {
		return ConfigurationChoice{}, fmt.Errorf("listing configs: %w", err)
	}

	// Build selection menu with existing configurations
	items := buildConfigurationMenu(configs)

	// Present user choice
	choice := ui.Select("Select a run configuration or create new", items)

	// Handle "Create new" selection
	if choice == "Create new configuration" || choice == "" {
		return ConfigurationChoice{CreateNew: true}, nil
	}

	// Find and return selected existing configuration
	return findSelectedConfiguration(choice, configs)
}

// buildConfigurationMenu creates the menu items for configuration selection
func buildConfigurationMenu(configs []envfile.ConfigRef) []string {
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
func findSelectedConfiguration(choice string, configs []envfile.ConfigRef) (ConfigurationChoice, error) {
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
func isMatchingConfig(config envfile.ConfigRef, selectedName string) bool {
	return (config.Name == "default" && selectedName == "default") || config.Name == selectedName
}

// promptForDistributionAccount prompts the user to enter existing distributionAccount
func promptForDistributionAccount(network utils.NetworkType) accounts.Info {
	distributionSeed := ui.Input("Distribution Private Key", accounts.ValidateSecret)
	return accounts.Generate(network, distributionSeed)
}

// setupAccountsInteractive handles account setup in interactive mode
func setupAccountsInteractive(network utils.NetworkType) accounts.Info {
	choice := ui.Select("Account setup", []string{"Generate new accounts", "Enter existing accounts"})

	if choice == "Generate new accounts" {
		return accounts.Generate(network, "")
	}

	return promptForDistributionAccount(network)
}

// handleLaunch manages the launch process for the configured environment
func handleLaunch(opts *Options, envPath string) error {
	project := envfile.ComposeProject("sdp", strings.TrimSpace(opts.SetupName))
	if !ui.Confirm(fmt.Sprintf("Launch local environment now (project=%s, setup=%s)", project, opts.SetupName)) {
		fmt.Println("🐳 Next steps:")
		fmt.Println("   Run ./setup again and choose to launch when prompted")
		fmt.Println()
		return nil
	}

	if err := docker.Preflight(); err != nil {
		fmt.Printf("❌ Preflight failed: %v\n", err)
		fmt.Println("   Fix the above issues and run ./setup again to launch")
		return nil
	}

	config := docker.DefaultConfig()
	config.EnvFile = envPath
	config.Project = project

	dockerSvc, err := docker.NewService(config)
	if err != nil {
		return fmt.Errorf("failed to create docker service: %w", err)
	}

	if err = dockerSvc.StartDockerStack(); err != nil {
		if errors.Is(err, docker.ErrUserDeclined) {
			return nil // User declined gracefully, don't treat as error
		}
		return fmt.Errorf("launching dev environment: %w", err)
	}

	return nil
}
