//go:build !pdfium_cgo

package pdf

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
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

// A compilation cache that cannot be used (on Windows another process may be
// writing the same entry, and the rename fails) must not stop the engine: it
// starts without the cache. Here every cache entry is replaced by a directory.
func TestEngineSurvivesABrokenCache(t *testing.T) {
	dir := t.TempDir()
	e, err := NewEngine(EngineOptions{Workers: 1, CacheDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	e.Close()
	broken := 0
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if err := os.Remove(p); err != nil {
			return err
		}
		broken++
		return os.Mkdir(p, 0o755)
	})
	if err != nil || broken == 0 {
		t.Fatalf("could not break the cache (%d entries): %v", broken, err)
	}
	e, err = NewEngine(EngineOptions{Workers: 1, CacheDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if _, err := e.Open(context.Background(), "../../testdata/pdfs/example.pdf", ""); err != nil {
		t.Fatal(err)
	}
}
