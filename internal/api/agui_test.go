package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"videoflow-go/internal/models"
)

// aguiRequired is each event type's required fields, from the AG-UI 1.0 JSON Schema
// (@ag-ui/core 1.0.5).
var aguiRequired = map[string][]string{
	aguiRunStarted:         {"threadId", "runId"},
	aguiRunFinished:        {"threadId", "runId"},
	aguiRunError:           {"message"},
	aguiStepStarted:        {"stepName"},
	aguiStepFinished:       {"stepName"},
	aguiTextMessageStart:   {"messageId"},
	aguiTextMessageContent: {"messageId", "delta"},
	aguiTextMessageEnd:     {"messageId"},
	aguiToolCallStart:      {"toolCallId", "toolCallName"},
	aguiToolCallArgs:       {"toolCallId", "delta"},
	aguiToolCallEnd:        {"toolCallId"},
	aguiToolCallResult:     {"messageId", "toolCallId", "content"},
	aguiStateSnapshot:      {"snapshot"},
	aguiStateDelta:         {"delta"},
}

// aguiTypes is the Go type each event is sent as.
var aguiTypes = map[string]any{
	aguiRunStarted: aguiRunStartedEvent{}, aguiRunFinished: aguiRunFinishedEvent{}, aguiRunError: aguiRunErrorEvent{},
	aguiStepStarted: aguiStepEvent{}, aguiStepFinished: aguiStepEvent{},
	aguiTextMessageStart: aguiTextMessageStartEvent{}, aguiTextMessageContent: aguiTextMessageContentEvent{}, aguiTextMessageEnd: aguiTextMessageEndEvent{},
	aguiToolCallStart: aguiToolCallStartEvent{}, aguiToolCallArgs: aguiToolCallArgsEvent{}, aguiToolCallEnd: aguiToolCallEndEvent{}, aguiToolCallResult: aguiToolCallResultEvent{},
	aguiStateSnapshot: aguiStateSnapshotEvent{}, aguiStateDelta: aguiStateDeltaEvent{},
}

var jsonPointer = regexp.MustCompile(`^(/([^/~]|~[01])*)*$`)

// checkAGUIEvent fails the test when an event lacks a field the schema requires or
// carries a malformed patch.
func checkAGUIEvent(t *testing.T, event map[string]any) {
	t.Helper()
	kind, _ := event["type"].(string)
	required, known := aguiRequired[kind]
	if !known {
		t.Fatalf("unexpected event type %q", kind)
	}
	for _, field := range required {
		if _, ok := event[field]; !ok {
			t.Fatalf("%s lacks %s: %v", kind, field, event)
		}
	}
	if kind == aguiTextMessageContent && event["delta"] == "" {
		t.Fatalf("empty text delta: %v", event)
	}
	if kind == aguiStateDelta {
		for _, op := range event["delta"].([]any) {
			o := op.(map[string]any)
			_, hasValue := o["value"]
			if !jsonPointer.MatchString(o["path"].(string)) || (o["op"] != "remove") != hasValue {
				t.Fatalf("bad patch op %v", o)
			}
		}
	}
}

// The examples in testdata follow the AG-UI docs; each decodes into the Go type the
// server sends it as and encodes back unchanged.
func TestAGUIEventsMatchSchemaExamples(t *testing.T) {
	raw, err := os.ReadFile("testdata/agui_events.json")
	if err != nil {
		t.Fatal(err)
	}
	var examples []json.RawMessage
	if err = json.Unmarshal(raw, &examples); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, example := range examples {
		var event map[string]any
		_ = json.Unmarshal(example, &event)
		checkAGUIEvent(t, event)
		kind := event["type"].(string)
		seen[kind] = true
		typed := reflect.New(reflect.TypeOf(aguiTypes[kind]))
		if err = json.Unmarshal(example, typed.Interface()); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		again, _ := json.Marshal(typed.Elem().Interface())
		var back any
		_ = json.Unmarshal(again, &back)
		if !reflect.DeepEqual(jsonValue(event), back) {
			t.Fatalf("%s round trip:\n%s\n%s", kind, example, again)
		}
	}
	for kind := range aguiRequired {
		if !seen[kind] {
			t.Errorf("no example of %s", kind)
		}
	}
}

