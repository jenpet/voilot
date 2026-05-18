package agent

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// --- Permission SSE event parsing tests ---
// Test payloads match the real OpenCode SSE format observed via curl.

func TestParsePermissionAsked(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	// Real payload from OpenCode: permission.asked
	props := json.RawMessage(`{
		"id": "per_abc123",
		"permission": "external_directory",
		"patterns": ["/etc/*"],
		"always": ["/etc/*"],
		"metadata": {"filepath": "/etc/hosts", "parentDir": "/etc"},
		"sessionID": "ses_test1",
		"tool": {"messageID": "msg_test1", "callID": "tooluse_xyz"}
	}`)

	events := adapter.parsePermissionAsked(props)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	evt := events[0]
	if evt.Type != EventPermissionRequest {
		t.Errorf("expected type %q, got %q", EventPermissionRequest, evt.Type)
	}
	if evt.SessionID != "ses_test1" {
		t.Errorf("expected sessionID %q, got %q", "ses_test1", evt.SessionID)
	}
	if evt.MessageID != "msg_test1" {
		t.Errorf("expected messageID %q, got %q", "msg_test1", evt.MessageID)
	}
	// Title should be generated from permission type + filepath
	if evt.Content != "external_directory: /etc/hosts" {
		t.Errorf("unexpected content: %q", evt.Content)
	}

	// Verify meta fields
	if evt.Meta["permissionId"] != "per_abc123" {
		t.Errorf("expected permissionId %q, got %v", "per_abc123", evt.Meta["permissionId"])
	}
	if evt.Meta["permissionType"] != "external_directory" {
		t.Errorf("expected permissionType %q, got %v", "external_directory", evt.Meta["permissionType"])
	}
	if evt.Meta["callID"] != "tooluse_xyz" {
		t.Errorf("expected callID %q, got %v", "tooluse_xyz", evt.Meta["callID"])
	}
	patterns, ok := evt.Meta["pattern"].([]string)
	if !ok {
		t.Fatalf("expected pattern to be []string, got %T", evt.Meta["pattern"])
	}
	if len(patterns) != 1 || patterns[0] != "/etc/*" {
		t.Errorf("expected pattern [/etc/*], got %v", patterns)
	}
}

func TestParsePermissionAsked_NoCallID(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	props := json.RawMessage(`{
		"id": "per_def456",
		"permission": "bash",
		"patterns": ["rm *"],
		"always": [],
		"metadata": {},
		"sessionID": "ses_test2",
		"tool": {"messageID": "msg_test2"}
	}`)

	events := adapter.parsePermissionAsked(props)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	evt := events[0]
	if evt.Type != EventPermissionRequest {
		t.Errorf("expected type %q, got %q", EventPermissionRequest, evt.Type)
	}
	// No filepath in metadata, so title is just the permission type
	if evt.Content != "bash" {
		t.Errorf("unexpected content: %q", evt.Content)
	}
	// callID should not be in meta when empty
	if _, exists := evt.Meta["callID"]; exists {
		t.Error("callID should not be in meta when empty")
	}
}

func TestParsePermissionAsked_InvalidJSON(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	events := adapter.parsePermissionAsked(json.RawMessage(`{invalid json`))
	if len(events) != 0 {
		t.Errorf("expected 0 events for invalid JSON, got %d", len(events))
	}
}

func TestParsePermissionReplied(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	// Real payload from OpenCode: permission.replied
	props := json.RawMessage(`{
		"sessionID": "ses_test1",
		"requestID": "per_abc123",
		"reply": "once"
	}`)

	events := adapter.parsePermissionReplied(props)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	evt := events[0]
	if evt.Type != EventPermissionReplied {
		t.Errorf("expected type %q, got %q", EventPermissionReplied, evt.Type)
	}
	if evt.SessionID != "ses_test1" {
		t.Errorf("expected sessionID %q, got %q", "ses_test1", evt.SessionID)
	}
	if evt.Content != "once" {
		t.Errorf("expected content %q, got %q", "once", evt.Content)
	}
	if evt.Meta["permissionId"] != "per_abc123" {
		t.Errorf("expected permissionId %q, got %v", "per_abc123", evt.Meta["permissionId"])
	}
	if evt.Meta["response"] != "once" {
		t.Errorf("expected response %q, got %v", "once", evt.Meta["response"])
	}
}

