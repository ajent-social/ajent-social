// SPDX-License-Identifier: Apache-2.0

// Package skillcatalog serves an explicitly reviewed, public skill snapshot.
package skillcatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxCatalogBytes = 64 << 20

var identifier = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)

// Record is an instruction-only package. Publisher review is mandatory.
// Personal skills must never enter this public manifest without review.
type Record struct {
	ID                     string   `json:"id"`
	Name                   string   `json:"name"`
	Description            string   `json:"description"`
	Content                string   `json:"content"`
	SourceURL              string   `json:"source_url"`
	License                string   `json:"license"`
	LicenseText            string   `json:"license_text"`
	QualityStatus          string   `json:"quality_status"`
	RedistributionReviewed bool     `json:"redistribution_reviewed"`
	Prerequisites          []string `json:"prerequisites"`
}

type manifest struct {
	Schema string   `json:"schema"`
	Skills []Record `json:"skills"`
}

// Result contains bounded discovery metadata, never the instruction body.
type Result struct {
	ID            string   `json:"id"`
	Version       string   `json:"version"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	ContentSHA256 string   `json:"content_sha256"`
	ContentURL    string   `json:"content_url"`
	SourceURL     string   `json:"source_url"`
	License       string   `json:"license"`
	LicenseURL    string   `json:"license_url"`
	QualityStatus string   `json:"quality_status"`
	Prerequisites []string `json:"prerequisites"`
}

type entry struct {
	result  Result
	content string
	license string
	terms   map[string]int
	length  int
}

// Catalog is immutable after construction and safe for concurrent reads.
type Catalog struct {
	entries           []entry
	byKey             map[string]int
	documentFrequency map[string]int
	averageLength     float64
	version           string
	bootstrap         string
}

// New validates the entire snapshot before making any content available.
func New(reader io.Reader, origin, bootstrap string) (*Catalog, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("catalog origin must be an absolute origin")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")) {
		return nil, fmt.Errorf("catalog origin requires HTTPS or loopback HTTP")
	}
	origin = strings.TrimRight(origin, "/")
	data, err := io.ReadAll(io.LimitReader(reader, maxCatalogBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}
	if len(data) > maxCatalogBytes {
		return nil, fmt.Errorf("catalog exceeds size limit")
	}
	var m manifest
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return nil, fmt.Errorf("decode catalog: %w", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("catalog has trailing data")
	}
	if m.Schema != "ajent.skills.catalog/v1" || len(m.Skills) == 0 || len(m.Skills) > 100000 {
		return nil, fmt.Errorf("invalid catalog schema or entry count")
	}
	c := &Catalog{byKey: map[string]int{}, documentFrequency: map[string]int{}, bootstrap: strings.ReplaceAll(bootstrap, "{{ORIGIN}}", origin)}
	sort.Slice(m.Skills, func(i, j int) bool {
		if m.Skills[i].ID == m.Skills[j].ID {
			return m.Skills[i].Content < m.Skills[j].Content
		}
		return m.Skills[i].ID < m.Skills[j].ID
	})
	seen := map[string]bool{}
	for _, r := range m.Skills {
		if err := validate(r); err != nil {
			return nil, fmt.Errorf("invalid skill %q: %w", r.ID, err)
		}
		version := digest(r.Content)
		key := r.ID + "/" + version
		if seen[version] {
			return nil, fmt.Errorf("duplicate instruction content for %q", r.ID)
		}
		seen[version] = true
		terms := tokenize(r.Name + " " + r.Description)
		freq := map[string]int{}
		for _, t := range terms {
			freq[t]++
		}
		for t := range freq {
			c.documentFrequency[t]++
		}
		prerequisites := r.Prerequisites
		if prerequisites == nil {
			prerequisites = []string{}
		}
		result := Result{ID: r.ID, Version: version, Name: r.Name, Description: r.Description, ContentSHA256: version, ContentURL: origin + "/v1/skills/" + key, SourceURL: r.SourceURL, License: r.License, LicenseURL: origin + "/v1/licenses/" + key, QualityStatus: r.QualityStatus, Prerequisites: prerequisites}
		c.byKey[key] = len(c.entries)
		c.entries = append(c.entries, entry{result: result, content: r.Content, license: r.LicenseText, terms: freq, length: len(terms)})
		c.averageLength += float64(len(terms))
	}
	c.averageLength /= float64(len(c.entries))
	canonical, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("encode catalog fingerprint: %w", err)
	}
	c.version = digest(string(canonical))
	return c, nil
}

func validate(r Record) error {
	if !identifier.MatchString(r.ID) || len(r.Name) == 0 || len(r.Name) > 128 || len(r.Description) == 0 || len(r.Description) > 1024 || len(tokenize(r.Name+" "+r.Description)) == 0 {
		return fmt.Errorf("invalid identity or description")
	}
	if !r.RedistributionReviewed {
		return fmt.Errorf("redistribution has not been reviewed")
	}
	if r.License != "MIT" && r.License != "Apache-2.0" {
		return fmt.Errorf("unsupported license")
	}
	if len(r.LicenseText) == 0 || len(r.LicenseText) > 64<<10 {
		return fmt.Errorf("missing or oversized license notice")
	}
	if r.QualityStatus != "reviewed" && r.QualityStatus != "task-tested" {
		return fmt.Errorf("unsupported quality status")
	}
	if len(r.Content) == 0 || len(r.Content) > 256<<10 || !utf8.ValidString(r.Content) || !strings.HasPrefix(r.Content, "---\n") {
		return fmt.Errorf("invalid or oversized SKILL.md")
	}
	u, err := url.Parse(r.SourceURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return fmt.Errorf("source must be an HTTPS URL")
	}
	if len(r.Prerequisites) > 16 {
		return fmt.Errorf("too many prerequisites")
	}
	for _, p := range r.Prerequisites {
		if len(p) > 256 {
			return fmt.Errorf("prerequisite too long")
		}
	}
	return nil
}

func digest(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }

func tokenize(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// Search ranks metadata with BM25. Scores are not probabilities; an empty
// result only means this lexical query did not match this catalog snapshot.
func (c *Catalog) Search(query string, limit int) []Result {
	if limit < 1 || limit > 10 {
		limit = 5
	}
	type scored struct {
		index int
		score float64
	}
	scores := []scored{}
	queryTerms := map[string]bool{}
	for _, term := range tokenize(query) {
		queryTerms[term] = true
	}
	for i, e := range c.entries {
		var score float64
		for term := range queryTerms {
			tf := float64(e.terms[term])
			if tf == 0 {
				continue
			}
			df := float64(c.documentFrequency[term])
			idf := math.Log(1 + (float64(len(c.entries))-df+0.5)/(df+0.5))
			score += idf * tf * 2.2 / (tf + 1.2*(0.25+0.75*float64(e.length)/c.averageLength))
		}
		if score > 0 {
			scores = append(scores, scored{i, score})
		}
	}
	sort.SliceStable(scores, func(i, j int) bool { return scores[i].score > scores[j].score })
	results := []Result{}
	for _, s := range scores {
		if len(results) == limit {
			break
		}
		results = append(results, c.entries[s.index].result)
	}
	return results
}

// ServeHTTP exposes only anonymous, read-only catalog operations.
func (c *Catalog) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		failure(w, r, 405, "method_not_allowed", "Use GET or HEAD.")
		return
	}
	if len(r.URL.RequestURI()) > 8192 {
		failure(w, r, 414, "invalid_query", "Request URL is too long.")
		return
	}
	switch r.URL.Path {
	case "/", "/SKILL.md":
		serveText(w, r, c.bootstrap, "text/markdown; charset=utf-8", false)
	case "/healthz":
		writeJSON(w, r, 200, map[string]any{"status": "ok", "catalog_version": c.version, "skills": len(c.entries)})
	case "/v1/skills/search":
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			failure(w, r, 400, "invalid_query", "Invalid query encoding.")
			return
		}
		for key, values := range q {
			if (key != "q" && key != "limit") || len(values) != 1 {
				failure(w, r, 400, "invalid_query", "Use one q and optional limit.")
				return
			}
		}
		query := strings.TrimSpace(q.Get("q"))
		if len(query) == 0 || len(query) > 2048 || !utf8.ValidString(query) || len(tokenize(query)) == 0 {
			failure(w, r, 400, "invalid_query", "q must contain searchable text, at most 2048 bytes.")
			return
		}
		limit := 5
		if raw, ok := q["limit"]; ok {
			limit, err = strconv.Atoi(raw[0])
			if err != nil || limit < 1 || limit > 10 {
				failure(w, r, 400, "invalid_query", "limit must be between 1 and 10.")
				return
			}
		}
		writeJSON(w, r, 200, map[string]any{"schema": "ajent.skills.search/v1", "catalog_version": c.version, "results": c.Search(query, limit)})
	default:
		prefix := "/v1/skills/"
		media := "text/markdown; charset=utf-8"
		isLicense := strings.HasPrefix(r.URL.Path, "/v1/licenses/")
		if isLicense {
			prefix = "/v1/licenses/"
			media = "text/plain; charset=utf-8"
		}
		if !strings.HasPrefix(r.URL.Path, prefix) {
			failure(w, r, 404, "not_found", "Resource not found.")
			return
		}
		index, ok := c.byKey[strings.TrimPrefix(r.URL.Path, prefix)]
		if !ok {
			failure(w, r, 404, "not_found", "Skill version not found.")
			return
		}
		body := c.entries[index].content
		if isLicense {
			body = c.entries[index].license
		} else {
			w.Header().Set("X-Content-SHA256", c.entries[index].result.ContentSHA256)
		}
		serveText(w, r, body, media, !isLicense)
	}
}

func serveText(w http.ResponseWriter, r *http.Request, body, media string, immutable bool) {
	w.Header().Set("Content-Type", media)
	etag := `"` + digest(body) + `"`
	w.Header().Set("ETag", etag)
	if immutable {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method != http.MethodHead {
		_, err := io.WriteString(w, body)
		if err != nil {
			return
		}
	}
}

func failure(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, r, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func writeJSON(w http.ResponseWriter, r *http.Request, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "response encoding failed", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, err = w.Write(append(data, '\n'))
		if err != nil {
			return
		}
	}
}
