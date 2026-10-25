package web

import (
	"html/template"
	"net/http"

	"github.com/google/uuid"
)

type mcpConnection struct{ ID, Client, Agent, Workspace, Access, Expires string }

var mcpConnectionsTemplate = template.Must(template.New("connections").Parse(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/static/styles.css"><link rel="stylesheet" href="/static/human.css"><title>Ajent connections</title><body><main class="docs prose"><h1>Connections</h1><p>These applications can act through the listed agent. Revoking a connection leaves the agent and its other connections intact.</p>{{range .Connections}}<section><h2>{{.Client}}</h2><p>Agent: {{.Agent}} · Workspace: {{.Workspace}}</p><p>Permissions: {{.Access}} · Expires: {{.Expires}}</p><form method="post" action="/human/connections/revoke"><input type="hidden" name="csrf" value="{{$.CSRF}}"><input type="hidden" name="connection" value="{{.ID}}"><button>Revoke connection</button></form></section>{{else}}<p>No active connections.</p>{{end}}<p><a href="/human/workspaces">Back to workspaces</a></p></main></body></html>`))

func (s *Server) mcpConnections(w http.ResponseWriter, r *http.Request) {
	h := s.human(r)
	if h == nil {
		http.Redirect(w, r, "/human/login", 303)
		return
	}
	rows, e := s.API.DB.Pool.Query(r.Context(), `SELECT b.credential_id::text,b.client_id,a.handle,o.name,array_to_string(c.scopes,', '),to_char(c.expires_at AT TIME ZONE 'UTC','YYYY-MM-DD HH24:MI UTC') FROM mcp_oauth_bindings b JOIN credentials c ON c.id=b.credential_id JOIN agents a ON a.id=c.agent_id JOIN organizations o ON o.id=a.org_id WHERE b.human_id=$1 AND c.revoked_at IS NULL AND c.expires_at>now() AND a.status='active' AND o.status='active' AND o.human_owner_id=b.human_id AND EXISTS(SELECT 1 FROM mcp_oauth_records g WHERE g.kind='grants' AND g.record->>'binding'=c.id::text AND g.expires_at>now() AND NOT (g.record ? 'revoked_at')) ORDER BY b.created_at DESC LIMIT 100`, h.ID)
	if e != nil {
		http.Error(w, "Connections unavailable", 503)
		return
	}
	var list []mcpConnection
	for rows.Next() {
		var v mcpConnection
		if e = rows.Scan(&v.ID, &v.Client, &v.Agent, &v.Workspace, &v.Access, &v.Expires); e != nil {
			break
		}
		list = append(list, v)
	}
	rowErr := rows.Err()
	rows.Close()
	if e != nil || rowErr != nil {
		http.Error(w, "Connections unavailable", 503)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	mcpConnectionsTemplate.Execute(w, struct {
		CSRF        string
		Connections []mcpConnection
	}{h.CSRF, list})
}
func (s *Server) mcpDisconnect(w http.ResponseWriter, r *http.Request) {
	h := s.humanAction(w, r)
	if h == nil {
		return
	}
	id := r.PostForm.Get("connection")
	if _, e := uuid.Parse(id); e != nil {
		http.Error(w, "Connection unavailable", 404)
		return
	}
	tx, e := s.API.DB.Pool.Begin(r.Context())
	if e != nil {
		http.Error(w, "Connections unavailable", 503)
		return
	}
	defer tx.Rollback(r.Context())
	if _, e = tx.Exec(r.Context(), "SELECT pg_advisory_xact_lock(762185)"); e != nil {
		http.Error(w, "Connections unavailable", 503)
		return
	}
	if !mcpSessionLive(r.Context(), tx, r, h.ID) {
		http.Error(w, "Sign in again to continue.", 401)
		return
	}
	tag, e := tx.Exec(r.Context(), `UPDATE credentials c SET revoked_at=coalesce(c.revoked_at,now()) FROM mcp_oauth_bindings b WHERE b.credential_id=c.id AND b.human_id=$1 AND c.id=$2`, h.ID, id)
	if e != nil {
		http.Error(w, "Connections unavailable", 503)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "Connection unavailable", 404)
		return
	}
	_, e = tx.Exec(r.Context(), `UPDATE mcp_oauth_records SET record=jsonb_set(record,'{revoked_at}',to_jsonb(now())) WHERE kind='grants' AND record->>'binding'=$1 AND record->>'subject'=$2`, id, h.ID)
	if e == nil {
		_, e = tx.Exec(r.Context(), `INSERT INTO human_audit(human_id,action,object_id) VALUES($1,'disconnect_mcp',$2)`, h.ID, id)
	}
	if e != nil || tx.Commit(r.Context()) != nil {
		http.Error(w, "Connections unavailable", 503)
		return
	}
	http.Redirect(w, r, "/human/connections", 303)
}
