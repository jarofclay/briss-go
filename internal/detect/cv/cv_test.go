package cv

import (
	"image"
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/jarofclay/briss-go/internal/detect"
	"github.com/jarofclay/briss-go/internal/geom"
	"github.com/jarofclay/briss-go/internal/raster"
)

// sheet builds a gray page with optional scan artefacts.
type sheet struct {
	g   *image.Gray
	rng *rand.Rand
}

func newSheet(w, h int, seed int64) *sheet {
	g := image.NewGray(image.Rect(0, 0, w, h))
	for i := range g.Pix {
		g.Pix[i] = 238
	}
	return &sheet{g: g, rng: rand.New(rand.NewSource(seed))}
}

func (s *sheet) fill(x0, y0, x1, y1 int, v uint8) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			s.g.Pix[y*s.g.Stride+x] = v
		}
	}
}

// text paints lines of "words" (small dark blocks) inside a box.
func (s *sheet) text(x0, y0, x1, y1, lineH, gap int) {
	for y := y0; y+lineH <= y1; y += lineH + gap {
		for x := x0; x < x1; {
			wl := 6 + s.rng.Intn(22)
			if x+wl > x1 {
				wl = x1 - x
			}
			s.fill(x, y, x+wl, y+lineH, uint8(20+s.rng.Intn(40)))
			x += wl + 4
		}
	}
}

func (s *sheet) specks(n int) {
	w, h := s.g.Bounds().Dx(), s.g.Bounds().Dy()
	for i := 0; i < n; i++ {
		x, y := s.rng.Intn(w-2), s.rng.Intn(h-2)
		s.fill(x, y, x+2, y+2, 60)
	}
}

// lastLine returns the bottom of the last text line text() draws in [y0, y1).
func lastLine(y0, y1, lineH, gap int) int {
	y := y0
	for y+lineH+gap+lineH <= y1 {
		y += lineH + gap
	}
	return y + lineH
}

func px(r geom.Ratios, w, h int) [4]float64 {
	x, y, rw, rh := r.ToPixels(w, h)
	return [4]float64{x, y, x + rw, y + rh}
}

func near(t *testing.T, what string, got [4]float64, want [4]int, tol float64) {
	t.Helper()
	for i := range got {
		if math.Abs(got[i]-float64(want[i])) > tol {
			t.Fatalf("%s: got %v want %v (+-%v px)", what, got, want, tol)
		}
	}
}

// An open-book spread with a black top bar, a dark left edge, a gutter shadow
// touching top and bottom, a thin book-edge line, specks, and a margin note.
func TestTwoUpScan(t *testing.T) {
	const w, h = 1100, 850
	s := newSheet(w, h, 1)
	s.fill(0, 0, w, 18, 15)        // scanner lid bar at the top
	s.fill(0, 0, 25, h, 25)        // dark left edge
	s.fill(540, 0, 560, h, 120)    // gutter shadow
	s.fill(600, 60, 603, 800, 40)  // thin book-edge line
	s.text(90, 80, 505, 92, 10, 0) // running header of the left page
	s.text(90, 110, 505, 760, 10, 5)
	s.text(45, 140, 82, 190, 6, 3)   // margin note left of the left page
	s.text(620, 80, 1040, 92, 10, 0) // right header
	s.text(620, 110, 1040, 700, 10, 5)
	s.specks(60)
	r := New().DetectPage(s.g, detect.Options{KeepHeaders: true})
	if len(r.Rects) != 2 {
		t.Fatalf("expected two pages, got %+v", r)
	}
	pad := 4.0
	near(t, "left page", px(r.Rects[0], w, h), [4]int{45, 80, 505, lastLine(110, 760, 10, 5)}, pad+2)
	near(t, "right page", px(r.Rects[1], w, h), [4]int{620, 80, 1040, lastLine(110, 700, 10, 5)}, pad+2)
}

