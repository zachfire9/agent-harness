package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/zachfire9/agent-harness/internal/cli"
	"github.com/zachfire9/agent-harness/internal/llm"
)

// streamingChatClient replays responses; ChatStream emits each response's
// text in fragments and honours cancellation like a real stream.
type streamingChatClient struct {
	responses   []llm.ChatResponse
	fragments   [][]string
	requests    []llm.ChatRequest
	streamCalls int
	chatCalls   int
}

func (c *streamingChatClient) next(request llm.ChatRequest) (llm.ChatResponse, []string) {
	c.requests = append(c.requests, request)
	response, fragments := c.responses[0], c.fragments[0]
	c.responses, c.fragments = c.responses[1:], c.fragments[1:]
	return response, fragments
}

func (c *streamingChatClient) Chat(_ context.Context, request llm.ChatRequest) (llm.ChatResponse, error) {
	c.chatCalls++
	response, _ := c.next(request)
	return response, nil
}

func (c *streamingChatClient) ChatStream(ctx context.Context, request llm.ChatRequest, onText func(string)) (llm.ChatResponse, error) {
	c.streamCalls++
	if err := ctx.Err(); err != nil {
		return llm.ChatResponse{}, fmt.Errorf("chat stream cancelled: %w", err)
	}
	response, fragments := c.next(request)
	for _, fragment := range fragments {
		onText(fragment)
	}
	return response, nil
}

func answer(text string, fragments ...string) (llm.ChatResponse, []string) {
	return llm.ChatResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: text}}, fragments
}

func newStreamingClient(pairs ...any) *streamingChatClient {
	client := &streamingChatClient{}
	for i := 0; i < len(pairs); i += 2 {
		client.responses = append(client.responses, pairs[i].(llm.ChatResponse))
		client.fragments = append(client.fragments, pairs[i+1].([]string))
	}
	return client
}

func narrationThenAnswer() *streamingChatClient {
	toolStep := llm.ChatResponse{
		Message:   llm.Message{Role: llm.RoleAssistant, Content: "Let me check."},
		ToolCalls: []llm.ToolCall{{ID: "c1", Name: "echo", Arguments: json.RawMessage(`{"message":"hi"}`)}},
	}
	final, finalFragments := answer("done", "do", "ne")
	return newStreamingClient(toolStep, []string{"Let me ", "check."}, final, finalFragments)
}

func runStreamingApp(app cli.App, args ...string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	code := app.Run(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

func TestChatStreamsReplyAndPrintsItOnce(t *testing.T) {
	response, fragments := answer("Hello", "Hel", "lo")
	client := newStreamingClient(response, fragments)
	stdout, stderr, code := runStreamingApp(cli.NewAppWithInput(client, "m", strings.NewReader("hi\nexit\n")), "agent-harness", "chat")

	if code != 0 || client.streamCalls != 1 || client.chatCalls != 0 {
		t.Fatalf("code=%d stream=%d chat=%d stderr=%q", code, client.streamCalls, client.chatCalls, stderr)
	}
	if strings.Count(stdout, "Hello") != 1 || !strings.Contains(stdout, "Agent: Hello\n") {
		t.Fatalf("expected the streamed reply exactly once, got %q", stdout)
	}
}

func TestChatStreamsNarrationBeforeToolCallsOnItsOwnLine(t *testing.T) {
	client := narrationThenAnswer()
	stdout, _, code := runStreamingApp(cli.NewAppWithInput(client, "m", strings.NewReader("go\nexit\n")), "agent-harness", "chat")

	if code != 0 || !strings.Contains(stdout, "Agent: Let me check.\nAgent: done\n") {
		t.Fatalf("expected narration and answer on separate lines, got %q", stdout)
	}
}

func TestChatNoStreamFlagUsesSingleResponse(t *testing.T) {
	response, fragments := answer("Hello", "Hel", "lo")
	client := newStreamingClient(response, fragments)
	stdout, _, code := runStreamingApp(cli.NewAppWithInput(client, "m", strings.NewReader("hi\nexit\n")), "agent-harness", "chat", "--no-stream")

	if code != 0 || client.streamCalls != 0 || client.chatCalls != 1 || !strings.Contains(stdout, "Agent: Hello\n") {
		t.Fatalf("code=%d stream=%d chat=%d stdout=%q", code, client.streamCalls, client.chatCalls, stdout)
	}
}

func TestAskStreamsOnlyWhenOutputIsATerminal(t *testing.T) {
	t.Run("piped output keeps only the final answer", func(t *testing.T) {
		client := narrationThenAnswer()
		stdout, _, code := runStreamingApp(cli.NewAppWithInput(client, "m", strings.NewReader("")), "agent-harness", "ask", "go")
		if code != 0 || client.streamCalls != 0 || stdout != "done\n" {
			t.Fatalf("code=%d stream=%d stdout=%q", code, client.streamCalls, stdout)
		}
	})
	t.Run("terminal output streams", func(t *testing.T) {
		client := narrationThenAnswer()
		app := cli.NewAppWithInput(client, "m", strings.NewReader("")).WithTerminalOutput(true)
		stdout, _, code := runStreamingApp(app, "agent-harness", "ask", "go")
		if code != 0 || client.streamCalls != 2 || stdout != "Let me check.\ndone\n" {
			t.Fatalf("code=%d stream=%d stdout=%q", code, client.streamCalls, stdout)
		}
	})
	t.Run("--no-stream on a terminal", func(t *testing.T) {
		client := narrationThenAnswer()
		app := cli.NewAppWithInput(client, "m", strings.NewReader("")).WithTerminalOutput(true)
		stdout, _, code := runStreamingApp(app, "agent-harness", "ask", "--no-stream", "go")
		if code != 0 || client.streamCalls != 0 || stdout != "done\n" {
			t.Fatalf("code=%d stream=%d stdout=%q", code, client.streamCalls, stdout)
		}
	})
}

func TestChatCtrlCCancelsOnlyTheCurrentReply(t *testing.T) {
	response, fragments := answer("second answer", "second answer")
	client := newStreamingClient(response, fragments)
	turns := 0
	app := cli.NewAppWithInput(client, "m", strings.NewReader("first\nsecond\nexit\n")).
		WithTurnContext(func() (context.Context, context.CancelFunc) {
			turns++
			ctx, cancel := context.WithCancel(context.Background())
			if turns == 1 {
				cancel() // simulate Ctrl+C during the first reply
			}
			return ctx, cancel
		})

	stdout, stderr, code := runStreamingApp(app, "agent-harness", "chat")
	if code != 0 {
		t.Fatalf("chat must keep running after a cancelled reply; code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "(reply cancelled)") || !strings.Contains(stdout, "Agent: second answer\n") || !strings.Contains(stdout, "Goodbye.") {
		t.Fatalf("unexpected output %q", stdout)
	}
	if strings.Contains(stderr, "chat turn failed") {
		t.Fatalf("a cancelled reply is not a failure, got stderr %q", stderr)
	}
	// The cancelled turn must not be kept in the conversation history.
	for _, message := range client.requests[len(client.requests)-1].Messages {
		if message.Content == "first" {
			t.Fatalf("cancelled prompt leaked into history: %#v", client.requests[len(client.requests)-1].Messages)
		}
	}
}
