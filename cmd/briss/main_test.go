package main

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jarofclay/briss-go/internal/app"
	"github.com/jarofclay/briss-go/internal/cropdef"
	"github.com/jarofclay/briss-go/internal/pdf"
)

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	return runCLIStdin(t, "", args...)
}

func runCLIStdin(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

func pages(t *testing.T, path, password string) []pdf.PageInfo {
	t.Helper()
	eng, err := pdf.NewEngine(pdf.EngineOptions{Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	doc, err := eng.Open(context.Background(), path, password)
	if err != nil {
		t.Fatal(err)
	}
	return doc.Pages
}

// copyPDF copies a test PDF into a temp dir so default destinations land there.
func copyPDF(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("../../testdata/pdfs", name))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func mustOK(t *testing.T, code int, out, errs string) {
	t.Helper()
	if code != exitOK {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, out, errs)
	}
}

// runOK runs the CLI and fails the test unless it exits 0.
func runOK(t *testing.T, args ...string) string {
	t.Helper()
	code, out, errs := runCLI(t, args...)
	mustOK(t, code, out, errs)
	return out
}

// ---- legacy command line

func TestLegacyDefaultDestination(t *testing.T) {
	src := copyPDF(t, "example.pdf")
	code, out, errs := runCLI(t, "-s", src)
	mustOK(t, code, out, errs)
	ps := pages(t, strings.TrimSuffix(src, ".pdf")+"_cropped.pdf", "")
	if len(ps) != 1 || ps[0].Width >= 595 || ps[0].Height >= 841 {
		t.Fatalf("expected one cropped page, got %+v", ps)
	}
	if !strings.Contains(out, "Cropping successful") {
		t.Fatalf("output:\n%s", out)
	}
}

func TestLegacyFlagsCaseInsensitiveAndSplit(t *testing.T) {
	src := copyPDF(t, "CREATIVE_COMMONS.pdf")
	dst := filepath.Join(t.TempDir(), "out.pdf")
	runOK(t, "-S", src, "-D", dst, "--SPLIT-COL")
	if n := len(pages(t, dst, "")); n != 56 {
		t.Fatalf("28 pages split into columns should give 56, got %d", n)
	}
}

func TestLegacyDestinationDirectoryAndPassword(t *testing.T) {
	src := copyPDF(t, "example-protected.pdf")
	if code, _, errs := runCLI(t, "-s", src); code != exitInput || !strings.Contains(errs, "-p") {
		t.Fatalf("missing password: exit %d, %q", code, errs)
	}
	if code, _, _ := runCLI(t, "-s", src, "-p", "nope"); code != exitInput {
		t.Fatalf("wrong password: exit %d", code)
	}
	dir := t.TempDir()
	runOK(t, "-s", src, "-p", "secret", "-d", dir)
	pages(t, filepath.Join(dir, "example-protected_cropped.pdf"), "secret")
}

// The legacy flags and `crop --detector legacy` must produce the same pages.
func TestLegacyMatchesCropSubcommand(t *testing.T) {
	src := copyPDF(t, "book300.pdf")
	dir := t.TempDir()
	a, b := filepath.Join(dir, "legacy.pdf"), filepath.Join(dir, "crop.pdf")
	runOK(t, "-s", src, "-d", a, "--split-col")
	runOK(t, "crop", src, "-o", b, "--detector", "legacy", "--split-col", "-q")
	pa, pb := pages(t, a, ""), pages(t, b, "")
	if len(pa) != len(pb) {
		t.Fatalf("page counts differ: %d vs %d", len(pa), len(pb))
	}
	for i := range pa {
		if pa[i].Width != pb[i].Width || pa[i].Height != pb[i].Height {
			t.Fatalf("page %d differs: %+v vs %+v", i+1, pa[i], pb[i])
		}
	}
}

// ---- subcommands

func TestCropJSONReport(t *testing.T) {
	src := copyPDF(t, "example.pdf")
	out := filepath.Join(t.TempDir(), "out.pdf")
	code, stdout, errs := runCLI(t, "crop", src, "-o", out, "--json")
	mustOK(t, code, stdout, errs)
	var rep app.Report
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout)
	}
	if rep.Output != out || rep.SourcePages != 1 || rep.OutputPages != 1 || len(rep.Clusters) != 1 || rep.Detector != "legacy@1" {
		t.Fatalf("report %+v", rep)
	}
}

