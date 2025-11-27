package docker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/tools/sdp-setup/internal/config"
	"github.com/stellar/stellar-disbursement-platform-backend/tools/sdp-setup/internal/ui"
)

// ErrUserDeclined is returned when the user chooses not to proceed with an operation
var ErrUserDeclined = errors.New("user declined to proceed")

// Service provides Docker Compose operations for SDP environment.
type Service struct {
	cfg      config.Config
	executor CommandExecutor
	files    []string
}

// NewService creates a new Docker service.
func NewService(cfg config.Config) (*Service, error) {
	if cfg.WorkDir != "" {
		if abs, err := filepath.Abs(cfg.WorkDir); err == nil {
			cfg.WorkDir = abs
		}
	}
	if cfg.EnvFilePath != "" {
		if abs, err := filepath.Abs(cfg.EnvFilePath); err == nil {
			cfg.EnvFilePath = abs
		}
	}

	files := []string{"docker-compose.yml"}
	if cfg.UseHTTPS {
		files = append(files, "docker-compose-https-frontend.yml")
	}

	return &Service{
		cfg: cfg,
		executor: &DefaultExecutor{
			cfg: cfg,
		},
		files: files,
	}, nil
}

// Preflight checks for required tools and running docker daemon.
func Preflight() error {
	for _, bin := range []string{"docker", "curl", "go"} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("required tool '%s' not found in PATH", bin)
		}
	}
	if err := exec.Command("docker", "ps").Run(); err != nil {
		return fmt.Errorf("docker not running: %w", err)
	}
	if err := exec.Command("docker", "compose", "version").Run(); err != nil {
		return fmt.Errorf("docker compose missing: %w", err)
	}
	return nil
}

// StartDockerStack starts the Docker stack and initializes tenants/users.
func (s *Service) StartDockerStack(ctx context.Context) error {
	if err := Preflight(); err != nil {
		return err
	}

	fmt.Printf("Using network: %s (project=%s)\n", s.cfg.NetworkType, s.cfg.DockerProject)

	// Prompt to stop other running SDP projects (prevents port conflicts)
	if err := s.ensureExclusiveProject(ctx, s.cfg.DockerProject); err != nil {
		return err
	}

	if s.cfg.UseHTTPS {
		if err := s.prepareHTTPS(); err != nil {
			return err
		}
	}

	if err := s.composeDown(ctx); err != nil {
		return err
	}

	if err := s.composeUp(ctx); err != nil {
		return err
	}
	return nil
}

// prepareHTTPS validates that HTTPS certs already exist.
func (s *Service) prepareHTTPS() error {
	certsDir := filepath.Join(s.cfg.WorkDir, "certs")
	certPath := filepath.Join(certsDir, "stellar.local.pem")
	keyPath := filepath.Join(certsDir, "stellar.local-key.pem")

	if _, err := os.Stat(certPath); err == nil {
		if _, err := os.Stat(keyPath); err == nil {
			fmt.Println("Using TLS certs from dev/certs (HTTPS enabled)")
			return nil
		}
	}

	return fmt.Errorf("HTTPS selected but TLS certs are missing; expected dev/certs/stellar.local.pem and dev/certs/stellar.local-key.pem (generate with mkcert; see dev/README.md)")
}

// composeDown runs 'docker compose down' with the given config.
func (s *Service) composeDown(ctx context.Context) error {
	fmt.Println("====> docker compose down")

	args := []string{"compose", "-p", s.cfg.DockerProject}
	args = append(args, "--env-file", s.cfg.EnvFilePath)
	for _, file := range s.files {
		args = append(args, "-f", file)
	}
	args = append(args, "down")
	args = append(args, "--remove-orphans")

	if err := s.executor.Execute(ctx, "docker", args...); err != nil {
		return fmt.Errorf("running docker compose down: %w", err)
	}
	return nil
}

// composeUp runs 'docker compose up -d' with the given config.
func (s *Service) composeUp(ctx context.Context) error {
	fmt.Println("====> docker compose up -d --build")

	args := []string{"compose", "-p", s.cfg.DockerProject}
	args = append(args, "--env-file", s.cfg.EnvFilePath)
	for _, file := range s.files {
		args = append(args, "-f", file)
	}
	args = append(args, "up", "-d")

	if err := s.executor.Execute(ctx, "docker", args...); err != nil {
		return fmt.Errorf("running docker compose up: %w", err)
	}
	return nil
}

// ensureExclusiveProject checks for other running docker compose projects and prompts to stop them.
func (s *Service) ensureExclusiveProject(ctx context.Context, target string) error {
	others, err := listOtherActiveSDPProjects(ctx, target)
	if err != nil {
		return err
	}
	if len(others) == 0 {
		return nil
	}
	fmt.Printf("Detected other running SDP projects: %s\n", strings.Join(others, ", "))
	if !ui.ConfirmWithDefault("Stop these projects now", ui.ConfirmationDefaultYes) {
		fmt.Printf("⚠️  Cannot proceed with conflicting projects running.\n")
		fmt.Printf("   To launch manually later, stop conflicting projects first.\n")
		return ErrUserDeclined
	}
	// Stop each other project
	for _, p := range others {
		if err = s.composeStop(ctx, p); err != nil {
			return fmt.Errorf("failed to stop project %s: %w", p, err)
		}
	}
	return nil
}

func (s *Service) composeStop(ctx context.Context, project string) error {
	fmt.Println("====> docker compose stop")

	args := []string{"compose", "-p", project}
	args = append(args, "--env-file", s.cfg.EnvFilePath)
	args = append(args, "stop")

	if err := s.executor.Execute(ctx, "docker", args...); err != nil {
		return fmt.Errorf("running docker compose stop: %w", err)
	}
	return nil
}

// listOtherActiveSDPProjects lists active Docker Compose projects with "sdp" prefix, excluding the specified project.
func listOtherActiveSDPProjects(ctx context.Context, exclude string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "docker", "ps", "--format", "{{.Label \"com.docker.compose.project\"}}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("listing docker containers: %w", err)
	}

	output := strings.TrimSpace(string(out))
	if output == "" {
		return nil, nil
	}

	lines := strings.Split(output, "\n")
	projectSet := make(map[string]struct{}, len(lines))

	for _, line := range lines {
		project := strings.TrimSpace(line)
		if project == "" || project == exclude || !strings.HasPrefix(project, "sdp") {
			continue
		}
		projectSet[project] = struct{}{}
	}

	if len(projectSet) == 0 {
		return nil, nil
	}

	projects := make([]string, 0, len(projectSet))
	for project := range projectSet {
		projects = append(projects, project)
	}

	return projects, nil
}
