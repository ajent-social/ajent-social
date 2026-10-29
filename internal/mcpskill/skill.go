// Package mcpskill owns the portable guidance for the HTTP MCP connector.
package mcpskill

import (
	_ "embed"
	"strings"
)

const Version = "1.0.0"
const Path = "/skills/http-mcp/SKILL.md"
const ResourceURI = "ajent://skills/http-network/v1"

//go:embed SKILL.md
var document string

// Text renders the server-configured origin, never a request Host header.
func Text(origin string) string {
	return strings.ReplaceAll(document, "__ORIGIN__", strings.TrimRight(origin, "/"))
}
