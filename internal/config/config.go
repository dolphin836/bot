package config

import (
	"github.com/spf13/viper"
)

// Config holds all application configuration.
type Config struct {
	Telegram  TelegramConfig  `mapstructure:"telegram"`
	Anthropic AnthropicConfig `mapstructure:"anthropic"`
	Memory    MemoryConfig    `mapstructure:"memory"`
	DB        DBConfig        `mapstructure:"db"`
	Weather   WeatherConfig   `mapstructure:"weather"`
}

type WeatherConfig struct {
	DefaultCity      string  `mapstructure:"default_city"`
	DefaultLatitude  float64 `mapstructure:"default_latitude"`
	DefaultLongitude float64 `mapstructure:"default_longitude"`
}

// TelegramConfig holds Telegram bot configuration.
type TelegramConfig struct {
	Token   string `mapstructure:"token"`
	OwnerID int64  `mapstructure:"owner_id"`
}

// AnthropicConfig holds Anthropic API configuration.
type AnthropicConfig struct {
	APIKey string `mapstructure:"api_key"`
	Model  string `mapstructure:"model"`
}

// MemoryConfig holds memory/conversation history configuration.
type MemoryConfig struct {
	RecentLimit       int `mapstructure:"recent_limit"`
	SummaryMaxAgeDays int `mapstructure:"summary_max_age_days"`
}

// DBConfig holds database configuration.
type DBConfig struct {
	Path string `mapstructure:"path"`
}

// Load reads configuration from the YAML file at path, with environment
// variable overrides for sensitive values.
func Load(path string) (*Config, error) {
	v := viper.New()

	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	// Bind environment variable overrides.
	if err := v.BindEnv("telegram.token", "TELEGRAM_BOT_TOKEN"); err != nil {
		return nil, err
	}
	if err := v.BindEnv("anthropic.api_key", "ANTHROPIC_API_KEY"); err != nil {
		return nil, err
	}

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
