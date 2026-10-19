package onboarding

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

type promptWriter func([]byte) (int, error)

func (w promptWriter) Write(p []byte) (int, error) { return w(p) }

func TestSecretDisablesEchoBeforePrompt(t *testing.T) {
	for _, tc := range []struct {
		name                                       string
		input                                      string
		promptErr, readErr, disableErr, restoreErr error
	}{
		{name: "success", input: "synthetic-key\r\nnext\n"},
		{name: "read error", input: "partial", readErr: io.EOF},
		{name: "prompt error", promptErr: errors.New("write failed")},
		{name: "disable error", disableErr: errors.New("disable failed")},
		{name: "restore error", input: "synthetic-key\n", restoreErr: errors.New("restore failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			echoDisabled, restored, prompted := false, false, false
			var output bytes.Buffer
			c := &Console{
				reader: bufio.NewReader(strings.NewReader(tc.input)),
				disableEcho: func() (func() error, error) {
					if tc.disableErr != nil {
						return nil, tc.disableErr
					}
					echoDisabled = true
					return func() error { restored = true; echoDisabled = false; return tc.restoreErr }, nil
				},
				out: promptWriter(func(p []byte) (int, error) {
					if !echoDisabled {
						t.Error("output appeared before echo was disabled or after restoration")
					}
					prompted = true
					if tc.promptErr != nil {
						return 0, tc.promptErr
					}
					return output.Write(p)
				}),
			}
			got, err := c.Secret()
			wantErr := errors.Join(tc.promptErr, tc.readErr, tc.disableErr, tc.restoreErr)
			if (err == nil) != (wantErr == nil) {
				t.Fatalf("error presence: got %v, want %v", err, wantErr)
			}
			for _, e := range []error{tc.promptErr, tc.readErr, tc.disableErr, tc.restoreErr} {
				if e != nil && !errors.Is(err, e) {
					t.Errorf("missing expected error: %v", e)
				}
			}
			if tc.disableErr != nil {
				if prompted || restored {
					t.Error("disable failure must not prompt or restore")
				}
				return
			}
			if !prompted || !restored || echoDisabled {
				t.Error("must prompt with echo disabled and restore on return")
			}
			if strings.Contains(output.String(), "synthetic-key") {
				t.Error("secret appeared in output")
			}
			if tc.name == "success" {
				if got != "synthetic-key" {
					t.Error("incorrect secret")
				}
				next, err := c.reader.ReadString('\n')
				if next != "next\n" || err != nil {
					t.Fatal("buffered following input lost")
				}
			}
		})
	}
}

func TestSecretUsesBufferedAskInput(t *testing.T) {
	c := &Console{
		reader:      bufio.NewReader(strings.NewReader("answer\nsynthetic-key\nnext\n")),
		out:         io.Discard,
		disableEcho: func() (func() error, error) { return func() error { return nil }, nil },
	}
	if _, err := c.Ask("Question", ""); err != nil {
		t.Fatal(err)
	}
	if c.reader.Buffered() == 0 {
		t.Fatal("test must prefetch secret into shared reader")
	}
	got, err := c.Secret()
	if err != nil || got != "synthetic-key" {
		t.Fatal("secret buffered by Ask was lost")
	}
	got, err = c.Ask("Next", "")
	if err != nil || got != "next" {
		t.Fatal("input buffered by Secret was lost")
	}
}
