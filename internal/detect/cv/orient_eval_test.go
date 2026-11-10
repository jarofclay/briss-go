package cv

import (
	"context"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jarofclay/briss-go/internal/detect"
	"github.com/jarofclay/briss-go/internal/pdf"
	"github.com/jarofclay/briss-go/internal/raster"
)

// TestOrientationEval renders every page of the PDFs in BRISS_CV_ORIENT
// (comma-separated), turns each a quarter at a time and reports how often
// the detector finds the turn that puts it back. Skipped when unset.
func TestOrientationEval(t *testing.T) {
	in := os.Getenv("BRISS_CV_ORIENT")
	if in == "" {
		t.Skip("set BRISS_CV_ORIENT")
	}
	eng, err := pdf.NewEngine(pdf.EngineOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	ctx := context.Background()
	d := New()
	total, right, wrong, none := 0, 0, 0, 0
	for fi, path := range strings.Split(in, ",") {
		doc, err := eng.Open(ctx, path, "")
		if err != nil {
			t.Fatal(err)
		}
		var jobs []pdf.RenderJob
		for i, pg := range doc.Pages {
			w := d.RenderWidth()
			jobs = append(jobs, pdf.RenderJob{Page: i, Width: w, Height: int(math.Round(float64(w) * pg.Height / pg.Width))})
		}
		type res struct{ page, q, got int }
		results := make(chan res, 4*len(jobs))
		err = eng.Render(ctx, doc, jobs, func(j pdf.RenderJob, g *image.Gray) error {
			for _, q := range []int{0, 90, 180, 270} {
				img := raster.RotateQuarter(g, q)
				_, tr := d.trace(img, detect.Options{KeepHeaders: true, AutoRotate: true})
				results <- res{j.Page + 1, q, tr.Turn}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		close(results)
		for r := range results {
			total++
			want := (360 - r.q) % 360
			switch {
			case r.got == want:
				right++
			case r.got == 0:
				none++
				t.Logf("file %d (%s) page %d turned %d: not turned back", fi, filepath.Base(path)[:4], r.page, r.q)
			default:
				wrong++
				t.Logf("file %d (%s) page %d turned %d: WRONG turn %d", fi, filepath.Base(path)[:4], r.page, r.q, r.got)
			}
		}
	}
	t.Logf("orientation: %d cases, %d right, %d left alone, %d wrong", total, right, none, wrong)
}
