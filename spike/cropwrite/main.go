// Spike 3: write a cropped PDF with pdfcpu by editing the page tree in place.
//
// For every source page with k crop rectangles, the page dictionary is kept for
// rectangle 1 and cloned k-1 times (shallow copy: /Contents, /Resources, /Annots
// stay shared by reference). Every output page gets /MediaBox and /CropBox set
// to its rectangle, computed exactly like the Java RectangleHandler (smallest of
// media and crop box, briss ratios rotated by /Rotate). Because page objects keep
// their identity, outlines, named destinations and link annotations keep
// pointing at the original page (= first rectangle), the same semantics the
// Java app produced by shifting bookmark page numbers.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Ratios are briss crop ratios: fractions from the left, bottom, right and top edges.
type Ratios struct{ L, B, R, T float64 }

type cropDef map[int][]Ratios // 1-based page -> rectangles

func main() {
	in := flag.String("in", "", "source PDF")
	out := flag.String("out", "", "destination PDF")
	labels := flag.String("labels", "", "labels.json from gen_corpus.py (uses book300 ground truth)")
	splitCols := flag.Bool("split-columns", true, "emit two rectangles for two-column pages")
	optimize := flag.Bool("optimize", false, "run pdfcpu optimize before writing")
	password := flag.String("password", "", "user or owner password for encrypted sources")
	flag.Parse()
	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: cropwrite -in src.pdf -out dst.pdf [-labels labels.json]")
		os.Exit(1)
	}
	c := context.Background()

	t0 := time.Now()
	f, err := os.Open(*in)
	must(err)
	conf := model.NewDefaultConfiguration()
	conf.UserPW, conf.OwnerPW = *password, *password
	ctx, err := api.ReadContext(c, f, conf)
	must(err)
	must(api.ValidateContext(c, ctx))
	f.Close()
	tRead := time.Since(t0)

	def := buildCropDef(ctx, *labels, *splitCols)

	t1 := time.Now()
	inserted, err := applyCrops(c, ctx, def)
	must(err)
	tApply := time.Since(t1)

	if *optimize {
		must(api.OptimizeContext(c, ctx))
	}
	t2 := time.Now()
	must(api.WriteContextFile(c, ctx, *out))
	tWrite := time.Since(t2)

	fi, _ := os.Stat(*out)
	si, _ := os.Stat(*in)
	fmt.Printf("read+validate=%v apply=%v write=%v total=%v\n", tRead.Round(time.Millisecond), tApply.Round(time.Millisecond), tWrite.Round(time.Millisecond), time.Since(t0).Round(time.Millisecond))
	fmt.Printf("source pages=%d output pages=%d inserted=%d size in=%d out=%d (%.1f%%)\n",
		ctx.PageCount-inserted, ctx.PageCount, inserted, si.Size(), fi.Size(), 100*float64(fi.Size())/float64(si.Size()))
}

// buildCropDef derives per-cluster rectangles from the corpus ground truth so the
// writer is exercised with realistic values: one rectangle per (parity, size)
// cluster (median of the body boxes), plus two column rectangles on two-column pages.
func buildCropDef(ctx *model.Context, labelsPath string, splitCols bool) cropDef {
	def := cropDef{}
	if labelsPath == "" {
		// no labels: crop every page by 10% on each side
		for p := 1; p <= ctx.PageCount; p++ {
			def[p] = []Ratios{{0.1, 0.1, 0.1, 0.1}}
		}
		return def
	}
	raw, err := os.ReadFile(labelsPath)
	must(err)
	var all struct {
		Book map[string]struct {
			Size  []float64 `json:"size"`
			Body  []float64 `json:"body"`
			Extra struct {
				Kind    string      `json:"kind"`
				Columns [][]float64 `json:"columns"`
			} `json:"extra"`
		} `json:"book300"`
	}
	must(json.Unmarshal(raw, &all))
	type key struct {
		even bool
		w, h int
	}
	groups := map[key][]Ratios{}
	pageKey := map[int]key{}
	for ps, lb := range all.Book {
		var p int
		fmt.Sscanf(ps, "%d", &p)
		k := key{p%2 == 0, int(lb.Size[0]), int(lb.Size[1])}
		pageKey[p] = k
		if lb.Body[2]-lb.Body[0] < 50 { // title-page style tiny boxes: skip from the median
			continue
		}
		groups[k] = append(groups[k], toRatios(lb.Body, lb.Size[0], lb.Size[1]))
		if splitCols && lb.Extra.Kind == "two-column" {
			var cols []Ratios
			for _, cb := range lb.Extra.Columns {
				cols = append(cols, toRatios(cb, lb.Size[0], lb.Size[1]))
			}
			def[p] = cols
		}
	}
	median := map[key]Ratios{}
	for k, rs := range groups {
		median[k] = Ratios{med(rs, func(r Ratios) float64 { return r.L }), med(rs, func(r Ratios) float64 { return r.B }),
			med(rs, func(r Ratios) float64 { return r.R }), med(rs, func(r Ratios) float64 { return r.T })}
	}
	for p := 1; p <= ctx.PageCount; p++ {
		if _, ok := def[p]; ok {
			continue
		}
		if k, ok := pageKey[p]; ok {
			def[p] = []Ratios{median[k]}
		}
	}
	return def
}

