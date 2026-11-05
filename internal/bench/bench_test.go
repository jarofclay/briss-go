package bench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jarofclay/briss-go/internal/cropdef"
	"github.com/jarofclay/briss-go/internal/geom"
)

func r(l, b, rr, t float64) geom.Ratios { return geom.Ratios{L: l, B: b, R: rr, T: t} }

func TestCompare(t *testing.T) {
	truth := []geom.Ratios{r(0.1, 0.1, 0.1, 0.1)}
	if got := Compare(1, truth, truth); !got.Accepted || got.IoU != 1 || got.Waste != 0 {
		t.Fatalf("identical: %+v", got)
	}
	tight := Compare(1, []geom.Ratios{r(0.15, 0.1, 0.1, 0.1)}, truth)
	if !tight.Clipped || tight.Accepted || tight.Waste >= 0 {
		t.Fatalf("cutting 5%% into the label must clip: %+v", tight)
	}
	loose := Compare(1, []geom.Ratios{r(0.09, 0.1, 0.1, 0.1)}, truth)
	if loose.Clipped || !loose.Accepted || loose.Waste <= 0 {
		t.Fatalf("1%% extra margin is accepted with positive waste: %+v", loose)
	}
	split := []geom.Ratios{r(0.1, 0.1, 0.5, 0.1), r(0.5, 0.1, 0.1, 0.1)}
	if got := Compare(1, truth, split); got.CountMatch || got.Accepted {
		t.Fatalf("one rectangle for a two-column label must not be accepted: %+v", got)
	}
	wrongSplit := []geom.Ratios{r(0.1, 0.1, 0.7, 0.1), r(0.3, 0.1, 0.1, 0.1)}
	if got := Compare(1, wrongSplit, split); got.Accepted || !got.Clipped {
		t.Fatalf("a split at the wrong place must be clipped and rejected: %+v", got)
	}
	if got := Compare(1, nil, []geom.Ratios{{}}); !got.Accepted {
		t.Fatalf("no prediction on a do-not-crop page is right: %+v", got)
	}
}

func TestLoadAndScore(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.pdf"), []byte("%PDF"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.pdf"), []byte("%PDF"), 0o644)
	def := &cropdef.Definition{Version: 1, Source: cropdef.Source{Path: "a.pdf", Pages: 3},
		Clusters:      []cropdef.Cluster{{ID: 0, Pages: []int{1, 3}, Rects: []cropdef.Rect{{L: 0.1, B: 0.1, R: 0.1, T: 0.1}}}},
		PageOverrides: map[int][]cropdef.Rect{2: {}}}
	if err := def.Save(filepath.Join(dir, "a"+LabelSuffix)); err != nil {
		t.Fatal(err)
	}
	items, unl, err := Load(dir)
	if err != nil || len(items) != 1 || len(unl) != 1 {
		t.Fatalf("items %v unlabelled %v err %v", items, unl, err)
	}
	if len(items[0].Truth) != 3 || items[0].Truth[2][0] != (geom.Ratios{}) {
		t.Fatalf("truth %+v", items[0].Truth)
	}
	pred := map[int][]geom.Ratios{1: {r(0.1, 0.1, 0.1, 0.1)}, 3: {r(0.2, 0.1, 0.1, 0.1)}}
	fr := Score(items[0], "legacy", pred)
	if fr.Scored != 3 || fr.Accepted != 2 || fr.Clipped != 1 || len(fr.Worst) != 1 || fr.Worst[0].Page != 3 {
		t.Fatalf("score %+v", fr)
	}
	rep := &Report{Dataset: dir, Results: []FileResult{fr}, Unlabelled: unl}
	rep.Summarize()
	if s := rep.Summaries[0]; s.Pages != 3 || s.AcceptRate < 0.66 || s.AcceptRate > 0.67 {
		t.Fatalf("summary %+v", s)
	}
	md := rep.Markdown()
	if !strings.Contains(md, "| legacy | 1 | 0 | 3 |") || !strings.Contains(md, "p3 ") {
		t.Fatalf("markdown:\n%s", md)
	}
}

func TestLoadKeepsLabelOptions(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.pdf"), []byte("%PDF-1.4"), 0o644)
	def := &cropdef.Definition{Version: cropdef.Version, Source: cropdef.Source{Path: "a.pdf", Pages: 1},
		Options:       cropdef.Options{KeepHeaders: true},
		PageOverrides: map[int][]cropdef.Rect{1: {{L: 0.1, B: 0.1, R: 0.1, T: 0.1}}}}
	if err := def.Save(filepath.Join(dir, "a"+LabelSuffix)); err != nil {
		t.Fatal(err)
	}
	items, _, err := Load(dir)
	if err != nil || len(items) != 1 || !items[0].KeepHeaders {
		t.Fatalf("label options not carried: %v %+v", err, items)
	}
	pred := t.TempDir()
	def.Save(filepath.Join(pred, "a"+LabelSuffix))
	got, err := Precomputed(pred, "a.pdf")
	if err != nil || got.PageOverrides[1][0].L != 0.1 {
		t.Fatalf("precomputed: %v %+v", err, got)
	}
}
