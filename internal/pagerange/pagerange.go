// Package pagerange parses page lists like "1-2;34;3-16", the syntax of the
// Java Briss PageNumberParser. Commas and spaces are accepted as separators too.
package pagerange

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Parse returns the set of 1-based pages. maxPage, when positive, rejects
// pages beyond the document.
func Parse(s string, maxPage int) (map[int]bool, error) {
	out := map[int]bool{}
	s = strings.TrimSpace(s)
	if s == "" {
		return out, nil
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9') && r != '-' && r != ';' && r != ',' && r != ' ' {
			return nil, fmt.Errorf("pages %q: allowed characters are 0-9, '-', ';' and ','", s)
		}
	}
	for _, tok := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ',' || r == ' ' }) {
		parts := strings.Split(tok, "-")
		switch len(parts) {
		case 1:
			p, err := page(parts[0], tok, maxPage)
			if err != nil {
				return nil, err
			}
			out[p] = true
		case 2:
			a, err := page(parts[0], tok, maxPage)
			if err != nil {
				return nil, err
			}
			b, err := page(parts[1], tok, maxPage)
			if err != nil {
				return nil, err
			}
			if a > b {
				return nil, fmt.Errorf("pages %q: end must not be smaller than start", tok)
			}
			for p := a; p <= b; p++ {
				out[p] = true
			}
		default:
			return nil, fmt.Errorf("pages %q: too many '-' characters", tok)
		}
	}
	return out, nil
}

func page(s, tok string, maxPage int) (int, error) {
	p, err := strconv.Atoi(s)
	if err != nil || p < 1 {
		return 0, fmt.Errorf("pages %q: %q is not a page number", tok, s)
	}
	if maxPage > 0 && p > maxPage {
		return 0, fmt.Errorf("pages %q: page %d is beyond the last page %d", tok, p, maxPage)
	}
	return p, nil
}

// Format writes a sorted set back as compact ranges, for example "1-3;7".
func Format(pages map[int]bool) string {
	var ps []int
	for p, ok := range pages {
		if ok {
			ps = append(ps, p)
		}
	}
	sort.Ints(ps)
	var b strings.Builder
	for i := 0; i < len(ps); {
		j := i
		for j+1 < len(ps) && ps[j+1] == ps[j]+1 {
			j++
		}
		if b.Len() > 0 {
			b.WriteByte(';')
		}
		if j > i {
			fmt.Fprintf(&b, "%d-%d", ps[i], ps[j])
		} else {
			fmt.Fprintf(&b, "%d", ps[i])
		}
		i = j + 1
	}
	return b.String()
}

// Abbrev formats a page set for people: compact ranges, shortened to the first
// three and last two items with the page count when there are more than max.
func Abbrev(pages map[int]bool, max int) string {
	full := Format(pages)
	parts := strings.Split(full, ";")
	if len(parts) <= max || len(parts) < 6 {
		return full
	}
	n := 0
	for _, ok := range pages {
		if ok {
			n++
		}
	}
	return fmt.Sprintf("%s ... %s (%d pages)", strings.Join(parts[:3], ";"), strings.Join(parts[len(parts)-2:], ";"), n)
}
