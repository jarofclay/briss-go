// Package app implements the briss commands on top of the pipeline: analyse
// and detect, crop or apply a crop definition, preview, and the detector and
// margin options shared by all of them. cmd/briss only parses flags and
// prints results.
package app

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jarofclay/briss-go/internal/bench"
	"github.com/jarofclay/briss-go/internal/cluster"
	"github.com/jarofclay/briss-go/internal/crop"
	"github.com/jarofclay/briss-go/internal/cropdef"
	"github.com/jarofclay/briss-go/internal/detect"
	"github.com/jarofclay/briss-go/internal/detect/cv"
	"github.com/jarofclay/briss-go/internal/detect/legacy"
	"github.com/jarofclay/briss-go/internal/geom"
	"github.com/jarofclay/briss-go/internal/pagerange"
	"github.com/jarofclay/briss-go/internal/pdf"
	"github.com/jarofclay/briss-go/internal/pipeline"
	"github.com/jarofclay/briss-go/internal/preview"
)

// Error kinds, mapped to exit codes by cmd/briss (PLAN.md section 6).
var (
	ErrInput       = errors.New("input error")          // exit 2
	ErrUnavailable = errors.New("detector unavailable") // exit 3
	ErrWrite       = errors.New("write error")          // exit 4
)

// Detectors lists the detector names the command line accepts.
var Detectors = []string{"auto", "legacy", "structural", "cv", "ml", "claude"}

// DetectorByName returns the detector for a --detector value. "auto" is
// resolved per document by Analyze (scans get cv, born-digital PDFs legacy),
// so here it returns nil with no error.
func DetectorByName(name string) (detect.Detector, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "auto":
		return nil, nil
	case "legacy":
		return legacy.Detector{}, nil
	case "cv":
		return cv.New(), nil
	case "structural", "ml", "claude":
		return nil, fmt.Errorf("%w: the %s detector is not part of this build yet (PLAN.md section 9, phase 4)", ErrUnavailable, name)
	}
	return nil, fmt.Errorf("%w: unknown detector %q (choose one of %s)", ErrInput, name, strings.Join(Detectors, ", "))
}

// autoDetector picks the tier for a document: cv when the sampled pages are
// scans (one image covering most of the page and little or no text), legacy
// otherwise, until the structural tier exists.
func autoDetector(ctx context.Context, eng *pdf.Engine, res *pipeline.Result) (detect.Detector, string, error) {
	var sample []int
	for _, c := range res.Clusters {
		for i, p := range c.Cluster.Sample {
			if i < 3 {
				sample = append(sample, p-1)
			}
		}
	}
	kinds, err := eng.PageKinds(ctx, res.Doc, sample)
	if err != nil {
		return nil, "", err
	}
	scans := 0
	for _, k := range kinds {
		if k.Scanned() {
			scans++
		}
	}
	if len(kinds) > 0 && scans*2 > len(kinds) {
		return cv.New(), fmt.Sprintf("auto: %d of %d sampled pages are scans", scans, len(kinds)), nil
	}
	return legacy.Detector{}, fmt.Sprintf("auto: %d of %d sampled pages are scans", scans, len(kinds)), nil
}

// Margin is padding added around detected rectangles.
type Margin struct {
	Value   float64 // points, or percent of the page side when Percent
	Percent bool
}

// ParseMargin reads "3pt", "3" (points), "2mm", "0.5in" or "2%". Negative
// values trim inwards.
func ParseMargin(s string) (Margin, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return Margin{}, nil
	}
	units := []struct {
		suffix string
		scale  float64
		pct    bool
	}{{"%", 1, true}, {"pt", 1, false}, {"mm", 72 / 25.4, false}, {"in", 72, false}}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, u.suffix)), 64)
			if err != nil {
				return Margin{}, fmt.Errorf("%w: margin %q", ErrInput, s)
			}
			return Margin{Value: v * u.scale, Percent: u.pct}, nil
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return Margin{}, fmt.Errorf("%w: margin %q (use pt, mm, in or %%)", ErrInput, s)
	}
	return Margin{Value: v}, nil
}

