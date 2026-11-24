package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// AG-UI (https://docs.ag-ui.com, protocol 1.0) is the open event protocol POST /agui
// speaks. The types below are the subset of its events this server sends, matched
// field for field to the protocol's JSON Schema as published in @ag-ui/core 1.0.5.
// The community Go SDK is not used: it has no tagged release and pulls in a logger.

// aguiProtocolVersion is the AG-UI protocol version these events follow.
const aguiProtocolVersion = "1.0"

// AG-UI event types.
const (
	aguiRunStarted         = "RUN_STARTED"
	aguiRunFinished        = "RUN_FINISHED"
	aguiRunError           = "RUN_ERROR"
	aguiStepStarted        = "STEP_STARTED"
	aguiStepFinished       = "STEP_FINISHED"
	aguiTextMessageStart   = "TEXT_MESSAGE_START"
	aguiTextMessageContent = "TEXT_MESSAGE_CONTENT"
	aguiTextMessageEnd     = "TEXT_MESSAGE_END"
	aguiToolCallStart      = "TOOL_CALL_START"
	aguiToolCallArgs       = "TOOL_CALL_ARGS"
	aguiToolCallEnd        = "TOOL_CALL_END"
	aguiToolCallResult     = "TOOL_CALL_RESULT"
	aguiStateSnapshot      = "STATE_SNAPSHOT"
	aguiStateDelta         = "STATE_DELTA"
)

type aguiRunStartedEvent struct {
	Type            string `json:"type"`
	ThreadID        string `json:"threadId"`
	RunID           string `json:"runId"`
	ProtocolVersion string `json:"protocolVersion,omitempty"`
	ParentRunID     string `json:"parentRunId,omitempty"`
}

type aguiRunFinishedEvent struct {
	Type     string `json:"type"`
	ThreadID string `json:"threadId"`
	RunID    string `json:"runId"`
	Result   any    `json:"result,omitempty"`
}

type aguiRunErrorEvent struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

type aguiStepEvent struct {
	Type     string `json:"type"`
	StepName string `json:"stepName"`
}

type aguiTextMessageStartEvent struct {
	Type      string `json:"type"`
	MessageID string `json:"messageId"`
	Role      string `json:"role,omitempty"`
}

type aguiTextMessageContentEvent struct {
	Type      string `json:"type"`
	MessageID string `json:"messageId"`
	Delta     string `json:"delta"`
}

type aguiTextMessageEndEvent struct {
	Type      string `json:"type"`
	MessageID string `json:"messageId"`
}

type aguiToolCallStartEvent struct {
	Type            string `json:"type"`
	ToolCallID      string `json:"toolCallId"`
	ToolCallName    string `json:"toolCallName"`
	ParentMessageID string `json:"parentMessageId,omitempty"`
}

type aguiToolCallArgsEvent struct {
	Type       string `json:"type"`
	ToolCallID string `json:"toolCallId"`
	Delta      string `json:"delta"`
}

type aguiToolCallEndEvent struct {
	Type       string `json:"type"`
	ToolCallID string `json:"toolCallId"`
}

type aguiToolCallResultEvent struct {
	Type       string `json:"type"`
	MessageID  string `json:"messageId"`
	ToolCallID string `json:"toolCallId"`
	Content    string `json:"content"`
	Role       string `json:"role,omitempty"`
}

type aguiStateSnapshotEvent struct {
	Type     string `json:"type"`
	Snapshot any    `json:"snapshot"`
}

type aguiStateDeltaEvent struct {
	Type  string    `json:"type"`
	Delta []patchOp `json:"delta"`
}

// aguiRunInput is AG-UI's RunAgentInput, the body of POST /agui. VideoFlow's own
// chat fields (selected item, attachments, auto mode, a quick reply's answer, the
// creative brief) travel in ForwardedProps, the protocol's place for them.
type aguiRunInput struct {
	ThreadID       string            `json:"threadId"`
	RunID          string            `json:"runId"`
	ParentRunID    string            `json:"parentRunId,omitempty"`
	State          json.RawMessage   `json:"state,omitempty"`
	Messages       []aguiMessage     `json:"messages"`
	Tools          []json.RawMessage `json:"tools"`
	Context        []aguiContext     `json:"context"`
	ForwardedProps json.RawMessage   `json:"forwardedProps,omitempty"`
}

// aguiMessage is one message of the run's conversation. Content is a string, or for
// a user message an array of content parts.
type aguiMessage struct {
	ID         string          `json:"id"`
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	ToolCallID string          `json:"toolCallId,omitempty"`
}

type aguiContext struct {
	Description string `json:"description"`
	Value       string `json:"value"`
}

// text is the message's text: its string content, or its text parts joined.
func (m aguiMessage) text() string {
	var plain string
	if json.Unmarshal(m.Content, &plain) == nil {
		return plain
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(m.Content, &parts) != nil {
		return ""
	}
	texts := []string{}
	for _, p := range parts {
		if p.Type == "text" {
			texts = append(texts, p.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// aguiStream writes AG-UI events as server-sent events, one JSON event per data line.
type aguiStream struct {
	w http.ResponseWriter
	f http.Flusher
}

// emit sends one event. A failed write means the client left; later writes fail too.
func (st *aguiStream) emit(event any) error {
	payload, err := json.Marshal(event)
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
func (st *aguiStream) heartbeat() {
	fmt.Fprint(st.w, ": heartbeat\n\n")
	st.f.Flush()
}

// replyChunks splits a reply into the TEXT_MESSAGE_CONTENT deltas it is streamed as:
// pieces of about size bytes that end after a space or newline, so words stay whole.
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
