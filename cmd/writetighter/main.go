package main

import (
	"os"

	"github.com/sdougbrown/writetighter/internal/cli"
)

func main() { os.Exit(run(os.Args[1:])) }

// run executes one CLI invocation and returns the process exit code. The
// declared surface in internal/cli owns dispatch, help, and exit codes.
func run(args []string) int {
	return cli.Run(args, os.Stdout, os.Stderr)
}