// Apply grows r by the margin on every side of a page of w x h points and
// clamps it to the page.
func (m Margin) Apply(r geom.Ratios, w, h float64) geom.Ratios {
	if m.Value == 0 {
		return r
	}
	dx, dy := m.Value/w, m.Value/h
	if m.Percent {
		dx, dy = m.Value/100, m.Value/100
	}
	return geom.Ratios{L: r.L - dx, B: r.B - dy, R: r.R - dx, T: r.T - dy}.Clamp(0.01)
}

// Options are the analysis options shared by the commands.
type Options struct {
	Password string
	Exclude  string // page list, "1-3;10"
	Detector string
	Detect   detect.Options
	Margin   Margin
}

// Analysis is a document after detection.
type Analysis struct {
	Result *pipeline.Result
	Def    *cropdef.Definition
}

// Analyze opens, clusters, renders and detects, and returns the crop definition.
func Analyze(ctx context.Context, eng *pdf.Engine, src string, opts Options) (*Analysis, error) {
	det, err := DetectorByName(opts.Detector)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(src); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInput, err)
	}
	// page ranges are validated against the page count after opening
	excl, err := pagerange.Parse(opts.Exclude, 0)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInput, err)
	}
	res, err := pipeline.Analyze(ctx, eng, src, pipeline.Options{Password: opts.Password, Cluster: cluster.Options{Exclude: excl}})
	if err != nil {
		return nil, inputErr(err)
	}
	for p := range excl {
		if p > len(res.Doc.Pages) {
			return nil, fmt.Errorf("%w: excluded page %d is beyond the last page %d", ErrInput, p, len(res.Doc.Pages))
		}
	}
	autoNote := ""
	if det == nil {
		if det, autoNote, err = autoDetector(ctx, eng, res); err != nil {
			return nil, inputErr(err)
		}
	}
	if pd, ok := det.(detect.PageDetector); ok {
		if err := detectPages(ctx, eng, res, pd, opts.Detect); err != nil {
			return nil, inputErr(err)
		}
	} else if err := res.Detect(ctx, det, opts.Detect); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	for _, c := range res.Clusters {
		for i, r := range c.Candidate.Rects {
			c.Candidate.Rects[i] = opts.Margin.Apply(r, c.Cluster.Width, c.Cluster.Height)
		}
		for p, rs := range c.Candidate.PerPage {
			pi := res.Doc.Pages[p-1]
			for i, r := range rs {
				rs[i] = opts.Margin.Apply(r, pi.Width, pi.Height)
			}
		}
		if autoNote != "" {
			c.Candidate.Notes = strings.TrimSpace(autoNote + "; " + c.Candidate.Notes)
			c.Candidate.Notes = strings.TrimSuffix(c.Candidate.Notes, ";")
		}
	}
	def, err := res.Definition(opts.Detect, excl)
	if err != nil {
		return nil, err
	}
	def.Options.Detector = det.Name()
	if opts.Margin.Percent {
		def.Options.MarginPct = opts.Margin.Value
	} else {
		def.Options.MarginPt = opts.Margin.Value
	}
	return &Analysis{Result: res, Def: def}, nil
}

func inputErr(err error) error {
	if errors.Is(err, pdf.ErrPasswordRequired) || errors.Is(err, pdf.ErrBadPassword) || os.IsNotExist(err) || strings.Contains(err.Error(), "pdf: opening") {
		return fmt.Errorf("%w: %w", ErrInput, err)
	}
	return err
}

// CropRequest describes one crop or apply run.
type CropRequest struct {
	Src, Dst  string
	Options   Options
	CropsIn   string // apply this crop definition instead of detecting
	SaveCrops string // write the crop definition used
	DryRun    bool   // detect and report, write no PDF
	Force     bool   // apply a crop definition made for a different file
	Extras    Extras
	Progress  Progress
}

