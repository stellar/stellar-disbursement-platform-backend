package crashtracker

import (
	"context"
	"fmt"
	"strings"

	"github.com/stellar/go/support/log"
)

type CrashTrackerType string

const (
	// CrashTrackerTypeSentry is used to monitor errors with sentry.
	CrashTrackerTypeSentry CrashTrackerType = "SENTRY"
	// CrashTrackerTypeDryRun is used for development environment
	CrashTrackerTypeDryRun CrashTrackerType = "DRY_RUN"
)

func ParseCrashTrackerType(messengerTypeStr string) (CrashTrackerType, error) {
	crashTrackerTypeStrUpper := strings.ToUpper(messengerTypeStr)
	ctType := CrashTrackerType(crashTrackerTypeStrUpper)

	switch ctType {
	case CrashTrackerTypeSentry, CrashTrackerTypeDryRun:
		return ctType, nil
	default:
		return "", fmt.Errorf("invalid crash tracker type %q", crashTrackerTypeStrUpper)
	}
}

type CrashTrackerOptions struct {
	CrashTrackerType CrashTrackerType
	Environment      string
	GitCommit        string

	// Sentry variables
	SentryDSN string
}

func GetClient(ctx context.Context, opts CrashTrackerOptions) (CrashTrackerClient, error) {
	switch opts.CrashTrackerType {
	case CrashTrackerTypeSentry:
		log.Ctx(ctx).Infof("Using %q crash tracker", opts.CrashTrackerType)
		return NewSentryClient(opts.SentryDSN, opts.Environment, opts.GitCommit)
	case CrashTrackerTypeDryRun:
		log.Ctx(ctx).Warnf("Using %q crash tracker", opts.CrashTrackerType)
		return NewDryRunClient()

	default:
		return nil, fmt.Errorf("unknown crash tracker type: %q", opts.CrashTrackerType)
	}
}
