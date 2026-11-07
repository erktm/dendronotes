package collab

import "strings"

// A note changed on disk while people were editing it: merge3 combines the
// two lines of development line by line. base is the version both started
// from, ours the live document, theirs the file. Where both changed the same
// lines differently, both versions are kept, ours first – nothing is lost.

// maxDiffEdits bounds the work of one diff; beyond it, the whole differing
// region counts as one change.
const maxDiffEdits = 1000

// hunk replaces base lines [o0, o1) with lines [x0, x1) of the other side.
type hunk struct{ o0, o1, x0, x1 int }

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.SplitAfter(s, "\n")
}

// merge3 returns the merged text and whether conflicting changes were kept
// side by side.
func merge3(base, ours, theirs string) (string, bool) {
	switch {
	case ours == theirs, theirs == base:
		return ours, false
	case ours == base:
		return theirs, false
	}
	o, a, b := splitLines(base), splitLines(ours), splitLines(theirs)
	ha, hb := diffLines(o, a), diffLines(o, b)

	type tagged struct {
		hunk
		mine bool
	}
	all := make([]tagged, 0, len(ha)+len(hb))
	i, j := 0, 0
	for i < len(ha) || j < len(hb) {
		if j >= len(hb) || (i < len(ha) && (ha[i].o0 < hb[j].o0 || (ha[i].o0 == hb[j].o0 && ha[i].o1 <= hb[j].o1))) {
			all = append(all, tagged{ha[i], true})
			i++
		} else {
			all = append(all, tagged{hb[j], false})
			j++
		}
	}

	var out strings.Builder
	conflict := false
	pos := 0               // next base line to copy
	shiftA, shiftB := 0, 0 // line offset of ours/theirs against base before pos
	for k := 0; k < len(all); {
		gs, ge := all[k].o0, all[k].o1
		group := []tagged{all[k]}
		for k++; k < len(all) && (all[k].o0 < ge || all[k].o0 == gs); k++ {
			group = append(group, all[k])
			ge = max(ge, all[k].o1)
		}
		for _, l := range o[pos:gs] {
			out.WriteString(l)
		}
		da, db := 0, 0
		hasA, hasB := false, false
		for _, h := range group {
			d := (h.x1 - h.x0) - (h.o1 - h.o0)
			if h.mine {
				da += d
				hasA = true
			} else {
				db += d
				hasB = true
			}
		}
		aStart, bStart := gs+shiftA, gs+shiftB
		va := strings.Join(a[aStart:aStart+(ge-gs)+da], "")
		vb := strings.Join(b[bStart:bStart+(ge-gs)+db], "")
		switch {
		case !hasB:
			out.WriteString(va)
		case !hasA, va == vb:
			out.WriteString(vb)
		default:
			conflict = true
			out.WriteString(va)
			if va != "" && !strings.HasSuffix(va, "\n") && vb != "" {
				out.WriteString("\n")
			}
			out.WriteString(vb)
		}
		shiftA += da
		shiftB += db
		pos = ge
	}
	for _, l := range o[pos:] {
		out.WriteString(l)
	}
	return out.String(), conflict
}

// diffLines returns the changes from o to x as hunks in base order.
func diffLines(o, x []string) []hunk {
	// Common prefix and suffix need no search.
	pre := 0
	for pre < len(o) && pre < len(x) && o[pre] == x[pre] {
		pre++
	}
	suf := 0
	for suf < len(o)-pre && suf < len(x)-pre && o[len(o)-1-suf] == x[len(x)-1-suf] {
		suf++
	}
	oo, xx := o[pre:len(o)-suf], x[pre:len(x)-suf]
	if len(oo) == 0 && len(xx) == 0 {
		return nil
	}
	pairs, ok := myers(oo, xx)
	if !ok {
		return []hunk{{pre, pre + len(oo), pre, pre + len(xx)}}
	}
	// Turn matched pairs into the hunks between them.
	var hs []hunk
	i, j := 0, 0
	for _, p := range append(pairs, [2]int{len(oo), len(xx)}) {
		if p[0] > i || p[1] > j {
			hs = append(hs, hunk{pre + i, pre + p[0], pre + j, pre + p[1]})
		}
		i, j = p[0]+1, p[1]+1
	}
	return hs
}

// myers finds a longest common subsequence of a and b (Myers' O(ND)
// algorithm) and returns its index pairs; ok is false beyond maxDiffEdits.
func myers(a, b []string) ([][2]int, bool) {
	n, m := len(a), len(b)
	maxD := min(n+m, maxDiffEdits)
	off := maxD + 1
	v := make([]int, 2*maxD+3)
	var trace [][]int
	for d := 0; d <= maxD; d++ {
		snap := make([]int, 2*d+1)
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
				x = v[off+k+1]
			} else {
				x = v[off+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[off+k] = x
			if x >= n && y >= m {
				for kk := -d; kk <= d; kk++ {
					snap[kk+d] = v[off+kk]
				}
				trace = append(trace, snap)
				return backtrack(trace, a, b), true
			}
		}
		for kk := -d; kk <= d; kk++ {
			snap[kk+d] = v[off+kk]
		}
		trace = append(trace, snap)
	}
	return nil, false
}

func backtrack(trace [][]int, a, b []string) [][2]int {
	x, y := len(a), len(b)
	var pairs [][2]int
	for d := len(trace) - 1; d > 0; d-- {
		prev := trace[d-1]
		k := x - y
		at := func(kk int) int { return prev[kk+(d-1)] }
		var pk int
		if k == -d || (k != d && at(k-1) < at(k+1)) {
			pk = k + 1
		} else {
			pk = k - 1
		}
		px := at(pk)
		py := px - pk
		for x > px && y > py {
			x--
			y--
			pairs = append(pairs, [2]int{x, y})
		}
		x, y = px, py
	}
	for x > 0 && y > 0 {
		x--
		y--
		pairs = append(pairs, [2]int{x, y})
	}
	for i, j := 0, len(pairs)-1; i < j; i, j = i+1, j-1 {
		pairs[i], pairs[j] = pairs[j], pairs[i]
	}
	return pairs
}
