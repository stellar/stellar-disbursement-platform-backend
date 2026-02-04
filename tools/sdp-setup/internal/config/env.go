// Package config handles environment file generation and management for SDP configurations
package config

import (
	"fmt"
	"net/url"
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
	DefaultHTTPSPort = "3443"
	DefaultHTTPPort  = "3000"
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

	// Frontend HTTPS settings
	UseHTTPS         bool
	FrontendProtocol string // http or https
	FrontendPort     string // exposed port for the UI (3000/3443)

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
			// skip the example file.
			if suffix == "example" {
				continue
			}
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
	EnableHTTPS      bool
}

// NewConfig creates a new Config
func NewConfig(opts ConfigOpts) Config {
	cfg := fromAccounts(opts.Network, opts.Accounts)
	cfg.SetupName = opts.SetupName
	cfg.SingleTenantMode = opts.SingleTenantMode
	cfg.EnvFilePath = opts.EnvPath
	cfg.DockerProject = ComposeProjectName(opts.SetupName)
	if opts.EnableHTTPS {
		cfg.EnableHTTPS()
	}
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

		// Frontend defaults
		UseHTTPS:         false,
		FrontendProtocol: "http",
		FrontendPort:     DefaultHTTPPort,
	}

	// Parse SINGLE_TENANT_MODE
	if singleTenantModeStr := envMap["SINGLE_TENANT_MODE"]; singleTenantModeStr != "" {
		cfg.SingleTenantMode, err = strconv.ParseBool(singleTenantModeStr)
		if err != nil {
			return Config{}, fmt.Errorf("parsing SINGLE_TENANT_MODE: %w", err)
		}
	}

	// Parse USE_HTTPS (optional)
	if useHTTPS := strings.TrimSpace(envMap["USE_HTTPS"]); useHTTPS != "" {
		if cfg.UseHTTPS, err = strconv.ParseBool(useHTTPS); err != nil {
			return Config{}, fmt.Errorf("parsing USE_HTTPS: %w", err)
		}
	}

	// Parse SDP_UI_BASE_URL (optional)
	if base := strings.TrimSpace(envMap["SDP_UI_BASE_URL"]); base != "" {
		if u, parseErr := url.Parse(base); parseErr == nil && u.Scheme != "" {
			cfg.FrontendProtocol = u.Scheme
			if port := u.Port(); port != "" {
				cfg.FrontendPort = port
			}
		}
	}

	if cfg.UseHTTPS {
		cfg.EnableHTTPS()
	} else {
		cfg.DisableHTTPS()
	}

	return cfg, nil
}

// Write writes the environment configuration to a file using godotenv
func Write(cfg Config, path string) error {
	envConfigDir := filepath.Dir(path)
	if err := os.MkdirAll(envConfigDir, 0o755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", path, err)
	}

	singleTenantModeStr := "true"
	if !cfg.SingleTenantMode {
		singleTenantModeStr = "false"
	}

	// 1. Start with the configuration values we want to set
	apiHost := "localhost"
	uiHost := "localhost"
	if !cfg.SingleTenantMode {
		apiHost = "stellar.local"
		uiHost = "stellar.local"
	}

	configMap := map[string]string{
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
		"USE_HTTPS":                                  strconv.FormatBool(cfg.UseHTTPS),
		"SDP_UI_BASE_URL":                            cfg.FrontendBaseURL(uiHost),
		"BASE_URL":                                   fmt.Sprintf("http://%s:8000", apiHost),
		"DATABASE_URL":                               fmt.Sprintf("postgres://postgres@db:5432/%s?sslmode=disable", cfg.DatabaseName),
		"EMBEDDED_WALLETS_WASM_HASH":                 "9b784817dff1620a3e2b223fe1eb8dac56e18980dea9726f692847ccbbd3a853",
	}

	switch cfg.NetworkType {
	case "pubnet":
		configMap["TENANT_XLM_BOOTSTRAP_AMOUNT"] = "1"
		configMap["NUM_CHANNEL_ACCOUNTS"] = "1"
		configMap["SEP45_CONTRACT_ID"] = "CALI6JC3MSNDGFRP7Z2OKUEPREHOJRRXKMJEWQDEFZPFGXALA45RAUTH"
		configMap["RPC_URL"] = "https://rpc.lightsail.network"
	case "testnet":
		configMap["SEP45_CONTRACT_ID"] = "CDY4CS2VWHAZOMYVTKUFKGNZKIVFBCXUFNFQ5KSXOTAHKL5H5ZRTAUTH"
		configMap["RPC_URL"] = "https://soroban-testnet.stellar.org"
	}

	// 2. Load .env.example to use as a base
	examplePath := filepath.Join(envConfigDir, ".env.example")

	finalMap := make(map[string]string)

	if exampleMap, err := godotenv.Read(examplePath); err == nil {
		finalMap = exampleMap
	} else {
		fmt.Printf("Note: Could not load %s, generating minimal config\n", examplePath)
	}

	// 3. Override with our configuration values
	for k, v := range configMap {
		finalMap[k] = v
	}

	if err := godotenv.Write(finalMap, path); err != nil {
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

		UseHTTPS:         false,
		FrontendProtocol: "http",
		FrontendPort:     DefaultHTTPPort,
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

// EnableHTTPS toggles HTTPS related defaults on the config.
func (cfg *Config) EnableHTTPS() {
	cfg.UseHTTPS = true
	cfg.FrontendProtocol = "https"
	if cfg.FrontendPort == "" || cfg.FrontendPort == DefaultHTTPPort {
		cfg.FrontendPort = DefaultHTTPSPort
	}
}

// DisableHTTPS forces HTTP defaults on the config.
func (cfg *Config) DisableHTTPS() {
	cfg.UseHTTPS = false
	cfg.FrontendProtocol = "http"
	cfg.FrontendPort = DefaultHTTPPort
}

// FrontendBaseURL builds a UI base URL for the provided host (e.g., localhost or bluecorp.stellar.local)
func (cfg Config) FrontendBaseURL(host string) string {
	protocol := cfg.FrontendProtocol
	if protocol == "" {
		protocol = "http"
	}
	port := cfg.FrontendPort
	if port == "" {
		port = DefaultHTTPPort
	}
	return fmt.Sprintf("%s://%s:%s", protocol, host, port)
}
