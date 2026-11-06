package crop

import (
	"context"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jarofclay/briss-go/internal/geom"
	"github.com/jarofclay/briss-go/internal/pdf"
	"github.com/jarofclay/briss-go/internal/testpdf"
)

// darkRow returns the first row with dark pixels in column x, or -1.
func darkRow(g *image.Gray, x int) int {
	for y := 0; y < g.Bounds().Dy(); y++ {
		if g.Pix[y*g.Stride+x] < 100 {
			return y
		}
	}
	return -1
}

func render(t *testing.T, e *pdf.Engine, path string, page, width int) *image.Gray {
	t.Helper()
	doc, err := e.Open(context.Background(), path, "")
	if err != nil {
		t.Fatal(err)
	}
	p := doc.Pages[page]
	var out *image.Gray
	err = e.Render(context.Background(), doc, []pdf.RenderJob{{Page: page, Width: width, Height: int(float64(width) * p.Height / p.Width)}}, func(_ pdf.RenderJob, g *image.Gray) error {
		out = image.NewGray(g.Bounds())
		copy(out.Pix, g.Pix)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// A level bar turned 10 degrees counterclockwise rises to the right; invisible
// words are extractable where they were put, on plain and on /Rotate 90 pages,
// and each half of a split sheet carries only its own words.
func TestStraightenAndTextLayer(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "scan.pdf")
	bar := testpdf.Box{LLX: 50, LLY: 300, URX: 350, URY: 310}
	doc := testpdf.Doc{Pages: []testpdf.Page{
		{Media: testpdf.Box{URX: 400, URY: 600}, Fill: []testpdf.Box{bar}},
		{Media: testpdf.Box{URX: 600, URY: 800}, Rotate: 90},
	}}
	if err := os.WriteFile(src, testpdf.Build(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "out.pdf")
	full := geom.Ratios{L: 0.05, B: 0.05, R: 0.05, T: 0.05}
	left := geom.Ratios{L: 0.05, B: 0.1, R: 0.55, T: 0.1}
	right := geom.Ratios{L: 0.55, B: 0.1, R: 0.05, T: 0.1}
	outs := map[int][]Output{
		1: {{Rect: full, Angle: 10, Words: []Word{{Text: "Hello", Box: geom.Ratios{L: 0.2, B: 0.75, R: 0.6, T: 0.2}}}}},
		2: {
			{Rect: left, Words: []Word{{Text: "Left(1)", Box: geom.Ratios{L: 0.1, B: 0.7, R: 0.7, T: 0.25}}}},
			{Rect: right, Angle: -5, Words: []Word{{Text: "Café", Box: geom.Ratios{L: 0.6, B: 0.7, R: 0.2, T: 0.25}}}},
		},
	}
	st, err := WriteOutputs(context.Background(), src, dst, outs, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if st.OutputPages != 3 {
		t.Fatalf("pages: %+v", st)
	}
	e := testEngine(t)
	g := render(t, e, dst, 0, 360)
	l, r := darkRow(g, 70), darkRow(g, 290)
	if l < 0 || r < 0 || r >= l-20 {
		t.Fatalf("bar should rise to the right after a counterclockwise turn: left row %d, right row %d", l, r)
	}
	if v := g.Pix[(g.Bounds().Dy()/4)*g.Stride+g.Bounds().Dx()/2]; v < 250 {
		t.Fatalf("the text layer must be invisible, got grey %d", v)
	}
	od, err := e.Open(context.Background(), dst, "")
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"Hello", "Left(1)", "Café"} {
		text, runs, err := e.PageText(context.Background(), od, i)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(text, want) {
			t.Fatalf("page %d text %q lacks %q", i+1, text, want)
		}
		for _, other := range []string{"Hello", "Left(1)", "Café"} {
			if other != want && strings.Contains(text, other) {
				t.Fatalf("page %d carries another page's word %q", i+1, other)
			}
		}
		if i == 1 { // a /Rotate 90 half: the word must sit where it was put
			if len(runs) == 0 {
				t.Fatal("no text runs")
			}
			// The source is 600x800 user space shown turned a quarter: display x is
			// user y and display y is user x, so the word (display x 80..240,
			// y 150..180) lies at user x 150..180, y 80..240.
			ru := runs[0]
			if ru.Left < 145 || ru.Right > 185 || ru.Bottom < 75 || ru.Top > 245 || ru.Top-ru.Bottom < 140 {
				t.Fatalf("word box %+v not where it was put", ru)
			}
		}
	}
}

func TestEncodeText(t *testing.T) {
	if got := pdfString(encodeText("a(b)\\é✓")); got != `(a\(b\)\\\351?)` {
		t.Fatalf("got %s", got)
	}
}
