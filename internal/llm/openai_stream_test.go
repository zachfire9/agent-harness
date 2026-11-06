package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zachfire9/agent-harness/internal/llm"
)

var _ llm.StreamingChatClient = (*llm.OpenAIClient)(nil)

// sseServer replies with the given SSE lines, flushing after each and pausing
// `gap` between them. It records the request body and Accept header.
func sseServer(t *testing.T, gap time.Duration, lines ...string) (*httptest.Server, *map[string]any, *string) {
	t.Helper()
	var body map[string]any
	var accept string
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		accept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		flusher := w.(http.Flusher)
		for _, line := range lines {
			if line == "<stall>" {
				select {
				case <-release:
				case <-r.Context().Done():
				}
				return
			}
			fmt.Fprintf(w, "%s\n", line)
			flusher.Flush()
			if gap > 0 {
				time.Sleep(gap)
			}
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	return server, &body, &accept
}

func textEvent(text string) string {
	payload, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": text}}}})
	return "data: " + string(payload) + "\n"
}

func streamRequest() llm.ChatRequest {
	return llm.ChatRequest{Model: "test-model", Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}}}
}

func TestOpenAIClientStreamsTextInOrder(t *testing.T) {
	server, body, accept := sseServer(t, 0,
		`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`, "",
		textEvent("Hel"), textEvent("lo, "), textEvent("world"),
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`, "",
		"data: [DONE]", "")
	client := llm.NewOpenAIClient(server.URL, "test-api-key")

	var fragments []string
	response, err := client.ChatStream(context.Background(), streamRequest(), func(text string) { fragments = append(fragments, text) })
	if err != nil {
		t.Fatalf("chat stream: %v", err)
	}
	if strings.Join(fragments, "|") != "Hel|lo, |world" {
		t.Fatalf("expected fragments in order, got %q", fragments)
	}
	if response.Message.Content != "Hello, world" || response.Message.Role != llm.RoleAssistant || !response.IsFinalAnswer() {
		t.Fatalf("unexpected assembled response %#v", response)
	}
	if (*body)["stream"] != true || *accept != "text/event-stream" {
		t.Fatalf("expected a streaming request, got body=%v accept=%q", *body, *accept)
	}
}

func TestOpenAIClientReassemblesStreamedToolCalls(t *testing.T) {
	// Two interleaved calls; arguments split mid-token, as providers do.
	server, _, _ := sseServer(t, 0,
		textEvent("Checking."),
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"read_file","arguments":""}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"pa"}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"search_files","arguments":"{\"query\":"}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"th\":\"README.md\"}"}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"\"agent\"}"}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		"data: [DONE]")
	client := llm.NewOpenAIClient(server.URL, "test-api-key")

	var text strings.Builder
	response, err := client.ChatStream(context.Background(), streamRequest(), func(fragment string) { text.WriteString(fragment) })
	if err != nil {
		t.Fatalf("chat stream: %v", err)
	}
	if text.String() != "Checking." {
		t.Fatalf("tool-call fragments must not be sent as text, got %q", text.String())
	}
	if len(response.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %#v", response.ToolCalls)
	}
	want := []struct{ id, name, args string }{
		{"call_a", "read_file", `{"path":"README.md"}`},
		{"call_b", "search_files", `{"query":"agent"}`},
	}
	for i, w := range want {
		got := response.ToolCalls[i]
		if got.ID != w.id || got.Name != w.name || string(got.Arguments) != w.args {
			t.Fatalf("tool call %d: got %s %s %s, want %s %s %s", i, got.ID, got.Name, got.Arguments, w.id, w.name, w.args)
		}
	}
	if len(response.Message.ToolCalls) != 2 {
		t.Fatalf("expected tool calls on the assistant message too, got %#v", response.Message)
	}
}

func TestOpenAIClientStreamToolCallWithoutIndexOrRepeatedName(t *testing.T) {
	// Some compatible servers omit index and repeat the full name each time.
	server, _, _ := sseServer(t, 0,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_1","function":{"name":"echo","arguments":"{\"message\":"}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"function":{"name":"echo","arguments":"\"hi\"}"}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`)
	client := llm.NewOpenAIClient(server.URL, "test-api-key")

	response, err := client.ChatStream(context.Background(), streamRequest(), nil)
	if err != nil {
		t.Fatalf("chat stream: %v", err)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0].Name != "echo" || string(response.ToolCalls[0].Arguments) != `{"message":"hi"}` {
		t.Fatalf("unexpected tool calls %#v", response.ToolCalls)
	}
}

