// Copyright (c) 2026 Lingying. SPDX-License-Identifier: MIT
// Auth — credential storage and resolution.
// Priority: env LY_ACCESS_TOKEN > env LY_API_KEY > ~/.config/ly/config.json
package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Resolved struct {
	Value  string
	Type   string // "oauth" | "apikey"
	Source string // "env" | "config" | ""
}

func (r Resolved) Display() string {
	if r.Value == "" {
		return "none"
	}
	return fmt.Sprintf("%s (%s)", r.Value[:4]+"****", r.Type)
}

func (r Resolved) HasUpload() bool {
	return r.Type == "oauth"
}

type Config struct {
	APIKey       string `json:"api_key,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	DefaultModel string `json:"default_model,omitempty"`
	OutputDir    string `json:"output_dir,omitempty"`
}

func Resolve() Resolved {
	if t := os.Getenv("LY_ACCESS_TOKEN"); t != "" {
		return Resolved{Value: t, Type: "oauth", Source: "env"}
	}
	if k := os.Getenv("LY_API_KEY"); k != "" {
		return Resolved{Value: k, Type: "apikey", Source: "env"}
	}
	cfg := loadConfig()
	if cfg.AccessToken != "" {
		return Resolved{Value: cfg.AccessToken, Type: "oauth", Source: "config"}
	}
	if cfg.APIKey != "" {
		return Resolved{Value: cfg.APIKey, Type: "apikey", Source: "config"}
	}
	return Resolved{}
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ly", "config.json")
}

func loadConfig() Config {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return Config{}
	}
	var cfg Config
	json.Unmarshal(data, &cfg)
	return cfg
}

func saveConfig(cfg Config) error {
	p := configPath()
	os.MkdirAll(filepath.Dir(p), 0700)
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0600)
}

func StoreOAuthToken(accessToken, refreshToken string) error {
	cfg := loadConfig()
	cfg.AccessToken = accessToken
	cfg.RefreshToken = refreshToken
	return saveConfig(cfg)
}

func StoreAPIKey(key string) error {
	cfg := loadConfig()
	cfg.APIKey = key
	return saveConfig(cfg)
}

func StoreDefaultModel(id string) error {
	cfg := loadConfig()
	cfg.DefaultModel = id
	return saveConfig(cfg)
}

func GetDefaultModel() string {
	return loadConfig().DefaultModel
}
