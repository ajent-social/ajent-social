// Package knowledge implements the controlled supported-protocol boundary for
// deliberate finding ingestion. It owns no canonical brain files or writer queue.
package knowledge

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	ErrUnavailable = errors.New("knowledge writer unavailable; outcome may require recovery")
	ErrUnsupported = errors.New("knowledge writer lacks required operation capabilities")
	ErrConflict    = errors.New("knowledge operation conflicts with prior content")
	ErrCanceled    = errors.New("knowledge operation is canceled")
)

// Serenity is configured only by the server operator. Callers cannot select an
// endpoint, filesystem path, credential, model or tool. Each call uses a bounded
// SDK session with transport retries disabled; durable retry is the gateway's job.
type Serenity struct {
	endpoint, token string
	client          *http.Client
}

type RememberInput struct {
	OperationKey string
	Fact         string
	Provenance   string
	ExpiresAt    time.Time
}
type RememberResult struct {
	ID              string `json:"id"`
	SearchState     string `json:"search_state"`
	Expired         bool   `json:"expired"`
	ProtocolVersion int    `json:"protocol_version"`
}
type CancelResult struct {
	ID              string `json:"id"`
	OperationKey    string `json:"operation_key"`
	Canceled        bool   `json:"canceled"`
	Expired         bool   `json:"expired"`
	ProtocolVersion int    `json:"protocol_version"`
}
type SearchHit struct{ ID, Snippet string }

func NewSerenity(endpoint, token string) (*Serenity, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.TrimSpace(token) == "" || strings.ContainsAny(token, "\r\n") {
		return nil, ErrUnsupported
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && ip != nil && ip.IsLoopback()) {
		return nil, ErrUnsupported
	}
	// Do not send the private writer token through environment proxies or redirects.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Serenity{endpoint: endpoint, token: token, client: &http.Client{Transport: &writerTransport{base: transport, endpoint: endpoint, token: token}, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

type writerTransport struct {
	base            http.RoundTripper
	endpoint, token string
}

func (t *writerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.String() != t.endpoint {
		return nil, ErrUnavailable
	}
	copy := r.Clone(r.Context())
	copy.Header.Set("Authorization", "Bearer "+t.token)
	response, err := t.base.RoundTrip(copy)
	if err != nil {
		return nil, ErrUnavailable
	}
	// Bound any response before the SDK parses it. Stream responses are disabled
	// for this request/response-only integration.
	response.Body = &limitedBody{ReadCloser: response.Body, remaining: 1 << 20}
	return response, nil
}

type limitedBody struct {
	io.ReadCloser
	remaining int64
}

func (b *limitedBody) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		return 0, ErrUnavailable
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)
	return n, err
}

func (s *Serenity) call(ctx context.Context, name string, args any, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "ajent-knowledge-gateway", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: s.endpoint, HTTPClient: s.client, MaxRetries: -1, DisableStandaloneSSE: true}, nil)
	if err != nil {
		return ErrUnavailable
	}
	defer session.Close()
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		return ErrUnavailable
	}
	keyed, cancellable := false, false
	for _, tool := range listed.Tools {
		if tool.Name == "cancel_memory_operation" {
			cancellable = true
		}
		if tool.Name == "remember" {
			encoded, e := json.Marshal(tool.InputSchema)
			if e != nil {
				return ErrUnsupported
			}
			var schema struct {
				Properties map[string]json.RawMessage `json:"properties"`
			}
			if json.Unmarshal(encoded, &schema) == nil {
				_, keyed = schema.Properties["operation_key"]
			}
		}
	}
	if !keyed || !cancellable {
		return ErrUnsupported
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return ErrUnavailable
	}
	if len(result.Content) != 1 {
		return ErrUnavailable
	}
	content, ok := result.Content[0].(*mcp.TextContent)
	if !ok || len(content.Text) > 1<<20 {
		return ErrUnavailable
	}
	if result.IsError {
		var failure struct {
			Error string `json:"error"`
		}
		if json.Unmarshal([]byte(content.Text), &failure) == nil {
			switch failure.Error {
			case "operation_conflict":
				return ErrConflict
			case "operation_canceled":
				return ErrCanceled
			}
		}
		return ErrUnavailable
	}
	if json.Unmarshal([]byte(content.Text), out) != nil {
		return ErrUnavailable
	}
	return nil
}

func validFactID(id string) bool {
	decoded, err := hex.DecodeString(id)
	return err == nil && len(decoded) == 32 && strings.ToLower(id) == id
}
func (s *Serenity) Remember(ctx context.Context, in RememberInput) (RememberResult, error) {
	var out RememberResult
	if in.ExpiresAt.IsZero() || in.OperationKey == "" || in.Fact == "" || in.Provenance == "" {
		return out, ErrUnsupported
	}
	err := s.call(ctx, "remember", map[string]any{"operation_key": in.OperationKey, "fact": in.Fact, "provenance": in.Provenance, "ttl": in.ExpiresAt.UTC().Format(time.RFC3339Nano), "visibility": "world", "kind": "fact"}, &out)
	if err != nil {
		return RememberResult{}, err
	}
	if out.ProtocolVersion != 1 || !validFactID(out.ID) {
		return RememberResult{}, ErrUnavailable
	}
	switch out.SearchState {
	case "unavailable", "not_eligible", "lexical", "semantic":
	default:
		out.SearchState = "unavailable"
	}
	return out, nil
}
func (s *Serenity) Cancel(ctx context.Context, key string) (CancelResult, error) {
	var out CancelResult
	if key == "" {
		return out, ErrUnsupported
	}
	err := s.call(ctx, "cancel_memory_operation", map[string]any{"operation_key": key, "reason": "Ajent source publication withdrawn"}, &out)
	if err != nil {
		return CancelResult{}, err
	}
	if out.ProtocolVersion != 1 || out.OperationKey != key || !out.Canceled || (out.ID != "" && !validFactID(out.ID)) {
		return CancelResult{}, ErrUnavailable
	}
	return out, nil
}
func (s *Serenity) Search(ctx context.Context, query string) ([]SearchHit, error) {
	var out struct {
		ProtocolVersion int `json:"protocol_version"`
		Results         []struct {
			Slug  string `json:"slug"`
			Chunk string `json:"chunk"`
		} `json:"results"`
	}
	if len(query) > 4096 {
		return nil, ErrUnsupported
	}
	if err := s.call(ctx, "recall", map[string]any{"query": query, "limit": 20, "budget_tokens": 8000}, &out); err != nil {
		return nil, err
	}
	if out.ProtocolVersion != 1 {
		return nil, ErrUnavailable
	}
	hits := make([]SearchHit, 0, len(out.Results))
	for _, r := range out.Results {
		id, ok := strings.CutPrefix(r.Slug, "source-")
		if !ok || !validFactID(id) {
			continue
		}
		hits = append(hits, SearchHit{ID: id, Snippet: r.Chunk})
	}
	return hits, nil
}
