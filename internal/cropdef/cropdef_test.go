package cropdef

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jarofclay/briss-go/internal/geom"
)

func sample() *Definition {
	return &Definition{
		Version: Version,
		Source:  Source{Path: "book.pdf", Pages: 4},
		Clusters: []Cluster{
			{ID: 0, Pages: []int{1, 3}, BoxPt: [2]float64{612, 792}, Rects: []Rect{{L: 0.1, B: 0.1, R: 0.1, T: 0.1}}, Source: "legacy@1"},
			{ID: 1, Pages: []int{2, 4}, Even: true, BoxPt: [2]float64{612, 792}, Rects: []Rect{{L: 0.1, B: 0.1, R: 0.5, T: 0.1}, {L: 0.5, B: 0.1, R: 0.1, T: 0.1}}},
		},
		PageOverrides: map[int][]Rect{3: {{L: 0.2, B: 0.2, R: 0.2, T: 0.2}}, 4: {}},
	}
}

func TestPageRects(t *testing.T) {
	pr, err := sample().PageRects()
	if err != nil {
		t.Fatal(err)
	}
	if len(pr[1]) != 1 || pr[1][0] != (geom.Ratios{L: 0.1, B: 0.1, R: 0.1, T: 0.1}) {
		t.Fatalf("page 1: %v", pr[1])
	}
	if len(pr[2]) != 2 {
		t.Fatalf("page 2 should have two rectangles: %v", pr[2])
	}
	if pr[3][0].L != 0.2 {
		t.Fatalf("page 3 override not applied: %v", pr[3])
	}
	if _, ok := pr[4]; ok {
		t.Fatal("an empty override must leave the page uncropped")
	}
}

func TestRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "crops.json")
	if err := sample().Save(p); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	if !strings.Contains(string(raw), `"page_overrides": {`) || !strings.Contains(string(raw), `"3": [`) {
		t.Fatalf("unexpected JSON:\n%s", raw)
	}
	d, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Clusters) != 2 || d.Clusters[1].Rects[1].L != 0.5 {
		t.Fatalf("round trip lost data: %+v", d)
	}
}

func TestValidateErrors(t *testing.T) {
	cases := map[string]func(d *Definition){
		"version":        func(d *Definition) { d.Version = 2 },
		"page range":     func(d *Definition) { d.Clusters[0].Pages = append(d.Clusters[0].Pages, 9) },
		"duplicate page": func(d *Definition) { d.Clusters[1].Pages = append(d.Clusters[1].Pages, 1) },
		"bad rect":       func(d *Definition) { d.Clusters[0].Rects[0].L = 0.95 },
		"bad override":   func(d *Definition) { d.PageOverrides[3] = []Rect{{L: -0.1}} },
	}
	for name, mutate := range cases {
		d := sample()
		mutate(d)
		if err := d.Validate(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestHashFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x")
	os.WriteFile(p, []byte("abc"), 0o644)
	h, err := HashFile(p)
	if err != nil || h != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("hash %s err %v", h, err)
	}
}
