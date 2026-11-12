package main

import (
	"reflect"
	"testing"
)

func TestSplitNUL(t *testing.T) {
	got := splitNUL([]byte("/a/b.pdf\x00/c d/e.PDF\x00/books\x00"))
	want := []string{"/a/b.pdf", "/c d/e.PDF", "/books"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitNUL = %q, want %q", got, want)
	}
	if got := splitNUL(nil); got != nil {
		t.Fatalf("splitNUL(nil) = %q", got)
	}
}

func TestFileURIPaths(t *testing.T) {
	got := fileURIPaths([]string{"file:///home/a/b c.pdf", "https://example.org/x.pdf", "file://", "file:///d.pdf"})
	want := []string{"/home/a/b c.pdf", "/d.pdf"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fileURIPaths = %q, want %q", got, want)
	}
}