func toRatios(box []float64, w, h float64) Ratios {
	return Ratios{box[0] / w, box[1] / h, 1 - box[2]/w, 1 - box[3]/h}
}

func med(rs []Ratios, f func(Ratios) float64) float64 {
	v := make([]float64, len(rs))
	for i, r := range rs {
		v[i] = f(r)
	}
	sort.Float64s(v)
	return v[len(v)/2]
}

// applyCrops edits the page tree. Returns the number of inserted (cloned) pages.
func applyCrops(c context.Context, ctx *model.Context, def cropDef) (int, error) {
	n := ctx.PageCount
	// Resolve page object refs and boxes before mutating the tree, so page
	// numbers stay those of the source document.
	type pg struct {
		dict  types.Dict
		ref   *types.IndirectRef
		media *types.Rectangle
		crop  *types.Rectangle
		rot   int
	}
	pages := make([]pg, n)
	for p := 1; p <= n; p++ {
		d, ref, attrs, err := ctx.PageDict(c, p, false)
		if err != nil {
			return 0, fmt.Errorf("page %d: %w", p, err)
		}
		pages[p-1] = pg{dict: d, ref: ref, media: attrs.MediaBox, crop: attrs.CropBox, rot: attrs.Rotate}
	}
	inserted := 0
	for p := 1; p <= n; p++ {
		rects := def[p]
		if len(rects) == 0 {
			continue
		}
		cur := pages[p-1]
		prevRef := cur.ref
		for k, r := range rects {
			box := scaledBox(cur.media, cur.crop, r, cur.rot)
			target := cur.dict
			if k > 0 {
				clone := types.Dict{}
				for key, val := range cur.dict {
					clone[key] = val
				}
				ref, err := ctx.IndRefForNewObject(clone)
				if err != nil {
					return 0, err
				}
				if err := insertKidAfter(ctx, clone, prevRef, ref); err != nil {
					return 0, fmt.Errorf("page %d clone %d: %w", p, k, err)
				}
				prevRef = ref
				target = clone
				inserted++
			}
			target["MediaBox"] = box.Array()
			target["CropBox"] = box.Array()
			// Make the rotation explicit on clones so inheritance cannot drift.
			if cur.rot != 0 {
				target["Rotate"] = types.Integer(cur.rot)
			}
		}
	}
	ctx.PageCount = 0
	if err := ctx.EnsurePageCount(); err != nil {
		return 0, err
	}
	return inserted, nil
}

// insertKidAfter adds newRef right after afterRef in the parent's Kids array and
// bumps Count on every ancestor.
func insertKidAfter(ctx *model.Context, clone types.Dict, afterRef, newRef *types.IndirectRef) error {
	parentRef, ok := clone["Parent"].(types.IndirectRef)
	if !ok {
		return fmt.Errorf("page has no /Parent reference")
	}
	parent, err := ctx.DereferenceDict(parentRef)
	if err != nil {
		return err
	}
	kids := parent.ArrayEntry("Kids")
	idx := -1
	for i, k := range kids {
		if ir, ok := k.(types.IndirectRef); ok && ir.ObjectNumber == afterRef.ObjectNumber {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("page ref %d not found in parent Kids", afterRef.ObjectNumber)
	}
	nk := make(types.Array, 0, len(kids)+1)
	nk = append(nk, kids[:idx+1]...)
	nk = append(nk, *newRef)
	nk = append(nk, kids[idx+1:]...)
	parent["Kids"] = nk
	// bump Count up the chain
	d := parent
	for d != nil {
		cnt := d.IntEntry("Count")
		if cnt == nil {
			return fmt.Errorf("pages node without Count")
		}
		d["Count"] = types.Integer(*cnt + 1)
		pr, ok := d["Parent"].(types.IndirectRef)
		if !ok {
			break
		}
		d, err = ctx.DereferenceDict(pr)
		if err != nil {
			return err
		}
	}
	return nil
}

// scaledBox ports RectangleHandler.calculateScaledRectangle.
func scaledBox(media, crop *types.Rectangle, r Ratios, rotation int) *types.Rectangle {
	var smallest *types.Rectangle
	var area float64 = math.MaxFloat64
	for _, b := range []*types.Rectangle{media, crop} {
		if b == nil {
			continue
		}
		a := b.Width() * b.Height()
		if smallest == nil || a < area {
			smallest, area = b, a
		}
	}
	rr := rotateRatios(r, rotation)
	w, h := smallest.Width(), smallest.Height()
	return types.NewRectangle(smallest.LL.X+w*rr.L, smallest.LL.Y+h*rr.B, smallest.LL.X+w*(1-rr.R), smallest.LL.Y+h*(1-rr.T))
}

// rotateRatios ports RectangleHandler.rotateRatios (counter-clockwise until 0).
func rotateRatios(r Ratios, rotation int) Ratios {
	rotation = ((rotation % 360) + 360) % 360
	for rotation > 0 && rotation < 360 {
		r = Ratios{L: r.B, B: r.R, R: r.T, T: r.L}
		rotation += 90
	}
	return r
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
