// Package improvements implements durable improvement operations.
package improvements

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const githubAPI = "https://api.github.com"
const githubMaxPages = 100
const githubMaxBody = 8 << 20

// GitHubConfig accepts one externally provisioned credential source. Tokens
// must be scoped to Repository with Issues write permission. File rotation is
// observed on the next request; neither source is cached or logged.
type GitHubConfig struct{ Repository, TokenFile, TokenEnv string }
type GitHub struct {
	config GitHubConfig
	client *http.Client
	base   string
}

// GitHubError contains only bounded classifications, never provider bodies,
// credential values, or transport errors (which may contain URLs).
type GitHubError struct {
	Kind       string
	StatusCode int
	Ambiguous  bool
	RetryAfter time.Duration
}

func (e *GitHubError) Error() string {
	return fmt.Sprintf("github %s (status %d, ambiguous %t)", e.Kind, e.StatusCode, e.Ambiguous)
}

func NewGitHub(c GitHubConfig) (*GitHub, error) {
	if !ValidRepository(c.Repository) || (c.TokenFile == "") == (c.TokenEnv == "") {
		return nil, errors.New("github requires exact repository and one token source")
	}
	c.Repository = strings.ToLower(c.Repository)
	return &GitHub{config: c, base: githubAPI, client: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9_.-]+$`)

func ValidRepository(repo string) bool {
	if len(repo) > 200 || !repositoryPattern.MatchString(repo) {
		return false
	}
	parts := strings.Split(repo, "/")
	name := parts[1]
	return len(parts[0]) <= 100 && len(name) <= 100 && name != "." && name != ".."
}

var reportIDPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var markerPattern = regexp.MustCompile(`^<!-- ajent-improvement:[a-f0-9]{64} -->$`)

// Projection can only be published unchanged after ReportProjection constructs
// it. The unexported source binds content and repository to the safe template.
type Projection struct {
	Title, Body, Marker       string
	repo, id, category, route string
	synthetic                 bool
}

func ReportProjection(repo, id, category, route string, synthetic bool) (Projection, error) {
	if !ValidRepository(repo) || !reportIDPattern.MatchString(id) {
		return Projection{}, errors.New("invalid projection identity")
	}
	if category != "bug" && category != "feature" && category != "friction" {
		return Projection{}, errors.New("invalid projection category")
	}
	repo = strings.ToLower(repo)
	operation := map[string]string{"/v1/me": "identity lookup", "/v1/feed": "feed retrieval", "/v1/search": "search", "/v1/posts": "publication", "/v1/posts/{id}": "post access", "/v1/inbox": "inbox retrieval", "/v1/inbox/ack": "inbox acknowledgment", "/v1/reliability": "diagnostics", "/v1/groups/{id}/posts": "workspace feed retrieval"}[route]
	if operation == "" {
		operation = "workflow"
	}
	marker := fmt.Sprintf("<!-- ajent-improvement:%x -->", sha256.Sum256([]byte(repo+":"+id)))
	title := "Ajent " + category + ": investigate " + operation + " friction"
	body := "An agent reported a problem while using Ajent. Review the private improvement record for the attempted workflow, captured diagnostics and reproduction.\n\nAcceptance: reproduce the original workflow, deploy the fix, and obtain an affected-agent verification result. A merged PR alone is not resolution.\n\nPrivate reporter context and diagnostics are intentionally omitted.\n\n" + marker + "\n"
	if synthetic {
		title = "[Synthetic verification] " + title
		body = "Controlled end-to-end verification; this is not a product defect.\n\n" + body
	}
	return Projection{Title: title, Body: body, Marker: marker, repo: repo, id: id, category: category, route: route, synthetic: synthetic}, nil
}
func (g *GitHub) token() (string, error) {
	var token string
	if g.config.TokenFile != "" {
		f, e := os.Open(g.config.TokenFile)
		if e != nil {
			return "", &GitHubError{Kind: "credential"}
		}
		defer f.Close()
		b, e := io.ReadAll(io.LimitReader(f, 16385))
		if e != nil || len(b) > 16384 {
			return "", &GitHubError{Kind: "credential"}
		}
		token = string(b)
	} else {
		token = os.Getenv(g.config.TokenEnv)
	}
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 16384 || strings.ContainsAny(token, "\r\n\t ") {
		return "", &GitHubError{Kind: "credential"}
	}
	return token, nil
}
func (g *GitHub) request(ctx context.Context, method, path string, body []byte) ([]byte, http.Header, error) {
	token, err := g.token()
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, g.base+path, bytes.NewReader(body))
	if err != nil {
		return nil, nil, &GitHubError{Kind: "request"}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ajent-improvement-worker")
	res, err := g.client.Do(req)
	if err != nil {
		return nil, nil, &GitHubError{Kind: "unavailable", Ambiguous: method == http.MethodPost}
	}
	defer res.Body.Close()
	expected := http.StatusOK
	if method == http.MethodPost {
		expected = http.StatusCreated
	}
	if res.StatusCode != expected {
		kind := "rejected"
		if res.StatusCode == 401 || res.StatusCode == 403 {
			kind = "auth"
		}
		retry := retryAfter(res.Header)
		if res.StatusCode == 429 || (res.StatusCode == 403 && (res.Header.Get("X-RateLimit-Remaining") == "0" || res.Header.Get("Retry-After") != "")) {
			kind = "rate_limit"
		}
		if res.StatusCode >= 500 {
			kind = "unavailable"
		}
		// Unexpected success can represent a completed mutation too. Redirects are
		// never followed and are conservatively ambiguous for create.
		ambiguous := method == http.MethodPost && (res.StatusCode < 400 || res.StatusCode >= 500)
		return nil, nil, &GitHubError{Kind: kind, StatusCode: res.StatusCode, Ambiguous: ambiguous, RetryAfter: retry}
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, githubMaxBody+1))
	if err != nil || len(b) > githubMaxBody {
		return nil, nil, &GitHubError{Kind: "invalid_response", StatusCode: res.StatusCode, Ambiguous: method == http.MethodPost}
	}
	return b, res.Header, nil
}
func retryAfter(h http.Header) time.Duration {
	const capDelay = 24 * time.Hour
	var d time.Duration
	if s := h.Get("Retry-After"); s != "" {
		if n, e := strconv.ParseInt(s, 10, 64); e == nil && n >= 0 {
			if n > 86400 {
				return capDelay
			}
			d = time.Duration(n) * time.Second
		} else if t, e := http.ParseTime(s); e == nil {
			d = time.Until(t)
		}
	}
	if d <= 0 && h.Get("X-RateLimit-Remaining") == "0" {
		if n, e := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); e == nil {
			d = time.Until(time.Unix(n, 0))
		}
	}
	if d < 0 {
		return 0
	}
	if d > capDelay {
		return capDelay
	}
	return d
}
func (g *GitHub) validIssue(raw string) bool {
	prefix := "https://github.com/" + g.config.Repository + "/issues/"
	if !strings.HasPrefix(strings.ToLower(raw), prefix) {
		return false
	}
	n := raw[len(prefix):]
	if len(n) == 0 || n[0] < '1' || n[0] > '9' {
		return false
	}
	for _, r := range n {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

type githubIssue struct {
	URL         string          `json:"html_url"`
	Body        string          `json:"body"`
	Number      int64           `json:"number"`
	PullRequest json.RawMessage `json:"pull_request"`
}

// FindIssue scans repository issues in creation order, including closed issues,
// without search-index dependence. It never follows server-supplied links.
// Missing markers are returned only after a complete scan; an exhausted bound,
// malformed page or multiple distinct matches requires operator reconciliation.
func (g *GitHub) FindIssue(ctx context.Context, marker string) (string, error) {
	if !markerPattern.MatchString(marker) {
		return "", &GitHubError{Kind: "marker"}
	}
	matches := map[string]bool{}
	for page := 1; page <= githubMaxPages; page++ {
		b, h, err := g.request(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/issues?state=all&sort=created&direction=asc&per_page=100&page=%d", g.config.Repository, page), nil)
		if err != nil {
			return "", err
		}
		var rows []githubIssue
		if json.Unmarshal(b, &rows) != nil || rows == nil || len(rows) > 100 {
			return "", &GitHubError{Kind: "invalid_response", StatusCode: 200}
		}
		for _, row := range rows {
			if len(row.PullRequest) > 0 && string(row.PullRequest) != "null" {
				continue
			}
			if row.Number <= 0 || !g.validIssue(row.URL) || !strings.HasSuffix(row.URL, "/"+strconv.FormatInt(row.Number, 10)) {
				return "", &GitHubError{Kind: "invalid_response", StatusCode: 200}
			}
			if strings.Contains(row.Body, marker) {
				matches[strings.ToLower(row.URL)] = true
			}
		}
		if len(matches) > 1 {
			return "", &GitHubError{Kind: "multiple_matches"}
		}
		// A full page requires a further request even if Link is absent. A next
		// link on a short page also requires continuation, never early absence.
		more := len(rows) == 100 || strings.Contains(h.Get("Link"), `rel="next"`)
		if !more {
			for u := range matches {
				return u, nil
			}
			return "", nil
		}
	}
	return "", &GitHubError{Kind: "incomplete_search"}
}

// CreateIssue performs exactly one POST. Persist an intent first. Ambiguous
// errors authorize search/reconciliation only, never another blind create.
func (g *GitHub) CreateIssue(ctx context.Context, p Projection) (string, error) {
	expected, err := ReportProjection(p.repo, p.id, p.category, p.route, p.synthetic)
	if err != nil || p.repo != g.config.Repository || p.Title != expected.Title || p.Body != expected.Body || p.Marker != expected.Marker {
		return "", &GitHubError{Kind: "projection"}
	}
	body, err := json.Marshal(struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}{p.Title, p.Body})
	if err != nil {
		return "", &GitHubError{Kind: "projection"}
	}
	b, _, err := g.request(ctx, http.MethodPost, "/repos/"+g.config.Repository+"/issues", body)
	if err != nil {
		return "", err
	}
	var issue githubIssue
	if json.Unmarshal(b, &issue) != nil || issue.Number <= 0 || !g.validIssue(issue.URL) || !strings.HasSuffix(issue.URL, "/"+strconv.FormatInt(issue.Number, 10)) || (len(issue.PullRequest) > 0 && string(issue.PullRequest) != "null") {
		return "", &GitHubError{Kind: "invalid_response", StatusCode: 201, Ambiguous: true}
	}
	return issue.URL, nil
}
