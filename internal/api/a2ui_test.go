package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"videoflow-go/internal/models"
)

// a2uiKinds are the server-to-client message kinds of A2UI v1.0 this server sends.
var a2uiKinds = []string{"createSurface", "updateComponents", "updateDataModel", "deleteSurface"}

// checkA2UIMessage fails the test unless msg is an A2UI v1.0 message: the version and
// exactly one known message with a surfaceId. It returns the message's kind.
func checkA2UIMessage(t *testing.T, msg map[string]any) string {
	t.Helper()
	if msg["version"] != "1.0" || len(msg) != 2 {
		t.Fatalf("not an A2UI v1.0 message: %v", msg)
	}
	for _, kind := range a2uiKinds {
		if body, ok := msg[kind].(map[string]any); ok {
			if id, _ := body["surfaceId"].(string); id == "" {
				t.Fatalf("%s lacks surfaceId: %v", kind, msg)
			}
			if kind == "updateDataModel" && !strings.HasPrefix(body["path"].(string), "/") {
				t.Fatalf("bad path: %v", msg)
			}
			if kind == "createSurface" && body["catalogId"] != a2uiCatalogID {
				t.Fatalf("catalog: %v", msg)
			}
			return kind
		}
	}
	t.Fatalf("unknown message %v", msg)
	return ""
}

// a2uiMessages reads a stream's messages, checking each.
func a2uiMessages(t *testing.T, body string) []map[string]any {
	t.Helper()
	out := []map[string]any{}
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		if data, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
			var msg map[string]any
			if err := json.Unmarshal([]byte(data), &msg); err != nil {
				t.Fatalf("bad message %q: %v", data, err)
			}
			checkA2UIMessage(t, msg)
			out = append(out, msg)
		}
	}
	return out
}

// a2uiSurface is what a client holds of one surface after a stream.
type a2uiSurface struct {
	components map[string]map[string]any
	data       map[string]any
}

// a2uiFold applies the messages the way a client does: components by id, data model
// updates at JSON Pointer paths.
func a2uiFold(t *testing.T, messages []map[string]any, start *a2uiSurface) *a2uiSurface {
	t.Helper()
	s := start
	for _, msg := range messages {
		switch {
		case msg["createSurface"] != nil:
			c := msg["createSurface"].(map[string]any)
			s = &a2uiSurface{components: map[string]map[string]any{}, data: map[string]any{}}
			if d, ok := c["dataModel"].(map[string]any); ok {
				s.data = d
			}
			for _, comp := range c["components"].([]any) {
				m := comp.(map[string]any)
				s.components[m["id"].(string)] = m
			}
		case msg["updateComponents"] != nil:
			for _, comp := range msg["updateComponents"].(map[string]any)["components"].([]any) {
				m := comp.(map[string]any)
				s.components[m["id"].(string)] = m
			}
		case msg["updateDataModel"] != nil:
			u := msg["updateDataModel"].(map[string]any)
			tokens := strings.Split(strings.TrimPrefix(u["path"].(string), "/"), "/")
			node := s.data
			for _, tok := range tokens[:len(tokens)-1] {
				next, ok := node[tok].(map[string]any)
				if !ok {
					next = map[string]any{}
					node[tok] = next
				}
				node = next
			}
			node[tokens[len(tokens)-1]] = u["value"]
		}
	}
	return s
}

// dataPaths lists the updated paths in order, with repeats of the same path kept once.
func dataPaths(messages []map[string]any) []string {
	out := []string{}
	for _, msg := range messages {
		kind := ""
		for k := range msg {
			if k != "version" {
				kind = k
			}
		}
		if u, ok := msg["updateDataModel"].(map[string]any); ok {
			kind = u["path"].(string)
		}
		if len(out) > 0 && out[len(out)-1] == kind {
			continue
		}
		out = append(out, kind)
	}
	return out
}

func a2uiBody(message string, props map[string]any) string {
	body := map[string]any{"message": message}
	for k, v := range props {
		body[k] = v
	}
	payload, _ := json.Marshal(body)
	return string(payload)
}

