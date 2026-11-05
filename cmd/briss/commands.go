package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jarofclay/briss-go/internal/app"
	"github.com/jarofclay/briss-go/internal/detect"
	"github.com/jarofclay/briss-go/internal/pdf"
	"github.com/spf13/cobra"
)

// analysisFlags are shared by crop, detect, preview and bench.
type analysisFlags struct {
	detector      string
	splitCol      bool
	splitRow      bool
	exclude       string
	margin        string
	keepHeaders   bool
	password      string
	passwordStdin bool
	threads       int
}

func (f *analysisFlags) register(c *cobra.Command, withPassword bool) {
	fl := c.Flags()
	fl.StringVar(&f.detector, "detector", "auto", "detection tier: "+strings.Join(app.Detectors, ", ")+" (auto: cv for scans, legacy otherwise)")
	fl.BoolVar(&f.splitCol, "split-col", false, "split every rectangle into two columns")
	fl.BoolVar(&f.splitRow, "split-row", false, "split every rectangle into two rows")
	fl.StringVar(&f.exclude, "exclude", "", `pages kept out of clustering, each cropped on its own, e.g. "1-3;10"`)
	fl.StringVar(&f.margin, "margin", "", `padding around detected content: 3pt, 2mm, 0.1in or 2%`)
	fl.BoolVar(&f.keepHeaders, "keep-headers", false, "keep running headers, footers and page numbers (no effect on the legacy detector)")
	fl.IntVar(&f.threads, "threads", 0, "parallel render workers (default: up to 4, fewer on small machines)")
	if withPassword {
		fl.StringVar(&f.password, "password", "", "password of an encrypted PDF")
		fl.BoolVar(&f.passwordStdin, "password-stdin", false, "read the password from the first line of standard input")
	}
}

func (f *analysisFlags) options(stdin io.Reader) (app.Options, error) {
	m, err := app.ParseMargin(f.margin)
	if err != nil {
		return app.Options{}, err
	}
	pw := f.password
	if f.passwordStdin {
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && line == "" {
			return app.Options{}, usageError{fmt.Errorf("--password-stdin: nothing to read")}
		}
		pw = strings.TrimRight(line, "\r\n")
	}
	return app.Options{
		Password: pw, Exclude: f.exclude, Detector: f.detector, Margin: m,
		Detect: detect.Options{SplitColumns: f.splitCol, SplitRows: f.splitRow, KeepHeaders: f.keepHeaders},
	}, nil
}

func engine(threads int) (*pdf.Engine, error) {
	eng, err := pdf.NewEngine(pdf.EngineOptions{Workers: threads})
	if err != nil {
		return nil, fmt.Errorf("%w: starting the PDF engine: %w", app.ErrInput, err)
	}
	return eng, nil
}

func exactlyOnePDF(c *cobra.Command, args []string) error {
	if len(args) != 1 {
		return usageError{fmt.Errorf("%s needs exactly one PDF, got %d arguments", c.Name(), len(args))}
	}
	return nil
}

func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printReport(w io.Writer, rep *app.Report, quiet bool) {
	if quiet {
		return
	}
	for _, warn := range rep.Warnings {
		fmt.Fprintln(w, "warning:", warn)
	}
	fmt.Fprintf(w, "%s: %d pages, %d clusters (%s)\n", rep.Source, rep.SourcePages, len(rep.Clusters), rep.Detector)
	for _, c := range rep.Clusters {
		parity := "odd"
		if c.Even {
			parity = "even"
		}
		var rs []string
		for _, r := range c.Rects {
			rs = append(rs, fmt.Sprintf("[l %.3f b %.3f r %.3f t %.3f]", r.L, r.B, r.R, r.T))
		}
		fmt.Fprintf(w, "  cluster %d, %s pages %s, %.0fx%.0f pt: %s\n", c.ID, parity, c.PagesShort, c.SizePt[0], c.SizePt[1], strings.Join(rs, " "))
		if c.Notes != "" {
			fmt.Fprintf(w, "    note: %s\n", c.Notes)
		}
	}
	if rep.Crops != "" {
		fmt.Fprintln(w, "crop definition:", rep.Crops)
	}
	if rep.Output != "" {
		fmt.Fprintf(w, "wrote %s: %d pages (%d added by splits) in %d ms\n", rep.Output, rep.OutputPages, rep.Clones, rep.TimingsMS["total"])
	}
}

func newRoot(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:   "briss",
		Short: "Crop PDF pages so text renders larger on small screens",
		Long: `briss crops PDF pages. It groups similar pages, overlays them, detects the
content area, and writes a PDF with new page boxes (one output page per crop
rectangle). Outlines, links and named destinations are kept.

The Briss 2.0 command line still works:
  briss -s SOURCE.pdf [-d DEST.pdf] [-p PASSWORD] [--split-col] [--split-row]`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 1 {
				return usageError{fmt.Errorf("the graphical editor is not part of this build yet; to crop %s run: briss crop %s", args[0], args[0])}
			}
			if len(args) > 1 {
				return usageError{fmt.Errorf("unknown command %q", args[0])}
			}
			return c.Help()
		},
	}
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err} })
	root.AddCommand(cropCmd(stdin, stdout), detectCmd(stdin, stdout), applyCmd(stdin, stdout), previewCmd(stdin, stdout), benchCmd(stdout, stderr), versionCmd(stdout))
	return root
}

