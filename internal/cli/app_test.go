package cli_test

import (
	"bytes"
	"testing"

	"github.com/zachfire9/agent-harness/internal/cli"
)

func TestRunDefaultInvocationPrintsPlaceholder(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := cli.Run([]string{"agent-harness"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr.String())
	}

	want := "agent-harness: staged learning CLI ready\n"
	if stdout.String() != want {
		t.Fatalf("expected stdout %q, got %q", want, stdout.String())
	}

	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}
