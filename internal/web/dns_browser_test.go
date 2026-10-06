//go:build dnsbrowser

package web

import (
	"ajent/internal/api"
	"ajent/internal/auth"
	"ajent/internal/bootstrap"
	"ajent/migrations"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestBrowserMatchingDNSProof runs only with the dnsbrowser build tag and must
// run alone: it replaces the process-local resolver. It never changes host DNS
// or forwards queries upstream. Human authentication is a seeded session, not
// passkey proof.
func TestBrowserMatchingDNSProof(t *testing.T) {
	dsn := os.Getenv("AJENT_TEST_DSN")
	if dsn == "" {
		t.Fatal("AJENT_TEST_DSN must select a fresh disposable loopback database")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.Host != "127.0.0.1" || !strings.HasPrefix(cfg.ConnConfig.Database, "ajent_dns_browser_") {
		t.Fatal("requires 127.0.0.1 and disposable ajent_dns_browser_ database")
	}
	ctx := context.Background()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var existing int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM organizations").Scan(&existing); err != nil || existing != 0 {
		t.Fatalf("fresh database required: count=%d error=%v", existing, err)
	}
	must := func(q string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	setup := "synthetic-dns-browser-owner-key-only"
	api.SetSetupToken(setup)
	t.Cleanup(func() { api.SetSetupToken("") })
	owner, err := bootstrap.Ensure(ctx, pool, "Synthetic DNS owner")
	if err != nil {
		t.Fatal(err)
	}
	var ownerOrg string
	if err = pool.QueryRow(ctx, "SELECT org_id::text FROM agents WHERE id=$1", owner).Scan(&ownerOrg); err != nil {
		t.Fatal(err)
	}
	org, agent, cred, human, humanOrg := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	key, lookup, digest, err := auth.Generate()
	if err != nil {
		t.Fatal(err)
	}
	must("INSERT INTO organizations(id,name,namespace) VALUES($1::uuid,'Synthetic original',$1::text)", org)
	must("INSERT INTO agents(id,org_id,handle) VALUES($1,$2,'synthetic-original')", agent, org)
	must("INSERT INTO credentials(id,agent_id,lookup_id,secret_digest,scopes) VALUES($1,$2,$3,$4,ARRAY['network:read'])", cred, agent, lookup, digest)
	must("INSERT INTO public_enrollments(credential_id) VALUES($1)", cred)
	must("INSERT INTO humans(id,name) VALUES($1,'Synthetic DNS human')", human)
	must("INSERT INTO organizations(id,name,namespace,human_owner_id) VALUES($1::uuid,'Synthetic human workspace',$1::text,$2)", humanOrg, human)
	raw := uuid.NewString()
	must("INSERT INTO human_sessions(digest,human_id,csrf) VALUES($1,$2,$3)", auth.Digest(raw), human, uuid.NewString())
	dns := &browserTXTResolver{records: make(map[string]string)}
	old := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: dns.dial}
	t.Cleanup(func() { net.DefaultResolver = old; dns.wg.Wait() })
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + listener.Addr().String()
	s, err := New(api.New(pool), false, origin, setup, false)
	if err != nil {
		listener.Close()
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.Routes(mux)
	s.API.Routes(mux)
	statuses := make(chan int, 16)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && (r.URL.Path == "/actions/verification" || r.URL.Path == "/human/domain") {
			capture := &browserDNSStatus{ResponseWriter: w}
			mux.ServeHTTP(capture, r)
			statuses <- capture.status
			return
		}
		mux.ServeHTTP(w, r)
	}))
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	for _, tc := range []struct{ role, org, key, path string }{{"owner", ownerOrg, setup, "/app/verification"}, {"original", org, key, "/app/verification"}, {"human", humanOrg, "", "/human/workspaces?workspace=" + humanOrg}} {
		t.Run(tc.role, func(t *testing.T) {
			session := "ajent-dns-" + tc.role + "-" + uuid.NewString()[:8]
			run := func(args ...string) string {
				t.Helper()
				cctx, cancel := context.WithTimeout(ctx, 40*time.Second)
				defer cancel()
				cmd := exec.CommandContext(cctx, "agent-browser", append([]string{"--session", session}, args...)...)
				out, e := cmd.CombinedOutput()
				if e != nil {
					t.Fatalf("browser %s failed: %v: %s", args[0], e, out)
				}
				return string(out)
			}
			t.Cleanup(func() {
				cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
				defer cancel()
				if out, e := exec.CommandContext(cctx, "agent-browser", "--session", session, "close").CombinedOutput(); e != nil {
					t.Errorf("browser cleanup: %v %s", e, out)
				}
			})
			if tc.role == "human" {
				run("cookies", "set", "ajent_human", raw, "--url", origin, "--httpOnly", "--sameSite", "Lax")
			} else {
				run("open", origin+"/login")
				run("fill", "#token", tc.key)
				run("click", "form[action='/login'] button")
				run("wait", "--url", "**/app")
			}
			run("open", origin+tc.path)
			domain := "synthetic-" + tc.role + "-" + uuid.NewString() + ".invalid"
			run("fill", "input[name=domain]:not([type=hidden])", domain)
			run("click", "button[name=action][value=challenge]")
			wantStatus := func(want int) {
				t.Helper()
				select {
				case got := <-statuses:
					if got != want {
						t.Fatalf("action status %d want %d", got, want)
					}
				case <-time.After(10 * time.Second):
					t.Fatal("action not observed")
				}
			}
			wantStatus(303)
			run("wait", "--text", "ajent-verification=")
			body := run("get", "text", "body")
			match := regexp.MustCompile(`ajent-verification=([A-Za-z0-9_-]{43})`).FindStringSubmatch(body)
			if len(match) != 2 || !strings.Contains(body, "_ajent-verification."+domain) {
				t.Fatal("rendered challenge missing")
			}
			proof := match[1]
			var stored string
			if e := pool.QueryRow(ctx, "SELECT token FROM domain_challenges WHERE org_id=$1 AND domain=$2", tc.org, domain).Scan(&stored); e != nil || stored != proof {
				t.Fatalf("UI challenge mismatch: %v", e)
			}
			question := "_ajent-verification." + domain
			dns.set(question, "ajent-verification=wrong-token")
			run("click", "button[name=action][value=check]")
			wantStatus(409)
			var count int
			if e := pool.QueryRow(ctx, "SELECT count(*) FROM verified_domains WHERE domain=$1", domain).Scan(&count); e != nil || count != 0 {
				t.Fatalf("wrong token mutated verification: count=%d err=%v", count, e)
			}
			recovery := "DNS proof not found. Add the exact TXT record and retry"
			if tc.role == "human" {
				recovery = "Domain change failed. Check the DNS proof and retry."
			}
			run("wait", "--text", recovery)
			wrongBody := run("get", "text", "body")
			if !strings.Contains(wrongBody, recovery) {
				t.Fatal("wrong-token recovery missing")
			}
			dns.assertQuery(t, question, "ajent-verification=wrong-token")
			t.Log("PASS wrong TXT: actual form409, recovery text, zero verified rows, exact resolver TXT question")
			run("open", origin+tc.path)
			dns.set(question, "ajent-verification="+proof)
			run("click", "button[name=action][value=check]")
			wantStatus(303)
			label := "Verified domain until"
			if tc.role == "human" {
				label = "Verified until"
			}
			run("wait", "--text", label)
			var persisted bool
			if e := pool.QueryRow(ctx, "SELECT org_id=$2 AND proof_token=$3 AND verified_until>now()+interval '29 days' AND next_check_at>now() FROM verified_domains WHERE domain=$1", domain, tc.org, proof).Scan(&persisted); e != nil || !persisted {
				t.Fatalf("matching token not persisted: %v %v", persisted, e)
			}
			dns.assertQuery(t, question, "ajent-verification="+proof)
			run("open", origin+tc.path)
			run("wait", "--text", label)
			if !strings.Contains(run("get", "text", "body"), domain) {
				t.Fatal("verified domain missing on reload")
			}
			t.Log("PASS matching TXT: actual form303, verified UI survives reload, exact org/proof/30-day validity/next check persisted, exact resolver TXT question")
		})
	}
}

