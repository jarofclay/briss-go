package pipeline

import (
	"context"
	"testing"

	"github.com/jarofclay/briss-go/internal/detect"
	"github.com/jarofclay/briss-go/internal/detect/legacy"
	"github.com/jarofclay/briss-go/internal/pdf"
)

func TestAnalyzeBookAndDetect(t *testing.T) {
	eng, err := pdf.NewEngine(pdf.EngineOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	ctx := context.Background()
	res, err := Analyze(ctx, eng, "../../testdata/pdfs/book300.pdf", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Doc.Pages) != 290 || len(res.Clusters) != 4 {
		t.Fatalf("pages %d clusters %d", len(res.Doc.Pages), len(res.Clusters))
	}
	c0 := res.Clusters[0]
	if b := c0.Overlay.Bounds(); b.Dx() != 695 || b.Dy() != 900 || len(c0.Cluster.Sample) != 15 {
		t.Fatalf("cluster 0 overlay %v sample %d", b, len(c0.Cluster.Sample))
	}
	if err := res.Detect(ctx, legacy.Detector{}, detect.Options{}); err != nil {
		t.Fatal(err)
	}
	d, err := res.Definition(detect.Options{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	pr, err := d.PageRects()
	if err != nil {
		t.Fatal(err)
	}
	if len(pr) != 290 {
		t.Fatalf("every page should get a rectangle, got %d", len(pr))
	}
	// the synthetic book has 90 pt inner and 72 pt outer margins on odd pages
	r := pr[3][0]
	if r.L < 0.10 || r.L > 0.16 || r.R < 0.07 || r.R > 0.13 {
		t.Fatalf("odd-page legacy rectangle implausible: %+v", r)
	}
	t.Logf("timings: %+v", res.Timings)
}
