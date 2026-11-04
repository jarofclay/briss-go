// Spike 6: propose-and-choose crop detection with Claude.
//
// For every cluster overlay (from spike/render -out ... -dump-samples 2) the
// harness builds candidate rectangles locally, draws them numbered on the
// overlay and up to two sample pages, and asks Claude to pick the candidate
// that best matches the briss cropping rules, with an optional adjustment.
// Modes: dry (write images and prompt only), mock (pick candidate 0 locally),
// live (call the API; needs ANTHROPIC_API_KEY).
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

type choice struct {
	Choice  int     `json:"choice"`
	Adjust  *adjust `json:"adjust"`
	Split   string  `json:"split"`
	Reasons string  `json:"reasons"`
}
type adjust struct {
	L, B, R, T int // thousandths of page size, fractions from each edge
}

func (a *adjust) UnmarshalJSON(b []byte) error {
	var m map[string]int
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	a.L, a.B, a.R, a.T = m["l"], m["b"], m["r"], m["t"]
	return nil
}

const systemPrompt = `You help a PDF cropping tool (Briss) choose crop rectangles. Users crop pages so text renders larger on e-ink readers and tablets.

You receive an overlay image of several pages of one cluster (pages with the same size and parity, blended so content that moves between pages looks grey and content identical on every page, such as running headers and rules, looks faint) plus one or two individual pages. Numbered coloured rectangles are candidate crops.

Rules, in priority order:
1. Never clip body text, figures, tables or captions. A rectangle that cuts into content is wrong.
2. Running headers, footers and page numbers must be OUTSIDE the rectangle unless told to keep them.
3. Margin notes (narrow text in the outer margin) count as content and must stay inside.
4. Among rectangles that satisfy 1 to 3, prefer the tightest one.

Answer with JSON only: the id of the best candidate; an optional adjustment with l, b, r, t given as thousandths of the page width or height measured from the left, bottom, right and top edges (replace the chosen rectangle entirely; use null when the candidate is right as drawn); split as "none", "columns" or "rows" if the page clearly contains two separate pages or columns that should become separate output pages; and a one-sentence reason.`

var schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"choice": map[string]any{"type": "integer", "description": "id of the chosen candidate rectangle"},
		"adjust": map[string]any{
			"anyOf": []any{
				map[string]any{"type": "null"},
				map[string]any{
					"type": "object",
					"properties": map[string]any{
						"l": map[string]any{"type": "integer", "minimum": 0, "maximum": 1000},
						"b": map[string]any{"type": "integer", "minimum": 0, "maximum": 1000},
						"r": map[string]any{"type": "integer", "minimum": 0, "maximum": 1000},
						"t": map[string]any{"type": "integer", "minimum": 0, "maximum": 1000},
					},
					"required":             []string{"l", "b", "r", "t"},
					"additionalProperties": false,
				},
			},
		},
		"split":   map[string]any{"type": "string", "enum": []string{"none", "columns", "rows"}},
		"reasons": map[string]any{"type": "string"},
	},
	"required":             []string{"choice", "adjust", "split", "reasons"},
	"additionalProperties": false,
}

type clusterCase struct {
	Name     string
	Overlay  *image.Gray
	Samples  []*image.Gray
	Truth    *Ratios // nil when no label
	Cands    []Candidate
	Pdf      string
	ClusterI int
}

