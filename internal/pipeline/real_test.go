package pipeline

// TestRealPDFs runs the whole legacy pipeline (analyse, detect, crop, write)
// on every PDF below $BRISS_REAL_PDFS and checks the output: page count,
// readable pages, and that every outline item, named destination and link
// still points at the first copy of its original page. It is skipped unless
// the variable is set, so any local corpus can be used without committing it:
//
//	BRISS_REAL_PDFS=~/pdfs BRISS_REAL_PDFS_PASSWORDS=openpassword go test -run RealPDFs -v ./internal/pipeline/

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jarofclay/briss-go/internal/crop"
	"github.com/jarofclay/briss-go/internal/detect"
	"github.com/jarofclay/briss-go/internal/detect/legacy"
	"github.com/jarofclay/briss-go/internal/geom"
	"github.com/jarofclay/briss-go/internal/pdf"
)

func TestRealPDFs(t *testing.T) {
	dir := os.Getenv("BRISS_REAL_PDFS")
	if dir == "" {
		t.Skip("set BRISS_REAL_PDFS to a directory of PDFs to run this test")
	}
	passwords := append([]string{""}, strings.Split(os.Getenv("BRISS_REAL_PDFS_PASSWORDS"), ",")...)
	var files []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".pdf") {
			files = append(files, p)
		}
		return nil
	})
	if len(files) == 0 {
		t.Fatalf("no PDFs below %s", dir)
	}
	eng := goldenEngine(t)
	ctx := context.Background()
	for _, f := range files {
		rel, _ := filepath.Rel(dir, f)
		t.Run(rel, func(t *testing.T) {
			var res *Result
			var err error
			var pw string
			for _, pw = range passwords {
				res, err = Analyze(ctx, eng, f, Options{Password: pw})
				if !errors.Is(err, pdf.ErrPasswordRequired) && !errors.Is(err, pdf.ErrBadPassword) {
					break
				}
			}
			if err != nil {
				if strings.Contains(err.Error(), "pdf: opening") {
					// PDFium itself cannot read the source (for example a trailer
					// without /Root); that is the file, not briss.
					t.Skipf("source unreadable: %v", err)
				}
				t.Fatalf("analyse: %v", err)
			}
			for _, split := range []bool{false, true} {
				t0 := time.Now()
				opts := detect.Options{SplitColumns: split}
				if err := res.Detect(ctx, legacy.Detector{}, opts); err != nil {
					t.Fatal(err)
				}
				def, err := res.Definition(opts, nil)
				if err != nil {
					t.Fatal(err)
				}
				rects, err := def.PageRects()
				if err != nil {
					t.Fatal(err)
				}
				dst := filepath.Join(t.TempDir(), "out.pdf")
				st, err := crop.Write(ctx, f, dst, rects, crop.Options{Password: pw})
				if err != nil {
					t.Fatalf("write (split=%v): %v", split, err)
				}
				checkNavigation(t, eng, f, dst, pw, rects, st)
				t.Logf("split=%v: %d -> %d pages, %d clusters, %s (analyse %s)", split, st.SourcePages, st.OutputPages, len(res.Clusters), time.Since(t0).Round(time.Millisecond), (res.Timings.Open + res.Timings.Render).Round(time.Millisecond))
			}
		})
	}
}

// checkNavigation compares the source's outline, named destinations and links
// with the output's, mapping each source page to its first output copy.
func checkNavigation(t *testing.T, eng *pdf.Engine, src, dst, pw string, rects map[int][]geom.Ratios, st crop.Stats) {
	t.Helper()
	ctx := context.Background()
	sdoc, err := eng.Open(ctx, src, pw)
	if err != nil {
		t.Fatal(err)
	}
	ddoc, err := eng.Open(ctx, dst, pw)
	if err != nil {
		t.Fatalf("output does not open: %v", err)
	}
	if len(ddoc.Pages) != st.OutputPages {
		t.Fatalf("output has %d pages, writer reported %d", len(ddoc.Pages), st.OutputPages)
	}
	first := make([]int, len(sdoc.Pages)) // source index -> first output index
	o := 0
	for i := range sdoc.Pages {
		first[i] = o
		o += max(1, len(rects[i+1]))
	}
	if o != len(ddoc.Pages) {
		t.Fatalf("expected %d output pages, got %d", o, len(ddoc.Pages))
	}
	mapPage := func(p int) int {
		if p < 0 || p >= len(first) {
			return p
		}
		return first[p]
	}
	sn, err := eng.Navigation(ctx, sdoc)
	if err != nil {
		t.Fatal(err)
	}
	dn, err := eng.Navigation(ctx, ddoc)
	if err != nil {
		t.Fatal(err)
	}
	if len(sn.Outline) != len(dn.Outline) {
		t.Fatalf("outline: %d items in, %d out", len(sn.Outline), len(dn.Outline))
	}
	for i, it := range sn.Outline {
		if dn.Outline[i].Title != it.Title || dn.Outline[i].Page != mapPage(it.Page) {
			t.Errorf("outline %q: page %d, want %d", it.Title, dn.Outline[i].Page, mapPage(it.Page))
		}
	}
	for name, p := range sn.NamedDests {
		if got, ok := dn.NamedDests[name]; !ok || got != mapPage(p) {
			t.Errorf("named destination %q: page %d (present %v), want %d", name, got, ok, mapPage(p))
		}
	}
	// links: every source link must exist on the first copy of its page with the mapped target
	type key struct{ from, to int }
	want := map[key]int{}
	for _, l := range sn.Links {
		want[key{mapPage(l.FromPage), mapPage(l.ToPage)}]++
	}
	got := map[key]int{}
	for _, l := range dn.Links {
		got[key{l.FromPage, l.ToPage}]++
	}
	for k, n := range want {
		if got[k] < n {
			t.Errorf("link from output page %d to %d: %d in source, %d in output", k.from+1, k.to+1, n, got[k])
		}
	}
}
