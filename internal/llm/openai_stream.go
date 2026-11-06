package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sort"
	"strings"
	"time"
)

// DefaultStreamIdleTimeout is how long ChatStream waits for the next piece of
// data, including the first token. It is generous because reasoning models can
// think for a while before emitting anything, but unlike a total deadline it
// never cuts off a long answer that is still arriving.
const DefaultStreamIdleTimeout = 2 * time.Minute

// maxStreamLineBytes bounds one server-sent event line.
const maxStreamLineBytes = 1 << 20

// maxErrorBodyBytes bounds how much of an error response is read.
const maxErrorBodyBytes = 64 << 10

var errStreamIdle = errors.New("stream idle timeout")

// WithStreamIdleTimeout returns a copy of the client whose streams fail after d
// without receiving data.
func (c *OpenAIClient) WithStreamIdleTimeout(d time.Duration) *OpenAIClient {
	copied := *c
	copied.streamIdleTimeout = d
	return &copied
}

// ChatStream sends a streaming chat completions request (server-sent events),
// calls onText with each assistant text fragment as it arrives, and returns the
// assembled response. Tool calls arrive as fragments too; they are reassembled
// by index and validated before being returned, never passed out partially.
//
// If the provider ignores "stream": true and replies with an ordinary JSON
// completion, ChatStream accepts it and passes the whole text to onText once.
func (c *OpenAIClient) ChatStream(ctx context.Context, request ChatRequest, onText func(text string)) (ChatResponse, error) {
	if onText == nil {
		onText = func(string) {}
	}
	body, err := json.Marshal(newOpenAIChatRequest(request, true))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("encode chat request: %w", err)
	}

	parent := ctx
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	idle := c.streamIdleTimeout
	if idle <= 0 {
		idle = DefaultStreamIdleTimeout
	}
	// The watchdog covers connecting, waiting for headers, and every gap
	// between events. Any data received resets it.
	watchdog := time.AfterFunc(idle, func() { cancel(errStreamIdle) })
	defer watchdog.Stop()

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("create chat request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "text/event-stream")

	client := c.streamHTTPClient
	if client == nil {
		client = &http.Client{}
	}
	httpResponse, err := client.Do(httpRequest)
	if err != nil {
		return ChatResponse{}, streamError(ctx, parent, idle, fmt.Errorf("send chat request: %w", err))
	}
	defer httpResponse.Body.Close()
	watchdog.Reset(idle)

	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(io.LimitReader(httpResponse.Body, maxErrorBodyBytes))
		return ChatResponse{}, fmt.Errorf("OpenAI-compatible chat request failed: status %d: %s", httpResponse.StatusCode, parseOpenAIErrorMessage(responseBody))
	}

	if mediaType, _, _ := mime.ParseMediaType(httpResponse.Header.Get("Content-Type")); mediaType != "text/event-stream" {
		responseBody, err := io.ReadAll(httpResponse.Body)
		if err != nil {
			return ChatResponse{}, streamError(ctx, parent, idle, fmt.Errorf("read chat response: %w", err))
		}
		response, err := decodeOpenAIChatResponse(responseBody)
		if err != nil {
			return ChatResponse{}, err
		}
		if response.Message.Content != "" {
			onText(response.Message.Content)
		}
		return response, nil
	}

	var acc streamAccumulator
	scanner := bufio.NewScanner(httpResponse.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), maxStreamLineBytes)
	finished := false
	for !finished && scanner.Scan() {
		watchdog.Reset(idle)
		line := scanner.Text()
		// SSE: only "data:" lines carry payloads; blank lines end an event and
		// lines starting with ":" are keep-alive comments.
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			finished = true
			break
		}

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return ChatResponse{}, fmt.Errorf("decode chat stream event: %w", err)
		}
		if chunk.Error != nil {
			message := strings.TrimSpace(chunk.Error.Message)
			if message == "" {
				message = "unknown error"
			}
			return ChatResponse{}, fmt.Errorf("OpenAI-compatible chat stream failed: %s", message)
		}
		for _, choice := range chunk.Choices {
			if choice.Index != 0 {
				continue // only the first choice is used, as in Chat
			}
			if choice.Delta.Role != "" {
				acc.role = choice.Delta.Role
			}
			if choice.Delta.Content != "" {
				acc.text.WriteString(choice.Delta.Content)
				onText(choice.Delta.Content)
			}
			for _, fragment := range choice.Delta.ToolCalls {
				acc.addToolCallFragment(fragment)
			}
			if choice.FinishReason != "" {
				acc.finishReason = choice.FinishReason
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return ChatResponse{}, streamError(ctx, parent, idle, fmt.Errorf("read chat stream: %w", err))
	}
	if err := context.Cause(ctx); err != nil {
		return ChatResponse{}, streamError(ctx, parent, idle, err)
	}
	if !finished && acc.finishReason == "" {
		return ChatResponse{}, errors.New("chat stream ended before the response was complete")
	}

	return acc.response()
}