// aguiEvents reads a stream's events, checking each against the schema.
func aguiEvents(t *testing.T, body string) []map[string]any {
	t.Helper()
	events := []map[string]any{}
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		if data, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
			var event map[string]any
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				t.Fatalf("bad event %q: %v", data, err)
			}
			checkAGUIEvent(t, event)
			events = append(events, event)
		}
	}
	return events
}

// eventTypes lists the events' types, with consecutive text and step deltas kept.
func eventTypes(events []map[string]any) []string {
	out := []string{}
	for _, e := range events {
		kind := e["type"].(string)
		if name, ok := e["stepName"].(string); ok {
			kind += ":" + name
		}
		if kind == aguiTextMessageContent && len(out) > 0 && out[len(out)-1] == kind {
			continue
		}
		out = append(out, kind)
	}
	return out
}

func aguiBody(message string, props map[string]any) string {
	payload, _ := json.Marshal(map[string]any{
		"threadId": "thread-1", "runId": "run-1", "state": nil, "tools": []any{}, "context": []any{},
		"messages":       []any{map[string]any{"id": "m1", "role": "user", "content": message}},
		"forwardedProps": props,
	})
	return string(payload)
}

// applyState folds the run's STATE_SNAPSHOT and STATE_DELTA events into the state a
// client ends with.
func applyState(t *testing.T, events []map[string]any) any {
	t.Helper()
	var state any
	for _, e := range events {
		switch e["type"] {
		case aguiStateSnapshot:
			state = e["snapshot"]
		case aguiStateDelta:
			raw, _ := json.Marshal(e["delta"])
			var ops []patchOp
			_ = json.Unmarshal(raw, &ops)
			var err error
			if state, err = applyJSONPatch(state, ops); err != nil {
				t.Fatalf("delta does not apply: %v", err)
			}
		}
	}
	return state
}

func TestAGUINormalTurnStreamsTheChatTurn(t *testing.T) {
	s := routerServer(t, nil)
	answer := `{"reply":"I'll write a short story about a lighthouse keeper who befriends a storm, in three scenes.","action":"generate_script","decision":{"mode":"act"}}`
	plannerSays(s, &answer)
	result := call(t, s, "POST", "/agui", aguiBody("Make a story about a lighthouse", map[string]any{"auto": true}))
	if result.Code != 200 || result.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("%d %s", result.Code, result.Body.String())
	}
	events := aguiEvents(t, result.Body.String())
	want := []string{
		"RUN_STARTED", "STATE_SNAPSHOT",
		"STEP_STARTED:read_project", "STEP_FINISHED:read_project",
		"STEP_STARTED:ask_ai_session", "STEP_FINISHED:ask_ai_session",
		"STEP_STARTED:check_answer", "STEP_FINISHED:check_answer",
		"TEXT_MESSAGE_START", "TEXT_MESSAGE_CONTENT", "TEXT_MESSAGE_END",
		"TOOL_CALL_START", "TOOL_CALL_ARGS", "TOOL_CALL_END", "RUN_FINISHED",
	}
	if got := eventTypes(events); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("sequence\n got %v\nwant %v", got, want)
	}
	if events[1]["snapshot"].(map[string]any)["mode"] != "auto" {
		t.Fatalf("snapshot %v", events[1])
	}
	text, args, finished := "", "", events[len(events)-1]
	for _, e := range events {
		switch e["type"] {
		case aguiTextMessageContent:
			text += e["delta"].(string)
		case aguiToolCallArgs:
			args += e["delta"].(string)
		case aguiToolCallStart:
			if e["toolCallName"] != "generate_script" || e["parentMessageId"] == nil {
				t.Fatalf("tool call %v", e)
			}
		}
	}
	var intent map[string]any
	if err := json.Unmarshal([]byte(args), &intent); err != nil {
		t.Fatalf("tool args are not JSON: %v", err)
	}
	decision, _ := intent["decision"].(map[string]any)
	result1 := finished["result"].(map[string]any)
	final := result1["intent"].(map[string]any)
	if text != final["reply"] || intent["action"] != "generate_script" || decision["mode"] != decisionAct || final["director_proposal"] == nil || result1["tool_call_id"] == nil {
		t.Fatalf("text %q args %v result %v", text, intent, result1)
	}
	if finished["threadId"] != "thread-1" || finished["runId"] != "run-1" {
		t.Fatalf("finished %v", finished)
	}
}

