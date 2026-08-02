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
// as files under ~/.kiro/sessions/ (see PathsV3), NOT in the SQLite database.
//
// The on-disk format is documented as "not backward-compatible" with v2 but the
// exact wire schema is not published for the preview and is expected to shift
// between early-access builds. To stay resilient, LoadV3 reuses the v2 metadata
// layout where present and extractV3Messages parses several plausible line
// shapes rather than assuming one. Sessions are tagged Source "jsonl_v3".

// LoadV3 reads Kiro CLI v3 preview sessions from PathsV3()/*.json (+ *.jsonl).
func LoadV3() []Session {
	dir := PathsV3()
	if dir == "" {
		return nil
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil
	}

	out := make([]Session, 0, len(files))
	for _, f := range files {
		// Skip *.jsonl content files that the glob may also match on some
		// platforms; only *.json metadata files drive discovery.
		if strings.HasSuffix(f, ".jsonl") {
			continue
		}
		info, err := os.Stat(f)
		if err != nil || info.Size() > MaxFileSize {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var m jsonlMeta
		if json.Unmarshal(data, &m) != nil {
			continue
		}

		jpath := strings.TrimSuffix(f, ".json") + ".jsonl"

		sessionID := m.SessionID
		if sessionID == "" {
			// Fall back to the file stem (v3 names files by UUID).
			sessionID = strings.TrimSuffix(filepath.Base(f), ".json")
		}
		title := m.Title
		if title == "" {
			title = "(untitled)"
		}

		// Estimate msg count from jsonl file size (avoids parsing at load time).
		msgCount := 0
		if ji, err := os.Stat(jpath); err == nil {
			msgCount = int(ji.Size() / 2000) // ~2KB per message avg
			if msgCount < 1 && ji.Size() > 0 {
				msgCount = 1
			}
		}

		out = append(out, Session{
			SessionID:   sessionID,
			Title:       title,
			Cwd:         m.Cwd,
			CreatedAt:   m.CreatedAt,
			UpdatedAt:   m.UpdatedAt,
			Source:      "jsonl_v3",
			DurationMin: computeDuration(m.CreatedAt, m.UpdatedAt),
			JSONLPath:   jpath,
		})
	}
	return out
}

// v3Line is a permissive view over a single v3 JSONL record. Different
// early-access builds may key role/content differently, so every plausible
// field name is captured and resolved in extractV3Messages.
type v3Line struct {
	Kind    string          `json:"kind"`
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Data    json.RawMessage `json:"data"`
	Content json.RawMessage `json:"content"`
	Payload json.RawMessage `json:"payload"`
	Text    string          `json:"text"`
}

// extractV3Messages reads messages from a v3 .jsonl file, tolerating multiple
// record shapes:
//   - v2 TUI blocks:  {"kind":"Prompt"|"AssistantMessage","data":{"content":[{"kind":"text","data":"..."}]}}
//   - role/content:   {"role":"user"|"assistant","content":"..." | [{"type":"text","text":"..."}]}
//   - typed payload:  {"type":"...","payload":{...}} with a nested text/content field
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
		var line v3Line
		if dec.Decode(&line) != nil {
			break
		}

		role := v3Role(line)
		if role == "" {
			continue
		}

		text := v3Text(line)
		if text == "" {
			continue
		}

		msgs = append(msgs, Msg{Role: role, Text: text})
		if limit > 0 && len(msgs) >= limit {
			return msgs
		}
	}
	return msgs
}

// v3Role maps a record's kind/type/role onto "you" or "kiro" (or "" to skip).
func v3Role(line v3Line) string {
	switch strings.ToLower(line.Kind) {
	case "prompt":
		return "you"
	case "assistantmessage", "response":
		return "kiro"
	}
	switch strings.ToLower(line.Role) {
	case "user", "human", "you":
		return "you"
	case "assistant", "kiro", "ai", "model":
		return "kiro"
	}
	switch strings.ToLower(line.Type) {
	case "prompt", "user", "user_message", "usermessage":
		return "you"
	case "assistant", "assistant_message", "assistantmessage", "response":
		return "kiro"
	}
	return ""
}

// v3Text pulls the human-readable text out of whichever field carries it.
func v3Text(line v3Line) string {
	// Direct text field.
	if t := strings.TrimSpace(line.Text); t != "" {
		return t
	}
	// content / data / payload may each be a string or a structured blob.
	for _, raw := range []json.RawMessage{line.Data, line.Content, line.Payload} {
		if len(raw) == 0 {
			continue
		}
		if t := textFromRaw(raw); t != "" {
			return t
		}
	}
	return ""
}

// textFromRaw extracts text from a raw JSON value that may be:
//   - a plain string
//   - {"content": <string | blocks>}
//   - {"text": "..."}
//   - {"prompt": "..."}
//   - a list of blocks [{"kind"|"type":"text","data"|"text":"..."}]
func textFromRaw(raw json.RawMessage) string {
	// Plain string.
	var s string
	if json.Unmarshal(raw, &s) == nil && strings.TrimSpace(s) != "" {
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
			// content itself may be a plain string.
			var cs string
			if json.Unmarshal(obj.Content, &cs) == nil && strings.TrimSpace(cs) != "" {
				return strings.TrimSpace(cs)
			}
		}
	}

	// Bare list of blocks.
	if t := textFromBlocks(raw); t != "" {
		return t
	}
	return ""
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

// extractV3Index returns the lowercased search text and the exact message count
// for a v3 session in a single pass. Because the v3 preview wire format is not
// fixed, a marker-based fast scan (like the v2 countFast) can't reliably match
// every line shape, so v3 always uses the tolerant parser for an accurate count.
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