func TestParsePermissionReplied_Reject(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	props := json.RawMessage(`{
		"sessionID": "ses_test2",
		"requestID": "per_xyz",
		"reply": "reject"
	}`)

	events := adapter.parsePermissionReplied(props)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	evt := events[0]
	if evt.Content != "reject" {
		t.Errorf("expected content %q, got %q", "reject", evt.Content)
	}
	if evt.Meta["response"] != "reject" {
		t.Errorf("expected response %q, got %v", "reject", evt.Meta["response"])
	}
}

func TestParsePermissionReplied_InvalidJSON(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	events := adapter.parsePermissionReplied(json.RawMessage(`not json`))
	if len(events) != 0 {
		t.Errorf("expected 0 events for invalid JSON, got %d", len(events))
	}
}

// --- Permission SSE routing test (via parseSSEData) ---

func TestParseSSEData_PermissionAsked(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	data := `{
		"type": "permission.asked",
		"properties": {
			"id": "per_route1",
			"permission": "external_directory",
			"patterns": ["/etc/*"],
			"always": ["/etc/*"],
			"metadata": {"filepath": "/etc/hosts", "parentDir": "/etc"},
			"sessionID": "ses_route",
			"tool": {"messageID": "msg_route", "callID": "tooluse_route"}
		}
	}`

	events := adapter.parseSSEData(data)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventPermissionRequest {
		t.Errorf("expected type %q, got %q", EventPermissionRequest, events[0].Type)
	}
}

func TestParseSSEData_PermissionReplied(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	data := `{
		"type": "permission.replied",
		"properties": {
			"sessionID": "ses_route",
			"requestID": "per_route1",
			"reply": "always"
		}
	}`

	events := adapter.parseSSEData(data)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventPermissionReplied {
		t.Errorf("expected type %q, got %q", EventPermissionReplied, events[0].Type)
	}
	if events[0].Content != "always" {
		t.Errorf("expected content %q, got %q", "always", events[0].Content)
	}
}

// --- Tool part pending status regression test ---

func TestParseToolPart_PendingReturnsNil(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	part := OpenCodePart{
		ID:        "part-1",
		SessionID: "sess-1",
		MessageID: "msg-1",
		Type:      "tool",
		Tool:      "read",
		State:     json.RawMessage(`{"status": "pending", "input": {"path": "/some/file"}, "raw": "read /some/file"}`),
	}

	events := adapter.parseToolPart(part, "")
	if events != nil {
		t.Errorf("expected nil for pending tool part, got %d events", len(events))
	}
}

func TestParseToolPart_ErrorEmitsToolResult(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	part := OpenCodePart{
		ID:        "part-1",
		SessionID: "sess-1",
		MessageID: "msg-1",
		Type:      "tool",
		Tool:      "read",
		State:     json.RawMessage(`{"status": "error", "error": "Tool execution aborted", "input": {"filePath": "/var/log/system.log"}}`),
	}

	events := adapter.parseToolPart(part, "")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	evt := events[0]
	// Must be tool_result, NOT error — avoids redundant TTS/system messages
	if evt.Type != EventToolResult {
		t.Errorf("expected event type %q, got %q", EventToolResult, evt.Type)
	}
	if evt.Content != "Tool execution aborted" {
		t.Errorf("expected content %q, got %q", "Tool execution aborted", evt.Content)
	}
	if evt.Meta["error"] != "Tool execution aborted" {
		t.Errorf("expected meta error %q, got %v", "Tool execution aborted", evt.Meta["error"])
	}
	if evt.Meta["status"] != "error" {
		t.Errorf("expected meta status %q, got %v", "error", evt.Meta["status"])
	}
}

func TestParseMessageUpdated_SuppressesAbortError(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	props := json.RawMessage(`{
		"info": {
			"id": "msg-1",
			"sessionID": "sess-1",
			"role": "assistant",
			"error": {
				"name": "MessageAbortedError",
				"data": {"message": "The operation was aborted."}
			}
		}
	}`)

	events := adapter.parseMessageUpdated(props)
	if events != nil {
		t.Errorf("expected nil for MessageAbortedError, got %d events", len(events))
	}
}

