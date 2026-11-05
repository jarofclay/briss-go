// Package crop writes the cropped PDF.
//
// The page tree is edited in place with pdfcpu: for a page with k rectangles
// the page dictionary is kept for the first rectangle and k-1 shallow clones
// are inserted right after it, sharing /Contents, /Resources and /Annots by
// reference. Every output page gets /MediaBox and /CropBox set to its
// rectangle. Because original page objects keep their identity, outlines,
// named destinations and links keep pointing at the first copy of their page,
// which is what the Java Briss achieved by renumbering bookmarks.
package crop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jarofclay/briss-go/internal/geom"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Options configures Write.
type Options struct {
	Password string // user or owner password of an encrypted source
}

// Stats reports what Write did.
type Stats struct {
	SourcePages int
	OutputPages int
	Clones      int
}

// RecommendedDestination returns "<source without .pdf>_cropped.pdf", the
// default output name of the Java Briss.
func RecommendedDestination(src string) string {
	base := src
	if strings.EqualFold(filepath.Ext(src), ".pdf") {
		base = src[:len(src)-4]
	}
	return base + "_cropped.pdf"
}

// Write crops src into dst. pages maps 1-based source page numbers to their
// rectangles in output order; pages without an entry are copied unchanged.
// The output is written to a temporary file next to dst and renamed into place.
func Write(ctx context.Context, src, dst string, pages map[int][]geom.Ratios, opts Options) (Stats, error) {
	f, err := os.Open(src)
	if err != nil {
		return Stats{}, err
	}
	defer f.Close()
	conf := model.NewDefaultConfiguration()
	conf.UserPW, conf.OwnerPW = opts.Password, opts.Password
	pctx, err := api.ReadContext(ctx, f, conf)
	if err != nil {
		return Stats{}, fmt.Errorf("crop: reading %s: %w", src, err)
	}
	if err := api.ValidateContext(ctx, pctx); err != nil {
		return Stats{}, fmt.Errorf("crop: validating %s: %w", src, err)
	}
	st, err := apply(ctx, pctx, pages)
	if err != nil {
		return st, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".briss-*.pdf")
	if err != nil {
		return st, err
	}
	tmpName := tmp.Name()
	tmp.Close()
	if err := api.WriteContextFile(ctx, pctx, tmpName); err != nil {
		os.Remove(tmpName)
		return st, fmt.Errorf("crop: writing %s: %w", dst, err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		return st, err
	}
	return st, nil
}

type pageRef struct {
	dict  types.Dict
	ref   *types.IndirectRef
	media geom.Rect
	crop  *geom.Rect
	rot   int
}

func toRect(r *types.Rectangle) *geom.Rect {
	if r == nil {
		return nil
	}
	return &geom.Rect{LLX: r.LL.X, LLY: r.LL.Y, URX: r.UR.X, URY: r.UR.Y}
}

func apply(ctx context.Context, pctx *model.Context, pages map[int][]geom.Ratios) (Stats, error) {
	n := pctx.PageCount
	st := Stats{SourcePages: n}
	for p, rects := range pages {
		if p < 1 || p > n {
			return st, fmt.Errorf("crop: page %d out of range 1..%d", p, n)
		}
		for i, r := range rects {
			if !r.Valid() {
				return st, fmt.Errorf("crop: page %d rectangle %d is invalid: %+v", p, i, r)
			}
		}
	}
	// Resolve every page before touching the tree so page numbers stay those of the source.
	refs := make([]pageRef, n)
	for p := 1; p <= n; p++ {
		d, ref, attrs, err := pctx.PageDict(ctx, p, false)
		if err != nil {
			return st, fmt.Errorf("crop: page %d: %w", p, err)
		}
		if d == nil || ref == nil || attrs == nil || attrs.MediaBox == nil {
			return st, fmt.Errorf("crop: page %d has no media box", p)
		}
		refs[p-1] = pageRef{dict: d, ref: ref, media: *toRect(attrs.MediaBox), crop: toRect(attrs.CropBox), rot: geom.NormalizeRotation(attrs.Rotate)}
	}
	for p := 1; p <= n; p++ {
		rects := pages[p]
		cur := refs[p-1]
		prev := cur.ref
		for k, r := range rects {
			b := geom.ScaledBox(cur.media, cur.crop, r, cur.rot)
			target := cur.dict
			if k > 0 {
				clone := types.Dict{}
				for key, val := range cur.dict {
					clone[key] = val
				}
				ref, err := pctx.IndRefForNewObject(clone)
				if err != nil {
					return st, err
				}
				if err := insertKidAfter(pctx, clone, prev, ref); err != nil {
					return st, fmt.Errorf("crop: page %d copy %d: %w", p, k+1, err)
				}
				prev, target = ref, clone
				st.Clones++
			}
			box := types.NewRectangle(b.LLX, b.LLY, b.URX, b.URY).Array()
			target["MediaBox"] = box
			target["CropBox"] = box
			if cur.rot != 0 {
				target["Rotate"] = types.Integer(cur.rot) // explicit, so inheritance cannot drift
			}
		}
	}
	pctx.PageCount = 0
	if err := pctx.EnsurePageCount(); err != nil {
		return st, err
	}
	st.OutputPages = pctx.PageCount
	if st.OutputPages != n+st.Clones {
		return st, fmt.Errorf("crop: page tree has %d pages, expected %d", st.OutputPages, n+st.Clones)
	}
	return st, nil
}

// insertKidAfter puts newRef right after afterRef in the parent's /Kids and
// bumps /Count on every ancestor.
func insertKidAfter(pctx *model.Context, clone types.Dict, afterRef, newRef *types.IndirectRef) error {
	parentRef, ok := clone["Parent"].(types.IndirectRef)
	if !ok {
		return errors.New("page has no /Parent reference")
	}
	parent, err := pctx.DereferenceDict(parentRef)
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
		return fmt.Errorf("page object %d not found in its parent's /Kids", afterRef.ObjectNumber)
	}
	nk := make(types.Array, 0, len(kids)+1)
	nk = append(nk, kids[:idx+1]...)
	nk = append(nk, *newRef)
	nk = append(nk, kids[idx+1:]...)
	parent["Kids"] = nk
	for d, depth := parent, 0; d != nil && depth < 64; depth++ {
		cnt := d.IntEntry("Count")
		if cnt == nil {
			return errors.New("pages node without /Count")
		}
		d["Count"] = types.Integer(*cnt + 1)
		pr, ok := d["Parent"].(types.IndirectRef)
		if !ok {
			break
		}
		if d, err = pctx.DereferenceDict(pr); err != nil {
			return err
		}
	}
	return nil
}
