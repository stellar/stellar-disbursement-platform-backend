// Package config handles environment file generation and management for SDP configurations
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/tools/sdp-setup/internal/accounts"
)

// Default configuration values
const (
	DefaultProject   = "sdp"
	DefaultAdminUser = "SDP-admin"
	DefaultAdminKey  = "api_key_1234567890"
	DefaultAdminURL  = "http://localhost:8003"
	DefaultWorkDir   = "dev"
)

// Config represents the environment configuration for SDP
type Config struct {
	NetworkType        string // testnet or mainnet
	NetworkPassphrase  string // Stellar network passphrase
	HorizonURL         string // Horizon API endpoint
	DatabaseName       string // Database name for this configuration
	DisableMFA         string // Whether to disable MFA (string for env file)
	SEP10PublicKey     string // SEP-10 authentication public key
	SEP10PrivateKey    string // SEP-10 authentication private key
	DistributionPublic string // Distribution account public key
	DistributionSeed   string // Distribution account secret seed
	SetupName          string // Optional setup name for multi-config support
	SingleTenantMode   bool   // Whether to use single-tenant mode

	// Following config doesn't get written to env file
	WorkDir       string
	EnvFilePath   string
	DockerProject string
	AdminUser     string
	AdminKey      string
	AdminURL      string
}

// Resolve determines the environment file path based on setup name
func Resolve(setupName string) (string, error) {
	// Determine base path under dev
	if st, err := os.Stat("dev"); err == nil && st.IsDir() {
		name := ".env"
		if setupName != "" {
			name = ".env." + setupName
		}
		return filepath.Join("dev", name), nil
	}
	if st, err := os.Stat(filepath.Clean("../../dev")); err == nil && st.IsDir() {
		name := ".env"
		if setupName != "" {
			name = ".env." + setupName
		}
		return filepath.Clean(filepath.Join("../../dev", name)), nil
	}
	return "", fmt.Errorf("could not locate dev directory; run from repo root or provide -env path")
}

// FindDevDir returns the absolute path to the dev directory used by compose.
func FindDevDir() (string, error) {
	if st, err := os.Stat("dev"); err == nil && st.IsDir() {
		p, err := filepath.Abs("dev")
		if err != nil {
			return "", fmt.Errorf("getting absolute path for dev: %w", err)
		}
		return p, nil
	}
	p := filepath.Clean("../../dev")
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		ap, err := filepath.Abs(p)
		if err != nil {
			return "", fmt.Errorf("getting absolute path for %s: %w", p, err)
		}
		return ap, nil
	}
	return "", fmt.Errorf("could not locate dev directory")
}

type ConfigRef struct {
	Name string // "default" or suffix after .env.
	Path string // absolute path
}

// ListConfigs scans the dev directory for .env files and returns run configurations.
func ListConfigs(devDir string) ([]ConfigRef, error) {
	entries, err := os.ReadDir(devDir)
	if err != nil {
		return nil, fmt.Errorf("reading dev directory: %w", err)
	}
	var out []ConfigRef
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == ".env" {
			out = append(out, ConfigRef{Name: "default", Path: filepath.Join(devDir, name)})
			continue
		}
		if strings.HasPrefix(name, ".env.") {
			suffix := strings.TrimPrefix(name, ".env.")
			out = append(out, ConfigRef{Name: suffix, Path: filepath.Join(devDir, name)})
		}
	}
	return out, nil
}

type ConfigOpts struct {
	EnvPath          string
	SetupName        string
	Network          utils.NetworkType
	SingleTenantMode bool
	Accounts         accounts.Info
}

// NewConfig creates a new Config
func NewConfig(opts ConfigOpts) Config {
	cfg := fromAccounts(opts.Network, opts.Accounts)
	cfg.SetupName = opts.SetupName
	cfg.SingleTenantMode = opts.SingleTenantMode
	cfg.EnvFilePath = opts.EnvPath
	cfg.DockerProject = ComposeProjectName(opts.SetupName)
	return cfg
}