// Report summarises a run; it is also the --json output.
type Report struct {
	Source       string           `json:"source"`
	Output       string           `json:"output,omitempty"`
	Crops        string           `json:"crops,omitempty"`
	Detector     string           `json:"detector,omitempty"`
	SourcePages  int              `json:"source_pages"`
	OutputPages  int              `json:"output_pages,omitempty"`
	Clones       int              `json:"clones,omitempty"`
	DryRun       bool             `json:"dry_run,omitempty"`
	Clusters     []ClusterSummary `json:"clusters"`
	TimingsMS    map[string]int64 `json:"timings_ms"`
	Warnings     []string         `json:"warnings,omitempty"`
	Straightened int              `json:"straightened,omitempty"` // output pages turned level
	Normalized   bool             `json:"normalized,omitempty"`
	OCRPages     int              `json:"ocr_pages,omitempty"` // source pages given a text layer
}

// ClusterSummary is one cluster in a Report.
type ClusterSummary struct {
	ID         int            `json:"id"`
	Pages      string         `json:"pages"`
	PagesShort string         `json:"-"` // abbreviated for text output
	Even       bool           `json:"even"`
	SizePt     [2]float64     `json:"size_pt"`
	Rects      []cropdef.Rect `json:"rects"`
	Source     string         `json:"source,omitempty"`
	Confidence float64        `json:"confidence,omitempty"`
	Notes      string         `json:"notes,omitempty"`
}

func summarize(def *cropdef.Definition, res *pipeline.Result) []ClusterSummary {
	notes := map[int]string{}
	if res != nil {
		for _, c := range res.Clusters {
			notes[c.Cluster.ID] = c.Candidate.Notes
		}
	}
	var out []ClusterSummary
	for _, c := range def.Clusters {
		set := map[int]bool{}
		for _, p := range c.Pages {
			set[p] = true
		}
		out = append(out, ClusterSummary{ID: c.ID, Pages: pagerange.Format(set), PagesShort: pagerange.Abbrev(set, 8), Even: c.Even, SizePt: c.BoxPt, Rects: c.Rects, Source: c.Source, Confidence: c.Confidence, Notes: notes[c.ID]})
	}
	return out
}

// Crop runs detection (or loads CropsIn) and writes the cropped PDF.
func Crop(ctx context.Context, eng *pdf.Engine, req CropRequest) (*Report, error) {
	t0 := time.Now()
	rep := &Report{Source: req.Src, TimingsMS: map[string]int64{}, DryRun: req.DryRun}
	var def *cropdef.Definition
	var res *pipeline.Result
	if req.Progress != nil {
		req.Progress("detect", 0, 1)
	}
	if req.CropsIn != "" {
		d, warn, err := LoadCropsFor(ctx, eng, req.CropsIn, req.Src, req.Options.Password, req.Force)
		if err != nil {
			return nil, err
		}
		def, rep.Crops, rep.Warnings = d, req.CropsIn, warn
		rep.Detector = "crops file"
	} else {
		a, err := Analyze(ctx, eng, req.Src, req.Options)
		if err != nil {
			return nil, err
		}
		def, res = a.Def, a.Result
		rep.Detector = def.Options.Detector
		rep.TimingsMS["analyse"] = (res.Timings.Open + res.Timings.Cluster + res.Timings.Render).Milliseconds()
		rep.TimingsMS["detect"] = res.Timings.Detect.Milliseconds()
	}
	rep.SourcePages = def.Source.Pages
	rep.Clusters = summarize(def, res)
	if req.SaveCrops != "" {
		if err := def.Save(req.SaveCrops); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrWrite, err)
		}
		rep.Crops = req.SaveCrops
	}
	if req.DryRun {
		rep.TimingsMS["total"] = time.Since(t0).Milliseconds()
		return rep, nil
	}
	dst, err := Destination(req.Src, req.Dst)
	if err != nil {
		return nil, err
	}
	var doc *pdf.Document
	if res != nil {
		doc = res.Doc
	} else if doc, err = eng.Open(ctx, req.Src, req.Options.Password); err != nil {
		return nil, inputErr(err)
	}
	if req.Progress != nil {
		req.Progress("detect", 1, 1)
	}
	outs, err := Outputs(def, doc.Pages, req.Extras)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInput, err)
	}
	for _, os := range outs {
		for _, o := range os {
			if o.Angle != 0 {
				rep.Straightened++
			}
		}
	}
	rep.Normalized = req.Extras.Normalize
	if req.Extras.OCR {
		t2 := time.Now()
		n, warn, err := AddOCR(ctx, eng, doc, outs, req.Extras.OCRLang, req.Progress)
		if err != nil {
			return nil, err
		}
		rep.OCRPages, rep.Warnings = n, append(rep.Warnings, warn...)
		rep.TimingsMS["ocr"] = time.Since(t2).Milliseconds()
	}
	if req.Progress != nil {
		req.Progress("write", 0, 1)
	}
	t1 := time.Now()
	st, err := crop.WriteOutputs(ctx, req.Src, dst, outs, crop.Options{Password: req.Options.Password})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrWrite, err)
	}
	if req.Progress != nil {
		req.Progress("write", 1, 1)
	}
	rep.Output, rep.OutputPages, rep.Clones = dst, st.OutputPages, st.Clones
	rep.TimingsMS["write"] = time.Since(t1).Milliseconds()
	rep.TimingsMS["total"] = time.Since(t0).Milliseconds()
	return rep, nil
}

