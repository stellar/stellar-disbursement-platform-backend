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
	BaseURL           string
	NetworkPassphrase string
}

// populateConfigOptions populates the CrastTrackerOptions from the global options.
func (g GlobalOptionsType) PopulateCrashTrackerOptions(crashTrackerOptions *crashtracker.CrashTrackerOptions) {
	if crashTrackerOptions.CrashTrackerType == crashtracker.CrashTrackerTypeSentry {
		crashTrackerOptions.SentryDSN = g.SentryDSN
	}
	crashTrackerOptions.Environment = g.Environment
	crashTrackerOptions.GitCommit = g.GitCommit
}
