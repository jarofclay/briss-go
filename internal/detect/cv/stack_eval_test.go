package cv

import (
	"context"
	"image"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/jarofclay/briss-go/internal/detect"
	"github.com/jarofclay/briss-go/internal/pdf"
)

// TestStackEval counts the pages of the PDFs in BRISS_CV_STACK that the
// detector would cut into a top and a bottom page (none should be, on
// ordinary single pages). Skipped when unset.
func TestStackEval(t *testing.T) {
	in := os.Getenv("BRISS_CV_STACK")
	if in == "" {
		t.Skip("set BRISS_CV_STACK")
	}
	eng, err := pdf.NewEngine(pdf.EngineOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	d := New()
	pages, cut := 0, 0
	for _, path := range strings.Split(in, ",") {
		doc, err := eng.Open(context.Background(), path, "")
		if err != nil {
			t.Fatal(err)
		}
		var jobs []pdf.RenderJob
		for i, pg := range doc.Pages {
			w := d.RenderWidth()
			jobs = append(jobs, pdf.RenderJob{Page: i, Width: w, Height: int(math.Round(float64(w) * pg.Height / pg.Width))})
		}
		hits := make(chan int, len(jobs))
		eng.Render(context.Background(), doc, jobs, func(j pdf.RenderJob, g *image.Gray) error {
			if _, tr := d.trace(g, detect.Options{KeepHeaders: true}); tr.RowSplit > 0 {
				hits <- j.Page + 1
			} else {
				hits <- 0
			}
			return nil
		})
		close(hits)
		for p := range hits {
			pages++
			if p > 0 {
				cut++
				t.Logf("%s page %d cut into top and bottom", path[strings.LastIndex(path, "/")+1:], p)
			}
		}
	}
	t.Logf("stacked: %d of %d pages cut", cut, pages)
}