// Destination resolves the output path: empty means SOURCE_cropped.pdf, an
// existing directory gets SOURCE_cropped.pdf inside it.
func Destination(src, dst string) (string, error) {
	if dst == "" {
		dst = crop.RecommendedDestination(src)
	} else if st, err := os.Stat(dst); err == nil && st.IsDir() {
		dst = filepath.Join(dst, filepath.Base(crop.RecommendedDestination(src)))
	}
	a, _ := filepath.Abs(dst)
	b, _ := filepath.Abs(src)
	if a == b {
		return "", fmt.Errorf("%w: the destination must differ from the source", ErrInput)
	}
	return dst, nil
}

// LoadCropsFor loads a crop definition and checks it fits the source: the page
// count must match, and a different SHA-256 is an error unless force is set.
func LoadCropsFor(ctx context.Context, eng *pdf.Engine, cropsPath, src, password string, force bool) (*cropdef.Definition, []string, error) {
	def, err := cropdef.Load(cropsPath)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrInput, err)
	}
	doc, err := eng.Open(ctx, src, password)
	if err != nil {
		return nil, nil, inputErr(err)
	}
	if def.Source.Pages != len(doc.Pages) {
		return nil, nil, fmt.Errorf("%w: %s was made for a %d-page document, %s has %d pages", ErrInput, cropsPath, def.Source.Pages, src, len(doc.Pages))
	}
	var warn []string
	if def.Source.SHA256 != "" {
		sum, err := cropdef.HashFile(src)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %w", ErrInput, err)
		}
		if sum != def.Source.SHA256 {
			msg := fmt.Sprintf("%s was made for a different file (%s, SHA-256 %.12s), not %s", cropsPath, def.Source.Path, def.Source.SHA256, filepath.Base(src))
			if !force {
				return nil, nil, fmt.Errorf("%w: %s; use --force to apply it anyway", ErrInput, msg)
			}
			warn = append(warn, msg)
		}
	}
	return def, warn, nil
}

// PreviewCrops draws a saved crop definition (a label, or one made by
// `detect --save-crops`) on the pages of src, one PNG per page or per page in
// pages, without running any detector. It returns the paths and warnings.
func PreviewCrops(ctx context.Context, eng *pdf.Engine, src, cropsPath, outDir, password, pages string, force bool) ([]string, []string, error) {
	def, warn, err := LoadCropsFor(ctx, eng, cropsPath, src, password, force)
	if err != nil {
		return nil, nil, err
	}
	doc, err := eng.Open(ctx, src, password)
	if err != nil {
		return nil, nil, inputErr(err)
	}
	paths, err := previewPages(ctx, eng, &Analysis{Result: &pipeline.Result{Path: src, Doc: doc}, Def: def}, src, outDir, pages)
	return paths, warn, err
}

