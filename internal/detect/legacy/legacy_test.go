package legacy

import (
	"context"
	"image"
	"math"
	"math/rand"
	"testing"

	"github.com/jarofclay/briss-go/internal/detect"
	"github.com/jarofclay/briss-go/internal/geom"
)

// textBlock paints a noisy "text" area (alternating dark rows of random length)
// into a white image, so the derivative-based detector sees structure.
func textBlock(img *image.Gray, x0, y0, x1, y1 int, rng *rand.Rand) {
	for y := y0; y < y1; y++ {
		if (y-y0)%6 >= 4 {
			continue
		}
		for x := x0; x < x1; x++ {
			if rng.Intn(3) > 0 {
				img.Pix[y*img.Stride+x] = uint8(rng.Intn(120))
			}
		}
	}
}

func white(w, h int) *image.Gray {
	g := image.NewGray(image.Rect(0, 0, w, h))
	for i := range g.Pix {
		g.Pix[i] = 255
	}
	return g
}

func TestAutoCropFindsTextBlock(t *testing.T) {
	g := white(695, 900)
	textBlock(g, 100, 120, 600, 800, rand.New(rand.NewSource(3)))
	r := AutoCrop(g)
	// The 30-sample look-ahead window makes the algorithm stop a few pixels
	// before the content, never inside it: check "no clip, within 2%".
	check := func(name string, got, edge float64) {
		if got > edge+1e-9 || edge-got > 0.02 {
			t.Errorf("%s: got %.4f, content edge %.4f (must be <= edge and within 0.02)", name, got, edge)
		}
	}
	check("L", r.L, 100.0/695)
	check("R", r.R, 1-600.0/695)
	check("T", r.T, 120.0/900)
	check("B", r.B, 1-800.0/900)
}

func TestBlankOverlayKeepsWholePage(t *testing.T) {
	c, err := Detector{}.Detect(context.Background(), detect.Request{Overlay: white(200, 300)})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Rects) != 1 || c.Rects[0] != (geom.Ratios{}) || c.Confidence != 0 {
		t.Fatalf("got %+v", c)
	}
}

func TestSplitColumnAtGutter(t *testing.T) {
	g := white(695, 900)
	rng := rand.New(rand.NewSource(5))
	textBlock(g, 80, 100, 335, 800, rng) // left column
	textBlock(g, 360, 100, 615, 800, rng)
	r := AutoCrop(g)
	parts := SplitColumn(g, r)
	if len(parts) != 2 {
		t.Fatalf("parts: %d", len(parts))
	}
	divider := parts[1].L
	// the gutter is at 335..360 px of 695: 0.482..0.518; the search window is 0.45..0.55 of the width
	if divider < 0.46 || divider > 0.54 {
		t.Fatalf("divider %.3f not near the gutter", divider)
	}
	if math.Abs((1-parts[0].R)-divider) > 1e-6 {
		t.Fatal("columns must meet at the divider")
	}
}

func TestSplitRowOverlap(t *testing.T) {
	g := white(695, 900)
	textBlock(g, 80, 100, 615, 800, rand.New(rand.NewSource(7)))
	r := AutoCrop(g)
	parts := SplitRow(g, r)
	top, bottom := parts[0], parts[1]
	if top.T != r.T || bottom.B != r.B {
		t.Fatalf("outer edges must be kept: %+v %+v from %+v", top, bottom, r)
	}
	// the parts overlap by 2 x 1% of the page height around the divider
	overlap := (1 - top.B) - bottom.T
	if math.Abs(overlap-2*float64(rowOverlapRatio)) > 1e-6 {
		t.Fatalf("overlap %.4f, want %.4f", overlap, 2*rowOverlapRatio)
	}
}

func TestDetectorSplits(t *testing.T) {
	g := white(400, 500)
	textBlock(g, 40, 50, 360, 450, rand.New(rand.NewSource(9)))
	c, err := Detector{}.Detect(context.Background(), detect.Request{Overlay: g, Options: detect.Options{SplitColumns: true, SplitRows: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Rects) != 4 || c.Source != Name {
		t.Fatalf("expected 4 rectangles from %s, got %d from %q", Name, len(c.Rects), c.Source)
	}
}
