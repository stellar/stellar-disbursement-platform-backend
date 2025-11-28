package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/stellar/stellar-disbursement-platform-backend/tools/sdp-setup/internal/config"
)

type CommandExecutor interface {
	Execute(ctx context.Context, name string, args ...string) error
}

type DefaultExecutor struct {
	cfg config.Config
}

func (e *DefaultExecutor) Execute(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = e.cfg.WorkDir
	cmd.Env = append(os.Environ(), fmt.Sprintf("COMPOSE_PROJECT_NAME=%s", e.cfg.DockerProject))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("executing command '%s %v': %w", name, args, err)
	}
	return nil
}