func TestDryRunWritesNoPDF(t *testing.T) {
	src := copyPDF(t, "example.pdf")
	crops := filepath.Join(t.TempDir(), "c.json")
	runOK(t, "crop", src, "--dry-run", "--save-crops", crops)
	if _, err := os.Stat(strings.TrimSuffix(src, ".pdf") + "_cropped.pdf"); !os.IsNotExist(err) {
		t.Fatal("dry run must not write a PDF")
	}
	if _, err := cropdef.Load(crops); err != nil {
		t.Fatal(err)
	}
}

func TestDetectSaveThenApply(t *testing.T) {
	src := copyPDF(t, "CREATIVE_COMMONS.pdf")
	dir := t.TempDir()
	crops := filepath.Join(dir, "cc.crops.json")
	runOK(t, "detect", src, "--save-crops", crops, "--split-col")
	out := filepath.Join(dir, "out.pdf")
	code, stdout, errs := runCLI(t, "apply", src, "--crops", crops, "-o", out)
	mustOK(t, code, stdout, errs)
	if n := len(pages(t, out, "")); n != 56 {
		t.Fatalf("applied split definition should give 56 pages, got %d", n)
	}
	// edit the definition: page 1 uncropped, page 2 cropped hard
	def, _ := cropdef.Load(crops)
	def.PageOverrides = map[int][]cropdef.Rect{1: {}, 2: {{L: 0.25, B: 0.25, R: 0.25, T: 0.25}}}
	def.Save(crops)
	runOK(t, "apply", src, "--crops", crops, "-o", out, "-q")
	ps := pages(t, out, "")
	if len(ps) != 54 || math.Abs(ps[0].Width-960.12) > 0.01 || math.Abs(ps[1].Width-480.06) > 0.01 {
		t.Fatalf("overrides not applied: %d pages, first %+v second %+v", len(ps), ps[0], ps[1])
	}
}

func TestApplyChecksTheFile(t *testing.T) {
	cc := copyPDF(t, "CREATIVE_COMMONS.pdf")
	ex := copyPDF(t, "example.pdf")
	dir := t.TempDir()
	crops := filepath.Join(dir, "ex.crops.json")
	runOK(t, "detect", ex, "--save-crops", crops)
	if code, _, errs := runCLI(t, "apply", cc, "--crops", crops, "-o", filepath.Join(dir, "x.pdf")); code != exitInput || !strings.Contains(errs, "1-page") {
		t.Fatalf("page-count mismatch must fail with exit 2: %d %q", code, errs)
	}
	other := filepath.Join(dir, "other.pdf")
	b, _ := os.ReadFile(ex)
	os.WriteFile(other, append(b, '\n'), 0o644) // same pages, different bytes
	if code, _, errs := runCLI(t, "apply", other, "--crops", crops, "-o", filepath.Join(dir, "y.pdf")); code != exitInput || !strings.Contains(errs, "--force") {
		t.Fatalf("different file must need --force: %d %q", code, errs)
	}
	code, out, errs := runCLI(t, "apply", other, "--crops", crops, "-o", filepath.Join(dir, "y.pdf"), "--force")
	mustOK(t, code, out, errs)
	if !strings.Contains(out, "warning:") {
		t.Fatalf("--force should warn:\n%s", out)
	}
}

