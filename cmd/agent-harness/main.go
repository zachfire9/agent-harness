package main

import (
	"os"

	"github.com/zachfire9/agent-harness/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args, os.Stdout, os.Stderr))
}