// /chat and /agui answer the same planner reply with the same card.
func TestAGUIAndChatShareTheTurn(t *testing.T) {
	s := routerServer(t, nil)
	answer := `{"reply":"Here is the plan.","action":"generate_script","decision":{"mode":"act","assumptions":["three scenes"]}}`
	plannerSays(s, &answer)
	chat := call(t, s, "POST", "/chat", `{"message":"A story about a fox"}`)
	var chatBody map[string]any
	if err := json.Unmarshal(chat.Body.Bytes(), &chatBody); err != nil || chat.Code != 200 {
		t.Fatalf("%d %s", chat.Code, chat.Body.String())
	}
	events := aguiEvents(t, call(t, s, "POST", "/agui", aguiBody("A story about a fox", nil)).Body.String())
	agui := events[len(events)-1]["result"].(map[string]any)
	for _, key := range []string{"action", "reply", "decision", "creation_brief", "scene_count"} {
		a, b := chatBody["intent"].(map[string]any)[key], agui["intent"].(map[string]any)[key]
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("%s differs: /chat %v /agui %v", key, a, b)
		}
	}
	if _, has := chatBody["tool_call_id"]; has {
		t.Fatal("/chat gained a tool call id")
	}
}

func TestAGUIAskTurnRecordsAnswerAndSendsStateDelta(t *testing.T) {
	s := routerServer(t, nil)
	answer := `{"reply":"Which format should the video be?","action":"generate_script","decision":{"mode":"ask","question":"Which format?","reason":"options","key":"aspect_ratio","options":[{"label":"Vertical"},{"label":"Wide"}]},"remember":[{"key":"tone","value":"warm"}]}`
	plannerSays(s, &answer)
	body := aguiBody("Spanish", map[string]any{"answer": map[string]any{"key": "language", "question": "Which language?", "label": "Spanish"}})
	events := aguiEvents(t, call(t, s, "POST", "/agui", body).Body.String())
	types := eventTypes(events)
	if types[len(types)-2] != aguiStateDelta || types[len(types)-1] != aguiRunFinished {
		t.Fatalf("no state delta before the end: %v", types)
	}
	var args string
	for _, e := range events {
		if e["type"] == aguiToolCallArgs {
			args += e["delta"].(string)
		}
	}
	var intent struct {
		Decision chatDecision `json:"decision"`
	}
	_ = json.Unmarshal([]byte(args), &intent)
	if intent.Decision.Mode != decisionAsk || len(intent.Decision.Options) != 2 {
		t.Fatalf("ask decision %+v", intent.Decision)
	}
	// The client's state, snapshot plus deltas, is the decision log as saved.
	state := applyState(t, events).(map[string]any)
	saved := call(t, s, "GET", "/decisions", "")
	var log struct {
		Decisions any `json:"decisions"`
	}
	_ = json.Unmarshal(saved.Body.Bytes(), &log)
	if !reflect.DeepEqual(state["decisions"], log.Decisions) {
		t.Fatalf("state decisions %v\nsaved %v", state["decisions"], log.Decisions)
	}
	keys := []string{}
	for _, d := range state["decisions"].([]any) {
		keys = append(keys, d.(map[string]any)["key"].(string))
	}
	if !strings.Contains(strings.Join(keys, ","), "language") || !strings.Contains(strings.Join(keys, ","), "tone") {
		t.Fatalf("decisions %v", keys)
	}
}

