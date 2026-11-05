package cv

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jarofclay/briss-go/internal/detect"
)

// TestProbe lists components in BRISS_CV_PROBE="doc.pdf#page" inside
// BRISS_CV_BOX="x0,y0,x1,y1" (render pixels) with their fate.
func TestProbe(t *testing.T) {
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
	_, tr := New().trace(g, detect.Options{KeepHeaders: true})
	inBox := func(c comp) bool { return c.x1 >= b[0] && c.x0 <= b[2] && c.y1 >= b[1] && c.y0 <= b[3] }
	gs, mh := lines(tr.Kept, New().P)
	t.Logf("glyph height %d", mh)
	for _, gr := range gs {
		if inBox(gr.comp) && gr.n > 1 {
			t.Logf("group   %v n=%d medH=%d base=%d", gr.comp, gr.n, gr.medH, gr.base)
		}
	}
	for _, c := range tr.Kept {
		if inBox(c) {
			t.Logf("kept    %v", c)
		}
	}
	for i, c := range tr.Lines {
		if inBox(c) {
			t.Logf("line    %v %s", c, tr.LineWhy[i])
		}
	}
	for i, c := range tr.Dropped {
		if inBox(c) {
			t.Logf("dropped %v %s", c, tr.Reasons[i])
		}
	}
}
