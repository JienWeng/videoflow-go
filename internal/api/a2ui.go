package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"videoflow-go/internal/models"
)

// POST /a2ui is the chat assistant over A2UI (https://a2ui.org, protocol v1.0): the
// same turn as POST /chat, streamed as A2UI messages, one per server-sent event
// (docs/handbook/12-reference.md lists them). Each turn is one surface:
//
//   - its data model holds the reply (/reply, growing as it streams), the turn's phases
//     (/steps), the shared project state (/state: stage map, decision log, mode), the
//     prepared card (/intent), the card's outcome (/result) and how the request went
//     (/run: running, finished with the /chat response, or error);
//   - its components show the reply, the phases, the assistant's own interface (only
//     allowlisted basic-catalog components, a2ui_catalog.go) and the card as the
//     VideoFlowAction component of the app's catalog, which the app renders as its own
//     priced cards.
//
// A request is a user message, or an A2UI action: videoflow.reply (a choice the user
// picked in the assistant's interface, planned as the next message) or
// videoflow.card_result (the user ran the card; with a background op the request
// follows it on the event bus and keeps /state in step until it ends).

const (
	a2uiProtocolVersion = "1.0"
	// a2uiCatalogID is the app's catalog: the A2UI basic catalog plus VideoFlowAction.
	a2uiCatalogID = "https://videoflow.app/a2ui/catalog/v1"
	// a2uiCardResultAction reports what became of the card the user ran.
	a2uiCardResultAction = "videoflow.card_result"
	// a2uiCardID is the VideoFlowAction component's id in every turn surface.
	a2uiCardID = "card"
)

// a2uiReplyChunk is about how many bytes the reply grows by in each update.
const a2uiReplyChunk = 48

// a2uiOpWait is the longest a request follows one background op before it reports
// the op as still running.
var a2uiOpWait = 30 * time.Minute

// a2uiStateEvery is how often a request following an op recomputes the state after
// the event bus reported a change.
var a2uiStateEvery = time.Second

// a2uiMessage is one server-to-client A2UI message: the version and exactly one of
// the others.
type a2uiMessage struct {
	Version          string                `json:"version"`
	CreateSurface    *a2uiCreateSurface    `json:"createSurface,omitempty"`
	UpdateComponents *a2uiUpdateComponents `json:"updateComponents,omitempty"`
	UpdateDataModel  *a2uiUpdateDataModel  `json:"updateDataModel,omitempty"`
	DeleteSurface    *a2uiDeleteSurface    `json:"deleteSurface,omitempty"`
}

type a2uiCreateSurface struct {
	SurfaceID  string           `json:"surfaceId"`
	CatalogID  string           `json:"catalogId"`
	Components []map[string]any `json:"components,omitempty"`
	DataModel  map[string]any   `json:"dataModel,omitempty"`
}

type a2uiUpdateComponents struct {
	SurfaceID  string           `json:"surfaceId"`
	Components []map[string]any `json:"components"`
}

type a2uiUpdateDataModel struct {
	SurfaceID string `json:"surfaceId"`
	Path      string `json:"path"`
	Value     any    `json:"value"`
}

type a2uiDeleteSurface struct {
	SurfaceID string `json:"surfaceId"`
}

// a2uiAction is the client-to-server A2UI action event.
type a2uiAction struct {
	Name              string         `json:"name"`
	SurfaceID         string         `json:"surfaceId"`
	SourceComponentID string         `json:"sourceComponentId"`
	Timestamp         string         `json:"timestamp"`
	Context           map[string]any `json:"context"`
}

// a2uiRequest is the body of POST /a2ui: the chat request's fields, and for an action
// the A2UI action envelope ({"version", "action"}).
type a2uiRequest struct {
	chatRequest
	Version string      `json:"version"`
	Action  *a2uiAction `json:"action"`
}

