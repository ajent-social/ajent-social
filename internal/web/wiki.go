package web

import (
	"bytes"
	"embed"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"slices"
	"strings"
	"unicode/utf8"
)

// wikiAssets contains only reviewed public catalog projections and generated HTML.
// No request-time source retrieval, credentials, or database access is involved.
//
//go:embed wiki/*
var wikiAssets embed.FS

var wikiBundle struct {
	Source       json.RawMessage   `json:"source"`
	Capabilities []json.RawMessage `json:"capabilities"`
	Pages        map[string]string `json:"pages"`
	Cards        map[string]string `json:"cards"`
}
var wikiCatalogTemplate *template.Template
var wikiRecords []wikiRecord
var wikiDomains = []string{"identity", "billing", "infrastructure", "delivery", "protocol"}

func init() {
	raw, err := wikiAssets.ReadFile("wiki/bundle.json")
	if err != nil {
		panic(fmt.Errorf("reading embedded wiki: %w", err))
	}
	if err := json.Unmarshal(raw, &wikiBundle); err != nil {
		panic(fmt.Errorf("decoding embedded wiki: %w", err))
	}
	seen := make(map[string]bool)
	for _, raw := range wikiBundle.Capabilities {
		var c wikiRecord
		if err := json.Unmarshal(raw, &c); err != nil {
			panic(fmt.Errorf("decoding embedded wiki record: %w", err))
		}
		if c.ID == "" || seen[c.ID] || wikiBundle.Cards[c.ID] == "" || wikiBundle.Pages["/wiki/amsl/"+c.ID] == "" {
			panic("inconsistent embedded wiki assets")
		}
		seen[c.ID] = true
		c.Raw = raw
		wikiRecords = append(wikiRecords, c)
	}
	wikiCatalogTemplate, err = template.ParseFS(wikiAssets, "wiki/catalog.html")
	if err != nil {
		panic(fmt.Errorf("parsing embedded wiki: %w", err))
	}
}

type wikiRecord struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Summary string          `json:"summary"`
	Domain  string          `json:"domain"`
	UseWhen []string        `json:"use_when"`
	Raw     json.RawMessage `json:"-"`
}

func wikiVisible() []wikiRecord {
	if os.Getenv("WIKI_ENABLED") == "false" {
		return nil
	}
	hidden := strings.Split(os.Getenv("WIKI_HIDDEN_IDS"), ",")
	for i := range hidden {
		hidden[i] = strings.TrimSpace(hidden[i])
	}
	records := make([]wikiRecord, 0, len(wikiBundle.Capabilities))
	for _, c := range wikiRecords {
		if !slices.Contains(hidden, c.ID) {
			records = append(records, c)
		}
	}
	return records
}
func wikiPaths() []string {
	if os.Getenv("WIKI_ENABLED") == "false" {
		return nil
	}
	paths := []string{"/wiki", "/wiki/amsl"}
	for _, c := range wikiVisible() {
		paths = append(paths, "/wiki/amsl/"+c.ID)
	}
	return paths
}
func (s *Server) wiki(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	fail := func(status int) {
		w.Header().Set("X-Robots-Tag", "noindex")
		if r.Method == "HEAD" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(status)
		} else {
			http.Error(w, http.StatusText(status), status)
		}
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		w.Header().Set("Allow", "GET, HEAD")
		fail(405)
		return
	}
	if os.Getenv("WIKI_ENABLED") == "false" {
		fail(503)
		return
	}
	path := r.URL.Path
	if strings.HasSuffix(path, "/") {
		target := strings.TrimRight(path, "/")
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
		return
	}
	records := wikiVisible()
	var body []byte
	contentType := "text/html; charset=utf-8"
	switch path {
	case "/wiki/style.css":
		var err error
		body, err = wikiAssets.ReadFile("wiki/style.css")
		if err != nil {
			fail(500)
			return
		}
		contentType = "text/css; charset=utf-8"
	case "/wiki/amsl/catalog.json":
		caps := make([]json.RawMessage, 0, len(records))
		for _, c := range records {
			caps = append(caps, c.Raw)
		}
		var err error
		body, err = json.Marshal(struct {
			Source       json.RawMessage   `json:"source"`
			Capabilities []json.RawMessage `json:"capabilities"`
		}{wikiBundle.Source, caps})
		if err != nil {
			fail(500)
			return
		}
		contentType = "application/json; charset=utf-8"
	case "/wiki/sitemap.xml":
		type entry struct {
			Loc string `xml:"loc"`
		}
		doc := struct {
			XMLName xml.Name `xml:"urlset"`
			XMLNS   string   `xml:"xmlns,attr"`
			URLs    []entry  `xml:"url"`
		}{XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
		for _, p := range wikiPaths() {
			doc.URLs = append(doc.URLs, entry{s.Origin + p})
		}
		var err error
		body, err = xml.Marshal(doc)
		if err != nil {
			fail(500)
			return
		}
		body = append([]byte(xml.Header), body...)
		contentType = "application/xml; charset=utf-8"
	case "/wiki/amsl":
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		domain := r.URL.Query().Get("domain")
		if utf8.RuneCountInString(q) > 160 || (domain != "" && !slices.Contains(wikiDomains, domain)) {
			fail(400)
			return
		}
		cards := []template.HTML{}
		for _, c := range records {
			haystack := strings.ToLower(strings.Join(append([]string{c.ID, c.Name, c.Summary}, c.UseWhen...), " "))
			if (domain == "" || domain == c.Domain) && (q == "" || strings.Contains(haystack, strings.ToLower(q))) {
				// Only build-generated, escaped HTML from the embedded public snapshot is trusted.
				// Request values and source records are never converted to template.HTML.
				cards = append(cards, template.HTML(wikiBundle.Cards[c.ID]))
			}
		}
		data := struct {
			Query, Domain string
			Domains       []string
			Cards         []template.HTML
			Total         int
			Filtered      bool
		}{q, domain, wikiDomains, cards, len(records), q != "" || domain != ""}
		var buf bytes.Buffer
		if err := wikiCatalogTemplate.Execute(&buf, data); err != nil {
			fail(500)
			return
		}
		body = buf.Bytes()
	default:
		allowed := path == "/wiki"
		for _, c := range records {
			if path == "/wiki/amsl/"+c.ID {
				allowed = true
				break
			}
		}
		html, exists := wikiBundle.Pages[path]
		if !allowed || !exists {
			fail(404)
			return
		}
		body = []byte(html)
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	if r.Method != "HEAD" {
		_, _ = w.Write(body)
	}
}
