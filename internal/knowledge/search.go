package knowledge

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

type Preview struct {
	PostID        string `json:"post_id"`
	FactID        string `json:"fact_id"`
	Title         string `json:"title"`
	Excerpt       string `json:"excerpt"`
	SearchState   string `json:"search_state"`
	EvidenceState string `json:"evidence_state"`
}
type SearchResult struct {
	Trust     string    `json:"trust"`
	Results   []Preview `json:"results"`
	Truncated bool      `json:"truncated"`
}

// Search filters canonical candidate IDs through CURRENT source authority after
// the RPC, before returning any snippet. Unmapped sources, recent-facts output,
// withdrawn pointers and user-specific blocked sources never leave the gateway.
func (s *Service) Search(ctx context.Context, viewer, query string) (SearchResult, error) {
	out := SearchResult{Trust: "untrusted_reference_data", Results: []Preview{}}
	hits, err := s.Backend.Search(ctx, query)
	if err != nil {
		return out, err
	}
	if len(hits) > 20 {
		hits = hits[:20]
		out.Truncated = true
	}
	ids := make([]string, 0, len(hits))
	for _, h := range hits {
		ids = append(ids, h.ID)
	}
	rows, err := s.Pool.Query(ctx, `SELECT k.fact_id,k.post_id::text,coalesce(p.title,''),k.search_state
 FROM knowledge_publications k JOIN posts p ON p.id=k.post_id
 WHERE k.brain_id=$1 AND k.fact_id=ANY($2::text[]) AND k.state='indexed'
 AND k.revoked_at IS NULL AND k.expires_at>now() AND knowledge_source_eligible(k.post_id)
 AND can_read_post($3,k.post_id)`, s.BrainID, ids, viewer)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	allowed := map[string]Preview{}
	for rows.Next() {
		var p Preview
		if err := rows.Scan(&p.FactID, &p.PostID, &p.Title, &p.SearchState); err != nil {
			return out, err
		}
		p.EvidenceState = "author_reported_unverified"
		allowed[p.FactID] = p
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	seen := map[string]bool{}
	for _, h := range hits {
		p, ok := allowed[h.ID]
		if !ok || seen[h.ID] {
			continue
		}
		seen[h.ID] = true
		p.Excerpt = boundedUTF8(h.Snippet, 1800)
		candidate := out
		candidate.Results = append(append([]Preview{}, out.Results...), p)
		data, err := json.Marshal(candidate)
		if err != nil {
			return out, err
		}
		if len(out.Results) >= 5 || len(data) > 12000 {
			out.Truncated = true
			break
		}
		out.Results = candidate.Results
	}
	return out, nil
}
func boundedUTF8(s string, limit int) string {
	s = strings.ToValidUTF8(s, "�")
	if len(s) <= limit {
		return s
	}
	s = s[:limit]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}
