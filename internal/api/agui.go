package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"videoflow-go/internal/models"
)

// POST /agui is the chat assistant over AG-UI: the same turn as POST /chat, streamed as
// events (docs/handbook/12-reference.md lists them). A run whose last message is the
// user's plans that message. A run whose last message is a tool message reports what
// became of a card the user ran: when it names a background op, the run follows the op
// on the event bus and keeps the state in step until the op ends.

// aguiReplyChunk is about how many bytes of the reply each TEXT_MESSAGE_CONTENT holds.
const aguiReplyChunk = 48

// aguiOpWait is the longest a run follows one background op before it reports the op
// as still running.
var aguiOpWait = 30 * time.Minute

// aguiStateEvery is how often a run following an op recomputes the state after the
// event bus reported a change.
var aguiStateEvery = time.Second

func (s *Server) handleAGUI(w http.ResponseWriter, r *http.Request) {
	var input aguiRunInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.ThreadID == "" || input.RunID == "" || len(input.Messages) == 0 {
		http.Error(w, "an AG-UI RunAgentInput with threadId, runId and messages is required", http.StatusBadRequest)
		return
	}
	var props chatRequest
	if len(input.ForwardedProps) > 0 && string(input.ForwardedProps) != "null" {
		if err := json.Unmarshal(input.ForwardedProps, &props); err != nil {
			http.Error(w, "forwardedProps do not match the chat request", http.StatusBadRequest)
			return
		}
	}
	last := input.Messages[len(input.Messages)-1]
	if last.Role != "user" && !(last.Role == "tool" && last.ToolCallID != "") {
		http.Error(w, "the last message must be the user's, or a tool result with its toolCallId", http.StatusBadRequest)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}
	// A run can follow a background op for longer than the server's write limit.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	st := &aguiStream{w: w, f: flusher}
	run := &aguiRun{s: s, st: st, input: input, mode: aguiMode(props.Auto, props.Full)}
	run.projectID, _ = s.activeProjectID(r.Context())
	if st.emit(aguiRunStartedEvent{Type: aguiRunStarted, ThreadID: input.ThreadID, RunID: input.RunID, ParentRunID: input.ParentRunID, ProtocolVersion: aguiProtocolVersion}) != nil {
		return
	}
	run.state = run.currentState(r.Context())
	if st.emit(aguiStateSnapshotEvent{Type: aguiStateSnapshot, Snapshot: run.state}) != nil {
		return
	}
	var result any
	var failure *chatTurnError
	if last.Role == "user" {
		props.Message = strings.TrimSpace(last.text())
		result, failure = run.chat(r.Context(), props)
	} else {
		result, failure = run.toolResult(r.Context(), last)
	}
	if r.Context().Err() != nil {
		return // the client left; there is no one to tell
	}
	if failure != nil {
		code := failure.code
		if code == "" {
			code = "http_" + strconv.Itoa(failure.status)
		}
		_ = st.emit(aguiRunErrorEvent{Type: aguiRunError, Message: failure.message, Code: code})
		return
	}
	_ = st.emit(aguiRunFinishedEvent{Type: aguiRunFinished, ThreadID: input.ThreadID, RunID: input.RunID, Result: result})
}

// aguiRun is one POST /agui run: its stream and the state the client holds so far.
type aguiRun struct {
	s         *Server
	st        *aguiStream
	input     aguiRunInput
	projectID string
	mode      string
	// state is the last STATE_SNAPSHOT or STATE_DELTA result sent, as plain JSON.
	state any
}

// aguiMode names the chat mode as the state carries it.
func aguiMode(auto, full bool) string {
	switch {
	case full:
		return "full"
	case auto:
		return "auto"
	}
	return "manual"
}

// currentState is the shared AG-UI state: the project's stage map (GET /stages), its
// decision log (GET /decisions) and the chat mode. A part that cannot be read is null.
func (run *aguiRun) currentState(ctx context.Context) any {
	state := map[string]any{"stages": nil, "decisions": nil, "mode": run.mode}
	if run.projectID != "" {
		if stages, err := run.s.projectStages(ctx, run.projectID); err == nil {
			state["stages"] = stages
		}
		if decisions, err := run.s.database.ListDecisions(run.projectID); err == nil {
			state["decisions"] = decisions
		}
	}
	return jsonValue(state)
}

// syncState sends a STATE_DELTA when the stage map or decision log changed since the
// client's copy.
func (run *aguiRun) syncState(ctx context.Context) error {
	next := run.currentState(ctx)
	delta := diffJSON(run.state, next)
	if len(delta) == 0 {
		return nil
	}
	run.state = next
	return run.st.emit(aguiStateDeltaEvent{Type: aguiStateDelta, Delta: delta})
}