func TestHeadersDroppedWithoutKeepHeaders(t *testing.T) {
	const w, h = 700, 900
	s := newSheet(w, h, 2)
	s.text(80, 60, 620, 72, 10, 0)    // header, thin and detached
	s.text(80, 100, 620, 800, 12, 6)  // body
	s.text(330, 830, 370, 842, 10, 0) // page number
	keep := New().DetectPage(s.g, detect.Options{KeepHeaders: true})
	drop := New().DetectPage(s.g, detect.Options{})
	if len(keep.Rects) != 1 || len(drop.Rects) != 1 {
		t.Fatalf("one page expected: %+v %+v", keep, drop)
	}
	near(t, "keep headers", px(keep.Rects[0], w, h), [4]int{80, 60, 620, 842}, 5)
	near(t, "drop headers", px(drop.Rects[0], w, h), [4]int{80, 100, 620, lastLine(100, 800, 12, 6)}, 5)
}

func TestBlankHalfAndBlankPage(t *testing.T) {
	const w, h = 1100, 850
	s := newSheet(w, h, 3)
	s.text(620, 100, 1040, 700, 10, 5) // only the right page is printed
	s.text(60, 100, 70, 112, 10, 0)    // a tiny bleed mark on the left
	r := New().DetectPage(s.g, detect.Options{KeepHeaders: true})
	if len(r.Rects) != 1 || r.Rects[0].L < 0.5 {
		t.Fatalf("expected only the right page, got %+v", r)
	}
	blank := New().DetectPage(newSheet(600, 800, 4).g, detect.Options{})
	if len(blank.Rects) != 1 || blank.Rects[0] != (geom.Ratios{}) {
		t.Fatalf("blank page should keep the whole page: %+v", blank)
	}
}

// A landscape page whose text runs across the middle is not split.
func TestLandscapeSinglePage(t *testing.T) {
	const w, h = 1100, 850
	s := newSheet(w, h, 5)
	s.text(80, 100, 1020, 760, 10, 5)
	r := New().DetectPage(s.g, detect.Options{KeepHeaders: true})
	if len(r.Rects) != 1 {
		t.Fatalf("a full-width page must not be split: %+v", r)
	}
	near(t, "page", px(r.Rects[0], w, h), [4]int{80, 100, 1020, lastLine(100, 760, 10, 5)}, 6)
}

// A contents page: entries of varying length, leader dots and a column of
// page numbers. The numbers are part of the page; the dots are specks.
func TestContentsPageNumbers(t *testing.T) {
	const w, h = 700, 900
	s := newSheet(w, h, 6)
	y := 100
	for i := 0; i < 25; i++ {
		end := 200 + s.rng.Intn(200)
		s.text(80, y, end, y+10, 10, 0)
		for x := end + 10; x < 540; x += 9 {
			s.fill(x, y+8, x+2, y+10, 60) // leader dots
		}
		s.fill(560, y, 568, y+10, 40) // page number, two digits
		s.fill(570, y, 578, y+10, 40)
		y += 24
	}
	r := New().DetectPage(s.g, detect.Options{KeepHeaders: true})
	if len(r.Rects) != 1 {
		t.Fatalf("one page expected: %+v", r)
	}
	got := px(r.Rects[0], w, h)
	if got[2] < 575 {
		t.Fatalf("page numbers left out: %v", got)
	}
	near(t, "contents", got, [4]int{80, 100, 578, y - 14}, 6)
}

