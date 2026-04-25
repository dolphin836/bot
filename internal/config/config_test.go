package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dolphin836/bot/internal/config"
)

const testYAML = `
telegram:
  token: test-telegram-token
  owner_id: 12345

anthropic:
  api_key: test-anthropic-key
  model: claude-test-model

memory:
  recent_limit: 25
  summary_max_age_days: 14

db:
  path: /tmp/test.db
`

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	return path
}

func TestLoadFromFile(t *testing.T) {
	path := writeTempConfig(t, testYAML)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Telegram.Token != "test-telegram-token" {
		t.Errorf("Telegram.Token = %q, want %q", cfg.Telegram.Token, "test-telegram-token")
	}
	if cfg.Telegram.OwnerID != 12345 {
		t.Errorf("Telegram.OwnerID = %d, want %d", cfg.Telegram.OwnerID, 12345)
	}
	if cfg.Anthropic.APIKey != "test-anthropic-key" {
		t.Errorf("Anthropic.APIKey = %q, want %q", cfg.Anthropic.APIKey, "test-anthropic-key")
	}
	if cfg.Anthropic.Model != "claude-test-model" {
		t.Errorf("Anthropic.Model = %q, want %q", cfg.Anthropic.Model, "claude-test-model")
	}
	if cfg.Memory.RecentLimit != 25 {
		t.Errorf("Memory.RecentLimit = %d, want %d", cfg.Memory.RecentLimit, 25)
	}
	if cfg.Memory.SummaryMaxAgeDays != 14 {
		t.Errorf("Memory.SummaryMaxAgeDays = %d, want %d", cfg.Memory.SummaryMaxAgeDays, 14)
	}
	if cfg.DB.Path != "/tmp/test.db" {
		t.Errorf("DB.Path = %q, want %q", cfg.DB.Path, "/tmp/test.db")
	}
}

func TestLoadFromEnvOverride(t *testing.T) {
	path := writeTempConfig(t, testYAML)

	t.Setenv("TELEGRAM_BOT_TOKEN", "env-telegram-token")
	t.Setenv("ANTHROPIC_API_KEY", "env-anthropic-key")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Telegram.Token != "env-telegram-token" {
		t.Errorf("Telegram.Token = %q, want %q (env override)", cfg.Telegram.Token, "env-telegram-token")
	}
	if cfg.Anthropic.APIKey != "env-anthropic-key" {
		t.Errorf("Anthropic.APIKey = %q, want %q (env override)", cfg.Anthropic.APIKey, "env-anthropic-key")
	}
	// Non-overridden values should still come from file
	if cfg.Telegram.OwnerID != 12345 {
		t.Errorf("Telegram.OwnerID = %d, want %d (from file)", cfg.Telegram.OwnerID, 12345)
	}
	if cfg.Anthropic.Model != "claude-test-model" {
		t.Errorf("Anthropic.Model = %q, want %q (from file)", cfg.Anthropic.Model, "claude-test-model")
	}
}