func TestA2UINormalTurnStreamsTheChatTurn(t *testing.T) {
	s := routerServer(t, nil)
	answer := `{"reply":"I'll write a short story about a lighthouse keeper who befriends a storm, in three scenes.","action":"generate_script","decision":{"mode":"act"}}`
	plannerSays(s, &answer)
	result := call(t, s, "POST", "/a2ui", a2uiBody("Make a story about a lighthouse", map[string]any{"auto": true}))
	if result.Code != 200 || result.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("%d %s", result.Code, result.Body.String())
	}
	messages := a2uiMessages(t, result.Body.String())
	want := []string{"createSurface", "/steps", "/reply", "/intent", "updateComponents", "/run"}
	if got := dataPaths(messages); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("sequence\n got %v\nwant %v", got, want)
	}
	surface := a2uiFold(t, messages, nil)
	if surface.data["state"].(map[string]any)["mode"] != "auto" {
		t.Fatalf("state %v", surface.data["state"])
	}
	steps := []string{}
	for _, step := range surface.data["steps"].([]any) {
		st := step.(map[string]any)
		if st["status"] != "done" || st["label"] == "" {
			t.Fatalf("step %v", st)
		}
		steps = append(steps, st["name"].(string))
	}
	if strings.Join(steps, ",") != "read_project,ask_ai_session,check_answer" {
		t.Fatalf("steps %v", steps)
	}
	run := surface.data["run"].(map[string]any)
	final := run["result"].(map[string]any)
	intent := final["intent"].(map[string]any)
	card := surface.components[a2uiCardID]
	if run["status"] != "finished" || surface.data["reply"] != intent["reply"] || surface.data["intent"].(map[string]any)["action"] != "generate_script" ||
		final["a2ui_surface"] != messages[0]["createSurface"].(map[string]any)["surfaceId"] || card["component"] != "VideoFlowAction" {
		t.Fatalf("surface %v %v", surface.data, card)
	}
	if body := surface.components["body"]; !reflect.DeepEqual(body["children"], []any{a2uiCardID}) {
		t.Fatalf("body %v", body)
	}
	if intent["decision"].(map[string]any)["mode"] != decisionAct || intent["director_proposal"] == nil {
		t.Fatalf("intent %v", intent)
	}
}

// /chat and /a2ui answer the same planner reply with the same card.
func TestA2UIAndChatShareTheTurn(t *testing.T) {
	s := routerServer(t, nil)
	answer := `{"reply":"Here is the plan.","action":"generate_script","decision":{"mode":"act","assumptions":["three scenes"]}}`
	plannerSays(s, &answer)
	chat := call(t, s, "POST", "/chat", `{"message":"A story about a fox"}`)
	var chatBody map[string]any
	if err := json.Unmarshal(chat.Body.Bytes(), &chatBody); err != nil || chat.Code != 200 {
		t.Fatalf("%d %s", chat.Code, chat.Body.String())
	}
	surface := a2uiFold(t, a2uiMessages(t, call(t, s, "POST", "/a2ui", a2uiBody("A story about a fox", nil)).Body.String()), nil)
	streamed := surface.data["run"].(map[string]any)["result"].(map[string]any)
	for _, key := range []string{"action", "reply", "decision", "creation_brief", "scene_count"} {
		a, b := chatBody["intent"].(map[string]any)[key], streamed["intent"].(map[string]any)[key]
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("%s differs: /chat %v /a2ui %v", key, a, b)
		}
	}
	if _, has := chatBody["a2ui_surface"]; has {
		t.Fatal("/chat gained a surface id")
	}
}

func TestA2UIAskTurnRecordsAnswerAndUpdatesState(t *testing.T) {
	s := routerServer(t, nil)
	answer := `{"reply":"Which format should the video be?","action":"generate_script","decision":{"mode":"ask","question":"Which format?","reason":"options","key":"aspect_ratio","options":[{"label":"Vertical"},{"label":"Wide"}]},"remember":[{"key":"tone","value":"warm"}]}`
	plannerSays(s, &answer)
	body := a2uiBody("Spanish", map[string]any{"answer": map[string]any{"key": "language", "question": "Which language?", "label": "Spanish"}})
	messages := a2uiMessages(t, call(t, s, "POST", "/a2ui", body).Body.String())
	paths := dataPaths(messages)
	if paths[len(paths)-2] != "/state/decisions" || paths[len(paths)-1] != "/run" {
		t.Fatalf("no state update before the end: %v", paths)
	}
	surface := a2uiFold(t, messages, nil)
	raw, _ := json.Marshal(surface.data["intent"])
	var intent struct {
		Decision chatDecision `json:"decision"`
	}
	_ = json.Unmarshal(raw, &intent)
	if intent.Decision.Mode != decisionAsk || len(intent.Decision.Options) != 2 {
		t.Fatalf("ask decision %+v", intent.Decision)
	}
	// The client's state is the decision log as saved.
	state := surface.data["state"].(map[string]any)
	saved := call(t, s, "GET", "/decisions", "")
	var log struct {
		Decisions any `json:"decisions"`
	}
	_ = json.Unmarshal(saved.Body.Bytes(), &log)
	if !reflect.DeepEqual(state["decisions"], log.Decisions) {
		t.Fatalf("state decisions %v\nsaved %v", state["decisions"], log.Decisions)
	}
	keys := mustJSON(state["decisions"])
	if !strings.Contains(keys, "language") || !strings.Contains(keys, "tone") {
		t.Fatalf("decisions %v", keys)
	}
}

