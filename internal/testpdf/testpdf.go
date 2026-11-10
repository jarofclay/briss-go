// Package testpdf writes small, valid PDF files for tests: pages with media and
// crop boxes, rotation, filled rectangles and text, an outline, named
// destinations (both the /Names tree and the old /Dests dictionary), and link
// annotations with /Dest names and /GoTo actions. Output is deterministic.
package testpdf

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

// Box is a rectangle in PDF user space.
type Box struct{ LLX, LLY, URX, URY float64 }

func (b Box) arr() string {
	return fmt.Sprintf("[%s %s %s %s]", f(b.LLX), f(b.LLY), f(b.URX), f(b.URY))
}

// Page describes one page.
type Page struct {
	Media  Box
	Crop   *Box
	Rotate int
	Fill   []Box  // black filled rectangles
	Text   string // drawn in Helvetica 12 at (Media.LLX+20, Media.URY-30), unless empty
	Lines  []Line // more text, anywhere
	Links  []Link
}

// Line is one line of Helvetica text with its baseline starting at (X, Y).
type Line struct {
	X, Y, Size float64
	Text       string
}

// Paragraph lays out words as lines of at most width points starting at
// (x, y) and going down by 1.3 times size, until n lines are filled.
func Paragraph(x, y, width, size float64, n int, words []string) []Line {
	var out []Line
	cur, k := "", 0
	for len(out) < n {
		w := words[k%len(words)]
		k++
		next := w
		if cur != "" {
			next = cur + " " + w
		}
		if float64(len(next))*size*0.5 > width && cur != "" {
			out = append(out, Line{X: x, Y: y - float64(len(out))*1.3*size, Size: size, Text: cur})
			cur = w
			continue
		}
		cur = next
	}
	return out
}

// Link is a link annotation on a page. Exactly one of DestName or GoToPage is used:
// DestName points at a named destination, GoToPage (0-based) uses a /GoTo action.
type Link struct {
	Rect     Box
	DestName string
	GoToPage int
}

// Outline is a top-level bookmark with optional children.
type Outline struct {
	Title    string
	DestName string // named destination, or
	Page     int    // 0-based page with an explicit destination when DestName is empty
	Children []Outline
}

// Doc describes a whole document.
type Doc struct {
	Pages      []Page
	Outline    []Outline
	NamedDests map[string]int // name -> 0-based page, written to the /Names tree
	OldDests   map[string]int // name -> 0-based page, written to the catalog /Dests dictionary
	Title      string
}

type writer struct {
	objs []string // object bodies; object number = index + 1
}

func (w *writer) reserve() int {
	w.objs = append(w.objs, "")
	return len(w.objs)
}

func (w *writer) set(n int, body string) { w.objs[n-1] = body }

func ref(n int) string { return fmt.Sprintf("%d 0 R", n) }

