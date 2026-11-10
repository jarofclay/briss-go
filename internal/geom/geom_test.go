package geom

import (
	"math"
	"math/rand"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestUnrotateQuarterTurns(t *testing.T) {
	r := Ratios{L: 0.1, B: 0.2, R: 0.3, T: 0.4}
	cases := []struct {
		rot  int
		want Ratios
	}{
		{0, r},
		{360, r},
		// /Rotate 90 (clockwise for display): displayed left = unrotated bottom,
		// so unrotated L = displayed T, B = displayed L, R = displayed B, T = displayed R.
		{90, Ratios{L: 0.4, B: 0.1, R: 0.2, T: 0.3}},
		{180, Ratios{L: 0.3, B: 0.4, R: 0.1, T: 0.2}},
		{270, Ratios{L: 0.2, B: 0.3, R: 0.4, T: 0.1}},
		{-90, Ratios{L: 0.2, B: 0.3, R: 0.4, T: 0.1}},
		{450, Ratios{L: 0.4, B: 0.1, R: 0.2, T: 0.3}},
	}
	for _, c := range cases {
		if got := r.Unrotate(c.rot); got != c.want {
			t.Errorf("rotation %d: got %+v want %+v", c.rot, got, c.want)
		}
	}
}

// Rotating by 90 four times must be the identity, and 90 then 270 must cancel.
func TestUnrotateGroupProperties(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 1000; i++ {
		r := Ratios{rng.Float64() / 2, rng.Float64() / 2, rng.Float64() / 2, rng.Float64() / 2}
		if got := r.Unrotate(90).Unrotate(90).Unrotate(90).Unrotate(90); got != r {
			t.Fatalf("four quarter turns changed %+v to %+v", r, got)
		}
		if got := r.Unrotate(90).Unrotate(270); got != r {
			t.Fatalf("90 then 270 changed %+v to %+v", r, got)
		}
		if got := r.Unrotate(180).Unrotate(180); got != r {
			t.Fatalf("180 twice changed %+v to %+v", r, got)
		}
	}
}

func TestScaledBoxUsesSmallerBox(t *testing.T) {
	media := Rect{0, 0, 612, 792}
	crop := Rect{36, 36, 576, 756}
	r := Ratios{L: 0.1, B: 0.1, R: 0.1, T: 0.1}
	got := ScaledBox(media, &crop, r, 0)
	want := Rect{36 + 54, 36 + 72, 36 + 486, 36 + 648}
	if !near(got.LLX, want.LLX) || !near(got.LLY, want.LLY) || !near(got.URX, want.URX) || !near(got.URY, want.URY) {
		t.Fatalf("got %+v want %+v", got, want)
	}
	// a crop box larger than the media box is ignored
	big := Rect{-100, -100, 1000, 1000}
	if got := ScaledBox(media, &big, Ratios{}, 0); got != media {
		t.Fatalf("expected media box, got %+v", got)
	}
	if got := ScaledBox(media, nil, Ratios{}, 0); got != media {
		t.Fatalf("expected media box without crop box, got %+v", got)
	}
}

// A mark at the displayed top-left of a /Rotate 90 page sits at the unrotated
// bottom-left; cropping to the displayed top-left quarter must keep it.
func TestScaledBoxRotatedSemantics(t *testing.T) {
	media := Rect{0, 0, 600, 800} // unrotated portrait; displayed 800 wide x 600 tall
	displayedTopLeftQuarter := Ratios{L: 0, B: 0.5, R: 0.5, T: 0}
	got := ScaledBox(media, nil, displayedTopLeftQuarter, 90)
	// clockwise display rotation maps unrotated (x,y) to displayed (y, x):
	// displayed left edge = unrotated bottom edge, displayed top = unrotated left.
	want := Rect{0, 0, 300, 400}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestPixelsRoundTrip(t *testing.T) {
	r := FromPixels(70, 90, 500, 700, 695, 900)
	x, y, w, h := r.ToPixels(695, 900)
	if !near(x, 70) || !near(y, 90) || !near(w, 500) || !near(h, 700) {
		t.Fatalf("round trip gave %v %v %v %v", x, y, w, h)
	}
	clipped := FromPixels(-10, -10, 2000, 2000, 695, 900)
	if clipped != (Ratios{}) {
		t.Fatalf("rectangle past the image should clip to the full page, got %+v", clipped)
	}
}

func TestClampAndValid(t *testing.T) {
	if !(Ratios{0.1, 0.1, 0.1, 0.1}).Valid() {
		t.Fatal("expected valid")
	}
	if (Ratios{0.6, 0, 0.5, 0}).Valid() {
		t.Fatal("overlapping left and right should be invalid")
	}
	c := Ratios{-0.2, 0.7, 1.4, 0.5}.Clamp(0.01)
	if !c.Valid() {
		t.Fatalf("clamped ratios should be valid, got %+v", c)
	}
}

func TestIoU(t *testing.T) {
	a := Ratios{0.1, 0.1, 0.1, 0.1}
	if !near(IoU(a, a), 1) {
		t.Fatal("IoU of identical rectangles must be 1")
	}
	if IoU(Ratios{0, 0, 0.6, 0}, Ratios{0.6, 0, 0, 0}) != 0 {
		t.Fatal("disjoint rectangles must have IoU 0")
	}
}

// Turning a rectangle moves its corners where TurnPoint moves them: a quarter
// turn clockwise takes the top-left corner to the top-right.
func TestTurnMatchesTurnPoint(t *testing.T) {
	r := Ratios{L: 0.1, B: 0.2, R: 0.3, T: 0.4}
	for _, turn := range []int{0, 90, 180, 270} {
		g := r.Turn(turn)
		x0, y0 := TurnPoint(r.L, r.T, turn)
		x1, y1 := TurnPoint(1-r.R, 1-r.B, turn)
		want := Ratios{L: math.Min(x0, x1), T: math.Min(y0, y1), R: 1 - math.Max(x0, x1), B: 1 - math.Max(y0, y1)}
		if !near(g.L, want.L) || !near(g.T, want.T) || !near(g.R, want.R) || !near(g.B, want.B) {
			t.Fatalf("turn %d: got %+v want %+v", turn, g, want)
		}
		if back := g.Turn(360 - turn); !near(back.L, r.L) || !near(back.T, r.T) {
			t.Fatalf("turn %d does not undo: %+v", turn, back)
		}
	}
	if x, y := TurnPoint(0, 0, 90); x != 1 || y != 0 {
		t.Fatalf("top-left after a clockwise quarter turn: %v %v", x, y)
	}
}