func TestA2UIErrorEndsTheRequestWithRunError(t *testing.T) {
	s := routerServer(t, nil)
	s.agents.SetStructuredRunner(func(context.Context, string, string, string, any) error {
		return errors.New("Codex couldn't answer")
	})
	messages := a2uiMessages(t, call(t, s, "POST", "/a2ui", a2uiBody("Hello", nil)).Body.String())
	if got := strings.Join(dataPaths(messages), ","); got != "createSurface,/steps,/run" {
		t.Fatalf("got %s", got)
	}
	run := a2uiFold(t, messages, nil).data["run"].(map[string]any)
	if run["status"] != "error" || run["message"] != "Codex couldn't answer" || run["code"] != "http_500" {
		t.Fatalf("%v", run)
	}
}

func TestA2UIRejectsMalformedInputBeforeStreaming(t *testing.T) {
	s := routerServer(t, nil)
	for _, body := range []string{
		`{}`, `{"message":"  "}`, `not json`, `{"message":"hi","auto":"yes"}`,
		`{"version":"1.0","action":{"name":"generate_script","surfaceId":"s","sourceComponentId":"card"}}`,
		`{"version":"1.0","action":{"name":"videoflow.reply","surfaceId":"s","sourceComponentId":"ui-b","context":{}}}`,
		// A card result must come from the app's card, never from the assistant's interface.
		`{"version":"1.0","action":{"name":"videoflow.card_result","surfaceId":"s","sourceComponentId":"ui-b","context":{"op_id":"x"}}}`,
	} {
		if result := call(t, s, "POST", "/a2ui", body); result.Code != 400 {
			t.Fatalf("%s: %d", body, result.Code)
		}
	}
	if !exemptFromRequestTimeout("/a2ui") {
		t.Fatal("/a2ui must outlast the request timeout")
	}
}

// The assistant's interface reaches the surface rewritten; a choice picked in it is
// planned as the user's next message, never run as an action.
func TestA2UIAssistantInterfaceAndReplyAction(t *testing.T) {
	s := routerServer(t, nil)
	answer := `{"reply":"Two looks to pick from.","action":"unknown","decision":{"mode":"inform"},"ui":[` +
		`{"id":"root","component":"Row","children":["a","b"]},` +
		`{"id":"a","component":"Button","child":"at","action":{"event":{"name":"generate_video","context":{"reply":"Use the warm look"}}}},` +
		`{"id":"at","component":"Text","text":"Warm"},{"id":"b","component":"Image","url":"/media/b.png"}]}`
	plannerSays(s, &answer)
	messages := a2uiMessages(t, call(t, s, "POST", "/a2ui", a2uiBody("Show me looks", nil)).Body.String())
	surface := a2uiFold(t, messages, nil)
	button := surface.components["ui-a"]
	if button == nil || mustJSON(button["action"]) != `{"event":{"context":{"reply":"Use the warm look"},"name":"videoflow.reply"}}` {
		t.Fatalf("button %v", button)
	}
	if !reflect.DeepEqual(surface.components["body"]["children"], []any{a2uiUIRoot}) || surface.components[a2uiCardID] != nil {
		t.Fatalf("body %v", surface.components["body"])
	}
	// The picked choice comes back as an A2UI action and is planned as a message.
	var heard string
	answer = `{"reply":"Warm it is.","action":"unknown","decision":{"mode":"inform"}}`
	s.agents.SetStructuredRunner(func(_ context.Context, _, _, prompt string, target any) error {
		heard = prompt
		return json.Unmarshal([]byte(answer), target)
	})
	action := `{"version":"1.0","action":{"name":"videoflow.reply","surfaceId":"turn-x","sourceComponentId":"ui-a","timestamp":"2026-01-01T00:00:00Z","context":{"reply":"Use the warm look"}}}`
	next := a2uiFold(t, a2uiMessages(t, call(t, s, "POST", "/a2ui", action).Body.String()), nil)
	if !strings.Contains(heard, "User message: Use the warm look") || next.data["reply"] != "Warm it is." {
		t.Fatalf("heard %q, surface %v", heard, next.data)
	}
}

func cardResultBody(surfaceID string, outcome map[string]any) string {
	payload, _ := json.Marshal(map[string]any{"version": "1.0", "action": map[string]any{
		"name": a2uiCardResultAction, "surfaceId": surfaceID, "sourceComponentId": a2uiCardID,
		"timestamp": "2026-01-01T00:00:00Z", "context": outcome,
	}})
	return string(payload)
}

