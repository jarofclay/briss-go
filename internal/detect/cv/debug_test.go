package cv

// TestDebugDump writes a colour-coded view of the detector's decisions for
// page images given in BRISS_CV_DEBUG="in1.png,in2.png" (or "doc.pdf#3" to
// render page 3 the way the app does) into the directory
// BRISS_CV_DEBUG_OUT: kept components green, dropped ones red (edge junk),
// orange (thin lines, slivers) or grey (specks), trimmed edges blue, the
// split line magenta, blocks thick black. Skipped when unset.

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jarofclay/briss-go/internal/detect"
	"github.com/jarofclay/briss-go/internal/pdf"
)

func TestDebugDump(t *testing.T) {
	in, out := os.Getenv("BRISS_CV_DEBUG"), os.Getenv("BRISS_CV_DEBUG_OUT")
	if in == "" || out == "" {
		t.Skip("set BRISS_CV_DEBUG and BRISS_CV_DEBUG_OUT")
	}
	os.MkdirAll(out, 0o755)
	for _, p := range strings.Split(in, ",") {
		var g *image.Gray
		if doc, page, ok := strings.Cut(p, ".pdf#"); ok {
			n, _ := strconv.Atoi(page)
			g = renderPage(t, doc+".pdf", n)
			p = fmt.Sprintf("%s_p%d.png", filepath.Base(doc), n)
		} else {
			f, err := os.Open(p)
			if err != nil {
				t.Fatal(err)
			}
			src, err := png.Decode(f)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			g = toGray(src)
		}
		res, tr := New().trace(g, detect.Options{KeepHeaders: true})
		t.Logf("%s: threshold %d, edges l%d t%d r%d b%d, kept %d, dropped %d, parts %v, result %+v", filepath.Base(p), tr.Threshold, tr.EdgeL, tr.EdgeT, tr.EdgeR, tr.EdgeB, len(tr.Kept), len(tr.Dropped), tr.Parts, res)
		b := g.Bounds()
		o := image.NewRGBA(b)
		for i, v := range g.Pix {
			v = 200 + v/5 // faded page
			o.Pix[4*i], o.Pix[4*i+1], o.Pix[4*i+2], o.Pix[4*i+3] = v, v, v, 255
		}
		box := func(c comp, col color.RGBA, fill bool) {
			for y := c.y0; y <= c.y1; y++ {
				for x := c.x0; x <= c.x1; x++ {
					if fill || y == c.y0 || y == c.y1 || x == c.x0 || x == c.x1 {
						o.SetRGBA(x, y, col)
					}
				}
			}
		}
		for i, c := range tr.Dropped {
			col := color.RGBA{150, 150, 150, 255}
			switch tr.Reasons[i] {
			case "edge junk", "spans the page":
				col = color.RGBA{230, 40, 40, 255}
			case "thin line", "edge sliver", "edge stripe":
				col = color.RGBA{240, 150, 0, 255}
			}
			box(c, col, false)
		}
		for _, c := range tr.Kept {
			box(c, color.RGBA{30, 170, 60, 255}, false)
		}
		for _, c := range tr.Lines {
			box(c, color.RGBA{0, 160, 200, 255}, false)
		}
		w, h := b.Dx(), b.Dy()
		blue := color.RGBA{40, 90, 230, 255}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if x < tr.EdgeL || x >= w-tr.EdgeR || y < tr.EdgeT || y >= h-tr.EdgeB {
					if (x+y)%6 == 0 {
						o.SetRGBA(x, y, blue)
					}
				}
			}
		}
		if len(tr.Parts) == 2 {
			for y := 0; y < h; y++ {
				o.SetRGBA(tr.Parts[0][1], y, color.RGBA{220, 0, 220, 255})
			}
		}
		for _, bl := range tr.Blocks {
			for k := 0; k < 3; k++ {
				box(comp{x0: bl[0] - k, y0: bl[1] - k, x1: bl[2] + k, y1: bl[3] + k}, color.RGBA{0, 0, 0, 255}, false)
			}
		}
		of, _ := os.Create(filepath.Join(out, "debug_"+filepath.Base(p)))
		png.Encode(of, o)
		of.Close()
	}
}

func toGray(src image.Image) *image.Gray {
	if g, ok := src.(*image.Gray); ok {
		return g
	}
	b := src.Bounds()
	g := image.NewGray(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			g.Set(x, y, color.GrayModel.Convert(src.At(x, y)))
		}
	}
	return g
}

// renderPage renders a 1-based page as detectPages in internal/app does.
func renderPage(t *testing.T, path string, page int) *image.Gray {
	eng, err := pdf.NewEngine(pdf.EngineOptions{Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	ctx := context.Background()
	doc, err := eng.Open(ctx, path, "")
	if err != nil {
		t.Fatal(err)
	}
	pi := doc.Pages[page-1]
	w := New().RenderWidth()
	job := pdf.RenderJob{Page: page - 1, Width: w, Height: max(1, int(float64(w)*pi.Height/pi.Width+0.5))}
	var out *image.Gray
	err = eng.Render(ctx, doc, []pdf.RenderJob{job}, func(_ pdf.RenderJob, g *image.Gray) error {
		out = image.NewGray(g.Bounds())
		copy(out.Pix, g.Pix)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