func TestParseMessageUpdated_PropagatesOtherErrors(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	props := json.RawMessage(`{
		"info": {
			"id": "msg-1",
			"sessionID": "sess-1",
			"role": "assistant",
			"error": {
				"name": "RateLimitError",
				"data": {"message": "Too many requests"}
			}
		}
	}`)

	events := adapter.parseMessageUpdated(props)
	if len(events) != 1 {
		t.Fatalf("expected 1 event for non-abort error, got %d", len(events))
	}

	evt := events[0]
	if evt.Type != EventError {
		t.Errorf("expected event type %q, got %q", EventError, evt.Type)
	}
	if evt.Content != "Error (RateLimitError)" {
		t.Errorf("expected content %q, got %q", "Error (RateLimitError)", evt.Content)
	}
}

// --- RespondToPermission HTTP tests ---

func TestRespondToPermission_Success(t *testing.T) {
	var capturedPath, capturedMethod string
	var capturedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		w.WriteHeader(200)
		w.Write([]byte("true"))
	}))
	defer server.Close()

	adapter := NewOpenCodeAdapter(server.URL)
	err := adapter.RespondToPermission(context.Background(), "sess-1", "perm-abc", "once", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedMethod != "POST" {
		t.Errorf("expected POST, got %s", capturedMethod)
	}
	if capturedPath != "/session/sess-1/permissions/perm-abc" {
		t.Errorf("expected path %q, got %q", "/session/sess-1/permissions/perm-abc", capturedPath)
	}
	if capturedBody["response"] != "once" {
		t.Errorf("expected response %q, got %v", "once", capturedBody["response"])
	}
	// remember should not be in the body when false
	if _, exists := capturedBody["remember"]; exists {
		t.Error("remember should not be in body when false")
	}
}

func TestRespondToPermission_WithRemember(t *testing.T) {
	var capturedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		w.WriteHeader(200)
		w.Write([]byte("true"))
	}))
	defer server.Close()

	adapter := NewOpenCodeAdapter(server.URL)
	err := adapter.RespondToPermission(context.Background(), "sess-1", "perm-abc", "always", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedBody["response"] != "always" {
		t.Errorf("expected response %q, got %v", "always", capturedBody["response"])
	}
	if capturedBody["remember"] != true {
		t.Errorf("expected remember true, got %v", capturedBody["remember"])
	}
}

func TestRespondToPermission_Reject(t *testing.T) {
	var capturedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		w.WriteHeader(200)
		w.Write([]byte("true"))
	}))
	defer server.Close()

	adapter := NewOpenCodeAdapter(server.URL)
	err := adapter.RespondToPermission(context.Background(), "sess-1", "perm-abc", "reject", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedBody["response"] != "reject" {
		t.Errorf("expected response %q, got %v", "reject", capturedBody["response"])
	}
}

