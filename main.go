package main

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/cmd"
	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
)

// Version is the official version of this application. Whenever it's changed
// here, it also needs to be updated at the `helmchart/Chart.yaml#appVersion“.
const Version = "0.0.0-rc.0"

// GitCommit is populated at build time by
// go build -ldflags "-X main.GitCommit=$GIT_COMMIT"
var GitCommit string

func main() {
	if err := cmdUtils.LoadEnvFile(); err != nil {
		fmt.Fprintf(os.Stderr, "Error loading environment file: %v\n", err)
		os.Exit(1)
	}

	preConfigureLogger()
	log.Info("Version: ", Version)
	log.Info("GitCommit: ", GitCommit)

	rootCmd := cmd.SetupCLI(Version, GitCommit)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// preConfigureLogger will set the log level to Trace, so logs works from the
// start. This will eventually be overwritten in cmd/root.go
func preConfigureLogger() {
	log.DefaultLogger = log.New()
	log.DefaultLogger.SetLevel(logrus.TraceLevel)
}
