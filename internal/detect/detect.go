// Package detect defines the interface every crop-detection tier implements
// (PLAN.md section 4.2). The GUI, the CLI and the benchmark harness consume
// Candidates; nothing else talks to a detector's internals.
package detect

import (
	"context"
	"image"

	"github.com/jarofclay/briss-go/internal/cluster"
	"github.com/jarofclay/briss-go/internal/geom"
)

// Options are the user's cropping preferences.
type Options struct {
	SplitColumns bool    // split each rectangle into two columns
	SplitRows    bool    // split each rectangle into two rows
	KeepHeaders  bool    // keep running headers, footers and page numbers
	MarginPt     float64 // padding around detected content, in points
	AutoRotate   bool    // turn pages whose text is sideways or upside down (page detectors)
	ForceTurn    int     // turn every page clockwise by this instead of judging each (page detectors)
	NoAutoSplit  bool    // keep two-page sheets and stacked pages as one page (page detectors)
}

// Request is one cluster to detect.
type Request struct {
	Cluster *cluster.Cluster
	Overlay *image.Gray // may be nil for tiers that do not need it
	Options Options
}

// Candidate is a detector's proposal for one cluster.
type Candidate struct {
	Rects         []geom.Ratios         // rectangles in output order
	PerPage       map[int][]geom.Ratios // optional per-page overrides, 1-based pages
	PerPageAngles map[int][]float64     // optional skew per PerPage rectangle, degrees (see PageResult)
	PerPageTurns  map[int][]int         // optional quarter turn per PerPage rectangle (see PageResult)
	Confidence    float64               // 0..1, tier-defined
	Source        string                // tier name and version, for provenance
	Notes         string
}

// Detector is one detection tier.
type Detector interface {
	Name() string
	// Available reports why the detector cannot run (missing model, no API key).
	Available(ctx context.Context) error
	Detect(ctx context.Context, req Request) (Candidate, error)
}

// PageResult is a page detector's answer for one page.
type PageResult struct {
	Rects      []geom.Ratios // rectangles in output order (left page first on a two-up sheet)
	Angles     []float64     // per rectangle: skew of its text in degrees, counterclockwise turn that straightens it (nil: unknown)
	Turns      []int         // per rectangle: clockwise quarter turn (0, 90, 180, 270) that makes its text upright (nil: none)
	TurnSure   bool          // the page's orientation was judged from its text (not just left alone)
	Confidence float64
	Notes      string
}

// PageDetector is a tier that works on every page on its own instead of on a
// cluster overlay, for scans whose pages shift from sheet to sheet. The
// pipeline renders each page RenderWidth pixels wide and calls DetectPage,
// concurrently for different pages; DetectPage must not keep img.
type PageDetector interface {
	Detector
	RenderWidth() int
	DetectPage(img *image.Gray, opts Options) PageResult
}
