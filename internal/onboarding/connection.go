package onboarding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// CheckConnection is called only after separate consent. It sends no workspace
// content and never returns provider response bodies or transport details.
func CheckConnection(ctx context.Context, s Settings, key string) error {
	if e := s.Validate(); e != nil {
		return e
	}
	body, _ := json.Marshal(map[string]any{"model": s.Model, "messages": []map[string]string{{"role": "user", "content": "Reply OK."}}, "max_completion_tokens": 1})
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, "POST", s.BaseURL+"/chat/completions", bytes.NewReader(body))
	if e != nil {
		return errors.New("connection check: invalid endpoint")
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := client.Do(req)
	if e != nil {
		return errors.New("connection check failed: network/TLS/timeout; settings remain saved")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("connection check failed: HTTP %d (check endpoint, key, model and provider access); settings remain saved", resp.StatusCode)
	}
	var v struct {
		Choices []json.RawMessage `json:"choices"`
	}
	if json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<20)).Decode(&v) != nil || len(v.Choices) == 0 {
		return errors.New("connection check failed: invalid model response; settings remain saved")
	}
	return nil
}
