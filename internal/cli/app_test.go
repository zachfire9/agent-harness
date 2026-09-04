package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zachfire9/agent-harness/internal/cli"
)

func TestRunDefaultInvocationPrintsPlaceholder(t *testing.T) {
	stdout, stderr, exitCode := runCLI("agent-harness")

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}

	want := "agent-harness: staged learning CLI ready\n"
	if stdout != want {
		t.Fatalf("expected stdout %q, got %q", want, stdout)
	}

	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
}

func TestRunAskCommandPrintsProvidedPrompt(t *testing.T) {
	stdout, stderr, exitCode := runCLI("agent-harness", "ask", "What is an agent?")

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}

	if !strings.Contains(stdout, "Prompt: What is an agent?") {
		t.Fatalf("expected stdout to contain structured prompt, got %q", stdout)
	}

	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
}

func TestRunAskCommandRequiresPrompt(t *testing.T) {
	stdout, stderr, exitCode := runCLI("agent-harness", "ask")

	if exitCode == 0 {
		t.Fatal("expected non-zero exit code for missing prompt")
	}

	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}

	if !strings.Contains(stderr, "ask requires a prompt") {
		t.Fatalf("expected helpful missing prompt error, got %q", stderr)
	}
}

func TestRunUnknownCommandReturnsHelpfulError(t *testing.T) {
	stdout, stderr, exitCode := runCLI("agent-harness", "dance")

	if exitCode == 0 {
		t.Fatal("expected non-zero exit code for unknown command")
	}

	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}

	if !strings.Contains(stderr, "unknown command: dance") {
		t.Fatalf("expected helpful unknown command error, got %q", stderr)
	}
}

func runCLI(args ...string) (stdout string, stderr string, exitCode int) {
	var stdoutBuffer bytes.Buffer
	var stderrBuffer bytes.Buffer

	exitCode = cli.Run(args, &stdoutBuffer, &stderrBuffer)

	return stdoutBuffer.String(), stderrBuffer.String(), exitCode
}
