//go:build !pdfium_cgo

package pdf

import (
	"context"
	"os"
	"testing"
)

// The Windows desktop app is built without a console, so its standard output
// and error are not usable files; the engine must start and open PDFs anyway.
// A closed file stands in for those handles here.
func TestEngineWithoutUsableStdout(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = f, f
	e, err := NewEngine(EngineOptions{Workers: 1})
	os.Stdout, os.Stderr = stdout, stderr
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	doc, err := e.Open(context.Background(), "../../testdata/pdfs/example.pdf", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Pages) == 0 {
		t.Fatal("no pages")
	}
}
