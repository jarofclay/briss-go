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

	"github.com/jarofclay/briss-go/internal/app"
	"github.com/jarofclay/briss-go/internal/crop"
	"github.com/jarofclay/briss-go/internal/detect"
	"github.com/jarofclay/briss-go/internal/pdf"
)

type legacyJob struct {
	src, dst, password  string
	splitCols, splitRow bool
}

// parseLegacy mirrors BrissCMD.CommandValues.parseToWorkDescription: flags are
// matched case-insensitively, a value follows its flag, unknown arguments are
// ignored.
func parseLegacy(args []string) legacyJob {
	var j legacyJob
	for i := 0; i < len(args); i++ {
		next := func() string {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		switch strings.ToLower(strings.TrimSpace(args[i])) {
		case "-s":
			j.src = next()
		case "-d":
			j.dst = next()
		case "-p":
			j.password = next()
		case "--split-col":
			j.splitCols = true
		case "--split-row":
			j.splitRow = true
		}
	}
	return j
}

// runLegacy is `briss -s ...`: the legacy detector, Briss 2.0 messages.
func runLegacy(args []string, stdout, stderr io.Writer) int {
	j := parseLegacy(args)
	if j.src == "" {
		fmt.Fprintln(stderr, "No source file submitted: try \"briss -s filename.pdf\" (see briss --help)")
		return exitUsage
	}
	if st, err := os.Stat(j.src); err != nil || st.IsDir() {
		fmt.Fprintf(stderr, "File: %s doesn't exist\n", j.src)
		return exitInput
	}
	if j.dst == "" {
		fmt.Fprintf(stdout, "Since no destination was provided destination will be set to: %s\n", abs(crop.RecommendedDestination(j.src)))
	}
	eng, err := pdf.NewEngine(pdf.EngineOptions{})
	if err != nil {
		fmt.Fprintln(stderr, "Error starting the PDF engine:", err)
		return exitInput
	}
	defer eng.Close()
	t0 := time.Now()
	fmt.Fprintln(stdout, "Clustering PDF:", j.src)
	rep, err := app.Crop(context.Background(), eng, app.CropRequest{
		Src: j.src, Dst: j.dst,
		Options: app.Options{Password: j.password, Detector: "legacy", Detect: detect.Options{SplitColumns: j.splitCols, SplitRows: j.splitRow}},
	})
	switch {
	case errors.Is(err, pdf.ErrPasswordRequired):
		fmt.Fprintln(stderr, "The PDF is encrypted; give its password with -p.")
		return exitInput
	case errors.Is(err, pdf.ErrBadPassword):
		fmt.Fprintln(stderr, "The password given with -p does not open the PDF.")
		return exitInput
	case errors.Is(err, app.ErrWrite):
		fmt.Fprintln(stderr, "Error while cropping:", err)
		return exitWriteError
	case err != nil:
		fmt.Fprintln(stderr, "Error:", err)
		return exitInput
	}
	fmt.Fprintf(stdout, "Created %d clusters.\n", len(rep.Clusters))
	for _, c := range rep.Clusters {
		if c.Notes != "" {
			fmt.Fprintf(stdout, "  cluster %d (pages %s): %s\n", c.ID+1, c.PagesShort, c.Notes)
		}
	}
	fmt.Fprintf(stdout, "Cropping successful. Cropped to: %s (%d pages from %d, %s)\n", abs(rep.Output), rep.OutputPages, rep.SourcePages, time.Since(t0).Round(time.Millisecond))
	return exitOK
}

func abs(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}