// Preview writes one PNG per cluster (or only clusterID when >= 0) with the
// detected rectangles drawn on the overlay. A page-by-page detector draws on
// every page instead (or on the pages listed in pages). It returns the paths.
func Preview(ctx context.Context, eng *pdf.Engine, src, outDir string, opts Options, clusterID int, pages string) ([]string, *Analysis, error) {
	a, err := Analyze(ctx, eng, src, opts)
	if err != nil {
		return nil, nil, err
	}
	perPage := false
	for _, c := range a.Result.Clusters {
		perPage = perPage || len(c.Candidate.PerPage) > 0
	}
	if perPage || pages != "" {
		paths, err := previewPages(ctx, eng, a, src, outDir, pages)
		return paths, a, err
	}
	if outDir == "" {
		outDir = strings.TrimSuffix(src, filepath.Ext(src)) + "_preview"
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrWrite, err)
	}
	base := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	var paths []string
	found := false
	for _, c := range a.Result.Clusters {
		if clusterID >= 0 && c.Cluster.ID != clusterID {
			continue
		}
		found = true
		p := filepath.Join(outDir, fmt.Sprintf("%s_cluster%02d.png", base, c.Cluster.ID))
		f, err := os.Create(p)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %w", ErrWrite, err)
		}
		err = png.Encode(f, preview.Draw(c.Overlay, c.Candidate.Rects))
		f.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %w", ErrWrite, err)
		}
		paths = append(paths, p)
	}
	if !found {
		return nil, nil, fmt.Errorf("%w: no cluster %d (the document has %d)", ErrInput, clusterID, len(a.Result.Clusters))
	}
	return paths, a, nil
}

// Summaries returns the cluster summaries of an analysis, for reports.
func (a *Analysis) Summaries() []ClusterSummary { return summarize(a.Def, a.Result) }

// BenchRequest describes a benchmark run.
type BenchRequest struct {
	Dataset   string
	Detectors []string // names for DetectorByName; empty means "auto"
	Options   Options  // Password is ignored; Passwords are tried in order
	Passwords []string
	Progress  func(string)
}

// Bench runs every detector on every labelled PDF of a dataset.
func Bench(ctx context.Context, eng *pdf.Engine, req BenchRequest) (*bench.Report, error) {
	items, unlabelled, err := bench.Load(req.Dataset)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInput, err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: no labelled PDFs below %s (a label is NAME%s next to NAME.pdf)", ErrInput, req.Dataset, bench.LabelSuffix)
	}
	dets := req.Detectors
	if len(dets) == 0 {
		dets = []string{"auto"}
	}
	for _, d := range dets {
		if dir, ok := strings.CutPrefix(d, bench.DirPrefix); ok {
			if st, err := os.Stat(dir); err != nil || !st.IsDir() {
				return nil, fmt.Errorf("%w: %s is not a directory of predictions", ErrInput, dir)
			}
			continue
		}
		if _, err := DetectorByName(d); err != nil {
			return nil, err
		}
	}
	rep := &bench.Report{Dataset: req.Dataset, Unlabelled: unlabelled}
	passwords := append([]string{""}, req.Passwords...)
	for _, it := range items {
		rel, _ := filepath.Rel(req.Dataset, it.PDF)
		for _, d := range dets {
			if req.Progress != nil {
				req.Progress(fmt.Sprintf("%s (%s)", rel, d))
			}
			t0 := time.Now()
			fr := bench.FileResult{File: rel, Detector: d}
			opts := req.Options
			opts.Detector = d
			opts.Detect.KeepHeaders = opts.Detect.KeepHeaders || it.KeepHeaders // ask for what the label holds
			var def *cropdef.Definition
			if dir, ok := strings.CutPrefix(d, bench.DirPrefix); ok {
				def, err = bench.Precomputed(dir, rel)
			} else {
				var a *Analysis
				for _, pw := range passwords {
					opts.Password = pw
					a, err = Analyze(ctx, eng, it.PDF, opts)
					if !errors.Is(err, pdf.ErrPasswordRequired) && !errors.Is(err, pdf.ErrBadPassword) {
						break
					}
				}
				if err == nil {
					def = a.Def
				}
			}
			switch {
			case err != nil:
				fr.Error = err.Error()
			case def.Source.Pages != it.Pages:
				fr.Error = fmt.Sprintf("label is for %d pages, the prediction for %d", it.Pages, def.Source.Pages)
			default:
				pred, perr := def.PageRects()
				if perr != nil {
					fr.Error = perr.Error()
					break
				}
				fr = bench.Score(it, d, pred)
				fr.File = rel
				if it.SHA256 != "" && def.Source.SHA256 != "" && it.SHA256 != def.Source.SHA256 {
					fr.Warnings = append(fr.Warnings, "label SHA-256 differs from the PDF")
				}
			}
			fr.Seconds = time.Since(t0).Seconds()
			rep.Results = append(rep.Results, fr)
		}
	}
	rep.Summarize()
	return rep, nil
}

