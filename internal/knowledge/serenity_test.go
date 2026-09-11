package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const testFactID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func protocolWriter(t *testing.T, cancellation bool, call func(string, json.RawMessage) (any, bool)) *Serenity {
	return protocolReader(t, cancellation, false, call)
}
func protocolReader(t *testing.T, cancellation, readExtension bool, call func(string, json.RawMessage) (any, bool)) *Serenity {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "protocol-fixture", Version: "1"}, nil)
	names := []string{"remember", "recall"}
	if cancellation {
		names = append(names, "cancel_memory_operation")
	}
	if readExtension {
		names = append(names, "read_memory_fact")
	}
	for _, name := range names {
		schema := map[string]any{"type": "object", "properties": map[string]any{"operation_key": map[string]any{"type": "string"}}}
		server.AddTool(&mcp.Tool{Name: name, InputSchema: schema}, func(_ context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			value, bad := call(name, r.Params.Arguments)
			data, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			return &mcp.CallToolResult{IsError: bad, Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}, nil
		})
	}
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-writer-token" {
			t.Error("missing scoped credential")
			w.WriteHeader(401)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)
	adapter, err := NewSerenity(httpServer.URL, "test-writer-token")
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func TestSerenitySupportedSDKRoundTripAndStablePayload(t *testing.T) {
	var mu sync.Mutex
	var payloads []string
	adapter := protocolWriter(t, true, func(name string, args json.RawMessage) (any, bool) {
		switch name {
		case "remember":
			mu.Lock()
			payloads = append(payloads, string(args))
			mu.Unlock()
			return RememberResult{ID: testFactID, ProtocolVersion: 1, SearchState: "semantic"}, false
		case "cancel_memory_operation":
			return CancelResult{ID: testFactID, ProtocolVersion: 1, OperationKey: "post:1", Canceled: true, Expired: true}, false
		case "recall":
			return map[string]any{"protocol_version": 1, "facts": []any{map[string]any{"fact": "recent facts must not become query results"}}, "results": []any{map[string]any{"slug": "source-" + testFactID, "chunk": "sourced evidence"}, map[string]any{"slug": "page-unapproved", "chunk": "must be ignored"}}}, false
		}
		t.Error("unexpected tool")
		return nil, true
	})
	in := RememberInput{OperationKey: "post:1", Fact: "sourced claim", Provenance: "post:1", ExpiresAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	for range 2 {
		got, err := adapter.Remember(context.Background(), in)
		if err != nil || got.ID != testFactID || got.SearchState != "semantic" {
			t.Fatalf("remember: %+v %v", got, err)
		}
	}
	mu.Lock()
	if len(payloads) != 2 || payloads[0] != payloads[1] || !strings.Contains(payloads[0], "2026-10-01T00:00:00Z") {
		t.Error("retry changed durable payload")
	}
	mu.Unlock()
	canceled, err := adapter.Cancel(context.Background(), in.OperationKey)
	if err != nil || !canceled.Canceled {
		t.Fatalf("cancel: %+v %v", canceled, err)
	}
	hits, err := adapter.Search(context.Background(), "claim")
	if err != nil || len(hits) != 1 || hits[0].ID != testFactID || hits[0].Snippet != "sourced evidence" {
		t.Fatalf("search: %+v %v", hits, err)
	}
}
func TestSerenityOldWriterRefusedBeforeMutation(t *testing.T) {
	adapter := protocolWriter(t, false, func(string, json.RawMessage) (any, bool) { t.Error("old writer was mutated"); return nil, false })
	_, err := adapter.Remember(context.Background(), RememberInput{OperationKey: "post:1", Fact: "claim", Provenance: "test", ExpiresAt: time.Now().Add(time.Hour)})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("got %v", err)
	}
}
func TestSerenityFailureAndCancellationAcknowledgment(t *testing.T) {
	for _, mode := range []string{"conflict", "canceled", "wrong_ack", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			adapter := protocolWriter(t, true, func(string, json.RawMessage) (any, bool) {
				switch mode {
				case "conflict":
					return map[string]any{"error": "operation_conflict", "message": "PRIVATE-CANARY"}, true
				case "canceled":
					return map[string]any{"error": "operation_canceled", "message": "PRIVATE-CANARY"}, true
				case "wrong_ack":
					return CancelResult{OperationKey: "another-key", Canceled: true, ProtocolVersion: 1}, false
				default:
					return map[string]any{"error": "unknown", "message": "PRIVATE-CANARY"}, true
				}
			})
			_, err := adapter.Cancel(context.Background(), "post:1")
			want := ErrUnavailable
			if mode == "conflict" {
				want = ErrConflict
			}
			if mode == "canceled" {
				want = ErrCanceled
			}
			if !errors.Is(err, want) || strings.Contains(err.Error(), "PRIVATE-CANARY") {
				t.Fatalf("unsafe error %v", err)
			}
		})
	}
}
func TestSerenityEndpointAndRedirectContainCredentials(t *testing.T) {
	for _, endpoint := range []string{"http://remote.example/mcp", "http://localhost/mcp", "https://name:secret@example.test/mcp", "https://example.test/mcp?token=secret", "file:///brain"} {
		if _, err := NewSerenity(endpoint, "token"); err == nil {
			t.Fatalf("unsafe endpoint %q", endpoint)
		}
	}
	var received atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Store(true); w.WriteHeader(500) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	adapter, err := NewSerenity(redirect.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = adapter.Cancel(context.Background(), "post:1"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if received.Load() {
		t.Fatal("writer credential followed redirect")
	}
}

func TestSerenityReadExactRequiresExtensionAndValidFactID(t *testing.T) {
	adapter := protocolReader(t, true, false, func(name string, _ json.RawMessage) (any, bool) {
		t.Errorf("older backend without the extension was called: %s", name)
		return nil, true
	})
	if _, err := adapter.ReadExact(context.Background(), testFactID); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("got %v", err)
	}
	adapter = protocolReader(t, true, true, func(name string, _ json.RawMessage) (any, bool) {
		t.Errorf("invalid fact ID reached the backend: %s", name)
		return nil, true
	})
	for _, bad := range []string{"", "not-hex", strings.ToUpper(testFactID), testFactID[:63]} {
		if _, err := adapter.ReadExact(context.Background(), bad); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%q: got %v", bad, err)
		}
	}
}