// streamError explains why a stream stopped: the idle watchdog, the caller's
// own cancellation (for example Ctrl+C), or the underlying error.
func streamError(ctx context.Context, parent context.Context, idle time.Duration, err error) error {
	if errors.Is(context.Cause(ctx), errStreamIdle) {
		return fmt.Errorf("chat stream stalled: no data received for %s", idle)
	}
	if parentErr := parent.Err(); parentErr != nil {
		return fmt.Errorf("chat stream cancelled: %w", parentErr)
	}
	return err
}

type openAIStreamChunk struct {
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role      string                   `json:"role"`
			Content   string                   `json:"content"`
			ToolCalls []openAIToolCallFragment `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type openAIToolCallFragment struct {
	Index    *int   `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type partialToolCall struct {
	id        string
	name      string
	arguments strings.Builder
}

type streamAccumulator struct {
	role         string
	text         strings.Builder
	finishReason string
	calls        map[int]*partialToolCall
	lastIndex    int
}

// addToolCallFragment merges one tool-call delta. The first fragment for an
// index normally carries the id and function name; later ones carry pieces of
// the JSON arguments, which are concatenated in arrival order.
func (a *streamAccumulator) addToolCallFragment(fragment openAIToolCallFragment) {
	if a.calls == nil {
		a.calls = map[int]*partialToolCall{}
	}
	index := a.lastIndex
	switch {
	case fragment.Index != nil:
		index = *fragment.Index
	case fragment.ID != "":
		// Some providers omit index; a new id starts a new call.
		index = len(a.calls)
		for i, call := range a.calls {
			if call.id == fragment.ID {
				index = i
			}
		}
	}
	a.lastIndex = index

	call := a.calls[index]
	if call == nil {
		call = &partialToolCall{}
		a.calls[index] = call
	}
	if fragment.ID != "" {
		call.id = fragment.ID
	}
	// Names normally arrive whole in one fragment; some providers repeat the
	// full name on every fragment, and a few split it. Append only new text.
	if name := fragment.Function.Name; name != "" && name != call.name {
		call.name += name
	}
	call.arguments.WriteString(fragment.Function.Arguments)
}

func (a *streamAccumulator) response() (ChatResponse, error) {
	role := a.role
	if role == "" {
		role = string(RoleAssistant)
	}
	indexes := make([]int, 0, len(a.calls))
	for index := range a.calls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)

	assembled := make([]openAIToolCall, 0, len(indexes))
	for _, index := range indexes {
		call := a.calls[index]
		if strings.TrimSpace(call.name) == "" {
			return ChatResponse{}, fmt.Errorf("chat stream returned a tool call without a name (index %d)", index)
		}
		arguments := call.arguments.String()
		if strings.TrimSpace(arguments) == "" {
			arguments = "{}"
		}
		openAICall := openAIToolCall{ID: call.id, Type: "function"}
		openAICall.Function.Name = call.name
		openAICall.Function.Arguments = arguments
		assembled = append(assembled, openAICall)
	}
	// Reuses Chat's validation: malformed JSON arguments are rejected here.
	toolCalls, err := parseOpenAIToolCalls(assembled)
	if err != nil {
		return ChatResponse{}, err
	}
	return ChatResponse{
		Message: Message{
			Role:      Role(role),
			Content:   a.text.String(),
			ToolCalls: toolCalls,
		},
		ToolCalls: toolCalls,
	}, nil
}
