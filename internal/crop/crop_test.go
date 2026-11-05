package crop

import (
	"context"
	"image"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jarofclay/briss-go/internal/geom"
	"github.com/jarofclay/briss-go/internal/pdf"
	"github.com/jarofclay/briss-go/internal/testpdf"
	"github.com/pdfcpu/pdfcpu/pkg/api"
)

var (
	engOnce sync.Once
	eng     *pdf.Engine
	engErr  error
)

func testEngine(t *testing.T) *pdf.Engine {
	t.Helper()
	engOnce.Do(func() { eng, engErr = pdf.NewEngine(pdf.EngineOptions{Workers: 2}) })
	if engErr != nil {
		t.Fatal(engErr)
	}
	return eng
}

func TestRecommendedDestination(t *testing.T) {
	for in, want := range map[string]string{"a/book.pdf": "a/book_cropped.pdf", "x.PDF": "x_cropped.pdf", "noext": "noext_cropped.pdf"} {
		if got := RecommendedDestination(in); got != want {
			t.Errorf("%s: got %s want %s", in, got, want)
		}
	}
}

// Crop the navigation fixture: page 2 into two columns, page 3 (rotated) to its
// displayed top-left quarter, page 4 (crop box smaller than media box) by 10%,
// page 5 untouched. Then check page count, boxes, the rotated mark, and that
// every outline item, named destination and link lands on the right output page.
func TestWriteKeepsNavigationAndBoxes(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "nav.pdf")
	if err := os.WriteFile(src, testpdf.Build(testpdf.Navigation()), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "nav_cropped.pdf")
	pages := map[int][]geom.Ratios{
		1: {{L: 0.1, B: 0.1, R: 0.1, T: 0.1}},
		2: {{L: 0.1, B: 0.1, R: 0.5, T: 0.1}, {L: 0.5, B: 0.1, R: 0.1, T: 0.1}},
		3: {{L: 0, B: 0.5, R: 0.5, T: 0}},
		4: {{L: 0.1, B: 0.1, R: 0.1, T: 0.1}},
	}
	st, err := Write(context.Background(), src, dst, pages, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if st.SourcePages != 5 || st.OutputPages != 6 || st.Clones != 1 {
		t.Fatalf("stats %+v", st)
	}

	// boxes, read back with pdfcpu
	pctx, err := api.ReadContextFile(context.Background(), dst)
	if err != nil {
		t.Fatal(err)
	}
	r := func(a, b, c, d float64) geom.Rect { return geom.Rect{LLX: a, LLY: b, URX: c, URY: d} }
	wantBoxes := []geom.Rect{
		r(61.2, 79.2, 550.8, 712.8), // page 1
		r(61.2, 79.2, 306, 712.8),   // page 2 left column
		r(306, 79.2, 550.8, 712.8),  // page 2 right column (clone)
		r(0, 0, 306, 396),           // page 3: /Rotate 90, displayed top-left quarter = unrotated bottom-left
		r(90, 108, 522, 684),        // page 4: 10% of the 540x720 crop box at (36,36)
		r(0, 0, 792, 612),           // page 5 unchanged
	}
	for i, w := range wantBoxes {
		d, _, attrs, err := pctx.PageDict(context.Background(), i+1, false)
		if err != nil {
			t.Fatal(err)
		}
		mb := attrs.MediaBox
		got := geom.Rect{LLX: mb.LL.X, LLY: mb.LL.Y, URX: mb.UR.X, URY: mb.UR.Y}
		if math.Abs(got.LLX-w.LLX) > 0.01 || math.Abs(got.LLY-w.LLY) > 0.01 || math.Abs(got.URX-w.URX) > 0.01 || math.Abs(got.URY-w.URY) > 0.01 {
			t.Errorf("output page %d media box %+v want %+v", i+1, got, w)
		}
		if i < 5 && attrs.CropBox != nil && *attrs.CropBox != *mb {
			t.Errorf("output page %d: crop box %v differs from media box %v", i+1, attrs.CropBox, mb)
		}
		if i == 3 && d.IntEntry("Rotate") == nil {
			t.Errorf("rotated page lost /Rotate")
		}
	}

	// navigation, read back with PDFium (independent of the writer)
	e := testEngine(t)
	doc, err := e.Open(context.Background(), dst, "")
	if err != nil {
		t.Fatal(err)
	}
	nav, err := e.Navigation(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	// source pages 1..5 are output pages 1, 2(+clone 3), 4, 5, 6 -> 0-based 0, 1, 3, 4, 5
	wantOutline := map[string]int{"Start": 0, "Chapter 2": 1, "Rotated page": 3, "Cropped page": 4, "Landscape": 5}
	for _, it := range nav.Outline {
		if w, ok := wantOutline[it.Title]; !ok || it.Page != w {
			t.Errorf("outline %q -> page %d, want %d", it.Title, it.Page, w)
		}
	}
	if len(nav.Outline) != len(wantOutline) {
		t.Errorf("outline has %d items, want %d", len(nav.Outline), len(wantOutline))
	}
	for name, w := range map[string]int{"chapter2": 1, "rotated": 3, "landscape": 5} {
		if got := nav.NamedDests[name]; got != w {
			t.Errorf("named dest %q -> %d, want %d", name, got, w)
		}
	}
	wantLinks := []pdf.LinkTarget{{FromPage: 0, ToPage: 1}, {FromPage: 0, ToPage: 5}, {FromPage: 5, ToPage: 1}}
	if len(nav.Links) != len(wantLinks) {
		t.Fatalf("links %+v want %+v", nav.Links, wantLinks)
	}
	for i, w := range wantLinks {
		if nav.Links[i] != w {
			t.Errorf("link %d: %+v want %+v", i, nav.Links[i], w)
		}
	}

	// the rotated page shows its mark in the displayed top-left corner of the crop
	if doc.Pages[3].Width != 396 || doc.Pages[3].Height != 306 {
		t.Fatalf("rotated output page displayed as %vx%v, want 396x306", doc.Pages[3].Width, doc.Pages[3].Height)
	}
	var dark uint8 = 255
	err = e.Render(context.Background(), doc, []pdf.RenderJob{{Page: 3, Width: 396, Height: 306}}, func(_ pdf.RenderJob, g *image.Gray) error {
		dark = g.GrayAt(10, 10).Y
		return nil
	})
	if err != nil || dark > 64 {
		t.Fatalf("rotated crop should show the mark at the top-left: value %d err %v", dark, err)
	}
}

func TestWriteEncrypted(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out.pdf")
	_, err := Write(context.Background(), "../../testdata/pdfs/example-protected.pdf", dst, map[int][]geom.Ratios{1: {{L: 0.1, B: 0.1, R: 0.1, T: 0.1}}}, Options{Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	e := testEngine(t)
	if _, err := e.Open(context.Background(), dst, ""); err != pdf.ErrPasswordRequired {
		t.Fatalf("output must stay encrypted, got %v", err)
	}
	doc, err := e.Open(context.Background(), dst, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if w := doc.Pages[0].Width; math.Abs(w-0.8*595.28) > 0.05 {
		t.Fatalf("width %v, want 80%% of 595.28", w)
	}
}

func TestWriteRejectsBadInput(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out.pdf")
	src := "../../testdata/pdfs/example.pdf"
	if _, err := Write(context.Background(), src, dst, map[int][]geom.Ratios{2: {{}}}, Options{}); err == nil {
		t.Fatal("page out of range must fail")
	}
	if _, err := Write(context.Background(), src, dst, map[int][]geom.Ratios{1: {{L: 0.7, R: 0.7}}}, Options{}); err == nil {
		t.Fatal("invalid rectangle must fail")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("no output may be left behind on failure")
	}
}
