package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

const (
	envFileFlag   = "--env-file"
	envFileEnvVar = "ENV_FILE"
)

// LoadEnvFile loads environment variables from a file.
// Priority: --env-file flag > ENV_FILE environment variable > .env in working directory
func LoadEnvFile() error {
	envFilePath := determineEnvFilePath()

	if envFilePath != "" {
		return loadExplicitEnvFile(envFilePath)
	}

	return loadDefaultEnvFile()
}

// determineEnvFilePath determines the path to the env file based on priority.
func determineEnvFilePath() string {
	if path := parseEnvFileFlag(); path != "" {
		return toAbsolutePath(path)
	}

	if path := os.Getenv(envFileEnvVar); path != "" {
		return toAbsolutePath(path)
	}

	return ""
}

// parseEnvFileFlag checks command-line arguments for the --env-file flag.
func parseEnvFileFlag() string {
	for i, arg := range os.Args {
		if arg == envFileFlag && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
		if strings.HasPrefix(arg, envFileFlag+"=") {
			return strings.TrimPrefix(arg, envFileFlag+"=")
		}
	}
	return ""
}

// toAbsolutePath converts a relative path to an absolute path.
func toAbsolutePath(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return absPath
}

// loadExplicitEnvFile loads environment variables from the specified file.
func loadExplicitEnvFile(path string) error {
	if err := godotenv.Load(path); err != nil {
		return fmt.Errorf("loading env file %s: %w", path, err)
	}
	return nil
}

// loadDefaultEnvFile loads environment variables from the default .env file.
func loadDefaultEnvFile() error {
	err := godotenv.Load()
	if err == nil {
		return nil
	}

	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return fmt.Errorf("loading .env file: %w", err)
}
