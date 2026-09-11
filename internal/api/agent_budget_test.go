package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestAgentBudgetWorstCase(t *testing.T) {
	for _, text := range []string{"x", "😀", "\x00", "<"} {
		posts := make([]Post, 10)
		for i := range posts {
			posts[i] = Post{ID: "00000000-0000-0000-0000-000000000001", Title: strings.Repeat(text, 160), Body: strings.Repeat(text, 16000), Structured: json.RawMessage(`{"large":"` + strings.Repeat("x", 16000) + `"}`)}
		}
		for i := range posts {
			posts[i].Structured = json.RawMessage(`{"summary":"` + strings.Repeat("x", 10000) + `","evidence":"` + strings.Repeat("x", 10000) + `","applicability":"` + strings.Repeat("x", 10000) + `","limitations":"` + strings.Repeat("x", 10000) + `"}`)
			posts[i].AuthorID = posts[i].ID
			posts[i].GroupID = posts[i].ID
			posts[i].Handle = strings.Repeat(text, 80)
		}
		searchRaw, _ := json.Marshal(map[string]any{"posts": agentSearchPreviews(posts[:5]), "has_more": true})
		if len(searchRaw) > agentResultBytes {
			t.Fatalf("search exceeds budget: %d", len(searchRaw))
		}
		raw, _ := json.Marshal(map[string]any{"posts": agentPreviews(posts), "next_cursor": strings.Repeat("x", 100)})
		if len(raw) > agentResultBytes {
			t.Fatalf("preview exceeds byte budget: %d", len(raw))
		}
		offset := 0
		var content strings.Builder
		for {
			r := httptest.NewRequest("GET", "/", nil)
			if offset > 0 {
				q := r.URL.Query()
				q.Set("offset", strconv.Itoa(offset))
				r.URL.RawQuery = q.Encode()
			}
			w := httptest.NewRecorder()
			writeAgentPost(w, r, posts[0])
			if w.Code != 200 || w.Body.Len() > agentResultBytes {
				t.Fatalf("read status=%d size=%d", w.Code, w.Body.Len())
			}
			var chunk struct {
				Content string `json:"content"`
				Next    *int   `json:"next_offset"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &chunk); err != nil {
				t.Fatal(err)
			}
			content.WriteString(chunk.Content)
			if chunk.Next == nil {
				break
			}
			offset = *chunk.Next
		}
		var got Post
		if err := json.Unmarshal([]byte(content.String()), &got); err != nil || got.Body != posts[0].Body {
			t.Fatal("chunk reconstruction failed", err)
		}
	}
}

func TestAgentFeedBudgetAndCursor(t *testing.T) {
	f := setup(t)
	for i := 0; i < 12; i++ {
		f.post(f.ta, f.g, "")
	}
	// Legacy lifecycle data is filtered before the limit.
	_, err := f.s.DB.Pool.Exec(context.Background(), `INSERT INTO posts(author_id,group_id,kind,title,body,audience,expires_at) VALUES($1,$2,'status','Agent joined','Connected','private',now()+interval '1 day')`, f.a, f.g)
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string) map[string]any {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer "+f.ta)
		r.Header.Set("X-Ajent-Client", "mcp")
		w := httptest.NewRecorder()
		f.m.ServeHTTP(w, r)
		if w.Code != http.StatusOK || w.Body.Len() > agentResultBytes {
			t.Fatalf("%s: status %d size %d %s", path, w.Code, w.Body.Len(), w.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	first := get("/v1/groups/" + f.g + "/posts")
	if len(first["posts"].([]any)) != 10 {
		t.Fatal("expected ten previews")
	}
	cursor := first["next_cursor"].(string)
	next := get("/v1/groups/" + f.g + "/posts?cursor=" + cursor)
	if len(next["posts"].([]any)) != 2 {
		t.Fatal("legacy cursor did not advance", next)
	}
	modern := get("/v1/groups/" + f.g + "/posts?before=" + cursor)
	if len(modern["posts"].([]any)) != 2 {
		t.Fatal("before cursor did not advance")
	}
	search := get("/v1/search?q=needle&group_id=" + f.g)
	if len(search["posts"].([]any)) != 5 || search["has_more"] != true {
		t.Fatal("search not bounded", search)
	}
	for _, raw := range first["posts"].([]any) {
		post := raw.(map[string]any)
		if post["title"] == "Agent joined" || post["structured"] != nil || post["preview_only"] != true {
			t.Fatal("not useful compact preview", post)
		}
	}
	denied := f.request("POST", "/v1/posts", f.ta, `{"kind":"status","title":"Agent joined","body":"Connected","group_id":"`+f.g+`"}`, "lifecycle")
	if denied.Code != 400 || !strings.Contains(denied.Body.String(), "presence_not_content") {
		t.Fatal("presence was accepted", denied.Body.String())
	}
	f.post(f.ta, f.g, "") // ordinary publications remain usable
}
