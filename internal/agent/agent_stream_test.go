package agent_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zachfire9/agent-harness/internal/agent"
	"github.com/zachfire9/agent-harness/internal/llm"
	"github.com/zachfire9/agent-harness/internal/tools"
)

// streamingFake replays scripted responses; ChatStream emits each response's
// text in the given fragments.
type streamingFake struct {
	responses   []llm.ChatResponse
	fragments   [][]string
	streamCalls int
	chatCalls   int
}

func (f *streamingFake) next() (llm.ChatResponse, []string) {
	response, fragments := f.responses[0], f.fragments[0]
	f.responses, f.fragments = f.responses[1:], f.fragments[1:]
	return response, fragments
}

func (f *streamingFake) Chat(context.Context, llm.ChatRequest) (llm.ChatResponse, error) {
	f.chatCalls++
	response, _ := f.next()
	return response, nil
}

func (f *streamingFake) ChatStream(_ context.Context, _ llm.ChatRequest, onText func(string)) (llm.ChatResponse, error) {
	f.streamCalls++
	response, fragments := f.next()
	for _, fragment := range fragments {
		onText(fragment)
	}
	return response, nil
}

func echoRegistry(t *testing.T) tools.Registry {
	t.Helper()
	registry := tools.NewRegistry()
	if err := registry.Register(tools.NewEchoTool()); err != nil {
		t.Fatal(err)
	}
	return registry
}

func toolThenAnswer() *streamingFake {
	return &streamingFake{
		responses: []llm.ChatResponse{
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "Let me check."}, ToolCalls: []llm.ToolCall{{ID: "c1", Name: "echo", Arguments: json.RawMessage(`{"message":"hi"}`)}}},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "All done."}},
		},
		fragments: [][]string{{"Let me ", "check."}, {"All ", "done."}},
	}
}

func recordStream(events *[]string) agent.TextStream {
	return agent.TextStream{
		Text: func(text string) { *events = append(*events, "text:"+text) },
		End:  func() { *events = append(*events, "end") },
	}
}

func TestRunnerStreamsTextFragmentsAndStepBoundaries(t *testing.T) {
	fake := toolThenAnswer()
	var events []string
	runner := agent.NewWithTools(fake, "m", echoRegistry(t)).WithTextStream(recordStream(&events))

	result, err := runner.Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "text:Let me |text:check.|end|text:All |text:done.|end"
	if got := strings.Join(events, "|"); got != want {
		t.Fatalf("stream events:\n got %s\nwant %s", got, want)
	}
	if fake.streamCalls != 2 || fake.chatCalls != 0 {
		t.Fatalf("expected streaming calls only, got stream=%d chat=%d", fake.streamCalls, fake.chatCalls)
	}
	if result.Answer != "All done." {
		t.Fatalf("result must be unchanged by streaming, got %q", result.Answer)
	}
}

// chatOnly hides ChatStream so the runner sees a non-streaming client.
type chatOnly struct{ inner *streamingFake }

func (c chatOnly) Chat(ctx context.Context, request llm.ChatRequest) (llm.ChatResponse, error) {
	return c.inner.Chat(ctx, request)
}

func TestRunnerStreamFallsBackToWholeTextForNonStreamingClients(t *testing.T) {
	fake := toolThenAnswer()
	var events []string
	runner := agent.NewWithTools(chatOnly{fake}, "m", echoRegistry(t)).WithTextStream(recordStream(&events))

	if _, err := runner.Run(context.Background(), "go"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := strings.Join(events, "|"); got != "text:Let me check.|end|text:All done.|end" {
		t.Fatalf("unexpected fallback events %s", got)
	}
}

func TestRunnerStreamSkipsEndForModelCallsWithoutText(t *testing.T) {
	fake := &streamingFake{
		responses: []llm.ChatResponse{
			{Message: llm.Message{Role: llm.RoleAssistant}, ToolCalls: []llm.ToolCall{{ID: "c1", Name: "echo", Arguments: json.RawMessage(`{"message":"hi"}`)}}},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}},
		},
		fragments: [][]string{nil, {"done"}},
	}
	var events []string
	runner := agent.NewWithTools(fake, "m", echoRegistry(t)).WithTextStream(recordStream(&events))
	if _, err := runner.Run(context.Background(), "go"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := strings.Join(events, "|"); got != "text:done|end" {
		t.Fatalf("a silent tool-call step must not emit End, got %s", got)
	}
}

func TestRunnerWithoutStreamUsesChat(t *testing.T) {
	fake := toolThenAnswer()
	if _, err := agent.NewWithTools(fake, "m", echoRegistry(t)).Run(context.Background(), "go"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if fake.streamCalls != 0 || fake.chatCalls != 2 {
		t.Fatalf("a runner without a stream must not stream, got stream=%d chat=%d", fake.streamCalls, fake.chatCalls)
	}
}
