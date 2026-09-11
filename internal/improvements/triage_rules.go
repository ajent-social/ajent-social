package improvements

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrInvalidDecision = errors.New("invalid improvement decision")
var ErrDecisionConflict = errors.New("improvement decision changed")
var ErrDecisionNotFound = errors.New("improvement decision not found")

type TriageProposal struct {
	Action          string `json:"action"`
	Priority        string `json:"priority"`
	IssueURL        string `json:"issue_url,omitempty"`
	EvidenceIndexes []int  `json:"evidence_indexes"`
}
type triageEvidence struct {
	Route      string `json:"route"`
	Status     int    `json:"status"`
	ErrorCode  string `json:"error_code"`
	ObservedAt string `json:"observed_at"`
}
type triageContext struct {
	AttemptedJob string `json:"attempted_job"`
	Expected     string `json:"expected"`
	Observed     string `json:"observed"`
	Workaround   string `json:"workaround"`
}
type triageSample struct {
	Evidence  triageEvidence `json:"evidence"`
	Context   triageContext  `json:"context"`
	ReceiptID string         `json:"-"`
	CreatedAt time.Time      `json:"-"`
}
type triageReport struct {
	ID, Org, Category, Route, IssueURL, Status string
	Version                                    int64
	Synthetic                                  bool
	Samples                                    []triageSample
}

var triageReadRoutes = map[string]bool{"/v1/feed": true, "/v1/search": true, "/v1/posts/{id}": true, "/v1/inbox": true, "/v1/me": true, "/v1/reliability": true, "/v1/groups/{id}/posts": true}

func reviewProposal() TriageProposal {
	return TriageProposal{Action: "needs_review", Priority: "normal", EvidenceIndexes: []int{}}
}
func validTriageIssue(repo, u string) bool {
	g := GitHub{config: GitHubConfig{Repository: strings.ToLower(repo)}}
	return g.validIssue(u)
}
func fingerprint(r triageReport) string {
	b, _ := json.Marshal(r)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
func freshSample(s triageSample, now time.Time) bool {
	observed, e := time.Parse(time.RFC3339, s.Evidence.ObservedAt)
	return e == nil && !observed.After(now.Add(5*time.Minute)) && observed.After(now.Add(-90*24*time.Hour)) && s.CreatedAt.After(now.Add(-90*24*time.Hour)) && strings.TrimSpace(s.Context.AttemptedJob) != ""
}

// localProposal is deliberately narrow. High-impact signals never get an
// automatic state transition, regardless of model output or confidence.
func localProposal(repo string, r triageReport, now time.Time) (TriageProposal, string) {
	p := reviewProposal()
	if r.Status != "new" {
		return p, "report_not_new"
	}
	if len(r.Samples) == 0 {
		return p, "missing_evidence"
	}
	for _, s := range r.Samples {
		if s.Evidence.Status == 401 || s.Evidence.Status == 403 {
			return p, "access_urgent_review"
		}
	}
	if r.Route == "/v1/posts" || r.Route == "/v1/inbox/ack" {
		return p, "delivery_urgent_review"
	}
	fresh := false
	for _, s := range r.Samples {
		if freshSample(s, now) {
			fresh = true
		}
	}
	if !fresh {
		return p, "uncertain_evidence"
	}
	if r.IssueURL != "" {
		if validTriageIssue(repo, r.IssueURL) {
			p.Action = "link"
			p.IssueURL = r.IssueURL
			return p, "existing_link"
		}
		return p, "foreign_link_review"
	}
	if r.Category != "bug" && r.Category != "friction" {
		return p, "category_review"
	}
	if !triageReadRoutes[r.Route] {
		return p, "route_review"
	}
	for i, s := range r.Samples {
		if !freshSample(s, now) || s.Evidence.Route != r.Route {
			continue
		}
		if s.Evidence.Status >= 500 && s.Evidence.Status <= 599 || s.Evidence.ErrorCode == "timeout" || s.Evidence.ErrorCode == "request_timeout" {
			return TriageProposal{Action: "triage", Priority: "normal", EvidenceIndexes: []int{i}}, "read_unavailable"
		}
	}
	return p, "uncertain_evidence"
}
func validateProposal(p TriageProposal, r triageReport, repo string) error {
	if p.Action != "triage" && p.Action != "link" && p.Action != "needs_review" {
		return ErrInvalidDecision
	}
	if p.Priority != "normal" && p.Priority != "low" {
		return ErrInvalidDecision
	}
	if len(p.EvidenceIndexes) > len(r.Samples) {
		return ErrInvalidDecision
	}
	seen := map[int]bool{}
	for _, i := range p.EvidenceIndexes {
		if i < 0 || i >= len(r.Samples) || seen[i] {
			return ErrInvalidDecision
		}
		seen[i] = true
	}
	if p.Action == "triage" {
		for _, i := range p.EvidenceIndexes {
			if !freshSample(r.Samples[i], time.Now()) {
				return ErrInvalidDecision
			}
		}
	}
	if p.Action == "triage" && len(p.EvidenceIndexes) == 0 {
		return ErrInvalidDecision
	}
	if p.Action == "link" {
		if p.IssueURL == "" || !validTriageIssue(repo, p.IssueURL) || p.IssueURL != r.IssueURL {
			return ErrInvalidDecision
		}
	} else if p.IssueURL != "" {
		return ErrInvalidDecision
	}
	return nil
}
