// Package bench measures detectors against labelled PDFs (PLAN.md section 8.5).
//
// A dataset is a directory tree of PDFs. A PDF is labelled when a crop
// definition (cropdef v1, the format `briss detect --save-crops` writes) sits
// next to it as NAME.crops.json. The label's rectangles are the ground truth:
// cluster rectangles for every page of the cluster, page overrides for single
// pages; an empty override means "this page must not be cropped". Pages
// without any rectangle in the label are not scored. The label's
// keep_headers option says whether its rectangles include running headers,
// footers and page numbers; detectors are run with the same setting.
//
// Predictions made elsewhere (by a script, another tool or an earlier run)
// can be scored too: a detector named "dir:PATH" reads PATH/REL.crops.json
// for the PDF at REL.pdf below the dataset, in the same format.
package bench

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jarofclay/briss-go/internal/cropdef"
	"github.com/jarofclay/briss-go/internal/geom"
)

// LabelSuffix names the ground-truth file next to each PDF.
const LabelSuffix = ".crops.json"

// Defaults from the plan: a rectangle is accepted when IoU >= 0.95 with the
// label, the right number of rectangles, and no clipping beyond 1% of the page.
const (
	AcceptIoU = 0.95
	ClipTol   = 0.01
)

// Item is one labelled PDF.
type Item struct {
	PDF         string
	Labels      string
	Truth       map[int][]geom.Ratios // 1-based page -> rectangles
	Pages       int
	SHA256      string
	KeepHeaders bool // the label includes running headers, footers and page numbers
}

// DirPrefix marks a detector name that reads precomputed predictions.
const DirPrefix = "dir:"

// Precomputed loads the prediction for the PDF at rel (relative to the
// dataset) from a directory of crop definitions laid out like the dataset.
func Precomputed(dir, rel string) (*cropdef.Definition, error) {
	return cropdef.Load(filepath.Join(dir, strings.TrimSuffix(rel, filepath.Ext(rel))+LabelSuffix))
}

// Load finds labelled PDFs below dir. Unlabelled PDFs are returned separately.
func Load(dir string) (items []Item, unlabelled []string, err error) {
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".pdf") {
			return nil
		}
		lp := strings.TrimSuffix(p, filepath.Ext(p)) + LabelSuffix
		def, lerr := cropdef.Load(lp)
		if errors.Is(lerr, fs.ErrNotExist) {
			unlabelled = append(unlabelled, p)
			return nil
		}
		if lerr != nil {
			return fmt.Errorf("label %s: %w", lp, lerr)
		}
		truth, terr := Truth(def)
		if terr != nil {
			return fmt.Errorf("label %s: %w", lp, terr)
		}
		items = append(items, Item{PDF: p, Labels: lp, Truth: truth, Pages: def.Source.Pages, SHA256: def.Source.SHA256, KeepHeaders: def.Options.KeepHeaders})
		return nil
	})
	sort.Slice(items, func(i, j int) bool { return items[i].PDF < items[j].PDF })
	return items, unlabelled, err
}

// Truth turns a label into per-page rectangles; an empty page override is
// recorded as the whole page.
func Truth(def *cropdef.Definition) (map[int][]geom.Ratios, error) {
	t, err := def.PageRects()
	if err != nil {
		return nil, err
	}
	for p, rs := range def.PageOverrides {
		if len(rs) == 0 {
			t[p] = []geom.Ratios{{}}
		}
	}
	return t, nil
}

// PageResult compares one page.
type PageResult struct {
	Page       int     `json:"page"`
	IoU        float64 `json:"iou"`
	Clipped    bool    `json:"clipped"`
	Waste      float64 `json:"waste"` // predicted minus true area, fraction of the page
	CountMatch bool    `json:"count_match"`
	Accepted   bool    `json:"accepted"`
}