func TestOpenAIClientStreamRejectsMalformedToolArguments(t *testing.T) {
	server, _, _ := sseServer(t, 0,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c","function":{"name":"echo","arguments":"{\"message\":"}}]}}]}`,
		"data: [DONE]")
	_, err := llm.NewOpenAIClient(server.URL, "k").ChatStream(context.Background(), streamRequest(), nil)
	if err == nil || !strings.Contains(err.Error(), "invalid tool call arguments") {
		t.Fatalf("expected malformed arguments to be rejected, got %v", err)
	}
}

func TestOpenAIClientStreamIgnoresCommentsAndAcceptsFinishWithoutDone(t *testing.T) {
	server, _, _ := sseServer(t, 0, ": keep-alive", "", "event: message", textEvent("ok"),
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)
	response, err := llm.NewOpenAIClient(server.URL, "k").ChatStream(context.Background(), streamRequest(), nil)
	if err != nil || response.Message.Content != "ok" {
		t.Fatalf("expected ok, got %#v %v", response, err)
	}
}

func TestOpenAIClientStreamFailures(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  string
	}{
		{"mid-stream error event", []string{textEvent("par"), `data: {"error":{"message":"rate limited"}}`}, "chat stream failed: rate limited"},
		{"ended without finishing", []string{textEvent("partial")}, "ended before the response was complete"},
		{"undecodable event", []string{"data: {not json"}, "decode chat stream event"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, _, _ := sseServer(t, 0, tt.lines...)
			_, err := llm.NewOpenAIClient(server.URL, "k").ChatStream(context.Background(), streamRequest(), nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestOpenAIClientStreamHTTPErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"bad key"}}`)
	}))
	t.Cleanup(server.Close)
	_, err := llm.NewOpenAIClient(server.URL, "k").ChatStream(context.Background(), streamRequest(), nil)
	if err == nil || !strings.Contains(err.Error(), "status 401: bad key") {
		t.Fatalf("expected parsed error status, got %v", err)
	}
}

func TestOpenAIClientStreamFallsBackToPlainJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"not streamed"}}]}`)
	}))
	t.Cleanup(server.Close)
	var text string
	response, err := llm.NewOpenAIClient(server.URL, "k").ChatStream(context.Background(), streamRequest(), func(s string) { text += s })
	if err != nil || response.Message.Content != "not streamed" || text != "not streamed" {
		t.Fatalf("expected non-streaming fallback, got %#v text=%q err=%v", response, text, err)
	}
}

func TestOpenAIClientStreamFailsWhenIdle(t *testing.T) {
	for _, tt := range []struct {
		name  string
		lines []string
	}{
		{"before any data", []string{"<stall>"}},
		{"after the first token", []string{textEvent("hel"), "<stall>"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server, _, _ := sseServer(t, 0, tt.lines...)
			client := llm.NewOpenAIClient(server.URL, "k").WithStreamIdleTimeout(150 * time.Millisecond)

			start := time.Now()
			_, err := client.ChatStream(context.Background(), streamRequest(), nil)
			if err == nil || !strings.Contains(err.Error(), "stalled: no data received for 150ms") {
				t.Fatalf("expected idle timeout error, got %v", err)
			}
			if elapsed := time.Since(start); elapsed > 5*time.Second {
				t.Fatalf("idle stream was not bounded; took %s", elapsed)
			}
		})
	}
}

func TestOpenAIClientStreamHasNoTotalDeadline(t *testing.T) {
	// Six events 100ms apart (~600ms total) with a 250ms idle timeout: a slow
	// but steady answer must not be cut off.
	lines := []string{}
	for _, word := range []string{"a", "b", "c", "d", "e"} {
		lines = append(lines, textEvent(word))
	}
	lines = append(lines, "data: [DONE]")
	server, _, _ := sseServer(t, 100*time.Millisecond, lines...)
	client := llm.NewOpenAIClient(server.URL, "k").WithStreamIdleTimeout(250 * time.Millisecond)

	response, err := client.ChatStream(context.Background(), streamRequest(), nil)
	if err != nil || response.Message.Content != "abcde" {
		t.Fatalf("expected the full slow stream, got %q err=%v", response.Message.Content, err)
	}
}

func TestOpenAIClientStreamReportsCallerCancellation(t *testing.T) {
	server, _, _ := sseServer(t, 0, textEvent("hel"), "<stall>")
	ctx, cancel := context.WithCancel(context.Background())

	_, err := llm.NewOpenAIClient(server.URL, "k").ChatStream(ctx, streamRequest(), func(string) { cancel() })
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "chat stream cancelled") {
		t.Fatalf("expected a cancellation error wrapping context.Canceled, got %v", err)
	}
}
