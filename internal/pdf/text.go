package pdf

import (
	"context"
	"fmt"

	"github.com/klippa-app/go-pdfium/requests"
)

// TextRun is a piece of text on a page with its box in PDF user space
// (points, origin at the bottom left of the unrotated page, so Top > Bottom).
type TextRun struct {
	Text                     string
	Left, Top, Right, Bottom float64
}

// PageText returns the text runs of a 0-based page (for tests and for
// telling pages that already have a text layer).
func (e *Engine) PageText(ctx context.Context, doc *Document, page int) (string, []TextRun, error) {
	inst, err := e.instance(ctx)
	if err != nil {
		return "", nil, err
	}
	defer inst.Close()
	ref, done, err := openDoc(inst, doc.Path, doc.password)
	if err != nil {
		return "", nil, err
	}
	defer done()
	if page < 0 || page >= len(doc.Pages) {
		return "", nil, fmt.Errorf("pdf: page %d out of range", page+1)
	}
	p := requests.Page{ByIndex: &requests.PageByIndex{Document: ref, Index: page}}
	plain, err := inst.GetPageText(&requests.GetPageText{Page: p})
	if err != nil {
		return "", nil, fmt.Errorf("pdf: text of page %d: %w", page+1, err)
	}
	st, err := inst.GetPageTextStructured(&requests.GetPageTextStructured{Page: p, Mode: requests.GetPageTextStructuredModeRects})
	if err != nil {
		return "", nil, fmt.Errorf("pdf: text of page %d: %w", page+1, err)
	}
	var runs []TextRun
	for _, r := range st.Rects {
		runs = append(runs, TextRun{Text: r.Text, Left: r.PointPosition.Left, Top: r.PointPosition.Top, Right: r.PointPosition.Right, Bottom: r.PointPosition.Bottom})
	}
	return plain.Text, runs, nil
}
