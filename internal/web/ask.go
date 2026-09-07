package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

//go:embed guide/answers.json
var guideFS embed.FS

type GuideEntry struct{ ID, Title, Keywords, Answer string }
type AskPage struct {
	Question, Error string
	Asked           bool
	Answers         []GuideEntry
}

// These are published help entries, never network content. The small corpus is
// loaded once; the ask path has no database access or model-provider dependency.
var ajentGuide = func() []GuideEntry {
	b, err := guideFS.ReadFile("guide/answers.json")
	if err != nil {
		panic(err)
	}
	var entries []GuideEntry
	if err = json.Unmarshal(b, &entries); err != nil {
		panic(err)
	}
	return entries
}()

// Adapted from sirelog/sire-chat's keyword retrieval fallback: BM25 ranking,
// bounded results and an explicit no-match response. No generated answer text.
func guideTerms(text string) []string {
	stop := map[string]bool{"a": true, "an": true, "the": true, "is": true, "are": true, "i": true, "my": true, "me": true, "it": true, "to": true, "of": true, "for": true, "and": true, "in": true, "do": true, "does": true, "can": true, "how": true, "what": true, "with": true, "on": true, "you": true, "your": true, "this": true, "that": true, "about": true}
	aliases := map[string]string{"ajents": "ajent", "joining": "join", "connects": "connect", "connecting": "connect"}
	var out []string
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if v := aliases[word]; v != "" {
			word = v
		}
		if !stop[word] {
			out = append(out, word)
		}
	}
	return out
}
func findGuideAnswers(question string) []GuideEntry {
	query := guideTerms(question)
	if len(query) == 0 {
		return nil
	}
	// Product and agent names alone carry no discriminating evidence. Give the
	// introduction for those queries, but ignore them alongside a specific topic.
	var specific []string
	for _, term := range query {
		if term != "ajent" && term != "agent" && term != "agents" {
			specific = append(specific, term)
		}
	}
	if len(specific) == 0 {
		return []GuideEntry{ajentGuide[0]}
	}
	query = specific
	counts := make([]map[string]int, len(ajentGuide))
	lengths := make([]int, len(ajentGuide))
	df := map[string]int{}
	avg := 0.0
	for i, entry := range ajentGuide {
		// Rank only curated topics. Answer boilerplate must not cause an unrelated
		// privacy or account question to retrieve every entry.
		terms := guideTerms(entry.Title + " " + entry.Keywords)
		counts[i] = map[string]int{}
		lengths[i] = len(terms)
		avg += float64(len(terms))
		for _, term := range terms {
			counts[i][term]++
		}
		for term := range counts[i] {
			df[term]++
		}
	}
	avg /= float64(len(ajentGuide))
	type match struct {
		index int
		score float64
	}
	var matches []match
	for i := range ajentGuide {
		score := 0.0
		seen := map[string]bool{}
		for _, term := range query {
			if seen[term] {
				continue
			}
			seen[term] = true
			f := float64(counts[i][term])
			if f == 0 {
				continue
			}
			idf := math.Log(1 + (float64(len(ajentGuide)-df[term])+0.5)/(float64(df[term])+0.5))
			score += idf * f * 2.4 / (f + 1.4*(0.25+0.75*float64(lengths[i])/avg))
		}
		if score > 0 {
			matches = append(matches, match{i, score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].score > matches[j].score })
	var out []GuideEntry
	for _, m := range matches {
		if len(out) == 3 {
			break
		}
		out = append(out, ajentGuide[m.index])
	}
	return out
}
func (s *Server) Ask(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	p := AskPage{}
	status := http.StatusOK
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Question is too large or invalid.", http.StatusBadRequest)
			return
		}
		p.Question = strings.TrimSpace(r.PostForm.Get("question"))
		if !utf8.ValidString(p.Question) || utf8.RuneCountInString(p.Question) > 1000 || p.Question == "" {
			p.Error = "Enter a question between 1 and 1,000 characters."
			status = http.StatusBadRequest
		} else {
			p.Asked = true
			p.Answers = findGuideAnswers(p.Question)
		}
	}
	// Plain POST works without JavaScript and keeps questions out of URL/history.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := s.T.ExecuteTemplate(w, "ask", p); err != nil {
		fmt.Println("ask template:", err)
	}
}
func (s *Server) Guide(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.T.ExecuteTemplate(w, "guide", ajentGuide); err != nil {
		fmt.Println("guide template:", err)
	}
}
