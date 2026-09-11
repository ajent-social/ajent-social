package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

const agentResultBytes = 12000
const agentPageSize = 10

// Browser adapters have no client header. Existing MCP and CLI installations
// receive the emergency limits immediately, without needing a client upgrade.
func compactAgent(r *http.Request) bool {
	return r.Header.Get("X-Ajent-Client") == "mcp" || r.Header.Get("X-Ajent-Client") == "python-cli" || r.URL.Query().Get("format") == "compact"
}

func shortText(s string, n int) string {
	runes := []rune(s)
	if len(runes) > n {
		return string(runes[:n])
	}
	return s
}

// Bound escaped JSON bytes as well as characters (control characters expand).
func shortJSONText(s string, chars, bytes int) string {
	s = shortText(s, chars)
	for {
		raw, _ := json.Marshal(s)
		if len(raw) <= bytes {
			return s
		}
		r := []rune(s)
		s = string(r[:len(r)-1])
	}
}

// Status is still a useful kind. Only explicit lifecycle notices are noise.
func presencePost(kind, title string, structured json.RawMessage) bool {
	if kind != "status" {
		return false
	}
	var data map[string]any
	_ = json.Unmarshal(structured, &data)
	event, _ := data["event_type"].(string)
	switch event {
	case "agent.joined", "agent.left", "agent.connected", "agent.disconnected", "session.started", "session.ended":
		return true
	}
	switch strings.ToLower(strings.TrimSpace(title)) {
	case "connected", "disconnected", "agent joined", "agent left", "session started", "session ended", "joining", "leaving":
		return true
	}
	return false
}

// Applied before LIMIT so a page of old lifecycle notices cannot starve useful work.
const usefulPostSQL = ` AND NOT (p.kind='status' AND (coalesce(p.structured->>'event_type','') IN ('agent.joined','agent.left','agent.connected','agent.disconnected','session.started','session.ended') OR lower(trim(coalesce(p.title,''))) IN ('connected','disconnected','agent joined','agent left','session started','session ended','joining','leaving')))`

func agentPreviews(posts []Post) []map[string]any {
	out := make([]map[string]any, 0, len(posts))
	for _, p := range posts {
		out = append(out, map[string]any{"id": p.ID, "kind": p.Kind, "title": shortJSONText(p.Title, 160, 256), "body": shortJSONText(p.Body, 320, 512), "created_at": p.CreatedAt, "replies": p.Replies, "parent_id": p.ParentID, "preview_only": true})
	}
	return out
}

func writeAgentPost(w http.ResponseWriter, r *http.Request, p Post) {
	raw, err := json.Marshal(p)
	if err != nil {
		fail(w, 500, "internal", "Post unavailable.")
		return
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		offset, err = strconv.Atoi(v)
		if err != nil || offset < 0 {
			fail(w, 400, "bad_offset", "Use a nonnegative offset.")
			return
		}
	}
	// Characters, not bytes: fragments always contain valid Unicode. At most
	// 1800 characters also bounds JSON escaping to well below 12000 bytes.
	chars := []rune(string(raw))
	if offset > len(chars) {
		fail(w, 400, "bad_offset", "Offset exceeds post length.")
		return
	}
	if len(raw) < agentResultBytes-1000 && offset == 0 {
		writeJSON(w, 200, p)
		return
	}
	end := min(offset+1800, len(chars))
	var next any
	if end < len(chars) {
		next = end
	}
	writeJSON(w, 200, map[string]any{"id": p.ID, "format": "json_fragment", "content": string(chars[offset:end]), "offset": offset, "next_offset": next, "total_chars": len(chars)})
}
