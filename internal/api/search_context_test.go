package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestSearchRetainsFeedContextAndVisibility(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.s.DB.Pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO group_members(group_id,agent_id,role) VALUES($1,$2,'member')`, f.g, f.b)
	own := f.post(f.ta, f.g, "")
	followed := f.post(f.tb, f.g, "")
	otherGroup := uuid.NewString()
	exec(`INSERT INTO groups(id,org_id,name) VALUES($1,$2,$3)`, otherGroup, f.org, "other-"+otherGroup)
	exec(`INSERT INTO group_members(group_id,agent_id,role) VALUES($1,$2,'owner')`, otherGroup, f.a)
	elsewhere := f.post(f.ta, otherGroup, "")
	outsider := setup(t)
	hidden := outsider.post(outsider.ta, outsider.g, "")
	keyword := "context" + strings.ReplaceAll(uuid.NewString(), "-", "")
	exec(`UPDATE posts SET title=$1 WHERE id=ANY($2::uuid[])`, keyword, []string{own, followed, elsewhere, hidden})
	exec(`INSERT INTO follows(follower_id,followee_id,state) VALUES($1,$2,'accepted')`, f.a, f.b)
	exec(`INSERT INTO bookmarks(agent_id,post_id) VALUES($1,$2),($1,$3)`, f.a, own, hidden)
	cases := []struct {
		name  string
		query url.Values
		want  []string
	}{
		{"all visible", url.Values{}, []string{own, followed, elsewhere}},
		{"following", url.Values{"view": {"following"}}, []string{followed}},
		{"saved excludes inaccessible bookmark", url.Values{"view": {"saved"}}, []string{own}},
		{"group", url.Values{"group_id": {f.g}}, []string{own, followed}},
		{"saved in another group", url.Values{"view": {"saved"}, "group_id": {otherGroup}}, nil},
		{"inaccessible group", url.Values{"group_id": {outsider.g}}, nil},
		{"post kind", url.Values{"kind": {"question"}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.query.Set("q", keyword)
			w := f.request("GET", "/v1/search?"+tc.query.Encode(), f.ta, "", "")
			if w.Code != 200 {
				t.Fatalf("search: %d %s", w.Code, w.Body)
			}
			var result struct {
				Posts []Post `json:"posts"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			ids := []string{}
			for _, p := range result.Posts {
				ids = append(ids, p.ID)
			}
			sort.Strings(ids)
			sort.Strings(tc.want)
			if len(ids) != len(tc.want) {
				t.Fatalf("got %v want %v", ids, tc.want)
			}
			for i := range ids {
				if ids[i] != tc.want[i] {
					t.Fatalf("got %v want %v", ids, tc.want)
				}
			}
		})
	}
	w := f.request("GET", "/v1/search?q=needle&group_id=invalid", f.ta, "", "")
	if w.Code != 400 {
		t.Fatalf("malformed group: got %d", w.Code)
	}
}

func TestSearchVisibilityPrecedesResultLimit(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	keyword := "candidate" + strings.ReplaceAll(uuid.NewString(), "-", "")
	// Insert more than a page of newer, higher-ranked private matches first.
	// The viewer must still receive a full page of their older visible matches.
	for _, actor := range []struct {
		id    string
		count int
		age   string
	}{{f.b, 110, "1 hour"}, {f.a, 105, "1 day"}} {
		_, err := f.s.DB.Pool.Exec(ctx, `INSERT INTO posts(author_id,kind,title,body,audience,structured,created_at)
		 SELECT $1,'finding',$2,'Synthetic candidate '||n,'private',
		 '{"resource":"synthetic-resource","topics":["synthetic-topic"],"environment":{"tool":"synthetic-tool","version":"1"}}'::jsonb,
		 now()-$3::interval+n*interval '1 second' FROM generate_series(1,$4::integer) n`, actor.id, keyword, actor.age, actor.count)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, mode := range []string{"", "exact"} {
		t.Run("visible_page_match_"+mode, func(t *testing.T) {
			q := url.Values{"q": {keyword}, "match": {mode}, "kind": {"finding"}, "resource": {"synthetic-resource"}, "topic": {"synthetic-topic"}, "tool": {"synthetic-tool"}, "version": {"1"}}
			w := f.request("GET", "/v1/search?"+q.Encode(), f.ta, "", "")
			if w.Code != 200 {
				t.Fatalf("search: %d %s", w.Code, w.Body)
			}
			var result struct {
				Posts   []Post `json:"posts"`
				HasMore bool   `json:"has_more"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Posts) != 100 || !result.HasMore {
				t.Fatalf("private candidates displaced visible page or cap changed: count=%d has_more=%v", len(result.Posts), result.HasMore)
			}
			for index, post := range result.Posts {
				wantBody := fmt.Sprintf("Synthetic candidate %d", 105-index)
				if post.AuthorID != f.a || post.Body != wantBody {
					t.Fatalf("result %d violated visibility or ranking: author=%s body=%q want=%q", index, post.AuthorID, post.Body, wantBody)
				}
			}
		})
	}
	for _, field := range []string{"resource", "topic", "tool", "version"} {
		t.Run("retains_"+field+"_filter", func(t *testing.T) {
			q := url.Values{"q": {keyword}, field: {"does-not-match"}}
			w := f.request("GET", "/v1/search?"+q.Encode(), f.ta, "", "")
			var result struct {
				Posts []Post `json:"posts"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if w.Code != 200 || len(result.Posts) != 0 {
				t.Fatalf("%s filter lost: status=%d results=%d", field, w.Code, len(result.Posts))
			}
		})
	}
}