func TestMarginGrowsPages(t *testing.T) {
	src := copyPDF(t, "example.pdf")
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.pdf"), filepath.Join(dir, "b.pdf")
	runOK(t, "crop", src, "-o", a, "-q")
	runOK(t, "crop", src, "-o", b, "-q", "--margin", "10pt")
	pa, pb := pages(t, a, "")[0], pages(t, b, "")[0]
	if math.Abs((pb.Width-pa.Width)-20) > 0.05 || math.Abs((pb.Height-pa.Height)-20) > 0.05 {
		t.Fatalf("10pt margin should add 20pt per side pair: %+v -> %+v", pa, pb)
	}
}

func TestPasswordStdinAndExclude(t *testing.T) {
	src := copyPDF(t, "example-protected.pdf")
	out := filepath.Join(t.TempDir(), "o.pdf")
	code0, out0, errs0 := runCLIStdin(t, "secret\n", "crop", src, "-o", out, "--password-stdin", "-q")
	mustOK(t, code0, out0, errs0)
	book := copyPDF(t, "CREATIVE_COMMONS.pdf")
	code, stdout, errs := runCLI(t, "detect", book, "--exclude", "1;2", "--json")
	mustOK(t, code, stdout, errs)
	var def cropdef.Definition
	json.Unmarshal([]byte(stdout), &def)
	if len(def.Clusters) != 4 || len(def.ExcludedPages) != 2 {
		t.Fatalf("two excluded pages make two extra clusters: %d clusters, excluded %v", len(def.Clusters), def.ExcludedPages)
	}
}

func TestPreviewWritesImages(t *testing.T) {
	src := copyPDF(t, "CREATIVE_COMMONS.pdf")
	dir := t.TempDir()
	code, out, errs := runCLI(t, "preview", src, "--out", dir)
	mustOK(t, code, out, errs)
	files, _ := filepath.Glob(filepath.Join(dir, "*.png"))
	if len(files) != 2 {
		t.Fatalf("expected 2 previews, got %v", files)
	}
	if code, _, _ := runCLI(t, "preview", src, "--out", dir, "--cluster", "7"); code != exitInput {
		t.Fatalf("missing cluster: exit %d", code)
	}

	// a saved crop definition drawn on chosen pages, no detection
	crops := filepath.Join(t.TempDir(), "cc.crops.json")
	runOK(t, "detect", src, "--save-crops", crops)
	pagesDir := t.TempDir()
	code, out, errs = runCLI(t, "preview", src, "--crops", crops, "--pages", "1-2", "--out", pagesDir)
	mustOK(t, code, out, errs)
	if files, _ := filepath.Glob(filepath.Join(pagesDir, "*_page*.png")); len(files) != 2 {
		t.Fatalf("expected 2 page images, got %v", files)
	}
	other := copyPDF(t, "example.pdf")
	if code, _, _ := runCLI(t, "preview", other, "--crops", crops); code != exitInput {
		t.Fatalf("crop definition for another document: exit %d", code)
	}
}

