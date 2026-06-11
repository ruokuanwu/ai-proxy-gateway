package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"time"
)

type Config struct {
	Server   ServerConfig              `json:"server"`
	Auth     AuthConfig                `json:"auth"`
	Routing  RoutingConfig             `json:"routing"`
	Provider map[string]ProviderConfig `json:"provider"`
}

type ServerConfig struct {
	Addr            string        `json:"addr"`
	ReadTimeout     time.Duration `json:"-"`
	WriteTimeout    time.Duration `json:"-"`
	UpstreamTimeout time.Duration `json:"-"`
	RawReadTimeout  string        `json:"readTimeout"`
	RawWriteTimeout string        `json:"writeTimeout"`
	RawUpstreamTime string        `json:"upstreamTimeout"`
}

type AuthConfig struct {
	AppKeys  []AppKeyConfig `json:"appKeys"`
	AdminKey string         `json:"adminKey"`
}

type AppKeyConfig struct {
	Name    string   `json:"name"`
	Key     string   `json:"key"`
	Enabled bool     `json:"enabled"`
	Models  []string `json:"models"`
}

type RoutingConfig struct {
	Strategy       string        `json:"strategy"`
	ErrorWindow    time.Duration `json:"-"`
	RawErrorWindow string        `json:"errorWindow"`
	Retry          RetryConfig   `json:"retry"`
}

type RetryConfig struct {
	MaxAttempts          int           `json:"maxAttempts"`
	PerAttemptTimeout    time.Duration `json:"-"`
	RawPerAttemptTimeout string        `json:"perAttemptTimeout"`
	RetryOnStatus        []int         `json:"retryOnStatus"`
}

type ProviderConfig struct {
	Type    string                 `json:"type"`
	Enabled *bool                  `json:"enabled"`
	Weight  int                    `json:"weight"`
	Options ProviderOptions        `json:"options"`
	Models  map[string]ModelConfig `json:"models"`
}

type ProviderOptions struct {
	BaseURL string `json:"baseURL"`
	APIKey  string `json:"apiKey"`
}

type ModelConfig struct {
	Name          string                 `json:"name"`
	UpstreamModel string                 `json:"upstreamModel"`
	Limit         ModelLimit             `json:"limit"`
	Options       map[string]any         `json:"options"`
	Variants      map[string]VariantConf `json:"variants"`
}

type ModelLimit struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

type VariantConf map[string]any

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	applyDefaults(&cfg)
	expandSecrets(&cfg)
	if err := validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = ":8080"
	}
	cfg.Server.ReadTimeout = parseDurationDefault(cfg.Server.RawReadTimeout, 60*time.Second)
	cfg.Server.WriteTimeout = parseDurationDefault(cfg.Server.RawWriteTimeout, 300*time.Second)
	cfg.Server.UpstreamTimeout = parseDurationDefault(cfg.Server.RawUpstreamTime, 120*time.Second)
	if cfg.Routing.Strategy == "" {
		cfg.Routing.Strategy = "round_robin"
	}
	cfg.Routing.ErrorWindow = parseDurationDefault(cfg.Routing.RawErrorWindow, 5*time.Minute)
	if cfg.Routing.Retry.MaxAttempts <= 0 {
		cfg.Routing.Retry.MaxAttempts = 2
	}
	cfg.Routing.Retry.PerAttemptTimeout = parseDurationDefault(cfg.Routing.Retry.RawPerAttemptTimeout, 60*time.Second)
	if len(cfg.Routing.Retry.RetryOnStatus) == 0 {
		cfg.Routing.Retry.RetryOnStatus = []int{429, 500, 502, 503, 504}
	}
	for name, p := range cfg.Provider {
		if p.Type == "" {
			p.Type = "openai-compatible"
		}
		if p.Enabled == nil {
			enabled := true
			p.Enabled = &enabled
		}
		if p.Weight <= 0 {
			p.Weight = 1
		}
		for model, m := range p.Models {
			if m.UpstreamModel == "" {
				m.UpstreamModel = model
			}
			p.Models[model] = m
		}
		cfg.Provider[name] = p
	}
}

func validate(cfg *Config) error {
	if len(cfg.Auth.AppKeys) == 0 {
		return errors.New("auth.appKeys is required")
	}
	if len(cfg.Provider) == 0 {
		return errors.New("provider is required")
	}
	for name, p := range cfg.Provider {
		if !*p.Enabled {
			continue
		}
		if p.Options.BaseURL == "" {
			return fmt.Errorf("provider.%s.options.baseURL is required", name)
		}
		if p.Options.APIKey == "" {
			return fmt.Errorf("provider.%s.options.apiKey is required", name)
		}
		if len(p.Models) == 0 {
			return fmt.Errorf("provider.%s.models is required", name)
		}
	}
	return nil
}

var envPattern = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)

func expandSecrets(cfg *Config) {
	for name, p := range cfg.Provider {
		p.Options.APIKey = expandEnvValue(p.Options.APIKey)
		cfg.Provider[name] = p
	}
	for i := range cfg.Auth.AppKeys {
		cfg.Auth.AppKeys[i].Key = expandEnvValue(cfg.Auth.AppKeys[i].Key)
	}
	cfg.Auth.AdminKey = expandEnvValue(cfg.Auth.AdminKey)
}

func expandEnvValue(v string) string {
	matches := envPattern.FindStringSubmatch(v)
	if len(matches) != 2 {
		return v
	}
	return os.Getenv(matches[1])
}

func parseDurationDefault(raw string, def time.Duration) time.Duration {
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return def
	}
	return d
}