// The stacked page edges of a thick book show up as many thin vertical
// fragments beside the text; they are not content, and a spread side holding
// nothing else is blank.
func TestBookEdgeFragments(t *testing.T) {
	const w, h = 1100, 850
	s := newSheet(w, h, 7)
	edgeFrags := func(x0, x1 int) {
		for i := 0; i < 120; i++ {
			x, y := x0+s.rng.Intn(x1-x0), 40+s.rng.Intn(760)
			s.fill(x, y, x+2, y+12+s.rng.Intn(30), 50)
		}
	}
	edgeFrags(30, 60) // left page edges ...
	edgeFrags(70, 100)
	s.text(620, 100, 1040, 700, 10, 5) // ... of a blank left page
	r := New().DetectPage(s.g, detect.Options{KeepHeaders: true})
	if len(r.Rects) != 1 {
		t.Fatalf("expected only the right page, got %+v", r)
	}
	near(t, "right page", px(r.Rects[0], w, h), [4]int{620, 100, 1040, lastLine(100, 700, 10, 5)}, 6)

	s = newSheet(w, h, 8)
	edgeFrags(30, 70)
	s.text(110, 100, 500, 700, 10, 5)
	s.text(620, 100, 1040, 700, 10, 5)
	r = New().DetectPage(s.g, detect.Options{KeepHeaders: true})
	if len(r.Rects) != 2 {
		t.Fatalf("expected two pages, got %+v", r)
	}
	near(t, "left page", px(r.Rects[0], w, h), [4]int{110, 100, 500, lastLine(100, 700, 10, 5)}, 6)
}

// When a contents page leaves a wider empty run between its entries and
// their page numbers than the gutter itself, the binding shadow decides.
func TestGutterShadowWins(t *testing.T) {
	const w, h = 1100, 850
	s := newSheet(w, h, 9)
	s.fill(515, 0, 535, h, 165) // binding shadow, lighter than ink
	for y := 100; y < 700; y += 24 {
		s.text(80, y, 330, y+10, 10, 0)
		s.fill(470, y, 478, y+10, 40)
		s.fill(480, y, 488, y+10, 40)
	}
	s.text(560, 100, 1020, 700, 10, 5)
	r := New().DetectPage(s.g, detect.Options{KeepHeaders: true})
	if len(r.Rects) != 2 {
		t.Fatalf("expected two pages, got %+v", r)
	}
	left := px(r.Rects[0], w, h)
	if left[2] < 485 || left[2] > 500 {
		t.Fatalf("left page should end after its page numbers: %v", left)
	}
	near(t, "right page", px(r.Rects[1], w, h), [4]int{560, 100, 1020, lastLine(100, 700, 10, 5)}, 6)
}

// A two-page sheet whose left page leans 2 degrees counterclockwise and
// whose right page leans 1.5 degrees clockwise: the angles that straighten
// them are about -2 and +1.5.
func TestSkewAngles(t *testing.T) {
	const w, h = 1400, 1000
	s := newSheet(w, h, 11)
	s.text(100, 120, 620, 880, 11, 7)
	left := raster.Rotate(s.g, 2, 360, 500, 238)
	s2 := newSheet(w, h, 12)
	s2.text(780, 120, 1300, 880, 11, 7)
	right := raster.Rotate(s2.g, -1.5, 1040, 500, 238)
	sheet := image.NewGray(image.Rect(0, 0, w, h))
	for i := range sheet.Pix {
		sheet.Pix[i] = min(left.Pix[i], right.Pix[i])
	}
	r := New().DetectPage(sheet, detect.Options{KeepHeaders: true})
	if len(r.Rects) != 2 || len(r.Angles) != 2 {
		t.Fatalf("expected two pages with angles: %+v", r)
	}
	if math.Abs(r.Angles[0]-(-2)) > 0.15 || math.Abs(r.Angles[1]-1.5) > 0.15 {
		t.Fatalf("angles %v, want about [-2 1.5]", r.Angles)
	}
	flat := New().DetectPage(newSheetText(t), detect.Options{KeepHeaders: true})
	if len(flat.Angles) != 1 || math.Abs(flat.Angles[0]) > 0.05 {
		t.Fatalf("a level page must not be turned: %v", flat.Angles)
	}
}

func newSheetText(t *testing.T) *image.Gray {
	t.Helper()
	s := newSheet(700, 900, 13)
	s.text(80, 100, 620, 800, 12, 6)
	return s.g
}