// chat plans the user's message: each phase as a step, then the reply as a text
// message, the prepared card as a tool call and any state change as a delta. The run's
// result is the POST /chat response.
func (run *aguiRun) chat(ctx context.Context, body chatRequest) (any, *chatTurnError) {
	if body.Message == "" {
		return nil, turnFailure(http.StatusBadRequest, "message is required")
	}
	response, failure := run.s.runChatTurn(ctx, body, func(name string, started bool) {
		kind := aguiStepFinished
		if started {
			kind = aguiStepStarted
		}
		_ = run.st.emit(aguiStepEvent{Type: kind, StepName: name})
	})
	if failure != nil {
		_ = run.syncState(ctx) // a quick reply may have been kept before the turn failed
		return nil, failure
	}
	intent, _ := response["intent"].(map[string]interface{})
	messageID := uuid.NewString()
	reply, _ := intent["reply"].(string)
	_ = run.st.emit(aguiTextMessageStartEvent{Type: aguiTextMessageStart, MessageID: messageID, Role: "assistant"})
	for _, chunk := range replyChunks(reply, aguiReplyChunk) {
		_ = run.st.emit(aguiTextMessageContentEvent{Type: aguiTextMessageContent, MessageID: messageID, Delta: chunk})
	}
	_ = run.st.emit(aguiTextMessageEndEvent{Type: aguiTextMessageEnd, MessageID: messageID})
	if action, _ := intent["action"].(string); action != "" && action != "unknown" {
		toolCallID := uuid.NewString()
		args, _ := json.Marshal(intent)
		response["tool_call_id"] = toolCallID
		_ = run.st.emit(aguiToolCallStartEvent{Type: aguiToolCallStart, ToolCallID: toolCallID, ToolCallName: action, ParentMessageID: messageID})
		_ = run.st.emit(aguiToolCallArgsEvent{Type: aguiToolCallArgs, ToolCallID: toolCallID, Delta: string(args)})
		_ = run.st.emit(aguiToolCallEndEvent{Type: aguiToolCallEnd, ToolCallID: toolCallID})
		if cleared, done := intent["cleared"]; done {
			// Clearing failed videos is done while planning; its result is known now.
			content, _ := json.Marshal(map[string]any{"status": "succeeded", "cleared": cleared})
			_ = run.st.emit(aguiToolCallResultEvent{Type: aguiToolCallResult, MessageID: uuid.NewString(), ToolCallID: toolCallID, Content: string(content), Role: "tool"})
		}
	}
	_ = run.syncState(ctx)
	return response, nil
}

// aguiCardOutcome is what the client reports when the user ran a card: the background
// op it started, or how it ended when it ran at once.
type aguiCardOutcome struct {
	OpID   string `json:"op_id"`
	Status string `json:"status"`
	Error  string `json:"error"`
}

// toolResult answers a run that reports a card the user ran. With an op it follows
// the op on the event bus, sending a STATE_DELTA when the project changes, and fills
// the tool call's result when the op ends; without one it records the reported
// outcome.
func (run *aguiRun) toolResult(ctx context.Context, msg aguiMessage) (any, *chatTurnError) {
	var outcome aguiCardOutcome
	content := msg.text()
	_ = json.Unmarshal([]byte(content), &outcome) // a plain-text result has no op to follow
	if outcome.OpID == "" {
		if content == "" {
			content = `{"status":"succeeded"}`
		}
		_ = run.st.emit(aguiToolCallResultEvent{Type: aguiToolCallResult, MessageID: uuid.NewString(), ToolCallID: msg.ToolCallID, Content: content, Role: "tool"})
		_ = run.syncState(ctx)
		return map[string]any{"tool_call_id": msg.ToolCallID}, nil
	}
	op, failure := run.followOp(ctx, outcome.OpID)
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
	resultJSON, _ := json.Marshal(result)
	if op.Status != models.StatusRunning {
		_ = run.st.emit(aguiToolCallResultEvent{Type: aguiToolCallResult, MessageID: uuid.NewString(), ToolCallID: msg.ToolCallID, Content: string(resultJSON), Role: "tool"})
	}
	_ = run.syncState(ctx)
	return map[string]any{"tool_call_id": msg.ToolCallID, "op": result}, nil
}

// followOp waits until the op ends, the client leaves or aguiOpWait passes, and
// returns the op as it then stands. Every event on the bus may change the project, so
// one marks the state for a recompute; the op is also read every few seconds in case
// the bus dropped its op_done.
func (run *aguiRun) followOp(ctx context.Context, opID string) (*models.Op, *chatTurnError) {
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
	deadline := time.NewTimer(aguiOpWait)
	defer deadline.Stop()
	tick := time.NewTicker(aguiStateEvery)
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
