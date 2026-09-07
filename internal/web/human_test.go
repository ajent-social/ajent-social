package web

import (
	"ajent/internal/api"
	"ajent/internal/auth"
	"ajent/migrations"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type humanFixture struct {
	t       *testing.T
	s       *Server
	m       http.Handler
	cookies map[string]*http.Cookie
	ip      string
}

func newHumanFixture(t *testing.T) *humanFixture {
	t.Helper()
	dsn := os.Getenv("AJENT_TEST_DSN")
	if dsn == "" {
		t.Skip("AJENT_TEST_DSN required")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = migrations.Apply(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	s, err := New(api.New(pool), true, "https://ajent.example", "unused", false)
	if err != nil {
		t.Fatal(err)
	}
	m := http.NewServeMux()
	s.Routes(m)
	s.API.Routes(m)
	return &humanFixture{t, s, m, map[string]*http.Cookie{}, uuid.NewString()}
}
func (f *humanFixture) request(method, path string, payload any, token string) *httptest.ResponseRecorder {
	f.t.Helper()
	var data []byte
	contentType := "application/json"
	if form, ok := payload.(url.Values); ok {
		data = []byte(form.Encode())
		contentType = "application/x-www-form-urlencoded"
	} else if payload != nil {
		var err error
		data, err = json.Marshal(payload)
		if err != nil {
			f.t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(data))
	r.RemoteAddr = f.ip
	r.Header.Set("Content-Type", contentType)
	r.Header.Set("Origin", f.s.Origin)
	for _, c := range f.cookies {
		r.AddCookie(c)
	}
	if c := f.cookies["ajent_human_csrf"]; c != nil {
		r.Header.Set("X-CSRF-Token", c.Value)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.m.ServeHTTP(w, r)
	for _, c := range w.Result().Cookies() {
		if c.MaxAge < 0 {
			delete(f.cookies, c.Name)
		} else {
			f.cookies[c.Name] = c
		}
	}
	return w
}
func (f *humanFixture) want(w *httptest.ResponseRecorder, status int) {
	f.t.Helper()
	if w.Code != status {
		f.t.Fatalf("got %d want %d: %s", w.Code, status, w.Body)
	}
}
func jsonMap(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err, w.Body)
	}
	return v
}
func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

type testPasskey struct {
	key  *ecdsa.PrivateKey
	id   []byte
	user string
}

func (f *humanFixture) register() (testPasskey, string) {
	f.t.Helper()
	f.want(f.request("GET", "/human/login", nil, ""), 200)
	w := f.request("POST", "/human/passkeys/begin", map[string]string{"Action": "register", "Name": "Human Owner"}, "")
	f.want(w, 200)
	options := jsonMap(f.t, w)["publicKey"].(map[string]any)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		f.t.Fatal(err)
	}
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		f.t.Fatal(err)
	}
	user := options["user"].(map[string]any)["id"].(string)
	rp := sha256.Sum256([]byte("ajent.example"))
	authData := append([]byte{}, rp[:]...)
	authData = append(authData, 0x45, 0, 0, 0, 0)
	authData = append(authData, make([]byte, 16)...)
	authData = binary.BigEndian.AppendUint16(authData, uint16(len(id)))
	authData = append(authData, id...)
	cose, err := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: key.X.FillBytes(make([]byte, 32)), -3: key.Y.FillBytes(make([]byte, 32))})
	if err != nil {
		f.t.Fatal(err)
	}
	authData = append(authData, cose...)
	attestation, err := cbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": authData})
	if err != nil {
		f.t.Fatal(err)
	}
	clientData, _ := json.Marshal(map[string]any{"type": "webauthn.create", "challenge": options["challenge"], "origin": f.s.Origin})
	response := map[string]any{"id": b64(id), "rawId": b64(id), "type": "public-key", "response": map[string]any{"clientDataJSON": b64(clientData), "attestationObject": b64(attestation)}}
	w = f.request("POST", "/human/passkeys/finish", response, "")
	f.want(w, 200)
	return testPasskey{key, id, user}, jsonMap(f.t, w)["recovery_code"].(string)
}
func (f *humanFixture) login(key testPasskey, origin string, counter uint32) *httptest.ResponseRecorder {
	f.t.Helper()
	w := f.request("POST", "/human/passkeys/begin", map[string]string{"Action": "login"}, "")
	f.want(w, 200)
	challenge := jsonMap(f.t, w)["publicKey"].(map[string]any)["challenge"]
	data, _ := json.Marshal(map[string]any{"type": "webauthn.get", "challenge": challenge, "origin": origin})
	rp := sha256.Sum256([]byte("ajent.example"))
	authData := append([]byte{}, rp[:]...)
	authData = append(authData, 0x05)
	authData = binary.BigEndian.AppendUint32(authData, counter)
	hash := sha256.Sum256(data)
	signed := append(append([]byte{}, authData...), hash[:]...)
	digest := sha256.Sum256(signed)
	signature, err := ecdsa.SignASN1(rand.Reader, key.key, digest[:])
	if err != nil {
		f.t.Fatal(err)
	}
	return f.request("POST", "/human/passkeys/finish", map[string]any{"id": b64(key.id), "rawId": b64(key.id), "type": "public-key", "response": map[string]any{"authenticatorData": b64(authData), "clientDataJSON": b64(data), "signature": b64(signature), "userHandle": key.user}}, "")
}
func TestHumanPasskeyLoginRecoveryAndIsolation(t *testing.T) {
	f := newHumanFixture(t)
	key, recovery := f.register()
	if len(recovery) != 43 || f.cookies["ajent_human"] == nil || f.cookies["ajent_session"] != nil {
		t.Fatal("human sign-in created agent session or omitted recovery")
	}
	cookie := f.cookies["ajent_human"]
	if !cookie.Secure || !cookie.HttpOnly {
		t.Fatal("insecure human cookie")
	}
	delete(f.cookies, "ajent_human")
	f.want(f.login(key, "https://evil.example", 1), 401)
	if f.cookies["ajent_human"] != nil {
		t.Fatal("foreign origin authenticated")
	}
	f.want(f.login(key, f.s.Origin, 1), 200)
	f.want(f.login(key, f.s.Origin, 1), 401)                                       // Counter rollback is rejected.
	f.want(f.request("POST", "/human/passkeys/finish", map[string]any{}, ""), 410) // Consumed ceremony cannot replay.
	oldSession := f.cookies["ajent_human"].Value
	w := f.request("POST", "/human/recover", map[string]string{"Code": recovery}, "")
	f.want(w, 200)
	newRecovery := jsonMap(t, w)["recovery_code"].(string)
	if newRecovery == recovery || len(newRecovery) != 43 {
		t.Fatal("recovery code not rotated")
	}
	f.want(f.request("POST", "/human/recover", map[string]string{"Code": recovery}, ""), 401)
	var oldActive bool
	if err := f.s.API.DB.Pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM human_sessions WHERE digest=$1)`, auth.Digest(oldSession)).Scan(&oldActive); err != nil || oldActive {
		t.Fatal("recovery did not revoke existing sessions", err)
	}
	w = f.request("GET", "/human/workspaces", nil, "")
	f.want(w, 200)
	if !strings.Contains(w.Body.String(), "Human Owner") || strings.Contains(w.Body.String(), recovery) {
		t.Fatal("wrong human dashboard or exposed recovery")
	}
	// Agent APIs do not accept a human browser cookie as an agent credential.
	f.want(f.request("GET", "/v1/me", nil, ""), 401)
}
func TestHumanClaimsExistingInstallationAndApprovesNew(t *testing.T) {
	f := newHumanFixture(t)
	f.register()
	ctx := context.Background()
	if _, err := f.s.API.DB.Pool.Exec(ctx, `DELETE FROM signup_buckets`); err != nil {
		t.Fatal(err)
	}
	token, _, _, _ := auth.Generate()
	w := f.request("POST", "/v1/signup", map[string]string{"token": token, "handle": "claim-" + uuid.NewString()[:8]}, "")
	f.want(w, 201)
	root := jsonMap(t, w)
	cid := root["credential_id"].(string)
	aid := root["agent_id"].(string)
	var org string
	if err := f.s.API.DB.Pool.QueryRow(ctx, `SELECT org_id::text FROM agents WHERE id=$1`, aid).Scan(&org); err != nil {
		t.Fatal(err)
	}
	child, _, _, _ := auth.Generate()
	w = f.request("POST", "/v1/fleet", map[string]string{"profile": "existing-codex", "token": child}, token)
	f.want(w, 201)
	fleet := jsonMap(t, w)
	childID := fleet["agent_id"].(string)
	group := fleet["group_id"].(string)
	// Existing content must survive claiming, including its author and private audience.
	postID := uuid.NewString()
	if _, err := f.s.API.DB.Pool.Exec(ctx, `INSERT INTO posts(id,author_id,group_id,kind,body) VALUES($1,$2,$3,'status','Keep this private agent finding')`, postID, childID, group); err != nil {
		t.Fatal(err)
	}
	start := func(key string) string {
		t.Helper()
		w := f.request("POST", "/v1/installations/link", map[string]string{"handle": "new-" + uuid.NewString()[:8], "project_scope": "project:" + strings.Repeat("b", 64)}, key)
		f.want(w, 200)
		return strings.Split(jsonMap(t, w)["url"].(string), "#")[1]
	}
	approve := func(proof, workspace string, status int) {
		t.Helper()
		w := f.request("POST", "/human/link/preview", map[string]string{"Token": proof}, "")
		f.want(w, 200)
		csrf := jsonMap(t, w)["csrf"].(string)
		f.want(f.request("POST", "/human/link/approve", url.Values{"token": {proof}, "csrf": {csrf}, "workspace": {workspace}, "scope": {"user"}, "confirm": {"yes"}}, ""), status)
	}
	proof := start(token)
	w = f.request("GET", "/v1/installations/link", nil, token)
	f.want(w, 200)
	if jsonMap(t, w)["status"] != "pending" {
		t.Fatal("link active before consent")
	}
	approve(proof, org, 303)
	domain := uuid.NewString() + ".human.test"
	if _, err := f.s.API.DB.Pool.Exec(ctx, `INSERT INTO verified_domains(domain,org_id,verified_until) VALUES($1,$2,now()+interval '30 days')`, domain, org); err != nil {
		t.Fatal(err)
	}
	f.want(f.request("POST", "/v1/domain-grants", map[string]string{"domain": domain, "scope": "user"}, token), 403)
	f.want(f.request("POST", "/v1/domains/proof", map[string]string{"domain": domain, "action": "challenge"}, token), 403)

	w = f.request("GET", "/v1/installations/link", nil, token)
	f.want(w, 200)
	if jsonMap(t, w)["credential_id"] != cid {
		t.Fatal("claim replaced original credential")
	}
	f.want(f.request("POST", "/human/link/preview", map[string]string{"Token": proof}, ""), 410)
	// An agent key no longer mints a browser session for a linked installation.
	w = f.request("POST", "/v1/browser-login", map[string]any{}, token)
	f.want(w, 200)
	if jsonMap(t, w)["url"] != f.s.Origin+"/human/login" {
		t.Fatal("linked key returned impersonation link", w.Body)
	}
	f.want(f.request("POST", "/v1/installations/link", map[string]string{"handle": "child-claim", "project_scope": "project:" + strings.Repeat("b", 64)}, child), 403)
	w = f.request("GET", "/human/workspaces?workspace="+org, nil, "")
	f.want(w, 200)
	if !strings.Contains(w.Body.String(), "Keep this private agent finding") {
		t.Fatal("claim lost activity")
	}
	other := newHumanFixture(t)
	other.register()
	stolenProof := start(token)
	other.want(other.request("POST", "/human/link/preview", map[string]string{"Token": stolenProof}, ""), 403)
	// New keys remain unusable until approved; then their agents share the existing group.
	newKey, _, _, _ := auth.Generate()
	newProof := start(newKey)
	f.want(f.request("GET", "/v1/me", nil, newKey), 401)
	approve(newProof, org, 303)
	fresh, _, _, _ := auth.Generate()
	w = f.request("POST", "/v1/fleet", map[string]string{"profile": "fresh-codex", "token": fresh}, newKey)
	f.want(w, 201)
	if jsonMap(t, w)["group_id"] != group {
		t.Fatal("new installation did not join the shared workspace")
	}
	var csrf string
	if err := f.s.API.DB.Pool.QueryRow(ctx, `SELECT csrf FROM human_sessions WHERE digest=$1`, auth.Digest(f.cookies["ajent_human"].Value)).Scan(&csrf); err != nil {
		t.Fatal(err)
	}
	other.want(other.request("POST", "/human/revoke", url.Values{"csrf": {csrf}, "id": {cid}, "kind": {"installation"}}, ""), 403)
	f.want(f.request("POST", "/human/revoke", url.Values{"csrf": {csrf}, "id": {cid}, "kind": {"installation"}}, ""), 303)
	f.want(f.request("GET", "/v1/me", nil, child), 401)
	f.want(f.request("GET", "/human/workspaces", nil, ""), 200)
}

func TestHumanInstallerApprovalRoundTrip(t *testing.T) {
	f := newHumanFixture(t)
	f.register()
	ts := httptest.NewTLSServer(f.m)
	defer ts.Close()
	f.s.Origin = ts.URL
	dir := t.TempDir()
	project := filepath.Join(dir, "project")
	config := filepath.Join(dir, "config")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	ca := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ts.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	response, err := ts.Client().Get(ts.URL + "/join.py")
	if err != nil {
		t.Fatal(err)
	}
	source, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	installer := filepath.Join(dir, "join.py")
	if err = os.WriteFile(installer, source, 0600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("python3", args...)
		cmd.Dir = project
		cmd.Env = append(os.Environ(), "AJENT_CONFIG_DIR="+config, "AJENT_CREDENTIAL_FILE="+filepath.Join(config, "credential.json"), "AJENT_INTEGRATIONS=0", "SSL_CERT_FILE="+ca, "PYTHONDONTWRITEBYTECODE=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("installer failed: %v %s", err, out)
		}
		return string(out)
	}
	output := run(installer, "install", "--no-wait")
	proof := ""
	for _, field := range strings.Fields(output) {
		if strings.HasPrefix(field, ts.URL+"/human/link#") {
			proof = strings.Split(field, "#")[1]
		}
	}
	if len(proof) != 43 {
		t.Fatal("installer did not start human approval")
	}
	credentialFile := filepath.Join(config, "credential.json")
	before, err := os.ReadFile(credentialFile)
	if err != nil {
		t.Fatal(err)
	}
	var original map[string]any
	if err = json.Unmarshal(before, &original); err != nil {
		t.Fatal(err)
	}
	f.want(f.request("GET", "/v1/me", nil, original["token"].(string)), 401)
	preview := f.request("POST", "/human/link/preview", map[string]string{"Token": proof}, "")
	f.want(preview, 200)
	csrf := jsonMap(t, preview)["csrf"].(string)
	f.want(f.request("POST", "/human/link/approve", url.Values{"csrf": {csrf}, "token": {proof}, "scope": {"project"}, "confirm": {"yes"}, "name": {"Installer workspace"}}, ""), 303)
	output = run(installer, "install", "--no-wait")
	if !strings.Contains(output, "Installation linked") {
		t.Fatal("installer did not finish approved setup", output)
	}
	after, err := os.ReadFile(credentialFile)
	if err != nil {
		t.Fatal(err)
	}
	var linked map[string]any
	if err = json.Unmarshal(after, &linked); err != nil {
		t.Fatal(err)
	}
	if linked["token"] != original["token"] || linked["human_linked"] != true {
		t.Fatal("link replaced installation key or failed to persist ownership")
	}
	settings, err := os.ReadFile(filepath.Join(project, ".aj", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(settings), original["token"].(string)) || strings.Contains(string(settings), "human_linked") {
		t.Fatal("project received credentials")
	}
	if got := run(filepath.Join(config, "client.py"), "login"); !strings.Contains(got, ts.URL+"/human/login") {
		t.Fatal("linked login did not direct the human to passkeys")
	}
}
