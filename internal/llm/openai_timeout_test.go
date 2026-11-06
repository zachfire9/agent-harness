package llm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zachfire9/agent-harness/internal/llm"
)

// hangingServer accepts requests and never responds until the test ends.
func hangingServer(t *testing.T) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) }) // runs first: unblock handlers before Close waits on them
	return server
}

func TestOpenAIClientTimesOutOnStalledServer(t *testing.T) {
	server := hangingServer(t)
	client := llm.NewOpenAIClient(server.URL, "test-api-key").WithTimeout(100 * time.Millisecond)

	start := time.Now()
	// context.Background has no deadline: only the client timeout can end this call.
	_, err := client.Chat(context.Background(), llm.ChatRequest{Model: "test-model"})
	if err == nil {
		t.Fatal("expected a timeout error from a server that never responds")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("chat call was not bounded by the client timeout; took %s", elapsed)
	}
}

func TestOpenAIClientDefaultTimeoutIsBounded(t *testing.T) {
	if llm.DefaultRequestTimeout <= 0 || llm.DefaultRequestTimeout > 10*time.Minute {
		t.Fatalf("expected a finite default request timeout, got %s", llm.DefaultRequestTimeout)
	}
}
