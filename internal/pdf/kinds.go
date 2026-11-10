package pdf

import (
	"context"
	"fmt"

	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/requests"
)

// PageKind summarises the objects on a page, to tell scans from born-digital pages.
type PageKind struct {
	Page         int // 0-based
	TextObjects  int
	ImageObjects int
	LargestImage float64 // area of the largest image object as a fraction of the page
}

// Scanned reports a page that is mostly one image. An OCR text layer does not
// change that: scanners put invisible text behind the page image.
func (k PageKind) Scanned() bool { return k.LargestImage >= 0.6 }

// PageKinds inspects the given 0-based pages.
func (e *Engine) PageKinds(ctx context.Context, doc *Document, pages []int) ([]PageKind, error) {
	inst, err := e.instance(ctx)
	if err != nil {
		return nil, err
	}
	defer inst.Close()
	ref, done, err := openDoc(inst, doc.Path, doc.password)
	if err != nil {
		return nil, err
	}
	defer done()
	var out []PageKind
	for _, p := range pages {
		if p < 0 || p >= len(doc.Pages) {
			return nil, fmt.Errorf("pdf: page %d out of range", p+1)
		}
		page := requests.Page{ByIndex: &requests.PageByIndex{Document: ref, Index: p}}
		cnt, err := inst.FPDFPage_CountObjects(&requests.FPDFPage_CountObjects{Page: page})
		if err != nil {
			return nil, fmt.Errorf("pdf: objects of page %d: %w", p+1, err)
		}
		k := PageKind{Page: p}
		area := doc.Pages[p].Width * doc.Pages[p].Height
		for i := 0; i < cnt.Count; i++ {
			obj, err := inst.FPDFPage_GetObject(&requests.FPDFPage_GetObject{Page: page, Index: i})
			if err != nil {
				continue
			}
			t, err := inst.FPDFPageObj_GetType(&requests.FPDFPageObj_GetType{PageObject: obj.PageObject})
			if err != nil {
				continue
			}
			switch t.Type {
			case enums.FPDF_PAGEOBJ_TEXT:
				k.TextObjects++
			case enums.FPDF_PAGEOBJ_IMAGE:
				k.ImageObjects++
				if bd, err := inst.FPDFPageObj_GetBounds(&requests.FPDFPageObj_GetBounds{PageObject: obj.PageObject}); err == nil && area > 0 {
					a := float64(bd.Right-bd.Left) * float64(bd.Top-bd.Bottom) / area
					if a > k.LargestImage {
						k.LargestImage = a
					}
				}
			}
		}
		out = append(out, k)
	}
	return out, nil
}
