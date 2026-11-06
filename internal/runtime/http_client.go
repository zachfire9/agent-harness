package runtime

import (
	"net/http"
	"time"
)

// googleAPITimeout bounds each Google OAuth, Docs, or Gmail request made with
// the default client. http.DefaultClient has no timeout, so a stalled
// connection could otherwise hang a scheduled job indefinitely. It is a
// variable only so tests can shorten it.
var googleAPITimeout = 30 * time.Second

// defaultGoogleHTTPClient is used when a caller does not inject a client.
func defaultGoogleHTTPClient() *http.Client {
	return &http.Client{Timeout: googleAPITimeout}
}