// detectPages renders every page and runs a page detector on each, filling
// the clusters' per-page rectangles.
func detectPages(ctx context.Context, eng *pdf.Engine, res *pipeline.Result, pd detect.PageDetector, opts detect.Options) error {
	t0 := time.Now()
	width := pd.RenderWidth()
	jobs := make([]pdf.RenderJob, len(res.Doc.Pages))
	for i, p := range res.Doc.Pages {
		jobs[i] = pdf.RenderJob{Page: i, Width: width, Height: max(1, int(math.Round(float64(width)*p.Height/p.Width)))}
	}
	results := make([]detect.PageResult, len(jobs))
	err := eng.Render(ctx, res.Doc, jobs, func(j pdf.RenderJob, g *image.Gray) error {
		results[j.Page] = pd.DetectPage(g, opts) // distinct index per job, no lock needed
		return nil
	})
	if err != nil {
		return err
	}
	for _, c := range res.Clusters {
		cand := detect.Candidate{Source: pd.Name(), PerPage: map[int][]geom.Ratios{}, PerPageAngles: map[int][]float64{}, Confidence: 1}
		split := 0
		for _, p := range c.Cluster.Pages {
			r := results[p-1]
			cand.PerPage[p] = r.Rects
			if len(r.Angles) == len(r.Rects) {
				cand.PerPageAngles[p] = r.Angles
			}
			cand.Confidence = math.Min(cand.Confidence, r.Confidence)
			if len(r.Rects) > 1 {
				split++
			}
		}
		cand.Notes = fmt.Sprintf("detected page by page; %d of %d pages split into two", split, len(c.Cluster.Pages))
		c.Candidate = cand
	}
	res.Timings.Detect = time.Since(t0)
	return nil
}

// previewPages draws each page's rectangles on the page itself.
func previewPages(ctx context.Context, eng *pdf.Engine, a *Analysis, src, outDir, pages string) ([]string, error) {
	sel, err := pagerange.Parse(pages, len(a.Result.Doc.Pages))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInput, err)
	}
	rects, err := a.Def.PageRects()
	if err != nil {
		return nil, err
	}
	if outDir == "" {
		outDir = strings.TrimSuffix(src, filepath.Ext(src)) + "_preview"
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrWrite, err)
	}
	base := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	var jobs []pdf.RenderJob
	for i, p := range a.Result.Doc.Pages {
		if len(sel) > 0 && !sel[i+1] {
			continue
		}
		const width = 900
		jobs = append(jobs, pdf.RenderJob{Page: i, Width: width, Height: max(1, int(math.Round(width*p.Height/p.Width)))})
	}
	paths := make([]string, len(jobs))
	idx := map[int]int{}
	for k, j := range jobs {
		idx[j.Page] = k
	}
	err = eng.Render(ctx, a.Result.Doc, jobs, func(j pdf.RenderJob, g *image.Gray) error {
		rs := rects[j.Page+1]
		if len(rs) == 0 {
			rs = []geom.Ratios{{}}
		}
		p := filepath.Join(outDir, fmt.Sprintf("%s_page%03d.png", base, j.Page+1))
		f, err := os.Create(p)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrWrite, err)
		}
		defer f.Close()
		if err := png.Encode(f, preview.Draw(g, rs)); err != nil {
			return fmt.Errorf("%w: %w", ErrWrite, err)
		}
		paths[idx[j.Page]] = p
		return nil
	})
	return paths, err
}
