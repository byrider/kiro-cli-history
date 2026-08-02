package session

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds user preferences.
type Config struct {
	SQLiteEnabled bool   `json:"sqlite_enabled"`  // load classic mode SQLite sessions
	SQLiteIndex   bool   `json:"sqlite_index"`    // full-text index SQLite content
	DefaultView   string `json:"default_view"`    // "list" or "tree"
	V3Enabled     bool   `json:"v3_enabled"`      // load Kiro CLI v3 (preview) sessions
	V3SessionsDir string `json:"v3_sessions_dir"` // override base v3 sessions dir (empty = ~/.kiro/sessions)
}

var DefaultConfig = Config{
	SQLiteEnabled: true,
	SQLiteIndex:   false,
	DefaultView:   "list",
	V3Enabled:     true,
	V3SessionsDir: "",
}

var AppConfig = DefaultConfig

func configPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "kiro-cli-history", "config.json")
}

// LoadConfig reads config from ~/.config/kiro-cli-history/config.json.
// Creates default config if not found.
func LoadConfig() {
	path := configPath()
	if path == "" {
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		// Create default config
		SaveConfig(DefaultConfig)
		return
	}

	if json.Unmarshal(data, &AppConfig) != nil {
		AppConfig = DefaultConfig
	}
}

// SaveConfig writes config to disk.
func SaveConfig(cfg Config) {
	path := configPath()
	if path == "" {
		return
	}
	os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(path, data, 0644)
	AppConfig = cfg
}