type browserDNSStatus struct {
	http.ResponseWriter
	status int
}

func (w *browserDNSStatus) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }

type browserTXTResolver struct {
	mu        sync.Mutex
	wg        sync.WaitGroup
	records   map[string]string
	questions []string
}

func (d *browserTXTResolver) set(name, value string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.records[name] = value
}
func (d *browserTXTResolver) assertQuery(t *testing.T, name, value string) {
	t.Helper()
	d.mu.Lock()
	defer d.mu.Unlock()
	want := name + " TXT " + value
	for _, q := range d.questions {
		if q == want {
			t.Logf("DNS query observed: %s TXT (expected response matched)", name)
			return
		}
	}
	t.Fatalf("missing expected DNS question: %s", name)
}

// net.Resolver accepts any net.Conn. net.Pipe is a stream, so Go uses DNS
// length framing even when Dial receives network=udp (dnsclient_unix.go).
// This keeps even the synthetic DNS traffic entirely in-process.
func (d *browserTXTResolver) dial(ctx context.Context, network, address string) (net.Conn, error) {
	client, server := net.Pipe()
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer server.Close()
		if err := server.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
			return
		}
		var size [2]byte
		if _, err := io.ReadFull(server, size[:]); err != nil {
			return
		}
		n := int(binary.BigEndian.Uint16(size[:]))
		if n < 17 || n > 4096 {
			return
		}
		query := make([]byte, n)
		if _, err := io.ReadFull(server, query); err != nil {
			return
		}
		reply, err := d.answer(query)
		if err != nil {
			return
		}
		binary.BigEndian.PutUint16(size[:], uint16(len(reply)))
		if _, err = server.Write(append(size[:], reply...)); err != nil {
			return
		}
	}()
	return client, nil
}
func (d *browserTXTResolver) answer(q []byte) ([]byte, error) {
	if binary.BigEndian.Uint16(q[4:6]) != 1 {
		return nil, fmt.Errorf("expected one question")
	}
	at := 12
	var labels []string
	for {
		if at >= len(q) {
			return nil, io.ErrUnexpectedEOF
		}
		n := int(q[at])
		at++
		if n == 0 {
			break
		}
		if n > 63 || at+n > len(q) {
			return nil, fmt.Errorf("invalid label")
		}
		labels = append(labels, string(q[at:at+n]))
		at += n
	}
	if at+4 > len(q) || binary.BigEndian.Uint16(q[at:at+2]) != 16 || binary.BigEndian.Uint16(q[at+2:at+4]) != 1 {
		return nil, fmt.Errorf("expected IN TXT")
	}
	at += 4
	name := strings.Join(labels, ".")
	d.mu.Lock()
	value, ok := d.records[name]
	if ok {
		d.questions = append(d.questions, name+" TXT "+value)
	}
	d.mu.Unlock()
	if !ok || len(value) > 254 {
		return nil, fmt.Errorf("query not allowlisted")
	}
	reply := make([]byte, 12)
	copy(reply[:2], q[:2])
	binary.BigEndian.PutUint16(reply[2:4], 0x8180)
	binary.BigEndian.PutUint16(reply[4:6], 1)
	binary.BigEndian.PutUint16(reply[6:8], 1)
	reply = append(reply, q[12:at]...)
	reply = append(reply, 0xc0, 0x0c, 0, 16, 0, 1, 0, 0, 0, 0, 0, byte(len(value)+1), byte(len(value)))
	reply = append(reply, value...)
	return reply, nil
}
