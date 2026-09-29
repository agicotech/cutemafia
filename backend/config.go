package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type config struct {
	Server struct {
		Listen         string   `yaml:"listen"`
		AllowedOrigins []string `yaml:"allowed_origins"`
		AdminPassword  string   `yaml:"admin_password"`
	} `yaml:"server"`
	Database struct {
		Driver string `yaml:"driver"`
		DSN    string `yaml:"dsn"`
	} `yaml:"database"`
	Storage struct {
		Endpoint      string `yaml:"endpoint"`
		Region        string `yaml:"region"`
		Bucket        string `yaml:"bucket"`
		AccessKey     string `yaml:"access_key"`
		SecretKey     string `yaml:"secret_key"`
		PublicBaseURL string `yaml:"public_base_url"`
		Prefix        string `yaml:"prefix"`
	} `yaml:"storage"`
	Telegram struct {
		Enabled      bool     `yaml:"enabled"`
		APIHost      string   `yaml:"api_host"`
		BotToken     string   `yaml:"bot_token"`
		BotPublicID  string   `yaml:"bot_public_id"`
		APIKey       string   `yaml:"api_key"`
		ChatID       string   `yaml:"chat_id"`
		PollInterval string   `yaml:"poll_interval"`
		RetryDelays  []string `yaml:"retry_delays"`
	} `yaml:"telegram"`
	Uploads struct {
		MaxBytes   int64  `yaml:"max_bytes"`
		GCInterval string `yaml:"gc_interval"`
		GCMinAge   string `yaml:"gc_min_age"`
	} `yaml:"uploads"`

	pollInterval time.Duration
	retryDelays  []time.Duration
	gcInterval   time.Duration
	gcMinAge     time.Duration
}

func loadConfig(path string) (config, error) {
	var cfg config
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	decoder := yaml.NewDecoder(strings.NewReader(string(b)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Server.Listen == "" {
		cfg.Server.Listen = "127.0.0.1:8000"
	}
	if cfg.Storage.Endpoint == "" {
		cfg.Storage.Endpoint = "https://storage.yandexcloud.net"
	}
	if cfg.Storage.Region == "" {
		cfg.Storage.Region = "ru-central1"
	}
	if cfg.Storage.Prefix == "" {
		cfg.Storage.Prefix = "media/"
	}
	if !strings.HasSuffix(cfg.Storage.Prefix, "/") {
		cfg.Storage.Prefix += "/"
	}
	if cfg.Uploads.MaxBytes == 0 {
		cfg.Uploads.MaxBytes = 100 * 1024 * 1024
	}

	if cfg.Telegram.PollInterval == "" {
		cfg.Telegram.PollInterval = "15s"
	}
	if cfg.Telegram.APIHost == "" {
		cfg.Telegram.APIHost = "api.telegram.org"
	}
	if !strings.Contains(cfg.Telegram.APIHost, "://") {
		cfg.Telegram.APIHost = "https://" + cfg.Telegram.APIHost
	}
	telegramURL, parseErr := url.Parse(cfg.Telegram.APIHost)
	if parseErr != nil || telegramURL.Host == "" || (telegramURL.Scheme != "http" && telegramURL.Scheme != "https") {
		return cfg, errors.New("telegram.api_host must be an HTTP(S) host or URL")
	}
	if len(cfg.Telegram.RetryDelays) == 0 {
		cfg.Telegram.RetryDelays = []string{"1m", "5m", "30m", "2h", "12h", "24h"}
	}
	if cfg.Uploads.GCInterval == "" {
		cfg.Uploads.GCInterval = "24h"
	}
	if cfg.Uploads.GCMinAge == "" {
		cfg.Uploads.GCMinAge = "24h"
	}

	if cfg.pollInterval, err = positiveDuration("telegram.poll_interval", cfg.Telegram.PollInterval); err != nil {
		return cfg, err
	}
	for _, value := range cfg.Telegram.RetryDelays {
		d, parseErr := positiveDuration("telegram.retry_delays", value)
		if parseErr != nil {
			return cfg, parseErr
		}
		cfg.retryDelays = append(cfg.retryDelays, d)
	}
	if cfg.gcInterval, err = positiveDuration("uploads.gc_interval", cfg.Uploads.GCInterval); err != nil {
		return cfg, err
	}
	if cfg.gcMinAge, err = positiveDuration("uploads.gc_min_age", cfg.Uploads.GCMinAge); err != nil {
		return cfg, err
	}

	if cfg.Server.AdminPassword == "" {
		return cfg, errors.New("server.admin_password is required")
	}
	if cfg.Database.Driver != "sqlite" && cfg.Database.Driver != "postgres" {
		return cfg, errors.New("database.driver must be sqlite or postgres")
	}
	if cfg.Database.DSN == "" {
		return cfg, errors.New("database.dsn is required")
	}
	if cfg.Storage.Bucket == "" || cfg.Storage.AccessKey == "" || cfg.Storage.SecretKey == "" || cfg.Storage.PublicBaseURL == "" {
		return cfg, errors.New("storage bucket, credentials and public_base_url are required")
	}
	botGate := cfg.Telegram.BotPublicID != "" || cfg.Telegram.APIKey != ""
	if botGate && (cfg.Telegram.BotPublicID == "" || cfg.Telegram.APIKey == "") {
		return cfg, errors.New("telegram.bot_public_id and telegram.api_key must be set together")
	}
	if cfg.Telegram.Enabled && (cfg.Telegram.ChatID == "" || (!botGate && cfg.Telegram.BotToken == "")) {
		return cfg, errors.New("telegram.chat_id and either bot_token or BotGate credentials are required when enabled")
	}
	return cfg, nil
}

func positiveDuration(name, value string) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return d, nil
}
