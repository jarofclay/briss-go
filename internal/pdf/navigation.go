package pdf

import (
	"context"

	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
)

// Navigation lists where a document's outline items, named destinations and
// link annotations point, as 0-based page indexes (-1 when unresolved). It is
// used to check that cropping keeps every target on the right page.
type Navigation struct {
	Outline    []OutlineItem
	NamedDests map[string]int
	Links      []LinkTarget
}

// OutlineItem is one bookmark.
type OutlineItem struct {
	Title string
	Depth int
	Page  int
}

// LinkTarget is one internal link annotation.
type LinkTarget struct {
	FromPage int
	ToPage   int
}

// Navigation reads outline, named destinations and internal links.
func (e *Engine) Navigation(ctx context.Context, doc *Document) (*Navigation, error) {
	inst, err := e.instance(ctx)
	if err != nil {
		return nil, err
	}
	defer inst.Close()
	ref, err := openDoc(inst, doc.Path, doc.password)
	if err != nil {
		return nil, err
	}
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: ref})
	nav := &Navigation{NamedDests: map[string]int{}}

	destPage := func(d *references.FPDF_DEST) int {
		if d == nil {
			return -1
		}
		r, err := inst.FPDFDest_GetDestPageIndex(&requests.FPDFDest_GetDestPageIndex{Document: ref, Dest: *d})
		if err != nil {
			return -1
		}
		return r.Index
	}
	actionPage := func(a *references.FPDF_ACTION) int {
		if a == nil {
			return -1
		}
		t, err := inst.FPDFAction_GetType(&requests.FPDFAction_GetType{Action: *a})
		if err != nil || t.Type != enums.FPDF_ACTION_ACTION_GOTO {
			return -1
		}
		d, err := inst.FPDFAction_GetDest(&requests.FPDFAction_GetDest{Document: ref, Action: *a})
		if err != nil {
			return -1
		}
		return destPage(d.Dest)
	}

	var walk func(parent *references.FPDF_BOOKMARK, depth int)
	walk = func(parent *references.FPDF_BOOKMARK, depth int) {
		if depth > 64 {
			return
		}
		fc, err := inst.FPDFBookmark_GetFirstChild(&requests.FPDFBookmark_GetFirstChild{Document: ref, Bookmark: parent})
		if err != nil {
			return
		}
		for b := fc.Bookmark; b != nil; {
			item := OutlineItem{Depth: depth, Page: -1}
			if t, err := inst.FPDFBookmark_GetTitle(&requests.FPDFBookmark_GetTitle{Bookmark: *b}); err == nil {
				item.Title = t.Title
			}
			if d, err := inst.FPDFBookmark_GetDest(&requests.FPDFBookmark_GetDest{Document: ref, Bookmark: *b}); err == nil && d.Dest != nil {
				item.Page = destPage(d.Dest)
			} else if a, err := inst.FPDFBookmark_GetAction(&requests.FPDFBookmark_GetAction{Bookmark: *b}); err == nil {
				item.Page = actionPage(a.Action)
			}
			nav.Outline = append(nav.Outline, item)
			walk(b, depth+1)
			ns, err := inst.FPDFBookmark_GetNextSibling(&requests.FPDFBookmark_GetNextSibling{Document: ref, Bookmark: *b})
			if err != nil {
				break
			}
			b = ns.Bookmark
		}
	}
	walk(nil, 0)

	if cnt, err := inst.FPDF_CountNamedDests(&requests.FPDF_CountNamedDests{Document: ref}); err == nil {
		for i := 0; i < int(cnt.Count); i++ {
			nd, err := inst.FPDF_GetNamedDest(&requests.FPDF_GetNamedDest{Document: ref, Index: i})
			if err != nil {
				continue
			}
			d := nd.Dest
			nav.NamedDests[nd.Name] = destPage(&d)
		}
	}

	for p := range doc.Pages {
		pos := 0
		for {
			le, err := inst.FPDFLink_Enumerate(&requests.FPDFLink_Enumerate{Page: requests.Page{ByIndex: &requests.PageByIndex{Document: ref, Index: p}}, StartPos: pos})
			if err != nil || le.Link == nil {
				break
			}
			to := -1
			if d, err := inst.FPDFLink_GetDest(&requests.FPDFLink_GetDest{Document: ref, Link: *le.Link}); err == nil && d.Dest != nil {
				to = destPage(d.Dest)
			} else if a, err := inst.FPDFLink_GetAction(&requests.FPDFLink_GetAction{Link: *le.Link}); err == nil {
				to = actionPage(a.Action)
			}
			nav.Links = append(nav.Links, LinkTarget{FromPage: p, ToPage: to})
			if le.NextStartPos == nil {
				break
			}
			pos = *le.NextStartPos
		}
	}
	return nav, nil
}
