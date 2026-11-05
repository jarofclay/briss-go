package pdf

import (
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jarofclay/briss-go/internal/testpdf"
)

var (
	engOnce sync.Once
	eng     *Engine
	engErr  error
)

// testEngine shares one pool across the package's tests; the compiled module is
// cached in DefaultCacheDir so only the first test binary pays for the compile.
func testEngine(t *testing.T) *Engine {
	t.Helper()
	engOnce.Do(func() { eng, engErr = NewEngine(EngineOptions{Workers: 2}) })
	if engErr != nil {
		t.Fatal(engErr)
	}
	return eng
}

func writeFixture(t *testing.T, d testpdf.Doc) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fixture.pdf")
	if err := os.WriteFile(p, testpdf.Build(d), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestOpenReportsDisplayedSizes(t *testing.T) {
	e := testEngine(t)
	doc, err := e.Open(context.Background(), writeFixture(t, testpdf.Navigation()), "")
	if err != nil {
		t.Fatal(err)
	}
	want := [][2]float64{{612, 792}, {612, 792}, {792, 612}, {540, 720}, {792, 612}}
	if len(doc.Pages) != len(want) {
		t.Fatalf("pages: got %d want %d", len(doc.Pages), len(want))
	}
	for i, w := range want {
		if p := doc.Pages[i]; p.Width != w[0] || p.Height != w[1] {
			t.Errorf("page %d: got %vx%v want %vx%v (rotation and crop box must be applied)", i+1, p.Width, p.Height, w[0], w[1])
		}
	}
}

func TestPasswordErrors(t *testing.T) {
	e := testEngine(t)
	path := "../../testdata/pdfs/example-protected.pdf"
	ctx := context.Background()
	if _, err := e.Open(ctx, path, ""); !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("no password: got %v want ErrPasswordRequired", err)
	}
	if _, err := e.Open(ctx, path, "wrong"); !errors.Is(err, ErrBadPassword) {
		t.Fatalf("wrong password: got %v want ErrBadPassword", err)
	}
	doc, err := e.Open(ctx, path, "secret")
	if err != nil {
		t.Fatalf("right password: %v", err)
	}
	if len(doc.Pages) != 1 {
		t.Fatalf("pages: %d", len(doc.Pages))
	}
	var rendered int
	err = e.Render(ctx, doc, []RenderJob{{Page: 0, Width: 300, Height: 424}}, func(_ RenderJob, g *image.Gray) error {
		rendered++
		return nil
	})
	if err != nil || rendered != 1 {
		t.Fatalf("render encrypted page: rendered=%d err=%v", rendered, err)
	}
}

// The rotated fixture page has a black square at the unrotated bottom-left
// corner, which must appear at the displayed top-left of the rendering.
func TestRenderAppliesRotation(t *testing.T) {
	e := testEngine(t)
	ctx := context.Background()
	doc, err := e.Open(ctx, writeFixture(t, testpdf.Navigation()), "")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	got := map[int][2]uint8{}
	jobs := []RenderJob{{Page: 2, Width: 792, Height: 612}, {Page: 0, Width: 612, Height: 792}}
	err = e.Render(ctx, doc, jobs, func(j RenderJob, g *image.Gray) error {
		b := g.Bounds()
		if b.Dx() < j.Width-1 || b.Dy() < j.Height-1 {
			t.Errorf("page %d rendered at %v, want about %dx%d", j.Page+1, b.Size(), j.Width, j.Height)
		}
		tl := g.GrayAt(20, 20).Y
		br := g.GrayAt(b.Dx()-20, b.Dy()-20).Y
		mu.Lock()
		got[j.Page] = [2]uint8{tl, br}
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got[2][0] > 64 || got[2][1] < 192 {
		t.Errorf("rotated page: top-left=%d (want dark) bottom-right=%d (want white)", got[2][0], got[2][1])
	}
	if got[0][0] < 192 {
		t.Errorf("page 1 top-left corner should be white margin, got %d", got[0][0])
	}
}

func TestRenderPropagatesCallbackError(t *testing.T) {
	e := testEngine(t)
	ctx := context.Background()
	doc, err := e.Open(ctx, writeFixture(t, testpdf.Navigation()), "")
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	var jobs []RenderJob
	for i := 0; i < 5; i++ {
		jobs = append(jobs, RenderJob{Page: i, Width: 100, Height: 100})
	}
	err = e.Render(ctx, doc, jobs, func(RenderJob, *image.Gray) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("got %v want boom", err)
	}
}

func TestNavigation(t *testing.T) {
	e := testEngine(t)
	ctx := context.Background()
	doc, err := e.Open(ctx, writeFixture(t, testpdf.Navigation()), "")
	if err != nil {
		t.Fatal(err)
	}
	nav, err := e.Navigation(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	wantOutline := []OutlineItem{{"Start", 0, 0}, {"Chapter 2", 0, 1}, {"Rotated page", 1, 2}, {"Cropped page", 1, 3}, {"Landscape", 0, 4}}
	if len(nav.Outline) != len(wantOutline) {
		t.Fatalf("outline: got %+v", nav.Outline)
	}
	for i, w := range wantOutline {
		if nav.Outline[i] != w {
			t.Errorf("outline %d: got %+v want %+v", i, nav.Outline[i], w)
		}
	}
	for name, page := range map[string]int{"chapter2": 1, "rotated": 2, "landscape": 4} {
		if got, ok := nav.NamedDests[name]; !ok || got != page {
			t.Errorf("named dest %q: got %d (present=%v) want %d", name, got, ok, page)
		}
	}
	wantLinks := []LinkTarget{{0, 1}, {0, 4}, {4, 1}}
	if len(nav.Links) != len(wantLinks) {
		t.Fatalf("links: got %+v want %+v", nav.Links, wantLinks)
	}
	for i, w := range wantLinks {
		if nav.Links[i] != w {
			t.Errorf("link %d: got %+v want %+v", i, nav.Links[i], w)
		}
	}
}

func TestDefaultWorkersBounds(t *testing.T) {
	if n := DefaultWorkers(); n < 1 || n > 4 {
		t.Fatalf("DefaultWorkers() = %d, want 1..4", n)
	}
}
