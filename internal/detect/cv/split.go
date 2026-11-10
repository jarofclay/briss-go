package cv

import "github.com/jarofclay/briss-go/internal/detect"

// splitBlock cuts a page's content block (x0, y0, x1, y1, inclusive pixels)
// into two columns, two rows, or both, when the user asked for it: the
// columns first, each then into a top and a bottom part, in reading order.
// Every part is tightened to the content on its side of the cut.
func splitBlock(b [4]int, cs []comp, opts detect.Options) [][4]int {
	out := [][4]int{b}
	if opts.SplitColumns {
		var next [][4]int
		for _, r := range out {
			next = append(next, cutBlock(r, cs, true)...)
		}
		out = next
	}
	if opts.SplitRows {
		var next [][4]int
		for _, r := range out {
			next = append(next, cutBlock(r, cs, false)...)
		}
		out = next
	}
	return out
}

// cutBlock cuts block b in two across columns (cols) or rows. The cut goes
// through the widest stretch without ink in the middle 40% of the block,
// the one nearest the middle when several are as wide: between two text
// columns, or between paragraphs or lines, so no line is cut in half. With
// no such stretch (one column of text cut into columns) it cuts the middle.
func cutBlock(b [4]int, cs []comp, cols bool) [][4]int {
	a0, a1 := b[1], b[3] // the axis being cut
	if cols {
		a0, a1 = b[0], b[2]
	}
	n := a1 - a0 + 1
	if n < 8 {
		return [][4]int{b}
	}
	inside := func(c comp) bool { return c.cx() >= b[0] && c.cx() <= b[2] && c.cy() >= b[1] && c.cy() <= b[3] }
	ink := make([]bool, n)
	for _, c := range cs {
		if !inside(c) {
			continue
		}
		lo, hi := c.y0, c.y1
		if cols {
			lo, hi = c.x0, c.x1
		}
		for v := max(lo, a0); v <= min(hi, a1); v++ {
			ink[v-a0] = true
		}
	}
	lo, hi := n*3/10, n*7/10
	mid := n / 2
	cut, best, bestDist := mid, 0, n
	for i := lo; i <= hi; {
		if ink[i] {
			i++
			continue
		}
		j := i
		for j <= hi && !ink[j] {
			j++
		}
		c := (i + j) / 2
		d := abs(c - mid)
		if j-i > best || (j-i == best && d < bestDist) {
			cut, best, bestDist = c, j-i, d
		}
		i = j
	}
	cut += a0
	// each side keeps the marks whose centre lies on it, whole: a glyph the
	// cut goes through is not lost, it just shows on both sides
	var out [][4]int
	for side := 0; side < 2; side++ {
		lo, hi, found := 0, 0, false
		for _, c := range cs {
			if !inside(c) {
				continue
			}
			cc, clo, chi := c.cy(), c.y0, c.y1
			if cols {
				cc, clo, chi = c.cx(), c.x0, c.x1
			}
			if (side == 0) != (cc < cut) {
				continue
			}
			if !found {
				lo, hi, found = clo, chi, true
			}
			lo, hi = min(lo, clo), max(hi, chi)
		}
		if !found {
			continue // nothing on this side
		}
		part := b
		if cols {
			part[0], part[2] = max(b[0], lo), min(b[2], hi)
		} else {
			part[1], part[3] = max(b[1], lo), min(b[3], hi)
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return [][4]int{b}
	}
	return out
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
