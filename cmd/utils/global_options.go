package utils

import (
	"github.com/sirupsen/logrus"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
)

type GlobalOptionsType struct {
	LogLevel          logrus.Level
	SentryDSN         string
	Environment       string
	Version           string
	GitCommit         string
	DatabaseURL       string
	DBPool            DBPoolOptions
	BaseURL           string
	SDPUIBaseURL      string
	NetworkPassphrase string
	EnvFile           string
	// LogShippingURL is the Loki-compatible push endpoint (e.g. a Grafana
	// Alloy loki.source.api receiver) that structured logs are shipped to
	// directly over HTTP, bypassing stdout capture. Empty (the default)
	// disables log shipping entirely -- local dev and any deployment that
	// doesn't set this env var are completely unaffected.
	LogShippingURL string
}

// PopulateCrashTrackerOptions populates the CrastTrackerOptions from the global options.
func (g GlobalOptionsType) PopulateCrashTrackerOptions(crashTrackerOptions *crashtracker.CrashTrackerOptions) {
	if crashTrackerOptions.CrashTrackerType == crashtracker.CrashTrackerTypeSentry {
		crashTrackerOptions.SentryDSN = g.SentryDSN
	}
	crashTrackerOptions.Environment = g.Environment
	crashTrackerOptions.GitCommit = g.GitCommit
}