func TestAGUIErrorEndsTheRunWithRunError(t *testing.T) {
	s := routerServer(t, nil)
	s.agents.SetStructuredRunner(func(context.Context, string, string, string, any) error {
		return errors.New("Codex couldn't answer")
	})
	events := aguiEvents(t, call(t, s, "POST", "/agui", aguiBody("Hello", nil)).Body.String())
	want := "RUN_STARTED,STATE_SNAPSHOT,STEP_STARTED:read_project,STEP_FINISHED:read_project,STEP_STARTED:ask_ai_session,STEP_FINISHED:ask_ai_session,RUN_ERROR"
	if got := strings.Join(eventTypes(events), ","); got != want {
		t.Fatalf("got %s", got)
	}
	last := events[len(events)-1]
	if last["message"] != "Codex couldn't answer" || last["code"] != "http_500" {
		t.Fatalf("%v", last)
	}
}

func TestAGUIRejectsMalformedInputBeforeStreaming(t *testing.T) {
	s := routerServer(t, nil)
	for _, body := range []string{`{}`, `{"threadId":"t","runId":"r","messages":[]}`, `{"threadId":"t","runId":"r","messages":[{"id":"a","role":"assistant","content":"hi"}]}`, `{"threadId":"t","runId":"r","messages":[{"id":"a","role":"user","content":"hi"}],"forwardedProps":{"auto":"yes"}}`} {
		if result := call(t, s, "POST", "/agui", body); result.Code != 400 {
			t.Fatalf("%s: %d", body, result.Code)
		}
	}
	if !exemptFromRequestTimeout("/agui") {
		t.Fatal("/agui must outlast the request timeout")
	}
}

func toolBody(callID, content string) string {
	payload, _ := json.Marshal(map[string]any{
		"threadId": "thread-1", "runId": "run-2", "tools": []any{}, "context": []any{},
		"messages": []any{
			map[string]any{"id": "m1", "role": "user", "content": "Make the shots"},
			map[string]any{"id": "m2", "role": "assistant", "content": "", "toolCalls": []any{map[string]any{"id": callID, "type": "function", "function": map[string]any{"name": "generate_shots", "arguments": "{}"}}}},
			map[string]any{"id": "m3", "role": "tool", "toolCallId": callID, "content": content},
		},
	})
	return string(payload)
}

// A card that ran at once reports how it ended; the run echoes it as the result.
func TestAGUIToolResultWithoutOp(t *testing.T) {
	s := routerServer(t, nil)
	events := aguiEvents(t, call(t, s, "POST", "/agui", toolBody("call-1", `{"status":"succeeded"}`)).Body.String())
	if got := strings.Join(eventTypes(events), ","); got != "RUN_STARTED,STATE_SNAPSHOT,TOOL_CALL_RESULT,RUN_FINISHED" {
		t.Fatalf("got %s", got)
	}
	if events[2]["toolCallId"] != "call-1" || events[2]["content"] != `{"status":"succeeded"}` {
		t.Fatalf("%v", events[2])
	}
}