func TestRespondToPermission_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"error": "permission not found"}`))
	}))
	defer server.Close()

	adapter := NewOpenCodeAdapter(server.URL)
	err := adapter.RespondToPermission(context.Background(), "sess-1", "perm-missing", "once", false)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if err.Error() == "" {
		t.Error("error message should not be empty")
	}
}

// --- Question SSE event parsing tests ---

func TestParseQuestionAsked_SingleQuestion(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	props := json.RawMessage(`{
		"id": "question_1",
		"sessionID": "ses_q1",
		"questions": [
			{
				"question": "What HTTP method should the endpoint use?",
				"header": "HTTP Method",
				"options": [
					{"label": "GET", "description": "Read-only endpoint"},
					{"label": "POST", "description": "Create a new resource"},
					{"label": "PUT", "description": "Update an existing resource"},
					{"label": "DELETE", "description": "Remove a resource"}
				],
				"multiple": false
			}
		],
		"tool": {"messageID": "msg_q1", "callID": "call_q1"}
	}`)

	events := adapter.parseQuestionAsked(props)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	evt := events[0]
	if evt.Type != EventQuestionRequest {
		t.Errorf("expected type %q, got %q", EventQuestionRequest, evt.Type)
	}
	if evt.SessionID != "ses_q1" {
		t.Errorf("expected sessionID %q, got %q", "ses_q1", evt.SessionID)
	}
	if evt.Content != "What HTTP method should the endpoint use?" {
		t.Errorf("unexpected content: %q", evt.Content)
	}
	if evt.Meta["questionId"] != "question_1" {
		t.Errorf("expected questionId %q, got %v", "question_1", evt.Meta["questionId"])
	}
	if evt.Meta["questionIndex"] != 0 {
		t.Errorf("expected questionIndex 0, got %v", evt.Meta["questionIndex"])
	}
	if evt.Meta["totalQuestions"] != 1 {
		t.Errorf("expected totalQuestions 1, got %v", evt.Meta["totalQuestions"])
	}
	if evt.Meta["header"] != "HTTP Method" {
		t.Errorf("expected header %q, got %v", "HTTP Method", evt.Meta["header"])
	}
	if evt.Meta["multiple"] != false {
		t.Errorf("expected multiple false, got %v", evt.Meta["multiple"])
	}

	// Verify options are passed through
	options, ok := evt.Meta["options"].([]interface{})
	if !ok {
		t.Fatalf("expected options to be []interface{}, got %T", evt.Meta["options"])
	}
	if len(options) != 4 {
		t.Fatalf("expected 4 options, got %d", len(options))
	}
	firstOpt, ok := options[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected option to be map[string]interface{}, got %T", options[0])
	}
	if firstOpt["label"] != "GET" {
		t.Errorf("expected first option label %q, got %v", "GET", firstOpt["label"])
	}
	if firstOpt["description"] != "Read-only endpoint" {
		t.Errorf("expected first option description %q, got %v", "Read-only endpoint", firstOpt["description"])
	}
}

func TestParseQuestionAsked_MultiQuestion(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	props := json.RawMessage(`{
		"id": "question_2",
		"sessionID": "ses_q2",
		"questions": [
			{
				"question": "What should the composable be named?",
				"header": "Name",
				"options": [
					{"label": "useMetrics", "description": "Metrics tracking"},
					{"label": "useAnalytics", "description": "Analytics tracking"},
					{"label": "useTracking", "description": "General tracking"}
				]
			},
			{
				"question": "Include cleanup on unmount?",
				"header": "Cleanup",
				"options": [
					{"label": "Yes", "description": "Add cleanup function"},
					{"label": "No", "description": "Skip cleanup"}
				]
			},
			{
				"question": "What data format?",
				"header": "Format",
				"options": [
					{"label": "raw object", "description": "Plain object"},
					{"label": "readonly ref", "description": "Vue readonly ref"},
					{"label": "computed", "description": "Computed property"}
				]
			}
		],
		"tool": {"messageID": "msg_q2", "callID": "call_q2"}
	}`)

	events := adapter.parseQuestionAsked(props)
	if len(events) != 3 {
		t.Fatalf("expected 3 events (one per question), got %d", len(events))
	}

	// Verify each event has correct index and totalQuestions
	for i, evt := range events {
		if evt.Type != EventQuestionRequest {
			t.Errorf("event %d: expected type %q, got %q", i, EventQuestionRequest, evt.Type)
		}
		if evt.Meta["questionId"] != "question_2" {
			t.Errorf("event %d: expected questionId %q, got %v", i, "question_2", evt.Meta["questionId"])
		}
		if evt.Meta["questionIndex"] != i {
			t.Errorf("event %d: expected questionIndex %d, got %v", i, i, evt.Meta["questionIndex"])
		}
		if evt.Meta["totalQuestions"] != 3 {
			t.Errorf("event %d: expected totalQuestions 3, got %v", i, evt.Meta["totalQuestions"])
		}
	}

	// Verify individual question content
	if events[0].Content != "What should the composable be named?" {
		t.Errorf("event 0: unexpected content: %q", events[0].Content)
	}
	if events[1].Content != "Include cleanup on unmount?" {
		t.Errorf("event 1: unexpected content: %q", events[1].Content)
	}
	if events[2].Content != "What data format?" {
		t.Errorf("event 2: unexpected content: %q", events[2].Content)
	}
}

func TestParseQuestionAsked_NoOptions(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	// A question with no predefined options (free-form input expected)
	props := json.RawMessage(`{
		"id": "question_3",
		"sessionID": "ses_q3",
		"questions": [
			{
				"question": "What should the project be named?",
				"header": "Project Name",
				"options": []
			}
		],
		"tool": {"messageID": "msg_q3", "callID": "call_q3"}
	}`)

	events := adapter.parseQuestionAsked(props)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	evt := events[0]
	options, ok := evt.Meta["options"].([]interface{})
	if !ok {
		t.Fatalf("expected options to be []interface{}, got %T", evt.Meta["options"])
	}
	if len(options) != 0 {
		t.Errorf("expected 0 options, got %d", len(options))
	}
}

func TestParseQuestionAsked_InvalidJSON(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	events := adapter.parseQuestionAsked(json.RawMessage(`{broken`))
	if len(events) != 0 {
		t.Errorf("expected 0 events for invalid JSON, got %d", len(events))
	}
}

func TestParseQuestionAsked_EmptyQuestions(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	props := json.RawMessage(`{
		"id": "question_4",
		"sessionID": "ses_q4",
		"questions": [],
		"tool": {"messageID": "msg_q4"}
	}`)

	events := adapter.parseQuestionAsked(props)
	if events != nil {
		t.Errorf("expected nil for empty questions, got %d events", len(events))
	}
}

func TestParseQuestionReplied(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	props := json.RawMessage(`{
		"sessionID": "ses_q1",
		"requestID": "question_1",
		"answers": [["POST"]]
	}`)

	events := adapter.parseQuestionReplied(props)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	evt := events[0]
	if evt.Type != EventQuestionReplied {
		t.Errorf("expected type %q, got %q", EventQuestionReplied, evt.Type)
	}
	if evt.SessionID != "ses_q1" {
		t.Errorf("expected sessionID %q, got %q", "ses_q1", evt.SessionID)
	}
	if evt.Content != "POST" {
		t.Errorf("expected content %q, got %q", "POST", evt.Content)
	}
	if evt.Meta["questionId"] != "question_1" {
		t.Errorf("expected questionId %q, got %v", "question_1", evt.Meta["questionId"])
	}
	if evt.Meta["rejected"] != false {
		t.Errorf("expected rejected false, got %v", evt.Meta["rejected"])
	}
}

func TestParseQuestionReplied_MultiAnswer(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	props := json.RawMessage(`{
		"sessionID": "ses_q2",
		"requestID": "question_2",
		"answers": [["useMetrics"], ["Yes"], ["readonly ref"]]
	}`)

	events := adapter.parseQuestionReplied(props)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	evt := events[0]
	if evt.Content != "useMetrics; Yes; readonly ref" {
		t.Errorf("unexpected content: %q", evt.Content)
	}
}

func TestParseQuestionReplied_InvalidJSON(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	events := adapter.parseQuestionReplied(json.RawMessage(`{not valid`))
	if len(events) != 0 {
		t.Errorf("expected 0 events for invalid JSON, got %d", len(events))
	}
}

func TestParseQuestionRejected(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	props := json.RawMessage(`{
		"sessionID": "ses_q1",
		"requestID": "question_1"
	}`)

	events := adapter.parseQuestionRejected(props)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	evt := events[0]
	if evt.Type != EventQuestionReplied {
		t.Errorf("expected type %q, got %q", EventQuestionReplied, evt.Type)
	}
	if evt.Meta["rejected"] != true {
		t.Errorf("expected rejected true, got %v", evt.Meta["rejected"])
	}
	if evt.Meta["questionId"] != "question_1" {
		t.Errorf("expected questionId %q, got %v", "question_1", evt.Meta["questionId"])
	}
	if evt.Content != "Question dismissed" {
		t.Errorf("expected content %q, got %q", "Question dismissed", evt.Content)
	}
}

func TestParseQuestionRejected_InvalidJSON(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	events := adapter.parseQuestionRejected(json.RawMessage(`{bad`))
	if len(events) != 0 {
		t.Errorf("expected 0 events for invalid JSON, got %d", len(events))
	}
}

// --- Question SSE routing tests (via parseSSEData) ---

func TestParseSSEData_QuestionAsked(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	data := `{
		"type": "question.asked",
		"properties": {
			"id": "question_route1",
			"sessionID": "ses_route",
			"questions": [
				{
					"question": "Pick a color",
					"header": "Color",
					"options": [
						{"label": "Red", "description": "Warm color"},
						{"label": "Blue", "description": "Cool color"}
					]
				}
			],
			"tool": {"messageID": "msg_route", "callID": "call_route"}
		}
	}`

	events := adapter.parseSSEData(data)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventQuestionRequest {
		t.Errorf("expected type %q, got %q", EventQuestionRequest, events[0].Type)
	}
}

func TestParseSSEData_QuestionReplied(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	data := `{
		"type": "question.replied",
		"properties": {
			"sessionID": "ses_route",
			"requestID": "question_route1",
			"answers": [["Blue"]]
		}
	}`

	events := adapter.parseSSEData(data)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventQuestionReplied {
		t.Errorf("expected type %q, got %q", EventQuestionReplied, events[0].Type)
	}
	if events[0].Content != "Blue" {
		t.Errorf("expected content %q, got %q", "Blue", events[0].Content)
	}
}

func TestParseSSEData_QuestionRejected(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	data := `{
		"type": "question.rejected",
		"properties": {
			"sessionID": "ses_route",
			"requestID": "question_route1"
		}
	}`

	events := adapter.parseSSEData(data)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventQuestionReplied {
		t.Errorf("expected type %q, got %q", EventQuestionReplied, events[0].Type)
	}
	if events[0].Meta["rejected"] != true {
		t.Errorf("expected rejected true, got %v", events[0].Meta["rejected"])
	}
}

// --- Question tool suppression test ---

func TestParseToolPart_QuestionToolSkipped(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	// The "question" tool should be suppressed — it's handled via question.asked events
	part := OpenCodePart{
		ID:        "part-q1",
		SessionID: "sess-1",
		MessageID: "msg-1",
		Type:      "tool",
		Tool:      "question",
		State:     json.RawMessage(`{"status": "running", "input": {"questions": [{"question": "Pick one"}]}}`),
	}

	events := adapter.parseToolPart(part, "")
	if events != nil {
		t.Errorf("expected nil for question tool part, got %d events", len(events))
	}
}

func TestParseToolPart_QuestionToolCompletedSkipped(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://localhost:4096")

	part := OpenCodePart{
		ID:        "part-q2",
		SessionID: "sess-1",
		MessageID: "msg-1",
		Type:      "tool",
		Tool:      "question",
		State:     json.RawMessage(`{"status": "completed", "title": "Asked 1 question(s)", "output": "User has answered your questions: POST"}`),
	}

	events := adapter.parseToolPart(part, "")
	if events != nil {
		t.Errorf("expected nil for question tool completion, got %d events", len(events))
	}
}

// --- RespondToQuestion HTTP tests ---

func TestRespondToQuestion_Success(t *testing.T) {
	var capturedPath, capturedMethod string
	var capturedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		w.WriteHeader(200)
	}))
	defer server.Close()

	adapter := NewOpenCodeAdapter(server.URL)
	err := adapter.RespondToQuestion(context.Background(), "question_1", [][]string{{"POST"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedMethod != "POST" {
		t.Errorf("expected POST, got %s", capturedMethod)
	}
	if capturedPath != "/question/question_1/reply" {
		t.Errorf("expected path %q, got %q", "/question/question_1/reply", capturedPath)
	}

	answers, ok := capturedBody["answers"].([]interface{})
	if !ok {
		t.Fatalf("expected answers to be array, got %T", capturedBody["answers"])
	}
	if len(answers) != 1 {
		t.Fatalf("expected 1 answer group, got %d", len(answers))
	}
}

func TestRespondToQuestion_MultiAnswer(t *testing.T) {
	var capturedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		w.WriteHeader(200)
	}))
	defer server.Close()

	adapter := NewOpenCodeAdapter(server.URL)
	err := adapter.RespondToQuestion(context.Background(), "question_2", [][]string{
		{"useMetrics"},
		{"Yes"},
		{"readonly ref"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	answers, ok := capturedBody["answers"].([]interface{})
	if !ok {
		t.Fatalf("expected answers to be array, got %T", capturedBody["answers"])
	}
	if len(answers) != 3 {
		t.Errorf("expected 3 answer groups, got %d", len(answers))
	}
}

func TestRespondToQuestion_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"error": "question not found"}`))
	}))
	defer server.Close()

	adapter := NewOpenCodeAdapter(server.URL)
	err := adapter.RespondToQuestion(context.Background(), "question_missing", [][]string{{"x"}})
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

