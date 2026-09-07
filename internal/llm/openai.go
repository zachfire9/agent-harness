package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OpenAIClient calls an OpenAI-compatible chat completions API.
type OpenAIClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewOpenAIClient creates a chat client for an OpenAI-compatible API base URL.
func NewOpenAIClient(baseURL string, apiKey string) *OpenAIClient {
	return &OpenAIClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: http.DefaultClient,
	}
}

// Chat sends one chat completions request and returns the first assistant message.
func (c *OpenAIClient) Chat(ctx context.Context, request ChatRequest) (ChatResponse, error) {
	payload := openAIChatRequest{
		Model:    request.Model,
		Messages: make([]openAIMessage, len(request.Messages)),
	}
	for i, message := range request.Messages {
		payload.Messages[i] = openAIMessage{
			Role:    string(message.Role),
			Content: message.Content,
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("encode chat request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("create chat request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")

	httpResponse, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("send chat request: %w", err)
	}
	defer httpResponse.Body.Close()

	responseBody, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("read chat response: %w", err)
	}

	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		return ChatResponse{}, fmt.Errorf("OpenAI-compatible chat request failed: status %d: %s", httpResponse.StatusCode, parseOpenAIErrorMessage(responseBody))
	}

	var parsed openAIChatResponse
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return ChatResponse{}, fmt.Errorf("decode chat response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return ChatResponse{}, errors.New("OpenAI-compatible chat response contained no assistant message")
	}

	message := parsed.Choices[0].Message
	toolCalls, err := parseOpenAIToolCalls(message.ToolCalls)
	if err != nil {
		return ChatResponse{}, err
	}

	return ChatResponse{
		Message: Message{
			Role:    Role(message.Role),
			Content: message.Content,
		},
		ToolCalls: toolCalls,
	}, nil
}

func parseOpenAIToolCalls(openAIToolCalls []openAIToolCall) ([]ToolCall, error) {
	if len(openAIToolCalls) == 0 {
		return nil, nil
	}

	toolCalls := make([]ToolCall, 0, len(openAIToolCalls))
	for _, openAIToolCall := range openAIToolCalls {
		arguments := json.RawMessage(openAIToolCall.Function.Arguments)
		if !json.Valid(arguments) {
			return nil, fmt.Errorf("invalid tool call arguments for %s", openAIToolCall.Function.Name)
		}

		toolCalls = append(toolCalls, ToolCall{
			ID:        openAIToolCall.ID,
			Name:      openAIToolCall.Function.Name,
			Arguments: arguments,
		})
	}
	return toolCalls, nil
}

func parseOpenAIErrorMessage(body []byte) string {
	var parsed openAIErrorResponse
	if err := json.Unmarshal(body, &parsed); err == nil && strings.TrimSpace(parsed.Error.Message) != "" {
		return parsed.Error.Message
	}

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "empty error response"
	}
	return trimmed
}

type openAIChatRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
}

type openAIMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}
