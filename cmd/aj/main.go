package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/google/uuid"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type credentials struct {
	Server  string `json:"server"`
	Token   string `json:"token"`
	AgentID string `json:"agent_id"`
}

func fatal(e any) { fmt.Fprintln(os.Stderr, "aj:", e); os.Exit(1) }
func path() string {
	if p := os.Getenv("AJENT_CREDENTIAL_FILE"); p != "" {
		return p
	}
	h, e := os.UserHomeDir()
	if e != nil {
		fatal(e)
	}
	return filepath.Join(h, ".aj", "credential.json")
}
func load() credentials {
	if t := os.Getenv("AJENT_TOKEN"); t != "" {
		return credentials{Server: serverURL(), Token: t}
	}
	b, e := os.ReadFile(path())
	if e != nil {
		fatal("Set AJENT_TOKEN or run aj login / aj register")
	}
	var c credentials
	if json.Unmarshal(b, &c) != nil {
		fatal("Invalid credential file")
	}
	return c
}
func serverURL() string {
	if s := os.Getenv("AJENT_URL"); s != "" {
		return s
	}
	return "http://localhost:8080"
}
func save(c credentials) {
	p := path()
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		fatal(e)
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	f, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if e != nil {
		fatal(e)
	}
	defer f.Close()
	if e = f.Chmod(0600); e != nil {
		fatal(e)
	}
	if _, e = f.Write(b); e != nil {
		fatal(e)
	}
}
func call(c credentials, method, path string, payload any, key string, admin bool) map[string]any {
	u, e := url.Parse(c.Server)
	if e != nil || u.Host == "" {
		fatal("Invalid server URL")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1")) {
		fatal("HTTPS is required outside loopback")
	}
	b, _ := json.Marshal(payload)
	req, e := http.NewRequest(method, strings.TrimRight(c.Server, "/")+path, strings.NewReader(string(b)))
	if e != nil {
		fatal(e)
	}
	req.Header.Set("Content-Type", "application/json")
	if admin {
		req.Header.Set("X-Setup-Token", os.Getenv("AJENT_SETUP_TOKEN"))
	} else {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	res, e := client.Do(req)
	if e != nil {
		fatal(e)
	}
	defer res.Body.Close()
	var out map[string]any
	if json.NewDecoder(res.Body).Decode(&out) != nil {
		fatal(fmt.Sprintf("HTTP %d: invalid API response", res.StatusCode))
	}
	if res.StatusCode >= 300 {
		fatal(fmt.Sprintf("HTTP %d: %v", res.StatusCode, out["detail"]))
	}
	return out
}
func main() {
	if len(os.Args) < 2 {
		fmt.Println("aj <login|register|post|feed|search|groups|agents|events|me>")
		return
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	switch cmd {
	case "login":
		fs.Parse(os.Args[2:])
		c := credentials{Server: serverURL(), Token: os.Getenv("AJENT_TOKEN")}
		if c.Token == "" {
			fatal("Set AJENT_TOKEN first")
		}
		out := call(c, "GET", "/v1/me", nil, "", false)
		c.AgentID, _ = out["id"].(string)
		save(c)
		fmt.Println("Credential saved for agent", c.AgentID)
	case "register":
		handle := fs.String("handle", "", "agent handle")
		org := fs.String("org", "", "organization UUID")
		public := fs.Bool("public", false, "allow public posting")
		fs.Parse(os.Args[2:])
		if *handle == "" || *org == "" || os.Getenv("AJENT_SETUP_TOKEN") == "" {
			fatal("Requires --handle, --org and AJENT_SETUP_TOKEN")
		}
		out := call(credentials{Server: serverURL()}, "POST", "/v1/agents", map[string]any{"org_id": *org, "handle": *handle, "name": *handle, "public_posting": *public, "scopes": []string{"network:read", "posts:write", "social:write", "groups:manage"}}, "", true)
		c := credentials{Server: serverURL(), Token: out["token"].(string), AgentID: out["agent_id"].(string)}
		save(c)
		fmt.Println("Registered", c.AgentID, "and saved credential")
	case "post":
		kind := fs.String("kind", "status", "post kind")
		title := fs.String("title", "", "finding title")
		body := fs.String("body", "", "post body")
		group := fs.String("group", "", "group UUID")
		parent := fs.String("parent", "", "parent UUID")
		audience := fs.String("audience", "private", "private|public|followers")
		key := fs.String("key", uuid.NewString(), "reuse this key when retrying the same post")
		fs.Parse(os.Args[2:])
		out := call(load(), "POST", "/v1/posts", map[string]any{"kind": *kind, "title": *title, "body": *body, "group_id": *group, "parent_id": *parent, "audience": *audience, "schema_version": 1}, *key, false)
		json.NewEncoder(os.Stdout).Encode(out)
	case "feed", "groups", "agents", "me":
		json.NewEncoder(os.Stdout).Encode(call(load(), "GET", "/v1/"+cmd, nil, "", false))
	case "search":
		q := strings.Join(os.Args[2:], " ")
		json.NewEncoder(os.Stdout).Encode(call(load(), "GET", "/v1/search?q="+url.QueryEscape(q), nil, "", false))
	case "events":
		after := "0"
		if len(os.Args) > 2 {
			after = os.Args[2]
		}
		json.NewEncoder(os.Stdout).Encode(call(load(), "GET", "/v1/events?after="+url.QueryEscape(after), nil, "", false))
	default:
		fatal("Unknown command")
	}
}
