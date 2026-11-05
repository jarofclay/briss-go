// Command briss crops PDF pages, a Go rewrite of Briss.
//
// This build has the legacy command line of Briss 2.0:
//
//	briss -s SOURCE.pdf [-d DEST.pdf] [-p PASSWORD] [--split-col] [--split-row]
//
// The destination defaults to SOURCE_cropped.pdf. The graphical editor and
// the new subcommands (PLAN.md sections 6 and 7) come in later phases.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jarofclay/briss-go/internal/crop"
	"github.com/jarofclay/briss-go/internal/detect"
	"github.com/jarofclay/briss-go/internal/detect/legacy"
	"github.com/jarofclay/briss-go/internal/pdf"
	"github.com/jarofclay/briss-go/internal/pipeline"
)

// Exit codes (PLAN.md section 6).
const (
	exitOK         = 0
	exitUsage      = 1
	exitInput      = 2
	exitDetection  = 3
	exitWriteError = 4
)

var version = "0.1.0-dev"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

const usage = `briss crops PDF pages so text renders larger on small screens.

Usage:
  briss -s SOURCE.pdf [-d DEST.pdf] [-p PASSWORD] [--split-col] [--split-row]

  -s FILE        PDF to crop
  -d FILE|DIR    output file (default SOURCE_cropped.pdf; a directory gets SOURCE_cropped.pdf inside it)
  -p PASSWORD    password of an encrypted PDF
  --split-col    split every crop rectangle into two columns
  --split-row    split every crop rectangle into two rows
  --version      print the version

Flags are case-insensitive, as in Briss 2.0. The graphical editor is not part
of this build yet.
`

type job struct {
	src, dst, password  string
	splitCols, splitRow bool
}

// parse mirrors BrissCMD.CommandValues.parseToWorkDescription: flags are matched
// case-insensitively, a value follows its flag, unknown arguments are ignored.
func parse(args []string) (job, bool) {
	var j job
	any := false
	for i := 0; i < len(args); i++ {
		a := strings.TrimSpace(args[i])
		next := func() string {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		switch strings.ToLower(a) {
		case "-s":
			j.src, any = next(), true
		case "-d":
			j.dst, any = next(), true
		case "-p":
			j.password, any = next(), true
		case "--split-col":
			j.splitCols, any = true, true
		case "--split-row":
			j.splitRow, any = true, true
		}
	}
	return j, any
}

func run(args []string, stdout, stderr io.Writer) int {
	for _, a := range args {
		switch strings.ToLower(a) {
		case "--version", "-version":
			fmt.Fprintln(stdout, "briss", version)
			return exitOK
		case "-h", "--help", "-help":
			fmt.Fprint(stdout, usage)
			return exitOK
		}
	}
	j, ok := parse(args)
	if !ok || j.src == "" {
		if len(args) == 1 && !strings.HasPrefix(args[0], "-") {
			fmt.Fprintf(stderr, "The graphical editor is not part of this build yet. To crop %s automatically run:\n  briss -s %s\n", args[0], args[0])
		} else {
			fmt.Fprint(stderr, usage)
		}
		return exitUsage
	}
	if st, err := os.Stat(j.src); err != nil || st.IsDir() {
		fmt.Fprintf(stderr, "File: %s doesn't exist\n", j.src)
		return exitInput
	}
	if j.dst == "" {
		j.dst = crop.RecommendedDestination(j.src)
		fmt.Fprintf(stdout, "Since no destination was provided destination will be set to: %s\n", abs(j.dst))
	} else if st, err := os.Stat(j.dst); err == nil && st.IsDir() {
		j.dst = filepath.Join(j.dst, filepath.Base(crop.RecommendedDestination(j.src)))
	}
	if abs(j.dst) == abs(j.src) {
		fmt.Fprintln(stderr, "The destination must differ from the source.")
		return exitInput
	}
	return autoCrop(j, stdout, stderr)
}

func autoCrop(j job, stdout, stderr io.Writer) int {
	ctx := context.Background()
	t0 := time.Now()
	eng, err := pdf.NewEngine(pdf.EngineOptions{})
	if err != nil {
		fmt.Fprintln(stderr, "Error starting the PDF engine:", err)
		return exitInput
	}
	defer eng.Close()

	fmt.Fprintln(stdout, "Clustering PDF:", j.src)
	res, err := pipeline.Analyze(ctx, eng, j.src, pipeline.Options{Password: j.password})
	switch {
	case errors.Is(err, pdf.ErrPasswordRequired):
		fmt.Fprintln(stderr, "The PDF is encrypted; give its password with -p.")
		return exitInput
	case errors.Is(err, pdf.ErrBadPassword):
		fmt.Fprintln(stderr, "The password given with -p does not open the PDF.")
		return exitInput
	case err != nil:
		fmt.Fprintln(stderr, "Error occurred while clustering:", err)
		return exitInput
	}
	fmt.Fprintf(stdout, "Created %d clusters.\n", len(res.Clusters))
	fmt.Fprintln(stdout, "Calculating crop rectangles.")
	opts := detect.Options{SplitColumns: j.splitCols, SplitRows: j.splitRow}
	if err := res.Detect(ctx, legacy.Detector{}, opts); err != nil {
		fmt.Fprintln(stderr, "Error while detecting:", err)
		return exitDetection
	}
	for _, c := range res.Clusters {
		if c.Candidate.Notes != "" {
			fmt.Fprintf(stdout, "  cluster %d (pages %s): %s\n", c.Cluster.ID+1, pageList(c.Cluster.Pages), c.Candidate.Notes)
		}
	}
	def, err := res.Definition(opts, nil)
	if err != nil {
		fmt.Fprintln(stderr, "Error while building the crop definition:", err)
		return exitDetection
	}
	rects, err := def.PageRects()
	if err != nil {
		fmt.Fprintln(stderr, "Error while building the crop definition:", err)
		return exitDetection
	}
	fmt.Fprintln(stdout, "Starting to crop files.")
	st, err := crop.Write(ctx, j.src, j.dst, rects, crop.Options{Password: j.password})
	if err != nil {
		fmt.Fprintln(stderr, "Error while cropping:", err)
		return exitWriteError
	}
	fmt.Fprintf(stdout, "Cropping successful. Cropped to: %s (%d pages from %d, %s)\n", abs(j.dst), st.OutputPages, st.SourcePages, time.Since(t0).Round(time.Millisecond))
	return exitOK
}

func abs(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

func pageList(pages []int) string {
	var b strings.Builder
	for i, p := range pages {
		if i == 8 {
			fmt.Fprintf(&b, " ... (%d pages)", len(pages))
			break
		}
		if i > 0 {
			b.WriteString(" ")
		}
		fmt.Fprint(&b, p)
	}
	return b.String()
}
