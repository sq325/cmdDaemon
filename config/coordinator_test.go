package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestCoordinator_loadconfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("successful load", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		// Create a temporary config file
		tempDir := t.TempDir()
		configFile := filepath.Join(tempDir, "config.yaml")
		configContent := DefaultConfig
		err := os.WriteFile(configFile, []byte(configContent), 0644)
		if err != nil {
			t.Fatalf("failed to create temp config file: %v", err)
		}

		c := &coordinator{
			configFile: configFile,
			logger:     logger,
		}
		c.registerMetrics(reg)

		err = c.loadconfig()
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if c.config == nil {
			t.Error("expected config to be loaded, got nil")
		}
		t.Log("config loaded successfully:", c.config)
	})

	t.Run("file not found", func(t *testing.T) {
		reg := prometheus.NewRegistry()

		c := &coordinator{
			configFile: "/nonexistent/file.yaml",
			logger:     logger,
		}
		c.registerMetrics(reg)

		err := c.loadconfig()
		if err == nil {
			t.Error("expected error for nonexistent file, got nil")
		}
	})

	t.Run("invalid config format", func(t *testing.T) {
		reg := prometheus.NewRegistry()

		// Create a temporary invalid config file
		tempDir := t.TempDir()
		configFile := filepath.Join(tempDir, "invalid.yaml")
		invalidContent := `invalid: yaml: content: [`
		err := os.WriteFile(configFile, []byte(invalidContent), 0644)
		if err != nil {
			t.Fatalf("failed to create temp invalid config file: %v", err)
		}

		c := &coordinator{
			configFile: configFile,
			logger:     logger,
		}
		c.registerMetrics(reg)

		defer func() {
			if r := recover(); r == nil {
				t.Errorf("loadconfig did not panic for invalid config format as expected")
			}
		}()

		c.loadconfig()
	})
}