func main() {
	overlays := flag.String("overlays", "spike/testdata/overlays", "directory written by spike/render -out")
	labelsPath := flag.String("labels", "spike/testdata/labels.json", "corpus labels")
	mode := flag.String("mode", "dry", "dry | mock | live")
	model := flag.String("model", "claude-opus-5-5", "model id")
	outDir := flag.String("out", "spike/testdata/claude-run", "output directory")
	limit := flag.Int("limit", 0, "max clusters (0 = all)")
	flag.Parse()
	must(os.MkdirAll(*outDir, 0o755))

	cases := loadCases(*overlays, *labelsPath)
	if *limit > 0 && len(cases) > *limit {
		cases = cases[:*limit]
	}
	var client anthropic.Client
	if *mode == "live" {
		if os.Getenv("ANTHROPIC_API_KEY") == "" {
			fmt.Fprintln(os.Stderr, "live mode needs ANTHROPIC_API_KEY")
			os.Exit(2)
		}
		client = anthropic.NewClient(option.WithMaxRetries(2))
	}

	type row struct {
		name                 string
		nCands               int
		bestLocalIoU         float64
		chosen               int
		chosenIoU            float64
		clip                 bool
		latency              time.Duration
		inTok, outTok, cache int64
		reasons              string
	}
	var rows []row
	for _, cs := range cases {
		cs.Cands = buildCandidates(cs.Overlay)
		annotated := drawCandidates(cs.Overlay, cs.Cands)
		base := filepath.Join(*outDir, cs.Name)
		writePNG(base+"_overlay_candidates.png", annotated)
		var samplePNGs [][]byte
		for i, s := range cs.Samples {
			img := drawCandidates(s, cs.Cands)
			p := fmt.Sprintf("%s_sample%d_candidates.png", base, i)
			writePNG(p, img)
			samplePNGs = append(samplePNGs, encodePNG(img))
		}
		candJSON, _ := json.MarshalIndent(cs.Cands, "", " ")
		must(os.WriteFile(base+"_candidates.json", candJSON, 0o644))

		r := row{name: cs.Name, nCands: len(cs.Cands), chosen: -1}
		if cs.Truth != nil {
			for _, c := range cs.Cands {
				r.bestLocalIoU = max(r.bestLocalIoU, iou(c.Rect, *cs.Truth))
			}
		}
		userText := fmt.Sprintf("Cluster %s: %d candidate rectangles, ids 0 to %d (nested outlines are inset by 3 px per id so they stay visible). Candidate labels: %s. The first image is the overlay, the following images are individual pages from the cluster. Headers and footers must be excluded. Choose the best candidate.",
			cs.Name, len(cs.Cands), len(cs.Cands)-1, labelsOf(cs.Cands))
		must(os.WriteFile(base+"_prompt.txt", []byte(systemPrompt+"\n\n---\n"+userText), 0o644))

		var ch choice
		switch *mode {
		case "dry":
			rows = append(rows, r)
			continue
		case "mock":
			ch = choice{Choice: 0, Split: "none", Reasons: "mock"}
		case "live":
			blocks := []anthropic.ContentBlockParamUnion{anthropic.NewImageBlockBase64("image/png", base64.StdEncoding.EncodeToString(encodePNG(annotated)))}
			for _, p := range samplePNGs {
				blocks = append(blocks, anthropic.NewImageBlockBase64("image/png", base64.StdEncoding.EncodeToString(p)))
			}
			blocks = append(blocks, anthropic.NewTextBlock(userText))
			t0 := time.Now()
			resp, err := client.Messages.New(context.Background(), anthropic.MessageNewParams{
				Model:     anthropic.Model(*model),
				MaxTokens: 1024,
				System: []anthropic.TextBlockParam{{
					Text:         systemPrompt,
					CacheControl: anthropic.NewCacheControlEphemeralParam(),
				}},
				OutputConfig: anthropic.OutputConfigParam{
					Effort: anthropic.OutputConfigEffortLow,
					Format: anthropic.JSONOutputFormatParam{Schema: schema},
				},
				Messages: []anthropic.MessageParam{anthropic.NewUserMessage(blocks...)},
			})
			r.latency = time.Since(t0)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: API error: %v\n", cs.Name, err)
				rows = append(rows, r)
				continue
			}
			r.inTok, r.outTok, r.cache = resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.CacheReadInputTokens
			if resp.StopReason == anthropic.StopReasonRefusal {
				fmt.Fprintf(os.Stderr, "%s: refusal (%s)\n", cs.Name, resp.StopDetails.Category)
				rows = append(rows, r)
				continue
			}
			var text strings.Builder
			for _, b := range resp.Content {
				if tb, ok := b.AsAny().(anthropic.TextBlock); ok {
					text.WriteString(tb.Text)
				}
			}
			must(os.WriteFile(base+"_response.json", []byte(text.String()), 0o644))
			if err := json.Unmarshal([]byte(text.String()), &ch); err != nil {
				fmt.Fprintf(os.Stderr, "%s: bad JSON: %v\n", cs.Name, err)
				rows = append(rows, r)
				continue
			}
		}
		final := Ratios{}
		if ch.Choice >= 0 && ch.Choice < len(cs.Cands) {
			final = cs.Cands[ch.Choice].Rect
			r.chosen = ch.Choice
		}
		if ch.Adjust != nil {
			final = Ratios{L: float64(ch.Adjust.L) / 1000, B: float64(ch.Adjust.B) / 1000, R: float64(ch.Adjust.R) / 1000, T: float64(ch.Adjust.T) / 1000}
		}
		r.reasons = ch.Reasons
		if cs.Truth != nil {
			r.chosenIoU = iou(final, *cs.Truth)
			r.clip = clipped(final, *cs.Truth, 0.01)
		}
		rows = append(rows, r)
	}

	// report
	fmt.Printf("mode=%s model=%s clusters=%d\n", *mode, *model, len(rows))
	fmt.Printf("%-34s %5s %9s %6s %9s %5s %8s %6s %6s %6s\n", "cluster", "cands", "bestLocal", "chosen", "chosenIoU", "clip", "latency", "in", "out", "cached")
	var sumIoU float64
	var n, clips int
	var totalLat time.Duration
	var lats []time.Duration
	var inTok, outTok int64
	for _, r := range rows {
		fmt.Printf("%-34s %5d %9.3f %6d %9.3f %5v %8s %6d %6d %6d  %s\n", r.name, r.nCands, r.bestLocalIoU, r.chosen, r.chosenIoU, r.clip, r.latency.Round(time.Millisecond), r.inTok, r.outTok, r.cache, r.reasons)
		if r.chosen >= 0 {
			sumIoU += r.chosenIoU
			n++
			if r.clip {
				clips++
			}
			totalLat += r.latency
			lats = append(lats, r.latency)
			inTok += r.inTok
			outTok += r.outTok
		}
	}
	if n > 0 {
		sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
		cost := float64(inTok)*4/1e6 + float64(outTok)*20/1e6 // Opus 5.5 list prices
		fmt.Printf("mean IoU=%.3f clipped=%d/%d latency p50=%s  tokens in=%d out=%d  est. cost at Opus 5.5 prices=$%.4f (%.4f/cluster)\n",
			sumIoU/float64(n), clips, n, lats[len(lats)/2].Round(time.Millisecond), inTok, outTok, cost, cost/float64(n))
	}
}