// sideBySide puts two rendered book pages side by side with a gutter.
func sideBySide(l, r *image.Gray) *image.Gray {
	lw, h := l.Bounds().Dx(), l.Bounds().Dy()
	gap := lw / 20
	out := image.NewGray(image.Rect(0, 0, 2*lw+gap, h))
	for i := range out.Pix {
		out.Pix[i] = 255
	}
	for y := 0; y < h; y++ {
		copy(out.Pix[y*out.Stride:], l.Pix[y*l.Stride:y*l.Stride+lw])
		copy(out.Pix[y*out.Stride+lw+gap:], r.Pix[y*r.Stride:y*r.Stride+lw])
	}
	return out
}

// A two-page spread scanned sideways (the pages one above the other, the
// text running up or down the sheet) is turned upright and split into a
// left and a right page; the rectangles are those of the upright spread,
// measured on the sheet as stored, and carry the turn.
func TestAutoRotateSidewaysSpread(t *testing.T) {
	book := "../../../testdata/pdfs/book300.pdf"
	sheet := sideBySide(renderPage(t, book, 12), renderPage(t, book, 13))
	opts := detect.Options{KeepHeaders: true, AutoRotate: true}
	upright := New().DetectPage(sheet, opts)
	if len(upright.Rects) != 2 || len(upright.Turns) != 0 {
		t.Fatalf("upright spread: %+v", upright)
	}
	for _, q := range []int{90, 180, 270} {
		got := New().DetectPage(raster.RotateQuarter(sheet, q), opts)
		want := (360 - q) % 360
		if len(got.Rects) != 2 || len(got.Turns) != 2 || got.Turns[0] != want || got.Turns[1] != want {
			t.Fatalf("turned %d: %+v", q, got)
		}
		for i, r := range got.Rects {
			u := r.Turn(want) // back on the upright spread
			if math.Abs(u.L-upright.Rects[i].L) > 0.01 || math.Abs(u.T-upright.Rects[i].T) > 0.01 ||
				math.Abs(u.R-upright.Rects[i].R) > 0.01 || math.Abs(u.B-upright.Rects[i].B) > 0.01 {
				t.Fatalf("turned %d, page %d: %+v on the upright spread, want %+v", q, i+1, u, upright.Rects[i])
			}
		}
	}
	plain := New().DetectPage(raster.RotateQuarter(sheet, 90), detect.Options{KeepHeaders: true})
	if len(plain.Turns) != 0 {
		t.Fatalf("without AutoRotate nothing is turned: %+v", plain)
	}
}

// Two landscape pages printed one above the other on a portrait sheet are
// cut into a top and a bottom page; a single page whose text simply ends
// half way down, or a title page, is not.
func TestStackedPages(t *testing.T) {
	const w, h = 900, 1300
	s := newSheet(w, h, 21)
	s.text(80, 70, 820, 560, 11, 7)
	s.text(80, 720, 820, 1210, 11, 7)
	r := New().DetectPage(s.g, detect.Options{KeepHeaders: true})
	if len(r.Rects) != 2 || !strings.Contains(r.Notes, "one above the other") {
		t.Fatalf("expected a top and a bottom page: %+v", r)
	}
	near(t, "top page", px(r.Rects[0], w, h), [4]int{80, 70, 820, lastLine(70, 560, 11, 7)}, 6)
	near(t, "bottom page", px(r.Rects[1], w, h), [4]int{80, 720, 820, lastLine(720, 1210, 11, 7)}, 6)

	end := newSheet(w, h, 22) // a chapter that ends half way down
	end.text(80, 70, 820, 600, 11, 7)
	if r := New().DetectPage(end.g, detect.Options{KeepHeaders: true}); len(r.Rects) != 1 {
		t.Fatalf("a chapter end is one page: %+v", r)
	}
	title := newSheet(w, h, 23) // title high up, publisher at the foot
	title.text(200, 150, 700, 330, 22, 14)
	title.text(300, 1150, 600, 1200, 11, 7)
	if r := New().DetectPage(title.g, detect.Options{KeepHeaders: true}); len(r.Rects) != 1 {
		t.Fatalf("a title page is one page: %+v", r)
	}
}
