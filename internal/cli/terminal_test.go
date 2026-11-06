package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Redirected input must never count as a terminal, or scripts would be
// prompted (and could be fed "y") instead of failing closed.
func TestIsTerminalRejectsRedirectedInput(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "input.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if isTerminal(file) {
		t.Fatal("a regular file must not be treated as a terminal")
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	if isTerminal(reader) {
		t.Fatal("a pipe must not be treated as a terminal")
	}

	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	if isTerminal(null) {
		t.Fatalf("%s must not be treated as a terminal on %s", os.DevNull, runtime.GOOS)
	}
}