// Load loads environment configuration from a file and returns a Config
func Load(path string) (Config, error) {
	envMap, err := godotenv.Read(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading env file %s: %w", path, err)
	}

	cfg := Config{
		NetworkType:        envMap["NETWORK_TYPE"],
		NetworkPassphrase:  envMap["NETWORK_PASSPHRASE"],
		HorizonURL:         envMap["HORIZON_URL"],
		DatabaseName:       envMap["DATABASE_NAME"],
		DisableMFA:         envMap["DISABLE_MFA"],
		SEP10PublicKey:     envMap["SEP10_SIGNING_PUBLIC_KEY"],
		SEP10PrivateKey:    envMap["SEP10_SIGNING_PRIVATE_KEY"],
		DistributionPublic: envMap["DISTRIBUTION_PUBLIC_KEY"],
		DistributionSeed:   envMap["DISTRIBUTION_SEED"],
		SetupName:          envMap["SETUP_NAME"],

		// Non-env file fields with defaults
		EnvFilePath:   path,
		DockerProject: ComposeProjectName(strings.TrimSpace(envMap["SETUP_NAME"])),
		AdminUser:     DefaultAdminUser,
		AdminKey:      DefaultAdminKey,
		AdminURL:      DefaultAdminURL,
		WorkDir:       DefaultWorkDir,
	}

	// Parse SINGLE_TENANT_MODE
	if singleTenantModeStr := envMap["SINGLE_TENANT_MODE"]; singleTenantModeStr != "" {
		cfg.SingleTenantMode, err = strconv.ParseBool(singleTenantModeStr)
		if err != nil {
			return Config{}, fmt.Errorf("parsing SINGLE_TENANT_MODE: %w", err)
		}
	}

	return cfg, nil
}

// Write writes the environment configuration to a file using godotenv
func Write(cfg Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", path, err)
	}

	singleTenantModeStr := "true"
	if !cfg.SingleTenantMode {
		singleTenantModeStr = "false"
	}

	envMap := map[string]string{
		"NETWORK_TYPE":                               cfg.NetworkType,
		"NETWORK_PASSPHRASE":                         cfg.NetworkPassphrase,
		"HORIZON_URL":                                cfg.HorizonURL,
		"DATABASE_NAME":                              cfg.DatabaseName,
		"DISABLE_MFA":                                cfg.DisableMFA,
		"SINGLE_TENANT_MODE":                         singleTenantModeStr,
		"SETUP_NAME":                                 cfg.SetupName,
		"SEP10_SIGNING_PUBLIC_KEY":                   cfg.SEP10PublicKey,
		"SEP10_SIGNING_PRIVATE_KEY":                  cfg.SEP10PrivateKey,
		"DISTRIBUTION_PUBLIC_KEY":                    cfg.DistributionPublic,
		"DISTRIBUTION_SEED":                          cfg.DistributionSeed,
		"CHANNEL_ACCOUNT_ENCRYPTION_PASSPHRASE":      cfg.DistributionSeed,
		"DISTRIBUTION_ACCOUNT_ENCRYPTION_PASSPHRASE": cfg.DistributionSeed,
	}

	if cfg.NetworkType == "pubnet" {
		envMap["TENANT_XLM_BOOTSTRAP_AMOUNT"] = "1"
		envMap["NUM_CHANNEL_ACCOUNTS"] = "1"
	}

	if err := godotenv.Write(envMap, path); err != nil {
		return fmt.Errorf("writing env file %s: %w", path, err)
	}

	return nil
}

// fromAccounts creates a configuration from network type and account information
func fromAccounts(networkType utils.NetworkType, acc accounts.Info) Config {
	cfg := Config{
		NetworkType:        string(networkType),
		SEP10PublicKey:     acc.SEP10Public,
		SEP10PrivateKey:    acc.SEP10Private,
		DistributionPublic: acc.DistributionPublic,
		DistributionSeed:   acc.DistributionSeed,

		AdminUser: DefaultAdminUser,
		AdminKey:  DefaultAdminKey,
		AdminURL:  DefaultAdminURL,
		WorkDir:   DefaultWorkDir,
	}

	switch networkType {
	case utils.PubnetNetworkType:
		cfg.NetworkPassphrase = "Public Global Stellar Network ; September 2015"
		cfg.HorizonURL = "https://horizon.stellar.org"
		cfg.DatabaseName = "sdp_pubnet"
		cfg.DisableMFA = "false"
	case utils.TestnetNetworkType:
		cfg.NetworkPassphrase = "Test SDF Network ; September 2015"
		cfg.HorizonURL = "https://horizon-testnet.stellar.org"
		cfg.DatabaseName = "sdp_mtn"
		cfg.DisableMFA = "true"
	default:
		// Default to testnet for unknown networks
		cfg.NetworkPassphrase = "Test SDF Network ; September 2015"
		cfg.HorizonURL = "https://horizon-testnet.stellar.org"
		cfg.DatabaseName = "sdp_mtn"
		cfg.DisableMFA = "true"
	}

	return cfg
}

// ComposeProjectName returns the docker compose project name using a base and optional setup name.
func ComposeProjectName(setupName string) string {
	if setupName == "" || setupName == "default" {
		return DefaultProject
	}
	return fmt.Sprintf("%s-%s", DefaultProject, setupName)
}