func union(rs []geom.Ratios) geom.Ratios {
	if len(rs) == 0 {
		return geom.Ratios{}
	}
	u := rs[0]
	for _, r := range rs[1:] {
		u = geom.Ratios{L: math.Min(u.L, r.L), B: math.Min(u.B, r.B), R: math.Min(u.R, r.R), T: math.Min(u.T, r.T)}
	}
	return u
}

func area(r geom.Ratios) float64 { return math.Max(0, 1-r.L-r.R) * math.Max(0, 1-r.B-r.T) }

func clips(pred, truth geom.Ratios, tol float64) bool {
	return pred.L > truth.L+tol || pred.B > truth.B+tol || pred.R > truth.R+tol || pred.T > truth.T+tol
}

// Compare scores predicted rectangles of one page against the truth. With the
// same number of rectangles each pair is compared in order (so a wrong split
// position counts); otherwise the union of each side is compared.
func Compare(page int, pred, truth []geom.Ratios) PageResult {
	if len(pred) == 0 {
		pred = []geom.Ratios{{}}
	}
	r := PageResult{Page: page, CountMatch: len(pred) == len(truth)}
	pu, tu := union(pred), union(truth)
	r.IoU = geom.IoU(pu, tu)
	r.Clipped = clips(pu, tu, ClipTol)
	if r.CountMatch && len(pred) > 1 {
		r.IoU = 1
		for i := range pred {
			r.IoU = math.Min(r.IoU, geom.IoU(pred[i], truth[i]))
			r.Clipped = r.Clipped || clips(pred[i], truth[i], ClipTol)
		}
	}
	var pa, ta float64
	for _, p := range pred {
		pa += area(p)
	}
	for _, t := range truth {
		ta += area(t)
	}
	r.Waste = pa - ta
	r.Accepted = r.CountMatch && !r.Clipped && r.IoU >= AcceptIoU
	return r
}

// FileResult is one PDF under one detector.
type FileResult struct {
	File     string       `json:"file"`
	Detector string       `json:"detector"`
	Scored   int          `json:"scored_pages"`
	MeanIoU  float64      `json:"mean_iou"`
	Clipped  int          `json:"clipped"`
	Accepted int          `json:"accepted"`
	Mismatch int          `json:"count_mismatch"`
	Waste    float64      `json:"mean_waste"`
	Seconds  float64      `json:"seconds"`
	Error    string       `json:"error,omitempty"`
	Warnings []string     `json:"warnings,omitempty"`
	Worst    []PageResult `json:"worst,omitempty"`
}

// Score compares every labelled page of an item with the prediction.
func Score(it Item, detector string, pred map[int][]geom.Ratios) FileResult {
	fr := FileResult{File: it.PDF, Detector: detector}
	var all []PageResult
	pages := make([]int, 0, len(it.Truth))
	for p := range it.Truth {
		pages = append(pages, p)
	}
	sort.Ints(pages)
	for _, p := range pages {
		r := Compare(p, pred[p], it.Truth[p])
		all = append(all, r)
		fr.MeanIoU += r.IoU
		fr.Waste += r.Waste
		if r.Clipped {
			fr.Clipped++
		}
		if r.Accepted {
			fr.Accepted++
		}
		if !r.CountMatch {
			fr.Mismatch++
		}
	}
	fr.Scored = len(all)
	if fr.Scored > 0 {
		fr.MeanIoU /= float64(fr.Scored)
		fr.Waste /= float64(fr.Scored)
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Accepted != all[j].Accepted {
			return !all[i].Accepted
		}
		return all[i].IoU < all[j].IoU
	})
	for _, r := range all {
		if r.Accepted || len(fr.Worst) == 5 {
			break
		}
		fr.Worst = append(fr.Worst, r)
	}
	return fr
}

