// comment-catalog emits the experimental immutable comment catalog used only by
// the code-aware evaluation driver. It is deliberately outside cmd/ so it is
// not packaged as a WriteTighter command.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/sdougbrown/writetighter/internal/codecomment"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	flags := flag.NewFlagSet("comment-catalog", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	languageFlag := flags.String("language", "", "source language for stdin or extensionless files")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "usage: comment-catalog [--language go|ts|rust|py] [file]")
		return 2
	}

	filename := "<stdin>"
	var source []byte
	var err error
	if flags.NArg() == 1 {
		filename = flags.Arg(0)
		source, err = os.ReadFile(filename)
	} else {
		source, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", filename, err)
		return 2
	}

	var language codecomment.Language
	if *languageFlag != "" {
		language, err = codecomment.ParseLanguage(*languageFlag)
	} else {
		var ok bool
		language, ok = codecomment.DetectLanguage(filename)
		if !ok {
			fmt.Fprintln(os.Stderr, "--language is required for stdin or an unsupported extension")
			return 2
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	catalog, err := codecomment.Extract(filename, language, source)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if err := json.NewEncoder(os.Stdout).Encode(catalog); err != nil {
		fmt.Fprintf(os.Stderr, "encode catalog: %v\n", err)
		return 2
	}
	return 0
}
