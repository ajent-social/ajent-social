package api

import (
	"ajent/internal/events"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"testing"
	"time"
)

func (f *fixture) questionPost(token, kind, parent string) string {
	f.t.Helper()
	body, _ := json.Marshal(PostRequest{Kind: kind, Title: "Build question", Body: "How can this build use a checked dependency?", GroupID: f.g, ParentID: parent})
	w := f.request("POST", "/v1/posts", token, string(body), uuid.NewString())
	if w.Code != 201 {
		f.t.Fatalf("question post: %d %s", w.Code, w.Body)
	}
	var receipt struct{ ID string }
	if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
		f.t.Fatal(err)
	}
	return receipt.ID
}
func (f *fixture) readPost(token, id string) Post {
	f.t.Helper()
	w := f.request("GET", "/v1/posts/"+id, token, "", "")
	if w.Code != 200 {
		f.t.Fatalf("read: %d %s", w.Code, w.Body)
	}
	var p Post
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		f.t.Fatal(err)
	}
	return p
}
func TestQuestionAcceptanceAndChecksAreSeparate(t *testing.T) {
	f := setup(t)
	if _, err := f.s.DB.Pool.Exec(context.Background(), `INSERT INTO group_members(group_id,agent_id) VALUES($1,$2)`, f.g, f.b); err != nil {
		t.Fatal(err)
	}
	q := f.questionPost(f.ta, "question", "")
	a := f.questionPost(f.tb, "answer", q)
	state := f.readPost(f.ta, q).Question
	if state == nil || state.State != "answered" || state.Version != 1 {
		t.Fatalf("initial state: %+v", state)
	}
	body := fmt.Sprintf(`{"version":1,"answer_id":%q}`, a)
	path := "/v1/questions/" + q + "/resolution"
	key := uuid.NewString()
	if w := f.request("PUT", path, f.tb, body, key); w.Code != 404 {
		t.Fatalf("non-asker accepted: %d %s", w.Code, w.Body)
	}
	for i := 0; i < 2; i++ {
		if w := f.request("PUT", path, f.ta, body, key); w.Code != 200 {
			t.Fatalf("resolve/replay: %d %s", w.Code, w.Body)
		}
	}
	state = f.readPost(f.ta, q).Question
	if state.State != "resolved" || state.AcceptedAnswerID == nil || *state.AcceptedAnswerID != a || state.Version != 2 {
		t.Fatalf("resolution: %+v", state)
	}
	check := AnswerCheckRequest{Environment: "Go 1.25; isolated fixture", Result: "worked", Evidence: "Ran the requested build and observed the expected artifact.", CheckedAt: time.Now().UTC()}
	raw, _ := json.Marshal(check)
	w := f.request("POST", "/v1/answers/"+a+"/validations", f.ta, string(raw), uuid.NewString())
	if w.Code != 201 {
		t.Fatalf("check: %d %s", w.Code, w.Body)
	}
	var receipt struct{ ID string }
	if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	checks := f.readPost(f.ta, a).Checks
	if checks == nil || checks.Total != 1 || checks.Worked != 1 {
		t.Fatalf("check summary: %+v", checks)
	}
	check.SupersedesID = receipt.ID
	check.Result = "failed"
	check.Evidence = "Correction: the second build failed with the same compiler error."
	raw, _ = json.Marshal(check)
	if w = f.request("POST", "/v1/answers/"+a+"/validations", f.tb, string(raw), uuid.NewString()); w.Code != 409 {
		t.Fatalf("another checker superseded evidence: %d %s", w.Code, w.Body)
	}
	if w = f.request("POST", "/v1/answers/"+a+"/validations", f.ta, string(raw), uuid.NewString()); w.Code != 201 {
		t.Fatalf("correction: %d %s", w.Code, w.Body)
	}
	checks = f.readPost(f.ta, a).Checks
	if checks.Total != 1 || checks.Worked != 0 || checks.Failed != 1 {
		t.Fatalf("superseded evidence counted: %+v", checks)
	}
	if f.readPost(f.ta, q).Question.State != "resolved" {
		t.Fatal("negative check silently overruled asker acceptance")
	}
	if w = f.request("DELETE", "/v1/posts/"+a, f.tb, "", ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	state = f.readPost(f.ta, q).Question
	if state.State != "open" || state.AcceptedAnswerID != nil {
		t.Fatalf("deleted acceptance remained effective: %+v", state)
	}
	if w = f.request("PUT", path, f.ta, body, key); w.Code != 200 {
		t.Fatal("content-free replay failed", w.Code, w.Body)
	}
	if f.readPost(f.ta, q).Question.State != "open" {
		t.Fatal("replay resurrected deleted acceptance")
	}
}

func TestQuestionWrongRootVersionAndLegacyEvidence(t *testing.T) {
	f := setup(t)
	q := f.questionPost(f.ta, "question", "")
	other := f.questionPost(f.ta, "question", "")
	wrong := f.questionPost(f.ta, "answer", other)
	path := "/v1/questions/" + q + "/resolution"
	for _, tc := range []struct {
		body string
		code int
	}{
		{fmt.Sprintf(`{"version":1,"answer_id":%q}`, wrong), 400},
		{`{"version":1}`, 400}, {`{"version":9,"answer_id":null}`, 409},
	} {
		w := f.request("PUT", path, f.ta, tc.body, uuid.NewString())
		if w.Code != tc.code {
			t.Fatalf("%s: %d %s", tc.body, w.Code, w.Body)
		}
	}
	answer := f.questionPost(f.ta, "answer", q)
	f.questionPost(f.ta, "validation", answer)
	if checks := f.readPost(f.ta, answer).Checks; checks == nil || checks.Total != 0 {
		t.Fatalf("legacy claim became checked evidence: %+v", checks)
	}
	for _, result := range []string{"worked", "partially_worked", "failed", "unable_to_verify"} {
		t.Run(result, func(t *testing.T) {
			raw, _ := json.Marshal(AnswerCheckRequest{Environment: "test environment", Result: result, Evidence: "Observed outcome or reason unable to check.", CheckedAt: time.Now().UTC()})
			w := f.request("POST", "/v1/answers/"+answer+"/validations", f.ta, string(raw), uuid.NewString())
			if w.Code != 201 {
				t.Fatalf("%s: %d %s", result, w.Code, w.Body)
			}
		})
	}
	checks := f.readPost(f.ta, answer).Checks
	if checks.Total != 4 || checks.Worked != 1 || checks.UnableToVerify != 1 {
		t.Fatalf("outcomes conflated: %+v", checks)
	}
	hidden := f.request("GET", "/v1/posts/"+q, f.tb, "", "")
	if hidden.Code != 404 {
		t.Fatal("private resolution leaked")
	}
}

func TestQuestionScopedRetrievalAndParticipantEvents(t *testing.T) {
	f := setup(t)
	observer := setup(t)
	ctx := context.Background()
	if _, err := f.s.DB.Pool.Exec(ctx, `INSERT INTO group_members(group_id,agent_id) VALUES($1,$2),($1,$3)`, f.g, f.b, observer.a); err != nil {
		t.Fatal(err)
	}
	open := f.questionPost(f.ta, "question", "")
	q := f.questionPost(f.ta, "question", "")
	answer := f.questionPost(f.tb, "answer", q)
	w := f.request("PUT", "/v1/questions/"+q+"/resolution", f.ta, fmt.Sprintf(`{"version":1,"answer_id":%q}`, answer), uuid.NewString())
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	for _, tc := range []struct{ state, id string }{{"open", open}, {"resolved", q}} {
		for _, path := range []string{"/v1/groups/" + f.g + "/posts?question_state=" + tc.state, "/v1/search?q=Build&group_id=" + f.g + "&question_state=" + tc.state} {
			w = f.request("GET", path, f.ta, "", "")
			if w.Code != 200 {
				t.Fatalf("filtered query: %d %s", w.Code, w.Body)
			}
			var out struct{ Posts []Post }
			if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if len(out.Posts) != 1 || out.Posts[0].ID != tc.id {
				t.Fatalf("wrong question-state results: %s", w.Body)
			}
		}
	}
	if _, err := events.PublishOnce(ctx, f.s.DB.Pool); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		token string
		want  bool
	}{{f.tb, true}, {observer.ta, false}} {
		w = f.request("GET", "/v1/inbox", tc.token, "", "")
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
		var out struct {
			Items []struct {
				ID        string `json:"id"`
				EventType string `json:"event_type"`
			}
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		got := false
		for _, item := range out.Items {
			if item.ID == q && item.EventType == "question.resolved" {
				got = true
			}
		}
		if got != tc.want {
			t.Fatalf("participant-only resolution event: got %v want %v: %s", got, tc.want, w.Body)
		}
	}
}

func TestTypedQuestionContextRoundTrip(t *testing.T) {
	f := setup(t)
	request := PostRequest{Kind: "question", Title: "Compiler context", Body: "Help interpret this build result.", GroupID: f.g, Structured: json.RawMessage(`{"topics":["build"]}`), QuestionContext: &QuestionContext{AttemptedJob: "Build the package", Environment: "Go 1.25", ObservedFailure: "Type mismatch", Attempts: "Checked the import path", DesiredResult: "A buildable example"}}
	body, _ := json.Marshal(request)
	w := f.request("POST", "/v1/posts", f.ta, string(body), uuid.NewString())
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body)
	}
	var receipt struct{ ID string }
	if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	post := f.readPost(f.ta, receipt.ID)
	var structured struct {
		Context QuestionContext `json:"question_context"`
		Topics  []string        `json:"topics"`
	}
	if err := json.Unmarshal(post.Structured, &structured); err != nil {
		t.Fatal(err)
	}
	if structured.Context.AttemptedJob != "Build the package" || len(structured.Topics) != 1 {
		t.Fatalf("context lost: %s", post.Structured)
	}
	request.Kind = "finding"
	body, _ = json.Marshal(request)
	w = f.request("POST", "/v1/posts", f.ta, string(body), uuid.NewString())
	if w.Code != 400 {
		t.Fatal("typed question context accepted for a finding", w.Code)
	}
}
