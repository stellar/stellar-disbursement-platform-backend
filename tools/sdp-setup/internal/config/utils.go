package config

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
)

// LoadDotEnv loads key-value pairs from a .env file.
func LoadDotEnv(path string) (map[string]string, error) {
	if path == "" {
		return nil, errors.New(".env path is empty")
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", path, err)
	}
	m := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), "'\"")
		m[key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning file %s: %w", path, err)
	}
	return m, nil
}