// Summary aggregates one detector over a dataset.
type Summary struct {
	Detector    string  `json:"detector"`
	Files       int     `json:"files"`
	Failed      int     `json:"failed"`
	Pages       int     `json:"scored_pages"`
	MeanIoU     float64 `json:"mean_iou"`
	ClipRate    float64 `json:"clipped_rate"`
	AcceptRate  float64 `json:"accept_rate"`
	MismatchCnt int     `json:"count_mismatch"`
	MeanWaste   float64 `json:"mean_waste"`
	Seconds     float64 `json:"seconds"`
}

// Report is a whole benchmark run.
type Report struct {
	Dataset    string       `json:"dataset"`
	Results    []FileResult `json:"results"`
	Summaries  []Summary    `json:"summaries"`
	Unlabelled []string     `json:"unlabelled,omitempty"`
}

// Summarize fills r.Summaries from r.Results, weighting by scored pages.
func (r *Report) Summarize() {
	byDet := map[string]*Summary{}
	var order []string
	for _, fr := range r.Results {
		s, ok := byDet[fr.Detector]
		if !ok {
			s = &Summary{Detector: fr.Detector}
			byDet[fr.Detector] = s
			order = append(order, fr.Detector)
		}
		s.Files++
		s.Seconds += fr.Seconds
		if fr.Error != "" {
			s.Failed++
			continue
		}
		s.Pages += fr.Scored
		s.MeanIoU += fr.MeanIoU * float64(fr.Scored)
		s.MeanWaste += fr.Waste * float64(fr.Scored)
		s.ClipRate += float64(fr.Clipped)
		s.AcceptRate += float64(fr.Accepted)
		s.MismatchCnt += fr.Mismatch
	}
	r.Summaries = nil
	for _, d := range order {
		s := byDet[d]
		if s.Pages > 0 {
			n := float64(s.Pages)
			s.MeanIoU /= n
			s.MeanWaste /= n
			s.ClipRate /= n
			s.AcceptRate /= n
		}
		r.Summaries = append(r.Summaries, *s)
	}
}

// Markdown renders the report for `briss bench --report`.
func (r *Report) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# briss bench: %s\n\n", r.Dataset)
	b.WriteString("Accepted means IoU >= 0.95 with the label, the same number of rectangles, and no clipping beyond 1% of the page. Waste is the predicted area minus the labelled area, as a fraction of the page.\n\n")
	b.WriteString("| Detector | Files | Failed | Scored pages | Mean IoU | Accepted | Clipped | Count mismatch | Mean waste | Seconds |\n|---|---|---|---|---|---|---|---|---|---|\n")
	for _, s := range r.Summaries {
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %.3f | %.1f%% | %.1f%% | %d | %+.3f | %.1f |\n", s.Detector, s.Files, s.Failed, s.Pages, s.MeanIoU, 100*s.AcceptRate, 100*s.ClipRate, s.MismatchCnt, s.MeanWaste, s.Seconds)
	}
	b.WriteString("\n## Files\n\n| File | Detector | Scored | Mean IoU | Accepted | Clipped | Mismatch | Seconds | Worst pages |\n|---|---|---|---|---|---|---|---|---|\n")
	for _, fr := range r.Results {
		if fr.Error != "" {
			fmt.Fprintf(&b, "| %s | %s | error: %s | | | | | %.2f | |\n", fr.File, fr.Detector, fr.Error, fr.Seconds)
			continue
		}
		var worst []string
		for _, w := range fr.Worst {
			tag := ""
			if w.Clipped {
				tag = " clipped"
			}
			if !w.CountMatch {
				tag += " count"
			}
			worst = append(worst, fmt.Sprintf("p%d %.2f%s", w.Page, w.IoU, tag))
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %.3f | %d | %d | %d | %.2f | %s |\n", fr.File, fr.Detector, fr.Scored, fr.MeanIoU, fr.Accepted, fr.Clipped, fr.Mismatch, fr.Seconds, strings.Join(worst, ", "))
	}
	if len(r.Unlabelled) > 0 {
		fmt.Fprintf(&b, "\n%d PDFs without a %s label were skipped.\n", len(r.Unlabelled), LabelSuffix)
	}
	return b.String()
}
