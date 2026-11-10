package cv

import (
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/jarofclay/briss-go/internal/detect"
)

// TestSkewProbe prints the baseline fits of the text lines in
// BRISS_CV_BOX on page BRISS_CV_PROBE="doc.pdf#page", with BRISS_CV_GUTTER.
func TestSkewProbe(t *testing.T) {
	in, box := os.Getenv("BRISS_CV_PROBE"), os.Getenv("BRISS_CV_BOX")
	if in == "" || box == "" {
		t.Skip("set BRISS_CV_PROBE and BRISS_CV_BOX")
	}
	doc, page, _ := strings.Cut(in, ".pdf#")
	n, _ := strconv.Atoi(page)
	g := renderPage(t, doc+".pdf", n)
	var b [4]int
	for i, v := range strings.Split(box, ",") {
		b[i], _ = strconv.Atoi(v)
	}
	gutter, _ := strconv.Atoi(os.Getenv("BRISS_CV_GUTTER"))
	_, tr := New().trace(g, detect.Options{KeepHeaders: true})
	var in2 []comp
	for _, c := range tr.Kept {
		if c.cx() >= b[0] && c.cx() <= b[2] && c.cy() >= b[1] && c.cy() <= b[3] {
			in2 = append(in2, c)
		}
	}
	gs, mh := lines(in2, New().P)
	sort.Slice(gs, func(i, j int) bool { return gs[i].y0 < gs[j].y0 })
	for _, gr := range gs {
		if gr.n < 6 || gr.x1-gr.x0 < 10*mh {
			continue
		}
		full, _ := fitBaseline(baselinePoints(in2, gr, mh, -1, 0), mh)
		outer, ok := fitBaseline(baselinePoints(in2, gr, mh, gutter, 0.6), mh)
		t.Logf("line x %d-%d y %d-%d n=%d full %.2f° outer %.2f° %v", gr.x0, gr.x1, gr.y0, gr.y1, gr.n, math.Atan(full)*180/math.Pi, math.Atan(outer)*180/math.Pi, ok)
	}
}
