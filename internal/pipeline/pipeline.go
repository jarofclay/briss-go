// Package pipeline runs the briss steps in order: read page sizes, cluster,
// render the sampled pages of every cluster in parallel straight into its
// overlay, detect rectangles, and build a crop definition.
package pipeline

import (
	"context"
	"fmt"
	"image"
	"path/filepath"
	"time"

	"github.com/jarofclay/briss-go/internal/cluster"
	"github.com/jarofclay/briss-go/internal/cropdef"
	"github.com/jarofclay/briss-go/internal/detect"
	"github.com/jarofclay/briss-go/internal/overlay"
	"github.com/jarofclay/briss-go/internal/pdf"
)

// Options configures Analyze.
type Options struct {
	Password string
	Cluster  cluster.Options
}

// ClusterResult is one cluster with its overlay and, after Detect, its candidate.
type ClusterResult struct {
	Cluster   *cluster.Cluster
	Overlay   *image.Gray
	Candidate detect.Candidate
}

// Result is an analysed document.
type Result struct {
	Path     string
	Doc      *pdf.Document
	Clusters []*ClusterResult
	Timings  Timings
	password string
}

// Timings records how long each step took.
type Timings struct {
	Open, Cluster, Render, Detect time.Duration
}

// Analyze opens the document, clusters its pages and builds one overlay per cluster.
func Analyze(ctx context.Context, eng *pdf.Engine, path string, opts Options) (*Result, error) {
	t0 := time.Now()
	doc, err := eng.Open(ctx, path, opts.Password)
	if err != nil {
		return nil, err
	}
	res := &Result{Path: path, Doc: doc, password: opts.Password}
	res.Timings.Open = time.Since(t0)

	t1 := time.Now()
	sizes := make([]cluster.PageSize, len(doc.Pages))
	for i, p := range doc.Pages {
		sizes[i] = cluster.PageSize{Width: p.Width, Height: p.Height}
	}
	clusters := cluster.Build(sizes, opts.Cluster)
	accs := make([]*overlay.Accumulator, len(clusters))
	var jobs []pdf.RenderJob
	for i, c := range clusters {
		w, h := overlay.TargetSize(c.Width, c.Height)
		accs[i] = overlay.NewAccumulator(w, h)
		for _, p := range c.Sample {
			jobs = append(jobs, pdf.RenderJob{Page: p - 1, Width: w, Height: h, Tag: i})
		}
	}
	res.Timings.Cluster = time.Since(t1)

	t2 := time.Now()
	err = eng.Render(ctx, doc, jobs, func(j pdf.RenderJob, g *image.Gray) error {
		accs[j.Tag].Add(g)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for i, c := range clusters {
		res.Clusters = append(res.Clusters, &ClusterResult{Cluster: c, Overlay: accs[i].Image()})
	}
	res.Timings.Render = time.Since(t2)
	return res, nil
}

// Detect runs a detector on every cluster.
func (r *Result) Detect(ctx context.Context, det detect.Detector, opts detect.Options) error {
	if err := det.Available(ctx); err != nil {
		return fmt.Errorf("detector %s unavailable: %w", det.Name(), err)
	}
	t0 := time.Now()
	for _, c := range r.Clusters {
		cand, err := det.Detect(ctx, detect.Request{Cluster: c.Cluster, Overlay: c.Overlay, Options: opts})
		if err != nil {
			return fmt.Errorf("detector %s, cluster %d: %w", det.Name(), c.Cluster.ID, err)
		}
		c.Candidate = cand
	}
	r.Timings.Detect = time.Since(t0)
	return nil
}

// Definition builds a crop definition from the detected candidates.
func (r *Result) Definition(opts detect.Options, excluded map[int]bool) (*cropdef.Definition, error) {
	sum, err := cropdef.HashFile(r.Path)
	if err != nil {
		return nil, err
	}
	d := &cropdef.Definition{
		Version: cropdef.Version,
		Source:  cropdef.Source{Path: filepath.Base(r.Path), SHA256: sum, Pages: len(r.Doc.Pages)},
		Options: cropdef.Options{KeepHeaders: opts.KeepHeaders, MarginPt: opts.MarginPt, SplitColumns: opts.SplitColumns, SplitRows: opts.SplitRows},
	}
	for _, c := range r.Clusters {
		cd := cropdef.Cluster{
			ID: c.Cluster.ID, Pages: c.Cluster.Pages, Even: c.Cluster.Even,
			BoxPt:  [2]float64{c.Cluster.Width, c.Cluster.Height},
			Source: c.Candidate.Source, Confidence: c.Candidate.Confidence,
		}
		for _, rr := range c.Candidate.Rects {
			cd.Rects = append(cd.Rects, cropdef.FromRatios(rr))
		}
		for p, rs := range c.Candidate.PerPage {
			if d.PageOverrides == nil {
				d.PageOverrides = map[int][]cropdef.Rect{}
			}
			for i, rr := range rs {
				cr := cropdef.FromRatios(rr)
				if as := c.Candidate.PerPageAngles[p]; i < len(as) {
					cr.Angle = as[i]
				}
				d.PageOverrides[p] = append(d.PageOverrides[p], cr)
			}
		}
		d.Clusters = append(d.Clusters, cd)
	}
	for p := range excluded {
		d.ExcludedPages = append(d.ExcludedPages, p)
	}
	return d, d.Validate()
}

// Password returns the password the document was opened with, for the writer.
func (r *Result) Password() string { return r.password }
