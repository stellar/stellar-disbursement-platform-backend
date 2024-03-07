package main

import (
	"github.com/sirupsen/logrus"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/cli"
)

// Version is the official version of this application.
const Version = "0.1.0"

// GitCommit is populated at build time by
// go build -ldflags "-X main.GitCommit=$GIT_COMMIT"
var GitCommit string

func main() {
	log.DefaultLogger = log.New()
	log.DefaultLogger.SetLevel(logrus.TraceLevel)

	cmd := cli.SetupCLI(Version, GitCommit)
	if err := cmd.Execute(); err != nil {
		log.Ctx(cmd.Context()).Fatalf("error executing: %s", err.Error())
	}
}
