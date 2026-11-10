// Command briss crops PDF pages, a Go rewrite of Briss.
//
//	briss crop book.pdf -o out.pdf              detect and crop
//	briss detect book.pdf --save-crops c.json   detect only, save the crop definition
//	briss apply book.pdf --crops c.json         crop with a saved definition
//	briss preview book.pdf                      overlay PNGs with rectangles drawn
//	briss bench --dataset dir                   score detectors against labels
//
// The Briss 2.0 command line keeps working unchanged:
//
//	briss -s SOURCE.pdf [-d DEST.pdf] [-p PASSWORD] [--split-col] [--split-row]
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jarofclay/briss-go/internal/app"
)

// Exit codes (PLAN.md section 6).
const (
	exitOK         = 0
	exitUsage      = 1
	exitInput      = 2
	exitDetection  = 3
	exitWriteError = 4
)

var version = "2.0.0-beta"

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

// usageError marks command-line mistakes (exit 1).
type usageError struct{ error }

// run dispatches to the legacy command line or the subcommands and maps
// errors to exit codes.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if isLegacy(args) {
		return runLegacy(args, stdout, stderr)
	}
	root := newRoot(stdin, stdout, stderr)
	root.SetArgs(args)
	err := root.Execute()
	if err == nil {
		return exitOK
	}
	fmt.Fprintln(stderr, "briss:", err)
	var ue usageError
	switch {
	case errors.As(err, &ue):
		return exitUsage
	case errors.Is(err, app.ErrUnavailable):
		return exitDetection
	case errors.Is(err, app.ErrWrite):
		return exitWriteError
	case errors.Is(err, app.ErrInput):
		return exitInput
	case strings.HasPrefix(err.Error(), "unknown command"), strings.HasPrefix(err.Error(), "unknown flag"), strings.Contains(err.Error(), "required flag"):
		return exitUsage
	}
	return exitInput
}

var subcommands = map[string]bool{"crop": true, "detect": true, "apply": true, "preview": true, "bench": true, "version": true, "help": true, "completion": true}

// isLegacy reports a Briss 2.0 invocation: no subcommand first, and one of
// the old flags anywhere (matched case-insensitively, like Java).
func isLegacy(args []string) bool {
	if len(args) == 0 || subcommands[strings.ToLower(args[0])] {
		return false
	}
	for _, a := range args {
		switch strings.ToLower(strings.TrimSpace(a)) {
		case "-s", "-d", "-p", "--split-col", "--split-row":
			return true
		}
	}
	return false
}