func TestBench(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"example.pdf", "CREATIVE_COMMONS.pdf"} {
		b, _ := os.ReadFile(filepath.Join("../../testdata/pdfs", n))
		os.WriteFile(filepath.Join(dir, n), b, 0o644)
	}
	// label example.pdf with its own detection, shifted so one side clips
	label := filepath.Join(dir, "example.crops.json")
	runOK(t, "detect", filepath.Join(dir, "example.pdf"), "--save-crops", label)
	def, _ := cropdef.Load(label)
	def.Clusters[0].Rects[0].L -= 0.05 // the label keeps 5% more on the left
	def.Save(label)
	report := filepath.Join(dir, "bench.md")
	code, out, errs := runCLI(t, "bench", "--dataset", dir, "--report", report, "-q")
	mustOK(t, code, out, errs)
	if !strings.Contains(out, "| auto | 1 | 0 | 1 |") || !strings.Contains(out, "clipped") || !strings.Contains(out, "1 PDFs without") {
		t.Fatalf("report:\n%s", out)
	}
	if _, err := os.Stat(report); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runCLI(t, "bench", "--dataset", t.TempDir()); code != exitInput {
		t.Fatalf("empty dataset: exit %d", code)
	}

	// precomputed predictions: the label itself scores perfectly, a missing
	// directory is a usage error
	pred := t.TempDir()
	b, _ := os.ReadFile(label)
	os.WriteFile(filepath.Join(pred, "example.crops.json"), b, 0o644)
	code, out, errs = runCLI(t, "bench", "--dataset", dir, "--detectors", "dir:"+pred, "--json")
	mustOK(t, code, out, errs)
	var rep struct {
		Summaries []struct {
			Detector   string  `json:"detector"`
			AcceptRate float64 `json:"accept_rate"`
		} `json:"summaries"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil || len(rep.Summaries) != 1 || rep.Summaries[0].AcceptRate != 1 {
		t.Fatalf("dir: detector: %v\n%s", err, out)
	}
	if code, _, _ := runCLI(t, "bench", "--dataset", dir, "--detectors", "dir:"+filepath.Join(pred, "nope")); code != exitInput {
		t.Fatalf("missing prediction dir: exit %d", code)
	}
}

func TestExitCodes(t *testing.T) {
	src := copyPDF(t, "example.pdf")
	cases := []struct {
		args []string
		want int
	}{
		{nil, exitOK}, // help
		{[]string{"book.pdf"}, exitUsage},
		{[]string{"crop"}, exitUsage},
		{[]string{"crop", src, "--bogus"}, exitUsage},
		{[]string{"apply", src}, exitUsage}, // --crops required
		{[]string{"frobnicate", "x", "y"}, exitUsage},
		{[]string{"crop", "/does/not/exist.pdf"}, exitInput},
		{[]string{"crop", src, "--detector", "bogus"}, exitInput},
		{[]string{"crop", src, "--exclude", "999"}, exitInput},
		{[]string{"crop", src, "--margin", "3furlongs"}, exitInput},
		{[]string{"crop", src, "--detector", "claude"}, exitDetection},
		{[]string{"crop", src, "-o", filepath.Join(src, "under-a-file", "out.pdf")}, exitWriteError}, // a folder inside a file cannot be made on any system
		{[]string{"--version"}, exitOK},
		{[]string{"version"}, exitOK},
		{[]string{"-s"}, exitUsage},
		{[]string{"-s", "/does/not/exist.pdf"}, exitInput},
	}
	for _, c := range cases {
		code, _, errs := runCLI(t, c.args...)
		if code != c.want {
			t.Errorf("%v: exit %d, want %d (%s)", c.args, code, c.want, strings.TrimSpace(errs))
		}
	}
}

// --normalize gives every output page of the same source size one size;
// --straighten and --ocr leave a born-digital book alone.
func TestCropNormalize(t *testing.T) {
	src := copyPDF(t, "book300.pdf")
	dst := filepath.Join(t.TempDir(), "out.pdf")
	code, out, errs := runCLI(t, "crop", src, "-o", dst, "--normalize", "--straighten", "--ocr", "--json")
	mustOK(t, code, out, errs)
	var rep struct {
		Straightened int  `json:"straightened"`
		Normalized   bool `json:"normalized"`
		OCRPages     int  `json:"ocr_pages"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil || !rep.Normalized || rep.Straightened != 0 || rep.OCRPages != 0 {
		t.Fatalf("report %+v %v\n%s", rep, err, out)
	}
	eng, err := pdf.NewEngine(pdf.EngineOptions{Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	doc, err := eng.Open(context.Background(), dst, "")
	if err != nil {
		t.Fatal(err)
	}
	sizes := map[[2]int]int{}
	for _, p := range doc.Pages {
		sizes[[2]int{int(math.Round(p.Width)), int(math.Round(p.Height))}]++
	}
	// portrait odd, portrait even and the landscape spreads: at most a few sizes
	if len(sizes) > 4 {
		t.Fatalf("too many page sizes after --normalize: %v", sizes)
	}
}
