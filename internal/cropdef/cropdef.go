// Package cropdef is the crop-definition file format (version 1, PLAN.md
// section 6): the contract between the CLI, the GUI and the benchmark
// datasets. A definition lists clusters of pages with their rectangles,
// optional per-page overrides and excluded pages.
package cropdef

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/jarofclay/briss-go/internal/geom"
)

// Version is the format version written by this package.
const Version = 1

// Rect is a crop rectangle in briss ratios.
type Rect struct {
	L float64 `json:"l"`
	B float64 `json:"b"`
	R float64 `json:"r"`
	T float64 `json:"t"`
}

// Ratios converts to geom.Ratios.
func (r Rect) Ratios() geom.Ratios { return geom.Ratios{L: r.L, B: r.B, R: r.R, T: r.T} }

// FromRatios converts from geom.Ratios.
func FromRatios(r geom.Ratios) Rect { return Rect{L: r.L, B: r.B, R: r.R, T: r.T} }

// Source identifies the PDF the definition was made for.
type Source struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
	Pages  int    `json:"pages"`
}

// Options records the cropping preferences in effect.
type Options struct {
	KeepHeaders  bool    `json:"keep_headers,omitempty"`
	MarginPt     float64 `json:"margin_pt,omitempty"`
	MarginPct    float64 `json:"margin_pct,omitempty"`
	Detector     string  `json:"detector,omitempty"`
	SplitColumns bool    `json:"split_columns,omitempty"`
	SplitRows    bool    `json:"split_rows,omitempty"`
}

// Cluster is a group of pages sharing rectangles.
type Cluster struct {
	ID         int        `json:"id"`
	Pages      []int      `json:"pages"`
	Even       bool       `json:"even"`
	BoxPt      [2]float64 `json:"box_pt"`
	Rects      []Rect     `json:"rects"`
	Source     string     `json:"source,omitempty"`
	Confidence float64    `json:"confidence,omitempty"`
}

// Definition is a whole crop definition.
type Definition struct {
	Version       int            `json:"version"`
	Source        Source         `json:"source"`
	Options       Options        `json:"options"`
	Clusters      []Cluster      `json:"clusters"`
	PageOverrides map[int][]Rect `json:"page_overrides,omitempty"`
	ExcludedPages []int          `json:"excluded_pages,omitempty"`
}

// Validate checks version, page ranges, that every page is in at most one
// cluster, and that every rectangle is a valid crop.
func (d *Definition) Validate() error {
	if d.Version != Version {
		return fmt.Errorf("cropdef: unsupported version %d (this build reads %d)", d.Version, Version)
	}
	if d.Source.Pages < 1 {
		return errors.New("cropdef: source page count missing")
	}
	inRange := func(p int) bool { return p >= 1 && p <= d.Source.Pages }
	seen := map[int]int{}
	for _, c := range d.Clusters {
		for _, p := range c.Pages {
			if !inRange(p) {
				return fmt.Errorf("cropdef: cluster %d: page %d out of range 1..%d", c.ID, p, d.Source.Pages)
			}
			if other, dup := seen[p]; dup {
				return fmt.Errorf("cropdef: page %d is in clusters %d and %d", p, other, c.ID)
			}
			seen[p] = c.ID
		}
		for i, r := range c.Rects {
			if !r.Ratios().Valid() {
				return fmt.Errorf("cropdef: cluster %d rectangle %d is invalid: %+v", c.ID, i, r)
			}
		}
	}
	for p, rs := range d.PageOverrides {
		if !inRange(p) {
			return fmt.Errorf("cropdef: override for page %d out of range", p)
		}
		for i, r := range rs {
			if !r.Ratios().Valid() {
				return fmt.Errorf("cropdef: page %d override rectangle %d is invalid: %+v", p, i, r)
			}
		}
	}
	for _, p := range d.ExcludedPages {
		if !inRange(p) {
			return fmt.Errorf("cropdef: excluded page %d out of range", p)
		}
	}
	return nil
}

// PageRects returns the rectangles for every page that has any: cluster
// rectangles, replaced by a page override when there is one.
func (d *Definition) PageRects() (map[int][]geom.Ratios, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	out := map[int][]geom.Ratios{}
	conv := func(rs []Rect) []geom.Ratios {
		g := make([]geom.Ratios, len(rs))
		for i, r := range rs {
			g[i] = r.Ratios()
		}
		return g
	}
	for _, c := range d.Clusters {
		if len(c.Rects) == 0 {
			continue
		}
		for _, p := range c.Pages {
			out[p] = conv(c.Rects)
		}
	}
	for p, rs := range d.PageOverrides {
		if len(rs) == 0 {
			delete(out, p)
			continue
		}
		out[p] = conv(rs)
	}
	return out, nil
}

// Load reads and validates a definition.
func Load(path string) (*Definition, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d Definition
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("cropdef: %s: %w", path, err)
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return &d, nil
}

// Save validates and writes a definition as indented JSON.
func (d *Definition) Save(path string) error {
	if err := d.Validate(); err != nil {
		return err
	}
	sort.Ints(d.ExcludedPages)
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// HashFile returns the hex SHA-256 of a file, for Source.SHA256.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