func cropCmd(stdin io.Reader, stdout io.Writer) *cobra.Command {
	var f analysisFlags
	var out, crops, saveCrops string
	var dryRun, force, asJSON, quiet bool
	c := &cobra.Command{
		Use:   "crop SOURCE.pdf",
		Short: "Detect crop rectangles and write the cropped PDF",
		Example: `  briss crop book.pdf
  briss crop book.pdf -o small.pdf --margin 3pt --split-col
  briss crop scan.pdf --exclude "1-2" --save-crops scan.crops.json`,
		Args: exactlyOnePDF,
		RunE: func(c *cobra.Command, args []string) error {
			opts, err := f.options(stdin)
			if err != nil {
				return err
			}
			eng, err := engine(f.threads)
			if err != nil {
				return err
			}
			defer eng.Close()
			rep, err := app.Crop(context.Background(), eng, app.CropRequest{Src: args[0], Dst: out, Options: opts, CropsIn: crops, SaveCrops: saveCrops, DryRun: dryRun, Force: force})
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(stdout, rep)
			}
			printReport(stdout, rep, quiet)
			return nil
		},
	}
	f.register(c, true)
	c.Flags().StringVarP(&out, "output", "o", "", "output PDF or directory (default SOURCE_cropped.pdf)")
	c.Flags().StringVar(&crops, "crops", "", "apply this crop definition instead of detecting")
	c.Flags().StringVar(&saveCrops, "save-crops", "", "write the crop definition that was applied")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "detect and report, write no PDF")
	c.Flags().BoolVar(&force, "force", false, "apply a crop definition made for a different file")
	c.Flags().BoolVar(&asJSON, "json", false, "print a JSON report")
	c.Flags().BoolVarP(&quiet, "quiet", "q", false, "print nothing on success")
	return c
}

func detectCmd(stdin io.Reader, stdout io.Writer) *cobra.Command {
	var f analysisFlags
	var saveCrops string
	var asJSON bool
	c := &cobra.Command{
		Use:     "detect SOURCE.pdf",
		Short:   "Detect crop rectangles and print them; write no PDF",
		Example: "  briss detect book.pdf --save-crops book.crops.json\n  briss detect book.pdf --json",
		Args:    exactlyOnePDF,
		RunE: func(c *cobra.Command, args []string) error {
			opts, err := f.options(stdin)
			if err != nil {
				return err
			}
			eng, err := engine(f.threads)
			if err != nil {
				return err
			}
			defer eng.Close()
			a, err := app.Analyze(context.Background(), eng, args[0], opts)
			if err != nil {
				return err
			}
			if saveCrops != "" {
				if err := a.Def.Save(saveCrops); err != nil {
					return fmt.Errorf("%w: %w", app.ErrWrite, err)
				}
			}
			if asJSON {
				return printJSON(stdout, a.Def)
			}
			rep := &app.Report{Source: args[0], SourcePages: a.Def.Source.Pages, Detector: a.Def.Options.Detector, Clusters: a.Summaries(), Crops: saveCrops}
			printReport(stdout, rep, false)
			return nil
		},
	}
	f.register(c, true)
	c.Flags().StringVar(&saveCrops, "save-crops", "", "write the crop definition to this file")
	c.Flags().BoolVar(&asJSON, "json", false, "print the crop definition as JSON")
	return c
}

func applyCmd(stdin io.Reader, stdout io.Writer) *cobra.Command {
	var out, crops, password string
	var passwordStdin, force, asJSON, quiet bool
	var threads int
	c := &cobra.Command{
		Use:     "apply SOURCE.pdf --crops CROPS.json",
		Short:   "Crop a PDF with a saved crop definition",
		Example: "  briss apply book.pdf --crops book.crops.json -o small.pdf",
		Args:    exactlyOnePDF,
		RunE: func(c *cobra.Command, args []string) error {
			f := analysisFlags{password: password, passwordStdin: passwordStdin}
			opts, err := f.options(stdin)
			if err != nil {
				return err
			}
			eng, err := engine(threads)
			if err != nil {
				return err
			}
			defer eng.Close()
			rep, err := app.Crop(context.Background(), eng, app.CropRequest{Src: args[0], Dst: out, Options: opts, CropsIn: crops, Force: force})
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(stdout, rep)
			}
			printReport(stdout, rep, quiet)
			return nil
		},
	}
	c.Flags().StringVar(&crops, "crops", "", "crop definition to apply (required)")
	c.MarkFlagRequired("crops")
	c.Flags().StringVarP(&out, "output", "o", "", "output PDF or directory (default SOURCE_cropped.pdf)")
	c.Flags().StringVar(&password, "password", "", "password of an encrypted PDF")
	c.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read the password from the first line of standard input")
	c.Flags().BoolVar(&force, "force", false, "apply a crop definition made for a different file")
	c.Flags().BoolVar(&asJSON, "json", false, "print a JSON report")
	c.Flags().BoolVarP(&quiet, "quiet", "q", false, "print nothing on success")
	c.Flags().IntVar(&threads, "threads", 0, "parallel render workers")
	return c
}

