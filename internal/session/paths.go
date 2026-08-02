package session

import (
	"os"
	"path/filepath"
	"runtime"
)

// Paths returns the JSONL sessions dir and SQLite DB path.
// On Linux: ~/.local/share/kiro-cli/data.sqlite3
// On macOS: ~/Library/Application Support/kiro-cli/data.sqlite3
func Paths() (sessionsDir, sqliteDB string) {
	if d := os.Getenv("KIRO_DEMO_DIR"); d != "" {
		return filepath.Join(d, "kiro", "sessions", "cli"),
			filepath.Join(d, "kiro-cli", "data.sqlite3")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", ""
	}
	sessionsDir = filepath.Join(home, ".kiro", "sessions", "cli")

	switch runtime.GOOS {
	case "darwin":
		sqliteDB = filepath.Join(home, "Library", "Application Support", "kiro-cli", "data.sqlite3")
	default:
		sqliteDB = filepath.Join(home, ".local", "share", "kiro-cli", "data.sqlite3")
	}
	return
}

// PathsV3 returns the directory where Kiro CLI v3 (preview / early access,
// `kiro-cli --v3`) stores its session files.
//
// v3 is TUI-only (classic/SQLite mode is unsupported) and keeps its sessions
// under ~/.kiro/sessions/ in a new, non-backward-compatible format that lives
// separately from the v2 TUI sessions in ~/.kiro/sessions/cli/.
//
// The v3 preview is still moving, so the exact sub-directory can change between
// early-access builds. Resolution order (first match wins):
//  1. KIRO_V3_SESSIONS_DIR env var — explicit override
//  2. AppConfig.V3SessionsDir — persisted override from the config file
//  3. KIRO_DEMO_DIR — demo/screenshot fixtures
//  4. ~/.kiro/sessions/cli-v3 — default that never collides with v2's cli/ dir
func PathsV3() (sessionsDir string) {
	if d := os.Getenv("KIRO_V3_SESSIONS_DIR"); d != "" {
		return d
	}
	if AppConfig.V3SessionsDir != "" {
		return AppConfig.V3SessionsDir
	}
	if d := os.Getenv("KIRO_DEMO_DIR"); d != "" {
		return filepath.Join(d, "kiro", "sessions", "cli-v3")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".kiro", "sessions", "cli-v3")
}