// A card that started a background op: the run follows it on the event bus and fills
// the tool call's result when the op ends.
func TestAGUIToolResultFollowsBackgroundOp(t *testing.T) {
	defer func(wait time.Duration) { aguiStateEvery = wait }(aguiStateEvery)
	aguiStateEvery = 10 * time.Millisecond
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
	response, err := http.Post(server.URL+"/agui", "application/json", strings.NewReader(toolBody("call-7", `{"op_id":"op-follow"}`)))
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
				var event map[string]any
				_ = json.Unmarshal([]byte(data), &event)
				checkAGUIEvent(t, event)
				return event
			}
		}
	}
	if next()["type"] != aguiRunStarted || next()["type"] != aguiStateSnapshot {
		t.Fatal("run did not start with its state")
	}
	// The run is now waiting on the op. A decision saved meanwhile reaches the client
	// as a delta once the bus reports a change.
	call(t, s, "PUT", "/decisions/tone", `{"value":"calm"}`)
	// Publish until heard: the stream may subscribe after a single publish.
	stop := keepPublishing(s, map[string]any{"type": "asset_changed"})
	delta := next()
	stop()
	if delta["type"] != aguiStateDelta || !strings.Contains(mustJSON(delta), "calm") {
		t.Fatalf("expected a decision delta, got %v", delta)
	}
	result := json.RawMessage(`{"shots":3}`)
	if err = s.database.UpdateOp("op-follow", models.StatusSucceeded, result, nil); err != nil {
		t.Fatal(err)
	}
	stop = keepPublishing(s, map[string]any{"type": "op_done", "op_id": "op-follow", "status": models.StatusSucceeded})
	got := next()
	for got["type"] == aguiStateDelta {
		got = next()
	}
	stop()
	if got["type"] != aguiToolCallResult || got["toolCallId"] != "call-7" || !strings.Contains(got["content"].(string), `"status":"succeeded"`) || !strings.Contains(got["content"].(string), `"shots":3`) {
		t.Fatalf("tool result %v", got)
	}
	for got = next(); got["type"] == aguiStateDelta; got = next() {
	}
	if got["type"] != aguiRunFinished {
		t.Fatalf("expected the run to finish, got %v", got)
	}
	// Another project's op is not followed.
	events := aguiEvents(t, call(t, s, "POST", "/agui", toolBody("call-8", `{"op_id":"op-other"}`)).Body.String())
	if last := events[len(events)-1]; last["type"] != aguiRunError || last["code"] != "http_404" {
		t.Fatalf("%v", last)
	}
}

func mustJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func TestJSONPatchDiffAppliesCleanly(t *testing.T) {
	cases := []struct{ from, to string }{
		{`{"a":1}`, `{"a":2}`},
		{`{"a":1,"b":{"c":[1,2,3]}}`, `{"b":{"c":[1,5,3]},"d":null}`},
		{`{"list":[1,2]}`, `{"list":[1,2,3]}`},
		{`{"list":[{"k":"x"}]}`, `{"list":[]}`},
		{`{"a/b":1,"m~n":{"x":true}}`, `{"a/b":2,"m~n":{"x":false,"y":"new"}}`},
		{`{"stages":null}`, `{"stages":{"current":"brief","stages":[{"id":"brief","status":"ready"}]}}`},
		{`{"stages":{"current":"brief","stages":[{"id":"brief","status":"ready"}]}}`, `{"stages":{"current":"script","stages":[{"id":"brief","status":"done"}]}}`},
		{`[1,2]`, `{"now":"object"}`},
		{`{"same":[1,{"x":2}]}`, `{"same":[1,{"x":2}]}`},
	}
	for _, tc := range cases {
		var from, to any
		_ = json.Unmarshal([]byte(tc.from), &from)
		_ = json.Unmarshal([]byte(tc.to), &to)
		ops := diffJSON(jsonValue(from), to)
		raw, _ := json.Marshal(ops)
		var decoded []patchOp
		_ = json.Unmarshal(raw, &decoded)
		for _, op := range decoded {
			if !jsonPointer.MatchString(op.Path) {
				t.Fatalf("%s: bad pointer %q", tc.from, op.Path)
			}
		}
		got, err := applyJSONPatch(from, decoded)
		if err != nil || !reflect.DeepEqual(got, to) {
			t.Fatalf("%s -> %s: patch %s gave %v (%v)", tc.from, tc.to, raw, got, err)
		}
		if tc.from == tc.to && len(ops) != 0 {
			t.Fatalf("no change made a patch: %s", raw)
		}
	}
	if raw, _ := json.Marshal(diffJSON(map[string]any{"a": 1.0}, map[string]any{"a": nil})); string(raw) != `[{"op":"replace","path":"/a","value":null}]` {
		t.Fatalf("null replace lost its value: %s", raw)
	}
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