func labelsOf(c []Candidate) string {
	var s []string
	for _, x := range c {
		s = append(s, fmt.Sprintf("%d=%s", x.ID, x.Label))
	}
	return strings.Join(s, "; ")
}

// loadCases pairs overlay PNGs with sample pages and ground truth. Truth per
// cluster = median of the labelled body boxes of the cluster's pages (same rule
// as spike/cropwrite); scan corpora use their own label sections.
func loadCases(dir, labelsPath string) []*clusterCase {
	raw, err := os.ReadFile(labelsPath)
	must(err)
	var labels map[string]map[string]json.RawMessage
	must(json.Unmarshal(raw, &labels))
	entries, err := os.ReadDir(dir)
	must(err)
	var cases []*clusterCase
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".png") || strings.Contains(name, "_page") {
			continue
		}
		// <pdf>_c<ci>_<parity>_<w>x<h>_p<first>.png  (pdf may itself contain '_')
		parts := strings.Split(strings.TrimSuffix(name, ".png"), "_")
		if len(parts) < 5 {
			continue
		}
		k := len(parts) - 4
		pdf := strings.Join(parts[:k], "_")
		var ci, w, h, first int
		fmt.Sscanf(parts[k], "c%d", &ci)
		fmt.Sscanf(parts[k+2], "%dx%d", &w, &h)
		fmt.Sscanf(parts[k+3], "p%d", &first)
		even := parts[k+1] == "even"
		cs := &clusterCase{Name: strings.TrimSuffix(name, ".png"), Pdf: pdf, ClusterI: ci}
		cs.Overlay = readGray(filepath.Join(dir, name))
		samples, _ := filepath.Glob(filepath.Join(dir, fmt.Sprintf("%s_c%02d_page*.png", pdf, ci)))
		sort.Strings(samples)
		for _, s := range samples {
			cs.Samples = append(cs.Samples, readGray(s))
		}
		cs.Truth = truthFor(labels, pdf, even, w, h)
		cases = append(cases, cs)
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].Name < cases[j].Name })
	return cases
}

