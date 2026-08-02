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

// PathsV3 returns the base directory under which Kiro CLI v3 (preview / early
// access, `kiro-cli --v3`) stores its session files.
//
// v3 is TUI-only (classic/SQLite mode is unsupported) and stores each session
// as a directory in a new, non-backward-compatible layout:
//
//	<base>/<workspace-hash>/<session-dir>/session.json   (metadata)
//	<base>/<workspace-hash>/<session-dir>/messages.jsonl  (event stream)
//
// The base is ~/.kiro/sessions — the same root as the v2 TUI JSONL sessions,
// which live in the sibling ~/.kiro/sessions/cli/ directory. LoadV3 discovers
// sessions two levels deep, so it never picks up the flat v2 cli/ files.
//
// Resolution order (first match wins):
//  1. KIRO_V3_SESSIONS_DIR env var — explicit override of the base dir
//  2. AppConfig.V3SessionsDir — persisted override from the config file
//  3. KIRO_DEMO_DIR — demo/screenshot fixtures
//  4. ~/.kiro/sessions — default
func PathsV3() (sessionsDir string) {
	if d := os.Getenv("KIRO_V3_SESSIONS_DIR"); d != "" {
		return d
	}
	if AppConfig.V3SessionsDir != "" {
		return AppConfig.V3SessionsDir
	}
	if d := os.Getenv("KIRO_DEMO_DIR"); d != "" {
		return filepath.Join(d, "kiro", "sessions")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".kiro", "sessions")
}
