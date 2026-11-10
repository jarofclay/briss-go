package pipeline

// Golden tests compare the Go pipeline with the Java Briss on the PDFs in
// testdata/pdfs. The reference data in testdata/golden is produced by
// legacy-java/tools/golden/run.sh. Exact checks cover everything that does not
// depend on rasterisation (geometry, clustering, sampling, overlay size, the
// legacy algorithm on Java's own overlay); measured tolerances cover the parts
// where PDFium renders differently from PDFBox.

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jarofclay/briss-go/internal/crop"
	"github.com/jarofclay/briss-go/internal/detect"
	"github.com/jarofclay/briss-go/internal/detect/legacy"
	"github.com/jarofclay/briss-go/internal/geom"
	"github.com/jarofclay/briss-go/internal/overlay"
	"github.com/jarofclay/briss-go/internal/pdf"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Tolerances, set from the differences measured on 2026-11-05 (the test log
// prints the measured values every run).
var (
	// Mean absolute difference of overlay column/row means, in gray levels.
	// Measured 0.66 to 5.23: PDFium renders straight to the overlay size with
	// anti-aliasing, Java renders 300 DPI and downscales nearest-neighbour.
	profileTol = 6.0
	// Legacy auto-crop ratios, Go (PDFium render) vs Java (PDFBox render),
	// per side. Measured 0 to 0.0033 on all clusters but one; the plan's 0.5%.
	ratioTol = 0.005
	// The legacy "26 of the next 30 samples" rule flips on a sub-gray-level
	// profile difference at the right edge of the slide deck's even pages, so
	// Go stops 0.026 of the width earlier than Java there. The port itself is
	// exact (TestGoldenLegacyExact); this is the algorithm's sensitivity.
	unstableRatioTol = map[string]float64{"CREATIVE_COMMONS c1": 0.03}
	// page boxes in points
	strictBoxTol = 0.01
)

type gRatio struct {
	L float64 `json:"l"`
	B float64 `json:"b"`
	R float64 `json:"r"`
	T float64 `json:"t"`
}

func (g gRatio) ratios() geom.Ratios { return geom.Ratios{L: g.L, B: g.B, R: g.R, T: g.T} }

type gCluster struct {
	Even    bool  `json:"even"`
	Pages   []int `json:"pages"`
	Sample  []int `json:"sample"`
	Overlay struct {
		W       int     `json:"w"`
		H       int     `json:"h"`
		ColSums []int64 `json:"col_sums"`
		RowSums []int64 `json:"row_sums"`
	} `json:"overlay"`
	Autocrop gRatio   `json:"autocrop"`
	SplitCol []gRatio `json:"split_col"`
	SplitRow []gRatio `json:"split_row"`
}

type gBox struct {
	Media    [4]float64 `json:"media"`
	Crop     [4]float64 `json:"crop"`
	Rotation int        `json:"rotation"`
}

type gFile struct {
	File     string                     `json:"file"`
	Pages    int                        `json:"pages"`
	Clusters []gCluster                 `json:"clusters"`
	Output   map[string]json.RawMessage `json:"output"`
}

var goldenFiles = []struct{ name, password string }{
	{"example", ""}, {"example-protected", "secret"}, {"CREATIVE_COMMONS", ""}, {"book300", ""},
}

func loadGolden(t *testing.T, name string) gFile {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("../../testdata/golden", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var g gFile
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	return g
}

func (g gFile) boxes(t *testing.T, variant string) []gBox {
	t.Helper()
	var out []gBox
	if err := json.Unmarshal(g.Output[variant], &out); err != nil {
		t.Fatalf("%s %s output: %s", g.File, variant, g.Output[variant])
	}
	return out
}

func eq32(a, b float64) bool { return float32(a) == float32(b) }

func TestGoldenGeometryExact(t *testing.T) {
	b, err := os.ReadFile("../../testdata/golden/geom.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Media    [4]float64  `json:"media"`
		Crop     *[4]float64 `json:"crop"`
		Ratios   gRatio      `json:"ratios"`
		Rotation int         `json:"rotation"`
		Result   [4]float64  `json:"result"`
	}
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	for i, c := range cases {
		media := geom.Rect{LLX: c.Media[0], LLY: c.Media[1], URX: c.Media[2], URY: c.Media[3]}
		var cb *geom.Rect
		if c.Crop != nil {
			cb = &geom.Rect{LLX: c.Crop[0], LLY: c.Crop[1], URX: c.Crop[2], URY: c.Crop[3]}
		}
		got := geom.ScaledBox(media, cb, c.Ratios.ratios(), c.Rotation)
		g := [4]float64{got.LLX, got.LLY, got.URX, got.URY}
		for k := range g {
			if math.Abs(g[k]-c.Result[k]) > strictBoxTol {
				t.Errorf("case %d (rotation %d): got %v want %v", i, c.Rotation, g, c.Result)
				break
			}
		}
	}
	t.Logf("%d geometry cases checked against RectangleHandler", len(cases))
}

// The legacy port must reproduce Java's ratios exactly (float32) when fed
// Java's own overlay profile.
func TestGoldenLegacyExact(t *testing.T) {
	for _, gf := range goldenFiles {
		g := loadGolden(t, gf.name)
		for ci, c := range g.Clusters {
			p := legacy.ProfileFromSums(c.Overlay.ColSums, c.Overlay.RowSums)
			auto := legacy.AutoCropProfile(p)
			check := func(what string, got geom.Ratios, want gRatio) {
				if !eq32(got.L, want.L) || !eq32(got.B, want.B) || !eq32(got.R, want.R) || !eq32(got.T, want.T) {
					t.Errorf("%s cluster %d %s: got %+v want %+v", gf.name, ci, what, got, want)
				}
			}
			check("autocrop", auto, c.Autocrop)
			jr := c.Autocrop.ratios() // splits start from Java's float32 values
			sc := legacy.SplitColumnProfile(p, jr)
			sr := legacy.SplitRowProfile(p, jr)
			for k := 0; k < 2; k++ {
				check("split_col", sc[k], c.SplitCol[k])
				check("split_row", sr[k], c.SplitRow[k])
			}
		}
	}
}

var (
	gEngOnce sync.Once
	gEng     *pdf.Engine
	gEngErr  error
)

func goldenEngine(t *testing.T) *pdf.Engine {
	gEngOnce.Do(func() { gEng, gEngErr = pdf.NewEngine(pdf.EngineOptions{}) })
	if gEngErr != nil {
		t.Fatal(gEngErr)
	}
	return gEng
}

func meanAbsDiff(a []float64, sums []int64, div int) float64 {
	var s float64
	for i := range a {
		s += math.Abs(a[i] - float64(sums[i])/float64(div))
	}
	return s / float64(len(a))
}

func TestGoldenPipeline(t *testing.T) {
	eng := goldenEngine(t)
	ctx := context.Background()
	var report []string
	for _, gf := range goldenFiles {
		g := loadGolden(t, gf.name)
		res, err := Analyze(ctx, eng, filepath.Join("../../testdata/pdfs", gf.name+".pdf"), Options{Password: gf.password})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Doc.Pages) != g.Pages {
			t.Fatalf("%s: %d pages, Java %d", gf.name, len(res.Doc.Pages), g.Pages)
		}
		if len(res.Clusters) != len(g.Clusters) {
			t.Fatalf("%s: %d clusters, Java %d", gf.name, len(res.Clusters), len(g.Clusters))
		}
		if err := res.Detect(ctx, legacy.Detector{}, detect.Options{}); err != nil {
			t.Fatal(err)
		}
		for ci, jc := range g.Clusters {
			gc := res.Clusters[ci]
			if gc.Cluster.Even != jc.Even || !reflect.DeepEqual(gc.Cluster.Pages, jc.Pages) {
				t.Errorf("%s cluster %d: pages %v (even %v), Java %v (even %v)", gf.name, ci, gc.Cluster.Pages, gc.Cluster.Even, jc.Pages, jc.Even)
			}
			if !reflect.DeepEqual(gc.Cluster.Sample, jc.Sample) {
				t.Errorf("%s cluster %d: sample %v, Java %v", gf.name, ci, gc.Cluster.Sample, jc.Sample)
			}
			b := gc.Overlay.Bounds()
			if b.Dx() != jc.Overlay.W || b.Dy() != jc.Overlay.H {
				t.Errorf("%s cluster %d: overlay %dx%d, Java %dx%d", gf.name, ci, b.Dx(), b.Dy(), jc.Overlay.W, jc.Overlay.H)
				continue
			}
			cols, rows := overlay.Profiles(gc.Overlay)
			dc := meanAbsDiff(cols, jc.Overlay.ColSums, jc.Overlay.H)
			dr := meanAbsDiff(rows, jc.Overlay.RowSums, jc.Overlay.W)
			if dc > profileTol || dr > profileTol {
				t.Errorf("%s cluster %d: overlay profile differs by %.2f (cols) %.2f (rows) gray levels, tolerance %.1f", gf.name, ci, dc, dr, profileTol)
			}
			got := gc.Candidate.Rects[0]
			want := jc.Autocrop.ratios()
			if !want.Valid() {
				// Java returns a degenerate rectangle on some sparse overlays;
				// the Go detector keeps the whole page instead.
				if got != (geom.Ratios{}) {
					t.Errorf("%s cluster %d: Java ratios degenerate %+v, Go should keep the page, got %+v", gf.name, ci, want, got)
				}
				report = append(report, gf.name+" c"+strconv.Itoa(ci)+": Java degenerate, Go whole page")
				continue
			}
			d := math.Max(math.Max(math.Abs(got.L-want.L), math.Abs(got.B-want.B)), math.Max(math.Abs(got.R-want.R), math.Abs(got.T-want.T)))
			key := gf.name + " c" + strconv.Itoa(ci)
			tol := ratioTol
			if u, ok := unstableRatioTol[key]; ok {
				tol = u
			}
			if d > tol {
				t.Errorf("%s: legacy ratios %+v, Java %+v (max diff %.4f > %.3f)", key, got, want, d, tol)
			}
			report = append(report, key+": profile diff cols "+ftoa(dc)+" rows "+ftoa(dr)+", ratio max diff "+ftoa(d))
		}
	}
	t.Log("\n" + strings.Join(report, "\n"))
}

// Java's output boxes must equal geom.ScaledBox applied to the source page
// boxes with Java's ratios: this checks the writer maths on real files,
// independent of rendering.
func TestGoldenOutputBoxesExact(t *testing.T) {
	for _, gf := range goldenFiles {
		g := loadGolden(t, gf.name)
		src := filepath.Join("../../testdata/pdfs", gf.name+".pdf")
		pages := sourceBoxes(t, src, gf.password)
		for _, variant := range []string{"plain", "split_col"} {
			out := g.boxes(t, variant)
			o := 0
			for p := 1; p <= g.Pages; p++ {
				var rects []geom.Ratios
				for _, c := range g.Clusters {
					for _, cp := range c.Pages {
						if cp == p {
							if variant == "plain" {
								rects = []geom.Ratios{c.Autocrop.ratios()}
							} else {
								rects = []geom.Ratios{c.SplitCol[0].ratios(), c.SplitCol[1].ratios()}
							}
						}
					}
				}
				for _, r := range rects {
					sp := pages[p-1]
					want := geom.ScaledBox(sp.media, sp.crop, r, sp.rot)
					jb := out[o].Media
					if math.Abs(jb[0]-want.LLX) > strictBoxTol || math.Abs(jb[1]-want.LLY) > strictBoxTol || math.Abs(jb[2]-want.URX) > strictBoxTol || math.Abs(jb[3]-want.URY) > strictBoxTol {
						t.Errorf("%s %s output page %d: Java %v, geom.ScaledBox %+v", gf.name, variant, o+1, jb, want)
					}
					o++
				}
			}
			if o != len(out) {
				t.Errorf("%s %s: expected %d output pages, Java wrote %d", gf.name, variant, o, len(out))
			}
		}
	}
}

// End to end: the Go writer with Java's ratios produces Java's page count and boxes.
func TestGoldenWriterMatchesJava(t *testing.T) {
	ctx := context.Background()
	eng := goldenEngine(t)
	for _, gf := range goldenFiles {
		g := loadGolden(t, gf.name)
		src := filepath.Join("../../testdata/pdfs", gf.name+".pdf")
		for _, variant := range []string{"plain", "split_col"} {
			rects := map[int][]geom.Ratios{}
			for _, c := range g.Clusters {
				var rs []geom.Ratios
				if variant == "plain" {
					rs = []geom.Ratios{c.Autocrop.ratios()}
				} else {
					rs = []geom.Ratios{c.SplitCol[0].ratios(), c.SplitCol[1].ratios()}
				}
				if !rs[0].Valid() {
					continue // Java's degenerate rectangles cannot be written by the Go writer (by design)
				}
				for _, p := range c.Pages {
					rects[p] = rs
				}
			}
			dst := filepath.Join(t.TempDir(), gf.name+"_"+variant+".pdf")
			st, err := crop.Write(ctx, src, dst, rects, crop.Options{Password: gf.password})
			if err != nil {
				t.Fatal(err)
			}
			doc, err := eng.Open(ctx, dst, gf.password)
			if err != nil {
				t.Fatal(err)
			}
			out := g.boxes(t, variant)
			o, j := 0, 0
			for p := 1; p <= g.Pages; p++ {
				n := len(rects[p])
				jn := n
				if n == 0 { // page left uncropped by Go; Java wrote one or two degenerate boxes
					n = 1
					jn = 1
					if variant == "split_col" {
						jn = 2
					}
				}
				for k := 0; k < n; k++ {
					if len(rects[p]) > 0 {
						jb := out[j+k].Media
						w, h := jb[2]-jb[0], jb[3]-jb[1]
						if math.Abs(doc.Pages[o].Width-w) > 0.05 || math.Abs(doc.Pages[o].Height-h) > 0.05 {
							t.Errorf("%s %s source page %d copy %d: Go %vx%v, Java %vx%v", gf.name, variant, p, k+1, doc.Pages[o].Width, doc.Pages[o].Height, w, h)
						}
					}
					o++
				}
				j += jn
			}
			if st.OutputPages != len(doc.Pages) {
				t.Errorf("%s %s: stats %d pages, document %d", gf.name, variant, st.OutputPages, len(doc.Pages))
			}
		}
	}
}

type srcBox struct {
	media geom.Rect
	crop  *geom.Rect
	rot   int
}

func sourceBoxes(t *testing.T, path, password string) []srcBox {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	conf := model.NewDefaultConfiguration()
	conf.UserPW, conf.OwnerPW = password, password
	ctx, err := api.ReadContext(context.Background(), f, conf)
	if err != nil {
		t.Fatal(err)
	}
	if err := api.ValidateContext(context.Background(), ctx); err != nil { // fills PageCount
		t.Fatal(err)
	}
	out := make([]srcBox, ctx.PageCount)
	for p := 1; p <= ctx.PageCount; p++ {
		_, _, a, err := ctx.PageDict(context.Background(), p, false)
		if err != nil {
			t.Fatal(err)
		}
		sb := srcBox{media: geom.Rect{LLX: a.MediaBox.LL.X, LLY: a.MediaBox.LL.Y, URX: a.MediaBox.UR.X, URY: a.MediaBox.UR.Y}, rot: geom.NormalizeRotation(a.Rotate)}
		if a.CropBox != nil {
			sb.crop = &geom.Rect{LLX: a.CropBox.LL.X, LLY: a.CropBox.LL.Y, URX: a.CropBox.UR.X, URY: a.CropBox.UR.Y}
		}
		out[p-1] = sb
	}
	return out
}

func ftoa(f float64) string {
	b, _ := json.Marshal(math.Round(f*10000) / 10000)
	return string(b)
}
