package session

const MaxFileSize = 100 * 1024 * 1024 // 100MB

// Session represents a Kiro CLI conversation from any source.
type Session struct {
	SessionID   string
	Title       string
	Cwd         string
	CreatedAt   string
	UpdatedAt   string
	Source      string // "jsonl", "jsonl_v3", "sqlite_v1", "sqlite_v2"
	MsgCount    int
	DurationMin int
	JSONLPath   string // JSONL sessions
	SearchText  string // pre-built lowercase index
	Pinned      bool   // favorite (derived from Config.Pinned, not persisted here)
}

// Msg is a single conversation message.
type Msg struct {
	Role string // "you" or "kiro"
	Text string
}
