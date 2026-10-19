package onboarding

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConnectionOptInFakeEndpoint(t *testing.T) {
	for _, status := range []int{200, 401, 404, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("Authorization") != "Bearer fake-hidden-key" || r.URL.Path != "/v1/chat/completions" {
					t.Error("wrong request")
				}
				w.WriteHeader(status)
				if status == 200 {
					w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
				} else {
					w.Write([]byte("fake-hidden-key"))
				}
			}))
			defer srv.Close()
			e := CheckConnection(context.Background(), Settings{BaseURL: srv.URL + "/v1", Model: "fake", Workspace: t.TempDir()}, "fake-hidden-key")
			if (status == 200) != (e == nil) || calls != 1 {
				t.Fatal(e, calls)
			}
			if e != nil && strings.Contains(e.Error(), "fake-hidden-key") {
				t.Fatal("secret echo")
			}
		})
	}
}
func TestConnectionNetworkFailureSafe(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	e := CheckConnection(context.Background(), Settings{BaseURL: srv.URL, Model: "fake", Workspace: t.TempDir()}, "fake-hidden-key")
	if e == nil || strings.Contains(e.Error(), "fake-hidden-key") {
		t.Fatal(e)
	}
}
