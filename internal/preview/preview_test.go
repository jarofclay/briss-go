package preview

import (
	"image"
	"testing"

	"github.com/jarofclay/briss-go/internal/geom"
)

func TestDrawTintsOutsideAndOutlines(t *testing.T) {
	g := image.NewGray(image.Rect(0, 0, 100, 100))
	for i := range g.Pix {
		g.Pix[i] = 255
	}
	out := Draw(g, []geom.Ratios{{L: 0.2, B: 0.2, R: 0.2, T: 0.2}})
	if c := out.RGBAAt(50, 50); c.R != 255 || c.G != 255 {
		t.Fatalf("inside should keep the image, got %v", c)
	}
	if c := out.RGBAAt(5, 95); c.R <= c.G {
		t.Fatalf("outside should be tinted, got %v", c)
	}
	if c := out.RGBAAt(20, 60); c != palette[0] {
		t.Fatalf("left edge should carry the outline colour, got %v", c)
	}
}
