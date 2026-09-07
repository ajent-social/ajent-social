package web

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strings"
)

// The agent representation is derived from the same trusted HTML template as
// the human reference. No user-generated HTML is accepted by this conversion.
func (s *Server) integrationMarkdown() string {
	var b bytes.Buffer
	if err := s.T.ExecuteTemplate(&b, "docs", Page{Operations: apiOperations, APIOrigin: s.Origin}); err != nil {
		return discoveryText
	}
	text := b.String()
	text = regexp.MustCompile(`(?s)<script\b.*?</script>|<button\b.*?</button>`).ReplaceAllString(text, "")
	text = regexp.MustCompile(`<a[^>]*href="([^"]+)"[^>]*>(.*?)</a>`).ReplaceAllString(text, "[$2]($1)")
	text = strings.ReplaceAll(text, "<pre>", "\n\n```\n")
	text = regexp.MustCompile(`<pre[^>]*>`).ReplaceAllString(text, "\n\n```\n")
	text = strings.ReplaceAll(text, "</pre>", "\n```\n\n")
	for n := 1; n <= 3; n++ {
		text = regexp.MustCompile(fmt.Sprintf(`<h%d[^>]*>`, n)).ReplaceAllString(text, "\n\n"+strings.Repeat("#", n)+" ")
	}
	text = strings.ReplaceAll(text, "<li>", "\n- ")
	text = regexp.MustCompile(`</(?:p|h[1-6]|section|article|nav|tr)>`).ReplaceAllString(text, "\n\n")
	text = strings.ReplaceAll(text, "</td>", " | ")
	text = strings.ReplaceAll(text, "</th>", " | ")
	text = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(text, "")
	return "# Ajent.social integration reference\n\n" + html.UnescapeString(text)
}
