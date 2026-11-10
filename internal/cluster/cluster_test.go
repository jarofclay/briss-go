package cluster

import (
	"reflect"
	"testing"
)

func TestBuildParityAndSize(t *testing.T) {
	letter := PageSize{612, 792}
	a4 := PageSize{595.28, 841.89}
	pages := []PageSize{letter, letter, letter, letter, a4, a4, {618, 795}, {792, 612}}
	cs := Build(pages, Options{})
	got := [][]int{}
	for _, c := range cs {
		got = append(got, c.Pages)
	}
	want := [][]int{{1, 3, 7}, {2, 4}, {5}, {6}, {8}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if cs[0].Even || !cs[1].Even {
		t.Fatal("parity flags wrong")
	}
}

// Java's 20 pt buckets split 599 and 601 into different clusters; the
// tolerance window keeps them together.
func TestBuildNoBucketEdge(t *testing.T) {
	cs := Build([]PageSize{{599, 800}, {600, 800}, {601, 800}}, Options{})
	if len(cs) != 2 || !reflect.DeepEqual(cs[0].Pages, []int{1, 3}) {
		t.Fatalf("got %d clusters, first %v", len(cs), cs[0].Pages)
	}
}

func TestBuildExcluded(t *testing.T) {
	letter := PageSize{612, 792}
	cs := Build([]PageSize{letter, letter, letter, letter, letter}, Options{Exclude: map[int]bool{3: true}})
	got := [][]int{}
	for _, c := range cs {
		got = append(got, c.Pages)
	}
	want := [][]int{{1, 5}, {2, 4}, {3}}
	if !reflect.DeepEqual(got, want) || !cs[2].Excluded {
		t.Fatalf("got %v want %v (excluded flag %v)", got, want, cs[2].Excluded)
	}
}

func TestSampleMatchesJava(t *testing.T) {
	pages := func(n int) []int {
		p := make([]int, n)
		for i := range p {
			p[i] = 2*i + 1
		}
		return p
	}
	if got := Sample(pages(3)); !reflect.DeepEqual(got, []int{1, 3, 5}) {
		t.Fatalf("small cluster: %v", got)
	}
	if got := Sample(pages(15)); len(got) != 15 || got[14] != 29 {
		t.Fatalf("15 pages: %v", got)
	}
	// 145 odd pages of a 290-page book. Indexes evaluated with the Java
	// expression in jshell: float32 accumulation gives 115 (not 116) at step 12.
	got := Sample(pages(145))
	idx := []int{0, 9, 19, 29, 38, 48, 58, 67, 77, 87, 96, 106, 115, 125, 135}
	for i, k := range idx {
		if got[i] != 2*k+1 {
			t.Fatalf("sample %d: got page %d want %d (all: %v)", i, got[i], 2*k+1, got)
		}
	}
}

func TestForPage(t *testing.T) {
	cs := Build([]PageSize{{612, 792}, {612, 792}, {612, 792}}, Options{})
	if c := ForPage(cs, 3); c == nil || c.ID != 0 {
		t.Fatal("page 3 should be in cluster 0")
	}
	if ForPage(cs, 9) != nil {
		t.Fatal("page 9 does not exist")
	}
}
