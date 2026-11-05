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
}

// Request is one cluster to detect.
type Request struct {
	Cluster *cluster.Cluster
	Overlay *image.Gray // may be nil for tiers that do not need it
	Options Options
}

// Candidate is a detector's proposal for one cluster.
type Candidate struct {
	Rects      []geom.Ratios         // rectangles in output order
	PerPage    map[int][]geom.Ratios // optional per-page overrides, 1-based pages
	Confidence float64               // 0..1, tier-defined
	Source     string                // tier name and version, for provenance
	Notes      string
}

// Detector is one detection tier.
type Detector interface {
	Name() string
	// Available reports why the detector cannot run (missing model, no API key).
	Available(ctx context.Context) error
	Detect(ctx context.Context, req Request) (Candidate, error)
}
