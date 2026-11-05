// Package cluster groups pages that can share crop rectangles, like the Java
// Briss: same parity (odd or even page number) and nearly the same size. It
// also picks the pages sampled for each cluster's overlay image.
package cluster

import "math"

// DefaultTolerance is the size difference in points below which two pages
// merge. Java Briss bucketed sizes into 20 pt bins, which split near-identical
// sizes on either side of a bin edge; a +-10 pt window has the same width
// without that artifact.
const DefaultTolerance = 10

// MaxSample is the number of pages rendered per cluster (Java MAX_MERGE_PAGES).
const MaxSample = 15

// PageSize is a page's displayed size in points.
type PageSize struct{ Width, Height float64 }

// Options configures Build.
type Options struct {
	// Tolerance in points; zero selects DefaultTolerance.
	Tolerance float64
	// Exclude holds 1-based page numbers that never merge with other pages;
	// each becomes its own cluster.
	Exclude map[int]bool
}

// Cluster is a group of pages sharing crop rectangles.
type Cluster struct {
	ID       int
	Even     bool    // pages have even 1-based numbers
	Width    float64 // displayed size of the first page, points
	Height   float64
	Excluded bool  // an excluded single page
	Pages    []int // 1-based page numbers, ascending
	Sample   []int // 1-based pages rendered for the overlay
}

// Build clusters pages (index 0 is page 1). Clusters are ordered by their first
// page, as in Java.
func Build(pages []PageSize, opts Options) []*Cluster {
	tol := opts.Tolerance
	if tol <= 0 {
		tol = DefaultTolerance
	}
	var out []*Cluster
	for i, p := range pages {
		n := i + 1
		even := n%2 == 0
		excluded := opts.Exclude[n]
		var hit *Cluster
		if !excluded {
			for _, c := range out {
				if !c.Excluded && c.Even == even && math.Abs(c.Width-p.Width) <= tol && math.Abs(c.Height-p.Height) <= tol {
					hit = c
					break
				}
			}
		}
		if hit == nil {
			hit = &Cluster{ID: len(out), Even: even, Width: p.Width, Height: p.Height, Excluded: excluded}
			out = append(out, hit)
		}
		hit.Pages = append(hit.Pages, n)
	}
	for _, c := range out {
		c.Sample = Sample(c.Pages)
	}
	return out
}

// Sample ports PageCluster.choosePagesToMerge: all pages when there are fewer
// than MaxSample, otherwise MaxSample pages evenly spaced. The step is
// accumulated in float32 exactly like Java so the same pages are chosen.
func Sample(pages []int) []int {
	if len(pages) < MaxSample {
		return append([]int(nil), pages...)
	}
	step := float32(len(pages)) / float32(MaxSample)
	var total float32
	out := make([]int, 0, MaxSample)
	for i := 0; i < MaxSample; i++ {
		out = append(out, pages[int(math.Floor(float64(total)))])
		total += step
	}
	return out
}

// ForPage returns the cluster containing the 1-based page, or nil.
func ForPage(clusters []*Cluster, page int) *Cluster {
	for _, c := range clusters {
		for _, p := range c.Pages {
			if p == page {
				return c
			}
		}
	}
	return nil
}
