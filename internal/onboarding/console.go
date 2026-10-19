package onboarding

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

type Console struct {
	out         io.Writer
	reader      *bufio.Reader
	disableEcho func() (func() error, error)
}

func NewConsole(in io.Reader, out io.Writer) (*Console, error) {
	f, ok := in.(*os.File)
	if !ok || !terminal(f) {
		return nil, errors.New("setup requires an interactive Windows terminal; run agent-harness.exe setup in PowerShell")
	}
	return &Console{out: out, reader: bufio.NewReader(f), disableEcho: func() (func() error, error) {
		return disableEcho(f)
	}}, nil
}
func (c *Console) Ask(label, def string) (string, error) {
	fmt.Fprintf(c.out, "%s [%s]: ", label, def)
	v, e := c.reader.ReadString('\n')
	if e != nil {
		return "", e
	}
	v = strings.TrimSpace(v)
	if v == "" {
		v = def
	}
	return v, nil
}
func (c *Console) Secret() (value string, err error) {
	restore, err := c.disableEcho()
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, restore()) }()
	// The prompt is the signal to paste: echo must already be disabled.
	if _, err = fmt.Fprint(c.out, "API key (hidden): "); err != nil {
		return "", err
	}
	value, err = c.reader.ReadString('\n')
	_, newlineErr := fmt.Fprintln(c.out)
	return strings.TrimRight(value, "\r\n"), errors.Join(err, newlineErr)
}
func (c *Console) Notice(s string) { fmt.Fprintln(c.out, s) }
