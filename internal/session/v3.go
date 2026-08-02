package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Kiro CLI v3 (preview / early access) support.
//
// v3 is opt-in (`kiro-cli --v3`) and runs alongside a 2.x install. It is
// TUI-only — the classic/SQLite mode is not supported — so sessions are stored
// as files, NOT in the SQLite database. Each session is its own directory:
//
//	<base>/<workspace-hash>/<session-dir>/session.json   metadata
//	<base>/<workspace-hash>/<session-dir>/messages.jsonl  event stream
//
// where <base> is ~/.kiro/sessions (see PathsV3). The session directory name is
// the session UUID (some builds prefix it with "sess_"); the canonical ID is the
// "id" field inside session.json. Sessions are tagged Source "jsonl_v3".

// v3Meta mirrors the fields of session.json that this viewer needs.
type v3Meta struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	WorkspacePaths []string `json:"workspacePaths"`
	CreatedAt      string   `json:"createdAt"`
	LastModifiedAt string   `json:"lastModifiedAt"`
	Status         string   `json:"status"`
}

// v3Event is one line of messages.jsonl. The event stream carries many record
// types (user/assistant turns, tool_call/tool_result pairs, turn boundaries,
// steering inclusions, usage summaries); only the discriminator and content are
// needed to reconstruct the conversation.
type v3Event struct {
	Payload struct {
		Type    string          `json:"type"`
		Content json.RawMessage `json:"content"`
	} `json:"payload"`
}

// LoadV3 discovers Kiro CLI v3 preview sessions under PathsV3().
// Layout: <base>/<workspace-hash>/<session-dir>/session.json (+ messages.jsonl).
func LoadV3() []Session {
	base := PathsV3()
	if base == "" {
		return nil
	}
	// Two levels deep: workspace-hash / session-dir / session.json.
	matches, err := filepath.Glob(filepath.Join(base, "*", "*", "session.json"))
	if err != nil {
		return nil
	}

	out := make([]Session, 0, len(matches))
	for _, metaPath := range matches {
		info, err := os.Stat(metaPath)
		if err != nil || info.Size() > MaxFileSize {
			continue
		}
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var m v3Meta
		if json.Unmarshal(data, &m) != nil {
			continue
		}

		dir := filepath.Dir(metaPath)

		sessionID := m.ID
		if sessionID == "" {
			// Fall back to the session directory name (the UUID, possibly
			// prefixed with "sess_").
			sessionID = strings.TrimPrefix(filepath.Base(dir), "sess_")
		}
		title := m.Title
		if title == "" {
			title = "(untitled)"
		}
		cwd := ""
		if len(m.WorkspacePaths) > 0 {
			cwd = m.WorkspacePaths[0]
		}

		msgsPath := filepath.Join(dir, "messages.jsonl")

		// Estimate msg count from the event-stream size to avoid parsing at
		// load time; the exact count is filled in later by extractV3Index.
		msgCount := 0
		if ji, err := os.Stat(msgsPath); err == nil {
			msgCount = int(ji.Size() / 1500)
			if msgCount < 1 && ji.Size() > 0 {
				msgCount = 1
			}
		}

		out = append(out, Session{
			SessionID:   sessionID,
			Title:       title,
			Cwd:         cwd,
			CreatedAt:   m.CreatedAt,
			UpdatedAt:   m.LastModifiedAt,
			Source:      "jsonl_v3",
			MsgCount:    msgCount,
			DurationMin: computeDuration(m.CreatedAt, m.LastModifiedAt),
			JSONLPath:   msgsPath,
		})
	}
	return out
}

// extractV3Messages reconstructs the user/assistant conversation from a v3
// messages.jsonl event stream. Non-conversational events (tool calls/results,
// turn boundaries, session metadata, usage summaries) are skipped. Consecutive
// events from the same role are merged into a single bubble so a turn made of
// several streamed assistant events reads as one message.
func extractV3Messages(path string, limit int) []Msg {
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		return nil
	}
	if info.Size() > MaxFileSize {
		return []Msg{{Role: "system", Text: "(File too large to preview)"}}
	}

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var msgs []Msg
	dec := json.NewDecoder(f)
	for dec.More() {
		var e v3Event
		if dec.Decode(&e) != nil {
			break
		}

		var role string
		switch e.Payload.Type {
		case "user":
			role = "you"
		case "assistant":
			role = "kiro"
		default:
			continue
		}

		text := textFromRaw(e.Payload.Content)
		if text == "" {
			continue
		}

		// Merge consecutive same-role events into one bubble.
		if n := len(msgs); n > 0 && msgs[n-1].Role == role {
			msgs[n-1].Text += "\n\n" + text
			continue
		}
		msgs = append(msgs, Msg{Role: role, Text: text})
		if limit > 0 && len(msgs) >= limit {
			return msgs
		}
	}
	return msgs
}

// extractV3Index returns the lowercased search text and the exact message count
// for a v3 session in a single pass, using the tolerant conversation parser.
func extractV3Index(path string) (text string, count int) {
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 || info.Size() > MaxFileSize {
		return "", 0
	}
	msgs := extractV3Messages(path, 0)
	if len(msgs) == 0 {
		return "", 0
	}
	var b strings.Builder
	for _, m := range msgs {
		b.WriteString(strings.ToLower(m.Text))
		b.WriteByte('\n')
	}
	return b.String(), len(msgs)
}

// textFromRaw extracts human-readable text from a v3 content value, which is
// normally a plain string but is tolerated as a structured value too:
//   - a plain string
//   - {"content": <string | blocks>} / {"text": "..."} / {"prompt": "..."}
//   - a list of blocks [{"type"|"kind":"text","text"|"data":"..."}]
func textFromRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// Plain string (the common case for user/assistant content).
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}

	// Object with common text-bearing keys.
	var obj struct {
		Content json.RawMessage `json:"content"`
		Text    string          `json:"text"`
		Prompt  string          `json:"prompt"`
		Data    string          `json:"data"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		if t := strings.TrimSpace(obj.Text); t != "" {
			return t
		}
		if t := strings.TrimSpace(obj.Prompt); t != "" {
			return t
		}
		if t := strings.TrimSpace(obj.Data); t != "" {
			return t
		}
		if len(obj.Content) > 0 {
			if t := textFromBlocks(obj.Content); t != "" {
				return t
			}
			var cs string
			if json.Unmarshal(obj.Content, &cs) == nil && strings.TrimSpace(cs) != "" {
				return strings.TrimSpace(cs)
			}
		}
	}

	// Bare list of blocks.
	return textFromBlocks(raw)
}

// textFromBlocks joins the text of the "text" blocks in a content array.
func textFromBlocks(raw json.RawMessage) string {
	var blocks []struct {
		Kind string `json:"kind"`
		Type string `json:"type"`
		Data string `json:"data"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		kind := strings.ToLower(b.Kind)
		if kind == "" {
			kind = strings.ToLower(b.Type)
		}
		if kind != "" && kind != "text" {
			continue
		}
		if b.Data != "" {
			parts = append(parts, b.Data)
		} else if b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}