func TestSerenityReadExactValidatesProtocolBeforeTrustingContent(t *testing.T) {
	otherFact := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	for _, mode := range []string{"wrong_id", "group_visibility", "not_content_untrusted", "wrong_protocol_version", "ok"} {
		t.Run(mode, func(t *testing.T) {
			adapter := protocolReader(t, true, true, func(string, json.RawMessage) (any, bool) {
				out := ReadFactResult{ProtocolVersion: 1, ID: testFactID, Fact: "canonical fact", Visibility: "world", Provenance: "post:1", ContentUntrusted: true}
				switch mode {
				case "wrong_id":
					out.ID = otherFact
				case "group_visibility":
					out.Visibility = "group"
				case "not_content_untrusted":
					out.ContentUntrusted = false
				case "wrong_protocol_version":
					out.ProtocolVersion = 2
				}
				return out, false
			})
			got, err := adapter.ReadExact(context.Background(), testFactID)
			if mode == "ok" {
				if err != nil || got.ID != testFactID || got.Fact != "canonical fact" || !got.ContentUntrusted {
					t.Fatalf("%+v %v", got, err)
				}
				return
			}
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("%s: got %v", mode, err)
			}
		})
	}
}

func TestSerenityReadExactProviderErrorNeverLeaksContent(t *testing.T) {
	adapter := protocolReader(t, true, true, func(string, json.RawMessage) (any, bool) {
		return map[string]any{"error": "internal", "message": "PRIVATE-CANARY"}, true
	})
	_, err := adapter.ReadExact(context.Background(), testFactID)
	if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "PRIVATE-CANARY") {
		t.Fatalf("unsafe error %v", err)
	}
}

func TestSerenityReadExactMalformedOrOversizeOutputRejected(t *testing.T) {
	for _, mode := range []string{"malformed", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			adapter := protocolReader(t, true, true, func(string, json.RawMessage) (any, bool) {
				if mode == "malformed" {
					return map[string]any{"protocol_version": "not-a-number"}, false
				}
				return ReadFactResult{ProtocolVersion: 1, ID: testFactID, Fact: strings.Repeat("a", 1<<20), Visibility: "world", ContentUntrusted: true}, false
			})
			if _, err := adapter.ReadExact(context.Background(), testFactID); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("%s: got %v", mode, err)
			}
		})
	}
}
