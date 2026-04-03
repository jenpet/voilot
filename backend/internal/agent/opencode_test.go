package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
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