// --- RejectQuestion HTTP tests ---

func TestRejectQuestion_Success(t *testing.T) {
	var capturedPath, capturedMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedMethod = r.Method
		w.WriteHeader(200)
	}))
	defer server.Close()

	adapter := NewOpenCodeAdapter(server.URL)
	err := adapter.RejectQuestion(context.Background(), "question_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedMethod != "POST" {
		t.Errorf("expected POST, got %s", capturedMethod)
	}
	if capturedPath != "/question/question_1/reject" {
		t.Errorf("expected path %q, got %q", "/question/question_1/reject", capturedPath)
	}
}

func TestRejectQuestion_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`internal server error`))
	}))
	defer server.Close()

	adapter := NewOpenCodeAdapter(server.URL)
	err := adapter.RejectQuestion(context.Background(), "question_missing")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

// --- SSE lifecycle / Close tests ---

func TestClose_StopsSSEReader(t *testing.T) {
	// Start a server that accepts SSE connections and blocks.
	connected := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/event" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			close(connected)
			// Block until client disconnects.
			<-r.Context().Done()
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	adapter := NewOpenCodeAdapter(server.URL)
	defer adapter.Close()

	// Subscribe to start the SSE reader.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := adapter.SubscribeEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for SSE connection to be established.
	<-connected

	// Close the adapter — should stop the SSE reader.
	adapter.Close()

	// Verify sseRunning becomes false (reader exits).
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("SSE reader did not stop within 2s after Close()")
		default:
		}
		adapter.mu.RLock()
		running := adapter.sseRunning
		adapter.mu.RUnlock()
		if !running {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestClose_StopsReconnectLoop(t *testing.T) {
	// Server that always refuses connections (use a closed listener port).
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close() // Immediately close so all connections are refused.

	adapter := NewOpenCodeAdapter("http://" + addr)

	// Subscribe to start the SSE reader (which will enter reconnect loop).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err = adapter.SubscribeEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Give it a moment to enter the reconnect loop.
	time.Sleep(50 * time.Millisecond)

	// Close should stop the reconnect loop promptly.
	adapter.Close()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("SSE reconnect loop did not stop within 2s after Close()")
		default:
		}
		adapter.mu.RLock()
		running := adapter.sseRunning
		adapter.mu.RUnlock()
		if !running {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestClose_IsIdempotent(t *testing.T) {
	adapter := NewOpenCodeAdapter("http://127.0.0.1:1")
	// Multiple closes should not panic.
	adapter.Close()
	adapter.Close()
	adapter.Close()
}

func TestClose_InFlightRequestAborted(t *testing.T) {
	// Server that accepts the SSE connection but never sends data.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/event" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			// Block until client disconnects.
			<-r.Context().Done()
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	adapter := NewOpenCodeAdapter(server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := adapter.SubscribeEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for connection to be established.
	time.Sleep(100 * time.Millisecond)

	// Close should abort the in-flight HTTP request and exit quickly.
	start := time.Now()
	adapter.Close()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("adapter did not stop in time")
		default:
		}
		adapter.mu.RLock()
		running := adapter.sseRunning
		adapter.mu.RUnlock()
		if !running {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	elapsed := time.Since(start)
	if elapsed > 1*time.Second {
		t.Fatalf("Close took too long: %v (expected < 1s)", elapsed)
	}
}
