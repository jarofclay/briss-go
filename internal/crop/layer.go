package crop

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/jarofclay/briss-go/internal/geom"
	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"golang.org/x/text/encoding/charmap"
)

// Output is one page of the cropped document: a rectangle of a source page
// (display ratios, as everywhere in briss), optionally straightened and
// optionally carrying an invisible OCR text layer.
type Output struct {
	Rect  geom.Ratios
	Angle float64 // degrees counterclockwise, turned about the rectangle's centre
	Words []Word  // OCR words to lay over the page as invisible, selectable text
}

// Word is one recognised word: its text and its box on the source page as
// displayed, in ratios, before straightening (as the scan shows it).
type Word struct {
	Text string
	Box  geom.Ratios
}

// ocrFont names the font resource of the text layer.
const ocrFont = "BrissOCR"

// rotationCM is the content-stream operator that turns everything drawn
// after it by deg degrees counterclockwise about (cx, cy).
func rotationCM(deg, cx, cy float64) string {
	s, c := math.Sincos(deg * math.Pi / 180)
	e := cx - c*cx + s*cy
	f := cy - s*cx - c*cy
	return fmt.Sprintf("%.6f %.6f %.6f %.6f %.4f %.4f cm\n", c, s, -s, c, e, f)
}

var winAnsi = charmap.Windows1252.NewEncoder()

// encodeText converts text to WinAnsi bytes; characters outside it become '?'.
func encodeText(text string) []byte {
	var out []byte
	for _, r := range text {
		b, err := winAnsi.Bytes([]byte(string(r)))
		if err != nil || len(b) != 1 {
			out = append(out, '?')
			continue
		}
		out = append(out, b[0])
	}
	return out
}

// pdfString writes a literal string with the characters that need escaping
// escaped and bytes outside printable ASCII in octal.
func pdfString(b []byte) string {
	var sb strings.Builder
	sb.WriteByte('(')
	for _, c := range b {
		switch {
		case c == '(' || c == ')' || c == '\\':
			sb.WriteByte('\\')
			sb.WriteByte(c)
		case c < 32 || c > 126:
			fmt.Fprintf(&sb, "\\%03o", c)
		default:
			sb.WriteByte(c)
		}
	}
	sb.WriteByte(')')
	return sb.String()
}

// helveticaWidth is the advance of b in Helvetica at size 1 (text space).
func helveticaWidth(b []byte) float64 {
	w := 0
	for _, c := range b {
		cw, err := font.CharWidth(context.Background(), "Helvetica", rune(c))
		if err != nil || cw == 0 {
			cw = 556
		}
		w += cw
	}
	return float64(w) / 1000
}

// textLayer is the content of an invisible text layer (render mode 3) with
// every word stretched to its box. Boxes are in display ratios of a page
// whose displayed area is ref (the smaller of media and crop box) shown with
// rotation rot; the text is laid out so it reads left to right on screen.
func textLayer(words []Word, ref geom.Rect, rot int) []byte {
	if len(words) == 0 {
		return nil
	}
	var buf bytes.Buffer
	buf.WriteString("BT\n3 Tr\n")
	psi := float64(rot) * math.Pi / 180
	s, c := math.Sincos(psi)
	for _, w := range words {
		b := encodeText(w.Text)
		if len(bytes.TrimSpace(b)) == 0 {
			continue
		}
		u := geom.ScaledBox(ref, nil, w.Box, rot) // user-space box of the word
		var size, width, ox, oy float64
		switch rot {
		case 90:
			size, width = u.Width(), u.Height()
			ox, oy = u.URX-0.2*size, u.LLY
		case 180:
			size, width = u.Height(), u.Width()
			ox, oy = u.URX, u.URY-0.2*size
		case 270:
			size, width = u.Width(), u.Height()
			ox, oy = u.LLX+0.2*size, u.URY
		default:
			size, width = u.Height(), u.Width()
			ox, oy = u.LLX, u.LLY+0.2*size
		}
		if size <= 0 || width <= 0 {
			continue
		}
		adv := helveticaWidth(b) * size
		tz := 100.0
		if adv > 0 {
			tz = math.Max(10, math.Min(1000, 100*width/adv))
		}
		fmt.Fprintf(&buf, "/%s %.3f Tf %.2f Tz %.6f %.6f %.6f %.6f %.4f %.4f Tm %s Tj\n", ocrFont, size, tz, c, s, -s, c, ox, oy, pdfString(b))
	}
	buf.WriteString("ET\n")
	return buf.Bytes()
}

