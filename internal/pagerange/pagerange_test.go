package pagerange

import (
	"reflect"
	"testing"
)

func TestParseJavaSyntax(t *testing.T) {
	got, err := Parse("1-2;34;3-16", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 17 || !got[1] || !got[16] || !got[34] || got[17] {
		t.Fatalf("got %v", got)
	}
	if s := Format(got); s != "1-16;34" {
		t.Fatalf("format: %s", s)
	}
}

func TestParseExtensionsAndErrors(t *testing.T) {
	got, err := Parse(" 1, 3-4 ;9", 10)
	if err != nil || !reflect.DeepEqual(got, map[int]bool{1: true, 3: true, 4: true, 9: true}) {
		t.Fatalf("got %v err %v", got, err)
	}
	for _, bad := range []string{"a", "5-3", "1-2-3", "0", "11", "-4"} {
		if _, err := Parse(bad, 10); err == nil {
			t.Errorf("%q should fail", bad)
		}
	}
	if got, err := Parse("", 10); err != nil || len(got) != 0 {
		t.Fatalf("empty: %v %v", got, err)
	}
}

func TestAbbrev(t *testing.T) {
	odd := map[int]bool{}
	for p := 1; p <= 289; p += 2 {
		odd[p] = true
	}
	if got := Abbrev(odd, 8); got != "1;3;5 ... 287;289 (145 pages)" {
		t.Fatalf("got %q", got)
	}
	if got := Abbrev(map[int]bool{1: true, 2: true, 3: true, 9: true}, 8); got != "1-3;9" {
		t.Fatalf("short lists stay complete: %q", got)
	}
}
