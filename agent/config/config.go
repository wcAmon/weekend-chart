package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	ServerURL  string `json:"server_url"`
	AgentToken string `json:"agent_token"`
	AgentName  string `json:"agent_name"`
}

func GetConfigPath() string {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = "."
	}
	return filepath.Join(appData, "weekend-chart", "config.json")
}

func Load() (*Config, error) {
	path := GetConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func Save(cfg *Config) error {
	path := GetConfigPath()

	// Create directory if not exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

func GenerateToken() string {
	// Generate a random token
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte('a' + (os.Getpid()+i)%26)
	}
	return string(b)
}
