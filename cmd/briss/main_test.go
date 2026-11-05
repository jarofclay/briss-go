package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jarofclay/briss-go/internal/pdf"
)

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
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

// copyPDF copies a test PDF into a temp dir so the default destination lands there.
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

func TestLegacyDefaultDestination(t *testing.T) {
	src := copyPDF(t, "example.pdf")
	code, out, errs := runCLI(t, "-s", src)
	if code != exitOK {
		t.Fatalf("exit %d\n%s\n%s", code, out, errs)
	}
	dst := strings.TrimSuffix(src, ".pdf") + "_cropped.pdf"
	ps := pages(t, dst, "")
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
	code, out, errs := runCLI(t, "-S", src, "-D", dst, "--SPLIT-COL")
	if code != exitOK {
		t.Fatalf("exit %d\n%s\n%s", code, out, errs)
	}
	if n := len(pages(t, dst, "")); n != 56 {
		t.Fatalf("28 pages split into columns should give 56, got %d", n)
	}
}

func TestLegacyDestinationDirectory(t *testing.T) {
	src := copyPDF(t, "example.pdf")
	dir := t.TempDir()
	if code, out, errs := runCLI(t, "-s", src, "-d", dir); code != exitOK {
		t.Fatalf("exit %d\n%s\n%s", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "example_cropped.pdf")); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyPassword(t *testing.T) {
	src := copyPDF(t, "example-protected.pdf")
	if code, _, errs := runCLI(t, "-s", src); code != exitInput || !strings.Contains(errs, "-p") {
		t.Fatalf("missing password: exit %d, %q", code, errs)
	}
	if code, _, _ := runCLI(t, "-s", src, "-p", "nope"); code != exitInput {
		t.Fatalf("wrong password: exit %d", code)
	}
	if code, out, errs := runCLI(t, "-s", src, "-p", "secret"); code != exitOK {
		t.Fatalf("right password: exit %d\n%s\n%s", code, out, errs)
	}
	pages(t, strings.TrimSuffix(src, ".pdf")+"_cropped.pdf", "secret")
}

func TestUsageErrors(t *testing.T) {
	if code, _, _ := runCLI(t); code != exitUsage {
		t.Fatalf("no arguments: exit %d", code)
	}
	if code, _, errs := runCLI(t, "book.pdf"); code != exitUsage || !strings.Contains(errs, "graphical editor") {
		t.Fatalf("single file argument: exit %d %q", code, errs)
	}
	if code, _, _ := runCLI(t, "-s", "/does/not/exist.pdf"); code != exitInput {
		t.Fatalf("missing source: exit %d", code)
	}
	if code, out, _ := runCLI(t, "--version"); code != exitOK || !strings.HasPrefix(out, "briss ") {
		t.Fatalf("version: exit %d %q", code, out)
	}
	src := copyPDF(t, "example.pdf")
	if code, _, _ := runCLI(t, "-s", src, "-d", src); code != exitInput {
		t.Fatalf("destination equal to source must be refused, exit %d", code)
	}
}