func (s *Server) handleA2UI(w http.ResponseWriter, r *http.Request) {
	var body a2uiRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "a chat message or an A2UI action is required", http.StatusBadRequest)
		return
	}
	surfaceID := "turn-" + uuid.NewString()
	cardResult := false
	if a := body.Action; a != nil {
		switch {
		case a.Name == a2uiReplyAction:
			reply, _ := a.Context["reply"].(string)
			body.Message = reply
		case a.Name == a2uiCardResultAction && a.SourceComponentID == a2uiCardID && a.SurfaceID != "":
			cardResult, surfaceID = true, a.SurfaceID
		default:
			http.Error(w, "unknown A2UI action", http.StatusBadRequest)
			return
		}
	}
	body.Message = strings.TrimSpace(body.Message)
	if !cardResult && body.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}
	// A request can follow a background op for longer than the server's write limit.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	run := &a2uiRun{s: s, st: &a2uiStream{w: w, f: flusher}, surfaceID: surfaceID, mode: a2uiMode(body.Auto, body.Full)}
	run.projectID, _ = s.activeProjectID(r.Context())
	run.state = run.currentState(r.Context())
	var result any
	var failure *chatTurnError
	if cardResult {
		if run.update("/run", map[string]any{"status": "running"}) != nil || run.update("/state", run.state) != nil {
			return
		}
		result, failure = run.cardResult(r.Context(), body.Action.Context)
	} else {
		if run.st.emit(a2uiMessage{CreateSurface: &a2uiCreateSurface{
			SurfaceID: surfaceID, CatalogID: a2uiCatalogID, Components: a2uiTurnComponents(nil),
			DataModel: map[string]any{"run": map[string]any{"status": "running"}, "reply": "", "steps": []any{}, "state": run.state},
		}}) != nil {
			return
		}
		result, failure = run.chat(r.Context(), body.chatRequest)
	}
	if r.Context().Err() != nil {
		return // the client left; there is no one to tell
	}
	if failure != nil {
		code := failure.code
		if code == "" {
			code = "http_" + strconv.Itoa(failure.status)
		}
		_ = run.update("/run", map[string]any{"status": "error", "message": failure.message, "code": code})
		return
	}
	_ = run.update("/run", map[string]any{"status": "finished", "result": result})
}

// a2uiTurnComponents is a turn surface's layout: the reply, the phases and a body
// holding the assistant's interface and the card, when there are any.
func a2uiTurnComponents(body []string) []map[string]any {
	if body == nil {
		body = []string{}
	}
	return []map[string]any{
		{"id": "root", "component": "Column", "children": []string{"reply", "steps", "body"}},
		{"id": "reply", "component": "Text", "text": map[string]any{"@path": "/reply"}},
		{"id": "steps", "component": "List", "children": map[string]any{"path": "/steps", "componentId": "step"}},
		{"id": "step", "component": "Text", "variant": "caption", "text": map[string]any{"@path": "label"}},
		{"id": "body", "component": "Column", "children": body},
	}
}

// a2uiMode names the chat mode as the state carries it.
func a2uiMode(auto, full bool) string {
	switch {
	case full:
		return "full"
	case auto:
		return "auto"
	}
	return "manual"
}

// a2uiRun is one POST /a2ui request: its stream, its surface and the state the client
// holds so far.
type a2uiRun struct {
	s         *Server
	st        *a2uiStream
	surfaceID string
	projectID string
	mode      string
	// state is the last /state sent, as plain JSON.
	state map[string]any
}

// update sets one path of the surface's data model.
func (run *a2uiRun) update(path string, value any) error {
	return run.st.emit(a2uiMessage{UpdateDataModel: &a2uiUpdateDataModel{SurfaceID: run.surfaceID, Path: path, Value: value}})
}

// currentState is the shared state: the project's stage map (GET /stages), its
// decision log (GET /decisions) and the chat mode. A part that cannot be read is null.
func (run *a2uiRun) currentState(ctx context.Context) map[string]any {
	state := map[string]any{"stages": nil, "decisions": nil, "mode": run.mode}
	if run.projectID != "" {
		if stages, err := run.s.projectStages(ctx, run.projectID); err == nil {
			state["stages"] = stages
		}
		if decisions, err := run.s.database.ListDecisions(run.projectID); err == nil {
			state["decisions"] = decisions
		}
	}
	plain, _ := plainJSON(state).(map[string]any)
	return plain
}