// A card that ran at once reports how it ended; the request records it as /result.
func TestA2UICardResultWithoutOp(t *testing.T) {
	s := routerServer(t, nil)
	messages := a2uiMessages(t, call(t, s, "POST", "/a2ui", cardResultBody("turn-1", map[string]any{"status": "failed", "error": "no credits"})).Body.String())
	if got := strings.Join(dataPaths(messages), ","); got != "/run,/state,/result,/run" {
		t.Fatalf("got %s", got)
	}
	for _, m := range messages {
		if m["updateDataModel"].(map[string]any)["surfaceId"] != "turn-1" {
			t.Fatalf("wrong surface %v", m)
		}
	}
	if got := mustJSON(messages[2]["updateDataModel"].(map[string]any)["value"]); got != `{"error":"no credits","status":"failed"}` {
		t.Fatalf("%s", got)
	}
}

// A card that started a background op: the request follows it on the event bus and
// sets the card's result when the op ends.
func TestA2UICardResultFollowsBackgroundOp(t *testing.T) {
	defer func(wait time.Duration) { a2uiStateEvery = wait }(a2uiStateEvery)
	a2uiStateEvery = 10 * time.Millisecond
	s := routerServer(t, nil)
	project, _ := s.database.GetActiveProjectID()
	op := &models.Op{ID: "op-follow", Kind: "shots", Status: models.StatusRunning, ProjectID: &project}
	if err := s.database.CreateOp(op); err != nil {
		t.Fatal(err)
	}
	other := "another-project"
	if err := s.database.CreateOp(&models.Op{ID: "op-other", Kind: "shots", Status: models.StatusSucceeded, ProjectID: &other}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(s.Router())
	defer server.Close()
	response, err := http.Post(server.URL+"/a2ui", "application/json", strings.NewReader(cardResultBody("turn-7", map[string]any{"op_id": "op-follow"})))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	next := func() map[string]any {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("stream ended: %v", err)
			}
			if data, ok := strings.CutPrefix(strings.TrimSpace(line), "data: "); ok {
				var msg map[string]any
				_ = json.Unmarshal([]byte(data), &msg)
				checkA2UIMessage(t, msg)
				return msg["updateDataModel"].(map[string]any)
			}
		}
	}
	if next()["path"] != "/run" || next()["path"] != "/state" {
		t.Fatal("request did not start with its state")
	}
	// The request is now waiting on the op. A decision saved meanwhile reaches the
	// client once the bus reports a change.
	call(t, s, "PUT", "/decisions/tone", `{"value":"calm"}`)
	// Publish until heard: the stream may subscribe after a single publish.
	stop := keepPublishing(s, map[string]any{"type": "asset_changed"})
	update := next()
	stop()
	if update["path"] != "/state/decisions" || !strings.Contains(mustJSON(update), "calm") {
		t.Fatalf("expected a decision update, got %v", update)
	}
	if err = s.database.UpdateOp("op-follow", models.StatusSucceeded, json.RawMessage(`{"shots":3}`), nil); err != nil {
		t.Fatal(err)
	}
	stop = keepPublishing(s, map[string]any{"type": "op_done", "op_id": "op-follow", "status": models.StatusSucceeded})
	got := next()
	for strings.HasPrefix(got["path"].(string), "/state") {
		got = next()
	}
	stop()
	result := mustJSON(got["value"])
	if got["path"] != "/result" || got["surfaceId"] != "turn-7" || !strings.Contains(result, `"status":"succeeded"`) || !strings.Contains(result, `"shots":3`) {
		t.Fatalf("card result %v", got)
	}
	for got = next(); strings.HasPrefix(got["path"].(string), "/state"); got = next() {
	}
	if got["path"] != "/run" || got["value"].(map[string]any)["status"] != "finished" {
		t.Fatalf("expected the request to finish, got %v", got)
	}
	// Another project's op is not followed.
	messages := a2uiMessages(t, call(t, s, "POST", "/a2ui", cardResultBody("turn-8", map[string]any{"op_id": "op-other"})).Body.String())
	last := messages[len(messages)-1]["updateDataModel"].(map[string]any)["value"].(map[string]any)
	if last["status"] != "error" || last["code"] != "http_404" {
		t.Fatalf("%v", last)
	}
}

func mustJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func TestReplyChunksKeepWordsAndRunes(t *testing.T) {
	reply := "Ich schreibe eine kurze Geschichte über einen Leuchtturmwärter, der einen Sturm zum Freund gewinnt — 灯台守と嵐の物語を書きます。"
	chunks := replyChunks(reply, 16)
	if strings.Join(chunks, "") != reply || len(chunks) < 4 {
		t.Fatalf("%q", chunks)
	}
	for _, c := range chunks {
		if c == "" || !utf8Start(c[0]) {
			t.Fatalf("bad chunk %q", c)
		}
	}
	if len(replyChunks("", 16)) != 0 {
		t.Fatal("an empty reply has no chunks")
	}
}

// keepPublishing publishes event every few milliseconds until stop is called.
func keepPublishing(s *Server, event map[string]any) (stop func()) {
	done := make(chan struct{})
	go func() {
		for {
			s.broker.Publish(event)
			select {
			case <-done:
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}
