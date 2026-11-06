package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDefaultGoogleHTTPClientHasTimeout(t *testing.T) {
	if got := defaultGoogleHTTPClient().Timeout; got <= 0 {
		t.Fatalf("expected default Google client to have a timeout, got %s", got)
	}
}

func TestGoogleTokenRefreshWithDefaultClientTimesOutOnStalledServer(t *testing.T) {
	previous := googleAPITimeout
	googleAPITimeout = 100 * time.Millisecond
	t.Cleanup(func() { googleAPITimeout = previous })

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })

	start := time.Now()
	// nil client exercises the production fallback; Background has no deadline.
	_, err := refreshGoogleAccessToken(context.Background(), nil, server.URL,
		googleOAuthClientCredentials{ClientID: "fake-client-id", ClientSecret: "fake-client-secret"}, "fake-refresh-token")
	if err == nil {
		t.Fatal("expected a timeout error from a token endpoint that never responds")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("token refresh was not bounded by the default client timeout; took %s", elapsed)
	}
}