func truthFor(labels map[string]map[string]json.RawMessage, pdf string, even bool, w, h int) *Ratios {
	sect, ok := labels[pdf]
	if !ok {
		return nil
	}
	var rs []Ratios
	for ps, raw := range sect {
		var p int
		fmt.Sscanf(ps, "%d", &p)
		if (p%2 == 0) != even {
			continue
		}
		var lb struct {
			Size   []float64         `json:"size"`
			Body   []float64         `json:"body"`
			Boxes  [][]float64       `json:"boxes"`
			Ratios []json.RawMessage `json:"ratios"`
		}
		if json.Unmarshal(raw, &lb) != nil || len(lb.Size) != 2 || int(lb.Size[0]) != w || int(lb.Size[1]) != h {
			continue
		}
		if len(lb.Body) == 4 {
			if lb.Body[2]-lb.Body[0] < 50 {
				continue
			}
			rs = append(rs, Ratios{lb.Body[0] / lb.Size[0], lb.Body[1] / lb.Size[1], 1 - lb.Body[2]/lb.Size[0], 1 - lb.Body[3]/lb.Size[1]})
		} else if len(lb.Boxes) > 0 { // two-up: union of both boxes
			x0, y0, x1, y1 := lb.Boxes[0][0], lb.Boxes[0][1], lb.Boxes[0][2], lb.Boxes[0][3]
			for _, b := range lb.Boxes[1:] {
				x0, y0, x1, y1 = min(x0, b[0]), min(y0, b[1]), max(x1, b[2]), max(y1, b[3])
			}
			rs = append(rs, Ratios{x0 / lb.Size[0], y0 / lb.Size[1], 1 - x1/lb.Size[0], 1 - y1/lb.Size[1]})
		}
	}
	if len(rs) == 0 {
		return nil
	}
	med := func(f func(Ratios) float64) float64 {
		v := make([]float64, len(rs))
		for i, r := range rs {
			v[i] = f(r)
		}
		sort.Float64s(v)
		return v[len(v)/2]
	}
	return &Ratios{med(func(r Ratios) float64 { return r.L }), med(func(r Ratios) float64 { return r.B }), med(func(r Ratios) float64 { return r.R }), med(func(r Ratios) float64 { return r.T })}
}

func readGray(p string) *image.Gray {
	f, err := os.Open(p)
	must(err)
	defer f.Close()
	img, err := png.Decode(f)
	must(err)
	if g, ok := img.(*image.Gray); ok {
		return g
	}
	b := img.Bounds()
	g := image.NewGray(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, gg, bb, _ := img.At(x, y).RGBA()
			g.Pix[(y-b.Min.Y)*g.Stride+(x-b.Min.X)] = uint8((r*299 + gg*587 + bb*114) / 1000 >> 8)
		}
	}
	return g
}

func writePNG(p string, img image.Image) {
	f, err := os.Create(p)
	must(err)
	defer f.Close()
	must(png.Encode(f, img))
}

func encodePNG(img image.Image) []byte {
	var sb strings.Builder
	must(png.Encode(&sb, img))
	return []byte(sb.String())
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