// newStream adds a Flate-compressed content stream and returns its reference.
func newStream(pctx *model.Context, content []byte) (*types.IndirectRef, error) {
	sd, err := pctx.NewStreamDictForBuf(content)
	if err != nil {
		return nil, err
	}
	if err := sd.Encode(); err != nil {
		return nil, err
	}
	return pctx.IndRefForNewObject(*sd)
}

// contentRefs returns a page's /Contents as a list of objects.
func contentRefs(d types.Dict) types.Array {
	switch c := d["Contents"].(type) {
	case types.Array:
		return append(types.Array(nil), c...)
	case nil:
		return nil
	default:
		return types.Array{c}
	}
}

// effectiveResources returns the page's resource dictionary, inherited from
// the page tree when the page has none of its own, as a fresh shallow copy.
func effectiveResources(pctx *model.Context, d types.Dict) (types.Dict, error) {
	for node, depth := d, 0; node != nil && depth < 64; depth++ {
		if r, ok := node["Resources"]; ok {
			res, err := pctx.DereferenceDict(r)
			if err != nil {
				return nil, err
			}
			out := types.Dict{}
			for k, v := range res {
				out[k] = v
			}
			return out, nil
		}
		pr, ok := node["Parent"].(types.IndirectRef)
		if !ok {
			break
		}
		next, err := pctx.DereferenceDict(pr)
		if err != nil {
			return nil, err
		}
		node = next
	}
	return types.Dict{}, nil
}

// decorate turns and annotates one output page: its content becomes
// "q [turn] q <original> Q [text layer] Q", so the original drawing cannot
// leak state into what follows and the text layer turns with the scan.
func decorate(pctx *model.Context, target types.Dict, box geom.Rect, ref geom.Rect, rot int, out Output, fontRef **types.IndirectRef) error {
	turn := math.Abs(out.Angle) >= 0.01
	layer := textLayer(out.Words, ref, rot)
	if !turn && layer == nil {
		return nil
	}
	pre := "q\n"
	if turn {
		pre += rotationCM(out.Angle, (box.LLX+box.URX)/2, (box.LLY+box.URY)/2)
	}
	pre += "q\n"
	post := append([]byte("\nQ\n"), layer...)
	post = append(post, "Q\n"...)
	preRef, err := newStream(pctx, []byte(pre))
	if err != nil {
		return err
	}
	postRef, err := newStream(pctx, post)
	if err != nil {
		return err
	}
	contents := append(types.Array{*preRef}, contentRefs(target)...)
	target["Contents"] = append(contents, *postRef)
	if layer == nil {
		return nil
	}
	if *fontRef == nil {
		f := types.Dict{
			"Type":     types.Name("Font"),
			"Subtype":  types.Name("Type1"),
			"BaseFont": types.Name("Helvetica"),
			"Encoding": types.Name("WinAnsiEncoding"),
		}
		r, err := pctx.IndRefForNewObject(f)
		if err != nil {
			return err
		}
		*fontRef = r
	}
	res, err := effectiveResources(pctx, target)
	if err != nil {
		return err
	}
	fonts := types.Dict{}
	if f, ok := res["Font"]; ok {
		fd, err := pctx.DereferenceDict(f)
		if err != nil {
			return err
		}
		for k, v := range fd {
			fonts[k] = v
		}
	}
	fonts[ocrFont] = **fontRef
	res["Font"] = fonts
	target["Resources"] = res
	return nil
}