// plainJSON is v as plain JSON values (maps, slices, strings, float64, bool, nil).
func plainJSON(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out any
	_ = json.Unmarshal(raw, &out)
	return out
}

// syncState updates each part of /state that changed since the client's copy.
func (run *a2uiRun) syncState(ctx context.Context) error {
	next := run.currentState(ctx)
	for _, key := range []string{"stages", "decisions", "mode"} {
		if !reflect.DeepEqual(run.state[key], next[key]) {
			if err := run.update("/state/"+key, next[key]); err != nil {
				return err
			}
		}
	}
	run.state = next
	return nil
}

// chat plans the user's message: each phase into /steps, then the reply into /reply,
// the assistant's interface and the card as components, and any state change.
func (run *a2uiRun) chat(ctx context.Context, body chatRequest) (any, *chatTurnError) {
	steps := []map[string]any{}
	response, failure := run.s.runChatTurn(ctx, body, func(name string, started bool) {
		status := "done"
		if started {
			status = "running"
			steps = append(steps, map[string]any{"name": name, "label": chatStepLabels[name], "status": status})
		} else {
			for _, step := range steps {
				if step["name"] == name {
					step["status"] = status
				}
			}
		}
		_ = run.update("/steps", steps)
	})
	if failure != nil {
		_ = run.syncState(ctx) // a quick reply may have been kept before the turn failed
		return nil, failure
	}
	intent, _ := response["intent"].(map[string]interface{})
	reply, _ := intent["reply"].(string)
	text := ""
	for _, chunk := range replyChunks(reply, a2uiReplyChunk) {
		text += chunk
		_ = run.update("/reply", text)
	}
	components := []map[string]any{}
	body2 := []string{}
	if ui, _ := intent["ui"].([]map[string]any); len(ui) > 0 {
		components = append(components, ui...)
		body2 = append(body2, a2uiUIRoot)
	}
	if action, _ := intent["action"].(string); action != "" && action != "unknown" {
		_ = run.update("/intent", intent)
		if cleared, done := intent["cleared"]; done {
			// Clearing failed videos is done while planning; its outcome is known now.
			_ = run.update("/result", map[string]any{"status": "succeeded", "cleared": cleared})
		}
		components = append(components, map[string]any{
			"id": a2uiCardID, "component": "VideoFlowAction",
			"intent": map[string]any{"@path": "/intent"}, "result": map[string]any{"@path": "/result"},
		})
		body2 = append(body2, a2uiCardID)
	}
	if len(body2) > 0 {
		components = append(components, map[string]any{"id": "body", "component": "Column", "children": body2})
		_ = run.st.emit(a2uiMessage{UpdateComponents: &a2uiUpdateComponents{SurfaceID: run.surfaceID, Components: components}})
	}
	response["a2ui_surface"] = run.surfaceID
	_ = run.syncState(ctx)
	return response, nil
}

// chatStepLabels are the turn's phases as the chat shows them.
var chatStepLabels = map[string]string{
	chatStepReadProject: "Reading the project",
	chatStepAskSession:  "Asking the AI session",
	chatStepCheckAnswer: "Checking the answer",
}

// cardResult answers a videoflow.card_result action. With an op it follows the op on
// the event bus, updating /state when the project changes, and sets /result when the
// op ends; without one it records the reported outcome.
func (run *a2uiRun) cardResult(ctx context.Context, outcome map[string]any) (any, *chatTurnError) {
	opID, _ := outcome["op_id"].(string)
	if opID == "" {
		reported := map[string]any{"status": "succeeded"}
		if status, _ := outcome["status"].(string); status != "" {
			reported["status"] = status
		}
		if e, _ := outcome["error"].(string); e != "" {
			reported["error"] = e
		}
		_ = run.update("/result", reported)
		_ = run.syncState(ctx)
		return map[string]any{"a2ui_surface": run.surfaceID, "result": reported}, nil
	}
	op, failure := run.followOp(ctx, opID)
	if failure != nil {
		return nil, failure
	}
	result := map[string]any{"op_id": op.ID, "kind": op.Kind, "status": op.Status}
	if op.Error != nil {
		result["error"] = *op.Error
	}
	if len(op.ResultJSON) > 0 {
		result["result"] = op.ResultJSON
	}
	if op.Status != models.StatusRunning {
		_ = run.update("/result", plainJSON(result))
	}
	_ = run.syncState(ctx)
	return map[string]any{"a2ui_surface": run.surfaceID, "op": result}, nil
}