func f(v float64) string {
	s := fmt.Sprintf("%.4f", v)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

func pdfString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`)
	return "(" + r.Replace(s) + ")"
}

// Build returns the PDF bytes for d.
func Build(d Doc) []byte {
	w := &writer{}
	catalog := w.reserve()
	pagesObj := w.reserve()
	font := w.reserve()
	w.set(font, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")

	pageObjs := make([]int, len(d.Pages))
	for i := range d.Pages {
		pageObjs[i] = w.reserve()
	}
	dest := func(page int) string { return fmt.Sprintf("[%s /Fit]", ref(pageObjs[page])) }

	for i, p := range d.Pages {
		var cs bytes.Buffer
		for _, b := range p.Fill {
			fmt.Fprintf(&cs, "0 g %s %s %s %s re f\n", f(b.LLX), f(b.LLY), f(b.URX-b.LLX), f(b.URY-b.LLY))
		}
		if p.Text != "" {
			fmt.Fprintf(&cs, "BT /F1 12 Tf %s %s Td %s Tj ET\n", f(p.Media.LLX+20), f(p.Media.URY-30), pdfString(p.Text))
		}
		for _, l := range p.Lines {
			fmt.Fprintf(&cs, "BT /F1 %s Tf %s %s Td %s Tj ET\n", f(l.Size), f(l.X), f(l.Y), pdfString(l.Text))
		}
		content := w.reserve()
		w.set(content, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", cs.Len(), cs.String()))
		var annots []string
		for _, l := range p.Links {
			a := w.reserve()
			target := ""
			if l.DestName != "" {
				target = "/Dest " + pdfString(l.DestName)
			} else {
				target = "/A << /S /GoTo /D " + dest(l.GoToPage) + " >>"
			}
			w.set(a, fmt.Sprintf("<< /Type /Annot /Subtype /Link /Rect %s /Border [0 0 0] %s >>", l.Rect.arr(), target))
			annots = append(annots, ref(a))
		}
		var b strings.Builder
		fmt.Fprintf(&b, "<< /Type /Page /Parent %s /MediaBox %s", ref(pagesObj), p.Media.arr())
		if p.Crop != nil {
			fmt.Fprintf(&b, " /CropBox %s", p.Crop.arr())
		}
		if p.Rotate != 0 {
			fmt.Fprintf(&b, " /Rotate %d", p.Rotate)
		}
		fmt.Fprintf(&b, " /Resources << /Font << /F1 %s >> >> /Contents %s", ref(font), ref(content))
		if len(annots) > 0 {
			fmt.Fprintf(&b, " /Annots [%s]", strings.Join(annots, " "))
		}
		b.WriteString(" >>")
		w.set(pageObjs[i], b.String())
	}
	kids := make([]string, len(pageObjs))
	for i, n := range pageObjs {
		kids[i] = ref(n)
	}
	w.set(pagesObj, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pageObjs)))

	var cat strings.Builder
	fmt.Fprintf(&cat, "<< /Type /Catalog /Pages %s", ref(pagesObj))
	if len(d.Outline) > 0 {
		root := w.reserve()
		first, last, count := writeOutlines(w, d.Outline, root, dest)
		w.set(root, fmt.Sprintf("<< /Type /Outlines /First %s /Last %s /Count %d >>", ref(first), ref(last), count))
		fmt.Fprintf(&cat, " /Outlines %s /PageMode /UseOutlines", ref(root))
	}
	if len(d.NamedDests) > 0 {
		names := make([]string, 0, len(d.NamedDests))
		for n := range d.NamedDests {
			names = append(names, n)
		}
		sort.Strings(names)
		var arr []string
		for _, n := range names {
			arr = append(arr, pdfString(n)+" "+dest(d.NamedDests[n]))
		}
		tree := w.reserve()
		w.set(tree, fmt.Sprintf("<< /Names [%s] >>", strings.Join(arr, " ")))
		fmt.Fprintf(&cat, " /Names << /Dests %s >>", ref(tree))
	}
	if len(d.OldDests) > 0 {
		names := make([]string, 0, len(d.OldDests))
		for n := range d.OldDests {
			names = append(names, n)
		}
		sort.Strings(names)
		var parts []string
		for _, n := range names {
			parts = append(parts, "/"+n+" "+dest(d.OldDests[n]))
		}
		dd := w.reserve()
		w.set(dd, "<< "+strings.Join(parts, " ")+" >>")
		fmt.Fprintf(&cat, " /Dests %s", ref(dd))
	}
	cat.WriteString(" >>")
	w.set(catalog, cat.String())

	info := w.reserve()
	title := d.Title
	if title == "" {
		title = "briss-go test fixture"
	}
	w.set(info, fmt.Sprintf("<< /Title %s /Producer (briss-go testpdf) >>", pdfString(title)))

	var out bytes.Buffer
	out.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, len(w.objs))
	for i, body := range w.objs {
		offsets[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(w.objs)+1)
	for _, o := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root %s /Info %s >>\nstartxref\n%d\n%%%%EOF\n", len(w.objs)+1, ref(catalog), ref(info), xref)
	return out.Bytes()
}

// writeOutlines writes a sibling list under parent and returns first, last and
// the number of visible descendants (all items are open).
func writeOutlines(w *writer, items []Outline, parent int, dest func(int) string) (first, last, count int) {
	nums := make([]int, len(items))
	for i := range items {
		nums[i] = w.reserve()
	}
	for i, it := range items {
		var b strings.Builder
		fmt.Fprintf(&b, "<< /Title %s /Parent %s", pdfString(it.Title), ref(parent))
		if i > 0 {
			fmt.Fprintf(&b, " /Prev %s", ref(nums[i-1]))
		}
		if i < len(items)-1 {
			fmt.Fprintf(&b, " /Next %s", ref(nums[i+1]))
		}
		if it.DestName != "" {
			fmt.Fprintf(&b, " /Dest %s", pdfString(it.DestName))
		} else {
			fmt.Fprintf(&b, " /Dest %s", dest(it.Page))
		}
		count++
		if len(it.Children) > 0 {
			cf, cl, cc := writeOutlines(w, it.Children, nums[i], dest)
			fmt.Fprintf(&b, " /First %s /Last %s /Count %d", ref(cf), ref(cl), cc)
			count += cc
		}
		b.WriteString(" >>")
		w.set(nums[i], b.String())
	}
	return nums[0], nums[len(nums)-1], count
}

// Navigation is the standard fixture used by the pdf and crop tests: five pages
// (one rotated, one with a crop box smaller than its media box, one landscape),
// a two-level outline using names and explicit destinations, named destinations
// in both styles, and links of both kinds.
func Navigation() Doc {
	letter := Box{0, 0, 612, 792}
	crop := Box{36, 36, 576, 756}
	return Doc{
		Title: "navigation fixture",
		Pages: []Page{
			{Media: letter, Text: "page 1", Fill: []Box{{72, 72, 540, 720}},
				Links: []Link{{Rect: Box{72, 700, 200, 720}, DestName: "chapter2"}, {Rect: Box{72, 650, 200, 670}, GoToPage: 4}}},
			{Media: letter, Text: "page 2", Fill: []Box{{72, 72, 300, 720}, {312, 72, 540, 720}}},
			// /Rotate 90: displayed 792 wide x 612 tall. A mark at the unrotated
			// bottom-left shows at the displayed top-left.
			{Media: letter, Rotate: 90, Text: "page 3", Fill: []Box{{0, 0, 100, 100}, {200, 200, 400, 600}}},
			{Media: letter, Crop: &crop, Text: "page 4", Fill: []Box{{100, 100, 500, 700}}},
			{Media: Box{0, 0, 792, 612}, Text: "page 5", Fill: []Box{{72, 72, 720, 540}},
				Links: []Link{{Rect: Box{72, 560, 200, 580}, GoToPage: 1}}},
		},
		Outline: []Outline{
			{Title: "Start", Page: 0},
			{Title: "Chapter 2", DestName: "chapter2", Children: []Outline{
				{Title: "Rotated page", DestName: "rotated"},
				{Title: "Cropped page", Page: 3},
			}},
			{Title: "Landscape", DestName: "landscape"},
		},
		NamedDests: map[string]int{"chapter2": 1, "rotated": 2},
		OldDests:   map[string]int{"landscape": 4},
	}
}
