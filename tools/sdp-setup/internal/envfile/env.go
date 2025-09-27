// Package envfile handles environment file generation and management for SDP configurations
package envfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/tools/sdp-setup/internal/accounts"
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

// FromAccounts creates a configuration from network type and account information
func FromAccounts(networkType utils.NetworkType, acc accounts.Info) Config {
	cfg := Config{
		NetworkType:        string(networkType),
		SEP10PublicKey:     acc.SEP10Public,
		SEP10PrivateKey:    acc.SEP10Private,
		DistributionPublic: acc.DistributionPublic,
		DistributionSeed:   acc.DistributionSeed,
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

func Write(cfg Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", path, err)
	}

	singleTenantModeStr := "true"
	if !cfg.SingleTenantMode {
		singleTenantModeStr = "false"
	}

	content := fmt.Sprintf("NETWORK_TYPE=%s\nNETWORK_PASSPHRASE=%s\nHORIZON_URL=%s\nDATABASE_NAME=%s\nDISABLE_MFA=%s\nSINGLE_TENANT_MODE=%s\nSETUP_NAME=%s\nSEP10_SIGNING_PUBLIC_KEY=%s\nSEP10_SIGNING_PRIVATE_KEY=%s\nDISTRIBUTION_PUBLIC_KEY=%s\nDISTRIBUTION_SEED=%s\nCHANNEL_ACCOUNT_ENCRYPTION_PASSPHRASE=%s\nDISTRIBUTION_ACCOUNT_ENCRYPTION_PASSPHRASE=%s\n",
		cfg.NetworkType, cfg.NetworkPassphrase, cfg.HorizonURL, cfg.DatabaseName, cfg.DisableMFA, singleTenantModeStr,
		cfg.SetupName,
		cfg.SEP10PublicKey, cfg.SEP10PrivateKey, cfg.DistributionPublic, cfg.DistributionSeed,
		cfg.DistributionSeed, cfg.DistributionSeed,
	)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing file %s: %w", path, err)
	}
	return nil
}

// ComposeProject returns the docker compose project name using a base and optional setup name.
func ComposeProject(base, setupName string) string {
	if setupName == "" || setupName == "default" {
		return base
	}
	return fmt.Sprintf("%s-%s", base, setupName)
}