// followOp waits until the op ends, the client leaves or a2uiOpWait passes, and
// returns the op as it then stands. Every event on the bus may change the project, so
// one marks the state for a recompute; the op is also read every few seconds in case
// the bus dropped its op_done.
func (run *a2uiRun) followOp(ctx context.Context, opID string) (*models.Op, *chatTurnError) {
	events := run.s.broker.Subscribe()
	defer run.s.broker.Unsubscribe(events)
	read := func() (*models.Op, *chatTurnError) {
		op, err := run.s.database.GetOp(opID)
		if err != nil || op.ProjectID == nil || *op.ProjectID != run.projectID {
			return nil, turnFailure(http.StatusNotFound, "op not found")
		}
		return op, nil
	}
	op, failure := read()
	if failure != nil || op.Status != models.StatusRunning {
		return op, failure
	}
	deadline := time.NewTimer(a2uiOpWait)
	defer deadline.Stop()
	tick := time.NewTicker(a2uiStateEvery)
	defer tick.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	changed, ticks := false, 0
	for {
		select {
		case <-ctx.Done():
			return op, nil
		case <-deadline.C:
			return op, nil
		case <-heartbeat.C:
			run.st.heartbeat()
		case msg := <-events:
			changed = true
			if busEventEndsOp(msg, opID) {
				if op, failure = read(); failure != nil || op.Status != models.StatusRunning {
					return op, failure
				}
			}
		case <-tick.C:
			if ticks++; ticks%5 == 0 {
				if op, failure = read(); failure != nil || op.Status != models.StatusRunning {
					return op, failure
				}
			}
			if changed {
				changed = false
				if run.syncState(ctx) != nil {
					return op, nil
				}
			}
		}
	}
}

// busEventEndsOp says whether an event-bus message is the op_done of opID.
func busEventEndsOp(msg, opID string) bool {
	var event struct {
		Type string `json:"type"`
		OpID string `json:"op_id"`
	}
	data := strings.TrimSpace(strings.TrimPrefix(msg, "data: "))
	return json.Unmarshal([]byte(data), &event) == nil && event.Type == "op_done" && event.OpID == opID
}

// a2uiStream writes A2UI messages as server-sent events, one JSON message per data line.
type a2uiStream struct {
	w http.ResponseWriter
	f http.Flusher
}

// emit sends one message. A failed write means the client left; later writes fail too.
func (st *a2uiStream) emit(msg a2uiMessage) error {
	msg.Version = a2uiProtocolVersion
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintf(st.w, "data: %s\n\n", payload); err != nil {
		return err
	}
	st.f.Flush()
	return nil
}

// heartbeat keeps an idle stream open through proxies; clients ignore SSE comments.
func (st *a2uiStream) heartbeat() {
	fmt.Fprint(st.w, ": heartbeat\n\n")
	st.f.Flush()
}

// replyChunks splits a reply into the pieces it grows by: about size bytes each,
// ending after a space or newline, so words stay whole.
func replyChunks(reply string, size int) []string {
	chunks := []string{}
	for len(reply) > size {
		cut := strings.LastIndexAny(reply[:size], " \n")
		if cut <= 0 {
			cut = size
			for cut < len(reply) && !utf8Start(reply[cut]) {
				cut++
			}
		} else {
			cut++
		}
		chunks = append(chunks, reply[:cut])
		reply = reply[cut:]
	}
	if reply != "" {
		chunks = append(chunks, reply)
	}
	return chunks
}

// utf8Start says whether b begins a UTF-8 character (is not a continuation byte).
func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