func previewCmd(stdin io.Reader, stdout io.Writer) *cobra.Command {
	var f analysisFlags
	var outDir, pageSel string
	var clusterID int
	c := &cobra.Command{
		Use:     "preview SOURCE.pdf",
		Short:   "Write overlay images (or, page by page, page images) with the detected rectangles drawn",
		Example: "  briss preview book.pdf --out previews/\n  briss preview book.pdf --cluster 0 --margin 2%",
		Args:    exactlyOnePDF,
		RunE: func(c *cobra.Command, args []string) error {
			opts, err := f.options(stdin)
			if err != nil {
				return err
			}
			eng, err := engine(f.threads)
			if err != nil {
				return err
			}
			defer eng.Close()
			paths, _, err := app.Preview(context.Background(), eng, args[0], outDir, opts, clusterID, pageSel)
			if err != nil {
				return err
			}
			for _, p := range paths {
				fmt.Fprintln(stdout, p)
			}
			return nil
		},
	}
	f.register(c, true)
	c.Flags().StringVar(&outDir, "out", "", "directory for the PNG files (default SOURCE_preview/)")
	c.Flags().IntVar(&clusterID, "cluster", -1, "only this cluster (default: all)")
	c.Flags().StringVar(&pageSel, "pages", "", `draw on these pages instead of the cluster overlays, e.g. "1-4"`)
	return c
}

func benchCmd(stdout, stderr io.Writer) *cobra.Command {
	var f analysisFlags
	var dataset, report string
	var detectors, passwords []string
	var asJSON, quiet bool
	c := &cobra.Command{
		Use:   "bench --dataset DIR",
		Short: "Score detectors against labelled PDFs",
		Long: `bench runs detectors on every labelled PDF below a directory and reports how
close the results are to the labels. A PDF is labelled by a crop definition
next to it named NAME.crops.json, in the format "briss detect --save-crops"
writes; correct such a file by hand (or in the editor, later) to make a label.
Detectors run with the label's keep_headers setting. A detector named
dir:PATH scores predictions made elsewhere: PATH/REL.crops.json for each
REL.pdf below the dataset.`,
		Example: "  briss bench --dataset ~/briss-labels --report bench.md\n  briss bench --dataset labels --detectors legacy,auto --json\n  briss bench --dataset labels --detectors cv,dir:predictions/layout",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			opts, err := f.options(nil)
			if err != nil {
				return err
			}
			eng, err := engine(f.threads)
			if err != nil {
				return err
			}
			defer eng.Close()
			req := app.BenchRequest{Dataset: dataset, Detectors: detectors, Options: opts, Passwords: passwords}
			if !quiet && !asJSON {
				req.Progress = func(s string) { fmt.Fprintln(stderr, "bench:", s) }
			}
			rep, err := app.Bench(context.Background(), eng, req)
			if err != nil {
				return err
			}
			md := rep.Markdown()
			if report != "" {
				if err := os.WriteFile(report, []byte(md), 0o644); err != nil {
					return fmt.Errorf("%w: %w", app.ErrWrite, err)
				}
			}
			if asJSON {
				return printJSON(stdout, rep)
			}
			fmt.Fprint(stdout, md)
			return nil
		},
	}
	f.register(c, false)
	c.Flags().StringVar(&dataset, "dataset", "", "directory of PDFs with NAME.crops.json labels (required)")
	c.MarkFlagRequired("dataset")
	c.Flags().StringSliceVar(&detectors, "detectors", []string{"auto"}, "detectors to compare, comma-separated (dir:PATH for precomputed predictions)")
	c.Flags().StringSliceVar(&passwords, "password", nil, "passwords to try on encrypted PDFs (repeatable)")
	c.Flags().StringVar(&report, "report", "", "also write the Markdown report to this file")
	c.Flags().BoolVar(&asJSON, "json", false, "print the full report as JSON")
	c.Flags().BoolVarP(&quiet, "quiet", "q", false, "no progress lines")
	return c
}

func versionCmd(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  cobra.NoArgs,
		Run: func(*cobra.Command, []string) {
			fmt.Fprintf(stdout, "briss %s (PDF engine: PDFium %s)\n", version, pdfBackend())
		},
	}
}
